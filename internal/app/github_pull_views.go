package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const githubPullFilesMax = 3000

type githubPullFile struct {
	SHA              string  `json:"sha"`
	Filename         string  `json:"filename"`
	Status           string  `json:"status"`
	Additions        int     `json:"additions"`
	Deletions        int     `json:"deletions"`
	Changes          int     `json:"changes"`
	BlobURL          string  `json:"blob_url"`
	RawURL           string  `json:"raw_url"`
	ContentsURL      string  `json:"contents_url"`
	Patch            *string `json:"patch"`
	PreviousFilename string  `json:"previous_filename"`
}

type githubPullCommit struct {
	SHA     string     `json:"sha"`
	HTMLURL string     `json:"html_url"`
	Author  githubUser `json:"author"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Date  string `json:"date"`
		} `json:"author"`
		Committer struct {
			Name string `json:"name"`
			Date string `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

type githubCheckRun struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	HeadSHA     string `json:"head_sha"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
	DetailsURL  string `json:"details_url"`
	HTMLURL     string `json:"html_url"`
	ExternalID  string `json:"external_id"`
	App         *struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"app"`
	Output struct {
		Title            string `json:"title"`
		Summary          string `json:"summary"`
		Text             string `json:"text"`
		AnnotationsCount int    `json:"annotations_count"`
		AnnotationsURL   string `json:"annotations_url"`
	} `json:"output"`
}

type githubCheckRunsPage struct {
	TotalCount int              `json:"total_count"`
	CheckRuns  []githubCheckRun `json:"check_runs"`
}

