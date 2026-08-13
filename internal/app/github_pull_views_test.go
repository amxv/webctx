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

func TestParseGitHubPullFocusedTargets(t *testing.T) {
	tests := []struct {
		raw  string
		kind GitHubTargetKind
	}{
		{raw: "https://github.com/o/r/pull/42/files", kind: GitHubTargetPullFiles},
		{raw: "https://github.com/o/r/pull/42/files#diff-553490f999984ba28c4af0d7ffa919d10b5419f04a73f00141ee0b5a51c142e6R24", kind: GitHubTargetPullFiles},
		{raw: "https://github.com/o/r/pull/42/commits", kind: GitHubTargetPullCommits},
		{raw: "https://github.com/o/r/pull/42/checks?check_run_id=99", kind: GitHubTargetPullChecks},
		{raw: "https://github.com/o/r/pull/42.diff", kind: GitHubTargetPullDiff},
		{raw: "https://github.com/o/r/pull/42.patch", kind: GitHubTargetPullPatch},
	}
	for _, tt := range tests {
		target := parseGitHubTarget(tt.raw)
		if target == nil || target.Kind != tt.kind || target.Number != 42 {
			t.Fatalf("focused target %s parsed as %#v", tt.raw, target)
		}
	}
}

func TestGitHubDiffPathHashMatchesObservedGitHubAnchor(t *testing.T) {
	got := githubDiffPathHash("internal/ghcmd/cmd.go")
	want := "553490f999984ba28c4af0d7ffa919d10b5419f04a73f00141ee0b5a51c142e6"
	if got != want {
		t.Fatalf("path hash mismatch: got %s want %s", got, want)
	}
}

func TestParseGitHubDiffSelector(t *testing.T) {
	hash := strings.Repeat("a", 64)
	for _, tt := range []struct {
		fragment string
		side     byte
		start    int
		end      int
		wantErr  bool
	}{
		{fragment: "diff-" + hash},
		{fragment: "diff-" + hash + "L20", side: 'L', start: 20, end: 20},
		{fragment: "diff-" + hash + "R20-R24", side: 'R', start: 20, end: 24},
		{fragment: "diff-" + hash + "L24-R25", wantErr: true},
		{fragment: "diff-" + hash + "R25-R24", wantErr: true},
		{fragment: "diff-not-a-hash", wantErr: true},
		{fragment: "discussion_r1", wantErr: true},
	} {
		selector, ok, err := parseGitHubDiffSelector(tt.fragment)
		if !ok {
			t.Fatalf("selector %q was not claimed", tt.fragment)
		}
		if tt.wantErr {
			if err == nil {
				t.Fatalf("selector %q should fail", tt.fragment)
			}
			continue
		}
		if err != nil || selector.Side != tt.side || selector.Start != tt.start || selector.End != tt.end {
			t.Fatalf("selector %q => %#v, %v", tt.fragment, selector, err)
		}
	}
}

func TestSelectGitHubPatchHunksMapsLeftAndRightLines(t *testing.T) {
	patch := "@@ -10,3 +10,4 @@ func f() {\n context\n-old\n+new\n+added\n context\n@@ -30,2 +31,2 @@\n-old2\n+new2"
	left, err := selectGitHubPatchHunks(patch, 'L', 11, 11)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(left, "-old") || strings.Contains(left, "old2") {
		t.Fatalf("left selector chose wrong hunk:\n%s", left)
	}
	right, err := selectGitHubPatchHunks(patch, 'R', 12, 12)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(right, "+added") || strings.Contains(right, "new2") {
		t.Fatalf("right selector chose wrong hunk:\n%s", right)
	}
	if _, err := selectGitHubPatchHunks(patch, 'R', 999, 999); err == nil {
		t.Fatal("expected stale line selector error")
	}
}

func TestReadGitHubPullFilesPaginatesAndRepresentsPatchOmissions(t *testing.T) {
	var pages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"title":"conversation text must not appear","changed_files":4}`)
		case "/repos/o/r/pulls/42/files":
			atomic.AddInt32(&pages, 1)
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `[
                    {"filename":"image.png","status":"modified","changes":1,"blob_url":"https://github.com/o/r/blob/x/image.png","raw_url":"https://raw.example/image.png"},
                    {"filename":"old.txt","previous_filename":"older.txt","status":"renamed","additions":0,"deletions":0,"changes":0,"patch":"@@ -1 +1 @@\n same"}
                ]`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/pulls/42/files?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[
                {"filename":"a.go","status":"modified","additions":2,"deletions":1,"changes":3,"blob_url":"https://github.com/o/r/blob/x/a.go","raw_url":"https://raw.example/a.go","patch":"@@ -10,2 +10,3 @@\n-old\n+new\n+added"},
                {"filename":"new.go","status":"added","additions":1,"deletions":0,"changes":1,"patch":"@@ -0,0 +1 @@\n+hello"}
            ]`)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubPullFiles(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42/files"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"files_returned: 4", "## a.go", "```diff", "## image.png", "Patch unavailable from GitHub", "renamed from `older.txt`", "complete: true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("files output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "conversation text must not appear") {
		t.Fatalf("files view leaked PR conversation/title body:\n%s", out)
	}
	if atomic.LoadInt32(&pages) != 2 {
		t.Fatalf("expected two file pages")
	}
}

