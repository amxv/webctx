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
	ID                int64                 `json:"id"`
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
	Draft             bool                  `json:"draft"`
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

func (comment *githubIssueComment) UnmarshalJSON(data []byte) error {
	type alias githubIssueComment
	aux := struct {
		Minimized       json.RawMessage `json:"minimized"`
		MinimizedReason json.RawMessage `json:"minimized_reason"`
		*alias
	}{alias: (*alias)(comment)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	comment.Minimized, comment.MinimizedReason = decodeGitHubMinimized(aux.Minimized, aux.MinimizedReason)
	return nil
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

func (event *githubTimelineEvent) UnmarshalJSON(data []byte) error {
	type alias githubTimelineEvent
	aux := struct {
		Minimized       json.RawMessage `json:"minimized"`
		MinimizedReason json.RawMessage `json:"minimized_reason"`
		*alias
	}{alias: (*alias)(event)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	event.Minimized, event.MinimizedReason = decodeGitHubMinimized(aux.Minimized, aux.MinimizedReason)
	return nil
}

func decodeGitHubMinimized(raw, reasonRaw json.RawMessage) (bool, string) {
	minimized := false
	reason := ""
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "", trimmed == "null":
	case trimmed == "true":
		minimized = true
	case trimmed == "false":
	default:
		var object struct {
			Reason json.RawMessage `json:"reason"`
		}
		if json.Unmarshal(raw, &object) == nil && len(object.Reason) > 0 {
			minimized = true
			_ = json.Unmarshal(object.Reason, &reason)
		}
	}
	if strings.TrimSpace(reason) == "" {
		_ = json.Unmarshal(reasonRaw, &reason)
	}
	return minimized, strings.TrimSpace(reason)
}

type githubIssueRelationships struct {
	Parent                *githubIssue
	SubIssues             []githubIssue
	BlockedBy             []githubIssue
	Blocking              []githubIssue
	Fields                []githubIssueFieldValue
	SubIssuesProviderMore bool
	BlockedByProviderMore bool
	BlockingProviderMore  bool
	FieldsProviderMore    bool
}

type githubIssueAvailability struct {
	TimelineProviderMore bool
}

func (r githubIssueRelationships) providerMore() bool {
	return r.SubIssuesProviderMore || r.BlockedByProviderMore || r.BlockingProviderMore || r.FieldsProviderMore
}

type githubIssueSearchResponse struct {
	TotalCount        int           `json:"total_count"`
	IncompleteResults bool          `json:"incomplete_results"`
	Items             []githubIssue `json:"items"`
}

func readGitHubIssue(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if issueID, ok, err := parseIssueBodySelector(target.Fragment); ok {
		if err != nil {
			return "", err
		}
		return readGitHubIssueBody(ctx, client, target, issueID)
	}
	if commentID, ok, err := parseIssueCommentSelector(target.Fragment); ok {
		if err != nil {
			return "", err
		}
		return readGitHubIssueComment(ctx, client, target, commentID)
	}
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Issue fragment %q is not a supported native selector", target.Fragment)
	}

	issue, err := fetchGitHubIssue(ctx, client, target)
	if err != nil {
		return "", err
	}
	if issue.PullRequest != nil {
		canonical := issue.PullRequest.HTMLURL
		if canonical == "" {
			canonical = fmt.Sprintf("https://github.com/%s/%s/pull/%d", target.Owner, target.Repo, target.Number)
		}
		return "", fmt.Errorf("GitHub #%d is a pull request, not an Issue. Canonical URL: %s", target.Number, canonical)
	}

	timeline, timelineMore, err := fetchGitHubIssueTimelinePage(ctx, client, target)
	if err != nil {
		return "", err
	}
	relationships, err := fetchGitHubIssueRelationships(ctx, client, target)
	if err != nil {
		return "", err
	}
	return renderGitHubIssue(target, issue, timeline, relationships, githubIssueAvailability{TimelineProviderMore: timelineMore}), nil
}

func fetchGitHubIssue(ctx context.Context, client *GitHubClient, target *GitHubTarget) (githubIssue, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return githubIssue{}, err
	}
	var issue githubIssue
	if err := json.Unmarshal(resp.Body, &issue); err != nil {
		return githubIssue{}, fmt.Errorf("decode GitHub Issue: %w", err)
	}
	return issue, nil
}

