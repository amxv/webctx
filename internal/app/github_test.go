package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testHTTPResponse(req *http.Request, status int, body string, headers map[string]string) *http.Response {
	h := make(http.Header)
	for key, value := range headers {
		h.Set(key, value)
	}
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        h,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

func testGitHubClient(apiBase, rawBase, token string) *GitHubClient {
	return &GitHubClient{
		httpClient: http.DefaultClient,
		apiBase:    apiBase,
		rawBase:    rawBase,
		token:      token,
		userAgent:  "webctx-test",
	}
}

func TestParseGitHubTargetTable(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		kind      GitHubTargetKind
		tail      string
		fragment  string
		supported bool
	}{
		{name: "repository root", rawURL: "https://github.com/amxv/webctx", kind: GitHubTargetRepository, supported: true},
		{name: "repository root trailing slash", rawURL: "https://github.com/amxv/webctx/", kind: GitHubTargetRepository, supported: true},
		{name: "blob line range", rawURL: "https://github.com/amxv/webctx/blob/main/internal/app/app.go#L20-L40", kind: GitHubTargetBlob, tail: "main/internal/app/app.go", fragment: "L20-L40", supported: true},
		{name: "blob slash ref left unresolved", rawURL: "https://github.com/cli/cli/blob/andyfeller/test/README.md#installation", kind: GitHubTargetBlob, tail: "andyfeller/test/README.md", fragment: "installation", supported: true},
		{name: "tree slash ref left unresolved", rawURL: "https://github.com/cli/cli/tree/andyfeller/test/docs", kind: GitHubTargetTree, tail: "andyfeller/test/docs", supported: true},
		{name: "malformed blob remains authoritative family", rawURL: "https://github.com/amxv/webctx/blob/main", kind: GitHubTargetBlob, tail: "main", supported: true},
		{name: "issue", rawURL: "https://github.com/amxv/webctx/issues/1", kind: GitHubTargetIssue, supported: true},
		{name: "pull request conversation", rawURL: "https://github.com/amxv/webctx/pull/1", kind: GitHubTargetPull, supported: true},
		{name: "pull request files", rawURL: "https://github.com/amxv/webctx/pull/1/files", kind: GitHubTargetPullFiles, supported: true},
		{name: "pull request commits", rawURL: "https://github.com/amxv/webctx/pull/1/commits", kind: GitHubTargetPullCommits, supported: true},
		{name: "pull request checks", rawURL: "https://github.com/amxv/webctx/pull/1/checks", kind: GitHubTargetPullChecks, supported: true},
		{name: "pull request raw diff", rawURL: "https://github.com/amxv/webctx/pull/1.diff", kind: GitHubTargetPullDiff, supported: true},
		{name: "pull request raw patch", rawURL: "https://github.com/amxv/webctx/pull/1.patch", kind: GitHubTargetPullPatch, supported: true},
		{name: "security excluded", rawURL: "https://github.com/amxv/webctx/security/code-scanning", supported: false},
		{name: "settings excluded", rawURL: "https://github.com/amxv/webctx/settings", supported: false},
		{name: "profile", rawURL: "https://github.com/amxv", kind: GitHubTargetProfile, supported: true},
		{name: "other host", rawURL: "https://example.com/amxv/webctx", supported: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := parseGitHubTarget(tt.rawURL)
			if !tt.supported {
				if target != nil {
					t.Fatalf("expected unsupported target, got %#v", target)
				}
				return
			}
			if target == nil {
				t.Fatal("expected native GitHub target")
			}
			if target.Owner == "" || (target.Kind != GitHubTargetProfile && target.Repo == "") || target.Kind != tt.kind {
				t.Fatalf("unexpected target: %#v", target)
			}
			if got := strings.Join(target.Tail, "/"); got != tt.tail {
				t.Fatalf("tail mismatch: got %q want %q", got, tt.tail)
			}
			if target.Fragment != tt.fragment {
				t.Fatalf("fragment mismatch: got %q want %q", target.Fragment, tt.fragment)
			}
		})
	}
}

