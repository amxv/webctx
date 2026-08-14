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
	"path"
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

type githubActionsRunAvailability struct {
	JobsReported          int
	ArtifactsReported     int
	ArtifactsProviderMore bool
}

type githubJobLogPreview struct {
	Text       string
	Strategy   string
	Truncated  bool
	LinesTotal int
	LinesShown int
}

func readGitHubActionsOverview(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Actions fragment %q is not a supported native selector", target.Fragment)
	}
	if q := strings.TrimSpace(target.Query.Get("query")); q != "" {
		return "", fmt.Errorf("GitHub Actions UI filter query %q is not yet a supported native filter", q)
	}
	workflows, workflowTotal, _, err := fetchGitHubWorkflowPageForRepo(ctx, client, target.Owner, target.Repo, "", 4)
	if err != nil {
		return "", err
	}
	runs, runTotal, links, err := fetchGitHubRunPage(ctx, client, target, "", 8)
	if err != nil {
		return "", err
	}
	return renderGitHubActionsOverview(target, workflows, workflowTotal, runs, runTotal, links), nil
}

func readGitHubWorkflows(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub workflows fragment %q is not a supported native selector", target.Fragment)
	}
	workflows, total, links, err := fetchGitHubWorkflowPageForRepo(ctx, client, target.Owner, target.Repo, target.Query.Get("page"), githubPageableListSize)
	if err != nil {
		return "", err
	}
	return renderGitHubWorkflows(target, workflows, total, links), nil
}

func fetchGitHubWorkflowPageForRepo(ctx context.Context, client *GitHubClient, owner, repo, page string, perPage int) ([]githubWorkflow, int, GitHubLinkRelations, error) {
	if perPage <= 0 {
		perPage = githubPageableListSize
	}
	query := url.Values{"per_page": []string{strconv.Itoa(perPage)}}
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
	runs, total, links, err := fetchGitHubRunPage(ctx, client, target, target.Name, githubPageableListSize)
	if err != nil {
		return "", err
	}
	return renderGitHubWorkflow(target, workflow, runs, total, links), nil
}

func fetchGitHubRunPage(ctx context.Context, client *GitHubClient, target *GitHubTarget, workflow string, perPage int) ([]githubActionsRun, int, GitHubLinkRelations, error) {
	query := copySelectedQuery(target.Query, []string{"actor", "branch", "event", "status", "created", "exclude_pull_requests", "check_suite_id", "head_sha", "page"})
	if perPage <= 0 {
		perPage = githubPageableListSize
	}
	query.Set("per_page", strconv.Itoa(perPage))
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
	jobs, jobsReported, err := fetchGitHubActionsRunJobs(ctx, client, target, target.RunID)
	if err != nil {
		return "", err
	}
	artifacts, artifactsReported, artifactsMore, err := fetchGitHubActionsRunArtifactsPage(ctx, client, target, target.RunID)
	if err != nil {
		return "", err
	}
	availability := githubActionsRunAvailability{
		JobsReported:          jobsReported,
		ArtifactsReported:     artifactsReported,
		ArtifactsProviderMore: artifactsMore,
	}
	return renderGitHubActionsRun(target, run, jobs, artifacts, availability), nil
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

func fetchGitHubActionsRunJobs(ctx context.Context, client *GitHubClient, target *GitHubTarget, runID int64) ([]githubActionsJob, int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs?filter=latest&per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), runID)
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, 0, err
	}
	jobs := []githubActionsJob{}
	total := 0
	for _, page := range pages {
		var batch githubActionsJobsPage
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, 0, fmt.Errorf("decode GitHub Actions jobs: %w", err)
		}
		if batch.TotalCount > total {
			total = batch.TotalCount
		}
		jobs = append(jobs, batch.Jobs...)
	}
	return jobs, total, nil
}

func fetchGitHubActionsRunArtifactsPage(ctx context.Context, client *GitHubClient, target *GitHubTarget, runID int64) ([]githubActionsArtifact, int, bool, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/artifacts?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), runID)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, 0, false, err
	}
	var batch githubActionsArtifactsPage
	if err := json.Unmarshal(resp.Body, &batch); err != nil {
		return nil, 0, false, fmt.Errorf("decode GitHub Actions artifacts: %w", err)
	}
	return batch.Artifacts, batch.TotalCount, strings.TrimSpace(resp.Links()["next"]) != "", nil
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
	return renderGitHubActionsOverviewWithLimits(target, workflows, workflowTotal, runs, runTotal, links, len(workflows), len(runs))
}