func parseIssueBodySelector(fragment string) (int64, bool, error) {
	if !strings.HasPrefix(fragment, "issue-") || strings.HasPrefix(fragment, "issuecomment-") {
		return 0, false, nil
	}
	raw := strings.TrimPrefix(fragment, "issue-")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, true, fmt.Errorf("invalid GitHub Issue-body selector %q", fragment)
	}
	return id, true, nil
}

func readGitHubIssueBody(ctx context.Context, client *GitHubClient, target *GitHubTarget, issueID int64) (string, error) {
	issue, err := fetchGitHubIssue(ctx, client, target)
	if err != nil {
		return "", err
	}
	if issue.PullRequest != nil {
		canonical := issue.PullRequest.HTMLURL
		if canonical == "" {
			canonical = fmt.Sprintf("https://github.com/%s/%s/pull/%d", target.Owner, target.Repo, target.Number)
		}
		return "", fmt.Errorf("GitHub #%d is a pull request, not an Issue. Canonical URL: %s", target.Number, canonical)
	}
	if issue.ID != issueID {
		return "", fmt.Errorf("GitHub Issue body selector issue-%d does not belong to %s/%s#%d (Issue id %d)", issueID, target.Owner, target.Repo, target.Number, issue.ID)
	}
	return renderGitHubIssueBody(target, issue), nil
}

func renderGitHubIssueBody(target *GitHubTarget, issue githubIssue) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("issue: %d", issue.Number),
		fmt.Sprintf("issue_id: %d", issue.ID),
		"title: " + yamlScalar(issue.Title),
	}
	if issue.User.Login != "" {
		lines = append(lines, "author: "+yamlScalar("@"+issue.User.Login))
	}
	if issue.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(issue.HTMLURL+"#issue-"+strconv.FormatInt(issue.ID, 10)))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# Description of %s/%s#%d", target.Owner, target.Repo, issue.Number), "")
	if issue.Body == nil || strings.TrimSpace(stripInvisibleHTMLComments(*issue.Body)) == "" {
		lines = append(lines, "_No description provided._")
	} else {
		lines = append(lines, stripInvisibleHTMLComments(*issue.Body))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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

func fetchGitHubIssueTimelinePage(ctx context.Context, client *GitHubClient, target *GitHubTarget) ([]githubTimelineEvent, bool, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/timeline?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), target.Number)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, false, err
	}
	var events []githubTimelineEvent
	if err := json.Unmarshal(resp.Body, &events); err != nil {
		return nil, false, fmt.Errorf("decode GitHub Issue timeline: %w", err)
	}
	return events, strings.TrimSpace(resp.Links()["next"]) != "", nil
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
	out.SubIssues, out.SubIssuesProviderMore, errFetch = fetchOptionalIssuePage(ctx, client, base+"/sub_issues?per_page=100")
	if errFetch != nil {
		return out, errFetch
	}
	out.BlockedBy, out.BlockedByProviderMore, errFetch = fetchOptionalIssuePage(ctx, client, base+"/dependencies/blocked_by?per_page=100")
	if errFetch != nil {
		return out, errFetch
	}
	out.Blocking, out.BlockingProviderMore, errFetch = fetchOptionalIssuePage(ctx, client, base+"/dependencies/blocking?per_page=100")
	if errFetch != nil {
		return out, errFetch
	}
	out.Fields, out.FieldsProviderMore, errFetch = fetchOptionalIssueFieldPage(ctx, client, base+"/issue-field-values?per_page=100")
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

