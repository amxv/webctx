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
	"unicode/utf8"
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

func TestReadGitHubPullFilesOverviewStopsAfterFirstPageAndRepresentsProviderMore(t *testing.T) {
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
				t.Fatal("bounded Files overview followed provider pagination")
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
	for _, want := range []string{"overview: true", "files_returned: 2", "files_indexed: 2", "provider_more_available: true", "### `a.go`", "```diff", "Selector: https://github.com/o/r/pull/42/files#diff-", "complete: false"} {
		if !strings.Contains(out, want) {
			t.Fatalf("files output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "conversation text must not appear") || strings.Contains(out, "image.png") || strings.Contains(out, "old.txt") {
		t.Fatalf("files view leaked PR conversation/title body:\n%s", out)
	}
	if atomic.LoadInt32(&pages) != 1 {
		t.Fatalf("Files overview should fetch one provider page")
	}
}

func TestReadGitHubPullFilesSmallCompleteSetKeepsFullPatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"changed_files":2}`)
		case "/repos/o/r/pulls/42/files":
			_, _ = io.WriteString(w, `[
{"filename":"a.go","status":"modified","additions":2,"deletions":1,"changes":3,"patch":"@@ -1 +1 @@\n-old\n+new"},
{"filename":"image.png","status":"modified","changes":1,"blob_url":"https://github.com/o/r/blob/x/image.png","raw_url":"https://raw.example/image.png"}
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
	for _, want := range []string{"files_returned: 2", "files_rendered: 2", "complete: true", "## a.go", "```diff", "-old", "+new", "## image.png", "Patch unavailable from GitHub"} {
		if !strings.Contains(out, want) {
			t.Fatalf("small complete Files view missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "overview: true") {
		t.Fatalf("small complete Files view should retain full patch presentation:\n%s", out)
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

func TestLargePullFilesOverviewBoundsHundredsAndProviderCeilingSeparately(t *testing.T) {
	t.Run("provider has more after first page", func(t *testing.T) {
		files := make([]githubPullFile, 0, 100)
		for i := 0; i < 100; i++ {
			patch := fmt.Sprintf("@@ -1 +1 @@\n-old %03d\n+new %03d\n%s\nTAIL PATCH %03d MUST NOT APPEAR", i, i, strings.Repeat("+generated detail\n", 30), i)
			files = append(files, githubPullFile{
				Filename: fmt.Sprintf("pkg/file-%03d.go", i), Status: "modified", Additions: 31, Deletions: 1, Changes: 32,
				BlobURL: fmt.Sprintf("https://github.com/o/r/blob/head/pkg/file-%03d.go", i), RawURL: fmt.Sprintf("https://raw.example/pkg/file-%03d.go", i), Patch: &patch,
			})
		}
		out := renderGitHubPullFiles(&GitHubTarget{Owner: "o", Repo: "r", Number: 42}, githubPullRequest{Number: 42, ChangedFiles: 150}, files, githubPullFilesAvailability{
			ProviderReturned: 100, ProviderMore: true, ProviderComplete: false,
		}, githubDiffSelector{}, false)
		if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
			t.Fatalf("100-file overview exceeded shared target: %d runes\n%s", got, out)
		}
		for _, want := range []string{"overview: true", "changed_files: 150", "files_returned: 100", "provider_more_available: true", "files_local_omitted:", "locally omitted from this overview", "Selector: https://github.com/o/r/pull/42/files#diff-", "Blob: https://github.com/o/r/blob/head/pkg/file-000.go", "Raw: https://raw.example/pkg/file-000.go"} {
			if !strings.Contains(out, want) {
				t.Fatalf("100-file overview missing %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "TAIL PATCH 000 MUST NOT APPEAR") {
			t.Fatalf("large Files overview leaked full patch:\n%s", out)
		}
	})

	t.Run("three thousand provider ceiling remains distinct from local omission", func(t *testing.T) {
		files := make([]githubPullFile, 3000)
		for i := range files {
			files[i] = githubPullFile{Filename: fmt.Sprintf("generated/file-%04d.txt", i), Status: "modified", Changes: 1}
		}
		out := renderGitHubPullFiles(&GitHubTarget{Owner: "o", Repo: "r", Number: 42}, githubPullRequest{Number: 42, ChangedFiles: 3001}, files, githubPullFilesAvailability{
			ProviderReturned: 3000, ProviderComplete: false, ProviderCapReached: true,
		}, githubDiffSelector{}, false)
		if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
			t.Fatalf("3,000-file overview exceeded shared target: %d runes\n%s", got, out)
		}
		for _, want := range []string{"files_returned: 3000", "provider_result_ceiling: 3000", "3,000-file maximum", "files_local_omitted:", "locally omitted from this overview"} {
			if !strings.Contains(out, want) {
				t.Fatalf("provider ceiling/local omission truth missing %q:\n%s", want, out)
			}
		}
	})
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
	for _, want := range []string{"## Rollup", "Check run conclusions: failure 1, success 1", "## Check run index", "### build", "conclusion success", "### lint", "conclusion failure", "Focused check: https://github.com/o/r/pull/42/checks?check_run_id=1", "Focused check: https://github.com/o/r/pull/42/checks?check_run_id=2", "commit_statuses_returned: 2", "## Commit statuses", "Combined status state: `pending`", "**deploy** — pending", "**legacy-ci** — success", "does not infer a branch-protection/merge decision"} {
		if !strings.Contains(out, want) {
			t.Fatalf("checks output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "### lint") > strings.Index(out, "### build") {
		t.Fatalf("failed check must be prioritized ahead of routine success:\n%s", out)
	}
	if strings.Contains(out, "All good") || strings.Contains(out, "**Build**") {
		t.Fatalf("checks container embedded per-run output summary/title:\n%s", out)
	}
}

func TestLargePullChecksOverviewPrioritizesActionableRunsAndBoundsMachineText(t *testing.T) {
	runs := make([]githubCheckRun, 0, 130)
	for i := 0; i < 128; i++ {
		run := githubCheckRun{ID: int64(i + 1), Name: fmt.Sprintf("success-%03d", i), Status: "completed", Conclusion: "success", DetailsURL: fmt.Sprintf("https://ci.example/success/%d", i)}
		run.Output.Summary = "MACHINE SUMMARY MUST NOT APPEAR " + strings.Repeat("generated success detail ", 80)
		runs = append(runs, run)
	}
	failure := githubCheckRun{ID: 9001, Name: "zzz-hard-failure", Status: "completed", Conclusion: "failure", DetailsURL: "https://ci.example/failure"}
	failure.Output.AnnotationsCount = 3
	failure.Output.Summary = "FAILURE SUMMARY MUST NOT APPEAR " + strings.Repeat("generated failure detail ", 100)
	active := githubCheckRun{ID: 9002, Name: "yyy-active", Status: "in_progress", DetailsURL: "https://ci.example/active"}
	active.Output.Summary = "ACTIVE SUMMARY MUST NOT APPEAR"
	runs = append(runs, active, failure)
	status := githubCombinedStatus{State: "pending", TotalCount: 3, Statuses: []githubCommitStatus{
		{ID: 1, State: "success", Context: "legacy-success", Description: "done"},
		{ID: 2, State: "pending", Context: "legacy-pending", Description: "waiting"},
		{ID: 3, State: "failure", Context: "legacy-failure", Description: "failed"},
	}}
	out := renderGitHubPullChecks(&GitHubTarget{Owner: "o", Repo: "r", Number: 42}, githubPullRequest{Number: 42, Head: githubPullRef{SHA: "headsha"}}, runs, len(runs), status)
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("large Checks overview exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{
		"check_runs_returned: 130", `check_run_status_counts: {"completed":129,"in_progress":1}`, `check_run_conclusion_counts: {"failure":1,"none":1,"success":128}`,
		"check_runs_local_omitted:", "locally omitted from this overview", "zzz-hard-failure", "yyy-active", "success-000",
		"Focused check: https://github.com/o/r/pull/42/checks?check_run_id=9001", "Focused check: https://github.com/o/r/pull/42/checks?check_run_id=9002",
		"**legacy-failure** — failure", "**legacy-pending** — pending", "**legacy-success** — success",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("large Checks overview missing %q:\n%s", want, out)
		}
	}
	failureAt := strings.Index(out, "### zzz-hard-failure")
	activeAt := strings.Index(out, "### yyy-active")
	successAt := strings.Index(out, "### success-000")
	if failureAt < 0 || activeAt < 0 || successAt < 0 || !(failureAt < activeAt && activeAt < successAt) {
		t.Fatalf("check priority is not hard-failure -> active -> routine success:\n%s", out)
	}
	if strings.Contains(out, "MACHINE SUMMARY MUST NOT APPEAR") || strings.Contains(out, "FAILURE SUMMARY MUST NOT APPEAR") || strings.Contains(out, "ACTIVE SUMMARY MUST NOT APPEAR") {
		t.Fatalf("Checks container embedded machine-generated output summaries:\n%s", out)
	}
	indexed := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "check_runs_indexed: ") {
			_, _ = fmt.Sscanf(line, "check_runs_indexed: %d", &indexed)
		}
	}
	if indexed <= 0 || strings.Count(out, "Focused check: https://github.com/o/r/pull/42/checks?check_run_id=") != indexed {
		t.Fatalf("every indexed check must expose a focused URL (indexed=%d):\n%s", indexed, out)
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

func TestOversizedFocusedCheckBoundsSummaryRawDetailsAndAnnotationCollection(t *testing.T) {
	run := githubCheckRun{ID: 9, Name: "selected", HeadSHA: "headsha", Status: "completed", Conclusion: "failure", DetailsURL: "https://ci.example/check/9"}
	run.Output.Title = "Selected machine check"
	run.Output.AnnotationsCount = 50
	run.Output.Summary = "SUMMARY START\n\n" + strings.Repeat("generated summary detail. ", 300) + "\nSUMMARY TAIL MUST NOT APPEAR"
	annotations := make([]githubCheckAnnotation, 0, 40)
	for i := 0; i < 40; i++ {
		annotations = append(annotations, githubCheckAnnotation{
			Path: fmt.Sprintf("pkg/file-%02d.go", i), StartLine: i + 10, EndLine: i + 10, AnnotationLevel: "failure", Title: fmt.Sprintf("Failure %02d", i),
			Message:    fmt.Sprintf("MESSAGE %02d START %s MESSAGE %02d TAIL MUST NOT APPEAR", i, strings.Repeat("generated message detail. ", 80), i),
			RawDetails: fmt.Sprintf("RAW %02d START %s RAW %02d TAIL MUST NOT APPEAR", i, strings.Repeat("generated raw detail. ", 80), i),
		})
	}
	out := renderGitHubSelectedCheck(&GitHubTarget{Owner: "o", Repo: "r", Number: 42}, githubPullRequest{Number: 42, Head: githubPullRef{SHA: "headsha"}}, run, annotations)
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("focused check exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{
		"check_run_id: 9", "annotations_reported: 50", "annotations_returned: 40", "annotations_local_omitted:", "SUMMARY START", "Check summary preview locally truncated",
		"`pkg/file-00.go:10`", "MESSAGE 00 START", "Raw details preview locally truncated", "locally omitted from this overview", "provider-incomplete state", "Check details: https://ci.example/check/9",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("focused check bounded output missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"SUMMARY TAIL MUST NOT APPEAR", "MESSAGE 00 TAIL MUST NOT APPEAR", "RAW 00 TAIL MUST NOT APPEAR", "MESSAGE 39 START"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("focused check leaked oversized machine text %q:\n%s", forbidden, out)
		}
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
