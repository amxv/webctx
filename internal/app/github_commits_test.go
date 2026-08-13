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

func TestParseGitHubCommitCompareHistoryBlameTargets(t *testing.T) {
	tests := []struct {
		raw  string
		kind GitHubTargetKind
		name string
		tail []string
	}{
		{raw: "https://github.com/o/r/commit/abc123", kind: GitHubTargetCommit, name: "abc123"},
		{raw: "https://github.com/o/r/commit/abc123.diff", kind: GitHubTargetCommitDiff, name: "abc123"},
		{raw: "https://github.com/o/r/commit/abc123.patch", kind: GitHubTargetCommitPatch, name: "abc123"},
		{raw: "https://github.com/o/r/compare/main...feature/foo", kind: GitHubTargetCompare, tail: []string{"main", "feature/foo"}},
		{raw: "https://github.com/o/r/compare/feature/base...feature/head.diff", kind: GitHubTargetCompareDiff, tail: []string{"feature/base", "feature/head"}},
		{raw: "https://github.com/o/r/compare/main...next.patch", kind: GitHubTargetComparePatch, tail: []string{"main", "next"}},
		{raw: "https://github.com/o/r/commits/feature/slash/path.go", kind: GitHubTargetHistory, tail: []string{"feature", "slash", "path.go"}},
		{raw: "https://github.com/o/r/blame/feature/slash/path.go", kind: GitHubTargetBlame, tail: []string{"feature", "slash", "path.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			target := parseGitHubTarget(tt.raw)
			if target == nil || target.Kind != tt.kind || target.Name != tt.name {
				t.Fatalf("unexpected target: %#v", target)
			}
			if strings.Join(target.Tail, "|") != strings.Join(tt.tail, "|") {
				t.Fatalf("tail=%v want %v", target.Tail, tt.tail)
			}
		})
	}
	for _, raw := range []string{
		"https://github.com/o/r/commit/",
		"https://github.com/o/r/compare/main..head",
		"https://github.com/o/r/commits",
		"https://github.com/o/r/blame/main",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("expected unsupported target for %s, got %#v", raw, target)
		}
	}
}