func renderGitHubActionsOverviewWithLimits(target *GitHubTarget, workflows []githubWorkflow, workflowTotal int, runs []githubActionsRun, runTotal int, links GitHubLinkRelations, workflowLimit, runLimit int) string {
	workflowLimit = minInt(workflowLimit, len(workflows))
	runLimit = minInt(runLimit, len(runs))
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"view: actions",
		fmt.Sprintf("workflows_returned: %d", len(workflows)),
		fmt.Sprintf("workflows_reported: %d", workflowTotal),
		fmt.Sprintf("workflows_indexed: %d", workflowLimit),
		fmt.Sprintf("workflows_local_omitted: %d", len(workflows)-workflowLimit),
		fmt.Sprintf("runs_returned: %d", len(runs)),
		fmt.Sprintf("runs_reported: %d", runTotal),
		fmt.Sprintf("runs_indexed: %d", runLimit),
		fmt.Sprintf("runs_local_omitted: %d", len(runs)-runLimit),
	}
	if workflowTotal > len(workflows) {
		lines = append(lines, "workflows_provider_more_available: true")
	}
	if runTotal > len(runs) {
		lines = append(lines, "runs_provider_more_available: true")
	}
	lines = append(lines, "---", "", "# Actions", "", "All workflows: "+actionsWorkflowsURL(target), "", "## Workflow index", "")
	lines = append(lines, renderWorkflowList(target, workflows[:workflowLimit])...)
	lines = append(lines, "", "## Recent run index", "")
	lines = append(lines, renderActionsRunList(target, runs[:runLimit])...)
	if nav := renderGitHubUIPageNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "", "## Run navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubWorkflows(target *GitHubTarget, workflows []githubWorkflow, total int, links GitHubLinkRelations) string {
	limit := len(workflows)
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "view: workflows", fmt.Sprintf("workflows_returned: %d", len(workflows)), fmt.Sprintf("workflows_reported: %d", total), fmt.Sprintf("workflows_indexed: %d", limit), fmt.Sprintf("workflows_local_omitted: %d", len(workflows)-limit), "---", "", "# Workflows", ""}
	lines = append(lines, renderWorkflowList(target, workflows[:limit])...)
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
		href := actionsWorkflowURL(target, workflow)
		name := workflow.Name
		if name == "" {
			name = workflow.Path
		}
		name = actionsListLabel(name)
		line := "- [" + escapeMarkdownLinkText(name) + "](" + href + ")"
		meta := []string{}
		if workflow.State != "" {
			meta = append(meta, workflow.State)
		}
		if workflow.Path != "" {
			path, truncated := githubOverviewInlinePreview(workflow.Path, 140)
			if truncated {
				path += "…"
			}
			meta = append(meta, "`"+path+"`")
		}
		if workflow.HTMLURL != "" && workflow.Path != "" {
			meta = append(meta, "source "+workflow.HTMLURL)
		}
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	return lines
}

func renderGitHubWorkflow(target *GitHubTarget, workflow githubWorkflow, runs []githubActionsRun, total int, links GitHubLinkRelations) string {
	return renderGitHubWorkflowWithLimit(target, workflow, runs, total, links, len(runs))
}

