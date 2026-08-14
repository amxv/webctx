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

var githubSearchTypes = map[string]string{
	"repositories": "repositories",
	"issues":       "issues",
	"pullrequests": "issues",
	"code":         "code",
	"commits":      "commits",
	"users":        "users",
}

func isSupportedGitHubSearchURLQuery(query url.Values) bool {
	allowed := map[string]struct{}{"q": {}, "type": {}, "s": {}, "o": {}, "p": {}}
	for key := range query {
		if _, ok := allowed[key]; !ok {
			return false
		}
	}
	if strings.TrimSpace(query.Get("q")) == "" {
		return false
	}
	if _, ok := githubSearchTypes[query.Get("type")]; !ok {
		return false
	}
	if order := query.Get("o"); order != "" && order != "asc" && order != "desc" {
		return false
	}
	if page := query.Get("p"); page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n <= 0 {
			return false
		}
	}
	return true
}

func isSupportedGitHubProfileURLQuery(query url.Values) bool {
	for key := range query {
		if key != "tab" && key != "page" {
			return false
		}
	}
	if page := query.Get("page"); page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n <= 0 {
			return false
		}
	}
	if tab := query.Get("tab"); tab != "" {
		switch tab {
		case "repositories", "gists", "stars", "followers", "following", "people":
		default:
			return false
		}
	}
	return true
}

type githubSearchEnvelope struct {
	TotalCount        int             `json:"total_count"`
	IncompleteResults bool            `json:"incomplete_results"`
	Items             json.RawMessage `json:"items"`
}

