package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type githubPullRef struct {
	Label string `json:"label"`
	Ref   string `json:"ref"`
	SHA   string `json:"sha"`
}

type githubPullRequest struct {
	ID                int64         `json:"id"`
	Number            int           `json:"number"`
	State             string        `json:"state"`
	Title             string        `json:"title"`
	Body              *string       `json:"body"`
	HTMLURL           string        `json:"html_url"`
	User              githubUser    `json:"user"`
	AuthorAssociation string        `json:"author_association"`
	Draft             bool          `json:"draft"`
	Merged            bool          `json:"merged"`
	MergeableState    string        `json:"mergeable_state"`
	Locked            bool          `json:"locked"`
	CreatedAt         string        `json:"created_at"`
	UpdatedAt         string        `json:"updated_at"`
	ClosedAt          string        `json:"closed_at"`
	MergedAt          string        `json:"merged_at"`
	MergeCommitSHA    string        `json:"merge_commit_sha"`
	Comments          int           `json:"comments"`
	ReviewComments    int           `json:"review_comments"`
	Commits           int           `json:"commits"`
	Additions         int           `json:"additions"`
	Deletions         int           `json:"deletions"`
	ChangedFiles      int           `json:"changed_files"`
	Head              githubPullRef `json:"head"`
	Base              githubPullRef `json:"base"`
}

type githubPullReview struct {
	ID                int64      `json:"id"`
	User              githubUser `json:"user"`
	Body              *string    `json:"body"`
	State             string     `json:"state"`
	HTMLURL           string     `json:"html_url"`
	CommitID          string     `json:"commit_id"`
	SubmittedAt       string     `json:"submitted_at"`
	AuthorAssociation string     `json:"author_association"`
}

type githubPullReviewComment struct {
	ID                  int64      `json:"id"`
	PullRequestReviewID int64      `json:"pull_request_review_id"`
	InReplyToID         *int64     `json:"in_reply_to_id"`
	User                githubUser `json:"user"`
	Body                *string    `json:"body"`
	HTMLURL             string     `json:"html_url"`
	PullRequestURL      string     `json:"pull_request_url"`
	Path                string     `json:"path"`
	DiffHunk            string     `json:"diff_hunk"`
	CommitID            string     `json:"commit_id"`
	OriginalCommitID    string     `json:"original_commit_id"`
	Line                *int       `json:"line"`
	OriginalLine        *int       `json:"original_line"`
	StartLine           *int       `json:"start_line"`
	OriginalStartLine   *int       `json:"original_start_line"`
	Side                string     `json:"side"`
	OriginalSide        string     `json:"original_side"`
	StartSide           string     `json:"start_side"`
	CreatedAt           string     `json:"created_at"`
	UpdatedAt           string     `json:"updated_at"`
	AuthorAssociation   string     `json:"author_association"`
}

type githubPullThreadState struct {
	Resolved   bool
	Outdated   bool
	ResolvedBy string
}

type githubPullThread struct {
	Root    githubPullReviewComment
	Replies []githubPullReviewComment
	State   *githubPullThreadState
}

type githubPullOverviewAvailability struct {
	TimelineProviderMore       bool
	ReviewsProviderMore        bool
	ReviewCommentsProviderMore bool
	ReviewCommentsReturned     int
	ThreadStatesProviderMore   bool
}

type githubPullListItem struct {
	Number    int
	State     string
	Title     string
	HTMLURL   string
	User      githubUser
	UpdatedAt string
	CreatedAt string
	Labels    []githubIssueLabel
}

func readGitHubPullList(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Pull Request-list fragment %q is not a supported native selector", target.Fragment)
	}
	if strings.TrimSpace(target.Query.Get("q")) != "" {
		return readGitHubPullSearch(ctx, client, target)
	}
	query, err := githubPullListProviderQuery(target.Query)
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var pulls []githubPullRequest
	if err := json.Unmarshal(resp.Body, &pulls); err != nil {
		return "", fmt.Errorf("decode GitHub Pull Request list: %w", err)
	}
	items := make([]githubPullListItem, 0, len(pulls))
	for _, pr := range pulls {
		items = append(items, githubPullListItem{
			Number:    pr.Number,
			State:     pullDisplayState(pr),
			Title:     pr.Title,
			HTMLURL:   pr.HTMLURL,
			User:      pr.User,
			UpdatedAt: pr.UpdatedAt,
			CreatedAt: pr.CreatedAt,
		})
	}
	return renderGitHubPullList(target, items, resp.Links(), -1, false), nil
}

func readGitHubPullSearch(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	provider, err := githubPullSearchProviderQuery(target.Query, target.Owner, target.Repo)
	if err != nil {
		return "", err
	}
	resp, err := client.REST(ctx, http.MethodGet, "/search/issues?"+provider.Encode(), "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var result githubIssueSearchResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", fmt.Errorf("decode GitHub Pull Request search: %w", err)
	}
	items := make([]githubPullListItem, 0, len(result.Items))
	for _, item := range result.Items {
		state := item.State
		if item.Draft && item.State == "open" {
			state = "draft"
		}
		href := item.HTMLURL
		if item.PullRequest != nil && item.PullRequest.HTMLURL != "" {
			href = item.PullRequest.HTMLURL
		}
		items = append(items, githubPullListItem{
			Number:    item.Number,
			State:     state,
			Title:     item.Title,
			HTMLURL:   href,
			User:      item.User,
			UpdatedAt: item.UpdatedAt,
			CreatedAt: item.CreatedAt,
			Labels:    item.Labels,
		})
	}
	return renderGitHubPullList(target, items, resp.Links(), result.TotalCount, result.IncompleteResults), nil
}

func githubPullListProviderQuery(input url.Values) (url.Values, error) {
	allowed := map[string]struct{}{
		"state": {}, "head": {}, "base": {}, "sort": {}, "direction": {}, "page": {},
	}
	for key := range input {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("GitHub Pull Request list query parameter %q is not supported by this native view", key)
		}
	}
	query := copySelectedQuery(input, []string{"state", "head", "base", "sort", "direction", "page"})
	if state := query.Get("state"); state != "" && state != "open" && state != "closed" && state != "all" {
		return nil, fmt.Errorf("invalid GitHub Pull Request list state %q", state)
	}
	if sortBy := query.Get("sort"); sortBy != "" && sortBy != "created" && sortBy != "updated" && sortBy != "popularity" && sortBy != "long-running" {
		return nil, fmt.Errorf("invalid GitHub Pull Request list sort %q", sortBy)
	}
	if direction := query.Get("direction"); direction != "" && direction != "asc" && direction != "desc" {
		return nil, fmt.Errorf("invalid GitHub Pull Request list direction %q", direction)
	}
	if err := validateGitHubPageSize(query); err != nil {
		return nil, err
	}
	query.Set("per_page", strconv.Itoa(githubPageableListSize))
	return query, nil
}