func renderGitHubWorkflowWithLimit(target *GitHubTarget, workflow githubWorkflow, runs []githubActionsRun, total int, links GitHubLinkRelations, runLimit int) string {
	runLimit = minInt(runLimit, len(runs))
	name := actionsListLabel(workflow.Name)
	path, pathTruncated := githubOverviewInlinePreview(workflow.Path, 180)
	if pathTruncated {
		path += "…"
	}
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), fmt.Sprintf("workflow_id: %d", workflow.ID), "name: " + yamlScalar(name), "state: " + yamlScalar(workflow.State), "path: " + yamlScalar(path), fmt.Sprintf("runs_returned: %d", len(runs)), fmt.Sprintf("runs_reported: %d", total), fmt.Sprintf("runs_indexed: %d", runLimit), fmt.Sprintf("runs_local_omitted: %d", len(runs)-runLimit)}
	lines = append(lines, "url: "+yamlScalar(actionsWorkflowURL(target, workflow)))
	if workflow.HTMLURL != "" && workflow.Path != "" {
		lines = append(lines, "source_url: "+yamlScalar(workflow.HTMLURL))
	}
	lines = append(lines, "---", "", "# Workflow: "+name, "", "## Runs", "")
	lines = append(lines, renderActionsRunList(target, runs[:runLimit])...)
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
		name = actionsListLabel(name)
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
			branch, truncated := githubOverviewInlinePreview(run.HeadBranch, 100)
			if truncated {
				branch += "…"
			}
			meta = append(meta, branch)
		}
		line := fmt.Sprintf("- [%s](%s)", escapeMarkdownLinkText(name), href)
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	return lines
}

func actionsListLabel(value string) string {
	preview, truncated := githubOverviewInlinePreview(value, 180)
	if truncated {
		return preview + "…"
	}
	return preview
}

func actionsWorkflowsURL(target *GitHubTarget) string {
	return fmt.Sprintf("https://github.com/%s/%s/actions/workflows", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo))
}

