package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAuditPerRecordBoundsFromActualOperations(t *testing.T) {
	tests := []struct {
		contract map[string]any
		inputs   map[string]any
		credits  int
	}{
		{
			map[string]any{"id": "firecrawl-developer-index/search", "creditsCost": 2, "perRecord": true,
				"options": []any{map[string]any{"name": "k", "type": "number", "default": 10}}},
			map[string]any{"query": "Rust async programming", "k": float64(2)}, 4,
		},
		{
			map[string]any{"id": "fullenrich/companies/lookup", "creditsCost": 5, "perRecord": true,
				"response": map[string]any{"paginated": false}, "options": []any{map[string]any{"name": "domain", "type": "string"}}},
			map[string]any{"domain": "firecrawl.dev"}, 5,
		},
		{
			map[string]any{"id": "wikipedia/entries/lookup", "creditsCost": 2, "perRecord": true,
				"options": []any{map[string]any{"name": "ids", "type": "array"}}},
			map[string]any{"ids": []any{"A", "B"}}, 4,
		},
	}
	for _, tt := range tests {
		got, err := expectedCallCredits(tt.contract, tt.inputs)
		if err != nil || got != tt.credits {
			t.Errorf("%s: got %d (%v), want %d", tt.contract["id"], got, err, tt.credits)
		}
	}
}

func TestAuditPaginationAdaptersPreserveFilters(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		data   map[string]any
		param  string
		want   any
	}{
		{"treasury-link", map[string]any{"page": float64(1), "fields": "date,rate"},
			map[string]any{"links": map[string]any{"next": "https://fiscaldata.treasury.gov/api?page[number]=2&page[size]=1"}},
			"page", 2},
		{"clinical-trials", map[string]any{"query": "recruiting diabetes", "page_size": float64(2)},
			map[string]any{"next_page_token": "next-study-page"}, "page_token", "next-study-page"},
		{"semantic-scholar", map[string]any{"query": "CRISPR papers"},
			map[string]any{"continuation": "2@"}, "continuation", "2@"},
		{"fullenrich", map[string]any{"query": "finance firms", "limit": float64(1)},
			map[string]any{"metadata": map[string]any{"search_after": "WzAsNjAyNjE2MCJd"}}, "search_after", "WzAsNjAyNjE2MCJd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := continuationForDataResult("example/search", tt.inputs, tt.data)
			if next == nil {
				t.Fatal("missing continuation")
			}
			args := mapField(mapField(next)["arguments"])
			call := mapField(listField(args["calls"])[0])
			parameters := mapField(call["inputs"])
			if parameters[tt.param] != tt.want {
				t.Fatalf("missing next %s=%v: %+v", tt.param, tt.want, parameters)
			}
			for k, v := range tt.inputs {
				if k != tt.param && parameters[k] != v {
					t.Fatalf("lost original filter %s=%v", k, v)
				}
			}
			if tt.inputs[tt.param] == tt.want {
				t.Fatal("test did not advance cursor")
			}
		})
	}
}

