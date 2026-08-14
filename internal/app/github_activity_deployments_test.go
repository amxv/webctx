package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

func TestParseActivityStatisticsDeploymentTargets(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		kind GitHubTargetKind
		name string
	}{
		{raw: "https://github.com/o/r/activity", kind: GitHubTargetActivity},
		{raw: "https://github.com/o/r/graphs/contributors", kind: GitHubTargetStatsContributors},
		{raw: "https://github.com/o/r/graphs/commit-activity", kind: GitHubTargetStatsCommitActivity},
		{raw: "https://github.com/o/r/graphs/code-frequency", kind: GitHubTargetStatsCodeFrequency},
		{raw: "https://github.com/o/r/deployments", kind: GitHubTargetDeployments},
		{raw: "https://github.com/o/r/deployments/production", kind: GitHubTargetDeploymentEnvironment, name: "production"},
		{raw: "https://github.com/o/r/deployments/preview%2Fwest", kind: GitHubTargetDeploymentEnvironment, name: "preview/west"},
	} {
		target := parseGitHubTarget(tt.raw)
		if target == nil || target.Kind != tt.kind || target.Name != tt.name {
			t.Fatalf("target %s => %#v", tt.raw, target)
		}
	}
	for _, raw := range []string{
		"https://github.com/o/r/graphs/traffic",
		"https://github.com/o/r/graphs/punch-card",
		"https://github.com/o/r/activity/extra",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("unproven metrics route claimed: %s => %#v", raw, target)
		}
	}
}

func TestRepositoryActivityIsBoundedAndPreservesProviderFilters(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/activity" {
			t.Fatalf("activity endpoint=%s", r.URL.Path)
		}
		for key, want := range map[string]string{"ref": "refs/heads/main", "activity_type": "push", "actor": "alice", "page": "2", "per_page": "8"} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("activity %s=%q want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/activity?ref=refs%%2Fheads%%2Fmain&activity_type=push&actor=alice&per_page=8&page=1>; rel="prev", <%s/repos/o/r/activity?ref=refs%%2Fheads%%2Fmain&activity_type=push&actor=alice&per_page=8&page=3>; rel="next"`, server.URL, server.URL))
		_, _ = io.WriteString(w, `[{"id":1,"before":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","after":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","ref":"refs/heads/main","timestamp":"2026-08-01T00:00:00Z","activity_type":"push","actor":{"login":"alice"}}]`)
	}))
	defer server.Close()
	out, err := readGitHubActivity(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/activity?ref=refs%2Fheads%2Fmain&activity_type=push&actor=alice&page=2"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"view: \"activity\"", "push by @alice", "`refs/heads/main`", "`aaaaaaaaaaaa` → `bbbbbbbbbbbb`", "Previous", "page=1", "Next", "page=3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("activity missing %q:\n%s", want, out)
		}
	}
}

func TestStatisticsHTTP202IsComputingNotEmptySuccess(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/o/r/graphs/contributors",
		"https://github.com/o/r/graphs/commit-activity",
		"https://github.com/o/r/graphs/code-frequency",
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{}`)
		}))
		target := parseGitHubTarget(raw)
		result := readGitHubNativeWithClient(context.Background(), target, testGitHubClient(server.URL, server.URL, ""))
		server.Close()
		if result.Outcome != GitHubNativeSuccess || !strings.Contains(result.Markdown, "provider_status: computing") || !strings.Contains(result.Markdown, "HTTP 202") || !strings.Contains(result.Markdown, "webctx has no local cache") {
			t.Fatalf("statistics 202 was not truthful for %s: %#v", raw, result)
		}
	}
}

func TestContributorStatisticsRenderProviderCachedTruth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"total":5,"author":{"login":"alice"},"weeks":[{"w":1700000000,"a":12,"d":3,"c":2},{"w":1700600000,"a":4,"d":1,"c":3}]}]`)
	}))
	defer server.Close()
	out, err := readGitHubContributorStats(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/graphs/contributors"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "@alice — 5 commits · +16 -4") || !strings.Contains(out, "GitHub computes/caches repository statistics upstream") {
		t.Fatalf("contributor stats truth missing:\n%s", out)
	}
}