func TestGitHubClientRESTHeadersAndTokenPrecedence(t *testing.T) {
	var seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		if got := r.Header.Get("X-GitHub-Api-Version"); got != githubRESTAPIVersion {
			t.Errorf("API version header: got %q want %q", got, githubRESTAPIVersion)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept header: got %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "webctx-test" {
			t.Errorf("User-Agent: got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	t.Setenv("GH_TOKEN", "gh-wins")
	t.Setenv("GITHUB_TOKEN", "github-loses")
	client := testGitHubClient(server.URL, server.URL, githubTokenFromEnv())
	if _, err := client.REST(context.Background(), http.MethodGet, "/repos/o/r", ""); err != nil {
		t.Fatal(err)
	}
	if seenAuth != "Bearer gh-wins" {
		t.Fatalf("expected GH_TOKEN precedence, got %q", seenAuth)
	}

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "github-fallback")
	client.token = githubTokenFromEnv()
	if _, err := client.REST(context.Background(), http.MethodGet, "/repos/o/r", ""); err != nil {
		t.Fatal(err)
	}
	if seenAuth != "Bearer github-fallback" {
		t.Fatalf("expected GITHUB_TOKEN fallback, got %q", seenAuth)
	}
}

func TestGitHubClientRetriesPublicGETWithoutRejectedFineGrainedToken(t *testing.T) {
	var authCalls, anonymousCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			atomic.AddInt32(&authCalls, 1)
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"Resource not accessible by personal access token"}`)
			return
		}
		atomic.AddInt32(&anonymousCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"login":"public-user"}]`)
	}))
	defer server.Close()

	client := testGitHubClient(server.URL, server.URL, "narrow-token")
	resp, err := client.RESTPublic(context.Background(), http.MethodGet, "/repos/o/r/subscribers?per_page=30", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp.Body), "public-user") {
		t.Fatalf("anonymous retry body missing: %s", resp.Body)
	}
	if atomic.LoadInt32(&authCalls) != 1 || atomic.LoadInt32(&anonymousCalls) != 1 {
		t.Fatalf("unexpected retry calls auth=%d anon=%d", authCalls, anonymousCalls)
	}
}

func TestGitHubClientKeepsAuthenticatedErrorWhenAnonymousRetryAlsoFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"Resource not accessible by personal access token"}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Requires authentication"}`)
	}))
	defer server.Close()
	client := testGitHubClient(server.URL, server.URL, "narrow-token")
	_, err := client.RESTPublic(context.Background(), http.MethodGet, "/orgs/o/packages/container/p", "")
	if err == nil || !strings.Contains(err.Error(), "configured token permissions") {
		t.Fatalf("expected original fine-grained permission error, got %v", err)
	}
}

func TestGitHubClientNeverSendsTokenToRawOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("raw request leaked Authorization header: %q", got)
		}
		_, _ = io.WriteString(w, "plain text")
	}))
	defer server.Close()
	client := testGitHubClient("https://api.example.test", server.URL, "fake-secret-token")
	if _, err := client.Raw(context.Background(), server.URL+"/o/r/main/file.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubLinkPaginationPrimitives(t *testing.T) {
	header := `<https://api.github.com/resource?page=1>; rel="prev", <https://api.github.com/resource?page=3>; rel="next", <https://api.github.com/resource?page=9>; rel="last"`
	links := ParseGitHubLinkHeader(header)
	if links["prev"] != "https://api.github.com/resource?page=1" || links["next"] != "https://api.github.com/resource?page=3" || links["last"] != "https://api.github.com/resource?page=9" {
		t.Fatalf("unexpected links: %#v", links)
	}
	resp := GitHubResponse{Header: http.Header{"Link": []string{header}}}
	if got := resp.Links()["next"]; got != links["next"] {
		t.Fatalf("response Links mismatch: %q", got)
	}
}