func githubPullSearchProviderQuery(input url.Values, owner, repo string) (url.Values, error) {
	allowed := map[string]struct{}{"q": {}, "sort": {}, "order": {}, "page": {}}
	for key := range input {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("GitHub Pull Request search query parameter %q is not supported by this native view", key)
		}
	}
	q := strings.TrimSpace(input.Get("q"))
	if q == "" {
		return nil, fmt.Errorf("GitHub Pull Request search requires a non-empty q parameter")
	}
	resource, err := githubSearchResourceQualifier(q)
	if err != nil {
		return nil, err
	}
	if resource == "issue" {
		return nil, fmt.Errorf("GitHub Pull Request search query cannot explicitly select is:issue")
	}
	if resource == "" {
		q += " is:pr"
	}
	if !hasGitHubSearchQualifierPrefix(q, "repo:") {
		q = "repo:" + owner + "/" + repo + " " + q
	}
	provider := url.Values{"q": []string{strings.TrimSpace(q)}}
	for _, key := range []string{"sort", "order", "page"} {
		if value := input.Get(key); value != "" {
			provider.Set(key, value)
		}
	}
	if order := provider.Get("order"); order != "" && order != "asc" && order != "desc" {
		return nil, fmt.Errorf("invalid GitHub Pull Request search order %q", order)
	}
	if err := validateGitHubPageSize(provider); err != nil {
		return nil, err
	}
	provider.Set("per_page", strconv.Itoa(githubPageableListSize))
	return provider, nil
}

func validateGitHubPageSize(query url.Values) error {
	if rawPage := query.Get("page"); rawPage != "" {
		page, err := strconv.Atoi(rawPage)
		if err != nil || page <= 0 {
			return fmt.Errorf("invalid GitHub list page %q", rawPage)
		}
	}
	if rawSize := query.Get("per_page"); rawSize != "" {
		size, err := strconv.Atoi(rawSize)
		if err != nil || size <= 0 || size > 100 {
			return fmt.Errorf("invalid GitHub list per_page %q", rawSize)
		}
	}
	return nil
}

func renderGitHubPullList(target *GitHubTarget, pulls []githubPullListItem, links GitHubLinkRelations, total int, incomplete bool) string {
	return renderGitHubPullListWithLimit(target, pulls, links, total, incomplete, len(pulls))
}

