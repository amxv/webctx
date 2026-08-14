package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type githubDiscussionSummary struct {
	Number      int
	Title       string
	URL         string
	CreatedAt   string
	UpdatedAt   string
	UpvoteCount int
	Locked      bool
	Author      string
	Category    string
}

type githubDiscussionComment struct {
	ID                  string
	DatabaseID          int64
	ParentDatabaseID    int64
	Body                string
	URL                 string
	CreatedAt           string
	UpdatedAt           string
	UpvoteCount         int
	Author              string
	Replies             []githubDiscussionComment
	RepliesReported     int
	RepliesProviderMore bool
}

type githubDiscussionDetail struct {
	githubDiscussionSummary
	Body                 string
	Answer               *githubDiscussionComment
	Comments             []githubDiscussionComment
	CommentsReported     int
	CommentsProviderMore bool
}

type githubGistFile struct {
	Filename  string `json:"filename"`
	Type      string `json:"type"`
	Language  string `json:"language"`
	RawURL    string `json:"raw_url"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	Content   string `json:"content"`
}

type githubGistHistory struct {
	Version      string `json:"version"`
	CommittedAt  string `json:"committed_at"`
	URL          string `json:"url"`
	ChangeStatus struct {
		Total     int `json:"total"`
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
	} `json:"change_status"`
}

type githubGist struct {
	ID          string                    `json:"id"`
	Description *string                   `json:"description"`
	Public      bool                      `json:"public"`
	HTMLURL     string                    `json:"html_url"`
	CreatedAt   string                    `json:"created_at"`
	UpdatedAt   string                    `json:"updated_at"`
	Comments    int                       `json:"comments"`
	Owner       githubUser                `json:"owner"`
	Files       map[string]githubGistFile `json:"files"`
	History     []githubGistHistory       `json:"history"`
}

type githubGistComment struct {
	ID        int64      `json:"id"`
	Body      *string    `json:"body"`
	User      githubUser `json:"user"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
	APIURL    string     `json:"url"`
	HTMLURL   string     `json:"html_url"`
}

type githubGistCommentsAvailability struct {
	ProviderMore bool
}

type githubDiscussionPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type githubDiscussionGraphQLComment struct {
	ID          string `json:"id"`
	DatabaseID  int64  `json:"databaseId"`
	Body        string `json:"body"`
	URL         string `json:"url"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	UpvoteCount int    `json:"upvoteCount"`
	Author      *struct {
		Login string `json:"login"`
	} `json:"author"`
	Replies struct {
		Nodes      []githubDiscussionGraphQLComment `json:"nodes"`
		TotalCount int                              `json:"totalCount"`
		PageInfo   githubDiscussionPageInfo         `json:"pageInfo"`
	} `json:"replies"`
}

type githubDiscussionGraphQLDetail struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	URL         string `json:"url"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	UpvoteCount int    `json:"upvoteCount"`
	Locked      bool   `json:"locked"`
	Author      *struct {
		Login string `json:"login"`
	} `json:"author"`
	Category struct {
		Name string `json:"name"`
	} `json:"category"`
	Answer   *githubDiscussionGraphQLComment `json:"answer"`
	Comments struct {
		Nodes      []githubDiscussionGraphQLComment `json:"nodes"`
		TotalCount int                              `json:"totalCount"`
		PageInfo   githubDiscussionPageInfo         `json:"pageInfo"`
	} `json:"comments"`
}

type gistFileSelector struct {
	Slug  string
	Start int
	End   int
}

var (
	discussionCommentFragmentRE = regexp.MustCompile(`^discussioncomment-([0-9]+)$`)
	gistCommentFragmentRE       = regexp.MustCompile(`^gistcomment-([0-9]+)$`)
	gistLineSuffixRE            = regexp.MustCompile(`^(file-.+?)(?:-L([0-9]+)(?:-L([0-9]+))?)?$`)
)