func TestGitHubClientErrorClassificationAndSecretNonLeakage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/not-found":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"Resource not accessible"}`)
		case "/rate":
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1893456000")
			w.Header().Set("X-RateLimit-Resource", "core")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"API rate limit exceeded"}`)
		case "/secondary":
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"message":"secondary rate limit"}`)
		case "/provider-limit":
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"This API returns blobs up to 100 MB in size"}`)
		case "/auth":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"fake-secret-token must never appear"}`)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := testGitHubClient(server.URL, server.URL, "")
	tests := []struct {
		path string
		kind GitHubErrorKind
	}{
		{path: "/not-found", kind: GitHubErrorNotFound},
		{path: "/forbidden", kind: GitHubErrorForbidden},
		{path: "/rate", kind: GitHubErrorRateLimit},
		{path: "/secondary", kind: GitHubErrorRateLimit},
		{path: "/provider-limit", kind: GitHubErrorProviderLimit},
	}
	for _, tt := range tests {
		_, err := client.REST(context.Background(), http.MethodGet, tt.path, "")
		if err == nil {
			t.Fatalf("%s: expected error", tt.path)
		}
		ghErr, ok := err.(*GitHubError)
		if !ok || ghErr.Kind != tt.kind {
			t.Fatalf("%s: got %#v, want kind %s", tt.path, err, tt.kind)
		}
	}

	_, requiredErr := client.REST(context.Background(), http.MethodGet, "/auth", "")
	if requiredErr == nil || !strings.Contains(requiredErr.Error(), "authentication is required") {
		t.Fatalf("no-token 401 wording is not truthful: %v", requiredErr)
	}

	_, rateErr := client.REST(context.Background(), http.MethodGet, "/rate", "")
	if !strings.Contains(rateErr.Error(), "Resource: core") || !strings.Contains(rateErr.Error(), "Reset:") || !strings.Contains(rateErr.Error(), "GH_TOKEN") {
		t.Fatalf("rate error missing truthful context: %q", rateErr)
	}
	_, secondaryErr := client.REST(context.Background(), http.MethodGet, "/secondary", "")
	if !strings.Contains(secondaryErr.Error(), "Retry after: 60") {
		t.Fatalf("secondary limit missing retry context: %q", secondaryErr)
	}

	client.token = "fake-secret-token"
	_, authErr := client.REST(context.Background(), http.MethodGet, "/auth", "")
	if authErr == nil {
		t.Fatal("expected auth error")
	}
	if strings.Contains(authErr.Error(), "fake-secret-token") {
		t.Fatalf("secret leaked through error: %q", authErr)
	}
}

func TestResolveGitHubRefPathSlashRef(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ref := r.URL.Query().Get("ref")
		if ref == "feature/docs" && r.URL.Path == "/repos/o/r/contents/guide.md" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"type":"file","name":"guide.md","path":"guide.md","size":10}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	defer server.Close()

	target := parseGitHubTarget("https://github.com/o/r/blob/feature/docs/guide.md")
	resolved, err := resolveGitHubRefPath(context.Background(), testGitHubClient(server.URL, server.URL, ""), target, "file")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Ref != "feature/docs" || resolved.Path != "guide.md" {
		t.Fatalf("unexpected resolution: %#v", resolved)
	}
}