func renderGitHubPullListWithLimit(target *GitHubTarget, pulls []githubPullListItem, links GitHubLinkRelations, total int, incomplete bool, limit int) string {
	limit = minInt(limit, len(pulls))
	page := target.Query.Get("page")
	if page == "" {
		page = "1"
	}
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"view: pull_requests",
		"page: " + yamlScalar(page),
		fmt.Sprintf("results: %d", len(pulls)),
		fmt.Sprintf("results_indexed: %d", limit),
		fmt.Sprintf("results_local_omitted: %d", len(pulls)-limit),
	}
	if q := target.Query.Get("q"); q != "" {
		preview, truncated := githubOverviewInlinePreview(q, 300)
		lines = append(lines, "query: "+yamlScalar(preview))
		if truncated {
			lines = append(lines, "query_preview_truncated: true")
		}
	}
	if total >= 0 {
		lines = append(lines, fmt.Sprintf("total_matches: %d", total))
		if total > 1000 {
			lines = append(lines, "provider_result_ceiling: 1000")
		}
	}
	if incomplete {
		lines = append(lines, "complete: false")
	}
	lines = append(lines, "---", "", "# Pull Requests", "")
	if incomplete {
		lines = append(lines, "> GitHub marked this search result set as incomplete.", "")
	}
	if total > 1000 {
		lines = append(lines, "> GitHub Search exposes at most 1,000 results for a query; this page does not imply access beyond that provider ceiling.", "")
	}
	if len(pulls) == 0 {
		lines = append(lines, "_No Pull Requests on this page._")
	}
	for _, pr := range pulls[:limit] {
		href := pr.HTMLURL
		if href == "" {
			href = fmt.Sprintf("https://github.com/%s/%s/pull/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), pr.Number)
		}
		state := strings.TrimSpace(pr.State)
		if state == "" {
			state = "unknown"
		}
		meta := []string{state}
		if pr.User.Login != "" {
			meta = append(meta, "@"+pr.User.Login)
		}
		if pr.UpdatedAt != "" {
			meta = append(meta, "updated "+pr.UpdatedAt)
		} else if pr.CreatedAt != "" {
			meta = append(meta, "created "+pr.CreatedAt)
		}
		if labels := issueLabelNames(pr.Labels); len(labels) > 0 {
			meta = append(meta, "labels: "+githubOverviewLabelList(labels, 3))
		}
		title, truncated := githubOverviewInlinePreview(pr.Title, 140)
		if truncated {
			title += "…"
		}
		lines = append(lines, fmt.Sprintf("- [#%d %s](%s) — %s", pr.Number, escapeMarkdownLinkText(title), href, strings.Join(meta, " · ")))
	}
	if nav := renderGitHubUIPageNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "", "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubPullRequest(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target == nil || target.Number <= 0 {
		return "", fmt.Errorf("GitHub Pull Request URL is missing a number")
	}
	if issueID, ok, err := parseIssueBodySelector(target.Fragment); ok {
		if err != nil {
			return "", err
		}
		return readGitHubPullBody(ctx, client, target, issueID)
	}
	if commentID, ok, err := parseIssueCommentSelector(target.Fragment); ok {
		if err != nil {
			return "", err
		}
		return readGitHubIssueComment(ctx, client, target, commentID)
	}
	if commentID, ok, err := parsePullDiscussionSelector(target.Fragment); ok {
		if err != nil {
			return "", err
		}
		return readGitHubPullThreadSelector(ctx, client, target, commentID)
	}
	if reviewID, ok, err := parsePullReviewSelector(target.Fragment); ok {
		if err != nil {
			return "", err
		}
		return readGitHubPullReviewSelector(ctx, client, target, reviewID)
	}
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Pull Request fragment %q is not a supported native selector", target.Fragment)
	}

	base := fmt.Sprintf("/repos/%s/%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo))
	prResp, err := client.REST(ctx, http.MethodGet, fmt.Sprintf("%s/pulls/%d", base, target.Number), "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var pr githubPullRequest
	if err := json.Unmarshal(prResp.Body, &pr); err != nil {
		return "", fmt.Errorf("decode GitHub Pull Request: %w", err)
	}
	issue, err := fetchGitHubIssue(ctx, client, target)
	if err != nil {
		return "", err
	}
	if issue.PullRequest == nil || issue.ID <= 0 {
		return "", fmt.Errorf("GitHub Issue-side identity for Pull Request %s/%s#%d was unavailable", target.Owner, target.Repo, target.Number)
	}
	timeline, timelineMore, err := fetchGitHubPullTimelinePage(ctx, client, target)
	if err != nil {
		return "", err
	}
	reviews, reviewsMore, err := fetchGitHubPullReviewsPage(ctx, client, target)
	if err != nil {
		return "", err
	}
	comments, commentsMore, err := fetchGitHubPullReviewCommentsPage(ctx, client, target)
	if err != nil {
		return "", err
	}
	threads := groupGitHubPullThreads(comments)
	enrichmentNote := ""
	threadStatesMore := false
	if client.hasToken() && len(threads) > 0 {
		states, statesMore, enrichErr := fetchGitHubPullThreadStatesOverview(ctx, client, target)
		if enrichErr != nil {
			enrichmentNote = "Review-thread resolved/outdated enrichment was unavailable from GitHub GraphQL; REST thread content is still complete."
		} else {
			for i := range threads {
				if state, ok := states[threads[i].Root.ID]; ok {
					copyState := state
					threads[i].State = &copyState
				}
			}
			if statesMore {
				enrichmentNote = "Review-thread state enrichment is limited to GitHub's first GraphQL page for this overview; exact thread reads may resolve deeper state without expanding the PR root."
				threadStatesMore = true
			}
		}
	}
	availability := githubPullOverviewAvailability{
		TimelineProviderMore:       timelineMore,
		ReviewsProviderMore:        reviewsMore,
		ReviewCommentsProviderMore: commentsMore,
		ReviewCommentsReturned:     len(comments),
		ThreadStatesProviderMore:   threadStatesMore,
	}
	return renderGitHubPullRequest(target, pr, issue.ID, timeline, reviews, threads, availability, enrichmentNote, client.hasToken()), nil
}

func readGitHubPullBody(ctx context.Context, client *GitHubClient, target *GitHubTarget, issueID int64) (string, error) {
	issue, err := fetchGitHubIssue(ctx, client, target)
	if err != nil {
		return "", err
	}
	if issue.PullRequest == nil {
		return "", fmt.Errorf("GitHub #%d is an Issue, not a Pull Request", target.Number)
	}
	if issue.ID != issueID {
		return "", fmt.Errorf("GitHub Pull Request body selector issue-%d does not belong to %s/%s#%d (Issue-side id %d)", issueID, target.Owner, target.Repo, target.Number, issue.ID)
	}
	return renderGitHubPullBody(target, issue), nil
}

func renderGitHubPullBody(target *GitHubTarget, issue githubIssue) string {
	selector := pullBaseURL(target) + "#issue-" + strconv.FormatInt(issue.ID, 10)
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("pull_request: %d", target.Number),
		fmt.Sprintf("issue_id: %d", issue.ID),
		"title: " + yamlScalar(issue.Title),
	}
	if issue.User.Login != "" {
		lines = append(lines, "author: "+yamlScalar("@"+issue.User.Login))
	}
	lines = append(lines, "url: "+yamlScalar(selector), "---", "", fmt.Sprintf("# Description of %s/%s#%d", target.Owner, target.Repo, target.Number), "")
	if issue.Body == nil || strings.TrimSpace(stripInvisibleHTMLComments(*issue.Body)) == "" {
		lines = append(lines, "_No description provided._")
	} else {
		lines = append(lines, stripInvisibleHTMLComments(*issue.Body))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func fetchGitHubPullTimelinePage(ctx context.Context, client *GitHubClient, target *GitHubTarget) ([]githubTimelineEvent, bool, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/timeline?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, false, err
	}
	var events []githubTimelineEvent
	if err := json.Unmarshal(resp.Body, &events); err != nil {
		return nil, false, fmt.Errorf("decode GitHub Pull Request timeline: %w", err)
	}
	return events, strings.TrimSpace(resp.Links()["next"]) != "", nil
}

func fetchGitHubPullReviewsPage(ctx context.Context, client *GitHubClient, target *GitHubTarget) ([]githubPullReview, bool, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, false, err
	}
	var reviews []githubPullReview
	if err := json.Unmarshal(resp.Body, &reviews); err != nil {
		return nil, false, fmt.Errorf("decode GitHub Pull Request reviews: %w", err)
	}
	sort.SliceStable(reviews, func(i, j int) bool {
		if reviews[i].SubmittedAt == reviews[j].SubmittedAt {
			return reviews[i].ID < reviews[j].ID
		}
		return reviews[i].SubmittedAt < reviews[j].SubmittedAt
	})
	return reviews, strings.TrimSpace(resp.Links()["next"]) != "", nil
}

func fetchGitHubPullReviewCommentsPage(ctx context.Context, client *GitHubClient, target *GitHubTarget) ([]githubPullReviewComment, bool, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, false, err
	}
	var comments []githubPullReviewComment
	if err := json.Unmarshal(resp.Body, &comments); err != nil {
		return nil, false, fmt.Errorf("decode GitHub Pull Request review comments: %w", err)
	}
	return comments, strings.TrimSpace(resp.Links()["next"]) != "", nil
}

func parsePullDiscussionSelector(fragment string) (int64, bool, error) {
	if !strings.HasPrefix(fragment, "discussion_r") {
		return 0, false, nil
	}
	raw := strings.TrimPrefix(fragment, "discussion_r")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, true, fmt.Errorf("invalid GitHub review-comment selector %q", fragment)
	}
	return id, true, nil
}

func parsePullReviewSelector(fragment string) (int64, bool, error) {
	if !strings.HasPrefix(fragment, "pullrequestreview-") {
		return 0, false, nil
	}
	raw := strings.TrimPrefix(fragment, "pullrequestreview-")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, true, fmt.Errorf("invalid GitHub Pull Request review selector %q", fragment)
	}
	return id, true, nil
}

func fetchGitHubPullReviews(ctx context.Context, client *GitHubClient, target *GitHubTarget) ([]githubPullReview, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	out := []githubPullReview{}
	for _, page := range pages {
		var batch []githubPullReview
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub Pull Request reviews: %w", err)
		}
		out = append(out, batch...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SubmittedAt == out[j].SubmittedAt {
			return out[i].ID < out[j].ID
		}
		return out[i].SubmittedAt < out[j].SubmittedAt
	})
	return out, nil
}