func readGitHubDiscussions(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if !client.hasToken() {
		return "", fmt.Errorf("GitHub Discussions require authentication. Set GH_TOKEN or GITHUB_TOKEN")
	}
	if target.Fragment != "" || len(target.Query) > 0 {
		return "", fmt.Errorf("GitHub Discussions list filters/selectors are not yet part of the native bounded list contract")
	}
	const query = `query($owner:String!,$repo:String!){repository(owner:$owner,name:$repo){discussions(first:30,orderBy:{field:UPDATED_AT,direction:DESC}){nodes{number title url createdAt updatedAt upvoteCount locked author{login} category{name}} pageInfo{hasNextPage endCursor}}}}`
	var data struct {
		Repository *struct {
			Discussions struct {
				Nodes []struct {
					Number      int    `json:"number"`
					Title       string `json:"title"`
					URL         string `json:"url"`
					CreatedAt   string `json:"createdAt"`
					UpdatedAt   string `json:"updatedAt"`
					UpvoteCount int    `json:"upvoteCount"`
					Locked      bool   `json:"locked"`
					Author      *struct {
						Login string `json:"login"`
					} `json:"author"`
					Category struct {
						Name string `json:"name"`
					} `json:"category"`
				} `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"discussions"`
		} `json:"repository"`
	}
	if err := client.GraphQL(ctx, query, map[string]any{"owner": target.Owner, "repo": target.Repo}, &data); err != nil {
		return "", err
	}
	if data.Repository == nil {
		return "", fmt.Errorf("GitHub Discussions repository was not available")
	}
	items := make([]githubDiscussionSummary, 0, len(data.Repository.Discussions.Nodes))
	for _, node := range data.Repository.Discussions.Nodes {
		item := githubDiscussionSummary{Number: node.Number, Title: node.Title, URL: node.URL, CreatedAt: node.CreatedAt, UpdatedAt: node.UpdatedAt, UpvoteCount: node.UpvoteCount, Locked: node.Locked, Category: node.Category.Name}
		if node.Author != nil {
			item.Author = node.Author.Login
		}
		items = append(items, item)
	}
	return renderGitHubDiscussions(target, items, data.Repository.Discussions.PageInfo.HasNextPage), nil
}

func readGitHubDiscussion(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if !client.hasToken() {
		return "", fmt.Errorf("GitHub Discussions require authentication. Set GH_TOKEN or GITHUB_TOKEN")
	}
	if len(target.Query) > 0 {
		return "", fmt.Errorf("GitHub Discussion query parameters are not part of the native Discussion contract")
	}
	databaseID, hasSelector, err := parseDiscussionCommentSelector(target.Fragment)
	if err != nil {
		return "", err
	}
	if hasSelector {
		detail, comment, err := fetchGitHubDiscussionCommentByDatabaseID(ctx, client, target, databaseID)
		if err != nil {
			return "", err
		}
		return renderGitHubSelectedDiscussionComment(target, detail, comment), nil
	}
	detail, err := fetchGitHubDiscussionOverview(ctx, client, target)
	if err != nil {
		return "", err
	}
	return renderGitHubDiscussion(target, detail), nil
}

func parseDiscussionCommentSelector(fragment string) (int64, bool, error) {
	if fragment == "" {
		return 0, false, nil
	}
	match := discussionCommentFragmentRE.FindStringSubmatch(fragment)
	if match == nil {
		return 0, true, fmt.Errorf("GitHub Discussion fragment %q is not a supported discussion-comment selector", fragment)
	}
	id, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, true, fmt.Errorf("invalid GitHub Discussion comment selector %q", fragment)
	}
	return id, true, nil
}

func mapDiscussionGraphQLComment(node githubDiscussionGraphQLComment, parentDatabaseID int64) githubDiscussionComment {
	comment := githubDiscussionComment{
		ID:                  node.ID,
		DatabaseID:          node.DatabaseID,
		ParentDatabaseID:    parentDatabaseID,
		Body:                node.Body,
		URL:                 node.URL,
		CreatedAt:           node.CreatedAt,
		UpdatedAt:           node.UpdatedAt,
		UpvoteCount:         node.UpvoteCount,
		RepliesReported:     node.Replies.TotalCount,
		RepliesProviderMore: node.Replies.PageInfo.HasNextPage || node.Replies.TotalCount > len(node.Replies.Nodes),
	}
	if node.Author != nil {
		comment.Author = node.Author.Login
	}
	for _, replyNode := range node.Replies.Nodes {
		comment.Replies = append(comment.Replies, mapDiscussionGraphQLComment(replyNode, node.DatabaseID))
	}
	return comment
}

func mapDiscussionGraphQLDetail(d *githubDiscussionGraphQLDetail) githubDiscussionDetail {
	detail := githubDiscussionDetail{
		githubDiscussionSummary: githubDiscussionSummary{
			Number: d.Number, Title: d.Title, URL: d.URL, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
			UpvoteCount: d.UpvoteCount, Locked: d.Locked, Category: d.Category.Name,
		},
		Body:                 d.Body,
		CommentsReported:     d.Comments.TotalCount,
		CommentsProviderMore: d.Comments.PageInfo.HasNextPage || d.Comments.TotalCount > len(d.Comments.Nodes),
	}
	if d.Author != nil {
		detail.Author = d.Author.Login
	}
	if d.Answer != nil {
		answer := mapDiscussionGraphQLComment(*d.Answer, 0)
		detail.Answer = &answer
	}
	for _, node := range d.Comments.Nodes {
		detail.Comments = append(detail.Comments, mapDiscussionGraphQLComment(node, 0))
	}
	return detail
}

func fetchGitHubDiscussionOverview(ctx context.Context, client *GitHubClient, target *GitHubTarget) (githubDiscussionDetail, error) {
	const query = `query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){discussion(number:$number){number title body url createdAt updatedAt upvoteCount locked author{login} category{name} answer{id databaseId body url createdAt updatedAt upvoteCount author{login}} comments(first:30){totalCount nodes{id databaseId body url createdAt updatedAt upvoteCount author{login} replies(first:5){totalCount nodes{id databaseId body url createdAt updatedAt upvoteCount author{login}} pageInfo{hasNextPage endCursor}}} pageInfo{hasNextPage endCursor}}}}}`
	var data struct {
		Repository *struct {
			Discussion *githubDiscussionGraphQLDetail `json:"discussion"`
		} `json:"repository"`
	}
	if err := client.GraphQL(ctx, query, map[string]any{"owner": target.Owner, "repo": target.Repo, "number": target.Number}, &data); err != nil {
		return githubDiscussionDetail{}, err
	}
	if data.Repository == nil || data.Repository.Discussion == nil {
		return githubDiscussionDetail{}, fmt.Errorf("GitHub Discussion #%d was not available", target.Number)
	}
	return mapDiscussionGraphQLDetail(data.Repository.Discussion), nil
}

type githubDiscussionReplyCursor struct {
	ParentID         string
	ParentDatabaseID int64
	Cursor           string
}

