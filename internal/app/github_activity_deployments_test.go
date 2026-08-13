package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
		for key, want := range map[string]string{"ref": "refs/heads/main", "activity_type": "push", "actor": "alice", "page": "2", "per_page": "30"} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("activity %s=%q want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/activity?ref=refs%%2Fheads%%2Fmain&activity_type=push&actor=alice&per_page=30&page=1>; rel="prev", <%s/repos/o/r/activity?ref=refs%%2Fheads%%2Fmain&activity_type=push&actor=alice&per_page=30&page=3>; rel="next"`, server.URL, server.URL))
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
		for key, want := range map[string]string{"environment": "production", "page": "2", "per_page": "30"} {
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

func TestDeploymentEnvironmentFetchesPaginatedStatusHistory(t *testing.T) {
	var statusPages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/environments/production":
			_, _ = io.WriteString(w, `{"id":9,"name":"production","html_url":"https://github.com/o/r/deployments/production","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-08-01T00:00:00Z"}`)
		case "/repos/o/r/deployments":
			if r.URL.Query().Get("environment") != "production" || r.URL.Query().Get("per_page") != "10" {
				t.Fatalf("environment deployment query=%s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `[{"id":101,"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","ref":"main","environment":"production","creator":{"login":"alice"},"created_at":"2026-08-01T00:00:00Z"}]`)
		case "/repos/o/r/deployments/101/statuses":
			atomic.AddInt32(&statusPages, 1)
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `[{"id":2,"state":"success","description":"ready","creator":{"login":"bot"},"created_at":"2026-08-01T00:02:00Z","log_url":"https://logs/2"}]`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/deployments/101/statuses?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"id":1,"state":"in_progress","description":"deploying","creator":{"login":"bot"},"created_at":"2026-08-01T00:01:00Z"}]`)
		default:
			t.Fatalf("unexpected deployment environment request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubDeploymentEnvironment(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/deployments/production"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"environment: \"production\"", "Deployment 101", "in_progress — deploying", "success — ready", "logs https://logs/2", "GitHub controls deployment-status retention", "does not imply older statuses remain available indefinitely"} {
		if !strings.Contains(out, want) {
			t.Fatalf("deployment environment missing %q:\n%s", want, out)
		}
	}
	if atomic.LoadInt32(&statusPages) != 2 {
		t.Fatalf("status pagination pages=%d", statusPages)
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
