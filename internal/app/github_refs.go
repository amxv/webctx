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

type githubBranch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	Commit    struct {
		SHA string `json:"sha"`
		URL string `json:"url"`
	} `json:"commit"`
}

type githubTag struct {
	Name       string `json:"name"`
	ZipballURL string `json:"zipball_url"`
	TarballURL string `json:"tarball_url"`
	Commit     struct {
		SHA string `json:"sha"`
		URL string `json:"url"`
	} `json:"commit"`
}

type githubReleaseAsset struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Label              string     `json:"label"`
	ContentType        string     `json:"content_type"`
	State              string     `json:"state"`
	Size               int64      `json:"size"`
	DownloadCount      int        `json:"download_count"`
	BrowserDownloadURL string     `json:"browser_download_url"`
	CreatedAt          string     `json:"created_at"`
	UpdatedAt          string     `json:"updated_at"`
	Uploader           githubUser `json:"uploader"`
}

type githubRelease struct {
	ID          int64                `json:"id"`
	TagName     string               `json:"tag_name"`
	Target      string               `json:"target_commitish"`
	Name        string               `json:"name"`
	Body        *string              `json:"body"`
	HTMLURL     string               `json:"html_url"`
	Draft       bool                 `json:"draft"`
	Prerelease  bool                 `json:"prerelease"`
	CreatedAt   string               `json:"created_at"`
	PublishedAt string               `json:"published_at"`
	Author      githubUser           `json:"author"`
	Assets      []githubReleaseAsset `json:"assets"`
}

type githubReleaseAssetsAvailability struct {
	ProviderMore bool
}

type githubFork struct {
	FullName        string     `json:"full_name"`
	HTMLURL         string     `json:"html_url"`
	Description     *string    `json:"description"`
	Language        *string    `json:"language"`
	DefaultBranch   string     `json:"default_branch"`
	StargazersCount int        `json:"stargazers_count"`
	ForksCount      int        `json:"forks_count"`
	Archived        bool       `json:"archived"`
	UpdatedAt       string     `json:"updated_at"`
	Owner           githubUser `json:"owner"`
}

type githubStar struct {
	StarredAt string     `json:"starred_at"`
	User      githubUser `json:"user"`
}

func readGitHubBranches(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub branches fragment %q is not a supported native selector", target.Fragment)
	}
	query, err := boundedListQuery(target.Query, []string{"page", "protected"})
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/branches?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.RESTPublic(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var branches []githubBranch
	if err := json.Unmarshal(resp.Body, &branches); err != nil {
		return "", fmt.Errorf("decode GitHub branches: %w", err)
	}
	return renderGitHubBranches(target, branches, resp.Links()), nil
}

func readGitHubTags(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub tags fragment %q is not a supported native selector", target.Fragment)
	}
	query, err := boundedListQuery(target.Query, []string{"page"})
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/tags?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var tags []githubTag
	if err := json.Unmarshal(resp.Body, &tags); err != nil {
		return "", fmt.Errorf("decode GitHub tags: %w", err)
	}
	return renderGitHubTags(target, tags, resp.Links()), nil
}

func readGitHubReleases(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub releases fragment %q is not a supported native selector", target.Fragment)
	}
	query, err := boundedListQuery(target.Query, []string{"page"})
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/releases?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var releases []githubRelease
	if err := json.Unmarshal(resp.Body, &releases); err != nil {
		return "", fmt.Errorf("decode GitHub releases: %w", err)
	}
	return renderGitHubReleases(target, releases, resp.Links()), nil
}

