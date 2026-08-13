package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseGitHubSearchAndProfileTargets(t *testing.T) {
	for _, tt := range []struct {
		raw   string
		kind  GitHubTargetKind
		owner string
		repo  string
		name  string
	}{
		{raw: "https://github.com/search?q=webctx&type=repositories", kind: GitHubTargetSearch},
		{raw: "https://github.com/o/r/search?q=reader&type=code", kind: GitHubTargetSearch, owner: "o", repo: "r"},
		{raw: "https://github.com/octocat", kind: GitHubTargetProfile, owner: "octocat"},
		{raw: "https://github.com/openai?tab=repositories", kind: GitHubTargetProfile, owner: "openai"},
		{raw: "https://github.com/orgs/openai/people", kind: GitHubTargetProfile, owner: "openai", name: "people"},
	} {
		target := parseGitHubTarget(tt.raw)
		if target == nil || target.Kind != tt.kind || target.Owner != tt.owner || target.Repo != tt.repo || target.Name != tt.name {
			t.Fatalf("target %s => %#v", tt.raw, target)
		}
	}
	for _, raw := range []string{
		"https://github.com/search",
		"https://github.com/search?q=x&type=unknown",
		"https://github.com/search?q=x&type=repositories&unknown=y",
		"https://github.com/settings",
		"https://github.com/login",
		"https://github.com/marketplace",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("unsupported/global route was claimed: %s => %#v", raw, target)
		}
	}
}

func TestGitHubSearchRepositoryScopeSortOrderAndNavigation(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" {
			t.Fatalf("unexpected search path %s", r.URL.Path)
		}
		for key, want := range map[string]string{"q": "reader repo:o/r", "sort": "stars", "order": "desc", "page": "2", "per_page": "30"} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("search query %s=%q want %q (%s)", key, got, want, r.URL.RawQuery)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", fmt.Sprintf(`<%s/search/repositories?q=reader+repo%%3Ao%%2Fr&sort=stars&order=desc&per_page=30&page=1>; rel="prev", <%s/search/repositories?q=reader+repo%%3Ao%%2Fr&sort=stars&order=desc&per_page=30&page=3>; rel="next"`, server.URL, server.URL))
		_, _ = io.WriteString(w, `{"total_count":1200,"incomplete_results":true,"items":[{"full_name":"o/r","html_url":"https://github.com/o/r","description":"repo","language":"Go","stargazers_count":12,"forks_count":3,"archived":false}]}`)
	}))
	defer server.Close()
	target := parseGitHubTarget("https://github.com/o/r/search?q=reader&type=repositories&s=stars&o=desc&p=2")
	out, err := readGitHubSearch(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`repository_scope: "o/r"`, `total_count: 1200`, `incomplete_results: true`, `provider_result_ceiling: 1000`,
		"[o/r](https://github.com/o/r)", "12 stars", "GitHub marked these search results incomplete", "at most 1000 results",
		"Previous: https://github.com/o/r/search?", "p=1", "Next: https://github.com/o/r/search?", "p=3",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("search output missing %q:\n%s", want, out)
		}
	}
}

func TestGitHubSearchIssueAndPRQualifiers(t *testing.T) {
	for _, tt := range []struct {
		typeValue string
		want      string
		notWant   string
	}{
		{typeValue: "issues", want: "is:issue", notWant: "is:pr"},
		{typeValue: "pullrequests", want: "is:pr", notWant: "is:issue"},
	} {
		t.Run(tt.typeValue, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/search/issues" {
					t.Fatalf("issue/PR search endpoint=%s", r.URL.Path)
				}
				q := r.URL.Query().Get("q")
				if !strings.Contains(q, tt.want) || strings.Contains(q, tt.notWant) {
					t.Fatalf("provider query=%q want %q without %q", q, tt.want, tt.notWant)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"total_count":1,"incomplete_results":false,"items":[{"number":7,"title":"Result","state":"open","html_url":"https://github.com/o/r/issues/7","user":{"login":"alice"},"updated_at":"2026-08-01T00:00:00Z"}]}`)
			}))
			defer server.Close()
			out, err := readGitHubSearch(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/search?q=bug&type="+tt.typeValue))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "[#7 Result]") || !strings.Contains(out, "@alice") {
				t.Fatalf("search result missing:\n%s", out)
			}
		})
	}
}

func TestGitHubSearchCompactResultTypes(t *testing.T) {
	tests := []struct {
		typeValue string
		item      string
		want      []string
	}{
		{typeValue: "code", item: `{"name":"x.go","path":"internal/x.go","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","html_url":"https://github.com/o/r/blob/a/internal/x.go","repository":{"full_name":"o/r"}}`, want: []string{"internal/x.go", "o/r", "`aaaaaaaaaaaa`"}},
		{typeValue: "commits", item: `{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","html_url":"https://github.com/o/r/commit/b","commit":{"message":"Subject\nBody","author":{"date":"2026-08-01T00:00:00Z"}},"repository":{"full_name":"o/r"}}`, want: []string{"`bbbbbbbbbbbb`", "Subject", "o/r"}},
		{typeValue: "users", item: `{"login":"alice","type":"User","html_url":"https://github.com/alice"}`, want: []string{"@alice", "User"}},
	}
	for _, tt := range tests {
		t.Run(tt.typeValue, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"total_count":1,"incomplete_results":false,"items":[`+tt.item+`]}`)
			}))
			defer server.Close()
			out, err := readGitHubSearch(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/search?q=x&type="+tt.typeValue))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("%s search missing %q:\n%s", tt.typeValue, want, out)
				}
			}
		})
	}
}

func TestGitHubSearchRateLimitUsesSearchResource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Resource", "search")
		w.Header().Set("X-RateLimit-Reset", "1893456000")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"provider body must not leak"}`)
	}))
	defer server.Close()
	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/search?q=x&type=repositories"), testGitHubClient(server.URL, server.URL, ""))
	if result.Outcome != GitHubNativeFailure || result.Err == nil || !strings.Contains(strings.ToLower(result.Err.Error()), "search") || strings.Contains(result.Err.Error(), "provider body") {
		t.Fatalf("Search rate-limit resource/error wrong: %#v", result)
	}
}