func TestResolveGitHubRefPathDetectsOverlappingAmbiguity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ref := r.URL.Query().Get("ref")
		valid := (ref == "feature" && r.URL.Path == "/repos/o/r/contents/docs/guide.md") || (ref == "feature/docs" && r.URL.Path == "/repos/o/r/contents/guide.md")
		if valid {
			_, _ = io.WriteString(w, `{"type":"file","name":"guide.md","path":"guide.md","size":10}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	defer server.Close()

	target := parseGitHubTarget("https://github.com/o/r/blob/feature/docs/guide.md")
	_, err := resolveGitHubRefPath(context.Background(), testGitHubClient(server.URL, server.URL, ""), target, "file")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "feature/docs") {
		t.Fatalf("expected truthful ambiguity, got %v", err)
	}
}

func TestGitHubRepositoryRootPreviewAndHints(t *testing.T) {
	preFence := "# webctx\n\n<!-- hidden-root-marker -->\n\n" + strings.Repeat("A useful paragraph with enough words to exercise the repository preview boundary.\n\n", 61)
	longReadme := preFence + "```go\n" + strings.Repeat("fmt.Println(\"inside a long fenced block\")\n", 80) + "```\n\n## After\nnot in preview\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(longReadme))
	readmeURL := "https://github.com/amxv/webctx/blob/main/README.md"

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/amxv/webctx":
			_, _ = io.WriteString(w, `{"full_name":"amxv/webctx","html_url":"https://github.com/amxv/webctx","description":"fast & clean context","language":"Go","default_branch":"main","stargazers_count":7,"watchers_count":999,"topics":["web","agents"],"archived":false,"fork":false,"visibility":"public","license":{"spdx_id":"Apache-2.0","name":"Apache License 2.0"}}`)
		case "/repos/amxv/webctx/readme":
			payload := githubContent{Type: "file", Name: "README.md", Path: "README.md", Size: int64(len(longReadme)), Encoding: "base64", Content: encoded, HTMLURL: readmeURL, DownloadURL: "https://raw.githubusercontent.com/amxv/webctx/main/README.md"}
			_ = json.NewEncoder(w).Encode(payload)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()

	target := parseGitHubTarget("https://github.com/amxv/webctx")
	result := readGitHubNativeWithClient(context.Background(), target, testGitHubClient(api.URL, api.URL, ""))
	if result.Outcome != GitHubNativeSuccess {
		t.Fatalf("root read failed: %v", result.Err)
	}
	out := result.Markdown
	if !strings.HasPrefix(out, "---\nrepository: \"amxv/webctx\"") {
		t.Fatalf("root output does not begin with frontmatter: %q", out[:min(len(out), 120)])
	}
	for _, want := range []string{"description: \"fast & clean context\"", "language: \"Go\"", "default_branch: \"main\"", "license: \"Apache-2.0\"", "stars: 7", "topics: [\"agents\",\"web\"]"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing frontmatter field %q", want)
		}
	}
	if strings.Contains(out, "watchers") || strings.Contains(out, "999") {
		t.Fatalf("watchers alias leaked into root metadata: %q", out)
	}
	if strings.Contains(out, "hidden-root-marker") {
		t.Fatal("human repository preview retained invisible HTML comment")
	}
	if !strings.Contains(out, "README preview truncated") || !strings.Contains(out, "Full README: "+readmeURL) {
		t.Fatalf("missing truncation/full README direction: %q", out)
	}
	if strings.Count(out, "```")%2 != 0 {
		t.Fatal("root preview ended inside an open Markdown fence")
	}
	previewStart := strings.Index(out, "## README preview\n\n")
	previewEnd := strings.Index(out, "\n\n> README preview truncated")
	if previewStart < 0 || previewEnd < 0 {
		t.Fatal("could not locate README preview boundaries")
	}
	preview := out[previewStart+len("## README preview\n\n") : previewEnd]
	if got := utf8.RuneCountInString(preview); got > githubRootPreviewRunes {
		t.Fatalf("preview exceeds rune budget: %d", got)
	}
	for _, want := range []string{"#L20-L40", "/tree/main/path/to/directory", "/issues", "/pull/123"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing useful native hint %q", want)
		}
	}
	if strings.Count(out[strings.Index(out, "## Useful GitHub URLs"):], "\n-") > 5 {
		t.Fatal("root native hints exceeded the concise orientation budget")
	}
	if strings.Contains(out, "GH_TOKEN") || strings.Contains(out, "GITHUB_TOKEN") {
		t.Fatal("successful anonymous root read nags about authentication")
	}
}

func TestGitHubRepositoryRootDoesNotFalseTruncate(t *testing.T) {
	readme := "# Tiny\n\nvisible\n<!-- hidden -->\n\n```html\n<!-- visible-in-code-fence -->\n```\n"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r":
			_, _ = io.WriteString(w, `{"full_name":"o/r","html_url":"https://github.com/o/r","default_branch":"main","stargazers_count":0}`)
		case "/repos/o/r/readme":
			_ = json.NewEncoder(w).Encode(githubContent{Type: "file", Name: "README.md", Path: "README.md", Size: int64(len(readme)), Encoding: "base64", Content: base64.StdEncoding.EncodeToString([]byte(readme)), HTMLURL: "https://github.com/o/r/blob/main/README.md"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()
	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r"), testGitHubClient(api.URL, api.URL, ""))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if strings.Contains(result.Markdown, "preview truncated") {
		t.Fatal("small README was falsely marked truncated")
	}
	if strings.Contains(result.Markdown, "<!-- hidden -->") || !strings.Contains(result.Markdown, "visible") {
		t.Fatalf("unexpected human README sanitization: %q", result.Markdown)
	}
	if !strings.Contains(result.Markdown, "visible-in-code-fence") {
		t.Fatalf("human-visible fenced comment was stripped: %q", result.Markdown)
	}
	if !strings.Contains(result.Markdown, "stars: 0") {
		t.Fatal("zero star count should remain useful metadata")
	}
}

func TestGitHubPublicBlobRawFastPathPreservesSourceComments(t *testing.T) {
	var apiCalls int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer api.Close()
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "# Source\n\n<!-- keep-source-comment -->\nbody\n")
	}))
	defer raw.Close()

	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/blob/feature/docs/README.md"), testGitHubClient(api.URL, raw.URL, ""))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if !strings.Contains(result.Markdown, "keep-source-comment") {
		t.Fatal("direct source read stripped an HTML comment")
	}
	if apiCalls != 0 {
		t.Fatalf("public raw fast path spent %d REST requests", apiCalls)
	}
}

