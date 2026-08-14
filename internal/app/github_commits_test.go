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

func TestReadGitHubCommitOverviewStopsAfterFirstFileAndCommentPages(t *testing.T) {
	var commitPages, commentPages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/commits/ref":
			atomic.AddInt32(&commitPages, 1)
			if r.URL.Query().Get("page") == "2" {
				t.Fatal("plain commit overview followed file pagination")
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/commits/ref?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `{
                    "sha":"fullsha","html_url":"https://github.com/o/r/commit/fullsha",
                    "author":{"login":"alice"},"committer":{"login":"mergebot"},
                    "commit":{"message":"Subject\n\nBody","author":{"name":"Alice","date":"2026-08-01T00:00:00Z"},"committer":{"name":"Bot","date":"2026-08-01T01:00:00Z"},"comment_count":2,"verification":{"verified":true,"reason":"valid","verified_at":"2026-08-01T01:00:01Z"}},
                    "stats":{"total":3,"additions":2,"deletions":1},
                    "parents":[{"sha":"parentparentparent","html_url":"https://github.com/o/r/commit/parent"}],
                    "files":[{"filename":"a.go","status":"modified","additions":1,"deletions":1,"changes":2,"blob_url":"https://github.com/o/r/blob/fullsha/a.go","raw_url":"https://raw.example/a.go","patch":"@@ -1 +1 @@\n-old\n+new"}]
                }`)
		case "/repos/o/r/commits/fullsha/comments":
			atomic.AddInt32(&commentPages, 1)
			if r.URL.Query().Get("page") == "2" {
				t.Fatal("plain commit overview followed comment pagination")
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/commits/fullsha/comments?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"id":1,"body":"Visible comment\n<!-- hidden marker -->\nkept","html_url":"https://github.com/o/r/commit/fullsha#commitcomment-1","user":{"login":"reviewer"},"author_association":"MEMBER","path":"a.go","line":1,"created_at":"2026-08-02T00:00:00Z","commit_id":"fullsha"}]`)
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
		"overview: true", "files_returned: 1", "files_provider_more_available: true", "comments_reported: 2", "comments_returned: 1", "comments_provider_more_available: true",
		"Subject\n\nBody", "### `a.go`", "Selector: https://github.com/o/r/commit/fullsha#diff-", "Blob: https://github.com/o/r/blob/fullsha/a.go", "Raw: https://raw.example/a.go",
		"Comment `1` by @reviewer", "Location: `a.go` line 1", "Visible comment", "kept", "Selector: https://github.com/o/r/commit/fullsha#commitcomment-1",
		"Diff: https://github.com/o/r/commit/fullsha.diff", "Patch: https://github.com/o/r/commit/fullsha.patch",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("commit output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hidden marker") || strings.Contains(out, "```diff") || strings.Contains(out, "+new") {
		t.Fatalf("commit comment HTML marker leaked:\n%s", out)
	}
	if atomic.LoadInt32(&commitPages) != 1 || atomic.LoadInt32(&commentPages) != 1 {
		t.Fatalf("plain commit overview should fetch one file/comment page: commit=%d comments=%d", commitPages, commentPages)
	}
}

func TestRenderGitHubCommitSurfacesThreeThousandFileCap(t *testing.T) {
	detail := githubCommitDetail{SHA: "sha", HTMLURL: "https://github.com/o/r/commit/sha", Files: make([]githubPullFile, githubCommitFilesMax)}
	for i := range detail.Files {
		detail.Files[i].Filename = fmt.Sprintf("file-%04d.txt", i)
	}
	out := renderGitHubCommit(&GitHubTarget{Owner: "o", Repo: "r"}, detail, nil, githubCommitAvailability{FilesProviderMore: true})
	if !strings.Contains(out, "provider_file_ceiling: 3000") || !strings.Contains(out, "files_provider_more_available: true") || !strings.Contains(out, "files_local_omitted:") {
		t.Fatalf("commit provider cap missing")
	}
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("3,000-file commit overview exceeded shared target: %d", got)
	}
}

