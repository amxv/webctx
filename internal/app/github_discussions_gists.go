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
	ID          string
	Body        string
	URL         string
	CreatedAt   string
	UpdatedAt   string
	UpvoteCount int
	Author      string
	Replies     []githubDiscussionComment
}

type githubDiscussionDetail struct {
	githubDiscussionSummary
	Body     string
	AnswerID string
	Comments []githubDiscussionComment
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
	HTMLURL   string     `json:"html_url"`
}

type gistFileSelector struct {
	Slug  string
	Start int
	End   int
}

var gistLineSuffixRE = regexp.MustCompile(`^(file-.+?)(?:-L([0-9]+)(?:-L([0-9]+))?)?$`)

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
	if target.Fragment != "" || len(target.Query) > 0 {
		return "", fmt.Errorf("GitHub Discussion fragment/query selection is not yet part of the native Discussion contract")
	}
	detail, err := fetchGitHubDiscussion(ctx, client, target)
	if err != nil {
		return "", err
	}
	return renderGitHubDiscussion(target, detail), nil
}

func fetchGitHubDiscussion(ctx context.Context, client *GitHubClient, target *GitHubTarget) (githubDiscussionDetail, error) {
	const query = `query($owner:String!,$repo:String!,$number:Int!,$after:String){repository(owner:$owner,name:$repo){discussion(number:$number){number title body url createdAt updatedAt upvoteCount locked author{login} category{name} answer{id} comments(first:50,after:$after){nodes{id body url createdAt updatedAt upvoteCount author{login} replies(first:50){nodes{id body url createdAt updatedAt upvoteCount author{login}} pageInfo{hasNextPage endCursor}}} pageInfo{hasNextPage endCursor}}}}}`
	var detail githubDiscussionDetail
	var after any
	for {
		var data struct {
			Repository *struct {
				Discussion *struct {
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
					Answer *struct {
						ID string `json:"id"`
					} `json:"answer"`
					Comments struct {
						Nodes []struct {
							ID          string `json:"id"`
							Body        string `json:"body"`
							URL         string `json:"url"`
							CreatedAt   string `json:"createdAt"`
							UpdatedAt   string `json:"updatedAt"`
							UpvoteCount int    `json:"upvoteCount"`
							Author      *struct {
								Login string `json:"login"`
							} `json:"author"`
							Replies struct {
								Nodes []struct {
									ID          string `json:"id"`
									Body        string `json:"body"`
									URL         string `json:"url"`
									CreatedAt   string `json:"createdAt"`
									UpdatedAt   string `json:"updatedAt"`
									UpvoteCount int    `json:"upvoteCount"`
									Author      *struct {
										Login string `json:"login"`
									} `json:"author"`
								} `json:"nodes"`
								PageInfo struct {
									HasNextPage bool   `json:"hasNextPage"`
									EndCursor   string `json:"endCursor"`
								} `json:"pageInfo"`
							} `json:"replies"`
						} `json:"nodes"`
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
					} `json:"comments"`
				} `json:"discussion"`
			} `json:"repository"`
		}
		vars := map[string]any{"owner": target.Owner, "repo": target.Repo, "number": target.Number, "after": after}
		if err := client.GraphQL(ctx, query, vars, &data); err != nil {
			return githubDiscussionDetail{}, err
		}
		if data.Repository == nil || data.Repository.Discussion == nil {
			return githubDiscussionDetail{}, fmt.Errorf("GitHub Discussion #%d was not available", target.Number)
		}
		d := data.Repository.Discussion
		if detail.Number == 0 {
			detail.githubDiscussionSummary = githubDiscussionSummary{Number: d.Number, Title: d.Title, URL: d.URL, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, UpvoteCount: d.UpvoteCount, Locked: d.Locked, Category: d.Category.Name}
			detail.Body = d.Body
			if d.Author != nil {
				detail.Author = d.Author.Login
			}
			if d.Answer != nil {
				detail.AnswerID = d.Answer.ID
			}
		}
		for _, node := range d.Comments.Nodes {
			comment := githubDiscussionComment{ID: node.ID, Body: node.Body, URL: node.URL, CreatedAt: node.CreatedAt, UpdatedAt: node.UpdatedAt, UpvoteCount: node.UpvoteCount}
			if node.Author != nil {
				comment.Author = node.Author.Login
			}
			for _, replyNode := range node.Replies.Nodes {
				reply := githubDiscussionComment{ID: replyNode.ID, Body: replyNode.Body, URL: replyNode.URL, CreatedAt: replyNode.CreatedAt, UpdatedAt: replyNode.UpdatedAt, UpvoteCount: replyNode.UpvoteCount}
				if replyNode.Author != nil {
					reply.Author = replyNode.Author.Login
				}
				comment.Replies = append(comment.Replies, reply)
			}
			if node.Replies.PageInfo.HasNextPage {
				extra, err := fetchGitHubDiscussionReplies(ctx, client, node.ID, node.Replies.PageInfo.EndCursor)
				if err != nil {
					return githubDiscussionDetail{}, err
				}
				comment.Replies = append(comment.Replies, extra...)
			}
			detail.Comments = append(detail.Comments, comment)
		}
		if !d.Comments.PageInfo.HasNextPage {
			break
		}
		after = d.Comments.PageInfo.EndCursor
	}
	return detail, nil
}