func TestGitHubBlobLineSelectors(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "alpha-only\nbeta-only\ngamma-only\ndelta-only\n")
	}))
	defer raw.Close()
	client := testGitHubClient("https://api.example.test", raw.URL, "")

	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/blob/main/file.go#L2-L3"), client)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if !strings.Contains(result.Markdown, "beta-only\ngamma-only") || strings.Contains(result.Markdown, "alpha-only") || strings.Contains(result.Markdown, "delta-only") {
		t.Fatalf("line range was not narrowed: %q", result.Markdown)
	}
	if !strings.Contains(result.Markdown, "**Lines:** 2-3 of 4") {
		t.Fatalf("missing source coordinates: %q", result.Markdown)
	}

	for _, fragment := range []string{"L3-L2", "L0", "L2-L9"} {
		result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/blob/main/file.go#"+fragment), client)
		if result.Outcome != GitHubNativeFailure || result.Err == nil {
			t.Fatalf("%s: expected truthful selector failure, got %#v", fragment, result)
		}
	}
}

func TestGitHubBlobMarkdownHeadingSelectors(t *testing.T) {
	content := "# Guide\nintro\n\n## Install\nfirst install\n### Details\nchild\n\n## Usage\nusage body\n\n## Install\nsecond install\n\n## End\nend\n"
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, content)
	}))
	defer raw.Close()
	client := testGitHubClient("https://api.example.test", raw.URL, "")

	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/blob/main/README.md#install"), client)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if !strings.Contains(result.Markdown, "first install") || !strings.Contains(result.Markdown, "### Details") || strings.Contains(result.Markdown, "usage body") || strings.Contains(result.Markdown, "second install") {
		t.Fatalf("heading section was not bounded correctly: %q", result.Markdown)
	}

	second := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/blob/main/README.md#install-1"), client)
	if second.Err != nil || !strings.Contains(second.Markdown, "second install") || strings.Contains(second.Markdown, "first install") {
		t.Fatalf("duplicate heading selector failed: %#v", second)
	}

	missing := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/blob/main/README.md#does-not-exist"), client)
	if missing.Outcome != GitHubNativeFailure || missing.Err == nil || !strings.Contains(missing.Err.Error(), "was not found") {
		t.Fatalf("missing heading did not fail truthfully: %#v", missing)
	}
}

func TestGitHubBlobBinaryAndProviderLimit(t *testing.T) {
	binaryRaw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("not-real-png-but-explicitly-binary-mime"))
	}))
	defer binaryRaw.Close()
	binaryResult := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/blob/main/image.png"), testGitHubClient("https://api.example.test", binaryRaw.URL, ""))
	if binaryResult.Err != nil || !strings.Contains(binaryResult.Markdown, "Binary/non-text source") || !strings.Contains(binaryResult.Markdown, "Raw/download:") {
		t.Fatalf("binary handling failed: %#v", binaryResult)
	}

	client := &GitHubClient{
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			resp := testHTTPResponse(req, http.StatusOK, "", map[string]string{"Content-Type": "text/plain"})
			resp.ContentLength = githubBlobMaxBytes + 1
			return resp, nil
		})},
		apiBase:   "https://api.example.test",
		rawBase:   "https://raw.example.test",
		userAgent: "webctx-test",
	}
	limited := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/blob/main/huge.txt"), client)
	if limited.Outcome != GitHubNativeFailure || limited.Err == nil || !strings.Contains(limited.Err.Error(), "100 MB") {
		t.Fatalf("provider-size limit not surfaced: %#v", limited)
	}
}