func fetchGitHubPullReviewComments(ctx context.Context, client *GitHubClient, target *GitHubTarget) ([]githubPullReviewComment, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	out := []githubPullReviewComment{}
	for _, page := range pages {
		var batch []githubPullReviewComment
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub Pull Request review comments: %w", err)
		}
		out = append(out, batch...)
	}
	return out, nil
}

func groupGitHubPullThreads(comments []githubPullReviewComment) []githubPullThread {
	byID := make(map[int64]githubPullReviewComment, len(comments))
	for _, comment := range comments {
		byID[comment.ID] = comment
	}
	rootID := func(comment githubPullReviewComment) int64 {
		seen := map[int64]struct{}{comment.ID: {}}
		current := comment
		for current.InReplyToID != nil && *current.InReplyToID > 0 {
			parentID := *current.InReplyToID
			if _, cycle := seen[parentID]; cycle {
				break
			}
			seen[parentID] = struct{}{}
			parent, ok := byID[parentID]
			if !ok {
				return parentID
			}
			current = parent
		}
		return current.ID
	}
	groups := map[int64][]githubPullReviewComment{}
	for _, comment := range comments {
		id := rootID(comment)
		groups[id] = append(groups[id], comment)
	}
	threads := make([]githubPullThread, 0, len(groups))
	for id, group := range groups {
		root, ok := byID[id]
		if !ok {
			// A missing root should not discard substantive replies. Promote the
			// earliest visible reply and retain the rest in deterministic order.
			sortPullComments(group)
			root = group[0]
		}
		replies := make([]githubPullReviewComment, 0, len(group)-1)
		for _, comment := range group {
			if comment.ID != root.ID {
				replies = append(replies, comment)
			}
		}
		sortPullComments(replies)
		threads = append(threads, githubPullThread{Root: root, Replies: replies})
	}
	sort.SliceStable(threads, func(i, j int) bool {
		if threads[i].Root.CreatedAt == threads[j].Root.CreatedAt {
			return threads[i].Root.ID < threads[j].Root.ID
		}
		return threads[i].Root.CreatedAt < threads[j].Root.CreatedAt
	})
	return threads
}

func sortPullComments(comments []githubPullReviewComment) {
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].CreatedAt == comments[j].CreatedAt {
			return comments[i].ID < comments[j].ID
		}
		return comments[i].CreatedAt < comments[j].CreatedAt
	})
}