func fetchGitHubDiscussionReplies(ctx context.Context, client *GitHubClient, commentID, cursor string) ([]githubDiscussionComment, error) {
	const query = `query($id:ID!,$after:String){node(id:$id){... on DiscussionComment{replies(first:50,after:$after){nodes{id body url createdAt updatedAt upvoteCount author{login}} pageInfo{hasNextPage endCursor}}}}}`
	replies := []githubDiscussionComment{}
	var after any = cursor
	for {
		var data struct {
			Node *struct {
				Replies struct {
					Nodes []struct {
						ID          string `json:"id"`
						Body        string `json:"body"`
						URL         string `json:"url"`
						CreatedAt   string `json:"createdAt"`
						UpdatedAt   string `json:"updatedAt"`
						UpvoteCount int    `json:"upvoteCount"`
						Author      *struct {
							Login string `json:"login"`
						} `json:"author"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"replies"`
			} `json:"node"`
		}
		if err := client.GraphQL(ctx, query, map[string]any{"id": commentID, "after": after}, &data); err != nil {
			return nil, err
		}
		if data.Node == nil {
			return nil, fmt.Errorf("GitHub Discussion comment reply connection was unavailable")
		}
		for _, node := range data.Node.Replies.Nodes {
			reply := githubDiscussionComment{ID: node.ID, Body: node.Body, URL: node.URL, CreatedAt: node.CreatedAt, UpdatedAt: node.UpdatedAt, UpvoteCount: node.UpvoteCount}
			if node.Author != nil {
				reply.Author = node.Author.Login
			}
			replies = append(replies, reply)
		}
		if !data.Node.Replies.PageInfo.HasNextPage {
			break
		}
		after = data.Node.Replies.PageInfo.EndCursor
	}
	return replies, nil
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
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), fmt.Sprintf("discussion: %d", detail.Number), "title: " + yamlScalar(detail.Title), "category: " + yamlScalar(detail.Category), fmt.Sprintf("locked: %t", detail.Locked), fmt.Sprintf("upvotes: %d", detail.UpvoteCount), fmt.Sprintf("comments: %d", len(detail.Comments))}
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
	lines = append(lines, "---", "", "# "+detail.Title, "")
	body := strings.TrimSpace(stripInvisibleHTMLComments(detail.Body))
	if body == "" {
		body = "_No Discussion body._"
	}
	lines = append(lines, body, "", "## Conversation", "")
	if len(detail.Comments) == 0 {
		lines = append(lines, "_No comments._")
	}
	for _, comment := range detail.Comments {
		answer := comment.ID != "" && comment.ID == detail.AnswerID
		lines = append(lines, renderDiscussionComment(comment, answer, 3)...)
		for _, reply := range comment.Replies {
			lines = append(lines, renderDiscussionComment(reply, false, 4)...)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderDiscussionComment(comment githubDiscussionComment, answer bool, level int) []string {
	heading := strings.Repeat("#", level) + " "
	if answer {
		heading += "Accepted answer"
	} else if level > 3 {
		heading += "Reply"
	} else {
		heading += "Comment"
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
	lines = append(lines, body, "")
	return lines
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
	comments, err := fetchGitHubGistComments(ctx, client, target.Name)
	if err != nil {
		return "", err
	}
	selector, hasSelector, err := parseGistFileSelector(target.Fragment)
	if err != nil {
		return "", err
	}
	files := make([]githubGistFile, 0, len(gist.Files))
	for _, file := range gist.Files {
		files = append(files, file)
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Filename < files[j].Filename })
	if hasSelector {
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
		files = []githubGistFile{files[matched]}
	}
	for i := range files {
		if files[i].Truncated && files[i].RawURL != "" {
			if raw, ok := fetchGitHubGistRaw(ctx, client, files[i].RawURL); ok {
				files[i].Content = raw
				files[i].Truncated = false
			}
		}
	}
	return renderGitHubGist(target, gist, files, comments, selector, hasSelector), nil
}

func fetchGitHubGistComments(ctx context.Context, client *GitHubClient, gistID string) ([]githubGistComment, error) {
	endpoint := fmt.Sprintf("/gists/%s/comments?per_page=100", url.PathEscape(gistID))
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	comments := []githubGistComment{}
	for _, page := range pages {
		var batch []githubGistComment
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub Gist comments: %w", err)
		}
		comments = append(comments, batch...)
	}
	return comments, nil
}

func fetchGitHubGistRaw(ctx context.Context, client *GitHubClient, rawURL string) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", githubUserAgent)
	resp, err := client.doer.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || resp.ContentLength > githubBlobMaxBytes {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, githubBlobMaxBytes+1))
	if err != nil || len(body) > githubBlobMaxBytes || !utf8.Valid(body) {
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

func renderGitHubGist(target *GitHubTarget, gist githubGist, files []githubGistFile, comments []githubGistComment, selector gistFileSelector, hasSelector bool) string {
	lines := []string{"---", "gist: " + yamlScalar(gist.ID), fmt.Sprintf("public: %t", gist.Public), fmt.Sprintf("files: %d", len(files)), fmt.Sprintf("comments: %d", len(comments))}
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
	if gist.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(gist.HTMLURL))
	}
	if hasSelector {
		lines = append(lines, "selector: "+yamlScalar(target.Fragment))
	}
	lines = append(lines, "---", "", "# Gist "+gist.ID, "")
	for _, file := range files {
		lines = append(lines, "## "+file.Filename, "")
		meta := []string{}
		if file.Language != "" {
			meta = append(meta, file.Language)
		}
		if file.Size > 0 {
			meta = append(meta, fmt.Sprintf("%d bytes", file.Size))
		}
		if len(meta) > 0 {
			lines = append(lines, "_"+strings.Join(meta, " · ")+"_", "")
		}
		content := file.Content
		if selector.Start > 0 {
			selected, err := selectTextLines(content, selector.Start, selector.End)
			if err != nil {
				lines = append(lines, "_Selected line range is out of range for the Gist file._", "")
			} else {
				content = selected
				lines = append(lines, fmt.Sprintf("**Lines %d-%d:**", selector.Start, selector.End), "")
			}
		}
		if file.Truncated {
			lines = append(lines, "_GitHub marked this API file content truncated; it is not presented as complete._")
			if file.RawURL != "" {
				lines = append(lines, "Full raw file: "+file.RawURL)
			}
			lines = append(lines, "")
			continue
		}
		fence := "```"
		if strings.Contains(content, "```") {
			fence = "````"
		}
		lines = append(lines, fence, content, fence, "")
	}
	if !hasSelector {
		lines = append(lines, "## Comments", "")
		if len(comments) == 0 {
			lines = append(lines, "_No Gist comments._")
		}
		for _, comment := range comments {
			heading := "### Comment"
			if comment.User.Login != "" {
				heading += " by @" + comment.User.Login
			}
			if comment.CreatedAt != "" {
				heading += " — " + comment.CreatedAt
			}
			lines = append(lines, heading, "")
			body := ""
			if comment.Body != nil {
				body = strings.TrimSpace(stripInvisibleHTMLComments(*comment.Body))
			}
			if body == "" {
				body = "_Gist comment body is unavailable or empty._"
			}
			lines = append(lines, body, "")
		}
		if len(gist.History) > 0 {
			lines = append(lines, "## Revisions", "")
			for _, revision := range gist.History {
				line := "- `" + shortSHA(revision.Version) + "`"
				if revision.CommittedAt != "" {
					line += " — " + revision.CommittedAt
				}
				line += fmt.Sprintf(" — +%d -%d (%d changes)", revision.ChangeStatus.Additions, revision.ChangeStatus.Deletions, revision.ChangeStatus.Total)
				lines = append(lines, line)
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func selectTextLines(content string, start, end int) (string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if start <= 0 || end < start || start > len(lines) || end > len(lines) {
		return "", fmt.Errorf("line range out of bounds")
	}
	return strings.Join(lines[start-1:end], "\n"), nil
}