func readGitHubRelease(ctx context.Context, client *GitHubClient, target *GitHubTarget, latest bool) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub release fragment %q is not a supported native selector", target.Fragment)
	}
	if len(target.Query) > 0 {
		return "", fmt.Errorf("GitHub release detail query parameters are not part of the native release contract")
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/releases/latest", url.PathEscape(target.Owner), url.PathEscape(target.Repo))
	if !latest {
		endpoint = fmt.Sprintf("/repos/%s/%s/releases/tags/%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(target.Name))
	}
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var release githubRelease
	if err := json.Unmarshal(resp.Body, &release); err != nil {
		return "", fmt.Errorf("decode GitHub release: %w", err)
	}
	assets, availability, err := fetchGitHubReleaseAssets(ctx, client, target, release.ID)
	if err != nil {
		return "", err
	}
	sortReleaseAssetsByName(assets)
	release.Assets = assets
	return renderGitHubRelease(target, release, availability), nil
}

func fetchGitHubReleaseAssets(ctx context.Context, client *GitHubClient, target *GitHubTarget, releaseID int64) ([]githubReleaseAsset, githubReleaseAssetsAvailability, error) {
	if releaseID <= 0 {
		return nil, githubReleaseAssetsAvailability{}, fmt.Errorf("GitHub release did not report a release ID")
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/releases/%d/assets?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), releaseID)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, githubReleaseAssetsAvailability{}, err
	}
	var assets []githubReleaseAsset
	if err := json.Unmarshal(resp.Body, &assets); err != nil {
		return nil, githubReleaseAssetsAvailability{}, fmt.Errorf("decode GitHub release assets: %w", err)
	}
	return assets, githubReleaseAssetsAvailability{ProviderMore: resp.Links()["next"] != ""}, nil
}

func readGitHubForks(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub forks fragment %q is not a supported native selector", target.Fragment)
	}
	query, err := boundedListQuery(target.Query, []string{"page", "sort"})
	if err != nil {
		return "", err
	}
	if sortValue := query.Get("sort"); sortValue != "" && sortValue != "newest" && sortValue != "oldest" && sortValue != "stargazers" && sortValue != "watchers" {
		return "", fmt.Errorf("unsupported GitHub forks sort %q", sortValue)
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/forks?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var forks []githubFork
	if err := json.Unmarshal(resp.Body, &forks); err != nil {
		return "", fmt.Errorf("decode GitHub forks: %w", err)
	}
	return renderGitHubForks(target, forks, resp.Links()), nil
}

func readGitHubStargazers(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub stargazers fragment %q is not a supported native selector", target.Fragment)
	}
	query, err := boundedListQuery(target.Query, []string{"page"})
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/stargazers?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.RESTPublic(ctx, http.MethodGet, endpoint, "application/vnd.github.star+json")
	if err != nil {
		return "", err
	}
	var stars []githubStar
	if err := json.Unmarshal(resp.Body, &stars); err != nil {
		return "", fmt.Errorf("decode GitHub stargazers: %w", err)
	}
	return renderGitHubStargazers(target, stars, resp.Links()), nil
}

func readGitHubWatchers(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub watchers fragment %q is not a supported native selector", target.Fragment)
	}
	query, err := boundedListQuery(target.Query, []string{"page"})
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/subscribers?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), query.Encode())
	resp, err := client.RESTPublic(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var users []githubUser
	if err := json.Unmarshal(resp.Body, &users); err != nil {
		return "", fmt.Errorf("decode GitHub subscribers: %w", err)
	}
	return renderGitHubWatchers(target, users, resp.Links()), nil
}

func boundedListQuery(input url.Values, allowed []string) (url.Values, error) {
	allowedSet := map[string]struct{}{}
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range input {
		if _, ok := allowedSet[key]; !ok {
			return nil, fmt.Errorf("GitHub list query parameter %q is not supported by this native view", key)
		}
	}
	query := copySelectedQuery(input, allowed)
	query.Set("per_page", strconv.Itoa(githubPageableListSize))
	if rawPage := query.Get("page"); rawPage != "" {
		page, err := strconv.Atoi(rawPage)
		if err != nil || page <= 0 {
			return nil, fmt.Errorf("invalid GitHub list page %q", rawPage)
		}
	}
	return query, nil
}