func TestReadGitHubCommitPaginatesFilesAndComments(t *testing.T) {
	var commitPages, commentPages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/commits/ref":
			atomic.AddInt32(&commitPages, 1)
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `{"sha":"fullsha","files":[{"filename":"b.go","status":"added","additions":1,"changes":1,"patch":"@@ -0,0 +1 @@\n+b"}]}`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/commits/ref?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `{
                    "sha":"fullsha","html_url":"https://github.com/o/r/commit/fullsha",
                    "author":{"login":"alice"},"committer":{"login":"mergebot"},
                    "commit":{"message":"Subject\n\nBody","author":{"name":"Alice","date":"2026-08-01T00:00:00Z"},"committer":{"name":"Bot","date":"2026-08-01T01:00:00Z"},"comment_count":2,"verification":{"verified":true,"reason":"valid","verified_at":"2026-08-01T01:00:01Z"}},
                    "stats":{"total":3,"additions":2,"deletions":1},
                    "parents":[{"sha":"parentparentparent","html_url":"https://github.com/o/r/commit/parent"}],
                    "files":[{"filename":"a.go","status":"modified","additions":1,"deletions":1,"changes":2,"patch":"@@ -1 +1 @@\n-old\n+new"}]
                }`)
		case "/repos/o/r/commits/fullsha/comments":
			atomic.AddInt32(&commentPages, 1)
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `[{"id":2,"body":null,"user":{"login":"gone"},"created_at":"2026-08-03T00:00:00Z"}]`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/commits/fullsha/comments?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"id":1,"body":"Visible comment\n<!-- hidden marker -->\nkept","html_url":"https://github.com/o/r/commit/fullsha#commitcomment-1","user":{"login":"reviewer"},"author_association":"MEMBER","path":"a.go","line":1,"created_at":"2026-08-02T00:00:00Z"}]`)
		default:
			t.Fatalf("unexpected commit request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()
	out, err := readGitHubCommit(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/commit/ref"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`sha: "fullsha"`, `author: "@alice"`, `committer: "@mergebot"`, `verified: true`, `verification_reason: "valid"`,
		"Subject\n\nBody", "## a.go", "## b.go", "Comment by @reviewer", "Visible comment", "kept", "`a.go` line 1", "Commit comment body is unavailable or deleted",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("commit output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hidden marker") {
		t.Fatalf("commit comment HTML marker leaked:\n%s", out)
	}
	if atomic.LoadInt32(&commitPages) != 2 || atomic.LoadInt32(&commentPages) != 2 {
		t.Fatalf("pagination incomplete: commit=%d comments=%d", commitPages, commentPages)
	}
}

func TestRenderGitHubCommitSurfacesThreeThousandFileCap(t *testing.T) {
	detail := githubCommitDetail{SHA: "sha", Files: make([]githubPullFile, githubCommitFilesMax)}
	out := renderGitHubCommit(&GitHubTarget{Owner: "o", Repo: "r"}, detail, nil, false, true)
	if !strings.Contains(out, "files_complete: false") || !strings.Contains(out, "3,000-file maximum") {
		t.Fatalf("commit provider cap missing")
	}
}

func TestCommitRawDiffPatchPreserveMedia(t *testing.T) {
	for _, tt := range []struct {
		patch  bool
		accept string
		body   string
	}{
		{accept: "application/vnd.github.v3.diff", body: "diff --git a/a b/a\n-old\n+new\n"},
		{patch: true, accept: "application/vnd.github.v3.patch", body: "From abc Mon Sep 17 00:00:00 2001\nSubject: commit\n"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Accept"); got != tt.accept {
				t.Errorf("Accept=%q want %q", got, tt.accept)
			}
			_, _ = io.WriteString(w, tt.body)
		}))
		got, err := readGitHubCommitRawDiff(context.Background(), testGitHubClient(server.URL, server.URL, ""), &GitHubTarget{Owner: "o", Repo: "r", Name: "sha"}, tt.patch)
		server.Close()
		if err != nil || got != tt.body {
			t.Fatalf("raw commit media got %q err=%v", got, err)
		}
	}
}

func TestReadGitHubComparePaginatesCommitsButKeepsFirstPageFiles(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.RequestURI, "main...feature%2Ffoo") && !strings.Contains(r.RequestURI, "main...feature/foo") {
			t.Errorf("slash-containing compare head was not preserved in endpoint: %s", r.RequestURI)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_, _ = io.WriteString(w, `{"status":"ahead","ahead_by":2,"behind_by":0,"total_commits":2,"commits":[{"sha":"b","html_url":"https://github.com/o/r/commit/b","commit":{"message":"Second","author":{"date":"2026-08-02T00:00:00Z"}}}]}`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/compare/main...feature%%2Ffoo?per_page=100&page=2>; rel="next"`, server.URL))
		_, _ = io.WriteString(w, `{
            "html_url":"https://github.com/o/r/compare/main...feature/foo","status":"ahead","ahead_by":2,"behind_by":0,"total_commits":2,
            "commits":[{"sha":"a","html_url":"https://github.com/o/r/commit/a","commit":{"message":"First","author":{"date":"2026-08-01T00:00:00Z"}}}],
            "files":[{"filename":"a.go","status":"modified","patch":"@@ -1 +1 @@\n-a\n+b"}]
        }`)
	}))
	defer server.Close()
	out, err := readGitHubCompare(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/compare/main...feature/foo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`base: "main"`, `head: "feature/foo"`, "total_commits: 2", "commits_returned: 2", "commits_complete: true", "files_returned: 1", "files_complete: true", "First", "Second", "## a.go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("compare output missing %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "## a.go") != 1 {
		t.Fatalf("files should only come from comparison first page:\n%s", out)
	}
}