type githubCodeSearchItem struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	SHA        string `json:"sha"`
	HTMLURL    string `json:"html_url"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

type githubCommitSearchItem struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func readGitHubSearch(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target == nil || target.Kind != GitHubTargetSearch || !isSupportedGitHubSearchURLQuery(target.Query) {
		return "", fmt.Errorf("GitHub Search URL is not supported by the native reader")
	}
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Search fragment %q is not a supported selector", target.Fragment)
	}
	uiType := target.Query.Get("type")
	apiType := githubSearchTypes[uiType]
	q := strings.TrimSpace(target.Query.Get("q"))
	if target.Owner != "" && target.Repo != "" {
		q += " repo:" + target.Owner + "/" + target.Repo
	}
	if uiType == "issues" {
		q += " is:issue"
	} else if uiType == "pullrequests" {
		q += " is:pr"
	}
	provider := url.Values{"q": []string{q}, "per_page": []string{strconv.Itoa(githubPageableListSize)}}
	if sort := target.Query.Get("s"); sort != "" {
		provider.Set("sort", sort)
	}
	if order := target.Query.Get("o"); order != "" {
		provider.Set("order", order)
	}
	if page := target.Query.Get("p"); page != "" {
		provider.Set("page", page)
	}
	endpoint := "/search/" + apiType + "?" + provider.Encode()
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var envelope githubSearchEnvelope
	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		return "", fmt.Errorf("decode GitHub Search results: %w", err)
	}
	return renderGitHubSearch(target, uiType, envelope, resp.Links())
}

func renderGitHubSearch(target *GitHubTarget, searchType string, envelope githubSearchEnvelope, links GitHubLinkRelations) (string, error) {
	page := target.Query.Get("p")
	if page == "" {
		page = "1"
	}
	var rawItems []json.RawMessage
	if len(envelope.Items) > 0 {
		if err := json.Unmarshal(envelope.Items, &rawItems); err != nil {
			return "", fmt.Errorf("decode GitHub Search result count: %w", err)
		}
	}
	returned := len(rawItems)
	indexed := returned
	queryPreview, queryTruncated := githubOverviewInlinePreview(target.Query.Get("q"), 300)
	lines := []string{
		"---",
		"view: " + yamlScalar("search:"+searchType),
		"query: " + yamlScalar(queryPreview),
		"page: " + page,
		fmt.Sprintf("total_count: %d", envelope.TotalCount),
		fmt.Sprintf("incomplete_results: %t", envelope.IncompleteResults),
		fmt.Sprintf("returned: %d", returned),
		fmt.Sprintf("indexed: %d", indexed),
		fmt.Sprintf("local_omitted: %d", returned-indexed),
	}
	if queryTruncated {
		lines = append(lines, "query_preview_truncated: true")
	}
	if target.Owner != "" && target.Repo != "" {
		lines = append(lines, "repository_scope: "+yamlScalar(target.Owner+"/"+target.Repo))
	}
	if envelope.TotalCount > 1000 {
		lines = append(lines, "provider_result_ceiling: 1000")
	}
	lines = append(lines, "---", "", "# GitHub search", "")
	if envelope.IncompleteResults {
		lines = append(lines, "> GitHub marked these search results incomplete.", "")
	}
	if envelope.TotalCount > 1000 {
		lines = append(lines, "> GitHub Search exposes at most 1000 results for a query; this bounded page does not imply access beyond that provider ceiling.", "")
	}
	switch searchType {
	case "repositories":
		var items []githubRepository
		if err := json.Unmarshal(envelope.Items, &items); err != nil {
			return "", fmt.Errorf("decode GitHub repository search items: %w", err)
		}
		for _, item := range items[:minInt(indexed, len(items))] {
			href := item.HTMLURL
			if href == "" {
				href = "https://github.com/" + item.FullName
			}
			meta := []string{fmt.Sprintf("%d stars", item.StargazersCount)}
			if item.Language != "" {
				meta = append(meta, item.Language)
			}
			name, nameTruncated := githubOverviewInlinePreview(item.FullName, 120)
			if nameTruncated {
				name += "…"
			}
			line := "- [" + escapeMarkdownLinkText(name) + "](" + href + ") — " + strings.Join(meta, " · ")
			if item.Description != "" {
				description, truncated := githubOverviewInlinePreview(item.Description, 140)
				if truncated {
					description += "…"
				}
				line += " — " + description
			}
			lines = append(lines, line)
		}
	case "issues", "pullrequests":
		var items []githubIssue
		if err := json.Unmarshal(envelope.Items, &items); err != nil {
			return "", fmt.Errorf("decode GitHub Issue search items: %w", err)
		}
		for _, item := range items[:minInt(indexed, len(items))] {
			meta := []string{item.State}
			if item.User.Login != "" {
				meta = append(meta, "@"+item.User.Login)
			}
			title, truncated := githubOverviewInlinePreview(item.Title, 140)
			if truncated {
				title += "…"
			}
			lines = append(lines, fmt.Sprintf("- [#%d %s](%s) — %s", item.Number, escapeMarkdownLinkText(title), item.HTMLURL, strings.Join(meta, " · ")))
		}
	case "code":
		var items []githubCodeSearchItem
		if err := json.Unmarshal(envelope.Items, &items); err != nil {
			return "", fmt.Errorf("decode GitHub code search items: %w", err)
		}
		for _, item := range items[:minInt(indexed, len(items))] {
			path, truncated := githubOverviewInlinePreview(item.Path, 140)
			if truncated {
				path += "…"
			}
			repo, repoTruncated := githubOverviewInlinePreview(item.Repository.FullName, 120)
			if repoTruncated {
				repo += "…"
			}
			lines = append(lines, fmt.Sprintf("- [%s](%s) — %s · `%s`", escapeMarkdownLinkText(path), item.HTMLURL, repo, shortSHA(item.SHA)))
		}
	case "commits":
		var items []githubCommitSearchItem
		if err := json.Unmarshal(envelope.Items, &items); err != nil {
			return "", fmt.Errorf("decode GitHub commit search items: %w", err)
		}
		for _, item := range items[:minInt(indexed, len(items))] {
			message, truncated := githubOverviewInlinePreview(firstLine(item.Commit.Message), 140)
			if truncated {
				message += "…"
			}
			line := fmt.Sprintf("- [`%s`](%s) %s", shortSHA(item.SHA), item.HTMLURL, message)
			if item.Repository.FullName != "" {
				repo, repoTruncated := githubOverviewInlinePreview(item.Repository.FullName, 120)
				if repoTruncated {
					repo += "…"
				}
				line += " — " + repo
			}
			if item.Commit.Author.Date != "" {
				line += " · " + item.Commit.Author.Date
			}
			lines = append(lines, line)
		}
	case "users":
		var items []githubUser
		if err := json.Unmarshal(envelope.Items, &items); err != nil {
			return "", fmt.Errorf("decode GitHub user search items: %w", err)
		}
		for _, item := range items[:minInt(indexed, len(items))] {
			line := renderGitHubUserIdentity(item)
			if item.Type != "" {
				line += " — " + item.Type
			}
			lines = append(lines, line)
		}
	}
	if returned == 0 {
		lines = append(lines, "_No results returned on this page._")
	}
	if nav := renderGitHubSearchNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "", "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func renderGitHubSearchNavigation(target *GitHubTarget, links GitHubLinkRelations) []string {
	lines := []string{}
	for _, rel := range []string{"prev", "next"} {
		page, ok := pageFromGitHubLink(links[rel])
		if !ok {
			continue
		}
		parsed, err := url.Parse(target.OriginalURL)
		if err != nil {
			continue
		}
		q := parsed.Query()
		q.Set("p", strconv.Itoa(page))
		parsed.RawQuery = q.Encode()
		label := "Previous"
		if rel == "next" {
			label = "Next"
		}
		lines = append(lines, "- "+label+": "+parsed.String())
	}
	return lines
}

type githubProfile struct {
	Login       string `json:"login"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Bio         string `json:"bio"`
	Blog        string `json:"blog"`
	Location    string `json:"location"`
	Company     string `json:"company"`
	HTMLURL     string `json:"html_url"`
	PublicRepos int    `json:"public_repos"`
	PublicGists int    `json:"public_gists"`
	Followers   int    `json:"followers"`
	Following   int    `json:"following"`
	CreatedAt   string `json:"created_at"`
}

