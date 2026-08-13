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

var githubPackageTypes = map[string]struct{}{
	"container": {}, "docker": {}, "maven": {}, "npm": {}, "nuget": {}, "rubygems": {},
}

type githubPackage struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	PackageType  string     `json:"package_type"`
	HTMLURL      string     `json:"html_url"`
	Visibility   string     `json:"visibility"`
	Description  *string    `json:"description"`
	VersionCount int        `json:"version_count"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
	Owner        githubUser `json:"owner"`
	Repository   *struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
}

type githubPackageVersion struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	PackageHTMLURL string `json:"package_html_url"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	Metadata       struct {
		PackageType string `json:"package_type"`
		Container   struct {
			Tags []string `json:"tags"`
		} `json:"container"`
	} `json:"metadata"`
}

type githubProjectV2 struct {
	ID               string
	Number           int
	Title            string
	ShortDescription string
	URL              string
	Public           bool
	Closed           bool
	CreatedAt        string
	UpdatedAt        string
	Items            []githubProjectV2Item
	MoreItems        bool
}

type githubProjectV2Item struct {
	ID         string
	Type       string
	Title      string
	URL        string
	State      string
	Repository string
	Number     int
	Body       string
}

func isSupportedGitHubPackageType(packageType string) bool {
	_, ok := githubPackageTypes[packageType]
	return ok
}

func readGitHubPackage(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub Package fragment %q is not a supported native selector", target.Fragment)
	}
	for key := range target.Query {
		if key != "page" {
			return "", fmt.Errorf("GitHub Package query parameter %q is not supported by the native reader", key)
		}
	}
	page := target.Query.Get("page")
	if page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n <= 0 {
			return "", fmt.Errorf("invalid GitHub Package page %q", page)
		}
	}
	if len(target.Tail) != 2 {
		return "", fmt.Errorf("GitHub Package target is missing scope/type identity")
	}
	scope, packageType := target.Tail[0], target.Tail[1]
	base, err := githubPackageAPIBase(scope, target.Owner, packageType, target.Name)
	if err != nil {
		return "", err
	}
	resp, err := client.REST(ctx, http.MethodGet, base, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var pkg githubPackage
	if err := json.Unmarshal(resp.Body, &pkg); err != nil {
		return "", fmt.Errorf("decode GitHub Package: %w", err)
	}
	q := url.Values{"per_page": []string{"30"}}
	if page != "" {
		q.Set("page", page)
	}
	versionsResp, err := client.REST(ctx, http.MethodGet, base+"/versions?"+q.Encode(), "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var versions []githubPackageVersion
	if err := json.Unmarshal(versionsResp.Body, &versions); err != nil {
		return "", fmt.Errorf("decode GitHub Package versions: %w", err)
	}
	return renderGitHubPackage(target, pkg, versions, versionsResp.Links()), nil
}

func githubPackageAPIBase(scope, owner, packageType, name string) (string, error) {
	prefix := ""
	switch scope {
	case "org":
		prefix = "/orgs/" + url.PathEscape(owner)
	case "user":
		prefix = "/users/" + url.PathEscape(owner)
	default:
		return "", fmt.Errorf("unsupported GitHub Package scope %q", scope)
	}
	return fmt.Sprintf("%s/packages/%s/%s", prefix, url.PathEscape(packageType), url.PathEscape(name)), nil
}

func renderGitHubPackage(target *GitHubTarget, pkg githubPackage, versions []githubPackageVersion, links GitHubLinkRelations) string {
	page := target.Query.Get("page")
	if page == "" {
		page = "1"
	}
	lines := []string{
		"---",
		"owner: " + yamlScalar(target.Owner),
		"package: " + yamlScalar(pkg.Name),
		"package_type: " + yamlScalar(pkg.PackageType),
		"visibility: " + yamlScalar(pkg.Visibility),
		fmt.Sprintf("version_count: %d", pkg.VersionCount),
		"versions_page: " + page,
		fmt.Sprintf("versions_returned: %d", len(versions)),
	}
	if pkg.Repository != nil && pkg.Repository.FullName != "" {
		lines = append(lines, "repository: "+yamlScalar(pkg.Repository.FullName))
	}
	if pkg.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(pkg.CreatedAt))
	}
	if pkg.UpdatedAt != "" {
		lines = append(lines, "updated: "+yamlScalar(pkg.UpdatedAt))
	}
	if pkg.HTMLURL != "" {
		lines = append(lines, "url: "+yamlScalar(pkg.HTMLURL))
	}
	lines = append(lines, "---", "", "# Package: "+pkg.Name, "")
	if pkg.Description != nil && strings.TrimSpace(*pkg.Description) != "" {
		lines = append(lines, strings.TrimSpace(*pkg.Description), "")
	}
	lines = append(lines, "## Versions", "")
	if len(versions) == 0 {
		lines = append(lines, "_No package versions returned on this page._")
	}
	for _, version := range versions {
		name := version.Name
		if name == "" {
			name = fmt.Sprintf("version %d", version.ID)
		}
		line := "- " + name
		if len(version.Metadata.Container.Tags) > 0 {
			line += " — tags: " + strings.Join(version.Metadata.Container.Tags, ", ")
		}
		if version.UpdatedAt != "" {
			line += " — updated " + version.UpdatedAt
		}
		if version.PackageHTMLURL != "" {
			line += " — " + version.PackageHTMLURL
		}
		lines = append(lines, line)
	}
	if nav := renderGitHubUIPageNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "", "## Version navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubProjectV2(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" || len(target.Query) > 0 {
		return "", fmt.Errorf("GitHub Projects v2 query/fragment selectors are not part of the native project contract")
	}
	if len(target.Tail) != 1 || (target.Tail[0] != "org" && target.Tail[0] != "user") {
		return "", fmt.Errorf("GitHub Projects v2 target is missing owner scope")
	}
	project, err := fetchGitHubProjectV2(ctx, client, target)
	if err != nil {
		return "", err
	}
	return renderGitHubProjectV2(target, project), nil
}