func TestAuditSourceMapIsBoundedAndFilterable(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	t.Setenv("FIRECRAWL_API_KEY", "test-map")
	var links []any
	for i := 0; i < 2803; i++ {
		path := "https://docs.firecrawl.dev/features/page/" + strconv.Itoa(i)
		if i%4 == 0 {
			path = "https://docs.firecrawl.dev/ja/features/page/" + strconv.Itoa(i)
		}
		links = append(links, map[string]any{"url": path, "title": "Guide " + strconv.Itoa(i), "description": "Overview of API features"})
	}
	mock := map[string]any{"success": true, "links": links}
	body, _ := json.Marshal(mock)
	calls := 0
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v2/map" {
			return nil, errors.New("unexpected path")
		}
		return testHTTPResponse(req, 200, string(body), nil), nil
	})}
	base := MapSiteInput{URL: "https://docs.firecrawl.dev"}
	first, err := MapSitePage(base)
	if err != nil {
		t.Fatal(err)
	}
	if first["count"] != 30 || first["total_discovered"] != 2803 || first["language"] != "en" {
		t.Fatalf("unbounded mapping: %+v", first)
	}
	next := mapField(first["next"])
	if next["tool"] != "map_site" {
		t.Fatalf("missing paged navigation: %+v", next)
	}
	args := next["arguments"].(MapSiteInput)
	if args.Offset != 30 {
		t.Fatalf("wrong offset: %d", args.Offset)
	}
	second, err := MapSitePage(args)
	if err != nil {
		t.Fatal(err)
	}
	page1 := first["links"].([]map[string]any)
	page2 := second["links"].([]map[string]any)
	if page1[0]["url"] == page2[0]["url"] {
		t.Fatal("next site-map page repeats the first")
	}
	for _, v := range page1 {
		if strings.Contains(v["url"].(string), "/ja/") {
			t.Fatal("localized page leaked into English default")
		}
	}
	jp, err := MapSitePage(MapSiteInput{URL: base.URL, Language: "ja", Limit: 10, PathPrefix: "/ja/features/"})
	if err != nil || jp["count"] != 10 {
		t.Fatalf("Japanese/path filtered page failed: %+v %v", jp, err)
	}
	_, err = MapSitePage(MapSiteInput{URL: base.URL, Limit: 101})
	if err == nil {
		t.Fatal("must refuse unbounded client page")
	}
	oldText, err := MapSite(base.URL)
	if err != nil || len(oldText) > 13500 {
		t.Fatalf("URL-only map output too large (%d bytes): %v", len(oldText), err)
	}
	if calls != 4 {
		t.Logf("Site-map mock calls (cache disabled for test credential): %d", calls)
	}
}

func TestAuditTrueSchemaAlternativesAndExamples(t *testing.T) {
	contract := map[string]any{
		"id": "fiscal-ai/financials/income-statement",
		"options": []any{
			map[string]any{"name": "fscl", "type": "string", "example": "companyFiscalIdentifier"},
			map[string]any{"name": "companyKey", "type": "string"},
			map[string]any{"name": "ticker", "type": "string"},
		},
		"requiresOneOf": []any{[]any{"fscl", "companyKey", "ticker"}},
	}
	template, required := nativeInputTemplate(contract)
	if len(required) > 0 || template["fscl"] != nil {
		t.Fatalf("one-of group made fscl falsely required: %+v %v", template, required)
	}
	example := mapField(listField(contractExamples(contract))[0])
	if !example["ready_to_execute"].(bool) || mapField(example["inputs"])["companyKey"] != "NASDAQ_AAPL" {
		t.Fatalf("expected verified source-compatible alternative: %+v", example)
	}
	if err := validateDataInputs(contract, map[string]any{"companyKey": "NASDAQ_AAPL"}); err != nil {
		t.Fatalf("alternative company key should validate: %v", err)
	}
	emptyLookup := map[string]any{"id": "clinicaltrials-gov/studies/get", "options": []any{map[string]any{"name": "nct_id", "type": "string"}}}
	lookupExample := mapField(listField(contractExamples(emptyLookup))[0])
	if lookupExample["ready_to_execute"] == true || lookupExample["next"] != nil {
		t.Fatalf("no-identifier lookup must not advertise runnable example: %+v", lookupExample)
	}
	expensive := map[string]any{"id": "fullenrich/contacts/mobile", "creditsCost": 175, "options": []any{
		map[string]any{"name": "linkedin_url", "type": "string", "required": true, "example": "https://example.com"},
	}}
	mobile := mapField(listField(contractExamples(expensive))[0])
	if mobile["ready_to_execute"] == true || mobile["next"] != nil {
		t.Fatalf("a fake LinkedIn profile must not be executable: %+v", mobile)
	}
}