func TestGitHubPrivateBlobUsesAuthenticatedContentsFallback(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("token leaked to raw host")
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "not found")
	}))
	defer raw.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fake-private-token" {
			t.Errorf("authenticated contents request missing token: %q", got)
		}
		ref := r.URL.Query().Get("ref")
		if ref == "private/main" && r.URL.Path == "/repos/o/r/contents/secret.md" {
			content := "# Secret source\n<!-- private-source-comment -->\ncontent\n"
			_ = json.NewEncoder(w).Encode(githubContent{Type: "file", Name: "secret.md", Path: "secret.md", Size: int64(len(content)), Encoding: "base64", Content: base64.StdEncoding.EncodeToString([]byte(content)), HTMLURL: "https://github.com/o/r/blob/private/main/secret.md"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	defer api.Close()

	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/blob/private/main/secret.md"), testGitHubClient(api.URL, raw.URL, "fake-private-token"))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if !strings.Contains(result.Markdown, "Secret source") || !strings.Contains(result.Markdown, "private-source-comment") {
		t.Fatalf("authenticated source fallback lost content: %q", result.Markdown)
	}
}

func TestGitHubPrivateBlobProviderSizeMetadata(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer raw.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ref := r.URL.Query().Get("ref")
		if ref == "main" && r.URL.Path == "/repos/o/r/contents/huge.bin" {
			_ = json.NewEncoder(w).Encode(githubContent{Type: "file", Name: "huge.bin", Path: "huge.bin", Size: githubBlobMaxBytes + 1})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer api.Close()
	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/blob/main/huge.bin"), testGitHubClient(api.URL, raw.URL, "fake-token"))
	if result.Outcome != GitHubNativeFailure || result.Err == nil || !strings.Contains(result.Err.Error(), "100 MB") {
		t.Fatalf("large authenticated source not surfaced: %#v", result)
	}
}

func TestGitHubTreeSlashRefOneLevelREADME(t *testing.T) {
	readmeURL := "https://github.com/o/r/blob/feature/docs/subdir/README.md"
	readme := "# Directory\n\n<!-- hidden-tree-marker -->\n\n" + strings.Repeat("directory paragraph with useful context\n\n", 180)
	var requested []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path+"?ref="+r.URL.Query().Get("ref")+" accept="+r.Header.Get("Accept"))
		ref := r.URL.Query().Get("ref")
		if ref == "feature/docs" && r.URL.Path == "/repos/o/r/contents/subdir" {
			entries := []githubContent{
				{Type: "file", Name: "a.txt", Path: "subdir/a.txt", Size: 4, HTMLURL: "https://github.com/o/r/blob/feature/docs/subdir/a.txt"},
				{Type: "dir", Name: "nested", Path: "subdir/nested", HTMLURL: "https://github.com/o/r/tree/feature/docs/subdir/nested"},
				{Type: "file", Name: "README.md", Path: "subdir/README.md", Size: int64(len(readme)), HTMLURL: readmeURL},
			}
			_ = json.NewEncoder(w).Encode(entries)
			return
		}
		if ref == "feature/docs" && r.URL.Path == "/repos/o/r/contents/subdir/README.md" && r.Header.Get("Accept") == "application/vnd.github.raw+json" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, readme)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	defer api.Close()

	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/tree/feature/docs/subdir"), testGitHubClient(api.URL, api.URL, ""))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	out := result.Markdown
	for _, want := range []string{"ref: \"feature/docs\"", "path: \"subdir\"", "- file [a.txt]", "- dir [nested/]", "## Directory README", "Directory README preview truncated", "Full README: " + readmeURL} {
		if !strings.Contains(out, want) {
			t.Errorf("tree output missing %q", want)
		}
	}
	if strings.Contains(out, "hidden-tree-marker") {
		t.Fatal("tree human-view README retained hidden comment")
	}
	for _, req := range requested {
		if strings.Contains(req, "/nested/") {
			t.Fatalf("tree reader recursed unexpectedly: %s", req)
		}
	}
}

func TestGitHubTreeProviderCeilingIsTruthful(t *testing.T) {
	entries := make([]githubContent, 1000)
	for i := range entries {
		entries[i] = githubContent{Type: "file", Name: fmt.Sprintf("file-%04d.txt", i), Path: fmt.Sprintf("dir/file-%04d.txt", i), HTMLURL: fmt.Sprintf("https://github.com/o/r/blob/main/dir/file-%04d.txt", i)}
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref") == "main" && r.URL.Path == "/repos/o/r/contents/dir" {
			_ = json.NewEncoder(w).Encode(entries)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	defer api.Close()
	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/tree/main/dir"), testGitHubClient(api.URL, api.URL, ""))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if !strings.Contains(result.Markdown, "complete: false") || !strings.Contains(result.Markdown, "at most 1,000 entries") {
		t.Fatalf("directory provider ceiling not surfaced: %q", result.Markdown[:min(len(result.Markdown), 500)])
	}
}

