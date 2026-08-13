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

func readGitHubPullRequest(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target == nil || target.Number <= 0 {
		return "", fmt.Errorf("GitHub Pull Request URL is missing a number")
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

	timeline, err := fetchGitHubIssueTimeline(ctx, client, target)
	if err != nil {
		return "", err
	}
	reviews, err := fetchGitHubPullReviews(ctx, client, target)
	if err != nil {
		return "", err
	}
	comments, err := fetchGitHubPullReviewComments(ctx, client, target)
	if err != nil {
		return "", err
	}
	threads := groupGitHubPullThreads(comments)
	enrichmentNote := ""
	if client.hasToken() && len(threads) > 0 {
		states, enrichErr := fetchGitHubPullThreadStates(ctx, client, target)
		if enrichErr != nil {
			enrichmentNote = "Review-thread resolved/outdated enrichment was unavailable from GitHub GraphQL; REST thread content is still complete."
		} else {
			for i := range threads {
				if state, ok := states[threads[i].Root.ID]; ok {
					copyState := state
					threads[i].State = &copyState
				}
			}
		}
	}
	return renderGitHubPullRequest(target, pr, timeline, reviews, threads, enrichmentNote, client.hasToken()), nil
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

func renderGitHubPullRequest(target *GitHubTarget, pr githubPullRequest, timeline []githubTimelineEvent, reviews []githubPullReview, threads []githubPullThread, enrichmentNote string, hasToken bool) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("number: %d", pr.Number),
		"state: " + yamlScalar(pullDisplayState(pr)),
		"title: " + yamlScalar(pr.Title),
	}
	if pr.Draft {
		lines = append(lines, "draft: true")
	}
	if pr.User.Login != "" {
		lines = append(lines, "author: "+yamlScalar("@"+pr.User.Login))
	}
	if pr.Base.Label != "" {
		lines = append(lines, "base: "+yamlScalar(pr.Base.Label))
	}
	if pr.Head.Label != "" {
		lines = append(lines, "head: "+yamlScalar(pr.Head.Label))
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
		fmt.Sprintf("conversation_comments: %d", pr.Comments),
		fmt.Sprintf("review_comments: %d", pr.ReviewComments),
	)
	if pr.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(pr.HTMLURL))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# #%d %s", pr.Number, pr.Title), "", "## Body", "")
	if pr.Body == nil || strings.TrimSpace(stripInvisibleHTMLComments(*pr.Body)) == "" {
		lines = append(lines, "_No description provided._")
	} else {
		lines = append(lines, stripInvisibleHTMLComments(*pr.Body))
	}

	lines = append(lines, "", "## Conversation", "")
	visible := 0
	for _, event := range timeline {
		if event.Event == "commented" {
			comment := githubIssueComment{ID: event.ID, Body: event.Body, HTMLURL: event.HTMLURL, User: event.User, AuthorAssociation: event.AuthorAssociation, CreatedAt: event.CreatedAt, UpdatedAt: event.UpdatedAt, IsPinned: event.IsPinned, Minimized: event.Minimized, MinimizedReason: event.MinimizedReason}
			lines = append(lines, renderTimelineComment(comment)...)
			visible++
			continue
		}
		if rendered, ok := renderPullTimelineState(event); ok {
			lines = append(lines, rendered)
			visible++
		}
	}
	if visible == 0 {
		lines = append(lines, "_No substantive conversation timeline activity._")
	}

	lines = append(lines, "", "## Reviews", "")
	if len(reviews) == 0 {
		lines = append(lines, "_No submitted reviews._")
	}
	for _, review := range reviews {
		lines = append(lines, renderPullReview(review)...)
	}

	lines = append(lines, "", "## Inline review threads", "")
	if len(threads) == 0 {
		lines = append(lines, "_No inline review threads._")
	}
	for _, thread := range threads {
		lines = append(lines, renderPullThread(thread, 0)...)
	}
	if enrichmentNote != "" {
		lines = append(lines, "", "> "+enrichmentNote)
	} else if !hasToken && len(threads) > 0 {
		lines = append(lines, "", "> Optional: set GH_TOKEN or GITHUB_TOKEN to enrich review threads with GitHub resolved/outdated state.")
	}

	base := fmt.Sprintf("https://github.com/%s/%s/pull/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), target.Number)
	lines = append(lines, "", "## Useful GitHub URLs", "", "- Files changed: "+base+"/files", "- Commits: "+base+"/commits", "- Checks: "+base+"/checks")
	return strings.TrimSpace(strings.Join(lines, "\n"))
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