func fetchGitHubProjectV2(ctx context.Context, client *GitHubClient, target *GitHubTarget) (githubProjectV2, error) {
	prefix := "/orgs/" + url.PathEscape(target.Owner)
	if target.Tail[0] == "user" {
		prefix = "/users/" + url.PathEscape(target.Owner)
	}
	base := fmt.Sprintf("%s/projectsV2/%d", prefix, target.Number)
	projectResp, err := client.RESTPublic(ctx, http.MethodGet, base, "application/vnd.github+json")
	if err != nil {
		return githubProjectV2{}, err
	}
	var raw struct {
		ID               int64   `json:"id"`
		Number           int     `json:"number"`
		Title            string  `json:"title"`
		Description      *string `json:"description"`
		ShortDescription *string `json:"short_description"`
		Public           bool    `json:"public"`
		State            string  `json:"state"`
		CreatedAt        string  `json:"created_at"`
		UpdatedAt        string  `json:"updated_at"`
	}
	if err := json.Unmarshal(projectResp.Body, &raw); err != nil {
		return githubProjectV2{}, fmt.Errorf("decode GitHub Project v2: %w", err)
	}
	description := ""
	if raw.ShortDescription != nil {
		description = strings.TrimSpace(*raw.ShortDescription)
	}
	if description == "" && raw.Description != nil {
		description = strings.TrimSpace(*raw.Description)
	}
	project := githubProjectV2{
		ID:               strconv.FormatInt(raw.ID, 10),
		Number:           raw.Number,
		Title:            raw.Title,
		ShortDescription: description,
		URL:              target.OriginalURL,
		Public:           raw.Public,
		Closed:           strings.EqualFold(raw.State, "closed"),
		CreatedAt:        raw.CreatedAt,
		UpdatedAt:        raw.UpdatedAt,
	}
	itemsResp, err := client.RESTPublic(ctx, http.MethodGet, base+"/items?per_page=50", "application/vnd.github+json")
	if err != nil {
		return githubProjectV2{}, err
	}
	var rawItems []struct {
		ID          int64           `json:"id"`
		ContentType string          `json:"content_type"`
		Content     json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(itemsResp.Body, &rawItems); err != nil {
		return githubProjectV2{}, fmt.Errorf("decode GitHub Project v2 items: %w", err)
	}
	project.MoreItems = strings.TrimSpace(itemsResp.Links()["next"]) != ""
	for _, rawItem := range rawItems {
		item := githubProjectV2Item{ID: strconv.FormatInt(rawItem.ID, 10), Type: rawItem.ContentType}
		if len(rawItem.Content) > 0 && string(rawItem.Content) != "null" {
			var content struct {
				Number        int    `json:"number"`
				Title         string `json:"title"`
				Body          string `json:"body"`
				State         string `json:"state"`
				HTMLURL       string `json:"html_url"`
				RepositoryURL string `json:"repository_url"`
				Repository    *struct {
					FullName      string `json:"full_name"`
					NameWithOwner string `json:"nameWithOwner"`
				} `json:"repository"`
			}
			if err := json.Unmarshal(rawItem.Content, &content); err != nil {
				return githubProjectV2{}, fmt.Errorf("decode GitHub Project v2 item content: %w", err)
			}
			item.Title, item.URL, item.State, item.Number, item.Body = content.Title, content.HTMLURL, content.State, content.Number, content.Body
			if content.Repository != nil {
				item.Repository = content.Repository.FullName
				if item.Repository == "" {
					item.Repository = content.Repository.NameWithOwner
				}
			}
			if item.Repository == "" && content.RepositoryURL != "" {
				if parsed, parseErr := url.Parse(content.RepositoryURL); parseErr == nil {
					parts := splitGitHubPath(parsed.Path)
					if len(parts) >= 3 && parts[0] == "repos" {
						item.Repository = parts[1] + "/" + parts[2]
					}
				}
			}
		}
		project.Items = append(project.Items, item)
	}
	return project, nil
}

func renderGitHubProjectV2(target *GitHubTarget, project githubProjectV2) string {
	lines := []string{
		"---",
		"owner: " + yamlScalar(target.Owner),
		fmt.Sprintf("project: %d", project.Number),
		"title: " + yamlScalar(project.Title),
		fmt.Sprintf("public: %t", project.Public),
		fmt.Sprintf("closed: %t", project.Closed),
		fmt.Sprintf("items_returned: %d", len(project.Items)),
		fmt.Sprintf("more_items_available: %t", project.MoreItems),
	}
	if project.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(project.CreatedAt))
	}
	if project.UpdatedAt != "" {
		lines = append(lines, "updated: "+yamlScalar(project.UpdatedAt))
	}
	if project.URL != "" {
		lines = append(lines, "url: "+yamlScalar(project.URL))
	}
	lines = append(lines, "---", "", "# "+project.Title, "")
	if strings.TrimSpace(project.ShortDescription) != "" {
		lines = append(lines, strings.TrimSpace(project.ShortDescription), "")
	}
	lines = append(lines, "## Items", "")
	if len(project.Items) == 0 {
		lines = append(lines, "_No Project items returned._")
	}
	for _, item := range project.Items {
		label := item.Title
		if label == "" {
			label = item.Type
		}
		line := "- "
		if item.URL != "" {
			line += "[" + escapeMarkdownLinkText(label) + "](" + item.URL + ")"
		} else {
			line += label
		}
		meta := []string{}
		if item.Repository != "" {
			meta = append(meta, item.Repository)
		}
		if item.Number > 0 {
			meta = append(meta, fmt.Sprintf("#%d", item.Number))
		}
		if item.State != "" {
			meta = append(meta, item.State)
		}
		if item.Type != "" {
			meta = append(meta, item.Type)
		}
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	if project.MoreItems {
		lines = append(lines, "", "> More Project items exist upstream. This native long-tail Project view intentionally returns the first 50 items rather than expanding an unbounded project into context.")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