func actionsWorkflowURL(target *GitHubTarget, workflow githubWorkflow) string {
	selector := strings.TrimSpace(workflow.Path)
	if selector != "" {
		selector = path.Base(selector)
	}
	if selector == "" && workflow.ID > 0 {
		selector = strconv.FormatInt(workflow.ID, 10)
	}
	if selector == "" {
		return actionsWorkflowsURL(target)
	}
	return fmt.Sprintf("https://github.com/%s/%s/actions/workflows/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), url.PathEscape(selector))
}

func renderGitHubActionsRun(target *GitHubTarget, run githubActionsRun, jobs []githubActionsJob, artifacts []githubActionsArtifact, availability githubActionsRunAvailability) string {
	orderedJobs := append([]githubActionsJob(nil), jobs...)
	sortGitHubActionsJobs(orderedJobs)
	jobLimit := minInt(18, len(orderedJobs))
	artifactLimit := minInt(8, len(artifacts))
	for {
		out := renderGitHubActionsRunWithLimits(target, run, orderedJobs, artifacts, availability, jobLimit, artifactLimit)
		if githubOverviewFits(out) {
			return out
		}
		switch {
		case artifactLimit > 1:
			artifactLimit--
		case jobLimit > 1:
			jobLimit--
		default:
			return out
		}
	}
}

func renderGitHubActionsRunWithLimits(target *GitHubTarget, run githubActionsRun, jobs []githubActionsJob, artifacts []githubActionsArtifact, availability githubActionsRunAvailability, jobLimit, artifactLimit int) string {
	jobLimit = minInt(jobLimit, len(jobs))
	artifactLimit = minInt(artifactLimit, len(artifacts))
	jobsReported := availability.JobsReported
	if jobsReported < len(jobs) {
		jobsReported = len(jobs)
	}
	artifactsReported := availability.ArtifactsReported
	if artifactsReported < len(artifacts) {
		artifactsReported = len(artifacts)
	}
	jobStatusCounts, jobConclusionCounts := githubActionsJobCounts(jobs)
	expiredArtifacts := 0
	for _, artifact := range artifacts {
		if artifact.Expired {
			expiredArtifacts++
		}
	}
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("run_id: %d", run.ID),
		fmt.Sprintf("run_number: %d", run.RunNumber),
		fmt.Sprintf("attempt: %d", run.RunAttempt),
		"status: " + yamlScalar(run.Status),
	}
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
	lines = append(lines,
		fmt.Sprintf("jobs_returned: %d", len(jobs)),
		fmt.Sprintf("jobs_reported: %d", jobsReported),
		fmt.Sprintf("jobs_indexed: %d", jobLimit),
		fmt.Sprintf("jobs_local_omitted: %d", len(jobs)-jobLimit),
		"job_status_counts: "+jsonMapScalar(jobStatusCounts),
		"job_conclusion_counts: "+jsonMapScalar(jobConclusionCounts),
		fmt.Sprintf("artifacts_returned: %d", len(artifacts)),
		fmt.Sprintf("artifacts_reported: %d", artifactsReported),
		fmt.Sprintf("artifacts_indexed: %d", artifactLimit),
		fmt.Sprintf("artifacts_local_omitted: %d", len(artifacts)-artifactLimit),
		fmt.Sprintf("artifacts_expired_returned: %d", expiredArtifacts),
	)
	if jobsReported > len(jobs) {
		lines = append(lines, "jobs_provider_complete: false")
	}
	if availability.ArtifactsProviderMore {
		lines = append(lines, "artifacts_provider_more_available: true")
	} else if artifactsReported > len(artifacts) {
		lines = append(lines, "artifacts_provider_complete: false")
	}
	lines = append(lines, "---", "")
	title := run.DisplayTitle
	if title == "" {
		title = run.Name
	}
	if title == "" {
		title = fmt.Sprintf("Run %d", run.ID)
	}
	lines = append(lines, "# "+actionsListLabel(title), "", "## Rollup", "")
	lines = append(lines, "- Job statuses: "+formatStringCounts(jobStatusCounts))
	lines = append(lines, "- Job conclusions: "+formatStringCounts(jobConclusionCounts))
	lines = append(lines, fmt.Sprintf("- Artifacts reported: %d (%d returned on fetched provider page, %d expired)", artifactsReported, len(artifacts), expiredArtifacts))

	lines = append(lines, "", "## Job index", "")
	if len(jobs) == 0 {
		lines = append(lines, "_No jobs returned by GitHub._")
	}
	for _, job := range jobs[:jobLimit] {
		lines = append(lines, renderGitHubActionsJobIndex(target, run.ID, job))
	}
	if note := githubLocalOmissionNote("jobs", len(jobs)-jobLimit); note != "" {
		lines = append(lines, "", note)
	}
	if jobsReported > len(jobs) {
		lines = append(lines, "", "> GitHub reported more latest-attempt jobs than were returned by the provider pages; this provider-incomplete state is separate from local overview omission.")
	}

	lines = append(lines, "", "## Artifact index", "")
	if len(artifacts) == 0 {
		lines = append(lines, "_No artifacts returned by GitHub on the fetched provider page._")
	}
	for _, artifact := range artifacts[:artifactLimit] {
		lines = append(lines, renderGitHubActionsArtifactIndex(artifact))
	}
	if note := githubLocalOmissionNote("artifacts returned on this provider page", len(artifacts)-artifactLimit); note != "" {
		lines = append(lines, "", note)
	}
	if availability.ArtifactsProviderMore {
		lines = append(lines, "", "> GitHub has more artifacts beyond the provider page fetched for this overview.")
	} else if artifactsReported > len(artifacts) {
		lines = append(lines, "", "> GitHub reports more artifacts than were returned by the fetched provider data; this provider-incomplete state is separate from local overview omission.")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubActionsJobIndex(target *GitHubTarget, runID int64, job githubActionsJob) string {
	href := job.HTMLURL
	if href == "" {
		href = fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d/job/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), runID, job.ID)
	}
	name := actionsListLabel(job.Name)
	if name == "" {
		name = fmt.Sprintf("Job %d", job.ID)
	}
	meta := []string{fmt.Sprintf("id %d", job.ID)}
	if job.Status != "" {
		meta = append(meta, job.Status)
	}
	if job.Conclusion != "" {
		meta = append(meta, job.Conclusion)
	}
	return fmt.Sprintf("- [%s](%s) — %s", escapeMarkdownLinkText(name), href, strings.Join(meta, " · "))
}

func renderGitHubActionsArtifactIndex(artifact githubActionsArtifact) string {
	name := actionsListLabel(artifact.Name)
	if name == "" {
		name = fmt.Sprintf("Artifact %d", artifact.ID)
	}
	line := fmt.Sprintf("- **%s** — id %d · %d bytes", name, artifact.ID, artifact.SizeInBytes)
	if artifact.Expired {
		line += " · expired"
	} else if artifact.ExpiresAt != "" {
		line += " · expires " + artifact.ExpiresAt
	}
	if artifact.ArchiveDownloadURL != "" {
		line += " · archive API: " + artifact.ArchiveDownloadURL
	}
	return line
}

