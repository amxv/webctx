package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func dataTestContract() map[string]any {
	return map[string]any{
		"id":            "particle/podcasts/episodes/search",
		"provider":      "particle",
		"capability":    "podcasts/episodes/search",
		"name":          "Search podcast conversations",
		"description":   "Find episodes by what people discussed.",
		"creditsCost":   15,
		"perRecord":     false,
		"requiresOneOf": []any{[]any{"semantic_search", "keyword_search"}},
		"options": []any{
			map[string]any{"name": "semantic_search", "type": "string", "about": "Topic described in your own words."},
			map[string]any{"name": "keyword_search", "type": "string", "about": "Exact transcript keyword."},
			map[string]any{"name": "limit", "type": "number", "default": 25, "min": 1, "max": 100},
			map[string]any{"name": "cursor", "type": "string", "about": "Opaque pagination token."},
		},
		"response": map[string]any{"key": "data", "paginated": true, "fields": []any{map[string]any{"name": "episode", "type": "object"}}},
	}
}

type dataTestTransport struct {
	mu          sync.Mutex
	executions  int
	lastHeaders http.Header
	lastPayload map[string]any
	blockPaid   bool
}

func (d *dataTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "api.firecrawl.dev" || req.Method != http.MethodPost {
		return testHTTPResponse(req, http.StatusNotFound, "{}", nil), nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.lastPayload = payload
	d.lastHeaders = req.Header.Clone()
	d.mu.Unlock()
	write := func(status int, content map[string]any) (*http.Response, error) {
		b, _ := json.Marshal(content)
		return testHTTPResponse(req, status, string(b), nil), nil
	}
	if req.URL.Path == "/v2/search" {
		if strings.Contains(stringField(payload["query"]), "relevance ordering") {
			low := dataTestContract()
			low["id"] = "particle/podcasts/episodes/list"
			low["similarity"] = 0.1
			high := dataTestContract()
			high["id"] = "particle/podcasts/episodes/search"
			high["similarity"] = 0.97
			middle := dataTestContract()
			middle["id"] = "particle/podcasts/episode"
			middle["similarity"] = 0.8
			return write(200, map[string]any{"success": true, "data": map[string]any{"tools": []any{low, high, middle}}})
		}
		return write(200, map[string]any{"success": true, "data": map[string]any{
			"tools": []any{dataTestContract()},
		}})
	}
	if req.URL.Path != "/v2/scrape" {
		return write(404, map[string]any{"success": false})
	}
	if operation := mapField(payload["alexandria"]); operation != nil &&
		operation["provider"] == "firecrawl" && operation["capability"] == "find-tools" {
		opts := mapField(operation["options"])
		if opts["level"] == "providers" {
			return write(200, map[string]any{"success": true, "data": map[string]any{
				"alexandria": []any{map[string]any{"provider": "firecrawl", "capability": "find-tools", "creditsCost": 0, "data": map[string]any{
					"level": "providers", "total": 3,
					"items": []any{map[string]any{"id": "particle", "name": "Particle", "description": "Podcast data", "toolCount": 40}},
					"next": map[string]any{"provider": "firecrawl", "capability": "find-tools", "options": map[string]any{
						"level": "providers", "limit": 2, "offset": 2,
					}},
				}}},
			}})
		}
		return write(200, map[string]any{"success": true, "data": map[string]any{
			"alexandria": []any{map[string]any{"provider": "firecrawl", "capability": "find-tools", "creditsCost": 0, "data": map[string]any{
				"level": "tools", "total": 1, "items": []any{dataTestContract()},
			}}},
		}})
	}
	if calls := listField(payload["alexandria"]); len(calls) > 0 {
		d.mu.Lock()
		d.executions++
		d.mu.Unlock()
		if d.blockPaid {
			return write(403, map[string]any{
				"success": false, "code": "THIRD_PARTY_DATA_TERMS_REQUIRED",
				"error":          "Provider terms have not been accepted.",
				"requiresAction": map[string]any{"url": "https://firecrawl.dev/app/settings?tab=data-sources"},
			})
		}
		var results []any
		var total int
		for _, value := range calls {
			call := mapField(value)
			results = append(results, map[string]any{
				"provider": call["provider"], "capability": call["capability"],
				"creditsCost": 15, "alexandriaId": "paid-data-id-1",
				"data": map[string]any{"data": []any{map[string]any{"episode": map[string]any{"id": "ep_1"}}}, "next_cursor": "after-1"},
			})
			total += 15
		}
		return write(200, map[string]any{
			"success": true, "data": map[string]any{"alexandria": results, "creditsCost": total},
		})
	}
	return write(400, map[string]any{"success": false, "error": "Bad test request."})
}

func TestResearchSortsMatchesByActualSimilarity(t *testing.T) {
	installDataTestTransport(t)
	results, err := ResearchData(ResearchDataInput{Query: "relevance ordering", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	found := results["results"].([]map[string]any)
	if found[0]["id"] != "particle/podcasts/episodes/search" ||
		found[1]["id"] != "particle/podcasts/episode" ||
		found[2]["id"] != "particle/podcasts/episodes/list" {
		t.Fatalf("results were not ordered by reported relevance: %#v", found)
	}
}

func installDataTestTransport(t *testing.T) *dataTestTransport {
	t.Helper()
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	d := &dataTestTransport{}
	http.DefaultClient = &http.Client{Transport: d}
	t.Setenv("FIRECRAWL_API_KEY", "test-key")
	t.Setenv("WEBCTX_ALEXANDRIA_MAX_CREDITS", "100")
	return d
}

func TestProgressiveDataDiscoveryAndExecution(t *testing.T) {
	mock := installDataTestTransport(t)
	found, err := ResearchData(ResearchDataInput{Query: "podcast conversations about AI agents", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if found["cost_credits"] != 0 || found["mode"] != "ranked_search" {
		t.Fatalf("ranked discovery not free: %#v", found)
	}
	results := found["results"].([]map[string]any)
	id := results[0]["id"].(string)
	if id != "particle/podcasts/episodes/search" {
		t.Fatalf("wrong discovery ID: %s", id)
	}
	next := mapField(results[0]["next"])
	if next["tool"] != "inspect" || mapField(next["arguments"])["id"] != id {
		t.Fatalf("discovery did not provide inspect arguments: %#v", next)
	}
	details, err := InspectData(InspectDataInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if mapField(details["price"])["credits_per_call"] != float64(15) {
		t.Fatalf("missing pricing: %#v", details)
	}
	if len(listField(mapField(details["inputs"])["fields"])) != 4 {
		t.Fatalf("missing upstream contract fields: %#v", details["inputs"])
	}
	generated := listField(details["examples"])
	if len(generated) == 0 || mapField(generated[0])["valid_input_shape"] != true {
		t.Fatalf("inspection must generate an explicitly labeled, schema-valid example: %#v", details["examples"])
	}
	continued := mapField(details["next"])
	if continued["tool"] != "execute" {
		t.Fatalf("no next execution: %#v", continued)
	}
	args := mapField(continued["arguments"])
	skeleton := listField(args["calls"])
	if len(skeleton) != 1 {
		t.Fatalf("invalid next argument shape: %#v", args)
	}
	inspectedCall := mapField(skeleton[0])
	if inspectedCall == nil || inspectedCall["id"] != id || mapField(inspectedCall["inputs"])["semantic_search"] != nil {
		t.Fatalf("invalid template: %#v", skeleton)
	}
	got, err := ExecuteData(ExecuteDataInput{
		Calls: []DataCall{{ID: id, Inputs: map[string]any{"semantic_search": "AI agents", "limit": float64(2)}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["credits_used"] != float64(15) {
		t.Fatalf("credits missing: %#v", got)
	}
	executions := got["results"].([]map[string]any)
	if executions[0]["data_id"] != "paid-data-id-1" {
		t.Fatalf("provider provenance missing: %#v", got)
	}
	followup := mapField(executions[0]["next"])
	if followup["tool"] != "execute" {
		t.Fatalf("missing cursor continuation: %#v", followup)
	}
	followArgs := mapField(followup["arguments"])
	nextCall := mapField(listField(followArgs["calls"])[0])
	if mapField(nextCall["inputs"])["cursor"] != "after-1" {
		t.Fatalf("pagination lost filters: %#v", nextCall)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.executions != 1 || !safeRequestID.MatchString(got["request_id"].(string)) ||
		mock.lastHeaders.Get("x-request-id") != got["request_id"].(string) {
		t.Fatalf("request not paid once with stable ID: executions=%d req=%s", mock.executions, got["request_id"])
	}
}

func TestCatalogueBrowsingReturnsExactPaginationCall(t *testing.T) {
	installDataTestTransport(t)
	result, err := ResearchData(ResearchDataInput{View: "sources", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result["view"] != "sources" || result["total"] != float64(3) {
		t.Fatalf("bad catalogue: %#v", result)
	}
	next := mapField(result["next"])
	args := mapField(next["arguments"])
	if next["tool"] != "research" || args["offset"] != float64(2) || args["view"] != "sources" || args["mode"] != "catalogue" {
		t.Fatalf("invalid pagination continuation: %#v", next)
	}
	fromSource, err := InspectData(InspectDataInput{ID: "particle"})
	if err != nil {
		t.Fatal(err)
	}
	if fromSource["view"] != "operations" {
		t.Fatalf("provider exploration failed: %#v", fromSource)
	}
}

func TestExecutionValidationAndServerLimitRejectBeforePaidCall(t *testing.T) {
	mock := installDataTestTransport(t)
	id := "particle/podcasts/episodes/search"
	for _, testCase := range []struct {
		inputs map[string]any
		code   string
	}{
		{map[string]any{}, "invalid_inputs"},
		{map[string]any{"semantic_search": "AI agents", "limit": 101}, "invalid_inputs"},
		{map[string]any{"semantic_search": "AI agents", "unknown_field": 123}, "invalid_inputs"},
	} {
		_, err := ExecuteData(ExecuteDataInput{Calls: []DataCall{{ID: id, Inputs: testCase.inputs}}})
		typed, ok := err.(*DataError)
		if !ok || typed.Code != testCase.code {
			t.Fatalf("wanted %s, got %v", testCase.code, err)
		}
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.executions != 0 {
		t.Fatalf("invalid calls charged credits: %d", mock.executions)
	}
}

func TestExecuteHandlesProviderTermsWithoutAccepting(t *testing.T) {
	mock := installDataTestTransport(t)
	mock.blockPaid = true
	_, err := ExecuteData(ExecuteDataInput{Calls: []DataCall{{
		ID:     "particle/podcasts/episodes/search",
		Inputs: map[string]any{"semantic_search": "AI agents"},
	}}})
	typed, ok := err.(*DataError)
	if !ok || typed.Code != "THIRD_PARTY_DATA_TERMS_REQUIRED" ||
		mapField(typed.RequiresAction)["url"] != "https://firecrawl.dev/app/settings?tab=data-sources" {
		t.Fatalf("terms condition lost: %+v", err)
	}
	if mapField(typed.Details)["request_id"] == nil {
		t.Fatalf("lost retriable request identifier: %+v", typed)
	}
}

func TestCLIResearchAndInspectJSON(t *testing.T) {
	installDataTestTransport(t)
	var out, errBuf bytes.Buffer
	if code := Run([]string{"research", "podcasts about AI agents", "--limit", "2", "--raw"}, &out, &errBuf); code != 0 {
		t.Fatalf("CLI research failed: %s", errBuf.String())
	}
	var result map[string]any
	if json.Unmarshal(out.Bytes(), &result) != nil || mapField(result["raw"]) == nil {
		t.Fatalf("CLI JSON or --raw missing: %s", out.String())
	}
	out.Reset()
	if code := Run([]string{"inspect", "particle/podcasts/episodes/search"}, &out, &errBuf); code != 0 {
		t.Fatalf("CLI inspect failed: %s", errBuf.String())
	}
	if !strings.Contains(out.String(), "semantic_search") || strings.Contains(out.String(), "max_credits") {
		t.Fatalf("CLI inspect omitted exact execute syntax: %s", out.String())
	}
}

func TestProviderNextOffsetCreatesCopyReadyContinuation(t *testing.T) {
	id := "fred-stlouisfed-org/economic-data/series_observations"
	inputs := map[string]any{"series_id": "CPIAUCSL", "limit": 2.0, "offset": 0.0}
	next := continuationForDataResult(id, inputs, map[string]any{"next_offset": 2.0, "count": 956.0})
	if mapField(next)["tool"] != "execute" {
		t.Fatalf("expected executable pagination: %+v", next)
	}
	args := mapField(mapField(next)["arguments"])
	call := mapField(listField(args["calls"])[0])
	fields := mapField(call["inputs"])
	if fields["offset"] != 2 || fields["series_id"] != "CPIAUCSL" {
		t.Fatalf("next offset dropped exact source inputs: %+v", fields)
	}
	if original, _ := numberField(inputs["offset"]); original != 0 {
		t.Fatal("original inputs mutated")
	}
}

func TestPerRecordPricingRequiresKnownBound(t *testing.T) {
	contract := map[string]any{"creditsCost": float64(5), "perRecord": true, "options": []any{}}
	if n, err := expectedCallCredits(contract, map[string]any{"limit": float64(3)}); err != nil || n != 15 {
		t.Fatalf("bad predicted per-record charge: %d %v", n, err)
	}
	if _, err := expectedCallCredits(contract, map[string]any{}); err == nil {
		t.Fatal("unknown record count must not trigger unbounded paid call")
	}
}

func TestNumericEnumsMayArriveAsUpstreamStrings(t *testing.T) {
	contract := map[string]any{"id": "example/enum", "options": []any{
		map[string]any{"name": "year", "type": "number", "required": true, "oneOf": []any{"2021", "2023"}},
	}}
	if err := validateDataInputs(contract, map[string]any{"year": float64(2023)}); err != nil {
		t.Fatalf("upstream numeric enum from strings should work: %v", err)
	}
	if err := validateDataInputs(contract, map[string]any{"year": float64(2026)}); err == nil {
		t.Fatal("unlisted numeric value must be rejected")
	}
}

func TestInspectOnlyOutputStillHasExecuteInputTemplate(t *testing.T) {
	installDataTestTransport(t)
	details, err := InspectData(InspectDataInput{ID: "particle/podcasts/episodes/search", Include: []string{"output"}})
	if err != nil {
		t.Fatal(err)
	}
	if mapField(details["input_template"])["semantic_search"] != nil || len(listField(mapField(details["inputs"])["fields"])) != 4 {
		t.Fatalf("missing necessary next-call schema after include=output: %+v", details)
	}
}

func TestServerManagedCreditCeilingCannotBeOverridden(t *testing.T) {
	mock := installDataTestTransport(t)
	// The ceiling defaults to 200 and an invalidly high environment value
	// cannot silently widen it beyond 200.
	t.Setenv("WEBCTX_ALEXANDRIA_MAX_CREDITS", "200")
	if got := dataCreditCap(); got != 200 {
		t.Fatalf("bad production cap: %d", got)
	}
	t.Setenv("WEBCTX_ALEXANDRIA_MAX_CREDITS", "2000")
	if got := dataCreditCap(); got != 200 {
		t.Fatalf("server cap exceeded 200: %d", got)
	}
	t.Setenv("WEBCTX_ALEXANDRIA_MAX_CREDITS", "120")
	if got := dataCreditCap(); got != 120 {
		t.Fatalf("server environment not respected: %d", got)
	}
	id := "particle/podcasts/episodes/search"
	validInputs := map[string]any{"semantic_search": "AI agents"}
	// 9*15=135: reject before ANY provider is queried.
	batch := make([]DataCall, 9)
	for i := range batch {
		batch[i] = DataCall{ID: id, Inputs: validInputs}
	}
	_, err := ExecuteData(ExecuteDataInput{Calls: batch})
	typed, ok := err.(*DataError)
	if !ok || typed.Code != "server_credit_limit_exceeded" ||
		mapField(typed.Details)["estimated_credits"] != 135 {
		t.Fatalf("expected server-only limit, got %#v", err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.executions != 0 {
		t.Fatalf("server limit failed to prevent execution, calls=%d", mock.executions)
	}
}

func TestClientCreditControlIsNotAvailableInCLI(t *testing.T) {
	installDataTestTransport(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"execute", "particle/podcasts/episodes/search", "--inputs", `{"semantic_search":"AI agents"}`, "--max-credits", "1"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), `"code": "unsupported_flag"`) {
		t.Fatalf("old client credit option should fail clearly, status=%d error=%s", code, stderr.String())
	}
}

func TestCPIInterpretationDoesNotConfuseIndexWithInflation(t *testing.T) {
	levels := dataMeasurementNote(map[string]any{"series_id": "CPIAUCSL", "units": "lin"})
	if !strings.Contains(levels, "index levels") || !strings.Contains(levels, "not annual inflation percentages") {
		t.Fatalf("CPI level explanation missing: %s", levels)
	}
	percentage := dataMeasurementNote(map[string]any{"series_id": "CPIAUCSL", "units": "pc1"})
	if !strings.Contains(percentage, "percent change from one year ago") {
		t.Fatalf("transformation note missing: %s", percentage)
	}
	if dataMeasurementNote(map[string]any{"series_id": "UNRATE", "units": "lin"}) != "" {
		t.Fatal("unemployment must not get CPI-specific semantics")
	}
}

func TestGeneratedFREDExampleUsesDocumentedSeries(t *testing.T) {
	c := map[string]any{
		"id": "fred-stlouisfed-org/economic-data/series_observations",
		"options": []any{
			map[string]any{"name": "series_id", "type": "string", "required": true, "about": "FRED series id, e.g. GDP, UNRATE and CPIAUCSL"},
			map[string]any{"name": "limit", "type": "number", "default": 1000, "min": 1, "max": 10000},
		},
	}
	examples := listField(contractExamples(c))
	if len(examples) != 1 {
		t.Fatalf("no fallback example: %#v", examples)
	}
	example := mapField(examples[0])
	inputs := mapField(example["inputs"])
	if example["kind"] != "generated_from_inspected_schema" || example["valid_input_shape"] != true ||
		inputs["series_id"] != "CPIAUCSL" || inputs["limit"] != 5 {
		t.Fatalf("unexpected generated FRED sample: %#v", examples)
	}
	if mapField(example["next"])["tool"] != "execute" {
		t.Fatalf("generated example cannot be copied into execute: %#v", example)
	}
}
