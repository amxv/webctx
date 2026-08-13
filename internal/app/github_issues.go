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
	"time"
)

type githubUser struct {
	Login   string `json:"login"`
	HTMLURL string `json:"html_url"`
	Type    string `json:"type"`
}

type githubIssueLabel struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type githubIssueMilestone struct {
	Number       int        `json:"number"`
	State        string     `json:"state"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	HTMLURL      string     `json:"html_url"`
	Creator      githubUser `json:"creator"`
	OpenIssues   int        `json:"open_issues"`
	ClosedIssues int        `json:"closed_issues"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
	ClosedAt     string     `json:"closed_at"`
	DueOn        string     `json:"due_on"`
}

type githubIssueType struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type githubPullMarker struct {
	HTMLURL string `json:"html_url"`
}

type githubIssue struct {
	Number            int                   `json:"number"`
	State             string                `json:"state"`
	StateReason       string                `json:"state_reason"`
	Title             string                `json:"title"`
	Body              *string               `json:"body"`
	HTMLURL           string                `json:"html_url"`
	User              githubUser            `json:"user"`
	AuthorAssociation string                `json:"author_association"`
	Labels            []githubIssueLabel    `json:"labels"`
	Assignees         []githubUser          `json:"assignees"`
	Milestone         *githubIssueMilestone `json:"milestone"`
	Locked            bool                  `json:"locked"`
	ActiveLockReason  string                `json:"active_lock_reason"`
	Comments          int                   `json:"comments"`
	CreatedAt         string                `json:"created_at"`
	UpdatedAt         string                `json:"updated_at"`
	ClosedAt          string                `json:"closed_at"`
	PullRequest       *githubPullMarker     `json:"pull_request"`
	PinnedComment     json.RawMessage       `json:"pinned_comment"`
	Type              json.RawMessage       `json:"type"`
}