func sortGitHubActionsJobs(jobs []githubActionsJob) {
	sort.SliceStable(jobs, func(i, j int) bool {
		pi, pj := githubActionsJobPriority(jobs[i]), githubActionsJobPriority(jobs[j])
		if pi != pj {
			return pi < pj
		}
		if jobs[i].Name == jobs[j].Name {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].Name < jobs[j].Name
	})
}

func githubActionsJobPriority(job githubActionsJob) int {
	conclusion := strings.ToLower(strings.TrimSpace(job.Conclusion))
	status := strings.ToLower(strings.TrimSpace(job.Status))
	switch conclusion {
	case "failure", "error", "cancelled", "canceled", "timed_out", "action_required", "startup_failure":
		return 0
	}
	if status != "" && status != "completed" {
		return 1
	}
	if conclusion == "" {
		return 1
	}
	if conclusion == "success" || conclusion == "neutral" || conclusion == "skipped" || conclusion == "stale" {
		return 2
	}
	return 0
}

func githubActionsJobCounts(jobs []githubActionsJob) (map[string]int, map[string]int) {
	statuses := map[string]int{}
	conclusions := map[string]int{}
	for _, job := range jobs {
		status := strings.TrimSpace(job.Status)
		if status == "" {
			status = "unknown"
		}
		statuses[status]++
		conclusion := strings.TrimSpace(job.Conclusion)
		if conclusion == "" {
			conclusion = "none"
		}
		conclusions[conclusion]++
	}
	return statuses, conclusions
}

func renderGitHubActionsJob(target *GitHubTarget, job githubActionsJob, log githubJobLog) string {
	logBudget := 1800
	for {
		out := renderGitHubActionsJobWithLogBudget(target, job, log, logBudget)
		if githubOverviewFits(out) || log.Text == "" || logBudget <= 400 {
			return out
		}
		logBudget -= 200
	}
}