func renderGitHubBranches(target *GitHubTarget, branches []githubBranch, links GitHubLinkRelations) string {
	limit := len(branches)
	lines := listFrontmatter(target, "branches", len(branches), limit)
	lines = append(lines, "# Branches", "")
	if len(branches) == 0 {
		lines = append(lines, "_No branches returned by GitHub._")
	}
	for _, branch := range branches[:limit] {
		href := fmt.Sprintf("https://github.com/%s/%s/tree/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), escapePathPreservingSlashes(branch.Name))
		name, truncated := githubOverviewInlinePreview(branch.Name, 120)
		if truncated {
			name += "…"
		}
		line := fmt.Sprintf("- [%s](%s)", escapeMarkdownLinkText(name), href)
		meta := []string{}
		if branch.Commit.SHA != "" {
			meta = append(meta, "`"+shortSHA(branch.Commit.SHA)+"`")
		}
		if branch.Protected {
			meta = append(meta, "protected")
		}
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	return appendListNavigation(lines, target, links)
}

func renderGitHubTags(target *GitHubTarget, tags []githubTag, links GitHubLinkRelations) string {
	limit := len(tags)
	lines := listFrontmatter(target, "tags", len(tags), limit)
	lines = append(lines, "# Tags", "")
	if len(tags) == 0 {
		lines = append(lines, "_No tags returned by GitHub._")
	}
	for _, tag := range tags[:limit] {
		href := fmt.Sprintf("https://github.com/%s/%s/tree/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), escapePathPreservingSlashes(tag.Name))
		name, truncated := githubOverviewInlinePreview(tag.Name, 120)
		if truncated {
			name += "…"
		}
		line := fmt.Sprintf("- [%s](%s)", escapeMarkdownLinkText(name), href)
		if tag.Commit.SHA != "" {
			line += " — `" + shortSHA(tag.Commit.SHA) + "`"
		}
		lines = append(lines, line)
	}
	return appendListNavigation(lines, target, links)
}

func renderGitHubReleases(target *GitHubTarget, releases []githubRelease, links GitHubLinkRelations) string {
	limit := len(releases)
	lines := listFrontmatter(target, "releases", len(releases), limit)
	lines = append(lines, "# Releases", "")
	if len(releases) == 0 {
		lines = append(lines, "_No releases returned by GitHub._")
	}
	for _, release := range releases[:limit] {
		title := release.Name
		if title == "" {
			title = release.TagName
		}
		href := release.HTMLURL
		if href == "" {
			href = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), escapePathPreservingSlashes(release.TagName))
		}
		titlePreview, truncated := githubOverviewInlinePreview(title, 140)
		if truncated {
			titlePreview += "…"
		}
		line := fmt.Sprintf("- [%s](%s)", escapeMarkdownLinkText(titlePreview), href)
		meta := []string{}
		if release.TagName != "" && release.TagName != title {
			tagPreview, tagTruncated := githubOverviewInlinePreview(release.TagName, 100)
			if tagTruncated {
				tagPreview += "…"
			}
			meta = append(meta, "tag `"+tagPreview+"`")
		}
		if release.Draft {
			meta = append(meta, "draft")
		}
		if release.Prerelease {
			meta = append(meta, "prerelease")
		}
		if release.PublishedAt != "" {
			meta = append(meta, release.PublishedAt)
		}
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	return appendListNavigation(lines, target, links)
}

func renderGitHubRelease(target *GitHubTarget, release githubRelease, availability githubReleaseAssetsAvailability) string {
	assetLimit := minInt(16, len(release.Assets))
	notesRunes := 2200
	for {
		out := renderGitHubReleaseWithLimits(target, release, availability, assetLimit, notesRunes)
		if githubOverviewFits(out) {
			return out
		}
		switch {
		case assetLimit > 4:
			assetLimit--
		case notesRunes > 800:
			notesRunes -= 200
		case assetLimit > 1:
			assetLimit--
		default:
			return out
		}
	}
}

func renderGitHubReleaseWithLimits(target *GitHubTarget, release githubRelease, availability githubReleaseAssetsAvailability, assetLimit, notesRunes int) string {
	assetLimit = minInt(assetLimit, len(release.Assets))
	canonicalURL := strings.TrimSpace(release.HTMLURL)
	if canonicalURL == "" && release.TagName != "" {
		canonicalURL = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), escapePathPreservingSlashes(release.TagName))
	}
	tagPreview, tagTruncated := githubOverviewInlinePreview(release.TagName, 140)
	if tagTruncated {
		tagPreview += "…"
	}
	namePreview, nameTruncated := githubOverviewInlinePreview(release.Name, 180)
	if nameTruncated {
		namePreview += "…"
	}
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"tag: " + yamlScalar(tagPreview),
		"name: " + yamlScalar(namePreview),
		fmt.Sprintf("draft: %t", release.Draft),
		fmt.Sprintf("prerelease: %t", release.Prerelease),
		"overview: true",
		fmt.Sprintf("assets_returned: %d", len(release.Assets)),
		fmt.Sprintf("assets_indexed: %d", assetLimit),
		fmt.Sprintf("assets_local_omitted: %d", len(release.Assets)-assetLimit),
	}
	if tagTruncated {
		lines = append(lines, "tag_preview_truncated: true")
	}
	if nameTruncated {
		lines = append(lines, "name_preview_truncated: true")
	}
	if availability.ProviderMore {
		lines = append(lines, "assets_provider_more_available: true")
	}
	if release.Target != "" {
		lines = append(lines, "target: "+yamlScalar(release.Target))
	}
	if release.Author.Login != "" {
		lines = append(lines, "author: "+yamlScalar("@"+release.Author.Login))
	}
	if release.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(release.CreatedAt))
	}
	if release.PublishedAt != "" {
		lines = append(lines, "published: "+yamlScalar(release.PublishedAt))
	}
	if canonicalURL != "" {
		lines = append(lines, "url: "+yamlScalar(canonicalURL))
	}
	lines = append(lines, "---", "")
	title := release.Name
	if title == "" {
		title = release.TagName
	}
	title, titleTruncated := githubOverviewInlinePreview(title, 180)
	if titleTruncated {
		title += "…"
	}
	lines = append(lines, "# "+title, "", "## Release notes", "")
	humanNotes := ""
	if release.Body != nil {
		humanNotes = strings.TrimSpace(stripInvisibleHTMLComments(*release.Body))
	}
	if humanNotes == "" {
		lines = append(lines, "_No release notes._")
	} else {
		preview, truncated := githubOverviewPreview(humanNotes, notesRunes)
		lines = append(lines, preview)
		if truncated {
			lines = append(lines, "", "> Release notes preview locally truncated for this overview.")
			if canonicalURL != "" {
				lines = append(lines, "> Canonical GitHub release page (complete notes in the browser): "+canonicalURL)
			}
		}
	}
	lines = append(lines, "", "## Assets", "")
	if len(release.Assets) == 0 {
		if availability.ProviderMore {
			lines = append(lines, "_No release assets were returned on the provider page fetched for this overview._")
		} else {
			lines = append(lines, "_No release assets._")
		}
	}
	for _, asset := range release.Assets[:assetLimit] {
		name := asset.Name
		if asset.Label != "" {
			name += " — " + asset.Label
		}
		line := "- **" + name + "**"
		meta := []string{}
		if asset.Size > 0 {
			meta = append(meta, fmt.Sprintf("%d bytes", asset.Size))
		}
		if asset.ContentType != "" {
			meta = append(meta, asset.ContentType)
		}
		if asset.DownloadCount > 0 {
			meta = append(meta, fmt.Sprintf("%d downloads", asset.DownloadCount))
		}
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		if asset.BrowserDownloadURL != "" {
			line += " — " + asset.BrowserDownloadURL
		}
		lines = append(lines, line)
	}
	if note := githubLocalOmissionNote("release assets returned on this provider page", len(release.Assets)-assetLimit); note != "" {
		lines = append(lines, "", note)
	}
	if availability.ProviderMore {
		lines = append(lines, "", "> GitHub has more release assets beyond the provider page fetched for this overview; this provider incompleteness is separate from local overview omission.")
	}
	if canonicalURL != "" {
		lines = append(lines, "", "Canonical release page: "+canonicalURL)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderGitHubForks(target *GitHubTarget, forks []githubFork, links GitHubLinkRelations) string {
	limit := len(forks)
	lines := listFrontmatter(target, "forks", len(forks), limit)
	lines = append(lines, "# Forks", "")
	if len(forks) == 0 {
		lines = append(lines, "_No forks returned by GitHub._")
	}
	for _, fork := range forks[:limit] {
		name := fork.FullName
		if name == "" {
			name = fork.Owner.Login
		}
		namePreview, truncated := githubOverviewInlinePreview(name, 120)
		if truncated {
			namePreview += "…"
		}
		line := "- [" + escapeMarkdownLinkText(namePreview) + "](" + fork.HTMLURL + ")"
		meta := []string{}
		if fork.StargazersCount > 0 {
			meta = append(meta, fmt.Sprintf("%d stars", fork.StargazersCount))
		}
		if fork.Language != nil && strings.TrimSpace(*fork.Language) != "" {
			meta = append(meta, *fork.Language)
		}
		if fork.Archived {
			meta = append(meta, "archived")
		}
		if fork.UpdatedAt != "" {
			meta = append(meta, "updated "+fork.UpdatedAt)
		}
		if len(meta) > 0 {
			line += " — " + strings.Join(meta, " · ")
		}
		lines = append(lines, line)
	}
	return appendListNavigation(lines, target, links)
}

func renderGitHubStargazers(target *GitHubTarget, stars []githubStar, links GitHubLinkRelations) string {
	limit := len(stars)
	lines := listFrontmatter(target, "stargazers", len(stars), limit)
	lines = append(lines, "# Stargazers", "", "_These are users who starred the repository. GitHub's `watchers_count` is also a historical star-count alias; it is not the subscriber count._", "")
	if len(stars) == 0 {
		lines = append(lines, "_No stargazers returned by GitHub._")
	}
	for _, star := range stars[:limit] {
		line := renderGitHubUserIdentity(star.User)
		if star.StarredAt != "" {
			line += " — starred " + star.StarredAt
		}
		lines = append(lines, line)
	}
	return appendListNavigation(lines, target, links)
}

func renderGitHubWatchers(target *GitHubTarget, users []githubUser, links GitHubLinkRelations) string {
	limit := len(users)
	lines := listFrontmatter(target, "watchers", len(users), limit)
	lines = append(lines, "# Watchers / subscribers", "", "_This list uses GitHub's subscribers API: these are actual repository watchers/subscribers, not stars._", "")
	if len(users) == 0 {
		lines = append(lines, "_No watchers/subscribers returned by GitHub._")
	}
	for _, user := range users[:limit] {
		lines = append(lines, renderGitHubUserIdentity(user))
	}
	return appendListNavigation(lines, target, links)
}

func renderGitHubUserIdentity(user githubUser) string {
	login := user.Login
	if login == "" {
		login = "unknown"
	}
	href := "https://github.com/" + url.PathEscape(login)
	return "- [@" + escapeMarkdownLinkText(login) + "](" + href + ")"
}

func listFrontmatter(target *GitHubTarget, view string, returned int, indexed ...int) []string {
	page := target.Query.Get("page")
	if page == "" {
		page = "1"
	}
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"view: " + yamlScalar(view),
		"page: " + page,
		fmt.Sprintf("returned: %d", returned),
	}
	if len(indexed) > 0 {
		shown := minInt(indexed[0], returned)
		lines = append(lines, fmt.Sprintf("indexed: %d", shown), fmt.Sprintf("local_omitted: %d", returned-shown))
	}
	return append(lines, "---", "")
}

func appendListNavigation(lines []string, target *GitHubTarget, links GitHubLinkRelations) string {
	if nav := renderGitHubUIPageNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "", "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func sortReleaseAssetsByName(assets []githubReleaseAsset) {
	sort.SliceStable(assets, func(i, j int) bool {
		if assets[i].Name == assets[j].Name {
			return assets[i].ID < assets[j].ID
		}
		return assets[i].Name < assets[j].Name
	})
}
