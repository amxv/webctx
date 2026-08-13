package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	githubCommitFilesMax  = 3000
	githubCompareFilesMax = 300
)

type githubCommitDetail struct {
	SHA       string     `json:"sha"`
	HTMLURL   string     `json:"html_url"`
	Author    githubUser `json:"author"`
	Committer githubUser `json:"committer"`
	Commit    struct {
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Date  string `json:"date"`
		} `json:"author"`
		Committer struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Date  string `json:"date"`
		} `json:"committer"`
		CommentCount int `json:"comment_count"`
		Verification struct {
			Verified   bool   `json:"verified"`
			Reason     string `json:"reason"`
			VerifiedAt string `json:"verified_at"`
		} `json:"verification"`
	} `json:"commit"`
	Stats struct {
		Total     int `json:"total"`
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
	} `json:"stats"`
	Files   []githubPullFile `json:"files"`
	Parents []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
	} `json:"parents"`
}

type githubCommitComment struct {
	ID                int64      `json:"id"`
	Body              *string    `json:"body"`
	HTMLURL           string     `json:"html_url"`
	CommitID          string     `json:"commit_id"`
	User              githubUser `json:"user"`
	AuthorAssociation string     `json:"author_association"`
	Path              string     `json:"path"`
	Line              *int       `json:"line"`
	Position          *int       `json:"position"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
}

type githubCompareResult struct {
	HTMLURL         string             `json:"html_url"`
	PermalinkURL    string             `json:"permalink_url"`
	DiffURL         string             `json:"diff_url"`
	PatchURL        string             `json:"patch_url"`
	Status          string             `json:"status"`
	AheadBy         int                `json:"ahead_by"`
	BehindBy        int                `json:"behind_by"`
	TotalCommits    int                `json:"total_commits"`
	BaseCommit      githubPullCommit   `json:"base_commit"`
	MergeBaseCommit githubPullCommit   `json:"merge_base_commit"`
	Commits         []githubPullCommit `json:"commits"`
	Files           []githubPullFile   `json:"files"`
}

type githubBlameRange struct {
	StartingLine int
	EndingLine   int
	Age          int
	Commit       struct {
		OID             string
		AbbreviatedOID  string
		CommittedDate   string
		MessageHeadline string
		URL             string
		Author          struct {
			Name  string
			Email string
			User  *struct {
				Login string
			}
		}
	}
}

func readGitHubCommit(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub commit fragment %q is not a supported native selector", target.Fragment)
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/commits/%s?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(target.Name))
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	if len(pages) == 0 {
		return "", fmt.Errorf("GitHub returned no commit pages")
	}
	var detail githubCommitDetail
	files := []githubPullFile{}
	for i, page := range pages {
		var current githubCommitDetail
		if err := json.Unmarshal(page.Body, &current); err != nil {
			return "", fmt.Errorf("decode GitHub commit: %w", err)
		}
		if i == 0 {
			detail = current
		}
		files = append(files, current.Files...)
	}
	detail.Files = files
	comments, err := fetchGitHubCommitComments(ctx, client, target, detail.SHA)
	if err != nil {
		return "", err
	}
	capReached := len(files) >= githubCommitFilesMax
	return renderGitHubCommit(target, detail, comments, !capReached, capReached), nil
}

func fetchGitHubCommitComments(ctx context.Context, client *GitHubClient, target *GitHubTarget, sha string) ([]githubCommitComment, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits/%s/comments?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(sha))
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	comments := []githubCommitComment{}
	for _, page := range pages {
		var batch []githubCommitComment
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub commit comments: %w", err)
		}
		comments = append(comments, batch...)
	}
	return comments, nil
}

func renderGitHubCommit(target *GitHubTarget, detail githubCommitDetail, comments []githubCommitComment, complete, capReached bool) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"sha: " + yamlScalar(detail.SHA),
		fmt.Sprintf("files_returned: %d", len(detail.Files)),
		fmt.Sprintf("files_complete: %t", complete),
		fmt.Sprintf("comments: %d", len(comments)),
		fmt.Sprintf("additions: %d", detail.Stats.Additions),
		fmt.Sprintf("deletions: %d", detail.Stats.Deletions),
		fmt.Sprintf("changes: %d", detail.Stats.Total),
	}
	if detail.Author.Login != "" {
		lines = append(lines, "author: "+yamlScalar("@"+detail.Author.Login))
	} else if detail.Commit.Author.Name != "" {
		lines = append(lines, "author: "+yamlScalar(detail.Commit.Author.Name))
	}
	if detail.Commit.Author.Date != "" {
		lines = append(lines, "authored: "+yamlScalar(detail.Commit.Author.Date))
	}
	if detail.Committer.Login != "" {
		lines = append(lines, "committer: "+yamlScalar("@"+detail.Committer.Login))
	} else if detail.Commit.Committer.Name != "" {
		lines = append(lines, "committer: "+yamlScalar(detail.Commit.Committer.Name))
	}
	if detail.Commit.Committer.Date != "" {
		lines = append(lines, "committed: "+yamlScalar(detail.Commit.Committer.Date))
	}
	lines = append(lines, fmt.Sprintf("verified: %t", detail.Commit.Verification.Verified))
	if detail.Commit.Verification.Reason != "" {
		lines = append(lines, "verification_reason: "+yamlScalar(detail.Commit.Verification.Reason))
	}
	if detail.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(detail.HTMLURL))
	}
	lines = append(lines, "---", "", "# Commit "+shortSHA(detail.SHA), "")
	if strings.TrimSpace(detail.Commit.Message) != "" {
		lines = append(lines, detail.Commit.Message, "")
	}
	if len(detail.Parents) > 0 {
		lines = append(lines, "## Parents", "")
		for _, parent := range detail.Parents {
			label := "`" + shortSHA(parent.SHA) + "`"
			if parent.HTMLURL != "" {
				label = "[" + label + "](" + parent.HTMLURL + ")"
			}
			lines = append(lines, "- "+label)
		}
		lines = append(lines, "")
	}
	lines = append(lines, "## Changed files", "")
	if capReached {
		lines = append(lines, "> GitHub's commit JSON file listing has a 3,000-file maximum. This result must not be treated as complete beyond that provider ceiling.", "")
	}
	if len(detail.Files) == 0 {
		lines = append(lines, "_No changed files returned by GitHub._", "")
	}
	for _, file := range detail.Files {
		lines = append(lines, renderGitHubPullFile(file, githubDiffSelector{}, false)...)
	}
	lines = append(lines, "## Commit comments", "")
	if len(comments) == 0 {
		lines = append(lines, "_No commit comments._")
	}
	for _, comment := range comments {
		lines = append(lines, renderGitHubCommitComment(comment)...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubCommitComment(comment githubCommitComment) []string {
	heading := "### Comment"
	if comment.User.Login != "" {
		heading += " by @" + comment.User.Login
	}
	if comment.CreatedAt != "" {
		heading += " — " + comment.CreatedAt
	}
	lines := []string{heading, ""}
	meta := []string{}
	if comment.AuthorAssociation != "" {
		meta = append(meta, comment.AuthorAssociation)
	}
	if comment.Path != "" {
		coord := "`" + comment.Path + "`"
		if comment.Line != nil {
			coord += fmt.Sprintf(" line %d", *comment.Line)
		}
		meta = append(meta, coord)
	}
	if comment.HTMLURL != "" {
		meta = append(meta, comment.HTMLURL)
	}
	if len(meta) > 0 {
		lines = append(lines, "_"+strings.Join(meta, " · ")+"_", "")
	}
	if comment.Body == nil {
		lines = append(lines, "_Commit comment body is unavailable or deleted._")
	} else {
		body := strings.TrimSpace(stripInvisibleHTMLComments(*comment.Body))
		if body == "" {
			body = "_Comment is empty after removing invisible GitHub markup._"
		}
		lines = append(lines, body)
	}
	lines = append(lines, "")
	return lines
}

func readGitHubCommitRawDiff(ctx context.Context, client *GitHubClient, target *GitHubTarget, patch bool) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub raw commit diff/patch URLs do not support fragment selection in webctx")
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/commits/%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(target.Name))
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

func readGitHubCompare(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub compare fragment %q is not a supported native selector", target.Fragment)
	}
	base, head, err := githubCompareRefs(target)
	if err != nil {
		return "", err
	}
	endpoint := githubCompareEndpoint(target.Owner, target.Repo, base, head) + "?per_page=100"
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	if len(pages) == 0 {
		return "", fmt.Errorf("GitHub returned no comparison pages")
	}
	var result githubCompareResult
	commits := []githubPullCommit{}
	files := []githubPullFile{}
	for i, page := range pages {
		var current githubCompareResult
		if err := json.Unmarshal(page.Body, &current); err != nil {
			return "", fmt.Errorf("decode GitHub comparison: %w", err)
		}
		if i == 0 {
			result = current
			files = append(files, current.Files...)
		}
		commits = append(commits, current.Commits...)
	}
	result.Commits = commits
	result.Files = files
	filesComplete := len(files) < githubCompareFilesMax
	commitsComplete := result.TotalCommits <= len(commits)
	return renderGitHubCompare(target, base, head, result, commitsComplete, filesComplete), nil
}

func githubCompareRefs(target *GitHubTarget) (string, string, error) {
	if target == nil || len(target.Tail) != 2 || strings.TrimSpace(target.Tail[0]) == "" || strings.TrimSpace(target.Tail[1]) == "" {
		return "", "", fmt.Errorf("GitHub compare URL is missing base/head refs")
	}
	return target.Tail[0], target.Tail[1], nil
}

func githubCompareEndpoint(owner, repo, base, head string) string {
	basehead := base + "..." + head
	return fmt.Sprintf("/repos/%s/%s/compare/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(basehead))
}

func renderGitHubCompare(target *GitHubTarget, base, head string, result githubCompareResult, commitsComplete, filesComplete bool) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"base: " + yamlScalar(base),
		"head: " + yamlScalar(head),
		"status: " + yamlScalar(result.Status),
		fmt.Sprintf("ahead_by: %d", result.AheadBy),
		fmt.Sprintf("behind_by: %d", result.BehindBy),
		fmt.Sprintf("total_commits: %d", result.TotalCommits),
		fmt.Sprintf("commits_returned: %d", len(result.Commits)),
		fmt.Sprintf("commits_complete: %t", commitsComplete),
		fmt.Sprintf("files_returned: %d", len(result.Files)),
		fmt.Sprintf("files_complete: %t", filesComplete),
	}
	if result.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(result.HTMLURL))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# Compare %s...%s", base, head), "")
	if !commitsComplete {
		lines = append(lines, "> GitHub reported more comparison commits than were returned through pagination; commit history is incomplete.", "")
	}
	if !filesComplete {
		lines = append(lines, "> GitHub comparison JSON exposes changed files only on the first page and up to 300 files. This file list may be incomplete.", "")
	}
	lines = append(lines, "## Commits", "")
	if len(result.Commits) == 0 {
		lines = append(lines, "_No comparison commits returned by GitHub._")
	}
	for _, commit := range result.Commits {
		message := firstLine(commit.Commit.Message)
		label := "`" + shortSHA(commit.SHA) + "`"
		if commit.HTMLURL != "" {
			label = "[" + label + "](" + commit.HTMLURL + ")"
		}
		line := "- " + label
		if message != "" {
			line += " " + message
		}
		if commit.Commit.Author.Date != "" {
			line += " — " + commit.Commit.Author.Date
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "## Changed files", "")
	if len(result.Files) == 0 {
		lines = append(lines, "_No changed files returned by GitHub._")
	}
	for _, file := range result.Files {
		lines = append(lines, renderGitHubPullFile(file, githubDiffSelector{}, false)...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubCompareRawDiff(ctx context.Context, client *GitHubClient, target *GitHubTarget, patch bool) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub raw comparison diff/patch URLs do not support fragment selection in webctx")
	}
	base, head, err := githubCompareRefs(target)
	if err != nil {
		return "", err
	}
	accept := "application/vnd.github.v3.diff"
	if patch {
		accept = "application/vnd.github.v3.patch"
	}
	resp, err := client.REST(ctx, http.MethodGet, githubCompareEndpoint(target.Owner, target.Repo, base, head), accept)
	if err != nil {
		return "", err
	}
	return string(resp.Body), nil
}

func resolveGitHubHistoryRefPath(ctx context.Context, client *GitHubClient, target *GitHubTarget) (resolvedGitHubPath, error) {
	if target == nil || len(target.Tail) == 0 {
		return resolvedGitHubPath{}, fmt.Errorf("GitHub history URL is missing a ref")
	}
	matches := []resolvedGitHubPath{}
	var lastNotFound error
	for split := 1; split <= len(target.Tail); split++ {
		ref := strings.Join(target.Tail[:split], "/")
		filePath := strings.Join(target.Tail[split:], "/")
		query := url.Values{"sha": []string{ref}, "per_page": []string{"1"}}
		if filePath != "" {
			query.Set("path", filePath)
		}
		endpoint := fmt.Sprintf("/repos/%s/%s/commits?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
		resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
		if err != nil {
			var ghErr *GitHubError
			if asGitHubError(err, &ghErr) && ghErr.Kind == GitHubErrorNotFound {
				lastNotFound = err
				continue
			}
			return resolvedGitHubPath{}, err
		}
		var commits []githubPullCommit
		if err := json.Unmarshal(resp.Body, &commits); err != nil {
			return resolvedGitHubPath{}, fmt.Errorf("decode GitHub history resolver response: %w", err)
		}
		if len(commits) == 0 {
			continue
		}
		matches = append(matches, resolvedGitHubPath{Ref: ref, Path: filePath, Endpoint: endpoint, Response: resp})
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return resolvedGitHubPath{}, &githubAmbiguousRefError{candidates: matches}
	}
	if lastNotFound != nil {
		return resolvedGitHubPath{}, lastNotFound
	}
	return resolvedGitHubPath{}, &GitHubError{Kind: GitHubErrorNotFound, StatusCode: http.StatusNotFound, HasToken: client.hasToken()}
}

func readGitHubHistory(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub commit-history fragment %q is not a supported native selector", target.Fragment)
	}
	resolved, err := resolveGitHubRefPath(ctx, client, target, "history")
	if err != nil {
		return "", err
	}
	query := url.Values{"sha": []string{resolved.Ref}, "per_page": []string{"30"}}
	if resolved.Path != "" {
		query.Set("path", resolved.Path)
	}
	page := 1
	if rawPage := target.Query.Get("page"); rawPage != "" {
		parsed, err := strconv.Atoi(rawPage)
		if err != nil || parsed <= 0 {
			return "", fmt.Errorf("invalid GitHub commit-history page %q", rawPage)
		}
		page = parsed
		query.Set("page", rawPage)
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/commits?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var commits []githubPullCommit
	if err := json.Unmarshal(resp.Body, &commits); err != nil {
		return "", fmt.Errorf("decode GitHub commit history: %w", err)
	}
	return renderGitHubHistory(target, resolved, page, commits, resp.Links()), nil
}

func renderGitHubHistory(target *GitHubTarget, resolved resolvedGitHubPath, page int, commits []githubPullCommit, links GitHubLinkRelations) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"ref: " + yamlScalar(resolved.Ref),
		"path: " + yamlScalar(resolved.Path),
		fmt.Sprintf("page: %d", page),
		fmt.Sprintf("commits_returned: %d", len(commits)),
		"---",
		"",
	}
	title := "Commit history for " + resolved.Ref
	if resolved.Path != "" {
		title += " / " + resolved.Path
	}
	lines = append(lines, "# "+title, "")
	if len(commits) == 0 {
		lines = append(lines, "_No commits on this page._")
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
	if nav := renderGitHubUIPageNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "", "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubBlame(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub blame fragment %q is not a supported native selector", target.Fragment)
	}
	if !client.hasToken() {
		return "", fmt.Errorf("GitHub blame requires authentication. Set GH_TOKEN or GITHUB_TOKEN")
	}
	resolved, err := resolveGitHubRefPath(ctx, client, target, "file")
	if err != nil {
		return "", err
	}
	const query = `query($owner:String!,$repo:String!,$expression:String!,$path:String!){repository(owner:$owner,name:$repo){object(expression:$expression){... on Commit{oid blame(path:$path){ranges{startingLine endingLine age commit{oid abbreviatedOid committedDate messageHeadline url author{name email user{login}}}}}}}}}`
	variables := map[string]any{"owner": target.Owner, "repo": target.Repo, "expression": resolved.Ref, "path": resolved.Path}
	var data struct {
		Repository *struct {
			Object *struct {
				OID   string `json:"oid"`
				Blame struct {
					Ranges []struct {
						StartingLine int `json:"startingLine"`
						EndingLine   int `json:"endingLine"`
						Age          int `json:"age"`
						Commit       struct {
							OID             string `json:"oid"`
							AbbreviatedOID  string `json:"abbreviatedOid"`
							CommittedDate   string `json:"committedDate"`
							MessageHeadline string `json:"messageHeadline"`
							URL             string `json:"url"`
							Author          struct {
								Name  string `json:"name"`
								Email string `json:"email"`
								User  *struct {
									Login string `json:"login"`
								} `json:"user"`
							} `json:"author"`
						} `json:"commit"`
					} `json:"ranges"`
				} `json:"blame"`
			} `json:"object"`
		} `json:"repository"`
	}
	if err := client.GraphQL(ctx, query, variables, &data); err != nil {
		return "", err
	}
	if data.Repository == nil || data.Repository.Object == nil {
		return "", fmt.Errorf("GitHub blame target was not available from GraphQL")
	}
	ranges := make([]githubBlameRange, 0, len(data.Repository.Object.Blame.Ranges))
	for _, item := range data.Repository.Object.Blame.Ranges {
		var r githubBlameRange
		r.StartingLine = item.StartingLine
		r.EndingLine = item.EndingLine
		r.Age = item.Age
		r.Commit.OID = item.Commit.OID
		r.Commit.AbbreviatedOID = item.Commit.AbbreviatedOID
		r.Commit.CommittedDate = item.Commit.CommittedDate
		r.Commit.MessageHeadline = item.Commit.MessageHeadline
		r.Commit.URL = item.Commit.URL
		r.Commit.Author.Name = item.Commit.Author.Name
		r.Commit.Author.Email = item.Commit.Author.Email
		if item.Commit.Author.User != nil {
			r.Commit.Author.User = &struct{ Login string }{Login: item.Commit.Author.User.Login}
		}
		ranges = append(ranges, r)
	}
	return renderGitHubBlame(target, resolved, data.Repository.Object.OID, ranges), nil
}

func renderGitHubBlame(target *GitHubTarget, resolved resolvedGitHubPath, oid string, ranges []githubBlameRange) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"ref: " + yamlScalar(resolved.Ref),
		"path: " + yamlScalar(resolved.Path),
		"commit: " + yamlScalar(oid),
		fmt.Sprintf("ranges: %d", len(ranges)),
		"---",
		"",
		"# Blame: " + resolved.Path,
		"",
	}
	if len(ranges) == 0 {
		lines = append(lines, "_GitHub returned no blame ranges._")
	}
	for _, r := range ranges {
		sha := r.Commit.AbbreviatedOID
		if sha == "" {
			sha = shortSHA(r.Commit.OID)
		}
		label := "`" + sha + "`"
		if r.Commit.URL != "" {
			label = "[" + label + "](" + r.Commit.URL + ")"
		}
		author := r.Commit.Author.Name
		if r.Commit.Author.User != nil && r.Commit.Author.User.Login != "" {
			author = "@" + r.Commit.Author.User.Login
		}
		line := fmt.Sprintf("- Lines %d-%d — %s", r.StartingLine, r.EndingLine, label)
		if author != "" {
			line += " — " + author
		}
		if r.Commit.CommittedDate != "" {
			line += " — " + r.Commit.CommittedDate
		}
		if r.Commit.MessageHeadline != "" {
			line += " — " + r.Commit.MessageHeadline
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