func TestContributorStatisticsPrioritizeTotalsAndStayBounded(t *testing.T) {
	stats := make([]githubContributorStats, 300)
	for i := range stats {
		stats[i].Total = i
		stats[i].Author.Login = fmt.Sprintf("contributor-%03d", i)
	}
	stats[0].Total = 10000
	stats[0].Author.Login = "zeta"
	stats[1].Total = 10000
	stats[1].Author.Login = "alpha"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	}))
	defer server.Close()

	out, err := readGitHubContributorStats(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/graphs/contributors"))
	if err != nil {
		t.Fatal(err)
	}
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("contributor overview exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{"contributors_returned: 300", "contributors_local_omitted:", "locally omitted from this overview", "@alpha — 10000 commits", "@zeta — 10000 commits"} {
		if !strings.Contains(out, want) {
			t.Fatalf("contributor overview missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "contributors_local_omitted: 0") || strings.Index(out, "@alpha — 10000 commits") > strings.Index(out, "@zeta — 10000 commits") {
		t.Fatalf("contributor ordering/omission wrong:\n%s", out)
	}
}

func TestCommitActivityAndCodeFrequencyRenderWeeks(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		body string
		want string
	}{
		{raw: "https://github.com/o/r/graphs/commit-activity", body: `[{"week":1700000000,"total":7,"days":[1,1,1,1,1,1,1]}]`, want: "7 commits"},
		{raw: "https://github.com/o/r/graphs/code-frequency", body: `[[1700000000,120,-30]]`, want: "+120 -30"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, tt.body)
		}))
		result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget(tt.raw), testGitHubClient(server.URL, server.URL, ""))
		server.Close()
		if result.Outcome != GitHubNativeSuccess || !strings.Contains(result.Markdown, tt.want) || !strings.Contains(result.Markdown, "cached computation") {
			t.Fatalf("statistics render wrong for %s: %#v", tt.raw, result)
		}
	}
}

func TestCodeFrequencyLongHistoryUsesRecentWeeklyIndexAndFullAggregates(t *testing.T) {
	weeks := make([][]int64, 600)
	for i := range weeks {
		weeks[i] = []int64{1700000000 + int64(i*604800), int64(i), -int64(i)}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(weeks)
	}))
	defer server.Close()

	out, err := readGitHubCodeFrequencyStats(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/graphs/code-frequency"))
	if err != nil {
		t.Fatal(err)
	}
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("code-frequency overview exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{
		"weeks_returned: 600", "weeks_indexed: 52", "weeks_local_omitted: 548",
		"additions_total: 179700", "deletions_total: -179700", "most recent 52 weekly buckets", "+599 -599",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("long code-frequency overview missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "+0 0") || strings.Contains(out, "+547 -547") {
		t.Fatalf("old code-frequency buckets leaked into recent index:\n%s", out)
	}
}

func TestDeploymentListIsBoundedAndDoesNotFetchStatuses(t *testing.T) {
	var statusCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/statuses") {
			atomic.AddInt32(&statusCalls, 1)
			t.Fatal("deployment list must not fan out status history")
		}
		if r.URL.Path != "/repos/o/r/deployments" {
			t.Fatalf("deployment list endpoint=%s", r.URL.Path)
		}
		for key, want := range map[string]string{"environment": "production", "page": "2", "per_page": "8"} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("deployments %s=%q want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"id":101,"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","ref":"main","task":"deploy","environment":"production","creator":{"login":"alice"},"created_at":"2026-08-01T00:00:00Z"}]`)
	}))
	defer server.Close()
	out, err := readGitHubDeployments(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/deployments?environment=production&page=2"))
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&statusCalls) != 0 || !strings.Contains(out, "Deployment 101") || !strings.Contains(out, "[production](https://github.com/o/r/deployments/production)") {
		t.Fatalf("deployment list scoping wrong:\n%s", out)
	}
}