func TestCommitCommentSelectorUsesSingleCommentEndpointAndValidatesCommitIdentity(t *testing.T) {
	const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	longBody := "Exact commit comment\n\n" + strings.Repeat("selected comment detail stays faithful. ", 220)
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if r.URL.Path != "/repos/o/r/comments/99" {
			t.Fatalf("commit-comment selector fetched unrelated endpoint: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 99, "body": longBody + "\n<!-- hidden -->", "html_url": "https://github.com/o/r/commit/" + sha + "#commitcomment-99",
			"user": map[string]any{"login": "reviewer"}, "path": "a.go", "line": 12, "created_at": "2026-08-01T00:00:00Z", "commit_id": sha,
		})
	}))
	defer server.Close()

	out, err := readGitHubCommit(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/commit/"+sha+"#commitcomment-99"))
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("exact commit-comment selector should use one provider request, got %d", got)
	}
	for _, want := range []string{"comment_id: 99", "commit: \"" + sha + "\"", "Comment by @reviewer", "`a.go` line 12", strings.TrimSpace(longBody)} {
		if !strings.Contains(out, want) {
			t.Fatalf("selected commit comment missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hidden") || strings.Contains(out, "Changed-file index") {
		t.Fatalf("selected commit comment leaked hidden/parent context:\n%s", out)
	}

	atomic.StoreInt32(&requests, 0)
	wrong := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	_, err = readGitHubCommit(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/commit/"+wrong+"#commitcomment-99"))
	if err == nil || !strings.Contains(err.Error(), "belongs to commit") {
		t.Fatalf("mismatched commit-comment identity was not rejected: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("mismatched canonical commit-comment selector should reject after one provider request, got %d", got)
	}
}

func TestCommitDiffSelectorsTraverseProviderPagesAndSupportLineRanges(t *testing.T) {
	hash := githubDiffPathHash("b.go")
	for _, tt := range []struct {
		name      string
		fragment  string
		want      []string
		forbidden []string
	}{
		{name: "file", fragment: "diff-" + hash, want: []string{"@@ -1 +1 @@", "+new1", "@@ -10 +10 @@", "+new10"}, forbidden: []string{"a.go", "+a"}},
		{name: "line-range", fragment: "diff-" + hash + "R10-R10", want: []string{"@@ -10 +10 @@", "+new10"}, forbidden: []string{"@@ -1 +1 @@", "\n+new1\n", "a.go"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var pages int32
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&pages, 1)
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/repos/o/r/commits/ref" {
					t.Fatalf("unexpected exact commit diff endpoint: %s", r.URL.Path)
				}
				if r.URL.Query().Get("page") == "2" {
					_, _ = io.WriteString(w, `{"sha":"fullsha","html_url":"https://github.com/o/r/commit/fullsha","files":[{"filename":"b.go","status":"modified","patch":"@@ -1 +1 @@\n-old1\n+new1\n@@ -10 +10 @@\n-old10\n+new10"}]}`)
					return
				}
				w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/commits/ref?per_page=100&page=2>; rel="next"`, server.URL))
				_, _ = io.WriteString(w, `{"sha":"fullsha","html_url":"https://github.com/o/r/commit/fullsha","files":[{"filename":"a.go","status":"modified","patch":"@@ -1 +1 @@\n-a\n+b"}]}`)
			}))
			defer server.Close()
			out, err := readGitHubCommit(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/commit/ref#"+tt.fragment))
			if err != nil {
				t.Fatal(err)
			}
			if got := atomic.LoadInt32(&pages); got != 2 {
				t.Fatalf("exact commit diff selector should traverse provider pages, got %d", got)
			}
			for _, want := range append([]string{"file: \"b.go\"", "selector: \"" + tt.fragment + "\"", "Diff: https://github.com/o/r/commit/fullsha.diff", "Patch: https://github.com/o/r/commit/fullsha.patch"}, tt.want...) {
				if !strings.Contains(out, want) {
					t.Fatalf("exact commit diff missing %q:\n%s", want, out)
				}
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(out, forbidden) {
					t.Fatalf("exact commit diff leaked %q:\n%s", forbidden, out)
				}
			}
		})
	}
}

func TestLargeCommitOverviewBoundsFileAndCommentIndexesWithoutPatchBodies(t *testing.T) {
	detail := githubCommitDetail{SHA: "fullsha", HTMLURL: "https://github.com/o/r/commit/fullsha"}
	detail.Commit.Message = "Subject\n\nFull commit message body remains faithful."
	detail.Commit.CommentCount = 120
	detail.Stats.Total = 1000
	detail.Stats.Additions = 700
	detail.Stats.Deletions = 300
	for i := 0; i < 100; i++ {
		patch := fmt.Sprintf("@@ -1 +1 @@\n-old %03d\n+new %03d\nTAIL PATCH %03d MUST NOT APPEAR", i, i, i)
		detail.Files = append(detail.Files, githubPullFile{Filename: fmt.Sprintf("pkg/file-%03d.go", i), Status: "modified", Additions: 7, Deletions: 3, Changes: 10, BlobURL: fmt.Sprintf("https://github.com/o/r/blob/fullsha/pkg/file-%03d.go", i), RawURL: fmt.Sprintf("https://raw.example/pkg/file-%03d.go", i), Patch: &patch})
	}
	comments := make([]githubCommitComment, 0, 100)
	for i := 0; i < 100; i++ {
		body := fmt.Sprintf("Comment %03d marker. %s TAIL COMMENT %03d MUST NOT APPEAR", i, strings.Repeat("comment detail. ", 50), i)
		comments = append(comments, githubCommitComment{ID: int64(1000 + i), Body: &body, HTMLURL: fmt.Sprintf("https://github.com/o/r/commit/fullsha#commitcomment-%d", 1000+i), User: githubUser{Login: fmt.Sprintf("user-%03d", i)}, CommitID: "fullsha"})
	}
	out := renderGitHubCommit(&GitHubTarget{Owner: "o", Repo: "r"}, detail, comments, githubCommitAvailability{FilesProviderMore: true, CommentsProviderMore: true})
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("large commit overview exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{"Full commit message body remains faithful.", "files_local_omitted:", "comments_local_omitted:", "files_provider_more_available: true", "comments_provider_more_available: true", "Selector: https://github.com/o/r/commit/fullsha#diff-", "Selector: https://github.com/o/r/commit/fullsha#commitcomment-1000", "Diff: https://github.com/o/r/commit/fullsha.diff"} {
		if !strings.Contains(out, want) {
			t.Fatalf("large commit overview missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "TAIL PATCH 000 MUST NOT APPEAR") || strings.Contains(out, "TAIL COMMENT 000 MUST NOT APPEAR") || strings.Contains(out, "```diff") {
		t.Fatalf("large commit overview leaked subordinate bodies:\n%s", out)
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

func TestReadGitHubCompareOverviewStopsAfterFirstProviderPage(t *testing.T) {
	var requests int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if !strings.Contains(r.RequestURI, "main...feature%2Ffoo") && !strings.Contains(r.RequestURI, "main...feature/foo") {
			t.Errorf("slash-containing compare head was not preserved in endpoint: %s", r.RequestURI)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			t.Fatal("plain compare overview followed provider pagination")
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
	for _, want := range []string{
		`base: "main"`, `head: "feature/foo"`, "overview: true", "total_commits: 2", "commits_returned: 1", "commits_provider_complete: false", "commits_provider_more_available: true",
		"files_returned: 1", "files_complete: true", "First", "### `a.go`",
		"Diff: https://github.com/o/r/compare/main...feature/foo.diff", "Patch: https://github.com/o/r/compare/main...feature/foo.patch",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("compare output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Second") || strings.Contains(out, "```diff") || atomic.LoadInt32(&requests) != 1 {
		t.Fatalf("compare overview expanded provider pages/patches (requests=%d):\n%s", requests, out)
	}
}

func TestRenderGitHubCompareSurfacesThreeHundredFileCapAndCommitMismatch(t *testing.T) {
	result := githubCompareResult{Status: "ahead", TotalCommits: 5, Files: make([]githubPullFile, githubCompareFilesMax), Commits: []githubPullCommit{{SHA: "a"}}}
	for i := range result.Files {
		result.Files[i].Filename = fmt.Sprintf("file-%03d.txt", i)
	}
	out := renderGitHubCompare(&GitHubTarget{Owner: "o", Repo: "r"}, "main", "head", result, githubCompareAvailability{}, false)
	if !strings.Contains(out, "commits_provider_complete: false") || !strings.Contains(out, "files_complete: false") || !strings.Contains(out, "provider_file_ceiling: 300") || !strings.Contains(out, "up to 300 files") || !strings.Contains(out, "provider-incomplete") {
		t.Fatalf("compare completeness truth missing")
	}
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("300-file compare overview exceeded shared target: %d", got)
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
		if sha != "feature/slash" || path != "path.go" || perPage != "8" {
			t.Fatalf("resolved history query incorrect: %s", r.URL.RawQuery)
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/commits?sha=feature%%2Fslash&path=path.go&per_page=8&page=2>; rel="next"`, server.URL))
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