func TestUserProfileResolvesProviderTypeWithoutOrgProbe(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/users/alice" {
			t.Fatalf("User profile made guessed/org request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"login":"alice","type":"User","name":"Alice","bio":"Builder","public_repos":12,"public_gists":3,"followers":4,"following":5,"html_url":"https://github.com/alice","created_at":"2020-01-01T00:00:00Z"}`)
	}))
	defer server.Close()
	out, err := readGitHubProfile(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/alice"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`login: "alice"`, `type: "User"`, "public_repositories: 12", "public_gists: 3", "followers: 4", "# alice", "Builder", "?tab=repositories", "?tab=stars"} {
		if !strings.Contains(out, want) {
			t.Fatalf("User profile missing %q:\n%s", want, out)
		}
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("User profile should require one type-resolution call, got %d", calls)
	}
}

func TestOrganizationProfileUsesProviderTypeThenOrgEndpoint(t *testing.T) {
	var sequence []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sequence = append(sequence, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/acme":
			_, _ = io.WriteString(w, `{"login":"acme","type":"Organization","html_url":"https://github.com/acme"}`)
		case "/orgs/acme":
			_, _ = io.WriteString(w, `{"login":"acme","name":"Acme","description":"ignored","blog":"https://acme.test","location":"Earth","public_repos":42,"html_url":"https://github.com/acme","created_at":"2019-01-01T00:00:00Z"}`)
		default:
			t.Fatalf("unexpected org profile request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubProfile(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/acme"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(sequence, ",") != "/users/acme,/orgs/acme" || !strings.Contains(out, `type: "Organization"`) || !strings.Contains(out, "public_repositories: 42") || !strings.Contains(out, "/orgs/acme/people") {
		t.Fatalf("provider-resolved Organization profile wrong requests=%v:\n%s", sequence, out)
	}
}

func TestUserProfileTabsAreBoundedToSelectedEndpoint(t *testing.T) {
	for _, tt := range []struct {
		tab      string
		endpoint string
		body     string
		want     string
	}{
		{tab: "repositories", endpoint: "/users/alice/repos", body: `[{"full_name":"alice/r","html_url":"https://github.com/alice/r","stargazers_count":2}]`, want: "[alice/r]"},
		{tab: "stars", endpoint: "/users/alice/starred", body: `[{"full_name":"o/r","html_url":"https://github.com/o/r","stargazers_count":9}]`, want: "[o/r]"},
		{tab: "gists", endpoint: "/users/alice/gists", body: `[{"id":"abc","html_url":"https://gist.github.com/alice/abc","description":"demo","files":{"a.txt":{"filename":"a.txt"}}}]`, want: "[abc]"},
		{tab: "followers", endpoint: "/users/alice/followers", body: `[{"login":"bob","type":"User","html_url":"https://github.com/bob"}]`, want: "@bob"},
		{tab: "following", endpoint: "/users/alice/following", body: `[{"login":"carol","type":"User","html_url":"https://github.com/carol"}]`, want: "@carol"},
	} {
		t.Run(tt.tab, func(t *testing.T) {
			var listCalls int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/users/alice" {
					_, _ = io.WriteString(w, `{"login":"alice","type":"User"}`)
					return
				}
				atomic.AddInt32(&listCalls, 1)
				if r.URL.Path != tt.endpoint || r.URL.Query().Get("per_page") != "30" || r.URL.Query().Get("page") != "2" {
					t.Fatalf("tab %s request %s?%s", tt.tab, r.URL.Path, r.URL.RawQuery)
				}
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			out, err := readGitHubProfile(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/alice?tab="+tt.tab+"&page=2"))
			if err != nil {
				t.Fatal(err)
			}
			if atomic.LoadInt32(&listCalls) != 1 || !strings.Contains(out, `view: "`+tt.tab+`"`) || !strings.Contains(out, tt.want) {
				t.Fatalf("tab output wrong:\n%s", out)
			}
		})
	}
}

func TestOrganizationRepositoriesAndPublicPeople(t *testing.T) {
	for _, tt := range []struct {
		raw      string
		endpoint string
		want     string
	}{
		{raw: "https://github.com/acme?tab=repositories", endpoint: "/orgs/acme/repos", want: "[acme/r]"},
		{raw: "https://github.com/orgs/acme/people", endpoint: "/orgs/acme/public_members", want: "@member"},
	} {
		t.Run(tt.endpoint, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/users/acme":
					_, _ = io.WriteString(w, `{"login":"acme","type":"Organization"}`)
				case "/orgs/acme":
					_, _ = io.WriteString(w, `{"login":"acme"}`)
				case tt.endpoint:
					if strings.HasSuffix(tt.endpoint, "/repos") {
						_, _ = io.WriteString(w, `[{"full_name":"acme/r","html_url":"https://github.com/acme/r","stargazers_count":1}]`)
					} else {
						_, _ = io.WriteString(w, `[{"login":"member","type":"User","html_url":"https://github.com/member"}]`)
					}
				default:
					t.Fatalf("unexpected org tab request %s", r.URL.Path)
				}
			}))
			defer server.Close()
			out, err := readGitHubProfile(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget(tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tt.want) {
				t.Fatalf("org tab output missing %q:\n%s", tt.want, out)
			}
		})
	}
}

