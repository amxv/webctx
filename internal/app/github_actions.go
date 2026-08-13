package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type githubWorkflow struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	BadgeURL  string `json:"badge_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type githubWorkflowList struct {
	TotalCount int              `json:"total_count"`
	Workflows  []githubWorkflow `json:"workflows"`
}

type githubActionsRun struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	DisplayTitle string     `json:"display_title"`
	RunNumber    int        `json:"run_number"`
	RunAttempt   int        `json:"run_attempt"`
	Event        string     `json:"event"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	WorkflowID   int64      `json:"workflow_id"`
	HeadBranch   string     `json:"head_branch"`
	HeadSHA      string     `json:"head_sha"`
	Path         string     `json:"path"`
	HTMLURL      string     `json:"html_url"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
	RunStartedAt string     `json:"run_started_at"`
	Actor        githubUser `json:"actor"`
}

type githubWorkflowRunsPage struct {
	TotalCount   int                `json:"total_count"`
	WorkflowRuns []githubActionsRun `json:"workflow_runs"`
}

type githubActionsStep struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	Number      int    `json:"number"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

type githubActionsJob struct {
	ID           int64               `json:"id"`
	RunID        int64               `json:"run_id"`
	RunAttempt   int                 `json:"run_attempt"`
	HeadSHA      string              `json:"head_sha"`
	Name         string              `json:"name"`
	WorkflowName string              `json:"workflow_name"`
	HeadBranch   string              `json:"head_branch"`
	Status       string              `json:"status"`
	Conclusion   string              `json:"conclusion"`
	HTMLURL      string              `json:"html_url"`
	CheckRunURL  string              `json:"check_run_url"`
	CreatedAt    string              `json:"created_at"`
	StartedAt    string              `json:"started_at"`
	CompletedAt  string              `json:"completed_at"`
	RunnerName   string              `json:"runner_name"`
	RunnerGroup  string              `json:"runner_group_name"`
	Labels       []string            `json:"labels"`
	Steps        []githubActionsStep `json:"steps"`
}

type githubActionsJobsPage struct {
	TotalCount int                `json:"total_count"`
	Jobs       []githubActionsJob `json:"jobs"`
}