func TestReadGitHubPullFilesDiffSelectorNarrowsFileAndHunk(t *testing.T) {
	path := "internal/ghcmd/cmd.go"
	hash := githubDiffPathHash(path)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"changed_files":2}`)
		case "/repos/o/r/pulls/42/files":
			_, _ = io.WriteString(w, `[
                    {"filename":"internal/ghcmd/cmd.go","status":"modified","patch":"@@ -20,3 +20,4 @@\n same\n-old\n+new\n+selected\n same\n@@ -100 +101 @@\n-old2\n+new2"},
                    {"filename":"other.go","status":"modified","patch":"@@ -1 +1 @@\n-x\n+y"}
                ]`)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	target := parseGitHubTarget("https://github.com/o/r/pull/42/files#diff-" + hash + "R22")
	out, err := readGitHubPullFiles(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "## "+path) || !strings.Contains(out, "Selected R22") || !strings.Contains(out, "+selected") || strings.Contains(out, "old2") || strings.Contains(out, "other.go") {
		t.Fatalf("diff selector failed to narrow output:\n%s", out)
	}
}

func TestPullFilesSelectorRejectsStaleHashLineAndUnavailablePatch(t *testing.T) {
	path := "a.go"
	hash := githubDiffPathHash(path)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"changed_files":2}`)
		case "/repos/o/r/pulls/42/files":
			_, _ = io.WriteString(w, `[
                    {"filename":"a.go","patch":"@@ -1 +1 @@\n-old\n+new"},
                    {"filename":"binary.bin"}
                ]`)
		}
	}))
	defer server.Close()
	client := testGitHubClient(server.URL, server.URL, "")
	for _, tt := range []struct {
		fragment string
		want     string
	}{
		{fragment: "diff-" + strings.Repeat("f", 64), want: "does not match"},
		{fragment: "diff-" + hash + "R999", want: "stale or out of range"},
		{fragment: "diff-" + githubDiffPathHash("binary.bin") + "R1", want: "patch is unavailable"},
	} {
		_, err := readGitHubPullFiles(context.Background(), client, parseGitHubTarget("https://github.com/o/r/pull/42/files#"+tt.fragment))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("selector %q error=%v want %q", tt.fragment, err, tt.want)
		}
	}
}

func TestPullFilesSurfacesThreeThousandFileProviderCeiling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/pulls/42") {
			_, _ = io.WriteString(w, `{"number":42,"changed_files":3001}`)
			return
		}
		_, _ = io.WriteString(w, `[{"filename":"a.go","patch":"@@ -1 +1 @@\n-a\n+b"}]`)
	}))
	defer server.Close()
	out, err := readGitHubPullFiles(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42/files"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "complete: false") || !strings.Contains(out, "3,000-file maximum") {
		t.Fatalf("provider cap truth missing:\n%s", out)
	}
}

func TestReadGitHubPullCommitsIsCommitOnlyAndPaginated(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"body":"PR BODY MUST NOT RENDER","commits":2}`)
		case "/repos/o/r/pulls/42/commits":
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `[{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","html_url":"https://github.com/o/r/commit/b","commit":{"message":"Second\nbody","author":{"name":"Bob","date":"2026-08-02T00:00:00Z"}}}]`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/pulls/42/commits?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","html_url":"https://github.com/o/r/commit/a","author":{"login":"alice"},"commit":{"message":"First","author":{"date":"2026-08-01T00:00:00Z"}}}]`)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubPullCommits(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42/commits"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "commits_returned: 2") || !strings.Contains(out, "First") || !strings.Contains(out, "Second") || strings.Contains(out, "body\n") || strings.Contains(out, "PR BODY MUST NOT RENDER") || strings.Contains(out, "```diff") {
		t.Fatalf("commit-only view incorrect:\n%s", out)
	}
}

