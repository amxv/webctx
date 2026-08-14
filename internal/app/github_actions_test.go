package app

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

func TestParseGitHubActionsTargets(t *testing.T) {
	tests := []struct {
		raw      string
		kind     GitHubTargetKind
		name     string
		runID    int64
		jobID    int64
		fragment string
	}{
		{raw: "https://github.com/o/r/actions", kind: GitHubTargetActions},
		{raw: "https://github.com/o/r/actions/workflows", kind: GitHubTargetWorkflows},
		{raw: "https://github.com/o/r/actions/workflows/ci.yml", kind: GitHubTargetWorkflow, name: "ci.yml"},
		{raw: "https://github.com/o/r/actions/runs/123", kind: GitHubTargetActionsRun, runID: 123},
		{raw: "https://github.com/o/r/actions/runs/123/job/456", kind: GitHubTargetActionsJob, runID: 123, jobID: 456},
		{raw: "https://github.com/o/r/actions/runs/123/job/456#step:1:2", kind: GitHubTargetActionsJob, runID: 123, jobID: 456, fragment: "step:1:2"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			target := parseGitHubTarget(tt.raw)
			if target == nil || target.Kind != tt.kind || target.Name != tt.name || target.RunID != tt.runID || target.JobID != tt.jobID || target.Fragment != tt.fragment {
				t.Fatalf("unexpected target: %#v", target)
			}
		})
	}
	for _, raw := range []string{
		"https://github.com/o/r/actions/runs/nope",
		"https://github.com/o/r/actions/runs/123/jobs/456",
		"https://github.com/o/r/actions/runs/123/job/nope",
		"https://github.com/o/r/actions/unknown",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("unsupported Actions URL %s parsed as %#v", raw, target)
		}
	}
}