func TestRenderGitHubCompareSurfacesThreeHundredFileCapAndCommitMismatch(t *testing.T) {
	result := githubCompareResult{Status: "ahead", TotalCommits: 5, Files: make([]githubPullFile, githubCompareFilesMax), Commits: []githubPullCommit{{SHA: "a"}}}
	out := renderGitHubCompare(&GitHubTarget{Owner: "o", Repo: "r"}, "main", "head", result, false, false)
	if !strings.Contains(out, "commits_complete: false") || !strings.Contains(out, "files_complete: false") || !strings.Contains(out, "up to 300 files") || !strings.Contains(out, "commit history is incomplete") {
		t.Fatalf("compare completeness truth missing")
	}
}

func TestCompareRawDiffPatchPreserveMedia(t *testing.T) {
	for _, tt := range []struct {
		patch  bool
		accept string
		body   string
	}{
		{accept: "application/vnd.github.v3.diff", body: "diff compare\n"},
		{patch: true, accept: "application/vnd.github.v3.patch", body: "patch compare\n"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Accept"); got != tt.accept {
				t.Errorf("Accept=%q want %q", got, tt.accept)
			}
			_, _ = io.WriteString(w, tt.body)
		}))
		got, err := readGitHubCompareRawDiff(context.Background(), testGitHubClient(server.URL, server.URL, ""), &GitHubTarget{Owner: "o", Repo: "r", Tail: []string{"base", "head"}}, tt.patch)
		server.Close()
		if err != nil || got != tt.body {
			t.Fatalf("raw compare media got %q err=%v", got, err)
		}
	}
}