func renderGitHubActionsJobWithLogBudget(target *GitHubTarget, job githubActionsJob, log githubJobLog, logBudget int) string {
	preview := githubJobLogPreview{}
	if log.Text != "" {
		preview = buildGitHubJobLogPreview(log.Text, job, logBudget)
	}
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
	if log.Text != "" {
		lines = append(lines,
			"log_preview_strategy: "+yamlScalar(preview.Strategy),
			fmt.Sprintf("log_preview_truncated: %t", preview.Truncated),
			fmt.Sprintf("log_lines_total: %d", preview.LinesTotal),
			fmt.Sprintf("log_preview_lines_output: %d", preview.LinesShown),
		)
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
		lines = append(lines, fmt.Sprintf("- %d. **%s** — %s", step.Number, actionsListLabel(step.Name), strings.Join(meta, " · ")))
	}
	lines = append(lines, "", "## Log preview", "")
	switch {
	case log.Unavailable != "":
		lines = append(lines, "_"+log.Unavailable+"_")
	case log.TooLarge:
		lines = append(lines, fmt.Sprintf("_GitHub returned a job log larger than the supported %d-byte direct read limit._", githubBlobMaxBytes))
	case log.Text == "":
		lines = append(lines, "_GitHub returned an empty job log._")
	default:
		fence := markdownFenceForText(preview.Text)
		lines = append(lines, "Preview strategy: `"+preview.Strategy+"`.", "", fence+"text", preview.Text, fence)
		if preview.Truncated {
			lines = append(lines, "", "> Job log preview locally truncated. Use the stable GitHub log endpoint below when you explicitly need the provider's complete retained log.")
		}
	}
	lines = append(lines, "", "## Useful GitHub URLs", "")
	if job.HTMLURL != "" {
		lines = append(lines, "- Job page: "+job.HTMLURL)
	}
	lines = append(lines, "- Stable job log endpoint: "+githubActionsJobLogEndpointURL(target))
	lines = append(lines, "  - GitHub redirects this endpoint to a signed log download that expires after one minute when the retained log is available; webctx does not print that redirect location.")
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func buildGitHubJobLogPreview(text string, job githubActionsJob, maxRunes int) githubJobLogPreview {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.TrimRight(normalized, "\n")
	if normalized == "" {
		return githubJobLogPreview{Strategy: "empty"}
	}
	lines := strings.Split(normalized, "\n")
	if utf8.RuneCountInString(normalized) <= maxRunes {
		return githubJobLogPreview{Text: normalized, Strategy: "full", LinesTotal: len(lines), LinesShown: len(lines)}
	}

	failureLine, strategy := findGitHubJobFailureLogLine(lines, job)
	var candidate string
	if failureLine >= 0 {
		start := failureLine - 5
		if start < 0 {
			start = 0
		}
		end := failureLine + 11
		if end > len(lines) {
			end = len(lines)
		}
		candidate, _ = joinGitHubLogRanges(lines, [][2]int{{start, end}, {maxInt(0, len(lines)-6), len(lines)}})
	} else {
		strategy = "head+tail"
		candidate, _ = joinGitHubLogRanges(lines, [][2]int{{0, minInt(7, len(lines))}, {maxInt(0, len(lines)-7), len(lines)}})
	}
	preview, _ := githubOverviewPreview(candidate, maxRunes)
	return githubJobLogPreview{
		Text:       preview,
		Strategy:   strategy,
		Truncated:  true,
		LinesTotal: len(lines),
		LinesShown: strings.Count(preview, "\n") + 1,
	}
}

func findGitHubJobFailureLogLine(lines []string, job githubActionsJob) (int, string) {
	failedSteps := []string{}
	for _, step := range job.Steps {
		if githubActionsHardFailure(step.Conclusion) {
			name := strings.ToLower(strings.TrimSpace(step.Name))
			if name != "" {
				failedSteps = append(failedSteps, name)
			}
		}
	}
	if len(failedSteps) > 0 {
		for i, line := range lines {
			lower := strings.ToLower(line)
			for _, step := range failedSteps {
				if strings.Contains(lower, step) {
					return i, "failed-step-context+tail"
				}
			}
		}
	}
	if githubActionsHardFailure(job.Conclusion) {
		markers := []string{"##[error]", "error:", "fatal:", "panic:", "failed", "failure", "exit code"}
		for i, line := range lines {
			lower := strings.ToLower(line)
			for _, marker := range markers {
				if strings.Contains(lower, marker) {
					return i, "failure-marker-context+tail"
				}
			}
		}
	}
	return -1, "head+tail"
}

func githubActionsHardFailure(conclusion string) bool {
	switch strings.ToLower(strings.TrimSpace(conclusion)) {
	case "failure", "error", "cancelled", "canceled", "timed_out", "action_required", "startup_failure":
		return true
	default:
		return false
	}
}

func joinGitHubLogRanges(lines []string, ranges [][2]int) (string, int) {
	type logRange struct{ start, end int }
	clean := []logRange{}
	for _, raw := range ranges {
		start, end := raw[0], raw[1]
		if start < 0 {
			start = 0
		}
		if end > len(lines) {
			end = len(lines)
		}
		if start >= end {
			continue
		}
		if len(clean) > 0 && start <= clean[len(clean)-1].end {
			if end > clean[len(clean)-1].end {
				clean[len(clean)-1].end = end
			}
			continue
		}
		clean = append(clean, logRange{start: start, end: end})
	}
	parts := []string{}
	shown := 0
	previousEnd := 0
	for i, item := range clean {
		if i == 0 && item.start > 0 {
			parts = append(parts, fmt.Sprintf("[... %d earlier log lines omitted ...]", item.start))
		} else if i > 0 && item.start > previousEnd {
			parts = append(parts, fmt.Sprintf("[... %d log lines omitted ...]", item.start-previousEnd))
		}
		parts = append(parts, strings.Join(lines[item.start:item.end], "\n"))
		shown += item.end - item.start
		previousEnd = item.end
	}
	if len(clean) > 0 && previousEnd < len(lines) {
		parts = append(parts, fmt.Sprintf("[... %d later log lines omitted ...]", len(lines)-previousEnd))
	}
	return strings.Join(parts, "\n"), shown
}

func markdownFenceForText(text string) string {
	for length := 3; ; length++ {
		fence := strings.Repeat("`", length)
		if !strings.Contains(text, fence) {
			return fence
		}
	}
}

func githubActionsJobLogEndpointURL(target *GitHubTarget) string {
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/jobs/%d/logs", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), target.JobID)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