func TestGitHubTokenKeysUseDotEnvAndKeychainPrecedence(t *testing.T) {
	tmp := t.TempDir()
	envPath := tmp + "/.env.local"
	if err := os.WriteFile(envPath, []byte("GH_TOKEN=from-file-gh\nGITHUB_TOKEN=from-file-github\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_TOKEN", "from-process")
	t.Setenv("GITHUB_TOKEN", "")
	loadDotEnvFile(envPath)
	if got := os.Getenv("GH_TOKEN"); got != "from-process" {
		t.Fatalf("process GH_TOKEN did not win: %q", got)
	}
	if got := os.Getenv("GITHUB_TOKEN"); got != "" {
		t.Fatalf("explicitly empty GITHUB_TOKEN was overwritten: %q", got)
	}

	oldGH, hadGH := os.LookupEnv("GH_TOKEN")
	if err := os.Unsetenv("GH_TOKEN"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadGH {
			_ = os.Setenv("GH_TOKEN", oldGH)
		} else {
			_ = os.Unsetenv("GH_TOKEN")
		}
	})
	for _, key := range []string{"BRAVE_API_KEY", "TAVILY_API_KEY", "EXA_API_KEY", "FIRECRAWL_API_KEY", "GITHUB_TOKEN"} {
		t.Setenv(key, "")
	}
	originalLookup := keychainLookup
	t.Cleanup(func() { keychainLookup = originalLookup })
	keychainLookup = func(key string) (string, error) {
		if key == "GH_TOKEN" {
			return "from-keychain-gh", nil
		}
		return "", nil
	}
	loadKeychainEnv()
	if got := os.Getenv("GH_TOKEN"); got != "from-keychain-gh" {
		t.Fatalf("GitHub token was not loaded from Keychain seam: %q", got)
	}
}

func TestReadLinkDirectMarkdownRegression(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })
	var firecrawlCalled bool
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodHead && req.URL.String() == "https://docs.example/guide.md":
			resp := testHTTPResponse(req, http.StatusOK, "", map[string]string{"Content-Type": "text/plain"})
			resp.ContentLength = 100
			return resp, nil
		case req.Method == http.MethodGet && req.URL.String() == "https://docs.example/guide.md":
			return testHTTPResponse(req, http.StatusOK, "# Guide\n\ndirect markdown body\n", map[string]string{"Content-Type": "text/plain"}), nil
		case strings.Contains(req.URL.Host, "firecrawl"):
			firecrawlCalled = true
			return testHTTPResponse(req, http.StatusInternalServerError, "", nil), nil
		default:
			return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
		}
	})}
	t.Setenv("FIRECRAWL_API_KEY", "")
	out, err := ReadLink("https://docs.example/guide")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "direct markdown body") || firecrawlCalled {
		t.Fatalf("direct Markdown fallback regressed: %q", out)
	}
}

func TestReadLinkUnsupportedGitHubFallsThroughToFirecrawlWithExistingSettings(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })
	var firecrawlPayload map[string]any
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodHead && req.URL.Host == "github.com" {
			return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
		}
		if req.Method == http.MethodPost && req.URL.String() == "https://api.firecrawl.dev/v2/scrape" {
			body, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(body, &firecrawlPayload)
			response := `{"success":true,"data":{"metadata":{"title":"Wiki page"},"markdown":"scraped wiki body"}}`
			return testHTTPResponse(req, http.StatusOK, response, map[string]string{"Content-Type": "application/json"}), nil
		}
		return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
	})}
	t.Setenv("FIRECRAWL_API_KEY", "fake-firecrawl-key")
	out, err := ReadLink("https://github.com/o/r/wiki/Guide")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "scraped wiki body") {
		t.Fatalf("unsupported GitHub route was swallowed: %q", out)
	}
	for key, want := range map[string]any{
		"onlyMainContent":     true,
		"skipTlsVerification": true,
		"blockAds":            true,
		"removeBase64Images":  true,
		"maxAge":              float64(600000),
	} {
		if got := firecrawlPayload[key]; got != want {
			t.Errorf("Firecrawl setting %s changed: got %#v want %#v", key, got, want)
		}
	}
	formats, _ := firecrawlPayload["formats"].([]any)
	if len(formats) != 1 || formats[0] != "markdown" {
		t.Fatalf("Firecrawl formats changed: %#v", firecrawlPayload["formats"])
	}
	if _, ok := firecrawlPayload["excludeTags"]; !ok {
		t.Fatal("Firecrawl excludeTags setting disappeared")
	}
}