type githubIssueComment struct {
	ID                int64      `json:"id"`
	Body              *string    `json:"body"`
	HTMLURL           string     `json:"html_url"`
	IssueURL          string     `json:"issue_url"`
	User              githubUser `json:"user"`
	AuthorAssociation string     `json:"author_association"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
	IsPinned          bool       `json:"is_pinned"`
	Minimized         bool       `json:"minimized"`
	MinimizedReason   string     `json:"minimized_reason"`
}

type githubIssueFieldValue struct {
	IssueFieldName     string `json:"issue_field_name"`
	DataType           string `json:"data_type"`
	Value              any    `json:"value"`
	SingleSelectOption *struct {
		Name string `json:"name"`
	} `json:"single_select_option"`
	MultiSelectOptions []struct {
		Name string `json:"name"`
	} `json:"multi_select_options"`
}

type githubTimelineEvent struct {
	ID                int64                 `json:"id"`
	Event             string                `json:"event"`
	Actor             githubUser            `json:"actor"`
	User              githubUser            `json:"user"`
	Body              *string               `json:"body"`
	HTMLURL           string                `json:"html_url"`
	CreatedAt         string                `json:"created_at"`
	UpdatedAt         string                `json:"updated_at"`
	AuthorAssociation string                `json:"author_association"`
	Label             *githubIssueLabel     `json:"label"`
	Milestone         *githubIssueMilestone `json:"milestone"`
	Assignee          *githubUser           `json:"assignee"`
	CommitID          string                `json:"commit_id"`
	CommitURL         string                `json:"commit_url"`
	SHA               string                `json:"sha"`
	Message           string                `json:"message"`
	State             string                `json:"state"`
	SubmittedAt       string                `json:"submitted_at"`
	RequestedReviewer *githubUser           `json:"requested_reviewer"`
	ReviewRequester   *githubUser           `json:"review_requester"`
	RequestedTeam     *struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"requested_team"`
	LockReason string `json:"lock_reason"`
	Rename     *struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"rename"`
	Source *struct {
		Issue *githubIssue `json:"issue"`
	} `json:"source"`
	IsPinned        bool   `json:"is_pinned"`
	Minimized       bool   `json:"minimized"`
	MinimizedReason string `json:"minimized_reason"`
}

type githubIssueRelationships struct {
	Parent    *githubIssue
	SubIssues []githubIssue
	BlockedBy []githubIssue
	Blocking  []githubIssue
	Fields    []githubIssueFieldValue
}

type githubIssueSearchResponse struct {
	TotalCount        int           `json:"total_count"`
	IncompleteResults bool          `json:"incomplete_results"`
	Items             []githubIssue `json:"items"`
}

func readGitHubIssue(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if commentID, ok, err := parseIssueCommentSelector(target.Fragment); ok {
		if err != nil {
			return "", err
		}
		return readGitHubIssueComment(ctx, client, target, commentID)
	}
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Issue fragment %q is not a supported native selector", target.Fragment)
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var issue githubIssue
	if err := json.Unmarshal(resp.Body, &issue); err != nil {
		return "", fmt.Errorf("decode GitHub Issue: %w", err)
	}
	if issue.PullRequest != nil {
		canonical := issue.PullRequest.HTMLURL
		if canonical == "" {
			canonical = fmt.Sprintf("https://github.com/%s/%s/pull/%d", target.Owner, target.Repo, target.Number)
		}
		return "", fmt.Errorf("GitHub #%d is a pull request, not an Issue. Canonical URL: %s", target.Number, canonical)
	}

	timeline, err := fetchGitHubIssueTimeline(ctx, client, target)
	if err != nil {
		return "", err
	}
	relationships, err := fetchGitHubIssueRelationships(ctx, client, target)
	if err != nil {
		return "", err
	}
	return renderGitHubIssue(target, issue, timeline, relationships), nil
}

func parseIssueCommentSelector(fragment string) (int64, bool, error) {
	if !strings.HasPrefix(fragment, "issuecomment-") {
		return 0, false, nil
	}
	raw := strings.TrimPrefix(fragment, "issuecomment-")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, true, fmt.Errorf("invalid GitHub issue-comment selector %q", fragment)
	}
	return id, true, nil
}

func readGitHubIssueComment(ctx context.Context, client *GitHubClient, target *GitHubTarget, commentID int64) (string, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/issues/comments/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), commentID)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var comment githubIssueComment
	if err := json.Unmarshal(resp.Body, &comment); err != nil {
		return "", fmt.Errorf("decode GitHub Issue comment: %w", err)
	}
	wantSuffix := fmt.Sprintf("/repos/%s/%s/issues/%d", target.Owner, target.Repo, target.Number)
	if comment.IssueURL != "" && !strings.HasSuffix(strings.TrimRight(comment.IssueURL, "/"), wantSuffix) {
		return "", fmt.Errorf("GitHub comment %d does not belong to %s/%s#%d", commentID, target.Owner, target.Repo, target.Number)
	}
	return renderGitHubIssueComment(target, comment), nil
}

func fetchGitHubIssueTimeline(ctx context.Context, client *GitHubClient, target *GitHubTarget) ([]githubTimelineEvent, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/timeline?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	events := []githubTimelineEvent{}
	for _, page := range pages {
		var batch []githubTimelineEvent
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub Issue timeline: %w", err)
		}
		events = append(events, batch...)
	}
	return events, nil
}

func fetchGitHubIssueRelationships(ctx context.Context, client *GitHubClient, target *GitHubTarget) (githubIssueRelationships, error) {
	base := fmt.Sprintf("/repos/%s/%s/issues/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	out := githubIssueRelationships{}

	parentResp, err := client.REST(ctx, http.MethodGet, base+"/parent", "application/vnd.github+json")
	if err != nil {
		if !isOptionalGitHubRelationshipMiss(err) {
			return out, err
		}
	} else {
		var parent githubIssue
		if err := json.Unmarshal(parentResp.Body, &parent); err != nil {
			return out, fmt.Errorf("decode GitHub parent Issue: %w", err)
		}
		out.Parent = &parent
	}

	var errFetch error
	out.SubIssues, errFetch = fetchOptionalIssuePages(ctx, client, base+"/sub_issues?per_page=100")
	if errFetch != nil {
		return out, errFetch
	}
	out.BlockedBy, errFetch = fetchOptionalIssuePages(ctx, client, base+"/dependencies/blocked_by?per_page=100")
	if errFetch != nil {
		return out, errFetch
	}
	out.Blocking, errFetch = fetchOptionalIssuePages(ctx, client, base+"/dependencies/blocking?per_page=100")
	if errFetch != nil {
		return out, errFetch
	}
	out.Fields, errFetch = fetchOptionalIssueFieldPages(ctx, client, base+"/issue-field-values?per_page=100")
	if errFetch != nil {
		return out, errFetch
	}
	return out, nil
}

func isOptionalGitHubRelationshipMiss(err error) bool {
	var ghErr *GitHubError
	if !asGitHubError(err, &ghErr) {
		return false
	}
	return ghErr.Kind == GitHubErrorNotFound || ghErr.Kind == GitHubErrorGone
}

func fetchOptionalIssuePages(ctx context.Context, client *GitHubClient, endpoint string) ([]githubIssue, error) {
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		if isOptionalGitHubRelationshipMiss(err) {
			return nil, nil
		}
		return nil, err
	}
	out := []githubIssue{}
	for _, page := range pages {
		var batch []githubIssue
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub Issue relationship: %w", err)
		}
		out = append(out, batch...)
	}
	return out, nil
}

func fetchOptionalIssueFieldPages(ctx context.Context, client *GitHubClient, endpoint string) ([]githubIssueFieldValue, error) {
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		if isOptionalGitHubRelationshipMiss(err) {
			return nil, nil
		}
		return nil, err
	}
	out := []githubIssueFieldValue{}
	for _, page := range pages {
		var batch []githubIssueFieldValue
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub Issue field values: %w", err)
		}
		out = append(out, batch...)
	}
	return out, nil
}

func renderGitHubIssue(target *GitHubTarget, issue githubIssue, timeline []githubTimelineEvent, rel githubIssueRelationships) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("number: %d", issue.Number),
		"state: " + yamlScalar(issue.State),
		"title: " + yamlScalar(issue.Title),
	}
	if issue.StateReason != "" {
		lines = append(lines, "state_reason: "+yamlScalar(issue.StateReason))
	}
	if issue.User.Login != "" {
		lines = append(lines, "author: "+yamlScalar("@"+issue.User.Login))
	}
	if issue.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(issue.CreatedAt))
	}
	if issue.UpdatedAt != "" {
		lines = append(lines, "updated: "+yamlScalar(issue.UpdatedAt))
	}
	if issue.ClosedAt != "" {
		lines = append(lines, "closed: "+yamlScalar(issue.ClosedAt))
	}
	if issue.Locked {
		lines = append(lines, "locked: true")
		if issue.ActiveLockReason != "" {
			lines = append(lines, "lock_reason: "+yamlScalar(issue.ActiveLockReason))
		}
	}
	if labels := issueLabelNames(issue.Labels); len(labels) > 0 {
		encoded, _ := json.Marshal(labels)
		lines = append(lines, "labels: "+string(encoded))
	}
	if assignees := githubUserLogins(issue.Assignees); len(assignees) > 0 {
		encoded, _ := json.Marshal(assignees)
		lines = append(lines, "assignees: "+string(encoded))
	}
	if issue.Milestone != nil && issue.Milestone.Title != "" {
		lines = append(lines, "milestone: "+yamlScalar(issue.Milestone.Title))
	}
	if issueType := githubIssueTypeName(issue.Type); issueType != "" {
		lines = append(lines, "type: "+yamlScalar(issueType))
	}
	lines = append(lines, fmt.Sprintf("comments: %d", issue.Comments))
	if issue.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(issue.HTMLURL))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# #%d %s", issue.Number, issue.Title), "", "## Body", "")
	if issue.Body == nil || strings.TrimSpace(stripInvisibleHTMLComments(*issue.Body)) == "" {
		lines = append(lines, "_No description provided._")
	} else {
		lines = append(lines, stripInvisibleHTMLComments(*issue.Body))
	}

	relLines := renderIssueRelationships(rel)
	if len(relLines) > 0 {
		lines = append(lines, "", "## Relationships", "")
		lines = append(lines, relLines...)
	}

	pinned := decodePinnedIssueComment(issue.PinnedComment)
	pinnedRenderedInTimeline := false
	if pinned != nil {
		for _, event := range timeline {
			if event.Event == "commented" && event.ID == pinned.ID {
				pinnedRenderedInTimeline = true
				break
			}
		}
	}
	if pinned != nil && !pinnedRenderedInTimeline {
		lines = append(lines, "", "## Pinned comment", "")
		lines = append(lines, renderIssueCommentBody(*pinned, true)...)
	}

	lines = append(lines, "", "## Timeline", "")
	visible := 0
	for _, event := range timeline {
		if event.Event == "commented" {
			comment := githubIssueComment{
				ID:                event.ID,
				Body:              event.Body,
				HTMLURL:           event.HTMLURL,
				User:              event.User,
				AuthorAssociation: event.AuthorAssociation,
				CreatedAt:         event.CreatedAt,
				UpdatedAt:         event.UpdatedAt,
				IsPinned:          event.IsPinned || (pinned != nil && pinned.ID == event.ID),
				Minimized:         event.Minimized,
				MinimizedReason:   event.MinimizedReason,
			}
			lines = append(lines, renderTimelineComment(comment)...)
			visible++
			continue
		}
		if rendered, ok := renderIssueTimelineState(event); ok {
			lines = append(lines, rendered)
			visible++
		}
	}
	if visible == 0 {
		lines = append(lines, "_No substantive timeline activity._")
	}

	base := fmt.Sprintf("https://github.com/%s/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo))
	lines = append(lines, "", "## Useful GitHub URLs", "", "- Issue list: "+base+"/issues")
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubIssueComment(target *GitHubTarget, comment githubIssueComment) string {
	identityKey := "issue"
	headingNoun := "Comment on"
	if target.Kind == GitHubTargetPull {
		identityKey = "pull_request"
		headingNoun = "Comment on Pull Request"
	}
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("%s: %d", identityKey, target.Number),
		fmt.Sprintf("comment_id: %d", comment.ID),
	}
	if comment.User.Login != "" {
		lines = append(lines, "author: "+yamlScalar("@"+comment.User.Login))
	}
	if comment.AuthorAssociation != "" {
		lines = append(lines, "association: "+yamlScalar(comment.AuthorAssociation))
	}
	if comment.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(comment.CreatedAt))
	}
	if comment.UpdatedAt != "" && comment.UpdatedAt != comment.CreatedAt {
		lines = append(lines, "updated: "+yamlScalar(comment.UpdatedAt))
	}
	if comment.IsPinned {
		lines = append(lines, "pinned: true")
	}
	if comment.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(comment.HTMLURL))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# %s %s/%s#%d", headingNoun, target.Owner, target.Repo, target.Number), "")
	lines = append(lines, renderIssueCommentBody(comment, false)...)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderTimelineComment(comment githubIssueComment) []string {
	heading := "### Comment"
	if comment.User.Login != "" {
		heading += " by @" + comment.User.Login
	}
	if comment.CreatedAt != "" {
		heading += " — " + comment.CreatedAt
	}
	if comment.IsPinned {
		heading += " · pinned"
	}
	lines := []string{heading, ""}
	lines = append(lines, renderIssueCommentBody(comment, comment.IsPinned)...)
	lines = append(lines, "")
	return lines
}

func renderIssueCommentBody(comment githubIssueComment, pinned bool) []string {
	if comment.Minimized {
		reason := strings.TrimSpace(comment.MinimizedReason)
		if reason == "" {
			reason = "reason not provided"
		}
		return []string{"_Comment minimized by GitHub (" + reason + ")._"}
	}
	if comment.Body == nil {
		return []string{"_Comment body is unavailable or deleted._"}
	}
	body := strings.TrimSpace(stripInvisibleHTMLComments(*comment.Body))
	if body == "" {
		body = "_Comment is empty after removing invisible GitHub markup._"
	}
	return []string{body}
}

func renderIssueTimelineState(event githubTimelineEvent) (string, bool) {
	name := strings.TrimSpace(event.Event)
	if name == "" || name == "commented" {
		return "", false
	}
	switch name {
	case "mentioned", "subscribed", "unsubscribed", "head_ref_deleted", "head_ref_restored":
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
	switch name {
	case "closed":
		return prefix + "closed the Issue.", true
	case "reopened":
		return prefix + "reopened the Issue.", true
	case "labeled", "unlabeled":
		label := ""
		if event.Label != nil {
			label = event.Label.Name
		}
		verb := "added"
		if name == "unlabeled" {
			verb = "removed"
		}
		return prefix + verb + " label `" + label + "`.", true
	case "milestoned", "demilestoned":
		title := ""
		if event.Milestone != nil {
			title = event.Milestone.Title
		}
		verb := "set milestone"
		if name == "demilestoned" {
			verb = "removed milestone"
		}
		return prefix + verb + " `" + title + "`.", true
	case "assigned", "unassigned":
		login := ""
		if event.Assignee != nil {
			login = event.Assignee.Login
		}
		verb := "assigned"
		if name == "unassigned" {
			verb = "unassigned"
		}
		if login != "" {
			return prefix + verb + " @" + login + ".", true
		}
		return prefix + verb + " the Issue.", true
	case "locked", "unlocked":
		detail := name + " the conversation"
		if name == "locked" && event.LockReason != "" {
			detail += " (" + event.LockReason + ")"
		}
		return prefix + detail + ".", true
	case "renamed":
		if event.Rename != nil {
			return prefix + "renamed the Issue from `" + event.Rename.From + "` to `" + event.Rename.To + "`.", true
		}
		return prefix + "renamed the Issue.", true
	case "cross-referenced":
		if event.Source != nil && event.Source.Issue != nil {
			source := event.Source.Issue
			label := fmt.Sprintf("#%d %s", source.Number, source.Title)
			if source.HTMLURL != "" {
				return prefix + "cross-referenced [" + escapeMarkdownLinkText(label) + "](" + source.HTMLURL + ").", true
			}
			return prefix + "cross-referenced " + label + ".", true
		}
		return prefix + "added a cross-reference.", true
	case "pinned":
		return prefix + "pinned a comment.", true
	case "unpinned":
		return prefix + "unpinned a comment.", true
	default:
		return prefix + strings.ReplaceAll(name, "_", " ") + ".", true
	}
}

func renderIssueRelationships(rel githubIssueRelationships) []string {
	lines := []string{}
	if rel.Parent != nil {
		lines = append(lines, "- Parent: "+issueRelationshipLink(*rel.Parent))
	}
	if len(rel.SubIssues) > 0 {
		lines = append(lines, "- Sub-issues: "+joinIssueRelationshipLinks(rel.SubIssues))
	}
	if len(rel.BlockedBy) > 0 {
		lines = append(lines, "- Blocked by: "+joinIssueRelationshipLinks(rel.BlockedBy))
	}
	if len(rel.Blocking) > 0 {
		lines = append(lines, "- Blocking: "+joinIssueRelationshipLinks(rel.Blocking))
	}
	for _, field := range rel.Fields {
		if strings.TrimSpace(field.IssueFieldName) == "" {
			continue
		}
		lines = append(lines, "- "+field.IssueFieldName+": "+renderIssueFieldValue(field))
	}
	return lines
}

func issueRelationshipLink(issue githubIssue) string {
	label := fmt.Sprintf("#%d %s", issue.Number, issue.Title)
	if issue.HTMLURL != "" {
		return "[" + escapeMarkdownLinkText(label) + "](" + issue.HTMLURL + ")"
	}
	return label
}

func joinIssueRelationshipLinks(issues []githubIssue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issueRelationshipLink(issue))
	}
	return strings.Join(parts, ", ")
}

func renderIssueFieldValue(field githubIssueFieldValue) string {
	if field.SingleSelectOption != nil && field.SingleSelectOption.Name != "" {
		return field.SingleSelectOption.Name
	}
	if len(field.MultiSelectOptions) > 0 {
		names := make([]string, 0, len(field.MultiSelectOptions))
		for _, option := range field.MultiSelectOptions {
			if option.Name != "" {
				names = append(names, option.Name)
			}
		}
		if len(names) > 0 {
			return strings.Join(names, ", ")
		}
	}
	switch value := field.Value.(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case nil:
		return "_empty_"
	default:
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
}

func decodePinnedIssueComment(raw json.RawMessage) *githubIssueComment {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return nil
	}
	var comment githubIssueComment
	if json.Unmarshal(raw, &comment) != nil || comment.ID == 0 {
		return nil
	}
	comment.IsPinned = true
	return &comment
}

func githubIssueTypeName(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return name
	}
	var issueType githubIssueType
	if json.Unmarshal(raw, &issueType) == nil {
		return issueType.Name
	}
	return ""
}

func issueLabelNames(labels []githubIssueLabel) []string {
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		if strings.TrimSpace(label.Name) != "" {
			names = append(names, label.Name)
		}
	}
	sort.Strings(names)
	return names
}

func githubUserLogins(users []githubUser) []string {
	logins := make([]string, 0, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.Login) != "" {
			logins = append(logins, "@"+user.Login)
		}
	}
	sort.Strings(logins)
	return logins
}

func readGitHubIssueList(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Issue-list fragment %q is not a supported native selector", target.Fragment)
	}
	if strings.TrimSpace(target.Query.Get("q")) != "" {
		return readGitHubIssueSearch(ctx, client, target)
	}
	query := copySelectedQuery(target.Query, []string{"milestone", "state", "assignee", "type", "creator", "mentioned", "labels", "sort", "direction", "since", "per_page", "page"})
	if query.Get("per_page") == "" {
		query.Set("per_page", "30")
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/issues?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var issues []githubIssue
	if err := json.Unmarshal(resp.Body, &issues); err != nil {
		return "", fmt.Errorf("decode GitHub Issue list: %w", err)
	}
	issues = filterPullRequestsFromIssues(issues)
	return renderGitHubIssueList(target, issues, resp.Links(), -1, false), nil
}

func readGitHubIssueSearch(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	q := strings.TrimSpace(target.Query.Get("q"))
	if !containsGitHubQualifier(q, "repo:") {
		q = "repo:" + target.Owner + "/" + target.Repo + " " + q
	}
	if !containsGitHubQualifier(q, "is:issue") {
		q += " is:issue"
	}
	apiQuery := url.Values{"q": []string{strings.TrimSpace(q)}}
	for _, key := range []string{"sort", "order", "per_page", "page"} {
		if value := target.Query.Get(key); value != "" {
			apiQuery.Set(key, value)
		}
	}
	if apiQuery.Get("per_page") == "" {
		apiQuery.Set("per_page", "30")
	}
	endpoint := "/search/issues?" + apiQuery.Encode()
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var result githubIssueSearchResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return "", fmt.Errorf("decode GitHub Issue search: %w", err)
	}
	result.Items = filterPullRequestsFromIssues(result.Items)
	return renderGitHubIssueList(target, result.Items, resp.Links(), result.TotalCount, result.IncompleteResults), nil
}

func containsGitHubQualifier(query, qualifier string) bool {
	return strings.Contains(strings.ToLower(query), strings.ToLower(qualifier))
}

func filterPullRequestsFromIssues(issues []githubIssue) []githubIssue {
	out := make([]githubIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.PullRequest == nil {
			out = append(out, issue)
		}
	}
	return out
}

func renderGitHubIssueList(target *GitHubTarget, issues []githubIssue, links GitHubLinkRelations, total int, incomplete bool) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"view: issues",
		fmt.Sprintf("results: %d", len(issues)),
	}
	if q := target.Query.Get("q"); q != "" {
		lines = append(lines, "query: "+yamlScalar(q))
	}
	if total >= 0 {
		lines = append(lines, fmt.Sprintf("total_matches: %d", total))
	}
	if incomplete {
		lines = append(lines, "complete: false")
	}
	if page := target.Query.Get("page"); page != "" {
		lines = append(lines, "page: "+yamlScalar(page))
	}
	lines = append(lines, "---", "", "# Issues", "")
	if incomplete {
		lines = append(lines, "> GitHub marked this search result set as incomplete.", "")
	}
	if len(issues) == 0 {
		lines = append(lines, "_No Issues on this page._")
	}
	for _, issue := range issues {
		urlText := issue.HTMLURL
		if urlText == "" {
			urlText = fmt.Sprintf("https://github.com/%s/%s/issues/%d", target.Owner, target.Repo, issue.Number)
		}
		meta := []string{issue.State}
		if issue.User.Login != "" {
			meta = append(meta, "@"+issue.User.Login)
		}
		if issue.UpdatedAt != "" {
			meta = append(meta, "updated "+issue.UpdatedAt)
		}
		if labels := issueLabelNames(issue.Labels); len(labels) > 0 {
			meta = append(meta, "labels: "+strings.Join(labels, ", "))
		}
		lines = append(lines, fmt.Sprintf("- [#%d %s](%s) — %s", issue.Number, escapeMarkdownLinkText(issue.Title), urlText, strings.Join(meta, " · ")))
	}
	if nav := renderGitHubUIPageNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "", "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubLabelList(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub label-list fragment %q is not supported", target.Fragment)
	}
	query := copySelectedQuery(target.Query, []string{"per_page", "page"})
	if query.Get("per_page") == "" {
		query.Set("per_page", "30")
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/labels?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var labels []githubIssueLabel
	if err := json.Unmarshal(resp.Body, &labels); err != nil {
		return "", fmt.Errorf("decode GitHub labels: %w", err)
	}
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "view: labels", fmt.Sprintf("results: %d", len(labels)), "---", "", "# Labels", ""}
	for _, label := range labels {
		labelURL := fmt.Sprintf("https://github.com/%s/%s/labels/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), url.PathEscape(label.Name))
		line := "- [" + escapeMarkdownLinkText(label.Name) + "](" + labelURL + ")"
		if label.Description != "" {
			line += " — " + label.Description
		}
		lines = append(lines, line)
	}
	if len(labels) == 0 {
		lines = append(lines, "_No labels on this page._")
	}
	if nav := renderGitHubUIPageNavigation(target, resp.Links()); len(nav) > 0 {
		lines = append(lines, "", "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func readGitHubLabel(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub label fragment %q is not supported", target.Fragment)
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/labels/%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(target.Name))
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var label githubIssueLabel
	if err := json.Unmarshal(resp.Body, &label); err != nil {
		return "", fmt.Errorf("decode GitHub label: %w", err)
	}
	listQuery := url.Values{"labels": []string{target.Name}, "state": []string{"all"}, "per_page": []string{"30"}}
	if page := target.Query.Get("page"); page != "" {
		listQuery.Set("page", page)
	}
	listEndpoint := fmt.Sprintf("/repos/%s/%s/issues?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), listQuery.Encode())
	listResp, err := client.REST(ctx, http.MethodGet, listEndpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var issues []githubIssue
	if err := json.Unmarshal(listResp.Body, &issues); err != nil {
		return "", fmt.Errorf("decode GitHub label Issues: %w", err)
	}
	issues = filterPullRequestsFromIssues(issues)
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "label: " + yamlScalar(label.Name)}
	if label.Color != "" {
		lines = append(lines, "color: "+yamlScalar("#"+label.Color))
	}
	if label.Description != "" {
		lines = append(lines, "description: "+yamlScalar(label.Description))
	}
	lines = append(lines, fmt.Sprintf("issues_on_page: %d", len(issues)), "---", "", "# Label: "+label.Name, "")
	lines = append(lines, renderIssueLinksCompact(issues)...)
	filteredURL := fmt.Sprintf("https://github.com/%s/%s/issues?q=%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), url.QueryEscape(`is:issue label:"`+label.Name+`"`))
	lines = append(lines, "", "## Useful GitHub URLs", "", "- Filtered Issues: "+filteredURL)
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func readGitHubMilestones(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub milestone-list fragment %q is not supported", target.Fragment)
	}
	query := copySelectedQuery(target.Query, []string{"state", "sort", "direction", "per_page", "page"})
	if query.Get("per_page") == "" {
		query.Set("per_page", "30")
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/milestones?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var milestones []githubIssueMilestone
	if err := json.Unmarshal(resp.Body, &milestones); err != nil {
		return "", fmt.Errorf("decode GitHub milestones: %w", err)
	}
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "view: milestones", fmt.Sprintf("results: %d", len(milestones)), "---", "", "# Milestones", ""}
	if len(milestones) == 0 {
		lines = append(lines, "_No milestones on this page._")
	}
	for _, milestone := range milestones {
		href := fmt.Sprintf("https://github.com/%s/%s/milestone/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), milestone.Number)
		meta := fmt.Sprintf("%s · %d open / %d closed", milestone.State, milestone.OpenIssues, milestone.ClosedIssues)
		if milestone.DueOn != "" {
			meta += " · due " + milestone.DueOn
		}
		lines = append(lines, fmt.Sprintf("- [%s](%s) — %s", escapeMarkdownLinkText(milestone.Title), href, meta))
	}
	if nav := renderGitHubUIPageNavigation(target, resp.Links()); len(nav) > 0 {
		lines = append(lines, "", "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func readGitHubMilestone(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub milestone fragment %q is not supported", target.Fragment)
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/milestones/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var milestone githubIssueMilestone
	if err := json.Unmarshal(resp.Body, &milestone); err != nil {
		return "", fmt.Errorf("decode GitHub milestone: %w", err)
	}
	listQuery := url.Values{"milestone": []string{strconv.Itoa(target.Number)}, "state": []string{"all"}, "per_page": []string{"30"}}
	if page := target.Query.Get("page"); page != "" {
		listQuery.Set("page", page)
	}
	listEndpoint := fmt.Sprintf("/repos/%s/%s/issues?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), listQuery.Encode())
	listResp, err := client.REST(ctx, http.MethodGet, listEndpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var issues []githubIssue
	if err := json.Unmarshal(listResp.Body, &issues); err != nil {
		return "", fmt.Errorf("decode GitHub milestone Issues: %w", err)
	}
	issues = filterPullRequestsFromIssues(issues)
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("milestone: %d", milestone.Number),
		"title: " + yamlScalar(milestone.Title),
		"state: " + yamlScalar(milestone.State),
		fmt.Sprintf("open_issues: %d", milestone.OpenIssues),
		fmt.Sprintf("closed_issues: %d", milestone.ClosedIssues),
	}
	if milestone.DueOn != "" {
		lines = append(lines, "due: "+yamlScalar(milestone.DueOn))
	}
	lines = append(lines, "---", "", "# Milestone: "+milestone.Title, "")
	if strings.TrimSpace(milestone.Description) != "" {
		lines = append(lines, stripInvisibleHTMLComments(milestone.Description), "")
	}
	lines = append(lines, "## Issues", "")
	lines = append(lines, renderIssueLinksCompact(issues)...)
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func renderIssueLinksCompact(issues []githubIssue) []string {
	if len(issues) == 0 {
		return []string{"_No Issues on this page._"}
	}
	lines := make([]string, 0, len(issues))
	for _, issue := range issues {
		href := issue.HTMLURL
		if href == "" {
			href = "#"
		}
		lines = append(lines, fmt.Sprintf("- [#%d %s](%s) — %s", issue.Number, escapeMarkdownLinkText(issue.Title), href, issue.State))
	}
	return lines
}

func copySelectedQuery(source url.Values, keys []string) url.Values {
	out := url.Values{}
	for _, key := range keys {
		for _, value := range source[key] {
			out.Add(key, value)
		}
	}
	return out
}

func renderGitHubUIPageNavigation(target *GitHubTarget, links GitHubLinkRelations) []string {
	lines := []string{}
	if page, ok := pageFromGitHubLink(links["prev"]); ok {
		lines = append(lines, "- Previous: "+githubTargetPageURL(target, page))
	}
	if page, ok := pageFromGitHubLink(links["next"]); ok {
		lines = append(lines, "- Next: "+githubTargetPageURL(target, page))
	}
	return lines
}

func pageFromGitHubLink(raw string) (int, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return 0, false
	}
	page, err := strconv.Atoi(parsed.Query().Get("page"))
	if err != nil || page <= 0 {
		return 0, false
	}
	return page, true
}

func githubTargetPageURL(target *GitHubTarget, page int) string {
	parsed, err := url.Parse(target.OriginalURL)
	if err != nil {
		return target.OriginalURL
	}
	query := parsed.Query()
	query.Set("page", strconv.Itoa(page))
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func parseGitHubTime(raw string) time.Time {
	t, _ := time.Parse(time.RFC3339, raw)
	return t
}