func TestHistoryResolverHandlesSlashRefAndBoundedPageNavigation(t *testing.T) {
	var resolverCalls int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/commits" {
			t.Fatalf("unexpected history path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		sha := r.URL.Query().Get("sha")
		path := r.URL.Query().Get("path")
		perPage := r.URL.Query().Get("per_page")
		if perPage == "1" {
			atomic.AddInt32(&resolverCalls, 1)
			if sha == "feature/slash" && path == "path.go" {
				_, _ = io.WriteString(w, `[{"sha":"probe"}]`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
			return
		}
		if sha != "feature/slash" || path != "path.go" || perPage != "30" {
			t.Fatalf("resolved history query incorrect: %s", r.URL.RawQuery)
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/commits?sha=feature%%2Fslash&path=path.go&per_page=30&page=2>; rel="next"`, server.URL))
		_, _ = io.WriteString(w, `[{"sha":"abc","html_url":"https://github.com/o/r/commit/abc","author":{"login":"alice"},"commit":{"message":"Change path","author":{"date":"2026-08-01T00:00:00Z"}}}]`)
	}))
	defer server.Close()
	target := parseGitHubTarget("https://github.com/o/r/commits/feature/slash/path.go")
	out, err := readGitHubHistory(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `ref: "feature/slash"`) || !strings.Contains(out, `path: "path.go"`) || !strings.Contains(out, "Change path") || !strings.Contains(out, "Next: https://github.com/o/r/commits/feature/slash/path.go?page=2") {
		t.Fatalf("slash-ref history output incorrect:\n%s", out)
	}
	if atomic.LoadInt32(&resolverCalls) != 3 {
		t.Fatalf("expected three candidate resolution probes, got %d", resolverCalls)
	}
}

func TestHistoryResolverRejectsOverlappingValidRefPathSplits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("per_page") == "1" {
			_, _ = io.WriteString(w, `[{"sha":"probe"}]`)
			return
		}
		t.Fatalf("ambiguous history should not reach full list request")
	}))
	defer server.Close()
	_, err := readGitHubHistory(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/commits/feature/path.go"))
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected history ambiguity, got %v", err)
	}
}

func TestHistoryRejectsInvalidPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("per_page") == "1" {
			if r.URL.Query().Get("sha") == "main" && r.URL.Query().Get("path") == "path.go" {
				_, _ = io.WriteString(w, `[{"sha":"probe"}]`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
			return
		}
		t.Fatal("invalid page should fail before full history request")
	}))
	defer server.Close()
	_, err := readGitHubHistory(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/commits/main/path.go?page=nope"))
	if err == nil || !strings.Contains(err.Error(), "invalid GitHub commit-history page") {
		t.Fatalf("expected invalid page error, got %v", err)
	}
}

func TestBlameWithoutTokenFailsBeforeProviderRead(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		t.Fatal("no-token blame should not make provider requests")
	}))
	defer server.Close()
	_, err := readGitHubBlame(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/blame/main/a.go"))
	if err == nil || !strings.Contains(err.Error(), "requires authentication") || !strings.Contains(err.Error(), "GH_TOKEN") {
		t.Fatalf("expected concise auth-required blame result, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("no-token blame made provider calls")
	}
}

func TestAuthenticatedBlameResolvesSlashRefAndRendersRanges(t *testing.T) {
	var graphqlCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/contents/slash/a.go":
			if r.URL.Query().Get("ref") != "feature" {
				t.Fatalf("unexpected first ref %q", r.URL.Query().Get("ref"))
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		case "/repos/o/r/contents/a.go":
			if r.URL.Query().Get("ref") != "feature/slash" {
				t.Fatalf("resolved slash ref lost: %q", r.URL.Query().Get("ref"))
			}
			_, _ = io.WriteString(w, `{"type":"file","path":"a.go","sha":"blobsha"}`)
		case "/graphql":
			atomic.AddInt32(&graphqlCalls, 1)
			if r.Header.Get("Authorization") != "Bearer fake-token" {
				t.Errorf("GraphQL auth missing")
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"expression":"feature/slash"`) || !strings.Contains(string(body), `"path":"a.go"`) || !strings.Contains(string(body), "blame(path:$path)") {
				t.Errorf("blame variables/query wrong: %s", body)
			}
			_, _ = io.WriteString(w, `{"data":{"repository":{"object":{"oid":"commitsha","blame":{"ranges":[
                    {"startingLine":1,"endingLine":10,"age":1,"commit":{"oid":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","abbreviatedOid":"aaaaaaa","committedDate":"2026-08-01T00:00:00Z","messageHeadline":"Initial","url":"https://github.com/o/r/commit/a","author":{"name":"Alice","user":{"login":"alice"}}}},
                    {"startingLine":11,"endingLine":20,"age":4,"commit":{"oid":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","abbreviatedOid":"bbbbbbb","committedDate":"2026-08-02T00:00:00Z","messageHeadline":"Next","url":"https://github.com/o/r/commit/b","author":{"name":"Bob","user":null}}}
                ]}}}}}`)
		default:
			t.Fatalf("unexpected blame request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubBlame(context.Background(), testGitHubClient(server.URL, server.URL, "fake-token"), parseGitHubTarget("https://github.com/o/r/blame/feature/slash/a.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`ref: "feature/slash"`, `path: "a.go"`, `commit: "commitsha"`, "Lines 1-10", "@alice", "Initial", "Lines 11-20", "Bob", "Next"} {
		if !strings.Contains(out, want) {
			t.Fatalf("blame output missing %q:\n%s", want, out)
		}
	}
	if atomic.LoadInt32(&graphqlCalls) != 1 {
		t.Fatalf("expected one GraphQL blame call")
	}
}

func TestCommitCompareHistoryBlameFailuresStayNative(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"private provider body"}`)
	}))
	defer server.Close()
	for _, raw := range []string{
		"https://github.com/o/r/commit/abc",
		"https://github.com/o/r/compare/main...head",
		"https://github.com/o/r/commits/main/path.go",
	} {
		result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget(raw), testGitHubClient(server.URL, server.URL, ""))
		if result.Outcome != GitHubNativeFailure || result.Err == nil || !strings.Contains(result.Err.Error(), "may be private") || strings.Contains(result.Err.Error(), "private provider body") {
			t.Fatalf("native failure boundary regressed for %s: %#v", raw, result)
		}
	}
}