func fetchOptionalIssuePage(ctx context.Context, client *GitHubClient, endpoint string) ([]githubIssue, bool, error) {
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		if isOptionalGitHubRelationshipMiss(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var out []githubIssue
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, false, fmt.Errorf("decode GitHub Issue relationship: %w", err)
	}
	return out, strings.TrimSpace(resp.Links()["next"]) != "", nil
}

func fetchOptionalIssueFieldPage(ctx context.Context, client *GitHubClient, endpoint string) ([]githubIssueFieldValue, bool, error) {
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		if isOptionalGitHubRelationshipMiss(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var out []githubIssueFieldValue
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, false, fmt.Errorf("decode GitHub Issue field values: %w", err)
	}
	return out, strings.TrimSpace(resp.Links()["next"]) != "", nil
}

func renderGitHubIssue(target *GitHubTarget, issue githubIssue, timeline []githubTimelineEvent, rel githubIssueRelationships, availability githubIssueAvailability) string {
	complete := renderGitHubIssueComplete(target, issue, timeline, rel)
	if !availability.TimelineProviderMore && !rel.providerMore() && githubOverviewFits(complete) {
		return complete
	}
	return renderGitHubIssueOverview(target, issue, timeline, rel, availability)
}

func renderGitHubIssueComplete(target *GitHubTarget, issue githubIssue, timeline []githubTimelineEvent, rel githubIssueRelationships) string {
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

func renderGitHubIssueOverview(target *GitHubTarget, issue githubIssue, timeline []githubTimelineEvent, rel githubIssueRelationships, availability githubIssueAvailability) string {
	visible := substantiveIssueTimeline(timeline)
	maxIndexed := len(visible)
	if maxIndexed > 10 {
		maxIndexed = 10
	}
	for indexed := maxIndexed; indexed >= 0; indexed-- {
		out := renderGitHubIssueOverviewWithLimit(target, issue, timeline, visible, rel, availability, indexed)
		if githubOverviewFits(out) {
			return out
		}
	}
	// Mandatory metadata/navigation win over the soft target if an extreme
	// provider title/relationship value alone consumes the whole budget.
	return renderGitHubIssueOverviewWithLimit(target, issue, timeline, visible, rel, availability, 0)
}

func renderGitHubIssueOverviewWithLimit(target *GitHubTarget, issue githubIssue, timeline, visible []githubTimelineEvent, rel githubIssueRelationships, availability githubIssueAvailability, indexedLimit int) string {
	if indexedLimit < 0 {
		indexedLimit = 0
	}
	if indexedLimit > len(visible) {
		indexedLimit = len(visible)
	}
	commentReturned := 0
	for _, event := range timeline {
		if event.Event == "commented" {
			commentReturned++
		}
	}
	commentIndexed := 0
	for _, event := range visible[:indexedLimit] {
		if event.Event == "commented" {
			commentIndexed++
		}
	}

	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("number: %d", issue.Number),
	}
	if issue.ID > 0 {
		lines = append(lines, fmt.Sprintf("issue_id: %d", issue.ID))
	}
	titlePreview, titleTruncated := githubOverviewInlinePreview(issue.Title, 180)
	if titleTruncated {
		titlePreview += "…"
	}
	lines = append(lines,
		"state: "+yamlScalar(issue.State),
		"title: "+yamlScalar(titlePreview),
		"overview: true",
	)
	if titleTruncated {
		lines = append(lines, "title_preview_truncated: true")
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
		shown := labels[:minInt(5, len(labels))]
		encoded, _ := json.Marshal(shown)
		lines = append(lines, "labels: "+string(encoded))
		if len(labels) > len(shown) {
			lines = append(lines, fmt.Sprintf("labels_local_omitted: %d", len(labels)-len(shown)))
		}
	}
	if assignees := githubUserLogins(issue.Assignees); len(assignees) > 0 {
		shown := assignees[:minInt(5, len(assignees))]
		encoded, _ := json.Marshal(shown)
		lines = append(lines, "assignees: "+string(encoded))
		if len(assignees) > len(shown) {
			lines = append(lines, fmt.Sprintf("assignees_local_omitted: %d", len(assignees)-len(shown)))
		}
	}
	if issue.Milestone != nil && issue.Milestone.Title != "" {
		lines = append(lines, "milestone: "+yamlScalar(issue.Milestone.Title))
	}
	if issueType := githubIssueTypeName(issue.Type); issueType != "" {
		lines = append(lines, "type: "+yamlScalar(issueType))
	}
	lines = append(lines,
		fmt.Sprintf("comments_reported: %d", issue.Comments),
		fmt.Sprintf("comments_returned: %d", commentReturned),
		fmt.Sprintf("comments_indexed: %d", commentIndexed),
		fmt.Sprintf("timeline_items_returned: %d", len(visible)),
		fmt.Sprintf("timeline_items_indexed: %d", indexedLimit),
		fmt.Sprintf("timeline_items_omitted: %d", len(visible)-indexedLimit),
	)
	if availability.TimelineProviderMore || issue.Comments > commentReturned {
		lines = append(lines, "comments_provider_complete: false")
	}
	if availability.TimelineProviderMore {
		lines = append(lines, "timeline_provider_more_available: true")
	}
	if rel.providerMore() {
		lines = append(lines, "relationships_provider_more_available: true")
	}
	if issue.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(issue.HTMLURL))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# #%d %s", issue.Number, titlePreview), "", "> Large Issue overview: subordinate conversation text is previewed/indexed so the default read stays near 5,000 characters.")

	bodySelector := issueBodySelectorURL(target, issue)
	lines = append(lines, "", "## Body preview", "")
	if issue.Body == nil || strings.TrimSpace(stripInvisibleHTMLComments(*issue.Body)) == "" {
		lines = append(lines, "_No description provided._")
	} else {
		body := stripInvisibleHTMLComments(*issue.Body)
		preview, truncated := githubOverviewPreview(body, 1200)
		lines = append(lines, preview)
		if truncated {
			lines = append(lines, "", "> Description preview locally truncated for this overview.")
		}
	}
	if bodySelector != "" {
		lines = append(lines, "> Full description: "+bodySelector)
	}

	relLines, relOmitted := renderIssueRelationshipsOverview(rel)
	if len(relLines) > 0 {
		lines = append(lines, "", "## Relationships", "")
		lines = append(lines, relLines...)
		if note := githubLocalOmissionNote("relationship entries", relOmitted); note != "" {
			lines = append(lines, "", note)
		}
	}

	pinned := decodePinnedIssueComment(issue.PinnedComment)
	pinnedInTimeline := false
	if pinned != nil {
		for _, event := range timeline {
			if event.Event == "commented" && event.ID == pinned.ID {
				pinnedInTimeline = true
				break
			}
		}
	}
	if pinned != nil && !pinnedInTimeline {
		lines = append(lines, "", "## Pinned comment", "")
		lines = append(lines, renderIssueCommentIndex(target, *pinned)...)
	}

	lines = append(lines, "", "## Timeline index", "")
	if len(visible) == 0 {
		lines = append(lines, "_No substantive timeline activity._")
	}
	for _, event := range visible[:indexedLimit] {
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
			lines = append(lines, renderIssueCommentIndex(target, comment)...)
			continue
		}
		if rendered, ok := renderIssueTimelineState(event); ok {
			lines = append(lines, rendered)
		}
	}
	if note := githubLocalOmissionNote("substantive timeline items", len(visible)-indexedLimit); note != "" {
		lines = append(lines, "", note)
	}
	if availability.TimelineProviderMore || issue.Comments > commentReturned {
		lines = append(lines, "", fmt.Sprintf("> Provider-incomplete comment data: GitHub reports %d comments, while the bounded timeline page returned %d comment events. More timeline pages may exist upstream; this is separate from local overview omission.", issue.Comments, commentReturned))
	}
	if rel.providerMore() {
		lines = append(lines, "", "> More Issue relationship entries exist upstream beyond the first provider pages fetched for this overview; local relationship omission is reported separately.")
	}

	base := fmt.Sprintf("https://github.com/%s/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo))
	lines = append(lines, "", "## Useful GitHub URLs", "")
	if bodySelector != "" {
		lines = append(lines, "- Full Issue description: "+bodySelector)
	}
	lines = append(lines, "- Issue page: "+issuePageURL(target, issue), "- Issue list: "+base+"/issues")
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func substantiveIssueTimeline(timeline []githubTimelineEvent) []githubTimelineEvent {
	visible := make([]githubTimelineEvent, 0, len(timeline))
	for _, event := range timeline {
		if event.Event == "commented" {
			visible = append(visible, event)
			continue
		}
		if _, ok := renderIssueTimelineState(event); ok {
			visible = append(visible, event)
		}
	}
	return visible
}

func renderIssueRelationshipsOverview(rel githubIssueRelationships) ([]string, int) {
	lines := []string{}
	omitted := 0
	if rel.Parent != nil {
		lines = append(lines, "- Parent: "+issueRelationshipLink(*rel.Parent))
	}
	appendIssues := func(label string, issues []githubIssue) {
		limit := len(issues)
		if limit > 3 {
			limit = 3
		}
		for _, issue := range issues[:limit] {
			lines = append(lines, "- "+label+": "+issueRelationshipLink(issue))
		}
		omitted += len(issues) - limit
	}
	appendIssues("Sub-issue", rel.SubIssues)
	appendIssues("Blocked by", rel.BlockedBy)
	appendIssues("Blocking", rel.Blocking)
	fieldLimit := len(rel.Fields)
	if fieldLimit > 4 {
		fieldLimit = 4
	}
	for _, field := range rel.Fields[:fieldLimit] {
		if strings.TrimSpace(field.IssueFieldName) == "" {
			continue
		}
		fieldName, fieldNameTruncated := githubOverviewInlinePreview(field.IssueFieldName, 80)
		if fieldNameTruncated {
			fieldName += "…"
		}
		fieldValue, fieldValueTruncated := githubOverviewInlinePreview(renderIssueFieldValue(field), 140)
		if fieldValueTruncated {
			fieldValue += "…"
		}
		lines = append(lines, "- "+fieldName+": "+fieldValue)
	}
	omitted += len(rel.Fields) - fieldLimit
	return lines, omitted
}

func renderIssueCommentIndex(target *GitHubTarget, comment githubIssueComment) []string {
	heading := fmt.Sprintf("### Comment `%d`", comment.ID)
	if comment.User.Login != "" {
		heading += " by @" + comment.User.Login
	}
	if comment.CreatedAt != "" {
		heading += " — " + comment.CreatedAt
	}
	if comment.IsPinned {
		heading += " · pinned"
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
	lines = append(lines, "", "Selector: "+issueCommentSelectorURL(target, comment), "")
	return lines
}

func issuePageURL(target *GitHubTarget, issue githubIssue) string {
	if strings.TrimSpace(issue.HTMLURL) != "" {
		return strings.TrimRight(issue.HTMLURL, "/")
	}
	return fmt.Sprintf("https://github.com/%s/%s/issues/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), target.Number)
}

func issueBodySelectorURL(target *GitHubTarget, issue githubIssue) string {
	if issue.ID <= 0 {
		return ""
	}
	return issuePageURL(target, issue) + "#issue-" + strconv.FormatInt(issue.ID, 10)
}

func issueCommentSelectorURL(target *GitHubTarget, comment githubIssueComment) string {
	if strings.TrimSpace(comment.HTMLURL) != "" {
		return comment.HTMLURL
	}
	return issuePageURL(target, githubIssue{Number: target.Number}) + "#issuecomment-" + strconv.FormatInt(comment.ID, 10)
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
	title, truncated := githubOverviewInlinePreview(issue.Title, 120)
	if truncated {
		title += "…"
	}
	label := fmt.Sprintf("#%d %s", issue.Number, title)
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
		resource, err := githubSearchResourceQualifier(target.Query.Get("q"))
		if err != nil {
			return "", err
		}
		if resource == "pull_request" {
			return readGitHubPullSearch(ctx, client, target)
		}
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
	resource, err := githubSearchResourceQualifier(q)
	if err != nil {
		return "", err
	}
	if resource == "pull_request" {
		return readGitHubPullSearch(ctx, client, target)
	}
	if !hasGitHubSearchQualifierPrefix(q, "repo:") {
		q = "repo:" + target.Owner + "/" + target.Repo + " " + q
	}
	if resource == "" {
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

func githubSearchResourceQualifier(query string) (string, error) {
	hasIssue := false
	hasPR := false
	for _, token := range githubSearchTokens(query) {
		switch strings.ToLower(token) {
		case "is:issue":
			hasIssue = true
		case "is:pr":
			hasPR = true
		}
	}
	if hasIssue && hasPR {
		return "", fmt.Errorf("GitHub search query contains conflicting explicit is:issue and is:pr qualifiers")
	}
	if hasPR {
		return "pull_request", nil
	}
	if hasIssue {
		return "issue", nil
	}
	return "", nil
}

func hasGitHubSearchQualifierPrefix(query, prefix string) bool {
	prefix = strings.ToLower(prefix)
	for _, token := range githubSearchTokens(query) {
		if strings.HasPrefix(strings.ToLower(token), prefix) {
			return true
		}
	}
	return false
}

func githubSearchTokens(query string) []string {
	tokens := []string{}
	var current strings.Builder
	inQuote := false
	escaped := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}
	for _, r := range query {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if inQuote && r == '\\' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			current.WriteRune(r)
			continue
		}
		if !inQuote && (r == ' ' || r == '\t' || r == '\n' || r == '\r') {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	flush()
	return tokens
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
	limit := minInt(30, len(issues))
	for {
		out := renderGitHubIssueListWithLimit(target, issues, links, total, incomplete, limit)
		if githubOverviewFits(out) || limit <= 1 {
			return out
		}
		limit--
	}
}

func renderGitHubIssueListWithLimit(target *GitHubTarget, issues []githubIssue, links GitHubLinkRelations, total int, incomplete bool, limit int) string {
	limit = minInt(limit, len(issues))
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"view: issues",
		fmt.Sprintf("results: %d", len(issues)),
		fmt.Sprintf("results_indexed: %d", limit),
		fmt.Sprintf("results_local_omitted: %d", len(issues)-limit),
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
	for _, issue := range issues[:limit] {
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
			meta = append(meta, "labels: "+githubOverviewLabelList(labels, 3))
		}
		title, truncated := githubOverviewInlinePreview(issue.Title, 140)
		if truncated {
			title += "…"
		}
		lines = append(lines, fmt.Sprintf("- [#%d %s](%s) — %s", issue.Number, escapeMarkdownLinkText(title), urlText, strings.Join(meta, " · ")))
	}
	if note := githubLocalOmissionNote("Issues returned on this provider page", len(issues)-limit); note != "" {
		lines = append(lines, "", note)
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
	out := githubBoundedOverviewList(len(labels), 20, func(limit int) string {
		lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "view: labels", fmt.Sprintf("results: %d", len(labels)), fmt.Sprintf("results_indexed: %d", limit), fmt.Sprintf("results_local_omitted: %d", len(labels)-limit), "---", "", "# Labels", ""}
		for _, label := range labels[:limit] {
			labelURL := fmt.Sprintf("https://github.com/%s/%s/labels/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), url.PathEscape(label.Name))
			name, truncated := githubOverviewInlinePreview(label.Name, 100)
			if truncated {
				name += "…"
			}
			line := "- [" + escapeMarkdownLinkText(name) + "](" + labelURL + ")"
			if label.Description != "" {
				description, descriptionTruncated := githubOverviewInlinePreview(label.Description, 140)
				if descriptionTruncated {
					description += "…"
				}
				line += " — " + description
			}
			lines = append(lines, line)
		}
		if len(labels) == 0 {
			lines = append(lines, "_No labels on this page._")
		}
		if note := githubLocalOmissionNote("labels returned on this provider page", len(labels)-limit); note != "" {
			lines = append(lines, "", note)
		}
		if nav := renderGitHubUIPageNavigation(target, resp.Links()); len(nav) > 0 {
			lines = append(lines, "", "## Navigation", "")
			lines = append(lines, nav...)
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	})
	return out, nil
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
		description, truncated := githubOverviewInlinePreview(label.Description, 500)
		lines = append(lines, "description: "+yamlScalar(description))
		if truncated {
			lines = append(lines, "description_preview_truncated: true")
		}
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
	out := githubBoundedOverviewList(len(milestones), 20, func(limit int) string {
		lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "view: milestones", fmt.Sprintf("results: %d", len(milestones)), fmt.Sprintf("results_indexed: %d", limit), fmt.Sprintf("results_local_omitted: %d", len(milestones)-limit), "---", "", "# Milestones", ""}
		if len(milestones) == 0 {
			lines = append(lines, "_No milestones on this page._")
		}
		for _, milestone := range milestones[:limit] {
			href := fmt.Sprintf("https://github.com/%s/%s/milestone/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), milestone.Number)
			meta := fmt.Sprintf("%s · %d open / %d closed", milestone.State, milestone.OpenIssues, milestone.ClosedIssues)
			if milestone.DueOn != "" {
				meta += " · due " + milestone.DueOn
			}
			title, truncated := githubOverviewInlinePreview(milestone.Title, 140)
			if truncated {
				title += "…"
			}
			lines = append(lines, fmt.Sprintf("- [%s](%s) — %s", escapeMarkdownLinkText(title), href, meta))
		}
		if note := githubLocalOmissionNote("milestones returned on this provider page", len(milestones)-limit); note != "" {
			lines = append(lines, "", note)
		}
		if nav := renderGitHubUIPageNavigation(target, resp.Links()); len(nav) > 0 {
			lines = append(lines, "", "## Navigation", "")
			lines = append(lines, nav...)
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	})
	return out, nil
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
	titlePreview, titleTruncated := githubOverviewInlinePreview(milestone.Title, 180)
	if titleTruncated {
		titlePreview += "…"
	}
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("milestone: %d", milestone.Number),
		"title: " + yamlScalar(titlePreview),
		"state: " + yamlScalar(milestone.State),
		fmt.Sprintf("open_issues: %d", milestone.OpenIssues),
		fmt.Sprintf("closed_issues: %d", milestone.ClosedIssues),
	}
	if milestone.DueOn != "" {
		lines = append(lines, "due: "+yamlScalar(milestone.DueOn))
	}
	lines = append(lines, "---", "", "# Milestone: "+titlePreview, "")
	if strings.TrimSpace(milestone.Description) != "" {
		preview, truncated := githubOverviewPreview(stripInvisibleHTMLComments(milestone.Description), 1000)
		lines = append(lines, preview, "")
		if truncated {
			href := fmt.Sprintf("https://github.com/%s/%s/milestone/%d", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), milestone.Number)
			lines = append(lines, "> Milestone description preview locally truncated. Full milestone: "+href, "")
		}
	}
	lines = append(lines, "## Issues", "")
	lines = append(lines, renderIssueLinksCompact(issues)...)
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func renderIssueLinksCompact(issues []githubIssue) []string {
	if len(issues) == 0 {
		return []string{"_No Issues on this page._"}
	}
	limit := minInt(20, len(issues))
	lines := make([]string, 0, limit+2)
	for _, issue := range issues[:limit] {
		href := issue.HTMLURL
		if href == "" {
			href = "#"
		}
		title, truncated := githubOverviewInlinePreview(issue.Title, 140)
		if truncated {
			title += "…"
		}
		lines = append(lines, fmt.Sprintf("- [#%d %s](%s) — %s", issue.Number, escapeMarkdownLinkText(title), href, issue.State))
	}
	if note := githubLocalOmissionNote("Issues returned on this provider page", len(issues)-limit); note != "" {
		lines = append(lines, "", note)
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