func fetchGitHubDiscussionCommentByDatabaseID(ctx context.Context, client *GitHubClient, target *GitHubTarget, databaseID int64) (githubDiscussionDetail, githubDiscussionComment, error) {
	const query = `query($owner:String!,$repo:String!,$number:Int!,$after:String){repository(owner:$owner,name:$repo){discussion(number:$number){number title url createdAt updatedAt upvoteCount locked author{login} category{name} comments(first:100,after:$after){totalCount nodes{id databaseId body url createdAt updatedAt upvoteCount author{login} replies(first:100){totalCount nodes{id databaseId body url createdAt updatedAt upvoteCount author{login}} pageInfo{hasNextPage endCursor}}} pageInfo{hasNextPage endCursor}}}}}`
	var detail githubDiscussionDetail
	var after any
	var deferred []githubDiscussionReplyCursor
	for {
		var data struct {
			Repository *struct {
				Discussion *githubDiscussionGraphQLDetail `json:"discussion"`
			} `json:"repository"`
		}
		if err := client.GraphQL(ctx, query, map[string]any{"owner": target.Owner, "repo": target.Repo, "number": target.Number, "after": after}, &data); err != nil {
			return githubDiscussionDetail{}, githubDiscussionComment{}, err
		}
		if data.Repository == nil || data.Repository.Discussion == nil {
			return githubDiscussionDetail{}, githubDiscussionComment{}, fmt.Errorf("GitHub Discussion #%d was not available", target.Number)
		}
		d := data.Repository.Discussion
		if detail.Number == 0 {
			detail = mapDiscussionGraphQLDetail(d)
			detail.Comments = nil
		}
		for _, node := range d.Comments.Nodes {
			comment := mapDiscussionGraphQLComment(node, 0)
			if comment.DatabaseID == databaseID {
				if err := validateDiscussionCommentURL(target, comment, databaseID); err != nil {
					return githubDiscussionDetail{}, githubDiscussionComment{}, err
				}
				return detail, comment, nil
			}
			for _, reply := range comment.Replies {
				if reply.DatabaseID == databaseID {
					if err := validateDiscussionCommentURL(target, reply, databaseID); err != nil {
						return githubDiscussionDetail{}, githubDiscussionComment{}, err
					}
					return detail, reply, nil
				}
			}
			if node.Replies.PageInfo.HasNextPage {
				deferred = append(deferred, githubDiscussionReplyCursor{ParentID: node.ID, ParentDatabaseID: node.DatabaseID, Cursor: node.Replies.PageInfo.EndCursor})
			}
		}
		if !d.Comments.PageInfo.HasNextPage {
			break
		}
		after = d.Comments.PageInfo.EndCursor
	}
	for _, pending := range deferred {
		reply, found, err := fetchGitHubDiscussionReplyByDatabaseID(ctx, client, pending, databaseID)
		if err != nil {
			return githubDiscussionDetail{}, githubDiscussionComment{}, err
		}
		if found {
			if err := validateDiscussionCommentURL(target, reply, databaseID); err != nil {
				return githubDiscussionDetail{}, githubDiscussionComment{}, err
			}
			return detail, reply, nil
		}
	}
	return githubDiscussionDetail{}, githubDiscussionComment{}, fmt.Errorf("GitHub Discussion comment selector %q was not found in Discussion #%d", target.Fragment, target.Number)
}

func fetchGitHubDiscussionReplyByDatabaseID(ctx context.Context, client *GitHubClient, pending githubDiscussionReplyCursor, databaseID int64) (githubDiscussionComment, bool, error) {
	const query = `query($id:ID!,$after:String){node(id:$id){... on DiscussionComment{replies(first:100,after:$after){nodes{id databaseId body url createdAt updatedAt upvoteCount author{login}} pageInfo{hasNextPage endCursor}}}}}`
	var after any = pending.Cursor
	for {
		var data struct {
			Node *struct {
				Replies struct {
					Nodes    []githubDiscussionGraphQLComment `json:"nodes"`
					PageInfo githubDiscussionPageInfo         `json:"pageInfo"`
				} `json:"replies"`
			} `json:"node"`
		}
		if err := client.GraphQL(ctx, query, map[string]any{"id": pending.ParentID, "after": after}, &data); err != nil {
			return githubDiscussionComment{}, false, err
		}
		if data.Node == nil {
			return githubDiscussionComment{}, false, fmt.Errorf("GitHub Discussion comment reply connection was unavailable")
		}
		for _, node := range data.Node.Replies.Nodes {
			reply := mapDiscussionGraphQLComment(node, pending.ParentDatabaseID)
			if reply.DatabaseID == databaseID {
				return reply, true, nil
			}
		}
		if !data.Node.Replies.PageInfo.HasNextPage {
			return githubDiscussionComment{}, false, nil
		}
		after = data.Node.Replies.PageInfo.EndCursor
	}
}

func validateDiscussionCommentURL(target *GitHubTarget, comment githubDiscussionComment, databaseID int64) error {
	if comment.DatabaseID != databaseID || strings.TrimSpace(comment.URL) == "" {
		return fmt.Errorf("GitHub Discussion comment %d did not return stable selector identity", databaseID)
	}
	parsed, err := url.Parse(comment.URL)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") || parsed.Fragment != fmt.Sprintf("discussioncomment-%d", databaseID) {
		return fmt.Errorf("GitHub Discussion comment %d returned unexpected canonical URL %q", databaseID, comment.URL)
	}
	parts := splitGitHubPath(parsed.Path)
	if len(parts) < 4 || parts[len(parts)-2] != "discussions" {
		return fmt.Errorf("GitHub Discussion comment %d returned unexpected canonical URL %q", databaseID, comment.URL)
	}
	number, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || number != target.Number {
		return fmt.Errorf("GitHub Discussion comment %d belongs to a different Discussion", databaseID)
	}
	return nil
}