func TestAuditDirectRetrievalBeatsAncillarySearchOperations(t *testing.T) {
	cases := []struct {
		query, direct, ancillary string
	}{
		{"Historical US inflation from FRED", "fred-stlouisfed-org/economic-data/series_observations", "fred-stlouisfed-org/economic-data/releases"},
		{"Recruiting diabetes clinical trials", "clinicaltrials-gov/studies/search", "clinicaltrials-gov/studies/suggest"},
		{"Podcast episodes about AI agents", "particle/podcasts/episodes/search", "particle/podcasts/shows/directory"},
		{"Scientific papers about CRISPR", "semantic-scholar/papers/search", "semantic-scholar/papers/related"},
	}
	for _, tt := range cases {
		direct := map[string]any{"id": tt.direct, "similarity": 0.45, "name": "Search source records"}
		side := map[string]any{"id": tt.ancillary, "similarity": 0.55, "name": "Related source operations"}
		if intentRelevance(direct, tt.query) <= intentRelevance(side, tt.query) {
			t.Fatalf("ancillary operation beat direct retrieval for %q: %v <= %v", tt.query, intentRelevance(direct, tt.query), intentRelevance(side, tt.query))
		}
	}
}

func TestAuditTypedConstraintsAndNullableOutput(t *testing.T) {
	c := map[string]any{"id": "example/list", "options": []any{
		map[string]any{"name": "limit", "type": "number", "min": 1, "max": 100},
		map[string]any{"name": "query", "type": "string"},
	}, "response": map[string]any{"fields": []any{
		map[string]any{"name": "value", "type": "number", "about": "null when data is unavailable"},
	}}}
	fields := normalizedDataInputFields(c)
	if fields[0]["type"] != "integer" || normalizedDataOutputFields(c)[0]["nullable"] != true {
		t.Fatalf("lost typed metadata %+v %+v", fields, normalizedDataOutputFields(c))
	}
	if err := validateDataInputs(c, map[string]any{"limit": float64(1.5)}); err == nil {
		t.Fatal("fractional page size accepted even though it must be an integer")
	}
}

func TestAuditURLAndLicenseAnswers(t *testing.T) {
	repo := "https://github.com/firecrawl/firecrawl"
	body := "---\nrepository: firecrawl/firecrawl\nlicense: \"AGPL-3.0\"\n---\n\n" +
		"[License](https://github.com/firecrawl/firecrawl/blob/main/LICENSE)\n" + strings.Repeat("Unrelated markdown line\n", 300)
	answer, err := readLinkFocused(repo, "What license does this public repository use? Quote the specific source URL.", func(string) (string, error) { return body, nil })
	if err != nil || !strings.Contains(answer, "**AGPL-3.0**") ||
		!strings.Contains(answer, repo+"/blob/main/LICENSE") || len(answer) > 650 {
		t.Fatalf("focused GitHub license answer poor: %s / %v", answer, err)
	}
}

func TestAuditRateLimitReturnsMachineReadableReset(t *testing.T) {
	previous := http.DefaultClient
	t.Cleanup(func() {
		http.DefaultClient = previous
		freeDataCooldown.Lock()
		freeDataCooldown.until = time.Time{}
		freeDataCooldown.Unlock()
	})
	t.Setenv("FIRECRAWL_API_KEY", "test-ratelimit")
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return testHTTPResponse(req, 429, `{"success":false,"error":"Rate limit exceeded. Upgrade your plan."}`,
			map[string]string{"Retry-After": "31"}), nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	_, err := doDataAPI(ctx, "/search", map[string]any{"query": "test"}, "")
	e, ok := err.(*DataError)
	if !ok || e.Code != "rate_limited" || e.RetryAfterSeconds != 31 || e.RetryAt == "" || !e.Retryable {
		t.Fatalf("rate limit not actionable: %+v", err)
	}
	_, err = doDataAPI(ctx, "/search", map[string]any{"query": "test"}, "")
	if e, ok = err.(*DataError); !ok || e.Code != "rate_limited" || e.RetryAfterSeconds < 20 {
		t.Fatalf("worker ignored cooldown: %+v", err)
	}
}

func TestAuditUnknownOperationErrorIsPrecise(t *testing.T) {
	installDataTestTransport(t)
	_, err := InspectData(InspectDataInput{ID: "particle/never-existing/operation"})
	e, ok := err.(*DataError)
	if !ok || e.Code != "operation_not_found" {
		t.Fatalf("wrong unknown-op error: %+v", err)
	}
}