func TestReadLinkGitHubPackageAuthFailureUsesExplicitBestEffortFirecrawl(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })
	var firecrawlCalled bool
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "api.github.com" && req.URL.Path == "/orgs/acme/packages/container/widget":
			return testHTTPResponse(req, http.StatusUnauthorized, `{"message":"Requires authentication"}`, map[string]string{"Content-Type": "application/json"}), nil
		case req.Method == http.MethodPost && req.URL.String() == "https://api.firecrawl.dev/v2/scrape":
			firecrawlCalled = true
			body := `{"success":true,"data":{"metadata":{"title":"Package widget · GitHub"},"markdown":"# widget\n\nPublic Latest\n\n### Recent tagged image versions\n\n- v1\n\n$ docker pull ghcr.io/acme/widget:v1\n"}}`
			return testHTTPResponse(req, http.StatusOK, body, map[string]string{"Content-Type": "application/json"}), nil
		default:
			return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
		}
	})}
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("FIRECRAWL_API_KEY", "fake-firecrawl-key")

	out, err := ReadLink("https://github.com/orgs/acme/packages/container/package/widget")
	if err != nil {
		t.Fatal(err)
	}
	if !firecrawlCalled {
		t.Fatal("expected GitHub Package auth failure to use best-effort Firecrawl")
	}
	for _, want := range []string{"Best-effort GitHub Package page crawl", "may be incomplete", "Recent tagged image versions", "ghcr.io/acme/widget:v1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Package fallback output missing %q:\n%s", want, out)
		}
	}
}

func TestReadLinkGitHubPackageRateLimitDoesNotUseFirecrawl(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })
	var firecrawlCalled bool
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.github.com" {
			return testHTTPResponse(req, http.StatusForbidden, `{"message":"API rate limit exceeded"}`, map[string]string{
				"X-RateLimit-Remaining": "0",
				"X-RateLimit-Resource":  "core",
				"X-RateLimit-Reset":     "1893456000",
			}), nil
		}
		if req.URL.Host == "api.firecrawl.dev" {
			firecrawlCalled = true
		}
		return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
	})}
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("FIRECRAWL_API_KEY", "fake-firecrawl-key")

	_, err := ReadLink("https://github.com/orgs/acme/packages/container/package/widget")
	if err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("expected native rate-limit error, got %v", err)
	}
	if firecrawlCalled {
		t.Fatal("Package rate limit must remain authoritative instead of falling back to Firecrawl")
	}
}

func TestReadLinkGitHubPackageWithoutFirecrawlKeyKeepsNativeAuthError(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.github.com" {
			return testHTTPResponse(req, http.StatusUnauthorized, `{"message":"Requires authentication"}`, nil), nil
		}
		return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
	})}
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("FIRECRAWL_API_KEY", "")

	_, err := ReadLink("https://github.com/orgs/acme/packages/container/package/widget")
	if err == nil || !strings.Contains(err.Error(), "authentication is required") || strings.Contains(err.Error(), "Firecrawl") {
		t.Fatalf("expected unchanged native auth error without Firecrawl key, got %v", err)
	}
}

func TestReadLinkNonPackageNativeAuthFailureDoesNotUseFirecrawl(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })
	var firecrawlCalled bool
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.github.com" {
			return testHTTPResponse(req, http.StatusUnauthorized, `{"message":"Requires authentication"}`, nil), nil
		}
		if req.URL.Host == "api.firecrawl.dev" {
			firecrawlCalled = true
		}
		return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
	})}
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("FIRECRAWL_API_KEY", "fake-firecrawl-key")

	_, err := ReadLink("https://github.com/o/r/issues/1")
	if err == nil || !strings.Contains(err.Error(), "authentication is required") {
		t.Fatalf("expected native Issue auth error, got %v", err)
	}
	if firecrawlCalled {
		t.Fatal("non-Package native auth failures must not fall back to Firecrawl")
	}
}