func fetchGitHubPullThreadStatesOverview(ctx context.Context, client *GitHubClient, target *GitHubTarget) (map[int64]githubPullThreadState, bool, error) {
	const query = `query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$number){reviewThreads(first:100){nodes{isResolved isOutdated resolvedBy{login} comments(first:1){nodes{fullDatabaseId}}}pageInfo{hasNextPage}}}}}`
	var data struct {
		Repository struct {
			PullRequest *struct {
				ReviewThreads struct {
					Nodes []struct {
						IsResolved bool `json:"isResolved"`
						IsOutdated bool `json:"isOutdated"`
						ResolvedBy *struct {
							Login string `json:"login"`
						} `json:"resolvedBy"`
						Comments struct {
							Nodes []struct {
								FullDatabaseID string `json:"fullDatabaseId"`
							} `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool `json:"hasNextPage"`
					} `json:"pageInfo"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	if err := client.GraphQL(ctx, query, map[string]any{"owner": target.Owner, "repo": target.Repo, "number": target.Number}, &data); err != nil {
		return nil, false, err
	}
	if data.Repository.PullRequest == nil {
		return nil, false, fmt.Errorf("GitHub GraphQL Pull Request was unavailable")
	}
	states := map[int64]githubPullThreadState{}
	connection := data.Repository.PullRequest.ReviewThreads
	for _, node := range connection.Nodes {
		if len(node.Comments.Nodes) == 0 {
			continue
		}
		id, err := strconv.ParseInt(node.Comments.Nodes[0].FullDatabaseID, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		state := githubPullThreadState{Resolved: node.IsResolved, Outdated: node.IsOutdated}
		if node.ResolvedBy != nil {
			state.ResolvedBy = node.ResolvedBy.Login
		}
		states[id] = state
	}
	return states, connection.PageInfo.HasNextPage, nil
}

func fetchGitHubPullThreadStates(ctx context.Context, client *GitHubClient, target *GitHubTarget) (map[int64]githubPullThreadState, error) {
	const query = `query($owner:String!,$repo:String!,$number:Int!,$after:String){repository(owner:$owner,name:$repo){pullRequest(number:$number){reviewThreads(first:100,after:$after){nodes{isResolved isOutdated resolvedBy{login} comments(first:1){nodes{fullDatabaseId}}}pageInfo{hasNextPage endCursor}}}}}`
	states := map[int64]githubPullThreadState{}
	var after any
	for {
		variables := map[string]any{"owner": target.Owner, "repo": target.Repo, "number": target.Number, "after": after}
		var data struct {
			Repository struct {
				PullRequest *struct {
					ReviewThreads struct {
						Nodes []struct {
							IsResolved bool `json:"isResolved"`
							IsOutdated bool `json:"isOutdated"`
							ResolvedBy *struct {
								Login string `json:"login"`
							} `json:"resolvedBy"`
							Comments struct {
								Nodes []struct {
									FullDatabaseID string `json:"fullDatabaseId"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		if err := client.GraphQL(ctx, query, variables, &data); err != nil {
			return nil, err
		}
		if data.Repository.PullRequest == nil {
			return nil, fmt.Errorf("GitHub GraphQL Pull Request was unavailable")
		}
		connection := data.Repository.PullRequest.ReviewThreads
		for _, node := range connection.Nodes {
			if len(node.Comments.Nodes) == 0 {
				continue
			}
			id, err := strconv.ParseInt(node.Comments.Nodes[0].FullDatabaseID, 10, 64)
			if err != nil || id <= 0 {
				continue
			}
			state := githubPullThreadState{Resolved: node.IsResolved, Outdated: node.IsOutdated}
			if node.ResolvedBy != nil {
				state.ResolvedBy = node.ResolvedBy.Login
			}
			states[id] = state
		}
		if !connection.PageInfo.HasNextPage {
			break
		}
		if connection.PageInfo.EndCursor == "" {
			return nil, fmt.Errorf("GitHub GraphQL review-thread pagination omitted an end cursor")
		}
		after = connection.PageInfo.EndCursor
	}
	return states, nil
}

func readGitHubPullThreadSelector(ctx context.Context, client *GitHubClient, target *GitHubTarget, commentID int64) (string, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/comments/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), commentID)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var selected githubPullReviewComment
	if err := json.Unmarshal(resp.Body, &selected); err != nil {
		return "", fmt.Errorf("decode GitHub review comment: %w", err)
	}
	if selected.PullRequestURL != "" && !strings.HasSuffix(strings.TrimRight(selected.PullRequestURL, "/"), fmt.Sprintf("/repos/%s/%s/pulls/%d", target.Owner, target.Repo, target.Number)) {
		return "", fmt.Errorf("GitHub review comment %d does not belong to %s/%s#%d", commentID, target.Owner, target.Repo, target.Number)
	}
	comments, err := fetchGitHubPullReviewComments(ctx, client, target)
	if err != nil {
		return "", err
	}
	threads := groupGitHubPullThreads(comments)
	for _, thread := range threads {
		if thread.Root.ID == selected.ID || pullThreadContains(thread, selected.ID) {
			if client.hasToken() {
				if states, enrichErr := fetchGitHubPullThreadStates(ctx, client, target); enrichErr == nil {
					if state, ok := states[thread.Root.ID]; ok {
						thread.State = &state
					}
				}
			}
			return renderGitHubPullThreadSelector(target, thread, selected.ID), nil
		}
	}
	// If list pagination changed between reads, preserve the selected comment
	// rather than pretending it did not exist.
	return renderGitHubPullThreadSelector(target, githubPullThread{Root: selected}, selected.ID), nil
}

func pullThreadContains(thread githubPullThread, id int64) bool {
	for _, reply := range thread.Replies {
		if reply.ID == id {
			return true
		}
	}
	return false
}

func readGitHubPullReviewSelector(ctx context.Context, client *GitHubClient, target *GitHubTarget, reviewID int64) (string, error) {
	base := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number, reviewID)
	resp, err := client.REST(ctx, http.MethodGet, base, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var review githubPullReview
	if err := json.Unmarshal(resp.Body, &review); err != nil {
		return "", fmt.Errorf("decode GitHub Pull Request review: %w", err)
	}
	pages, err := client.RESTPages(ctx, base+"/comments?per_page=100", "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	comments := []githubPullReviewComment{}
	for _, page := range pages {
		var batch []githubPullReviewComment
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return "", fmt.Errorf("decode GitHub Pull Request review comments: %w", err)
		}
		comments = append(comments, batch...)
	}
	return renderGitHubPullReviewSelector(target, review, groupGitHubPullThreads(comments)), nil
}

func renderGitHubPullRequest(target *GitHubTarget, pr githubPullRequest, issueID int64, timeline []githubTimelineEvent, reviews []githubPullReview, threads []githubPullThread, availability githubPullOverviewAvailability, enrichmentNote string, hasToken bool) string {
	comments, events := splitPullTimelineForOverview(timeline)
	commentLimit := minInt(3, len(comments))
	eventLimit := minInt(6, len(events))
	reviewLimit := minInt(4, len(reviews))
	threadLimit := minInt(4, len(threads))
	bodyLimit := 900
	for {
		out := renderGitHubPullOverviewWithLimits(target, pr, issueID, comments, events, reviews, threads, availability, enrichmentNote, hasToken, commentLimit, eventLimit, reviewLimit, threadLimit, bodyLimit)
		if githubOverviewFits(out) {
			return out
		}
		switch {
		case eventLimit > 0:
			eventLimit--
		case threadLimit > 1:
			threadLimit--
		case reviewLimit > 1:
			reviewLimit--
		case bodyLimit > 400:
			bodyLimit = 400
		case bodyLimit > 200:
			bodyLimit = 200
		case commentLimit > 1:
			commentLimit--
		default:
			// Mandatory metadata, one representative human comment per available
			// conversation, and exact navigation win over the shared soft target.
			return out
		}
	}
}

func renderGitHubPullOverviewWithLimits(target *GitHubTarget, pr githubPullRequest, issueID int64, comments, events []githubTimelineEvent, reviews []githubPullReview, threads []githubPullThread, availability githubPullOverviewAvailability, enrichmentNote string, hasToken bool, commentLimit, eventLimit, reviewLimit, threadLimit, bodyLimit int) string {
	commentLimit = minInt(commentLimit, len(comments))
	eventLimit = minInt(eventLimit, len(events))
	reviewLimit = minInt(reviewLimit, len(reviews))
	threadLimit = minInt(threadLimit, len(threads))
	titlePreview, titleTruncated := githubOverviewInlinePreview(pr.Title, 180)
	if titleTruncated {
		titlePreview += "…"
	}
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("number: %d", pr.Number),
		fmt.Sprintf("issue_id: %d", issueID),
		"state: " + yamlScalar(pullDisplayState(pr)),
		"title: " + yamlScalar(titlePreview),
		"overview: true",
	}
	if titleTruncated {
		lines = append(lines, "title_preview_truncated: true")
	}
	if pr.ID > 0 {
		lines = append(lines, fmt.Sprintf("pull_request_id: %d", pr.ID))
	}
	if pr.Draft {
		lines = append(lines, "draft: true")
	}
	if pr.User.Login != "" {
		lines = append(lines, "author: "+yamlScalar("@"+pr.User.Login))
	}
	if pr.Base.Label != "" {
		base, truncated := githubOverviewInlinePreview(pr.Base.Label, 140)
		if truncated {
			base += "…"
		}
		lines = append(lines, "base: "+yamlScalar(base))
	}
	if pr.Head.Label != "" {
		head, truncated := githubOverviewInlinePreview(pr.Head.Label, 140)
		if truncated {
			head += "…"
		}
		lines = append(lines, "head: "+yamlScalar(head))
	}
	for _, value := range []struct{ key, value string }{{"created", pr.CreatedAt}, {"updated", pr.UpdatedAt}, {"closed", pr.ClosedAt}, {"merged", pr.MergedAt}} {
		if value.value != "" {
			lines = append(lines, value.key+": "+yamlScalar(value.value))
		}
	}
	lines = append(lines,
		fmt.Sprintf("commits: %d", pr.Commits),
		fmt.Sprintf("changed_files: %d", pr.ChangedFiles),
		fmt.Sprintf("additions: %d", pr.Additions),
		fmt.Sprintf("deletions: %d", pr.Deletions),
		fmt.Sprintf("conversation_comments_reported: %d", pr.Comments),
		fmt.Sprintf("conversation_comments_returned: %d", len(comments)),
		fmt.Sprintf("conversation_comments_indexed: %d", commentLimit),
		fmt.Sprintf("conversation_comments_omitted: %d", len(comments)-commentLimit),
		fmt.Sprintf("conversation_events_returned: %d", len(events)),
		fmt.Sprintf("conversation_events_indexed: %d", eventLimit),
		fmt.Sprintf("conversation_events_omitted: %d", len(events)-eventLimit),
		fmt.Sprintf("conversation_items_returned: %d", len(comments)+len(events)),
		fmt.Sprintf("conversation_items_indexed: %d", commentLimit+eventLimit),
		fmt.Sprintf("conversation_items_omitted: %d", len(comments)+len(events)-commentLimit-eventLimit),
		fmt.Sprintf("reviews_returned: %d", len(reviews)),
		fmt.Sprintf("reviews_indexed: %d", reviewLimit),
		fmt.Sprintf("reviews_omitted: %d", len(reviews)-reviewLimit),
		fmt.Sprintf("review_comments_reported: %d", pr.ReviewComments),
		fmt.Sprintf("review_comments_returned: %d", availability.ReviewCommentsReturned),
		fmt.Sprintf("threads_returned: %d", len(threads)),
		fmt.Sprintf("threads_indexed: %d", threadLimit),
		fmt.Sprintf("threads_omitted: %d", len(threads)-threadLimit),
	)
	if availability.TimelineProviderMore {
		lines = append(lines, "conversation_provider_more_available: true")
	} else if pr.Comments > len(comments) {
		lines = append(lines, "conversation_provider_complete: false")
	}
	if availability.ReviewsProviderMore {
		lines = append(lines, "reviews_provider_more_available: true")
	}
	if availability.ReviewCommentsProviderMore {
		lines = append(lines, "review_comments_provider_more_available: true")
	} else if pr.ReviewComments > availability.ReviewCommentsReturned {
		lines = append(lines, "review_comments_provider_complete: false")
	}
	if len(threads) > 0 && !hasToken {
		lines = append(lines, "thread_state_enrichment: unavailable_without_auth")
	} else if availability.ThreadStatesProviderMore {
		lines = append(lines, "thread_state_enrichment: partial_provider_page")
	} else if enrichmentNote != "" {
		lines = append(lines, "thread_state_enrichment: unavailable")
	}
	if pr.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(pr.HTMLURL))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# #%d %s", pr.Number, titlePreview), "", "> Pull Request overview: description and child conversations are previewed/indexed; use the exact URLs below for full scoped content.")

	bodySelector := pullBaseURL(target) + "#issue-" + strconv.FormatInt(issueID, 10)
	lines = append(lines, "", "## Description preview", "")
	if pr.Body == nil || strings.TrimSpace(stripInvisibleHTMLComments(*pr.Body)) == "" {
		lines = append(lines, "_No description provided._")
	} else {
		preview, truncated := githubOverviewPreview(stripInvisibleHTMLComments(*pr.Body), bodyLimit)
		lines = append(lines, preview)
		if truncated {
			lines = append(lines, "", "> Description preview locally truncated for this overview.")
		}
	}
	lines = append(lines, "> Full description: "+bodySelector)

	lines = append(lines, "", "## Conversation comment index", "")
	if len(comments) == 0 {
		lines = append(lines, "_No ordinary conversation comments returned on this provider page._")
	}
	for _, event := range comments[:commentLimit] {
		comment := githubIssueComment{ID: event.ID, Body: event.Body, HTMLURL: event.HTMLURL, User: event.User, AuthorAssociation: event.AuthorAssociation, CreatedAt: event.CreatedAt, UpdatedAt: event.UpdatedAt, IsPinned: event.IsPinned, Minimized: event.Minimized, MinimizedReason: event.MinimizedReason}
		lines = append(lines, renderPullConversationCommentIndex(target, comment)...)
	}
	if note := githubLocalOmissionNote("conversation comments", len(comments)-commentLimit); note != "" {
		lines = append(lines, "", note)
	}

	lines = append(lines, "", "## Timeline event index", "")
	if len(events) == 0 {
		lines = append(lines, "_No substantive non-comment timeline events returned on this provider page._")
	}
	for _, event := range events[:eventLimit] {
		if rendered, ok := renderPullTimelineState(event); ok {
			lines = append(lines, rendered)
		}
	}
	if note := githubLocalOmissionNote("timeline events", len(events)-eventLimit); note != "" {
		lines = append(lines, "", note)
	}
	if availability.TimelineProviderMore {
		lines = append(lines, "", "> GitHub has more conversation timeline items beyond the provider page fetched for this overview.")
	} else if pr.Comments > len(comments) {
		lines = append(lines, "", fmt.Sprintf("> GitHub reports %d conversation comments, while the fetched timeline page returned %d comment events; this provider-incomplete state is separate from local overview omission.", pr.Comments, len(comments)))
	}

	lines = append(lines, "", "## Review index", "")
	if len(reviews) == 0 {
		lines = append(lines, "_No submitted reviews returned on this provider page._")
	}
	for _, review := range reviews[:reviewLimit] {
		lines = append(lines, renderPullReviewIndex(target, review)...)
	}
	if note := githubLocalOmissionNote("reviews", len(reviews)-reviewLimit); note != "" {
		lines = append(lines, "", note)
	}
	if availability.ReviewsProviderMore {
		lines = append(lines, "", "> GitHub has more submitted reviews beyond the provider page fetched for this overview.")
	}

	lines = append(lines, "", "## Inline thread index", "")
	if len(threads) == 0 {
		lines = append(lines, "_No inline review threads returned on this provider page._")
	}
	for _, thread := range threads[:threadLimit] {
		lines = append(lines, renderPullThreadIndex(target, thread)...)
	}
	if note := githubLocalOmissionNote("inline review threads", len(threads)-threadLimit); note != "" {
		lines = append(lines, "", note)
	}
	if availability.ReviewCommentsProviderMore {
		lines = append(lines, "", "> GitHub has more inline review comments beyond the provider page fetched for this overview; additional threads may therefore exist upstream.")
	} else if pr.ReviewComments > availability.ReviewCommentsReturned {
		lines = append(lines, "", "> GitHub reports more inline review comments than were returned by the fetched provider page; this provider-incomplete state is separate from local overview omission.")
	}
	if enrichmentNote != "" {
		lines = append(lines, "", "> "+enrichmentNote)
	}

	base := pullBaseURL(target)
	lines = append(lines, "", "## Useful GitHub URLs", "", "- Full description: "+bodySelector, "- Files changed: "+base+"/files", "- Commits: "+base+"/commits", "- Checks: "+base+"/checks")
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func splitPullTimelineForOverview(timeline []githubTimelineEvent) ([]githubTimelineEvent, []githubTimelineEvent) {
	comments := make([]githubTimelineEvent, 0, len(timeline))
	events := make([]githubTimelineEvent, 0, len(timeline))
	for _, event := range timeline {
		if event.Event == "commented" {
			comments = append(comments, event)
			continue
		}
		if _, ok := renderPullTimelineState(event); ok {
			events = append(events, event)
		}
	}
	return comments, events
}

func renderPullConversationCommentIndex(target *GitHubTarget, comment githubIssueComment) []string {
	heading := fmt.Sprintf("### Comment `%d`", comment.ID)
	if comment.User.Login != "" {
		heading += " by @" + comment.User.Login
	}
	if comment.CreatedAt != "" {
		heading += " — " + comment.CreatedAt
	}
	body := strings.Join(renderIssueCommentBody(comment, comment.IsPinned), "\n")
	preview, truncated := githubOverviewPreview(body, githubIndexPreviewRunes)
	lines := []string{heading, ""}
	for _, line := range strings.Split(preview, "\n") {
		lines = append(lines, "> "+line)
	}
	if truncated {
		lines = append(lines, "> _Preview locally truncated._")
	}
	lines = append(lines, "", "Selector: "+pullIssueCommentSelectorURL(target, comment), "")
	return lines
}

func renderPullReviewIndex(target *GitHubTarget, review githubPullReview) []string {
	heading := fmt.Sprintf("### Review `%d`", review.ID)
	if review.State != "" {
		heading += " — " + strings.ToUpper(review.State)
	}
	if review.User.Login != "" {
		heading += " by @" + review.User.Login
	}
	if review.SubmittedAt != "" {
		heading += " — " + review.SubmittedAt
	}
	lines := []string{heading, ""}
	if review.Body != nil {
		body := strings.TrimSpace(stripInvisibleHTMLComments(*review.Body))
		if body != "" {
			preview, truncated := githubOverviewPreview(body, githubIndexPreviewRunes)
			for _, line := range strings.Split(preview, "\n") {
				lines = append(lines, "> "+line)
			}
			if truncated {
				lines = append(lines, "> _Preview locally truncated._")
			}
		}
	}
	lines = append(lines, "", "Selector: "+pullReviewSelectorURL(target, review), "")
	return lines
}

func renderPullThreadIndex(target *GitHubTarget, thread githubPullThread) []string {
	root := thread.Root
	heading := fmt.Sprintf("### Thread `%d`", root.ID)
	if root.User.Login != "" {
		heading += " by @" + root.User.Login
	}
	if root.CreatedAt != "" {
		heading += " — " + root.CreatedAt
	}
	lines := []string{heading}
	if coord := pullCommentCoordinate(root); coord != "" {
		lines = append(lines, "Coordinate: "+coord)
	}
	if thread.State != nil {
		state := "unresolved"
		if thread.State.Resolved {
			state = "resolved"
		}
		if thread.State.Outdated {
			state += " · outdated"
		}
		if thread.State.ResolvedBy != "" {
			state += " · by @" + thread.State.ResolvedBy
		}
		lines = append(lines, "State: "+state)
	}
	if len(thread.Replies) > 0 {
		lines = append(lines, fmt.Sprintf("Replies: %d", len(thread.Replies)))
	}
	lines = append(lines, "")
	body := "_Review comment body is unavailable or deleted._"
	if root.Body != nil {
		body = strings.TrimSpace(stripInvisibleHTMLComments(*root.Body))
		if body == "" {
			body = "_Review comment is empty after removing invisible GitHub markup._"
		}
	}
	preview, truncated := githubOverviewPreview(body, githubIndexPreviewRunes)
	for _, line := range strings.Split(preview, "\n") {
		lines = append(lines, "> "+line)
	}
	if truncated {
		lines = append(lines, "> _Preview locally truncated._")
	}
	lines = append(lines, "", "Selector: "+pullThreadSelectorURL(target, root), "")
	return lines
}

func pullBaseURL(target *GitHubTarget) string {
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), target.Number)
}

func pullIssueCommentSelectorURL(target *GitHubTarget, comment githubIssueComment) string {
	if strings.TrimSpace(comment.HTMLURL) != "" {
		return comment.HTMLURL
	}
	return pullBaseURL(target) + "#issuecomment-" + strconv.FormatInt(comment.ID, 10)
}

func pullReviewSelectorURL(target *GitHubTarget, review githubPullReview) string {
	if strings.TrimSpace(review.HTMLURL) != "" {
		return review.HTMLURL
	}
	return pullBaseURL(target) + "#pullrequestreview-" + strconv.FormatInt(review.ID, 10)
}

func pullThreadSelectorURL(target *GitHubTarget, comment githubPullReviewComment) string {
	if strings.TrimSpace(comment.HTMLURL) != "" {
		return comment.HTMLURL
	}
	return pullBaseURL(target) + "#discussion_r" + strconv.FormatInt(comment.ID, 10)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func pullDisplayState(pr githubPullRequest) string {
	if pr.Merged || pr.MergedAt != "" {
		return "merged"
	}
	if pr.Draft && pr.State == "open" {
		return "draft"
	}
	if pr.State != "" {
		return pr.State
	}
	return "unknown"
}

func renderPullReview(review githubPullReview) []string {
	heading := "### " + strings.ToUpper(review.State)
	if review.User.Login != "" {
		heading += " by @" + review.User.Login
	}
	if review.SubmittedAt != "" {
		heading += " — " + review.SubmittedAt
	}
	lines := []string{heading}
	meta := []string{}
	if review.ID != 0 {
		meta = append(meta, fmt.Sprintf("review %d", review.ID))
	}
	if review.CommitID != "" {
		meta = append(meta, "commit `"+shortSHA(review.CommitID)+"`")
	}
	if review.AuthorAssociation != "" {
		meta = append(meta, "association "+review.AuthorAssociation)
	}
	if review.HTMLURL != "" {
		meta = append(meta, review.HTMLURL)
	}
	if len(meta) > 0 {
		lines = append(lines, "", "_"+strings.Join(meta, " · ")+"_")
	}
	if review.Body != nil {
		body := strings.TrimSpace(stripInvisibleHTMLComments(*review.Body))
		if body != "" {
			lines = append(lines, "", body)
		}
	}
	lines = append(lines, "")
	return lines
}

func renderPullThread(thread githubPullThread, selectedID int64) []string {
	root := thread.Root
	coord := pullCommentCoordinate(root)
	heading := fmt.Sprintf("### Thread %d", root.ID)
	if coord != "" {
		heading += " — " + coord
	}
	if thread.State != nil {
		states := []string{}
		if thread.State.Resolved {
			states = append(states, "resolved")
		} else {
			states = append(states, "unresolved")
		}
		if thread.State.Outdated {
			states = append(states, "outdated")
		}
		if thread.State.ResolvedBy != "" {
			states = append(states, "by @"+thread.State.ResolvedBy)
		}
		heading += " · " + strings.Join(states, " · ")
	}
	lines := []string{heading, ""}
	lines = append(lines, renderPullReviewComment(root, selectedID == root.ID)...)
	for _, reply := range thread.Replies {
		lines = append(lines, "", "#### Reply", "")
		lines = append(lines, renderPullReviewComment(reply, selectedID == reply.ID)...)
	}
	lines = append(lines, "")
	return lines
}

func renderPullReviewComment(comment githubPullReviewComment, selected bool) []string {
	meta := []string{}
	if comment.User.Login != "" {
		meta = append(meta, "@"+comment.User.Login)
	}
	if comment.CreatedAt != "" {
		meta = append(meta, comment.CreatedAt)
	}
	if comment.AuthorAssociation != "" {
		meta = append(meta, comment.AuthorAssociation)
	}
	if selected {
		meta = append(meta, "selected")
	}
	if comment.HTMLURL != "" {
		meta = append(meta, comment.HTMLURL)
	}
	lines := []string{}
	if len(meta) > 0 {
		lines = append(lines, "_"+strings.Join(meta, " · ")+"_", "")
	}
	if comment.Body == nil {
		lines = append(lines, "_Review comment body is unavailable or deleted._")
	} else {
		body := strings.TrimSpace(stripInvisibleHTMLComments(*comment.Body))
		if body == "" {
			body = "_Review comment is empty after removing invisible GitHub markup._"
		}
		lines = append(lines, body)
	}
	return lines
}

func pullCommentCoordinate(comment githubPullReviewComment) string {
	if comment.Path == "" {
		return ""
	}
	line := comment.Line
	if line == nil {
		line = comment.OriginalLine
	}
	start := comment.StartLine
	if start == nil {
		start = comment.OriginalStartLine
	}
	coord := "`" + comment.Path + "`"
	if line != nil {
		if start != nil && *start != *line {
			coord += fmt.Sprintf(" lines %d-%d", *start, *line)
		} else {
			coord += fmt.Sprintf(" line %d", *line)
		}
	}
	side := comment.Side
	if side == "" {
		side = comment.OriginalSide
	}
	if side != "" {
		coord += " " + strings.ToLower(side)
	}
	return coord
}

func renderPullTimelineState(event githubTimelineEvent) (string, bool) {
	switch event.Event {
	case "commented":
		return "", false
	case "reviewed":
		// Reviews are rendered from the complete /pulls/{n}/reviews endpoint.
		return "", false
	case "mentioned", "subscribed", "unsubscribed", "copilot_work_started":
		return "", false
	}
	actor := event.Actor.Login
	if actor == "" {
		actor = event.User.Login
	}
	prefix := "- "
	if event.CreatedAt != "" {
		prefix += event.CreatedAt + " — "
	}
	if actor != "" {
		prefix += "@" + actor + " "
	}
	switch event.Event {
	case "committed":
		sha := event.SHA
		if sha == "" {
			sha = event.CommitID
		}
		detail := "added commit"
		if sha != "" {
			detail += " `" + shortSHA(sha) + "`"
		}
		if event.Message != "" {
			detail += " — " + firstLine(event.Message)
		}
		return prefix + detail + ".", true
	case "head_ref_force_pushed":
		detail := "force-pushed the head ref"
		if event.CommitID != "" {
			detail += " to `" + shortSHA(event.CommitID) + "`"
		}
		return prefix + detail + ".", true
	case "base_ref_changed":
		return prefix + "changed the base ref.", true
	case "ready_for_review":
		return prefix + "marked the Pull Request ready for review.", true
	case "review_requested", "review_request_removed":
		login := ""
		if event.RequestedReviewer != nil {
			login = event.RequestedReviewer.Login
		}
		team := ""
		if event.RequestedTeam != nil {
			team = event.RequestedTeam.Name
			if team == "" {
				team = event.RequestedTeam.Slug
			}
		}
		verb := "requested review"
		if event.Event == "review_request_removed" {
			verb = "removed review request"
		}
		if login != "" {
			return prefix + verb + " from @" + login + ".", true
		}
		if team != "" {
			return prefix + verb + " from team `" + team + "`.", true
		}
		return prefix + verb + ".", true
	case "renamed":
		if event.Rename != nil {
			return prefix + "renamed the Pull Request from `" + event.Rename.From + "` to `" + event.Rename.To + "`.", true
		}
		return prefix + "renamed the Pull Request.", true
	case "merged":
		return prefix + "merged the Pull Request.", true
	case "closed":
		return prefix + "closed the Pull Request.", true
	case "reopened":
		return prefix + "reopened the Pull Request.", true
	default:
		return renderIssueTimelineState(event)
	}
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

func renderGitHubPullThreadSelector(target *GitHubTarget, thread githubPullThread, selectedID int64) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("pull_request: %d", target.Number),
		fmt.Sprintf("selected_review_comment: %d", selectedID),
		"---",
		"",
		fmt.Sprintf("# Review thread on %s/%s#%d", target.Owner, target.Repo, target.Number),
		"",
	}
	lines = append(lines, renderPullThread(thread, selectedID)...)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubPullReviewSelector(target *GitHubTarget, review githubPullReview, threads []githubPullThread) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("pull_request: %d", target.Number),
		fmt.Sprintf("review_id: %d", review.ID),
		"state: " + yamlScalar(review.State),
	}
	if review.User.Login != "" {
		lines = append(lines, "reviewer: "+yamlScalar("@"+review.User.Login))
	}
	if review.SubmittedAt != "" {
		lines = append(lines, "submitted: "+yamlScalar(review.SubmittedAt))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# Review on %s/%s#%d", target.Owner, target.Repo, target.Number), "")
	lines = append(lines, renderPullReview(review)...)
	if len(threads) > 0 {
		lines = append(lines, "## Review comments", "")
		for _, thread := range threads {
			lines = append(lines, renderPullThread(thread, 0)...)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
