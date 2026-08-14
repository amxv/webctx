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

type githubCommitAvailability struct {
	FilesProviderMore    bool
	CommentsProviderMore bool
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

type githubCompareAvailability struct {
	CommitsProviderMore bool
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
	if commentID, ok, err := parseCommitCommentSelector(target.Fragment); ok {
		if err != nil {
			return "", err
		}
		return readGitHubCommitComment(ctx, client, target, commentID)
	}
	if selector, ok, err := parseGitHubDiffSelector(target.Fragment); ok {
		if err != nil {
			return "", err
		}
		return readGitHubCommitDiffSelector(ctx, client, target, selector)
	}
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub commit fragment %q is not a supported native selector", target.Fragment)
	}
	detail, filesMore, err := fetchGitHubCommitPage(ctx, client, target)
	if err != nil {
		return "", err
	}
	comments, commentsMore, err := fetchGitHubCommitCommentsPage(ctx, client, target, detail)
	if err != nil {
		return "", err
	}
	return renderGitHubCommit(target, detail, comments, githubCommitAvailability{FilesProviderMore: filesMore, CommentsProviderMore: commentsMore}), nil
}

func fetchGitHubCommitPage(ctx context.Context, client *GitHubClient, target *GitHubTarget) (githubCommitDetail, bool, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits/%s?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(target.Name))
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return githubCommitDetail{}, false, err
	}
	var detail githubCommitDetail
	if err := json.Unmarshal(resp.Body, &detail); err != nil {
		return githubCommitDetail{}, false, fmt.Errorf("decode GitHub commit: %w", err)
	}
	return detail, strings.TrimSpace(resp.Links()["next"]) != "", nil
}

func fetchGitHubCommitAllFiles(ctx context.Context, client *GitHubClient, target *GitHubTarget) (githubCommitDetail, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits/%s?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(target.Name))
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return githubCommitDetail{}, err
	}
	if len(pages) == 0 {
		return githubCommitDetail{}, fmt.Errorf("GitHub returned no commit pages")
	}
	var detail githubCommitDetail
	files := []githubPullFile{}
	for i, page := range pages {
		var current githubCommitDetail
		if err := json.Unmarshal(page.Body, &current); err != nil {
			return githubCommitDetail{}, fmt.Errorf("decode GitHub commit: %w", err)
		}
		if i == 0 {
			detail = current
		}
		files = append(files, current.Files...)
	}
	detail.Files = files
	return detail, nil
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

func fetchGitHubCommitCommentsPage(ctx context.Context, client *GitHubClient, target *GitHubTarget, detail githubCommitDetail) ([]githubCommitComment, bool, error) {
	if detail.Commit.CommentCount == 0 {
		return nil, false, nil
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/commits/%s/comments?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(detail.SHA))
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, false, err
	}
	var comments []githubCommitComment
	if err := json.Unmarshal(resp.Body, &comments); err != nil {
		return nil, false, fmt.Errorf("decode GitHub commit comments: %w", err)
	}
	return comments, strings.TrimSpace(resp.Links()["next"]) != "", nil
}

func parseCommitCommentSelector(fragment string) (int64, bool, error) {
	if fragment == "" || !strings.HasPrefix(fragment, "commitcomment-") {
		return 0, false, nil
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(fragment, "commitcomment-"), 10, 64)
	if err != nil || id <= 0 {
		return 0, true, fmt.Errorf("invalid GitHub commit-comment selector %q", fragment)
	}
	return id, true, nil
}

func readGitHubCommitComment(ctx context.Context, client *GitHubClient, target *GitHubTarget, commentID int64) (string, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/comments/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), commentID)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var comment githubCommitComment
	if err := json.Unmarshal(resp.Body, &comment); err != nil {
		return "", fmt.Errorf("decode GitHub commit comment: %w", err)
	}
	if isHexCommitish(target.Name) {
		if !commitishMatchesSHA(target.Name, comment.CommitID) {
			return "", fmt.Errorf("GitHub commit-comment selector commitcomment-%d belongs to commit %s, not %s", commentID, comment.CommitID, target.Name)
		}
	} else if !commitishMatchesSHA(target.Name, comment.CommitID) {
		detail, _, err := fetchGitHubCommitPage(ctx, client, target)
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(detail.SHA, comment.CommitID) {
			return "", fmt.Errorf("GitHub commit-comment selector commitcomment-%d belongs to commit %s, not %s", commentID, comment.CommitID, detail.SHA)
		}
	}
	return renderGitHubSelectedCommitComment(target, comment), nil
}