type githubProfileGist struct {
	ID          string `json:"id"`
	HTMLURL     string `json:"html_url"`
	Description string `json:"description"`
	Files       map[string]struct {
		Filename string `json:"filename"`
	} `json:"files"`
}

func readGitHubProfile(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target == nil || target.Kind != GitHubTargetProfile || target.Owner == "" {
		return "", fmt.Errorf("GitHub profile target is invalid")
	}
	if target.Fragment != "" || !isSupportedGitHubProfileURLQuery(target.Query) {
		return "", fmt.Errorf("GitHub profile query/fragment is not supported")
	}
	resp, err := client.REST(ctx, http.MethodGet, "/users/"+url.PathEscape(target.Owner), "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var profile githubProfile
	if err := json.Unmarshal(resp.Body, &profile); err != nil {
		return "", fmt.Errorf("decode GitHub profile: %w", err)
	}
	if profile.Login == "" {
		profile.Login = target.Owner
	}
	if strings.EqualFold(profile.Type, "Organization") {
		orgResp, err := client.REST(ctx, http.MethodGet, "/orgs/"+url.PathEscape(target.Owner), "application/vnd.github+json")
		if err != nil {
			return "", err
		}
		var org githubProfile
		if err := json.Unmarshal(orgResp.Body, &org); err != nil {
			return "", fmt.Errorf("decode GitHub Organization profile: %w", err)
		}
		org.Type = "Organization"
		if org.Login == "" {
			org.Login = target.Owner
		}
		profile = org
	}
	tab := target.Query.Get("tab")
	if target.Name == "people" {
		tab = "people"
	}
	if tab == "" {
		return renderGitHubProfile(profile), nil
	}
	return readGitHubProfileTab(ctx, client, target, profile, tab)
}

func renderGitHubProfile(profile githubProfile) string {
	lines := []string{"---", "login: " + yamlScalar(profile.Login), "type: " + yamlScalar(profile.Type)}
	if profile.Name != "" {
		lines = append(lines, "name: "+yamlScalar(profile.Name))
	}
	lines = append(lines, fmt.Sprintf("public_repositories: %d", profile.PublicRepos))
	if !strings.EqualFold(profile.Type, "Organization") {
		lines = append(lines, fmt.Sprintf("public_gists: %d", profile.PublicGists), fmt.Sprintf("followers: %d", profile.Followers), fmt.Sprintf("following: %d", profile.Following))
	}
	if profile.Location != "" {
		lines = append(lines, "location: "+yamlScalar(profile.Location))
	}
	if profile.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(profile.CreatedAt))
	}
	if profile.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(profile.HTMLURL))
	}
	lines = append(lines, "---", "", "# "+profile.Login, "")
	if profile.Bio != "" {
		lines = append(lines, profile.Bio, "")
	}
	base := "https://github.com/" + url.PathEscape(profile.Login)
	lines = append(lines, "## Useful GitHub URLs", "", "- Repositories: "+base+"?tab=repositories")
	if strings.EqualFold(profile.Type, "Organization") {
		lines = append(lines, "- Public members: https://github.com/orgs/"+url.PathEscape(profile.Login)+"/people")
	} else {
		lines = append(lines, "- Gists: "+base+"?tab=gists", "- Stars: "+base+"?tab=stars", "- Followers: "+base+"?tab=followers", "- Following: "+base+"?tab=following")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubProfileTab(ctx context.Context, client *GitHubClient, target *GitHubTarget, profile githubProfile, tab string) (string, error) {
	page := target.Query.Get("page")
	provider := url.Values{"per_page": []string{strconv.Itoa(githubPageableListSize)}}
	if page != "" {
		provider.Set("page", page)
	}
	endpoint := ""
	kind := ""
	isOrg := strings.EqualFold(profile.Type, "Organization")
	if isOrg {
		switch tab {
		case "repositories":
			endpoint, kind = "/orgs/"+url.PathEscape(profile.Login)+"/repos", "repositories"
		case "people":
			endpoint, kind = "/orgs/"+url.PathEscape(profile.Login)+"/public_members", "people"
		default:
			return "", fmt.Errorf("GitHub Organization profile tab %q is not supported", tab)
		}
	} else {
		switch tab {
		case "repositories":
			endpoint, kind = "/users/"+url.PathEscape(profile.Login)+"/repos", "repositories"
		case "stars":
			endpoint, kind = "/users/"+url.PathEscape(profile.Login)+"/starred", "repositories"
		case "gists":
			endpoint, kind = "/users/"+url.PathEscape(profile.Login)+"/gists", "gists"
		case "followers":
			endpoint, kind = "/users/"+url.PathEscape(profile.Login)+"/followers", "users"
		case "following":
			endpoint, kind = "/users/"+url.PathEscape(profile.Login)+"/following", "users"
		default:
			return "", fmt.Errorf("GitHub User profile tab %q is not supported", tab)
		}
	}
	resp, err := client.REST(ctx, http.MethodGet, endpoint+"?"+provider.Encode(), "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	if page == "" {
		page = "1"
	}
	rows := []string{}
	returned := 0
	indexed := 0
	switch kind {
	case "repositories":
		var items []githubRepository
		if err := json.Unmarshal(resp.Body, &items); err != nil {
			return "", fmt.Errorf("decode GitHub profile repositories: %w", err)
		}
		returned = len(items)
		indexed = returned
		for _, item := range items[:indexed] {
			name, truncated := githubOverviewInlinePreview(item.FullName, 120)
			if truncated {
				name += "…"
			}
			rows = append(rows, fmt.Sprintf("- [%s](%s) — %d stars", escapeMarkdownLinkText(name), item.HTMLURL, item.StargazersCount))
		}
	case "gists":
		var items []githubProfileGist
		if err := json.Unmarshal(resp.Body, &items); err != nil {
			return "", fmt.Errorf("decode GitHub profile Gists: %w", err)
		}
		returned = len(items)
		indexed = returned
		for _, item := range items[:indexed] {
			line := "- [" + item.ID + "](" + item.HTMLURL + ")"
			if item.Description != "" {
				description, truncated := githubOverviewInlinePreview(item.Description, 140)
				if truncated {
					description += "…"
				}
				line += " — " + description
			}
			rows = append(rows, line)
		}
	case "users", "people":
		var items []githubUser
		if err := json.Unmarshal(resp.Body, &items); err != nil {
			return "", fmt.Errorf("decode GitHub profile users: %w", err)
		}
		returned = len(items)
		indexed = returned
		for _, item := range items[:indexed] {
			rows = append(rows, renderGitHubUserIdentity(item))
		}
	}
	lines := []string{"---", "login: " + yamlScalar(profile.Login), "type: " + yamlScalar(profile.Type), "view: " + yamlScalar(tab), "page: " + page, fmt.Sprintf("returned: %d", returned), fmt.Sprintf("indexed: %d", indexed), fmt.Sprintf("local_omitted: %d", returned-indexed), "---", "", "# " + profile.Login + " — " + tab, ""}
	lines = append(lines, rows...)
	if nav := renderGitHubUIPageNavigation(target, resp.Links()); len(nav) > 0 {
		lines = append(lines, "", "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}