func renderGitHubDiscussions(target *GitHubTarget, items []githubDiscussionSummary, hasMore bool) string {
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "view: discussions", fmt.Sprintf("returned: %d", len(items)), fmt.Sprintf("more_available: %t", hasMore), "---", "", "# Discussions", ""}
	if len(items) == 0 {
		lines = append(lines, "_No Discussions returned by GitHub._")
	}
	for _, item := range items {
		line := fmt.Sprintf("- [#%d %s](%s)", item.Number, escapeMarkdownLinkText(item.Title), item.URL)
		meta := []string{}
		if item.Category != "" {
			meta = append(meta, item.Category)
		}
		if item.Author != "" {
			meta = append(meta, "@"+item.Author)
		}
		if item.UpvoteCount > 0 {
			meta = append(meta, fmt.Sprintf("%d upvotes", item.UpvoteCount))
		}
		if item.Locked {
			meta = append(meta, "locked")
		}
		if item.UpdatedAt != "" {
			meta = append(meta, "updated "+item.UpdatedAt)
		}
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	if hasMore {
		lines = append(lines, "", "> More Discussions exist upstream. This native list intentionally returns only the first 30 because GitHub's GraphQL cursor is not a stable copied-page URL selector.")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubDiscussion(target *GitHubTarget, detail githubDiscussionDetail) string {
	comments, answerReturned := prioritizeDiscussionAnswer(detail.Comments, detail.Answer)
	commentLimit := minInt(10, len(comments))
	replyLimit := 2
	bodyRunes := 1200
	commentRunes := 220
	answerRunes := 500
	for {
		out := renderGitHubDiscussionWithLimits(target, detail, comments, answerReturned, commentLimit, replyLimit, bodyRunes, commentRunes, answerRunes)
		if githubOverviewFits(out) {
			return out
		}
		switch {
		case commentLimit > 4:
			commentLimit--
		case bodyRunes > 700:
			bodyRunes -= 100
		case replyLimit > 1:
			replyLimit--
		case commentRunes > 140:
			commentRunes -= 20
		case answerRunes > 300:
			answerRunes -= 50
		case commentLimit > 1:
			commentLimit--
		default:
			return out
		}
	}
}

func prioritizeDiscussionAnswer(comments []githubDiscussionComment, answer *githubDiscussionComment) ([]githubDiscussionComment, bool) {
	out := append([]githubDiscussionComment(nil), comments...)
	if answer == nil {
		return out, false
	}
	for i := range out {
		if discussionCommentSameIdentity(out[i], *answer) {
			if i > 0 {
				out[0], out[i] = out[i], out[0]
			}
			return out, true
		}
	}
	return out, false
}

func discussionCommentSameIdentity(a, b githubDiscussionComment) bool {
	if a.ID != "" && b.ID != "" {
		return a.ID == b.ID
	}
	return a.DatabaseID > 0 && a.DatabaseID == b.DatabaseID
}

func renderGitHubDiscussionWithLimits(target *GitHubTarget, detail githubDiscussionDetail, comments []githubDiscussionComment, answerReturned bool, commentLimit, replyLimit, bodyRunes, commentRunes, answerRunes int) string {
	commentLimit = minInt(commentLimit, len(comments))
	titlePreview, titleTruncated := githubOverviewInlinePreview(detail.Title, 180)
	if titleTruncated {
		titlePreview += "…"
	}
	categoryPreview, categoryTruncated := githubOverviewInlinePreview(detail.Category, 120)
	if categoryTruncated {
		categoryPreview += "…"
	}
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("discussion: %d", detail.Number),
		"title: " + yamlScalar(titlePreview),
		"category: " + yamlScalar(categoryPreview),
		fmt.Sprintf("locked: %t", detail.Locked),
		fmt.Sprintf("upvotes: %d", detail.UpvoteCount),
		"overview: true",
		fmt.Sprintf("comments_reported: %d", detail.CommentsReported),
		fmt.Sprintf("comments_returned: %d", len(detail.Comments)),
		fmt.Sprintf("comments_indexed: %d", commentLimit),
		fmt.Sprintf("comments_local_omitted: %d", len(detail.Comments)-commentLimit),
	}
	if titleTruncated {
		lines = append(lines, "title_preview_truncated: true")
	}
	if categoryTruncated {
		lines = append(lines, "category_preview_truncated: true")
	}
	if detail.CommentsProviderMore {
		lines = append(lines, "comments_provider_more_available: true")
	}
	if detail.Answer != nil {
		if detail.Answer.DatabaseID > 0 {
			lines = append(lines, fmt.Sprintf("accepted_answer_comment_id: %d", detail.Answer.DatabaseID))
		}
		if detail.Answer.URL != "" {
			lines = append(lines, "accepted_answer_url: "+yamlScalar(detail.Answer.URL))
		}
		if !answerReturned {
			lines = append(lines, "accepted_answer_outside_returned_comment_page: true")
		}
	}
	if detail.Author != "" {
		lines = append(lines, "author: "+yamlScalar("@"+detail.Author))
	}
	if detail.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(detail.CreatedAt))
	}
	if detail.UpdatedAt != "" {
		lines = append(lines, "updated: "+yamlScalar(detail.UpdatedAt))
	}
	if detail.URL != "" {
		lines = append(lines, "url: "+yamlScalar(detail.URL))
	}
	lines = append(lines, "---", "", "# "+titlePreview, "")
	body := strings.TrimSpace(stripInvisibleHTMLComments(detail.Body))
	if body == "" {
		body = "_No Discussion body._"
	}
	bodyPreview, bodyTruncated := githubOverviewPreview(body, bodyRunes)
	lines = append(lines, bodyPreview)
	if bodyTruncated {
		lines = append(lines, "", "> Discussion body preview locally truncated. Full Discussion: "+detail.URL)
	}
	if detail.Answer != nil && !answerReturned {
		lines = append(lines, "", "## Accepted answer", "")
		lines = append(lines, renderDiscussionCommentIndex(*detail.Answer, true, 3, answerRunes)...)
	}
	lines = append(lines, "", "## Conversation index", "")
	if len(detail.Comments) == 0 {
		lines = append(lines, "_No comments returned by GitHub._")
	}
	for _, comment := range comments[:commentLimit] {
		answer := detail.Answer != nil && discussionCommentSameIdentity(comment, *detail.Answer)
		lines = append(lines, renderDiscussionCommentIndex(comment, answer, 3, commentRunes)...)
		shownReplies := minInt(replyLimit, len(comment.Replies))
		for _, reply := range comment.Replies[:shownReplies] {
			lines = append(lines, renderDiscussionCommentIndex(reply, false, 4, commentRunes)...)
		}
		if omitted := len(comment.Replies) - shownReplies; omitted > 0 {
			lines = append(lines, fmt.Sprintf("> %d replies returned for comment `%d` locally omitted from this overview.", omitted, comment.DatabaseID), "")
		}
		if comment.RepliesProviderMore {
			lines = append(lines, fmt.Sprintf("> More replies to comment `%d` exist upstream beyond the provider page fetched for this overview.", comment.DatabaseID), "")
		}
	}
	if note := githubLocalOmissionNote("top-level Discussion comments returned by GitHub", len(detail.Comments)-commentLimit); note != "" {
		lines = append(lines, note)
	}
	if detail.CommentsProviderMore {
		lines = append(lines, "", "> More top-level Discussion comments exist upstream beyond the provider page fetched for this overview. Copy an indexed `#discussioncomment-...` URL for an exact comment read.")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderDiscussionCommentIndex(comment githubDiscussionComment, answer bool, level, maxRunes int) []string {
	heading := strings.Repeat("#", level) + " "
	if answer {
		heading += "Accepted answer"
	} else if comment.ParentDatabaseID > 0 || level > 3 {
		heading += "Reply"
	} else {
		heading += "Comment"
	}
	if comment.DatabaseID > 0 {
		heading += fmt.Sprintf(" `%d`", comment.DatabaseID)
	}
	if comment.Author != "" {
		heading += " by @" + comment.Author
	}
	if comment.CreatedAt != "" {
		heading += " — " + comment.CreatedAt
	}
	lines := []string{heading, ""}
	meta := []string{}
	if comment.UpvoteCount > 0 {
		meta = append(meta, fmt.Sprintf("%d upvotes", comment.UpvoteCount))
	}
	if comment.ParentDatabaseID > 0 {
		meta = append(meta, fmt.Sprintf("reply to `%d`", comment.ParentDatabaseID))
	}
	if comment.URL != "" {
		meta = append(meta, comment.URL)
	}
	if len(meta) > 0 {
		lines = append(lines, "_"+strings.Join(meta, " · ")+"_", "")
	}
	body := strings.TrimSpace(stripInvisibleHTMLComments(comment.Body))
	if body == "" {
		body = "_Empty comment._"
	}
	preview, truncated := githubOverviewPreview(body, maxRunes)
	lines = append(lines, preview)
	if truncated {
		lines = append(lines, "", "> Comment preview locally truncated; use the canonical comment URL above for exact/full context.")
	}
	lines = append(lines, "")
	return lines
}

func renderGitHubSelectedDiscussionComment(target *GitHubTarget, detail githubDiscussionDetail, comment githubDiscussionComment) string {
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		fmt.Sprintf("discussion: %d", detail.Number),
		"selector: " + yamlScalar(target.Fragment),
		fmt.Sprintf("comment_id: %d", comment.DatabaseID),
	}
	if comment.ParentDatabaseID > 0 {
		lines = append(lines, fmt.Sprintf("parent_comment_id: %d", comment.ParentDatabaseID))
	}
	if comment.Author != "" {
		lines = append(lines, "author: "+yamlScalar("@"+comment.Author))
	}
	if comment.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(comment.CreatedAt))
	}
	if comment.UpdatedAt != "" {
		lines = append(lines, "updated: "+yamlScalar(comment.UpdatedAt))
	}
	if comment.URL != "" {
		lines = append(lines, "url: "+yamlScalar(comment.URL))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# Discussion comment %d", comment.DatabaseID), "")
	if detail.Title != "" {
		lines = append(lines, "From: [#"+strconv.Itoa(detail.Number)+" "+escapeMarkdownLinkText(detail.Title)+"]("+detail.URL+")", "")
	}
	body := strings.TrimSpace(stripInvisibleHTMLComments(comment.Body))
	if body == "" {
		body = "_Empty comment._"
	}
	lines = append(lines, body)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubGist(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if len(target.Query) > 0 {
		return "", fmt.Errorf("GitHub Gist query parameters are not part of the native Gist contract")
	}
	endpoint := "/gists/" + url.PathEscape(target.Name)
	if len(target.Tail) == 1 {
		endpoint += "/" + url.PathEscape(target.Tail[0])
	}
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var gist githubGist
	if err := json.Unmarshal(resp.Body, &gist); err != nil {
		return "", fmt.Errorf("decode GitHub Gist: %w", err)
	}
	if gist.ID != "" && gist.ID != target.Name {
		return "", fmt.Errorf("GitHub Gist response identity %q did not match requested Gist %q", gist.ID, target.Name)
	}
	commentID, hasCommentSelector, err := parseGistCommentSelector(target.Fragment)
	if err != nil {
		return "", err
	}
	if hasCommentSelector {
		if len(target.Tail) > 0 {
			return "", fmt.Errorf("GitHub Gist comment selectors apply to the canonical Gist, not a revision URL")
		}
		comment, err := fetchGitHubGistComment(ctx, client, target.Name, commentID)
		if err != nil {
			return "", err
		}
		return renderGitHubSelectedGistComment(target, gist, comment), nil
	}
	selector, hasFileSelector, err := parseGistFileSelector(target.Fragment)
	if err != nil {
		return "", err
	}
	files := sortedGitHubGistFiles(gist.Files)
	if hasFileSelector {
		matched := -1
		for i := range files {
			if gistFileSlug(files[i].Filename) == selector.Slug {
				matched = i
				break
			}
		}
		if matched < 0 {
			return "", fmt.Errorf("GitHub Gist file selector %q did not match a file", target.Fragment)
		}
		file := files[matched]
		if file.Truncated && file.RawURL != "" {
			if raw, ok := fetchGitHubGistRaw(ctx, client, file.RawURL); ok {
				file.Content = raw
				file.Truncated = false
			}
		}
		return renderGitHubSelectedGistFile(target, gist, file, selector), nil
	}
	comments, availability, err := fetchGitHubGistCommentPage(ctx, client, target.Name)
	if err != nil {
		return "", err
	}
	return renderGitHubGistOverview(target, gist, files, comments, availability), nil
}

func sortedGitHubGistFiles(fileMap map[string]githubGistFile) []githubGistFile {
	files := make([]githubGistFile, 0, len(fileMap))
	for _, file := range fileMap {
		files = append(files, file)
	}
	sort.SliceStable(files, func(i, j int) bool {
		left := strings.ToLower(files[i].Filename)
		right := strings.ToLower(files[j].Filename)
		if left == right {
			return files[i].Filename < files[j].Filename
		}
		return left < right
	})
	return files
}

func parseGistCommentSelector(fragment string) (int64, bool, error) {
	if fragment == "" || !strings.HasPrefix(fragment, "gistcomment-") {
		return 0, false, nil
	}
	match := gistCommentFragmentRE.FindStringSubmatch(fragment)
	if match == nil {
		return 0, true, fmt.Errorf("invalid GitHub Gist comment selector %q", fragment)
	}
	id, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, true, fmt.Errorf("invalid GitHub Gist comment selector %q", fragment)
	}
	return id, true, nil
}