func TestReadGitHubPullChecksSeparatesRunsAndStatuses(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"head":{"sha":"headsha"}}`)
		case "/repos/o/r/commits/headsha/check-runs":
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `{"total_count":2,"check_runs":[{"id":2,"name":"lint","head_sha":"headsha","status":"completed","conclusion":"failure","output":{"annotations_count":2}}]}`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/commits/headsha/check-runs?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `{"total_count":2,"check_runs":[{"id":1,"name":"build","head_sha":"headsha","status":"completed","conclusion":"success","details_url":"https://ci/build","app":{"slug":"actions"},"output":{"title":"Build","summary":"All good","annotations_count":0}}]}`)
		case "/repos/o/r/commits/headsha/status":
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `{"state":"pending","total_count":2,"statuses":[{"id":8,"state":"success","context":"legacy-ci","description":"Passed"}]}`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/commits/headsha/status?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `{"state":"pending","total_count":2,"statuses":[{"id":7,"state":"pending","context":"deploy","description":"Waiting","target_url":"https://ci/deploy"}]}`)
		default:
			t.Fatalf("unexpected checks request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubPullChecks(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42/checks"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Check runs", "### build", "conclusion success", "### lint", "conclusion failure", "Focused check: https://github.com/o/r/pull/42/checks?check_run_id=2", "commit_statuses: 2", "## Commit statuses", "Combined status state: `pending`", "**deploy** — pending", "**legacy-ci** — success", "does not infer a branch-protection/merge decision"} {
		if !strings.Contains(out, want) {
			t.Fatalf("checks output missing %q:\n%s", want, out)
		}
	}
}

func TestReadGitHubSelectedCheckFetchesOnlySelectedRunAndAllAnnotations(t *testing.T) {
	var annotationsPages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"head":{"sha":"headsha"}}`)
		case "/repos/o/r/check-runs/9":
			_, _ = io.WriteString(w, `{"id":9,"name":"selected","head_sha":"headsha","status":"completed","conclusion":"failure","output":{"title":"Selected check","summary":"Summary","annotations_count":2}}`)
		case "/repos/o/r/check-runs/9/annotations":
			atomic.AddInt32(&annotationsPages, 1)
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `[{"path":"b.go","start_line":20,"end_line":22,"annotation_level":"warning","title":"B","message":"Second"}]`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/check-runs/9/annotations?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"path":"a.go","start_line":10,"end_line":10,"annotation_level":"failure","title":"A","message":"First","raw_details":"detail"}]`)
		case "/repos/o/r/commits/headsha/check-runs", "/repos/o/r/commits/headsha/status":
			t.Fatalf("focused check must not dump all checks/statuses: %s", r.URL.Path)
		default:
			t.Fatalf("unexpected selected-check request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubPullChecks(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42/checks?check_run_id=9"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "check_run_id: 9") || !strings.Contains(out, "annotations_returned: 2") || !strings.Contains(out, "`a.go:10`") || !strings.Contains(out, "`b.go:20-22`") || strings.Contains(out, "Focused check: ") {
		t.Fatalf("focused check output incorrect:\n%s", out)
	}
	if atomic.LoadInt32(&annotationsPages) != 2 {
		t.Fatalf("expected annotation pagination")
	}
}

func TestSelectedCheckRejectsRunFromDifferentHead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/pulls/42") {
			_, _ = io.WriteString(w, `{"number":42,"head":{"sha":"expected"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":9,"name":"wrong","head_sha":"other"}`)
	}))
	defer server.Close()
	_, err := readGitHubPullChecks(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42/checks?check_run_id=9"))
	if err == nil || !strings.Contains(err.Error(), "not Pull Request head") {
		t.Fatalf("expected check ownership error, got %v", err)
	}
}

func TestRawPullDiffAndPatchPreserveProviderBodyAndMediaType(t *testing.T) {
	for _, tt := range []struct {
		kind   GitHubTargetKind
		patch  bool
		accept string
		body   string
	}{
		{kind: GitHubTargetPullDiff, accept: "application/vnd.github.v3.diff", body: "diff --git a/a b/a\n-old\n+new\n"},
		{kind: GitHubTargetPullPatch, patch: true, accept: "application/vnd.github.v3.patch", body: "From abc Mon Sep 17 00:00:00 2001\nSubject: patch\n\nbody\n"},
	} {
		t.Run(string(tt.kind), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept") != tt.accept {
					t.Errorf("Accept=%q want %q", r.Header.Get("Accept"), tt.accept)
				}
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			target := &GitHubTarget{Owner: "o", Repo: "r", Number: 42, Kind: tt.kind}
			got, err := readGitHubPullRawDiff(context.Background(), testGitHubClient(server.URL, server.URL, ""), target, tt.patch)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.body {
				t.Fatalf("raw media changed: got %q want %q", got, tt.body)
			}
		})
	}
}

func TestPullFocusedProviderFailureStaysNative(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"private body should not print"}`)
	}))
	defer server.Close()
	for _, raw := range []string{
		"https://github.com/o/r/pull/42/files",
		"https://github.com/o/r/pull/42/commits",
		"https://github.com/o/r/pull/42/checks",
		"https://github.com/o/r/pull/42.diff",
	} {
		result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget(raw), testGitHubClient(server.URL, server.URL, ""))
		if result.Outcome != GitHubNativeFailure || result.Err == nil || !strings.Contains(result.Err.Error(), "may be private") || strings.Contains(result.Err.Error(), "private body") {
			t.Fatalf("focused view %s did not preserve native failure: %#v", raw, result)
		}
	}
}