type githubActionsArtifact struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	SizeInBytes        int64  `json:"size_in_bytes"`
	ArchiveDownloadURL string `json:"archive_download_url"`
	Expired            bool   `json:"expired"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
	ExpiresAt          string `json:"expires_at"`
}

type githubActionsArtifactsPage struct {
	TotalCount int                     `json:"total_count"`
	Artifacts  []githubActionsArtifact `json:"artifacts"`
}

type githubJobLog struct {
	Text        string
	Unavailable string
	TooLarge    bool
}

func readGitHubActionsOverview(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Actions fragment %q is not a supported native selector", target.Fragment)
	}
	if q := strings.TrimSpace(target.Query.Get("query")); q != "" {
		return "", fmt.Errorf("GitHub Actions UI filter query %q is not yet a supported native filter", q)
	}
	workflows, workflowTotal, _, err := fetchGitHubWorkflowPageForRepo(ctx, client, target.Owner, target.Repo, "")
	if err != nil {
		return "", err
	}
	runs, runTotal, links, err := fetchGitHubRunPage(ctx, client, target, "")
	if err != nil {
		return "", err
	}
	return renderGitHubActionsOverview(target, workflows, workflowTotal, runs, runTotal, links), nil
}

func readGitHubWorkflows(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub workflows fragment %q is not a supported native selector", target.Fragment)
	}
	workflows, total, links, err := fetchGitHubWorkflowPageForRepo(ctx, client, target.Owner, target.Repo, target.Query.Get("page"))
	if err != nil {
		return "", err
	}
	return renderGitHubWorkflows(target, workflows, total, links), nil
}

func fetchGitHubWorkflowPageForRepo(ctx context.Context, client *GitHubClient, owner, repo, page string) ([]githubWorkflow, int, GitHubLinkRelations, error) {
	query := url.Values{"per_page": []string{"30"}}
	if strings.TrimSpace(page) != "" {
		if parsed, err := strconv.Atoi(page); err != nil || parsed <= 0 {
			return nil, 0, nil, fmt.Errorf("invalid GitHub Actions page %q", page)
		}
		query.Set("page", page)
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/workflows?%s", url.PathEscape(owner), url.PathEscape(repo), query.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, 0, nil, err
	}
	var pageData githubWorkflowList
	if err := json.Unmarshal(resp.Body, &pageData); err != nil {
		return nil, 0, nil, fmt.Errorf("decode GitHub workflows: %w", err)
	}
	return pageData.Workflows, pageData.TotalCount, resp.Links(), nil
}

func readGitHubWorkflow(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub workflow fragment %q is not a supported native selector", target.Fragment)
	}
	if q := strings.TrimSpace(target.Query.Get("query")); q != "" {
		return "", fmt.Errorf("GitHub workflow UI filter query %q is not yet a supported native filter", q)
	}
	workflowEndpoint := fmt.Sprintf("/repos/%s/%s/actions/workflows/%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(target.Name))
	resp, err := client.REST(ctx, http.MethodGet, workflowEndpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var workflow githubWorkflow
	if err := json.Unmarshal(resp.Body, &workflow); err != nil {
		return "", fmt.Errorf("decode GitHub workflow: %w", err)
	}
	runs, total, links, err := fetchGitHubRunPage(ctx, client, target, target.Name)
	if err != nil {
		return "", err
	}
	return renderGitHubWorkflow(target, workflow, runs, total, links), nil
}

func fetchGitHubRunPage(ctx context.Context, client *GitHubClient, target *GitHubTarget, workflow string) ([]githubActionsRun, int, GitHubLinkRelations, error) {
	query := copySelectedQuery(target.Query, []string{"actor", "branch", "event", "status", "created", "exclude_pull_requests", "check_suite_id", "head_sha", "page"})
	query.Set("per_page", "30")
	if rawPage := query.Get("page"); rawPage != "" {
		if parsed, err := strconv.Atoi(rawPage); err != nil || parsed <= 0 {
			return nil, 0, nil, fmt.Errorf("invalid GitHub Actions page %q", rawPage)
		}
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs", url.PathEscape(target.Owner), url.PathEscape(target.Repo))
	if workflow != "" {
		endpoint = fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/runs", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(workflow))
	}
	endpoint += "?" + query.Encode()
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, 0, nil, err
	}
	var pageData githubWorkflowRunsPage
	if err := json.Unmarshal(resp.Body, &pageData); err != nil {
		return nil, 0, nil, fmt.Errorf("decode GitHub workflow runs: %w", err)
	}
	return pageData.WorkflowRuns, pageData.TotalCount, resp.Links(), nil
}

func readGitHubActionsRun(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Actions run fragment %q is not a supported native selector", target.Fragment)
	}
	run, err := fetchGitHubActionsRun(ctx, client, target, target.RunID)
	if err != nil {
		return "", err
	}
	jobs, err := fetchGitHubActionsRunJobs(ctx, client, target, target.RunID)
	if err != nil {
		return "", err
	}
	artifacts, err := fetchGitHubActionsRunArtifacts(ctx, client, target, target.RunID)
	if err != nil {
		return "", err
	}
	return renderGitHubActionsRun(target, run, jobs, artifacts), nil
}

func fetchGitHubActionsRun(ctx context.Context, client *GitHubClient, target *GitHubTarget, runID int64) (githubActionsRun, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), runID)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return githubActionsRun{}, err
	}
	var run githubActionsRun
	if err := json.Unmarshal(resp.Body, &run); err != nil {
		return githubActionsRun{}, fmt.Errorf("decode GitHub Actions run: %w", err)
	}
	return run, nil
}

func fetchGitHubActionsRunJobs(ctx context.Context, client *GitHubClient, target *GitHubTarget, runID int64) ([]githubActionsJob, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs?filter=latest&per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), runID)
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	jobs := []githubActionsJob{}
	for _, page := range pages {
		var batch githubActionsJobsPage
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub Actions jobs: %w", err)
		}
		jobs = append(jobs, batch.Jobs...)
	}
	return jobs, nil
}

func fetchGitHubActionsRunArtifacts(ctx context.Context, client *GitHubClient, target *GitHubTarget, runID int64) ([]githubActionsArtifact, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/artifacts?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), runID)
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	artifacts := []githubActionsArtifact{}
	for _, page := range pages {
		var batch githubActionsArtifactsPage
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub Actions artifacts: %w", err)
		}
		artifacts = append(artifacts, batch.Artifacts...)
	}
	return artifacts, nil
}

func readGitHubActionsJob(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Actions job step fragment %q is not a proven native selector", target.Fragment)
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/jobs/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.JobID)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var job githubActionsJob
	if err := json.Unmarshal(resp.Body, &job); err != nil {
		return "", fmt.Errorf("decode GitHub Actions job: %w", err)
	}
	if job.RunID != 0 && job.RunID != target.RunID {
		return "", fmt.Errorf("GitHub Actions job %d belongs to run %d, not selected run %d", target.JobID, job.RunID, target.RunID)
	}
	log, err := fetchGitHubActionsJobLog(ctx, client, target)
	if err != nil {
		return "", err
	}
	return renderGitHubActionsJob(target, job, log), nil
}

func fetchGitHubActionsJobLog(ctx context.Context, client *GitHubClient, target *GitHubTarget) (githubJobLog, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/jobs/%d/logs", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.JobID)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		var ghErr *GitHubError
		if asGitHubError(err, &ghErr) && (ghErr.Kind == GitHubErrorNotFound || ghErr.Kind == GitHubErrorGone) {
			return githubJobLog{Unavailable: fmt.Sprintf("GitHub did not make this job log available (HTTP %d). It may be expired, deleted, or not yet generated.", ghErr.StatusCode)}, nil
		}
		return githubJobLog{}, err
	}
	if resp.TooLarge {
		return githubJobLog{TooLarge: true}, nil
	}
	text, err := decodeGitHubJobLog(resp.Body)
	if err != nil {
		return githubJobLog{}, err
	}
	return githubJobLog{Text: text}, nil
}

func decodeGitHubJobLog(body []byte) (string, error) {
	if len(body) >= 4 && bytes.Equal(body[:4], []byte{'P', 'K', 3, 4}) {
		reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			return "", fmt.Errorf("decode GitHub Actions job log archive: %w", err)
		}
		files := append([]*zip.File(nil), reader.File...)
		sort.SliceStable(files, func(i, j int) bool { return files[i].Name < files[j].Name })
		parts := []string{}
		total := int64(0)
		for _, file := range files {
			if file.FileInfo().IsDir() {
				continue
			}
			total += int64(file.UncompressedSize64)
			if total > githubBlobMaxBytes {
				return "", fmt.Errorf("GitHub Actions job log archive exceeds the supported %d-byte read limit", githubBlobMaxBytes)
			}
			rc, err := file.Open()
			if err != nil {
				return "", fmt.Errorf("open GitHub Actions job log archive entry: %w", err)
			}
			data, readErr := io.ReadAll(io.LimitReader(rc, githubBlobMaxBytes+1))
			_ = rc.Close()
			if readErr != nil {
				return "", fmt.Errorf("read GitHub Actions job log archive entry: %w", readErr)
			}
			if !utf8.Valid(data) {
				return "", fmt.Errorf("GitHub Actions job log archive entry %q is not text", file.Name)
			}
			if len(files) > 1 {
				parts = append(parts, "===== "+file.Name+" =====\n"+string(data))
			} else {
				parts = append(parts, string(data))
			}
		}
		if len(parts) == 0 {
			return "", fmt.Errorf("GitHub Actions job log archive contained no files")
		}
		return strings.Join(parts, "\n"), nil
	}
	if !utf8.Valid(body) {
		return "", fmt.Errorf("GitHub Actions job log response is not UTF-8 text")
	}
	return string(body), nil
}

func renderGitHubActionsOverview(target *GitHubTarget, workflows []githubWorkflow, workflowTotal int, runs []githubActionsRun, runTotal int, links GitHubLinkRelations) string {
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "view: actions", fmt.Sprintf("workflows_returned: %d", len(workflows)), fmt.Sprintf("workflows_reported: %d", workflowTotal), fmt.Sprintf("runs_returned: %d", len(runs)), fmt.Sprintf("runs_reported: %d", runTotal), "---", "", "# Actions", "", "## Workflows", ""}
	lines = append(lines, renderWorkflowList(target, workflows)...)
	lines = append(lines, "", "## Recent runs", "")
	lines = append(lines, renderActionsRunList(target, runs)...)
	if nav := renderGitHubUIPageNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "", "## Run navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubWorkflows(target *GitHubTarget, workflows []githubWorkflow, total int, links GitHubLinkRelations) string {
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "view: workflows", fmt.Sprintf("workflows_returned: %d", len(workflows)), fmt.Sprintf("workflows_reported: %d", total), "---", "", "# Workflows", ""}
	lines = append(lines, renderWorkflowList(target, workflows)...)
	if nav := renderGitHubUIPageNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "", "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderWorkflowList(target *GitHubTarget, workflows []githubWorkflow) []string {
	if len(workflows) == 0 {
		return []string{"_No workflows returned by GitHub._"}
	}
	lines := []string{}
	for _, workflow := range workflows {
		href := workflow.HTMLURL
		if href == "" {
			href = fmt.Sprintf("https://github.com/%s/%s/actions/workflows/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), workflow.ID)
		}
		name := workflow.Name
		if name == "" {
			name = workflow.Path
		}
		line := "- [" + escapeMarkdownLinkText(name) + "](" + href + ")"
		meta := []string{}
		if workflow.State != "" {
			meta = append(meta, workflow.State)
		}
		if workflow.Path != "" {
			meta = append(meta, "`"+workflow.Path+"`")
		}
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	return lines
}

func renderGitHubWorkflow(target *GitHubTarget, workflow githubWorkflow, runs []githubActionsRun, total int, links GitHubLinkRelations) string {
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), fmt.Sprintf("workflow_id: %d", workflow.ID), "name: " + yamlScalar(workflow.Name), "state: " + yamlScalar(workflow.State), "path: " + yamlScalar(workflow.Path), fmt.Sprintf("runs_returned: %d", len(runs)), fmt.Sprintf("runs_reported: %d", total)}
	if workflow.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(workflow.HTMLURL))
	}
	lines = append(lines, "---", "", "# Workflow: "+workflow.Name, "", "## Runs", "")
	lines = append(lines, renderActionsRunList(target, runs)...)
	if nav := renderGitHubUIPageNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "", "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderActionsRunList(target *GitHubTarget, runs []githubActionsRun) []string {
	if len(runs) == 0 {
		return []string{"_No workflow runs returned by GitHub._"}
	}
	lines := []string{}
	for _, run := range runs {
		href := run.HTMLURL
		if href == "" {
			href = fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), run.ID)
		}
		name := run.DisplayTitle
		if name == "" {
			name = run.Name
		}
		if name == "" {
			name = fmt.Sprintf("Run %d", run.ID)
		}
		meta := []string{}
		if run.Status != "" {
			meta = append(meta, run.Status)
		}
		if run.Conclusion != "" {
			meta = append(meta, run.Conclusion)
		}
		if run.Event != "" {
			meta = append(meta, run.Event)
		}
		if run.HeadBranch != "" {
			meta = append(meta, run.HeadBranch)
		}
		line := fmt.Sprintf("- [%s](%s)", escapeMarkdownLinkText(name), href)
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	return lines
}

func renderGitHubActionsRun(target *GitHubTarget, run githubActionsRun, jobs []githubActionsJob, artifacts []githubActionsArtifact) string {
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), fmt.Sprintf("run_id: %d", run.ID), fmt.Sprintf("run_number: %d", run.RunNumber), fmt.Sprintf("attempt: %d", run.RunAttempt), "status: " + yamlScalar(run.Status)}
	if run.Conclusion != "" {
		lines = append(lines, "conclusion: "+yamlScalar(run.Conclusion))
	}
	if run.Event != "" {
		lines = append(lines, "event: "+yamlScalar(run.Event))
	}
	if run.HeadBranch != "" {
		lines = append(lines, "head_branch: "+yamlScalar(run.HeadBranch))
	}
	if run.HeadSHA != "" {
		lines = append(lines, "head_sha: "+yamlScalar(run.HeadSHA))
	}
	if run.Actor.Login != "" {
		lines = append(lines, "actor: "+yamlScalar("@"+run.Actor.Login))
	}
	for _, item := range []struct{ key, value string }{{"started", run.RunStartedAt}, {"created", run.CreatedAt}, {"updated", run.UpdatedAt}} {
		if item.value != "" {
			lines = append(lines, item.key+": "+yamlScalar(item.value))
		}
	}
	if run.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(run.HTMLURL))
	}
	lines = append(lines, fmt.Sprintf("jobs: %d", len(jobs)), fmt.Sprintf("artifacts: %d", len(artifacts)), "---", "")
	title := run.DisplayTitle
	if title == "" {
		title = run.Name
	}
	if title == "" {
		title = fmt.Sprintf("Run %d", run.ID)
	}
	lines = append(lines, "# "+title, "", "## Jobs", "")
	if len(jobs) == 0 {
		lines = append(lines, "_No jobs returned by GitHub._")
	}
	for _, job := range jobs {
		href := job.HTMLURL
		if href == "" {
			href = fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d/job/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), run.ID, job.ID)
		}
		meta := []string{}
		if job.Status != "" {
			meta = append(meta, job.Status)
		}
		if job.Conclusion != "" {
			meta = append(meta, job.Conclusion)
		}
		line := fmt.Sprintf("- [%s](%s)", escapeMarkdownLinkText(job.Name), href)
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "## Artifacts", "")
	if len(artifacts) == 0 {
		lines = append(lines, "_No artifacts returned by GitHub._")
	}
	for _, artifact := range artifacts {
		line := fmt.Sprintf("- **%s** — %d bytes", artifact.Name, artifact.SizeInBytes)
		if artifact.Expired {
			line += " — expired"
		} else if artifact.ExpiresAt != "" {
			line += " — expires " + artifact.ExpiresAt
		}
		if artifact.ArchiveDownloadURL != "" {
			line += " — " + artifact.ArchiveDownloadURL
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubActionsJob(target *GitHubTarget, job githubActionsJob, log githubJobLog) string {
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), fmt.Sprintf("run_id: %d", target.RunID), fmt.Sprintf("job_id: %d", target.JobID), "name: " + yamlScalar(job.Name), "status: " + yamlScalar(job.Status)}
	if job.Conclusion != "" {
		lines = append(lines, "conclusion: "+yamlScalar(job.Conclusion))
	}
	if job.HeadSHA != "" {
		lines = append(lines, "head_sha: "+yamlScalar(job.HeadSHA))
	}
	if job.RunAttempt > 0 {
		lines = append(lines, fmt.Sprintf("attempt: %d", job.RunAttempt))
	}
	if job.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(job.HTMLURL))
	}
	lines = append(lines, "---", "", "# Job: "+job.Name, "", "## Steps", "")
	if len(job.Steps) == 0 {
		lines = append(lines, "_No structured steps returned by GitHub._")
	}
	for _, step := range job.Steps {
		meta := []string{}
		if step.Status != "" {
			meta = append(meta, step.Status)
		}
		if step.Conclusion != "" {
			meta = append(meta, step.Conclusion)
		}
		lines = append(lines, fmt.Sprintf("- %d. **%s** — %s", step.Number, step.Name, strings.Join(meta, " · ")))
	}
	lines = append(lines, "", "## Log", "")
	switch {
	case log.Unavailable != "":
		lines = append(lines, "_"+log.Unavailable+"_")
	case log.TooLarge:
		lines = append(lines, fmt.Sprintf("_GitHub returned a job log larger than the supported %d-byte direct read limit._", githubBlobMaxBytes))
	case log.Text == "":
		lines = append(lines, "_GitHub returned an empty job log._")
	default:
		fence := "```"
		if strings.Contains(log.Text, "```") {
			fence = "````"
		}
		lines = append(lines, fence+"text", log.Text, fence)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