func fetchGitHubGistCommentPage(ctx context.Context, client *GitHubClient, gistID string) ([]githubGistComment, githubGistCommentsAvailability, error) {
	endpoint := fmt.Sprintf("/gists/%s/comments?per_page=30", url.PathEscape(gistID))
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, githubGistCommentsAvailability{}, err
	}
	var comments []githubGistComment
	if err := json.Unmarshal(resp.Body, &comments); err != nil {
		return nil, githubGistCommentsAvailability{}, fmt.Errorf("decode GitHub Gist comments: %w", err)
	}
	return comments, githubGistCommentsAvailability{ProviderMore: resp.Links()["next"] != ""}, nil
}

func fetchGitHubGistComment(ctx context.Context, client *GitHubClient, gistID string, commentID int64) (githubGistComment, error) {
	endpoint := fmt.Sprintf("/gists/%s/comments/%d", url.PathEscape(gistID), commentID)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return githubGistComment{}, err
	}
	var comment githubGistComment
	if err := json.Unmarshal(resp.Body, &comment); err != nil {
		return githubGistComment{}, fmt.Errorf("decode GitHub Gist comment: %w", err)
	}
	if comment.ID != commentID {
		return githubGistComment{}, fmt.Errorf("GitHub Gist comment response ID %d did not match selector %d", comment.ID, commentID)
	}
	if comment.APIURL != "" {
		parsed, err := url.Parse(comment.APIURL)
		parts := splitGitHubPath(parsed.Path)
		if err != nil || len(parts) < 4 || parts[len(parts)-4] != "gists" || parts[len(parts)-3] != gistID || parts[len(parts)-2] != "comments" || parts[len(parts)-1] != strconv.FormatInt(commentID, 10) {
			return githubGistComment{}, fmt.Errorf("GitHub Gist comment %d returned unexpected API identity %q", commentID, comment.APIURL)
		}
	}
	return comment, nil
}