type githubCommitStatus struct {
	ID          int64      `json:"id"`
	State       string     `json:"state"`
	Description string     `json:"description"`
	Context     string     `json:"context"`
	TargetURL   string     `json:"target_url"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
	Creator     githubUser `json:"creator"`
}

type githubCombinedStatus struct {
	State      string               `json:"state"`
	SHA        string               `json:"sha"`
	TotalCount int                  `json:"total_count"`
	Statuses   []githubCommitStatus `json:"statuses"`
}

type githubCheckAnnotation struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	StartColumn     *int   `json:"start_column"`
	EndColumn       *int   `json:"end_column"`
	AnnotationLevel string `json:"annotation_level"`
	Message         string `json:"message"`
	Title           string `json:"title"`
	RawDetails      string `json:"raw_details"`
	BlobHref        string `json:"blob_href"`
}

type githubDiffSelector struct {
	Hash  string
	Side  byte
	Start int
	End   int
}

var (
	githubDiffSelectorRE = regexp.MustCompile(`^diff-([0-9a-fA-F]{64})(?:([LR])([0-9]+)(?:-([LR])([0-9]+))?)?$`)
	githubDiffHunkRE     = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
)

func readGitHubPullFiles(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	selector, hasSelector, err := parseGitHubDiffSelector(target.Fragment)
	if err != nil {
		return "", err
	}
	pr, err := fetchGitHubPullIdentity(ctx, client, target)
	if err != nil {
		return "", err
	}
	files, err := fetchGitHubPullFiles(ctx, client, target)
	if err != nil {
		return "", err
	}
	complete := true
	capReached := false
	if pr.ChangedFiles > len(files) || pr.ChangedFiles >= githubPullFilesMax || len(files) >= githubPullFilesMax {
		complete = false
		capReached = true
	}

	selected := files
	if hasSelector {
		var file *githubPullFile
		for i := range files {
			if githubDiffPathHash(files[i].Filename) == selector.Hash {
				file = &files[i]
				break
			}
		}
		if file == nil {
			return "", fmt.Errorf("GitHub diff selector %q does not match any changed file in this Pull Request", target.Fragment)
		}
		copyFile := *file
		if selector.Side != 0 {
			if copyFile.Patch == nil || strings.TrimSpace(*copyFile.Patch) == "" {
				return "", fmt.Errorf("GitHub diff line selector %q targets a file whose patch is unavailable from the provider", target.Fragment)
			}
			patch, err := selectGitHubPatchHunks(*copyFile.Patch, selector.Side, selector.Start, selector.End)
			if err != nil {
				return "", fmt.Errorf("GitHub diff line selector %q is stale or out of range: %w", target.Fragment, err)
			}
			copyFile.Patch = &patch
		}
		selected = []githubPullFile{copyFile}
	}

	return renderGitHubPullFiles(target, pr, selected, complete, capReached, selector, hasSelector), nil
}

func fetchGitHubPullIdentity(ctx context.Context, client *GitHubClient, target *GitHubTarget) (githubPullRequest, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return githubPullRequest{}, err
	}
	var pr githubPullRequest
	if err := json.Unmarshal(resp.Body, &pr); err != nil {
		return githubPullRequest{}, fmt.Errorf("decode GitHub Pull Request: %w", err)
	}
	return pr, nil
}

func fetchGitHubPullFiles(ctx context.Context, client *GitHubClient, target *GitHubTarget) ([]githubPullFile, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/files?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	files := []githubPullFile{}
	for _, page := range pages {
		var batch []githubPullFile
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub Pull Request files: %w", err)
		}
		files = append(files, batch...)
	}
	return files, nil
}

func parseGitHubDiffSelector(fragment string) (githubDiffSelector, bool, error) {
	if fragment == "" {
		return githubDiffSelector{}, false, nil
	}
	if !strings.HasPrefix(fragment, "diff-") {
		return githubDiffSelector{}, true, fmt.Errorf("GitHub Files Changed fragment %q is not a supported diff selector", fragment)
	}
	match := githubDiffSelectorRE.FindStringSubmatch(fragment)
	if match == nil {
		return githubDiffSelector{}, true, fmt.Errorf("invalid GitHub diff selector %q", fragment)
	}
	selector := githubDiffSelector{Hash: strings.ToLower(match[1])}
	if match[2] == "" {
		return selector, true, nil
	}
	selector.Side = match[2][0]
	selector.Start, _ = strconv.Atoi(match[3])
	selector.End = selector.Start
	if match[4] != "" {
		if match[4][0] != selector.Side {
			return githubDiffSelector{}, true, fmt.Errorf("GitHub diff selector %q mixes left and right sides", fragment)
		}
		selector.End, _ = strconv.Atoi(match[5])
		if selector.End < selector.Start {
			return githubDiffSelector{}, true, fmt.Errorf("GitHub diff selector %q has a reversed line range", fragment)
		}
	}
	if selector.Start <= 0 || selector.End <= 0 {
		return githubDiffSelector{}, true, fmt.Errorf("GitHub diff selector %q has an invalid line number", fragment)
	}
	return selector, true, nil
}

func githubDiffPathHash(filename string) string {
	sum := sha256.Sum256([]byte(filename))
	return hex.EncodeToString(sum[:])
}

func selectGitHubPatchHunks(patch string, side byte, start, end int) (string, error) {
	lines := strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n")
	type hunk struct {
		lines   []string
		matches bool
	}
	hunks := []hunk{}
	current := -1
	oldLine, newLine := 0, 0
	for _, line := range lines {
		if match := githubDiffHunkRE.FindStringSubmatch(line); match != nil {
			oldLine, _ = strconv.Atoi(match[1])
			newLine, _ = strconv.Atoi(match[3])
			hunks = append(hunks, hunk{lines: []string{line}})
			current = len(hunks) - 1
			continue
		}
		if current < 0 {
			continue
		}
		hunks[current].lines = append(hunks[current].lines, line)
		if line == "" {
			// An empty context line in a unified diff is represented with a
			// leading space. A truly empty line here is provider formatting noise.
			continue
		}
		var oldAt, newAt int
		switch line[0] {
		case ' ':
			oldAt, newAt = oldLine, newLine
			oldLine++
			newLine++
		case '-':
			oldAt = oldLine
			oldLine++
		case '+':
			newAt = newLine
			newLine++
		case '\\':
			continue
		default:
			continue
		}
		at := newAt
		if side == 'L' {
			at = oldAt
		}
		if at >= start && at <= end {
			hunks[current].matches = true
		}
	}
	selected := []string{}
	for _, h := range hunks {
		if h.matches {
			selected = append(selected, strings.Join(h.lines, "\n"))
		}
	}
	if len(selected) == 0 {
		return "", fmt.Errorf("selected %c%d-%d line is not present in the provider patch", side, start, end)
	}
	return strings.Join(selected, "\n"), nil
}

func renderGitHubPullFiles(target *GitHubTarget, pr githubPullRequest, files []githubPullFile, complete, capReached bool, selector githubDiffSelector, hasSelector bool) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("pull_request: %d", target.Number),
		"view: files",
		fmt.Sprintf("files_returned: %d", len(files)),
		fmt.Sprintf("changed_files: %d", pr.ChangedFiles),
		fmt.Sprintf("complete: %t", complete),
	}
	if hasSelector {
		lines = append(lines, "selector: "+yamlScalar(target.Fragment))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# Files changed for %s/%s#%d", target.Owner, target.Repo, target.Number), "")
	if capReached {
		lines = append(lines, "> GitHub's Pull Request files API has a 3,000-file maximum. This result must not be treated as complete beyond that provider ceiling.", "")
	}
	if len(files) == 0 {
		lines = append(lines, "_No changed files returned by GitHub._")
	}
	for _, file := range files {
		lines = append(lines, renderGitHubPullFile(file, selector, hasSelector)...)
	}
	lines = append(lines, "", "## Useful GitHub URLs", "")
	lines = append(lines, pullViewHints(target, "files")...)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubPullFile(file githubPullFile, selector githubDiffSelector, hasSelector bool) []string {
	lines := []string{"## " + file.Filename, ""}
	meta := []string{}
	if file.Status != "" {
		meta = append(meta, "status "+file.Status)
	}
	meta = append(meta, fmt.Sprintf("+%d", file.Additions), fmt.Sprintf("-%d", file.Deletions), fmt.Sprintf("%d changes", file.Changes))
	if file.PreviousFilename != "" {
		meta = append(meta, "renamed from `"+file.PreviousFilename+"`")
	}
	lines = append(lines, "_"+strings.Join(meta, " · ")+"_", "")
	if file.BlobURL != "" {
		lines = append(lines, "Blob: "+file.BlobURL)
	}
	if file.RawURL != "" {
		lines = append(lines, "Raw: "+file.RawURL)
	}
	if file.BlobURL != "" || file.RawURL != "" {
		lines = append(lines, "")
	}
	if file.Patch == nil || strings.TrimSpace(*file.Patch) == "" {
		lines = append(lines, "_Patch unavailable from GitHub (for example binary, oversized, or provider-omitted content)._", "")
		return lines
	}
	if hasSelector && selector.Side != 0 {
		label := fmt.Sprintf("Selected %c%d", selector.Side, selector.Start)
		if selector.End != selector.Start {
			label += fmt.Sprintf("-%c%d", selector.Side, selector.End)
		}
		lines = append(lines, "**"+label+" — matching diff hunk:**", "")
	}
	lines = append(lines, "```diff", *file.Patch, "```", "")
	return lines
}

func readGitHubPullCommits(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Pull Request commits fragment %q is not a supported native selector", target.Fragment)
	}
	pr, err := fetchGitHubPullIdentity(ctx, client, target)
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/commits?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	commits := []githubPullCommit{}
	for _, page := range pages {
		var batch []githubPullCommit
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return "", fmt.Errorf("decode GitHub Pull Request commits: %w", err)
		}
		commits = append(commits, batch...)
	}
	return renderGitHubPullCommits(target, pr, commits), nil
}

func renderGitHubPullCommits(target *GitHubTarget, pr githubPullRequest, commits []githubPullCommit) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("pull_request: %d", target.Number),
		"view: commits",
		fmt.Sprintf("commits_returned: %d", len(commits)),
		fmt.Sprintf("commits_reported: %d", pr.Commits),
		"---",
		"",
		fmt.Sprintf("# Commits for %s/%s#%d", target.Owner, target.Repo, target.Number),
		"",
	}
	if len(commits) == 0 {
		lines = append(lines, "_No commits returned by GitHub._")
	}
	for _, commit := range commits {
		message := firstLine(commit.Commit.Message)
		label := "`" + shortSHA(commit.SHA) + "`"
		if commit.HTMLURL != "" {
			label = "[" + label + "](" + commit.HTMLURL + ")"
		}
		meta := []string{}
		if commit.Author.Login != "" {
			meta = append(meta, "@"+commit.Author.Login)
		} else if commit.Commit.Author.Name != "" {
			meta = append(meta, commit.Commit.Author.Name)
		}
		if commit.Commit.Author.Date != "" {
			meta = append(meta, commit.Commit.Author.Date)
		}
		line := "- " + label
		if message != "" {
			line += " " + message
		}
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "## Useful GitHub URLs", "")
	lines = append(lines, pullViewHints(target, "commits")...)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubPullChecks(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Pull Request checks fragment %q is not a supported native selector", target.Fragment)
	}
	pr, err := fetchGitHubPullIdentity(ctx, client, target)
	if err != nil {
		return "", err
	}
	if pr.Head.SHA == "" {
		return "", fmt.Errorf("GitHub Pull Request did not report a head commit SHA for checks")
	}
	if rawID := strings.TrimSpace(target.Query.Get("check_run_id")); rawID != "" {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 {
			return "", fmt.Errorf("invalid GitHub check_run_id %q", rawID)
		}
		return readGitHubSelectedCheck(ctx, client, target, pr, id)
	}

	runs, total, err := fetchGitHubCheckRuns(ctx, client, target, pr.Head.SHA)
	if err != nil {
		return "", err
	}
	status, err := fetchGitHubCombinedStatus(ctx, client, target, pr.Head.SHA)
	if err != nil {
		return "", err
	}
	return renderGitHubPullChecks(target, pr, runs, total, status), nil
}

func fetchGitHubCheckRuns(ctx context.Context, client *GitHubClient, target *GitHubTarget, sha string) ([]githubCheckRun, int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(sha))
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, 0, err
	}
	runs := []githubCheckRun{}
	total := 0
	for _, page := range pages {
		var batch githubCheckRunsPage
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, 0, fmt.Errorf("decode GitHub check runs: %w", err)
		}
		if batch.TotalCount > total {
			total = batch.TotalCount
		}
		runs = append(runs, batch.CheckRuns...)
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].Name == runs[j].Name {
			return runs[i].ID < runs[j].ID
		}
		return runs[i].Name < runs[j].Name
	})
	return runs, total, nil
}

func fetchGitHubCombinedStatus(ctx context.Context, client *GitHubClient, target *GitHubTarget, sha string) (githubCombinedStatus, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits/%s/status?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(sha))
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return githubCombinedStatus{}, err
	}
	combined := githubCombinedStatus{}
	for _, page := range pages {
		var status githubCombinedStatus
		if err := json.Unmarshal(page.Body, &status); err != nil {
			return githubCombinedStatus{}, fmt.Errorf("decode GitHub commit status: %w", err)
		}
		if combined.State == "" {
			combined.State = status.State
		}
		if combined.SHA == "" {
			combined.SHA = status.SHA
		}
		if status.TotalCount > combined.TotalCount {
			combined.TotalCount = status.TotalCount
		}
		combined.Statuses = append(combined.Statuses, status.Statuses...)
	}
	return combined, nil
}

func readGitHubSelectedCheck(ctx context.Context, client *GitHubClient, target *GitHubTarget, pr githubPullRequest, id int64) (string, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/check-runs/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), id)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var run githubCheckRun
	if err := json.Unmarshal(resp.Body, &run); err != nil {
		return "", fmt.Errorf("decode GitHub check run: %w", err)
	}
	if run.HeadSHA != "" && !strings.EqualFold(run.HeadSHA, pr.Head.SHA) {
		return "", fmt.Errorf("GitHub check run %d belongs to head %s, not Pull Request head %s", id, shortSHA(run.HeadSHA), shortSHA(pr.Head.SHA))
	}
	annotations, err := fetchGitHubCheckAnnotations(ctx, client, target, id)
	if err != nil {
		return "", err
	}
	return renderGitHubSelectedCheck(target, pr, run, annotations), nil
}

func fetchGitHubCheckAnnotations(ctx context.Context, client *GitHubClient, target *GitHubTarget, id int64) ([]githubCheckAnnotation, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/check-runs/%d/annotations?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), id)
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	annotations := []githubCheckAnnotation{}
	for _, page := range pages {
		var batch []githubCheckAnnotation
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub check annotations: %w", err)
		}
		annotations = append(annotations, batch...)
	}
	return annotations, nil
}

func renderGitHubPullChecks(target *GitHubTarget, pr githubPullRequest, runs []githubCheckRun, total int, status githubCombinedStatus) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("pull_request: %d", target.Number),
		"view: checks",
		"head_sha: " + yamlScalar(pr.Head.SHA),
		fmt.Sprintf("check_runs_returned: %d", len(runs)),
		fmt.Sprintf("check_runs_reported: %d", total),
		fmt.Sprintf("commit_statuses: %d", len(status.Statuses)),
		"---",
		"",
		fmt.Sprintf("# Checks for %s/%s#%d", target.Owner, target.Repo, target.Number),
		"",
		"## Check runs",
		"",
	}
	if len(runs) == 0 {
		lines = append(lines, "_No check runs returned by GitHub._")
	}
	for _, run := range runs {
		lines = append(lines, renderCheckRunSummary(target, run, true)...)
	}
	lines = append(lines, "", "## Commit statuses", "")
	if status.State != "" {
		lines = append(lines, "Combined status state: `"+status.State+"`", "")
	}
	if len(status.Statuses) == 0 {
		lines = append(lines, "_No commit statuses returned by GitHub._")
	}
	for _, item := range status.Statuses {
		contextName := item.Context
		if contextName == "" {
			contextName = "status"
		}
		line := "- **" + contextName + "** — " + item.State
		if item.Description != "" {
			line += " — " + item.Description
		}
		if item.TargetURL != "" {
			line += " — " + item.TargetURL
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "> Check runs and commit statuses are provider facts for the PR head commit; webctx does not infer a branch-protection/merge decision from them.")
	lines = append(lines, "", "## Useful GitHub URLs", "")
	lines = append(lines, pullViewHints(target, "checks")...)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderCheckRunSummary(target *GitHubTarget, run githubCheckRun, includeFocusedHint bool) []string {
	name := run.Name
	if name == "" {
		name = fmt.Sprintf("Check run %d", run.ID)
	}
	lines := []string{"### " + name, ""}
	meta := []string{fmt.Sprintf("id %d", run.ID)}
	if run.Status != "" {
		meta = append(meta, "status "+run.Status)
	}
	if run.Conclusion != "" {
		meta = append(meta, "conclusion "+run.Conclusion)
	}
	if run.App != nil && run.App.Slug != "" {
		meta = append(meta, "app "+run.App.Slug)
	}
	if run.Output.AnnotationsCount > 0 {
		meta = append(meta, fmt.Sprintf("%d annotations", run.Output.AnnotationsCount))
	}
	lines = append(lines, "_"+strings.Join(meta, " · ")+"_", "")
	if run.Output.Title != "" {
		lines = append(lines, "**"+run.Output.Title+"**", "")
	}
	if strings.TrimSpace(run.Output.Summary) != "" {
		lines = append(lines, strings.TrimSpace(run.Output.Summary), "")
	}
	if run.DetailsURL != "" {
		lines = append(lines, "Details: "+run.DetailsURL)
	}
	if includeFocusedHint && run.ID > 0 && run.Output.AnnotationsCount > 0 {
		base := fmt.Sprintf("https://github.com/%s/%s/pull/%d/checks", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), target.Number)
		lines = append(lines, "Focused check: "+base+"?check_run_id="+strconv.FormatInt(run.ID, 10))
	}
	lines = append(lines, "")
	return lines
}

func renderGitHubSelectedCheck(target *GitHubTarget, pr githubPullRequest, run githubCheckRun, annotations []githubCheckAnnotation) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("pull_request: %d", target.Number),
		"view: check",
		fmt.Sprintf("check_run_id: %d", run.ID),
		"head_sha: " + yamlScalar(pr.Head.SHA),
		fmt.Sprintf("annotations_returned: %d", len(annotations)),
		"---",
		"",
		fmt.Sprintf("# Check: %s", run.Name),
		"",
	}
	lines = append(lines, renderCheckRunSummary(target, run, false)...)
	lines = append(lines, "## Annotations", "")
	if len(annotations) == 0 {
		lines = append(lines, "_No annotations returned by GitHub._")
	}
	for _, annotation := range annotations {
		coord := annotation.Path
		if annotation.StartLine > 0 {
			coord += fmt.Sprintf(":%d", annotation.StartLine)
			if annotation.EndLine > annotation.StartLine {
				coord += fmt.Sprintf("-%d", annotation.EndLine)
			}
		}
		level := annotation.AnnotationLevel
		if level == "" {
			level = "annotation"
		}
		line := "- **" + level + "**"
		if coord != "" {
			line += " `" + coord + "`"
		}
		if annotation.Title != "" {
			line += " — " + annotation.Title
		}
		if annotation.Message != "" {
			line += ": " + annotation.Message
		}
		lines = append(lines, line)
		if annotation.RawDetails != "" {
			lines = append(lines, "  "+annotation.RawDetails)
		}
	}
	lines = append(lines, "", "## Useful GitHub URLs", "")
	lines = append(lines, pullViewHints(target, "checks")...)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubPullRawDiff(ctx context.Context, client *GitHubClient, target *GitHubTarget, patch bool) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub raw Pull Request diff/patch URLs do not support fragment selection in webctx")
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	accept := "application/vnd.github.v3.diff"
	if patch {
		accept = "application/vnd.github.v3.patch"
	}
	resp, err := client.REST(ctx, http.MethodGet, endpoint, accept)
	if err != nil {
		return "", err
	}
	return string(resp.Body), nil
}

func pullViewHints(target *GitHubTarget, current string) []string {
	base := fmt.Sprintf("https://github.com/%s/%s/pull/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), target.Number)
	views := []struct {
		name string
		url  string
	}{
		{name: "conversation", url: base},
		{name: "files", url: base + "/files"},
		{name: "commits", url: base + "/commits"},
		{name: "checks", url: base + "/checks"},
	}
	hints := []string{}
	for _, view := range views {
		if view.name == current {
			continue
		}
		label := strings.ToUpper(view.name[:1]) + view.name[1:]
		hints = append(hints, "- "+label+": "+view.url)
		if len(hints) == 3 {
			break
		}
	}
	return hints
}