func TestActionsOverviewIsBoundedAndPreservesRunFilters(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/actions/workflows":
			if r.URL.Query().Get("page") != "" {
				t.Errorf("Actions run page leaked into workflow sidebar query: %s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("per_page") != "4" {
				t.Errorf("Actions root workflow slice=%s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"total_count":1,"workflows":[{"id":10,"name":"CI","path":".github/workflows/ci.yml","state":"active","html_url":"https://github.com/o/r/blob/main/.github/workflows/ci.yml"}]}`)
		case "/repos/o/r/actions/runs":
			for key, want := range map[string]string{"branch": "main", "status": "failure", "page": "2", "per_page": "8"} {
				if got := r.URL.Query().Get(key); got != want {
					t.Errorf("run query %s=%q want %q", key, got, want)
				}
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/actions/runs?branch=main&status=failure&per_page=8&page=1>; rel="prev", <%s/repos/o/r/actions/runs?branch=main&status=failure&per_page=8&page=3>; rel="next"`, server.URL, server.URL))
			_, _ = io.WriteString(w, `{"total_count":50,"workflow_runs":[{"id":100,"name":"CI","display_title":"Failing build","status":"completed","conclusion":"failure","event":"push","head_branch":"main","html_url":"https://github.com/o/r/actions/runs/100"}]}`)
		default:
			t.Fatalf("unexpected Actions overview request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	target := parseGitHubTarget("https://github.com/o/r/actions?branch=main&status=failure&page=2")
	out, err := readGitHubActionsOverview(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"workflows_reported: 1", "runs_reported: 50", "runs_provider_more_available: true", "[CI](https://github.com/o/r/actions/workflows/ci.yml)", "source https://github.com/o/r/blob/main/.github/workflows/ci.yml", "[Failing build](https://github.com/o/r/actions/runs/100)", "failure", "Previous", "page=1", "Next", "page=3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Actions overview missing %q:\n%s", want, out)
		}
	}
}

func TestLargeActionsOverviewAndWorkflowStayWithinSharedBudget(t *testing.T) {
	target := &GitHubTarget{Owner: "o", Repo: "r", Query: url.Values{"page": []string{"2"}}}
	workflows := make([]githubWorkflow, 0, 4)
	runs := make([]githubActionsRun, 0, githubPageableListSize)
	for i := 0; i < githubPageableListSize; i++ {
		if i < 4 {
			workflows = append(workflows, githubWorkflow{
				ID: int64(100 + i), Name: fmt.Sprintf("Workflow %02d %s", i, strings.Repeat("descriptive-name-", 15)),
				Path: fmt.Sprintf(".github/workflows/workflow-%02d.yml", i), State: "active",
				HTMLURL: fmt.Sprintf("https://github.com/o/r/blob/main/.github/workflows/workflow-%02d.yml", i),
			})
		}
		runs = append(runs, githubActionsRun{
			ID: int64(1000 + i), DisplayTitle: fmt.Sprintf("Run %02d %s", i, strings.Repeat("generated-title-", 18)),
			Status: "completed", Conclusion: "success", Event: "push", HeadBranch: "main",
			HTMLURL: fmt.Sprintf("https://github.com/o/r/actions/runs/%d", 1000+i),
		})
	}
	overview := renderGitHubActionsOverview(target, workflows, 30, runs, 300, nil)
	if got := utf8.RuneCountInString(overview); got > githubOverviewRunes {
		t.Fatalf("Actions root exceeded shared target: %d runes\n%s", got, overview)
	}
	for _, want := range []string{"All workflows: https://github.com/o/r/actions/workflows", "workflows_local_omitted: 0", "runs_local_omitted: 0", "workflows_provider_more_available: true", "runs_provider_more_available: true", "https://github.com/o/r/actions/workflows/workflow-00.yml"} {
		if !strings.Contains(overview, want) {
			t.Fatalf("Actions root missing %q:\n%s", want, overview)
		}
	}

	workflow := renderGitHubWorkflow(target, workflows[0], runs, 300, nil)
	if got := utf8.RuneCountInString(workflow); got > githubOverviewRunes {
		t.Fatalf("workflow run list exceeded shared target: %d runes\n%s", got, workflow)
	}
	if !strings.Contains(workflow, "runs_local_omitted: 0") || !strings.Contains(workflow, "url: \"https://github.com/o/r/actions/workflows/workflow-00.yml\"") || !strings.Contains(workflow, "source_url: \"https://github.com/o/r/blob/main/.github/workflows/workflow-00.yml\"") {
		t.Fatalf("workflow page did not preserve complete provider page/native navigation:\n%s", workflow)
	}
}

func TestActionsOverviewRejectsUnmappedUIQueryFilter(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer server.Close()
	_, err := readGitHubActionsOverview(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/actions?query=branch%3Amain"))
	if err == nil || !strings.Contains(err.Error(), "not yet a supported native filter") {
		t.Fatalf("expected truthful unsupported filter error, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("unsupported UI query made provider calls")
	}
}

func TestWorkflowDetailAndRunsStayBounded(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/actions/workflows/ci.yml":
			_, _ = io.WriteString(w, `{"id":10,"name":"CI","path":".github/workflows/ci.yml","state":"active","html_url":"https://github.com/o/r/blob/main/.github/workflows/ci.yml"}`)
		case "/repos/o/r/actions/workflows/ci.yml/runs":
			if r.URL.Query().Get("event") != "push" || r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "8" {
				t.Errorf("workflow run filters lost: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"total_count":1,"workflow_runs":[{"id":100,"display_title":"Push run","status":"completed","conclusion":"success","html_url":"https://github.com/o/r/actions/runs/100"}]}`)
		default:
			t.Fatalf("unexpected workflow request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubWorkflow(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/actions/workflows/ci.yml?event=push&page=2"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "workflow_id: 10") || !strings.Contains(out, `url: "https://github.com/o/r/actions/workflows/ci.yml"`) || !strings.Contains(out, `source_url: "https://github.com/o/r/blob/main/.github/workflows/ci.yml"`) || !strings.Contains(out, "runs_local_omitted: 0") || !strings.Contains(out, "[Push run](https://github.com/o/r/actions/runs/100)") || !strings.Contains(out, "success") {
		t.Fatalf("workflow output incorrect:\n%s", out)
	}
}

func TestWorkflowRejectsUnmappedUIQueryFilter(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer server.Close()
	_, err := readGitHubWorkflow(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/actions/workflows/ci.yml?query=event%3Apush"))
	if err == nil || !strings.Contains(err.Error(), "not yet a supported native filter") {
		t.Fatalf("expected truthful unsupported workflow filter error, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("unsupported workflow query made provider calls")
	}
}

func TestActionsRunPaginatesJobsButBoundsArtifactsWithoutLogs(t *testing.T) {
	var jobPages, artifactPages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/actions/runs/100":
			_, _ = io.WriteString(w, `{"id":100,"name":"CI","display_title":"Build main","run_number":7,"run_attempt":2,"event":"push","status":"completed","conclusion":"failure","head_branch":"main","head_sha":"abc","actor":{"login":"alice"},"run_started_at":"2026-08-01T00:00:00Z","html_url":"https://github.com/o/r/actions/runs/100"}`)
		case "/repos/o/r/actions/runs/100/jobs":
			atomic.AddInt32(&jobPages, 1)
			if r.URL.Query().Get("filter") != "latest" {
				t.Errorf("jobs must use latest attempt filter: %s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `{"total_count":2,"jobs":[{"id":202,"run_id":100,"name":"test","status":"completed","conclusion":"failure","html_url":"https://github.com/o/r/actions/runs/100/job/202"}]}`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/actions/runs/100/jobs?filter=latest&per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `{"total_count":2,"jobs":[{"id":201,"run_id":100,"name":"build","status":"completed","conclusion":"success","html_url":"https://github.com/o/r/actions/runs/100/job/201"}]}`)
		case "/repos/o/r/actions/runs/100/artifacts":
			atomic.AddInt32(&artifactPages, 1)
			if r.URL.Query().Get("page") == "2" {
				t.Fatal("Actions run overview followed artifact pagination")
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/actions/runs/100/artifacts?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `{"total_count":2,"artifacts":[{"id":301,"name":"report","size_in_bytes":10,"expired":false,"expires_at":"2026-09-01T00:00:00Z","archive_download_url":"https://api.github.test/artifacts/301/zip"}]}`)
		case "/repos/o/r/actions/jobs/201/logs", "/repos/o/r/actions/jobs/202/logs":
			t.Fatal("run page must not fetch job logs")
		default:
			t.Fatalf("unexpected run request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubActionsRun(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/actions/runs/100"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"run_id: 100", "run_number: 7", "attempt: 2", "conclusion: \"failure\"",
		"jobs_returned: 2", "jobs_reported: 2", "job_conclusion_counts: {\"failure\":1,\"success\":1}",
		"[test](https://github.com/o/r/actions/runs/100/job/202)", "[build](https://github.com/o/r/actions/runs/100/job/201)",
		"artifacts_returned: 1", "artifacts_reported: 2", "artifacts_provider_more_available: true",
		"**report** — id 301 · 10 bytes", "expires 2026-09-01T00:00:00Z", "archive API: https://api.github.test/artifacts/301/zip",
		"GitHub has more artifacts beyond the provider page fetched for this overview",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("run output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "[test]") > strings.Index(out, "[build]") {
		t.Fatalf("failed job was not prioritized ahead of success:\n%s", out)
	}
	if strings.Contains(out, "expired-report") {
		t.Fatalf("run overview rendered unfetched artifact page:\n%s", out)
	}
	if atomic.LoadInt32(&jobPages) != 2 || atomic.LoadInt32(&artifactPages) != 1 {
		t.Fatalf("run pagination incomplete jobs=%d artifacts=%d", jobPages, artifactPages)
	}
}

func TestLargeActionsRunPrioritizesJobsAndBoundsArtifacts(t *testing.T) {
	jobs := make([]githubActionsJob, 0, 130)
	for i := 0; i < 128; i++ {
		jobs = append(jobs, githubActionsJob{
			ID: int64(i + 1), RunID: 100, Name: fmt.Sprintf("success-%03d-%s", i, strings.Repeat("routine-", 10)),
			Status: "completed", Conclusion: "success", HTMLURL: fmt.Sprintf("https://github.com/o/r/actions/runs/100/job/%d", i+1),
		})
	}
	jobs = append(jobs,
		githubActionsJob{ID: 9002, RunID: 100, Name: "yyy-active", Status: "in_progress", HTMLURL: "https://github.com/o/r/actions/runs/100/job/9002"},
		githubActionsJob{ID: 9001, RunID: 100, Name: "zzz-hard-failure", Status: "completed", Conclusion: "failure", HTMLURL: "https://github.com/o/r/actions/runs/100/job/9001"},
	)
	artifacts := make([]githubActionsArtifact, 0, 100)
	for i := 0; i < 100; i++ {
		artifacts = append(artifacts, githubActionsArtifact{
			ID: int64(5000 + i), Name: fmt.Sprintf("artifact-%03d-%s", i, strings.Repeat("generated-", 8)), SizeInBytes: int64(100 + i),
			ExpiresAt: "2026-09-01T00:00:00Z", ArchiveDownloadURL: fmt.Sprintf("https://api.github.com/repos/o/r/actions/artifacts/%d/zip", 5000+i),
		})
	}
	artifacts[0].Expired = true
	out := renderGitHubActionsRun(&GitHubTarget{Owner: "o", Repo: "r", RunID: 100}, githubActionsRun{
		ID: 100, RunNumber: 7, RunAttempt: 1, Status: "completed", Conclusion: "failure", DisplayTitle: "Large run",
	}, jobs, artifacts, githubActionsRunAvailability{JobsReported: 130, ArtifactsReported: 133, ArtifactsProviderMore: true})
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("large Actions run exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{
		"jobs_returned: 130", `job_status_counts: {"completed":129,"in_progress":1}`, `job_conclusion_counts: {"failure":1,"none":1,"success":128}`,
		"jobs_local_omitted:", "zzz-hard-failure", "yyy-active", "success-000-", "id 9001", "id 9002",
		"artifacts_returned: 100", "artifacts_reported: 133", "artifacts_provider_more_available: true", "artifacts_local_omitted:", "archive API:",
		"locally omitted from this overview", "GitHub has more artifacts beyond the provider page fetched for this overview",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("large Actions run missing %q:\n%s", want, out)
		}
	}
	failureAt := strings.Index(out, "zzz-hard-failure")
	activeAt := strings.Index(out, "yyy-active")
	successAt := strings.Index(out, "success-000-")
	if failureAt < 0 || activeAt < 0 || successAt < 0 || !(failureAt < activeAt && activeAt < successAt) {
		t.Fatalf("job priority is not failure -> active -> routine success:\n%s", out)
	}
}

func TestActionsJobFetchesOnlySelectedJobAndPlainLog(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		switch r.URL.Path {
		case "/repos/o/r/actions/jobs/202":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":202,"run_id":100,"run_attempt":2,"name":"test","status":"completed","conclusion":"failure","head_sha":"abc","html_url":"https://github.com/o/r/actions/runs/100/job/202","steps":[{"number":1,"name":"Checkout","status":"completed","conclusion":"success"},{"number":2,"name":"Test","status":"completed","conclusion":"failure"}]}`)
		case "/repos/o/r/actions/jobs/202/logs":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, "line one\nline two\n")
		default:
			t.Fatalf("selected job fetched unrelated endpoint %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubActionsJob(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/actions/runs/100/job/202"))
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&requests) != 2 || !strings.Contains(out, "job_id: 202") || !strings.Contains(out, "1. **Checkout** — completed · success") || !strings.Contains(out, "2. **Test** — completed · failure") || !strings.Contains(out, "line one\nline two") || !strings.Contains(out, "log_preview_strategy: \"full\"") || !strings.Contains(out, "Stable job log endpoint: https://api.github.com/repos/o/r/actions/jobs/202/logs") || strings.Contains(out, "job/201") {
		t.Fatalf("job output/scoping incorrect:\n%s", out)
	}
}

func TestActionsJobLargeFailureLogIsBoundedAroundFailedStepAndRawNavigation(t *testing.T) {
	logLines := make([]string, 0, 2400)
	for i := 0; i < 1700; i++ {
		logLines = append(logLines, fmt.Sprintf("setup line %04d %s", i, strings.Repeat("routine ", 8)))
	}
	logLines = append(logLines, "2026-08-14T00:00:00Z Integration tests", "2026-08-14T00:00:01Z ##[error]expected 200 got 500", "2026-08-14T00:00:02Z Process completed with exit code 1")
	for i := 0; i < 650; i++ {
		logLines = append(logLines, fmt.Sprintf("cleanup line %04d %s", i, strings.Repeat("routine ", 8)))
	}
	logLines = append(logLines, "TERMINAL TAIL MARKER")
	largeLog := strings.Join(logLines, "\n") + "\n"
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		switch r.URL.Path {
		case "/repos/o/r/actions/jobs/202":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":202,"run_id":100,"name":"integration","status":"completed","conclusion":"failure","html_url":"https://github.com/o/r/actions/runs/100/job/202","steps":[{"number":1,"name":"Checkout","status":"completed","conclusion":"success"},{"number":2,"name":"Integration tests","status":"completed","conclusion":"failure"}]}`)
		case "/repos/o/r/actions/jobs/202/logs":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, largeLog)
		default:
			t.Fatalf("large selected job fetched unrelated endpoint %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubActionsJob(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/actions/runs/100/job/202"))
	if err != nil {
		t.Fatal(err)
	}
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("large selected job exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{
		"2. **Integration tests** — completed · failure", `log_preview_strategy: "failed-step-context+tail"`, "log_preview_truncated: true",
		"Integration tests", "##[error]expected 200 got 500", "Process completed with exit code 1", "TERMINAL TAIL MARKER",
		"Job log preview locally truncated", "Stable job log endpoint: https://api.github.com/repos/o/r/actions/jobs/202/logs", "expires after one minute", "signed log download", "does not print that redirect location",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("large selected job missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "setup line 0000") || strings.Contains(out, "setup line 1000") || atomic.LoadInt32(&requests) != 2 {
		t.Fatalf("large selected job did not stay scoped/bounded (requests=%d):\n%s", requests, out)
	}
}

func TestSuccessfulLargeJobLogUsesDeterministicHeadTailPreview(t *testing.T) {
	lines := []string{"HEAD LOG MARKER"}
	for i := 0; i < 300; i++ {
		lines = append(lines, fmt.Sprintf("routine line %03d %s", i, strings.Repeat("data ", 10)))
	}
	lines = append(lines, "TAIL LOG MARKER")
	preview := buildGitHubJobLogPreview(strings.Join(lines, "\n"), githubActionsJob{Conclusion: "success"}, 1200)
	if preview.Strategy != "head+tail" || !preview.Truncated || !strings.Contains(preview.Text, "HEAD LOG MARKER") || !strings.Contains(preview.Text, "TAIL LOG MARKER") || !strings.Contains(preview.Text, "log lines omitted") {
		t.Fatalf("unexpected successful-log head/tail preview: %#v\n%s", preview, preview.Text)
	}
	if utf8.RuneCountInString(preview.Text) > 1200 {
		t.Fatalf("head/tail preview exceeded requested budget: %d", utf8.RuneCountInString(preview.Text))
	}
}

func TestActionsJobRejectsMismatchedRunAndUnprovenStepFragment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":202,"run_id":999,"name":"wrong"}`)
	}))
	defer server.Close()
	_, err := readGitHubActionsJob(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/actions/runs/100/job/202"))
	if err == nil || !strings.Contains(err.Error(), "belongs to run 999") {
		t.Fatalf("expected run ownership error, got %v", err)
	}
	_, err = readGitHubActionsJob(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/actions/runs/100/job/202#step:1:2"))
	if err == nil || !strings.Contains(err.Error(), "not a proven native selector") {
		t.Fatalf("unproven step selector should be rejected, got %v", err)
	}
}

func TestActionsJobLogUnavailableIsTruthful(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/actions/jobs/202":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":202,"run_id":100,"name":"old","status":"completed","conclusion":"success"}`)
		case "/repos/o/r/actions/jobs/202/logs":
			w.WriteHeader(http.StatusGone)
			_, _ = io.WriteString(w, `{"message":"Gone"}`)
		}
	}))
	defer server.Close()
	out, err := readGitHubActionsJob(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/actions/runs/100/job/202"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "HTTP 410") || !strings.Contains(out, "expired, deleted, or not yet generated") {
		t.Fatalf("unavailable log state missing:\n%s", out)
	}
}

func TestDecodeGitHubJobLogZIPAndMalformedArchive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, item := range []struct{ name, body string }{{"2_test.txt", "test log\n"}, {"1_build.txt", "build log\n"}} {
		w, err := zw.Create(item.name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, item.body)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := decodeGitHubJobLog(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(text, "1_build.txt") > strings.Index(text, "2_test.txt") || !strings.Contains(text, "build log") || !strings.Contains(text, "test log") {
		t.Fatalf("zip job log order/content incorrect:\n%s", text)
	}
	if _, err := decodeGitHubJobLog([]byte{'P', 'K', 3, 4, 0, 1, 2}); err == nil || !strings.Contains(err.Error(), "decode GitHub Actions job log archive") {
		t.Fatalf("malformed zip should fail truthfully, got %v", err)
	}
}

func TestActionsJobLogRedirectDoesNotForwardTokenToStorageHost(t *testing.T) {
	var storageAuth string
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "redirected log\n")
	}))
	defer storage.Close()
	storageURL := strings.Replace(storage.URL, "127.0.0.1", "localhost", 1)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fake-token" {
			t.Errorf("API request missing configured auth")
		}
		http.Redirect(w, r, storageURL, http.StatusFound)
	}))
	defer api.Close()

	log, err := fetchGitHubActionsJobLog(context.Background(), testGitHubClient(api.URL, api.URL, "fake-token"), &GitHubTarget{Owner: "o", Repo: "r", JobID: 202})
	if err != nil {
		t.Fatal(err)
	}
	if log.Text != "redirected log\n" {
		t.Fatalf("redirected log body mismatch: %q", log.Text)
	}
	if storageAuth != "" {
		t.Fatalf("GitHub token leaked to redirected storage host: %q", storageAuth)
	}
}

func TestActionsProviderFailuresStayNative(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"private actions detail"}`)
	}))
	defer server.Close()
	for _, raw := range []string{
		"https://github.com/o/r/actions",
		"https://github.com/o/r/actions/workflows/ci.yml",
		"https://github.com/o/r/actions/runs/100",
		"https://github.com/o/r/actions/runs/100/job/202",
	} {
		result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget(raw), testGitHubClient(server.URL, server.URL, ""))
		if result.Outcome != GitHubNativeFailure || result.Err == nil || strings.Contains(result.Err.Error(), "private actions detail") {
			t.Fatalf("native Actions failure regressed for %s: %#v", raw, result)
		}
	}
}