func fetchGitHubGistRaw(ctx context.Context, client *GitHubClient, rawURL string) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", false
	}
	if strings.TrimSpace(client.userAgent) != "" {
		req.Header.Set("User-Agent", client.userAgent)
	}
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || resp.ContentLength > githubBlobMaxBytes {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, githubBlobMaxBytes+1))
	if err != nil || int64(len(body)) > githubBlobMaxBytes || !utf8.Valid(body) {
		return "", false
	}
	return string(body), true
}

func parseGistFileSelector(fragment string) (gistFileSelector, bool, error) {
	if fragment == "" {
		return gistFileSelector{}, false, nil
	}
	match := gistLineSuffixRE.FindStringSubmatch(fragment)
	if match == nil {
		return gistFileSelector{}, true, fmt.Errorf("GitHub Gist fragment %q is not a supported file/line selector", fragment)
	}
	selector := gistFileSelector{Slug: match[1]}
	if match[2] != "" {
		selector.Start, _ = strconv.Atoi(match[2])
		selector.End = selector.Start
		if match[3] != "" {
			selector.End, _ = strconv.Atoi(match[3])
		}
		if selector.Start <= 0 || selector.End < selector.Start {
			return gistFileSelector{}, true, fmt.Errorf("invalid GitHub Gist line selector %q", fragment)
		}
	}
	return selector, true, nil
}

