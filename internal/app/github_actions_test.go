package app

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
			_, _ = io.WriteString(w, `{"total_count":1,"workflows":[{"id":10,"name":"CI","path":".github/workflows/ci.yml","state":"active","html_url":"https://github.com/o/r/actions/workflows/ci.yml"}]}`)
		case "/repos/o/r/actions/runs":
			for key, want := range map[string]string{"branch": "main", "status": "failure", "page": "2", "per_page": "30"} {
				if got := r.URL.Query().Get(key); got != want {
					t.Errorf("run query %s=%q want %q", key, got, want)
				}
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/actions/runs?branch=main&status=failure&per_page=30&page=1>; rel="prev", <%s/repos/o/r/actions/runs?branch=main&status=failure&per_page=30&page=3>; rel="next"`, server.URL, server.URL))
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
	for _, want := range []string{"workflows_reported: 1", "runs_reported: 50", "[CI](https://github.com/o/r/actions/workflows/ci.yml)", "[Failing build](https://github.com/o/r/actions/runs/100)", "failure", "Previous", "page=1", "Next", "page=3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Actions overview missing %q:\n%s", want, out)
		}
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
			_, _ = io.WriteString(w, `{"id":10,"name":"CI","path":".github/workflows/ci.yml","state":"active","html_url":"https://github.com/o/r/actions/workflows/ci.yml"}`)
		case "/repos/o/r/actions/workflows/ci.yml/runs":
			if r.URL.Query().Get("event") != "push" || r.URL.Query().Get("page") != "2" {
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
	if !strings.Contains(out, "workflow_id: 10") || !strings.Contains(out, "[Push run](https://github.com/o/r/actions/runs/100)") || !strings.Contains(out, "success") {
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

func TestActionsRunPaginatesJobsAndArtifactsWithoutLogs(t *testing.T) {
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
				_, _ = io.WriteString(w, `{"total_count":2,"artifacts":[{"id":302,"name":"expired-report","size_in_bytes":20,"expired":true,"expires_at":"2026-08-02T00:00:00Z","archive_download_url":"https://api.github.test/artifacts/302/zip"}]}`)
				return
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
	for _, want := range []string{"run_id: 100", "run_number: 7", "attempt: 2", "conclusion: \"failure\"", "[build](https://github.com/o/r/actions/runs/100/job/201)", "[test](https://github.com/o/r/actions/runs/100/job/202)", "**report** — 10 bytes", "expires 2026-09-01T00:00:00Z", "**expired-report** — 20 bytes — expired"} {
		if !strings.Contains(out, want) {
			t.Fatalf("run output missing %q:\n%s", want, out)
		}
	}
	if atomic.LoadInt32(&jobPages) != 2 || atomic.LoadInt32(&artifactPages) != 2 {
		t.Fatalf("run pagination incomplete jobs=%d artifacts=%d", jobPages, artifactPages)
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
	if atomic.LoadInt32(&requests) != 2 || !strings.Contains(out, "job_id: 202") || !strings.Contains(out, "1. **Checkout** — completed · success") || !strings.Contains(out, "2. **Test** — completed · failure") || !strings.Contains(out, "line one\nline two") || strings.Contains(out, "job/201") {
		t.Fatalf("job output/scoping incorrect:\n%s", out)
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