func TestAuditCatalogueContinuationAndFallbackExamples(t *testing.T) {
	installDataTestTransport(t)
	id := "particle/podcasts/episodes/search"
	page, err := ResearchData(ResearchDataInput{Mode: "catalogue", View: "operations",
		Operations: []string{id}, Include: []string{"inputs", "output", "examples"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	items := page["results"].([]map[string]any)
	if len(items) != 1 || mapField(items[0]["next"])["tool"] != "inspect" {
		t.Fatalf("exact operation should have inspect continuation: %+v", page)
	}
	if len(listField(items[0]["examples"])) == 0 {
		t.Fatalf("research include examples must generate usable fallback: %+v", items[0])
	}
	source, err := InspectData(InspectDataInput{ID: "particle"})
	if err != nil {
		t.Fatal(err)
	}
	first := source["results"].([]map[string]any)[0]
	if first["inputs"] != nil || first["output"] != nil {
		t.Fatalf("source overview must not expand every schema by default: %+v", first)
	}
}

func TestAuditWorkerIdempotentReplayAndCollision(t *testing.T) {
	mock := installDataTestTransport(t)
	id := "audit-replay-unique-20261010"
	calls := []DataCall{{ID: "particle/podcasts/episodes/search", Inputs: map[string]any{"semantic_search": "AI agents"}}}
	first, err := ExecuteData(ExecuteDataInput{Calls: calls, RequestID: id})
	if err != nil {
		t.Fatal(err)
	}
	again, err := ExecuteData(ExecuteDataInput{Calls: calls, RequestID: id})
	if err != nil {
		t.Fatal(err)
	}
	if again["replay_status"] != "served_from_local_cache" || again["replayed"] != true ||
		again["credits_charged_this_request"] != float64(0) && again["credits_charged_this_request"] != 0 {
		t.Fatalf("replay accounting ambiguous: %+v", again)
	}
	if again["original_credits_used"] != first["credits_used"] {
		t.Fatal("lost original provider credit amount")
	}
	mock.mu.Lock()
	n := mock.executions
	mock.mu.Unlock()
	if n != 1 {
		t.Fatalf("replay caused duplicate upstream execution (%d)", n)
	}
	changed := []DataCall{{ID: calls[0].ID, Inputs: map[string]any{"semantic_search": "Completely different"}}}
	_, err = ExecuteData(ExecuteDataInput{Calls: changed, RequestID: id})
	e, ok := err.(*DataError)
	if !ok || e.Code != "duplicate_request" || mapField(e.Details)["retry"] != nil {
		t.Fatalf("conflicting ID returned unsafe recovery: %+v", err)
	}
}

func TestAuditMixedTermsBatchKeepsIndependentResultsAndCredits(t *testing.T) {
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	t.Setenv("FIRECRAWL_API_KEY", "test-mixed-batch")
	t.Setenv("WEBCTX_ALEXANDRIA_MAX_CREDITS", "200")
	paid := map[string]int{}
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var payload map[string]any
		_ = json.NewDecoder(req.Body).Decode(&payload)
		if find := mapField(payload["alexandria"]); find != nil {
			providers := listField(mapField(find["options"])["providers"])
			provider := stringField(providers[0])
			item := map[string]any{"id": provider + "/data/search", "provider": provider,
				"capability": "data/search", "name": "Search records", "creditsCost": 5,
				"perRecord": false, "options": []any{map[string]any{"name": "query", "type": "string", "required": true}},
			}
			encoded, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"alexandria": []any{
				map[string]any{"provider": "firecrawl", "capability": "find-tools", "data": map[string]any{
					"items": []any{item}, "total": 1,
				}},
			}}})
			return testHTTPResponse(req, 200, string(encoded), nil), nil
		}
		calls := listField(payload["alexandria"])
		if len(calls) != 1 {
			return nil, errors.New("mixed providers must not share an upstream batch transaction")
		}
		provider := stringField(mapField(calls[0])["provider"])
		paid[provider]++
		if req.Header.Get("x-request-id") == "" {
			return nil, errors.New("missing idempotency request id")
		}
		if provider == "particle" {
			return testHTTPResponse(req, 403, `{"success":false,"code":"THIRD_PARTY_DATA_TERMS_REQUIRED","error":"You must accept terms to query Particle","requiresAction":{"url":"https://firecrawl.dev/app/settings?tab=data-sources"}}`, nil), nil
		}
		data := map[string]any{"success": true, "data": map[string]any{"creditsCost": 5, "alexandria": []any{
			map[string]any{"provider": provider, "capability": "data/search", "creditsCost": 5, "alexandriaId": "shared-upstream-id",
				"data": map[string]any{"results": []any{map[string]any{"id": provider + "-row"}}}},
		}}}
		encoded, _ := json.Marshal(data)
		return testHTTPResponse(req, 200, string(encoded), nil), nil
	})}
	calls := []DataCall{
		{ID: "particle/data/search", Inputs: map[string]any{"query": "podcasts"}},
		{ID: "fred/data/search", Inputs: map[string]any{"query": "inflation"}},
		{ID: "sec/data/search", Inputs: map[string]any{"query": "filings"}},
	}
	result, err := ExecuteData(ExecuteDataInput{Calls: calls})
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != "partial_success" || result["failed_operations"] != 1 ||
		result["credits_used"] != float64(10) {
		t.Fatalf("mixed failure hid authorized work or incorrectly counted credits: %+v", result)
	}
	entries := result["results"].([]map[string]any)
	if len(entries) != 3 || entries[0]["ok"] != false || entries[1]["ok"] != true || entries[2]["ok"] != true {
		t.Fatalf("provider isolation failed: %+v", entries)
	}
	failure := entries[0]["error"].(*DataError)
	if failure.Code != "THIRD_PARTY_DATA_TERMS_REQUIRED" || failure.RequiresAction["url"] == nil {
		t.Fatalf("provider terms error lost: %+v", failure)
	}
	for i := 1; i < 3; i++ {
		if entries[i]["operation_index"] != i ||
			entries[i]["receipt_scope"] != "provider_response_or_batch" ||
			entries[i]["provider_request_id"] == entries[1-i+1]["provider_request_id"] && i == 2 {
			t.Fatalf("provenance does not distinguish provider responses: %+v", entries)
		}
	}
	if paid["particle"] != 1 || paid["fred"] != 1 || paid["sec"] != 1 {
		t.Fatalf("provider calls were not independent: %+v", paid)
	}
}