func gistFileSlug(filename string) string {
	var b strings.Builder
	b.WriteString("file-")
	lastDash := false
	for _, r := range strings.ToLower(filename) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

func gistCanonicalURL(target *GitHubTarget, gist githubGist) string {
	if strings.TrimSpace(gist.HTMLURL) != "" {
		return gist.HTMLURL
	}
	owner := target.Owner
	if owner == "" && gist.Owner.Login != "" {
		owner = gist.Owner.Login
	}
	if owner != "" {
		return fmt.Sprintf("https://gist.github.com/%s/%s", escapePathPreservingSlashes(owner), url.PathEscape(target.Name))
	}
	return "https://gist.github.com/" + url.PathEscape(target.Name)
}

func gistCommentCanonicalURL(target *GitHubTarget, gist githubGist, comment githubGistComment) string {
	if strings.TrimSpace(comment.HTMLURL) != "" {
		return comment.HTMLURL
	}
	return fmt.Sprintf("%s#gistcomment-%d", gistCanonicalURL(target, gist), comment.ID)
}

func renderGitHubGistOverview(target *GitHubTarget, gist githubGist, files []githubGistFile, comments []githubGistComment, availability githubGistCommentsAvailability) string {
	fileLimit := minInt(12, len(files))
	commentLimit := minInt(8, len(comments))
	revisionLimit := minInt(8, len(gist.History))
	filePreviewRunes := 220
	commentPreviewRunes := 180
	for {
		out := renderGitHubGistOverviewWithLimits(target, gist, files, comments, availability, fileLimit, commentLimit, revisionLimit, filePreviewRunes, commentPreviewRunes)
		if githubOverviewFits(out) {
			return out
		}
		switch {
		case fileLimit > 6:
			fileLimit--
		case commentLimit > 4:
			commentLimit--
		case filePreviewRunes > 120:
			filePreviewRunes -= 20
		case commentPreviewRunes > 120:
			commentPreviewRunes -= 20
		case revisionLimit > 4:
			revisionLimit--
		case fileLimit > 1:
			fileLimit--
		case commentLimit > 1:
			commentLimit--
		default:
			return out
		}
	}
}

func renderGitHubGistOverviewWithLimits(target *GitHubTarget, gist githubGist, files []githubGistFile, comments []githubGistComment, availability githubGistCommentsAvailability, fileLimit, commentLimit, revisionLimit, filePreviewRunes, commentPreviewRunes int) string {
	fileLimit = minInt(fileLimit, len(files))
	commentLimit = minInt(commentLimit, len(comments))
	revisionLimit = minInt(revisionLimit, len(gist.History))
	canonicalURL := gistCanonicalURL(target, gist)
	reportedComments := gist.Comments
	if reportedComments < len(comments) {
		reportedComments = len(comments)
	}
	lines := []string{
		"---",
		"gist: " + yamlScalar(gist.ID),
		fmt.Sprintf("public: %t", gist.Public),
		"overview: true",
		fmt.Sprintf("files_returned: %d", len(files)),
		fmt.Sprintf("files_indexed: %d", fileLimit),
		fmt.Sprintf("files_local_omitted: %d", len(files)-fileLimit),
		fmt.Sprintf("comments_reported: %d", reportedComments),
		fmt.Sprintf("comments_returned: %d", len(comments)),
		fmt.Sprintf("comments_indexed: %d", commentLimit),
		fmt.Sprintf("comments_local_omitted: %d", len(comments)-commentLimit),
		fmt.Sprintf("revisions_returned: %d", len(gist.History)),
		fmt.Sprintf("revisions_indexed: %d", revisionLimit),
		fmt.Sprintf("revisions_local_omitted: %d", len(gist.History)-revisionLimit),
	}
	if availability.ProviderMore {
		lines = append(lines, "comments_provider_more_available: true")
	}
	if target.Owner != "" {
		lines = append(lines, "owner: "+yamlScalar("@"+target.Owner))
	}
	if len(target.Tail) == 1 {
		lines = append(lines, "revision: "+yamlScalar(target.Tail[0]))
	}
	if gist.Description != nil && strings.TrimSpace(*gist.Description) != "" {
		lines = append(lines, "description: "+yamlScalar(*gist.Description))
	}
	if gist.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(gist.CreatedAt))
	}
	if gist.UpdatedAt != "" {
		lines = append(lines, "updated: "+yamlScalar(gist.UpdatedAt))
	}
	lines = append(lines, "url: "+yamlScalar(canonicalURL), "---", "", "# Gist "+gist.ID, "", "## File index", "")
	if len(files) == 0 {
		lines = append(lines, "_No files returned by GitHub._")
	}
	for _, file := range files[:fileLimit] {
		selectorURL := canonicalURL + "#" + gistFileSlug(file.Filename)
		line := fmt.Sprintf("- [%s](%s)", escapeMarkdownLinkText(file.Filename), selectorURL)
		meta := []string{}
		if file.Language != "" {
			meta = append(meta, file.Language)
		}
		if file.Size > 0 {
			meta = append(meta, fmt.Sprintf("%d bytes", file.Size))
		}
		if file.Truncated {
			meta = append(meta, "API content truncated")
		}
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
		content := file.Content
		if content != "" && !file.Truncated {
			preview, truncated := githubOverviewPreview(content, filePreviewRunes)
			lines = append(lines, "  Preview: `"+strings.ReplaceAll(strings.ReplaceAll(preview, "`", "'"), "\n", " ⏎ ")+"`")
			if truncated {
				lines = append(lines, "  Full file: "+selectorURL)
			}
		}
		if file.RawURL != "" {
			lines = append(lines, "  Raw: "+file.RawURL)
		}
	}
	if note := githubLocalOmissionNote("Gist files returned by GitHub", len(files)-fileLimit); note != "" {
		lines = append(lines, "", note)
	}
	lines = append(lines, "", "## Comment index", "")
	if len(comments) == 0 {
		lines = append(lines, "_No Gist comments returned on this provider page._")
	}
	for _, comment := range comments[:commentLimit] {
		commentURL := gistCommentCanonicalURL(target, gist, comment)
		heading := fmt.Sprintf("- [Comment %d](%s)", comment.ID, commentURL)
		meta := []string{}
		if comment.User.Login != "" {
			meta = append(meta, "@"+comment.User.Login)
		}
		if comment.CreatedAt != "" {
			meta = append(meta, comment.CreatedAt)
		}
		if len(meta) > 0 {
			heading += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, heading)
		body := ""
		if comment.Body != nil {
			body = strings.TrimSpace(stripInvisibleHTMLComments(*comment.Body))
		}
		if body == "" {
			body = "_Gist comment body is unavailable or empty._"
		}
		preview, truncated := githubOverviewPreview(body, commentPreviewRunes)
		lines = append(lines, "  "+strings.ReplaceAll(preview, "\n", " "))
		if truncated {
			lines = append(lines, "  Exact comment: "+commentURL)
		}
	}
	if note := githubLocalOmissionNote("Gist comments returned on this provider page", len(comments)-commentLimit); note != "" {
		lines = append(lines, "", note)
	}
	if availability.ProviderMore || reportedComments > len(comments) {
		lines = append(lines, "", "> More Gist comments exist upstream beyond the provider page fetched for this overview; copy a `#gistcomment-...` URL above for an exact comment read.")
	}
	if len(gist.History) > 0 {
		lines = append(lines, "", "## Revision index", "")
		for _, revision := range gist.History[:revisionLimit] {
			line := "- `" + shortSHA(revision.Version) + "`"
			if revision.CommittedAt != "" {
				line += " — " + revision.CommittedAt
			}
			line += fmt.Sprintf(" — +%d -%d (%d changes)", revision.ChangeStatus.Additions, revision.ChangeStatus.Deletions, revision.ChangeStatus.Total)
			lines = append(lines, line)
		}
		if note := githubLocalOmissionNote("Gist revisions returned by GitHub", len(gist.History)-revisionLimit); note != "" {
			lines = append(lines, "", note)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubSelectedGistFile(target *GitHubTarget, gist githubGist, file githubGistFile, selector gistFileSelector) string {
	lines := []string{
		"---",
		"gist: " + yamlScalar(gist.ID),
		"file: " + yamlScalar(file.Filename),
		"selector: " + yamlScalar(target.Fragment),
	}
	if len(target.Tail) == 1 {
		lines = append(lines, "revision: "+yamlScalar(target.Tail[0]))
	}
	if file.Language != "" {
		lines = append(lines, "language: "+yamlScalar(file.Language))
	}
	if file.Size > 0 {
		lines = append(lines, fmt.Sprintf("size: %d", file.Size))
	}
	if file.RawURL != "" {
		lines = append(lines, "raw_url: "+yamlScalar(file.RawURL))
	}
	lines = append(lines, "url: "+yamlScalar(gistCanonicalURL(target, gist)+"#"+target.Fragment), "---", "", "# "+file.Filename, "")
	content := file.Content
	if selector.Start > 0 {
		selected, err := selectTextLines(content, selector.Start, selector.End)
		if err != nil {
			lines = append(lines, "_Selected line range is out of range for the Gist file._")
			return strings.TrimSpace(strings.Join(lines, "\n"))
		}
		content = selected
		lines = append(lines, fmt.Sprintf("**Lines %d-%d:**", selector.Start, selector.End), "")
	}
	if file.Truncated {
		lines = append(lines, "_GitHub marked this API file content truncated; it is not presented as complete._")
		if file.RawURL != "" {
			lines = append(lines, "Full raw file: "+file.RawURL)
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	fence := "```"
	if strings.Contains(content, "```") {
		fence = "````"
	}
	lines = append(lines, fence, content, fence)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubSelectedGistComment(target *GitHubTarget, gist githubGist, comment githubGistComment) string {
	commentURL := gistCommentCanonicalURL(target, gist, comment)
	lines := []string{
		"---",
		"gist: " + yamlScalar(gist.ID),
		"selector: " + yamlScalar(target.Fragment),
		fmt.Sprintf("comment_id: %d", comment.ID),
		"url: " + yamlScalar(commentURL),
	}
	if comment.User.Login != "" {
		lines = append(lines, "author: "+yamlScalar("@"+comment.User.Login))
	}
	if comment.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(comment.CreatedAt))
	}
	if comment.UpdatedAt != "" {
		lines = append(lines, "updated: "+yamlScalar(comment.UpdatedAt))
	}
	lines = append(lines, "---", "", fmt.Sprintf("# Gist comment %d", comment.ID), "", "From: "+gistCanonicalURL(target, gist), "")
	body := ""
	if comment.Body != nil {
		body = strings.TrimSpace(stripInvisibleHTMLComments(*comment.Body))
	}
	if body == "" {
		body = "_Gist comment body is unavailable or empty._"
	}
	lines = append(lines, body)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func selectTextLines(content string, start, end int) (string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if start <= 0 || end < start || start > len(lines) || end > len(lines) {
		return "", fmt.Errorf("line range out of bounds")
	}
	return strings.Join(lines[start-1:end], "\n"), nil
}