func commitishMatchesSHA(commitish, sha string) bool {
	commitish = strings.TrimSpace(commitish)
	sha = strings.TrimSpace(sha)
	if len(commitish) < 7 || len(commitish) > len(sha) {
		return false
	}
	for _, r := range commitish {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') && !(r >= 'A' && r <= 'F') {
			return false
		}
	}
	return strings.HasPrefix(strings.ToLower(sha), strings.ToLower(commitish))
}

func isHexCommitish(commitish string) bool {
	commitish = strings.TrimSpace(commitish)
	if len(commitish) < 7 || len(commitish) > 64 {
		return false
	}
	for _, r := range commitish {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') && !(r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

func readGitHubCommitDiffSelector(ctx context.Context, client *GitHubClient, target *GitHubTarget, selector githubDiffSelector) (string, error) {
	detail, err := fetchGitHubCommitAllFiles(ctx, client, target)
	if err != nil {
		return "", err
	}
	var selected *githubPullFile
	for i := range detail.Files {
		if githubDiffPathHash(detail.Files[i].Filename) == selector.Hash {
			selected = &detail.Files[i]
			break
		}
	}
	if selected == nil {
		return "", fmt.Errorf("GitHub diff selector %q does not match any changed file in commit %s", target.Fragment, detail.SHA)
	}
	file := *selected
	if selector.Side != 0 {
		if file.Patch == nil || strings.TrimSpace(*file.Patch) == "" {
			return "", fmt.Errorf("GitHub diff line selector %q targets a file whose patch is unavailable from the provider", target.Fragment)
		}
		patch, err := selectGitHubPatchHunks(*file.Patch, selector.Side, selector.Start, selector.End)
		if err != nil {
			return "", fmt.Errorf("GitHub diff line selector %q is stale or out of range: %w", target.Fragment, err)
		}
		file.Patch = &patch
	}
	return renderGitHubSelectedCommitFile(target, detail, file, selector), nil
}

func renderGitHubCommit(target *GitHubTarget, detail githubCommitDetail, comments []githubCommitComment, availability githubCommitAvailability) string {
	fileLimit := minInt(14, len(detail.Files))
	commentLimit := minInt(6, len(comments))
	for {
		out := renderGitHubCommitWithLimits(target, detail, comments, availability, fileLimit, commentLimit)
		if githubOverviewFits(out) {
			return out
		}
		switch {
		case commentLimit > 1:
			commentLimit--
		case fileLimit > 1:
			fileLimit--
		default:
			return out
		}
	}
}

func renderGitHubCommitWithLimits(target *GitHubTarget, detail githubCommitDetail, comments []githubCommitComment, availability githubCommitAvailability, fileLimit, commentLimit int) string {
	fileLimit = minInt(fileLimit, len(detail.Files))
	commentLimit = minInt(commentLimit, len(comments))
	commentReported := detail.Commit.CommentCount
	if commentReported < len(comments) {
		commentReported = len(comments)
	}
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"sha: " + yamlScalar(detail.SHA),
		"overview: true",
		fmt.Sprintf("files_returned: %d", len(detail.Files)),
		fmt.Sprintf("files_indexed: %d", fileLimit),
		fmt.Sprintf("files_local_omitted: %d", len(detail.Files)-fileLimit),
		fmt.Sprintf("comments_reported: %d", commentReported),
		fmt.Sprintf("comments_returned: %d", len(comments)),
		fmt.Sprintf("comments_indexed: %d", commentLimit),
		fmt.Sprintf("comments_local_omitted: %d", len(comments)-commentLimit),
		fmt.Sprintf("additions: %d", detail.Stats.Additions),
		fmt.Sprintf("deletions: %d", detail.Stats.Deletions),
		fmt.Sprintf("changes: %d", detail.Stats.Total),
		fmt.Sprintf("provider_file_ceiling: %d", githubCommitFilesMax),
	}
	if availability.FilesProviderMore {
		lines = append(lines, "files_provider_more_available: true")
	}
	if availability.CommentsProviderMore {
		lines = append(lines, "comments_provider_more_available: true")
	} else if commentReported > len(comments) {
		lines = append(lines, "comments_provider_complete: false")
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
	lines = append(lines, "## Changed-file index", "")
	if availability.FilesProviderMore {
		lines = append(lines, "> GitHub has more changed files beyond the provider page fetched for this overview. Exact diff selectors and raw `.diff`/`.patch` remain explicit deep paths.", "")
	}
	if len(detail.Files) == 0 {
		lines = append(lines, "_No changed files returned by GitHub._", "")
	}
	for _, file := range detail.Files[:fileLimit] {
		lines = append(lines, renderCommitFileIndex(detail, file)...)
	}
	if note := githubLocalOmissionNote("changed files returned on this provider page", len(detail.Files)-fileLimit); note != "" {
		lines = append(lines, note, "")
	}
	lines = append(lines, "## Commit-comment index", "")
	if len(comments) == 0 {
		lines = append(lines, "_No commit comments returned on this provider page._")
	}
	for _, comment := range comments[:commentLimit] {
		lines = append(lines, renderGitHubCommitCommentIndex(target, detail, comment)...)
	}
	if note := githubLocalOmissionNote("commit comments returned on this provider page", len(comments)-commentLimit); note != "" {
		lines = append(lines, note)
	}
	if availability.CommentsProviderMore {
		lines = append(lines, "", "> GitHub has more commit comments beyond the provider page fetched for this overview.")
	} else if commentReported > len(comments) {
		lines = append(lines, "", "> GitHub reports more commit comments than were returned by the fetched provider data; this provider-incomplete state is separate from local overview omission.")
	}
	lines = append(lines, "", "## Raw GitHub representations", "", "- Diff: "+commitRawURL(detail, target, ".diff"), "- Patch: "+commitRawURL(detail, target, ".patch"))
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderCommitFileIndex(detail githubCommitDetail, file githubPullFile) []string {
	lines := []string{"### `" + file.Filename + "`", ""}
	meta := []string{}
	if file.Status != "" {
		meta = append(meta, "status "+file.Status)
	}
	meta = append(meta, fmt.Sprintf("+%d", file.Additions), fmt.Sprintf("-%d", file.Deletions), fmt.Sprintf("%d changes", file.Changes))
	if file.PreviousFilename != "" {
		meta = append(meta, "renamed from `"+file.PreviousFilename+"`")
	}
	lines = append(lines, "_"+strings.Join(meta, " · ")+"_")
	base := detail.HTMLURL
	if base != "" {
		lines = append(lines, "Selector: "+base+"#diff-"+githubDiffPathHash(file.Filename))
	}
	if file.BlobURL != "" {
		lines = append(lines, "Blob: "+file.BlobURL)
	}
	if file.RawURL != "" {
		lines = append(lines, "Raw: "+file.RawURL)
	}
	lines = append(lines, "")
	return lines
}

func renderGitHubCommitCommentIndex(target *GitHubTarget, detail githubCommitDetail, comment githubCommitComment) []string {
	heading := fmt.Sprintf("### Comment `%d`", comment.ID)
	if comment.User.Login != "" {
		heading += " by @" + comment.User.Login
	}
	if comment.CreatedAt != "" {
		heading += " — " + comment.CreatedAt
	}
	lines := []string{heading, ""}
	if coord := commitCommentCoordinate(comment); coord != "" {
		lines = append(lines, "Location: "+coord, "")
	}
	body := "_Commit comment body is unavailable or deleted._"
	if comment.Body != nil {
		body = strings.TrimSpace(stripInvisibleHTMLComments(*comment.Body))
		if body == "" {
			body = "_Comment is empty after removing invisible GitHub markup._"
		}
	}
	preview, truncated := githubOverviewPreview(body, githubIndexPreviewRunes)
	for _, line := range strings.Split(preview, "\n") {
		lines = append(lines, "> "+line)
	}
	if truncated {
		lines = append(lines, "> _Preview locally truncated._")
	}
	selector := comment.HTMLURL
	if selector == "" {
		base := detail.HTMLURL
		if base == "" {
			base = fmt.Sprintf("https://github.com/%s/%s/commit/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), detail.SHA)
		}
		selector = base + "#commitcomment-" + strconv.FormatInt(comment.ID, 10)
	}
	lines = append(lines, "", "Selector: "+selector, "")
	return lines
}

func commitCommentCoordinate(comment githubCommitComment) string {
	if comment.Path == "" {
		return ""
	}
	coord := "`" + comment.Path + "`"
	if comment.Line != nil {
		coord += fmt.Sprintf(" line %d", *comment.Line)
	}
	return coord
}

func renderGitHubSelectedCommitComment(target *GitHubTarget, comment githubCommitComment) string {
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "commit: " + yamlScalar(comment.CommitID), fmt.Sprintf("comment_id: %d", comment.ID)}
	if comment.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(comment.HTMLURL))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# Commit comment %d", comment.ID), "")
	lines = append(lines, renderGitHubCommitComment(comment)...)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubSelectedCommitFile(target *GitHubTarget, detail githubCommitDetail, file githubPullFile, selector githubDiffSelector) string {
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "commit: " + yamlScalar(detail.SHA), "selector: " + yamlScalar(target.Fragment), "file: " + yamlScalar(file.Filename), "---", "", fmt.Sprintf("# Commit %s — selected diff", shortSHA(detail.SHA)), ""}
	lines = append(lines, renderGitHubPullFile(file, selector, true)...)
	lines = append(lines, "## Raw GitHub representations", "", "- Diff: "+commitRawURL(detail, target, ".diff"), "- Patch: "+commitRawURL(detail, target, ".patch"))
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func commitRawURL(detail githubCommitDetail, target *GitHubTarget, suffix string) string {
	base := strings.TrimSpace(detail.HTMLURL)
	if base == "" {
		sha := detail.SHA
		if sha == "" {
			sha = target.Name
		}
		base = fmt.Sprintf("https://github.com/%s/%s/commit/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), sha)
	}
	return strings.TrimRight(base, "/") + suffix
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
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var result githubCompareResult
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", fmt.Errorf("decode GitHub comparison: %w", err)
	}
	filesComplete := len(result.Files) < githubCompareFilesMax
	availability := githubCompareAvailability{CommitsProviderMore: strings.TrimSpace(resp.Links()["next"]) != ""}
	return renderGitHubCompare(target, base, head, result, availability, filesComplete), nil
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

func renderGitHubCompare(target *GitHubTarget, base, head string, result githubCompareResult, availability githubCompareAvailability, filesComplete bool) string {
	commitLimit := minInt(20, len(result.Commits))
	fileLimit := minInt(14, len(result.Files))
	for {
		out := renderGitHubCompareWithLimits(target, base, head, result, availability, filesComplete, commitLimit, fileLimit)
		if githubOverviewFits(out) {
			return out
		}
		switch {
		case fileLimit > 1:
			fileLimit--
		case commitLimit > 1:
			commitLimit--
		default:
			return out
		}
	}
}

func renderGitHubCompareWithLimits(target *GitHubTarget, base, head string, result githubCompareResult, availability githubCompareAvailability, filesComplete bool, commitLimit, fileLimit int) string {
	commitLimit = minInt(commitLimit, len(result.Commits))
	fileLimit = minInt(fileLimit, len(result.Files))
	commitsProviderComplete := !availability.CommitsProviderMore && result.TotalCommits <= len(result.Commits)
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"base: " + yamlScalar(base),
		"head: " + yamlScalar(head),
		"status: " + yamlScalar(result.Status),
		"overview: true",
		fmt.Sprintf("ahead_by: %d", result.AheadBy),
		fmt.Sprintf("behind_by: %d", result.BehindBy),
		fmt.Sprintf("total_commits: %d", result.TotalCommits),
		fmt.Sprintf("commits_returned: %d", len(result.Commits)),
		fmt.Sprintf("commits_indexed: %d", commitLimit),
		fmt.Sprintf("commits_local_omitted: %d", len(result.Commits)-commitLimit),
		fmt.Sprintf("commits_provider_complete: %t", commitsProviderComplete),
		fmt.Sprintf("files_returned: %d", len(result.Files)),
		fmt.Sprintf("files_indexed: %d", fileLimit),
		fmt.Sprintf("files_local_omitted: %d", len(result.Files)-fileLimit),
		fmt.Sprintf("files_complete: %t", filesComplete),
		fmt.Sprintf("provider_file_ceiling: %d", githubCompareFilesMax),
	}
	if availability.CommitsProviderMore {
		lines = append(lines, "commits_provider_more_available: true")
	}
	if result.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(result.HTMLURL))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# Compare %s...%s", base, head), "", "## Commit index", "")
	if availability.CommitsProviderMore {
		lines = append(lines, "> GitHub has more commits beyond the provider page fetched for this overview.", "")
	} else if !commitsProviderComplete {
		lines = append(lines, "> GitHub reported more commits than were returned by the fetched provider data; this provider-incomplete state is separate from local overview omission.", "")
	}
	if len(result.Commits) == 0 {
		lines = append(lines, "_No comparison commits returned by GitHub._")
	}
	for _, commit := range result.Commits[:commitLimit] {
		message := actionsListLabel(firstLine(commit.Commit.Message))
		label := "`" + shortSHA(commit.SHA) + "`"
		if commit.HTMLURL != "" {
			label = "[" + label + "](" + commit.HTMLURL + ")"
		}
		line := "- " + label
		if message != "" {
			line += " — " + message
		}
		if commit.Commit.Author.Date != "" {
			line += " — " + commit.Commit.Author.Date
		}
		lines = append(lines, line)
	}
	if note := githubLocalOmissionNote("commits returned on this provider page", len(result.Commits)-commitLimit); note != "" {
		lines = append(lines, "", note)
	}
	lines = append(lines, "", "## Changed-file index", "")
	if !filesComplete {
		lines = append(lines, "> GitHub exposes comparison file metadata only on the first page and up to 300 files. Provider ceiling and local overview omission are separate facts.", "")
	}
	if len(result.Files) == 0 {
		lines = append(lines, "_No changed files returned by GitHub._")
	}
	for _, file := range result.Files[:fileLimit] {
		lines = append(lines, renderCompareFileIndex(file)...)
	}
	if note := githubLocalOmissionNote("changed files returned by the comparison provider response", len(result.Files)-fileLimit); note != "" {
		lines = append(lines, note)
	}
	lines = append(lines, "", "## Raw GitHub representations", "", "- Diff: "+compareRawURL(target, base, head, ".diff"), "- Patch: "+compareRawURL(target, base, head, ".patch"))
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderCompareFileIndex(file githubPullFile) []string {
	lines := []string{"### `" + file.Filename + "`", ""}
	meta := []string{}
	if file.Status != "" {
		meta = append(meta, "status "+file.Status)
	}
	meta = append(meta, fmt.Sprintf("+%d", file.Additions), fmt.Sprintf("-%d", file.Deletions), fmt.Sprintf("%d changes", file.Changes))
	if file.PreviousFilename != "" {
		meta = append(meta, "renamed from `"+file.PreviousFilename+"`")
	}
	lines = append(lines, "_"+strings.Join(meta, " · ")+"_")
	if file.BlobURL != "" {
		lines = append(lines, "Blob: "+file.BlobURL)
	}
	if file.RawURL != "" {
		lines = append(lines, "Raw: "+file.RawURL)
	}
	lines = append(lines, "")
	return lines
}

func compareRawURL(target *GitHubTarget, base, head, suffix string) string {
	return fmt.Sprintf("https://github.com/%s/%s/compare/%s...%s%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), escapePathPreservingSlashes(base), escapePathPreservingSlashes(head), suffix)
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
	return githubBoundedOverviewList(len(commits), 20, func(limit int) string {
		return renderGitHubHistoryWithLimit(target, resolved, page, commits, links, limit)
	})
}

func renderGitHubHistoryWithLimit(target *GitHubTarget, resolved resolvedGitHubPath, page int, commits []githubPullCommit, links GitHubLinkRelations, limit int) string {
	limit = minInt(limit, len(commits))
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"ref: " + yamlScalar(resolved.Ref),
		"path: " + yamlScalar(resolved.Path),
		fmt.Sprintf("page: %d", page),
		fmt.Sprintf("commits_returned: %d", len(commits)),
		fmt.Sprintf("commits_indexed: %d", limit),
		fmt.Sprintf("commits_local_omitted: %d", len(commits)-limit),
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
	for _, commit := range commits[:limit] {
		message, truncated := githubOverviewInlinePreview(firstLine(commit.Commit.Message), 140)
		if truncated {
			message += "…"
		}
		label := "`" + shortSHA(commit.SHA) + "`"
		if commit.HTMLURL != "" {
			label = "[" + label + "](" + commit.HTMLURL + ")"
		}
		meta := []string{}
		if commit.Author.Login != "" {
			meta = append(meta, "@"+commit.Author.Login)
		} else if commit.Commit.Author.Name != "" {
			author, truncated := githubOverviewInlinePreview(commit.Commit.Author.Name, 80)
			if truncated {
				author += "…"
			}
			meta = append(meta, author)
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
	if note := githubLocalOmissionNote("commits returned on this history page", len(commits)-limit); note != "" {
		lines = append(lines, "", note)
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