func TestInvalidProfileTabFailsAfterTypeResolutionBeforeListRead(t *testing.T) {
	var listCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/users/alice" {
			_, _ = io.WriteString(w, `{"login":"alice","type":"User"}`)
			return
		}
		atomic.AddInt32(&listCalls, 1)
	}))
	defer server.Close()
	_, err := readGitHubProfile(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/alice?tab=people"))
	if err == nil || !strings.Contains(err.Error(), "User profile tab") || atomic.LoadInt32(&listCalls) != 0 {
		t.Fatalf("invalid User tab handling err=%v listCalls=%d", err, listCalls)
	}
}

func TestProfileProvider404StaysNative(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"private provider body"}`)
	}))
	defer server.Close()
	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/nobody"), testGitHubClient(server.URL, server.URL, ""))
	if result.Outcome != GitHubNativeFailure || result.Err == nil || strings.Contains(result.Err.Error(), "private provider body") {
		t.Fatalf("profile native failure boundary regressed: %#v", result)
	}
}

func TestSearchNavigationKeepsCopiedUIQuery(t *testing.T) {
	target := parseGitHubTarget("https://github.com/search?q=go+cli&type=repositories&s=stars&o=desc&p=2")
	links := GitHubLinkRelations{
		"prev": "https://api.github.com/search/repositories?q=go+cli&page=1",
		"next": "https://api.github.com/search/repositories?q=go+cli&page=3",
	}
	lines := strings.Join(renderGitHubSearchNavigation(target, links), "\n")
	for _, want := range []string{"q=go+cli", "type=repositories", "s=stars", "o=desc", "p=1", "p=3"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("search navigation lost %q:\n%s", want, lines)
		}
	}
}

func TestSupportedSearchURLQueryValidation(t *testing.T) {
	valid := url.Values{"q": []string{"x"}, "type": []string{"repositories"}, "p": []string{"2"}}
	if !isSupportedGitHubSearchURLQuery(valid) {
		t.Fatal("valid search query rejected")
	}
	invalid := url.Values{"q": []string{"x"}, "type": []string{"repositories"}, "p": []string{"zero"}}
	if isSupportedGitHubSearchURLQuery(invalid) {
		t.Fatal("invalid search page accepted")
	}
}

func TestGitHubSearchEmptyPageIsExplicit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":0,"incomplete_results":false,"items":[]}`)
	}))
	defer server.Close()
	out, err := readGitHubSearch(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/search?q=does-not-exist&type=repositories"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "_No results returned on this page._") {
		t.Fatalf("empty Search page was not explicit:\n%s", out)
	}
}