func TestDeploymentEnvironmentReadsOnlyLatestStatusPage(t *testing.T) {
	var statusPages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/environments/production":
			_, _ = io.WriteString(w, `{"id":9,"name":"production","html_url":"https://github.com/o/r/deployments/production","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-08-01T00:00:00Z"}`)
		case "/repos/o/r/deployments":
			if r.URL.Query().Get("environment") != "production" || r.URL.Query().Get("per_page") != "8" {
				t.Fatalf("environment deployment query=%s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `[{"id":101,"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","ref":"main","environment":"production","creator":{"login":"alice"},"created_at":"2026-08-01T00:00:00Z"}]`)
		case "/repos/o/r/deployments/101/statuses":
			atomic.AddInt32(&statusPages, 1)
			if r.URL.Query().Get("page") != "" || r.URL.Query().Get("per_page") != "1" {
				t.Fatalf("deployment status overview followed/changed pagination: %s", r.URL.RawQuery)
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/deployments/101/statuses?per_page=1&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"id":2,"state":"success","description":"ready","creator":{"login":"bot"},"created_at":"2026-08-01T00:02:00Z","log_url":"https://logs/2","environment_url":"https://env/2"}]`)
		default:
			t.Fatalf("unexpected deployment environment request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubDeploymentEnvironment(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/deployments/production"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"environment: \"production\"", "Deployment 101", "latest status 2 — success — ready", "logs https://logs/2", "Environment: https://env/2", "Older statuses: available upstream", "reads only the latest returned status"} {
		if !strings.Contains(out, want) {
			t.Fatalf("deployment environment missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "deploying") || atomic.LoadInt32(&statusPages) != 1 {
		t.Fatalf("status pagination pages=%d", statusPages)
	}
}

func TestDeploymentEnvironmentDeepStatusHistoryStaysBounded(t *testing.T) {
	deployments := make([]githubDeployment, 10)
	for i := range deployments {
		deployments[i] = githubDeployment{ID: int64(100 + i), SHA: strings.Repeat("a", 40), Ref: fmt.Sprintf("deploy-%d", i), Environment: "production", CreatedAt: "2026-08-01T00:00:00Z"}
		deployments[i].Creator.Login = "bot"
	}
	var statusCalls int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/o/r/environments/production":
			_, _ = io.WriteString(w, `{"id":9,"name":"production"}`)
		case r.URL.Path == "/repos/o/r/deployments":
			_ = json.NewEncoder(w).Encode(deployments)
		case strings.HasPrefix(r.URL.Path, "/repos/o/r/deployments/") && strings.HasSuffix(r.URL.Path, "/statuses"):
			atomic.AddInt32(&statusCalls, 1)
			if r.URL.Query().Get("page") != "" || r.URL.Query().Get("per_page") != "1" {
				t.Fatalf("deep status history was paginated: %s", r.URL.RawQuery)
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?per_page=1&page=2>; rel="next", <%s%s?per_page=1&page=250>; rel="last"`, server.URL, r.URL.Path, server.URL, r.URL.Path))
			_, _ = io.WriteString(w, fmt.Sprintf(`[{"id":999,"state":"success","description":%q,"creator":{"login":"bot"},"created_at":"2026-08-01T00:02:00Z","log_url":"https://logs.example/latest","environment_url":"https://env.example/latest"}]`, strings.Repeat("latest status detail ", 100)))
		default:
			t.Fatalf("unexpected request %s", r.URL.String())
		}
	}))
	defer server.Close()

	out, err := readGitHubDeploymentEnvironment(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/deployments/production"))
	if err != nil {
		t.Fatal(err)
	}
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("deployment environment exceeded shared target: %d runes\n%s", got, out)
	}
	if got := atomic.LoadInt32(&statusCalls); got != 10 {
		t.Fatalf("deployment statuses made %d calls, want one bounded call per deployment", got)
	}
	for _, want := range []string{"deployments_returned: 10", "deployments_with_older_statuses: 10", "latest status 999 — success", "Older statuses: available upstream", "https://logs.example/latest", "https://env.example/latest"} {
		if !strings.Contains(out, want) {
			t.Fatalf("bounded deployment output missing %q:\n%s", want, out)
		}
	}
}

func TestActivityStatisticsDeploymentFailuresStayNative(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"private body"}`)
	}))
	defer server.Close()
	for _, raw := range []string{
		"https://github.com/o/r/activity",
		"https://github.com/o/r/graphs/contributors",
		"https://github.com/o/r/deployments",
		"https://github.com/o/r/deployments/production",
	} {
		result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget(raw), testGitHubClient(server.URL, server.URL, ""))
		if result.Outcome != GitHubNativeFailure || result.Err == nil || strings.Contains(result.Err.Error(), "private body") {
			t.Fatalf("native failure regressed for %s: %#v", raw, result)
		}
	}
}