func TestAuditHighPriceOperationCannotBeRecommendedAsRunnable(t *testing.T) {
	t.Setenv("WEBCTX_ALEXANDRIA_MAX_CREDITS", "200")
	original := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = original })
	t.Setenv("FIRECRAWL_API_KEY", "test-high-price")
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := map[string]any{"success": true, "data": map[string]any{"alexandria": []any{
			map[string]any{"provider": "firecrawl", "capability": "find-tools", "creditsCost": 0,
				"data": map[string]any{"items": []any{
					map[string]any{"id": "similarweb/channels/share", "provider": "similarweb", "capability": "channels/share",
						"name": "Channel traffic share", "creditsCost": 550, "options": []any{map[string]any{"name": "domain", "type": "string", "required": true}}},
				}},
			},
		}}}
		encoded, _ := json.Marshal(body)
		return testHTTPResponse(req, 200, string(encoded), nil), nil
	})}
	details, err := InspectData(InspectDataInput{ID: "similarweb/channels/share"})
	if err != nil {
		t.Fatal(err)
	}
	if details["executable"] != false || details["availability"] != "exceeds_server_credit_limit" ||
		details["next"] != nil || details["server_credit_limit"] != 200 {
		t.Fatalf("unaﬀordable operation falsely recommended: %+v", details)
	}
}

var _ = fmt.Sprint
var _ = url.URL{}
