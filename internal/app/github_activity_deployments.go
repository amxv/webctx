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

type githubRepositoryActivity struct {
	ID           int64      `json:"id"`
	NodeID       string     `json:"node_id"`
	Before       string     `json:"before"`
	After        string     `json:"after"`
	Ref          string     `json:"ref"`
	Timestamp    string     `json:"timestamp"`
	ActivityType string     `json:"activity_type"`
	Actor        githubUser `json:"actor"`
}

type githubContributorStats struct {
	Total  int        `json:"total"`
	Author githubUser `json:"author"`
	Weeks  []struct {
		Week      int64 `json:"w"`
		Additions int   `json:"a"`
		Deletions int   `json:"d"`
		Commits   int   `json:"c"`
	} `json:"weeks"`
}

type githubCommitActivityWeek struct {
	Days  []int `json:"days"`
	Total int   `json:"total"`
	Week  int64 `json:"week"`
}

type githubDeployment struct {
	ID                    int64      `json:"id"`
	SHA                   string     `json:"sha"`
	Ref                   string     `json:"ref"`
	Task                  string     `json:"task"`
	Environment           string     `json:"environment"`
	Description           *string    `json:"description"`
	CreatedAt             string     `json:"created_at"`
	UpdatedAt             string     `json:"updated_at"`
	Creator               githubUser `json:"creator"`
	TransientEnvironment  bool       `json:"transient_environment"`
	ProductionEnvironment bool       `json:"production_environment"`
}

type githubDeploymentStatus struct {
	ID             int64      `json:"id"`
	State          string     `json:"state"`
	Description    string     `json:"description"`
	Environment    string     `json:"environment"`
	LogURL         string     `json:"log_url"`
	EnvironmentURL string     `json:"environment_url"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
	Creator        githubUser `json:"creator"`
}

type githubEnvironment struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	HTMLURL   string `json:"html_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type githubDeploymentLatestStatus struct {
	Status       *githubDeploymentStatus
	ProviderMore bool
}

func readGitHubActivity(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub activity fragment %q is not a supported native selector", target.Fragment)
	}
	allowed := map[string]struct{}{"ref": {}, "activity_type": {}, "actor": {}, "time_period": {}, "before": {}, "after": {}, "page": {}}
	for key := range target.Query {
		if _, ok := allowed[key]; !ok {
			return "", fmt.Errorf("GitHub activity query parameter %q is not supported by the native reader", key)
		}
	}
	q := copySelectedQuery(target.Query, []string{"ref", "activity_type", "actor", "time_period", "before", "after", "page"})
	q.Set("per_page", strconv.Itoa(githubPageableListSize))
	if page := q.Get("page"); page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n <= 0 {
			return "", fmt.Errorf("invalid GitHub activity page %q", page)
		}
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/activity?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), q.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var items []githubRepositoryActivity
	if err := json.Unmarshal(resp.Body, &items); err != nil {
		return "", fmt.Errorf("decode GitHub repository activity: %w", err)
	}
	limit := len(items)
	lines := listFrontmatter(target, "activity", len(items), limit)
	lines = append(lines, "# Repository activity", "")
	if len(items) == 0 {
		lines = append(lines, "_No activity returned by GitHub on this page._")
	}
	for _, item := range items[:limit] {
		activityType, _ := githubOverviewInlinePreview(item.ActivityType, 60)
		line := "- " + activityType
		if item.Actor.Login != "" {
			actor, truncated := githubOverviewInlinePreview(item.Actor.Login, 80)
			if truncated {
				actor += "…"
			}
			line += " by @" + actor
		}
		if item.Ref != "" {
			ref, truncated := githubOverviewInlinePreview(item.Ref, 120)
			if truncated {
				ref += "…"
			}
			line += " — `" + ref + "`"
		}
		if item.Before != "" || item.After != "" {
			line += " — `" + shortSHA(item.Before) + "` → `" + shortSHA(item.After) + "`"
		}
		if item.Timestamp != "" {
			line += " — " + item.Timestamp
		}
		lines = append(lines, line)
	}
	return appendListNavigation(lines, target, resp.Links()), nil
}

func readGitHubContributorStats(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	resp, err := readGitHubStatistics(ctx, client, target, "contributors")
	if err != nil || resp.StatusCode == http.StatusAccepted {
		return renderStatisticsPending(target, "contributors", resp), err
	}
	var stats []githubContributorStats
	if err := json.Unmarshal(resp.Body, &stats); err != nil {
		return "", fmt.Errorf("decode GitHub contributor statistics: %w", err)
	}
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].Total != stats[j].Total {
			return stats[i].Total > stats[j].Total
		}
		left := strings.ToLower(stats[i].Author.Login)
		right := strings.ToLower(stats[j].Author.Login)
		if left == right {
			return stats[i].Author.Login < stats[j].Author.Login
		}
		return left < right
	})
	return renderGitHubContributorStats(target, stats), nil
}

func renderGitHubContributorStats(target *GitHubTarget, stats []githubContributorStats) string {
	limit := minInt(70, len(stats))
	for {
		out := renderGitHubContributorStatsWithLimit(target, stats, limit)
		if githubOverviewFits(out) || limit <= 1 {
			return out
		}
		limit--
	}
}

func renderGitHubContributorStatsWithLimit(target *GitHubTarget, stats []githubContributorStats, limit int) string {
	limit = minInt(limit, len(stats))
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"view: contributors",
		fmt.Sprintf("contributors_returned: %d", len(stats)),
		fmt.Sprintf("contributors_indexed: %d", limit),
		fmt.Sprintf("contributors_local_omitted: %d", len(stats)-limit),
		"---",
		"",
		"# Contributor statistics",
		"",
		statisticsFreshnessNote(),
		"",
	}
	for _, contributor := range stats[:limit] {
		adds, dels := 0, 0
		for _, week := range contributor.Weeks {
			adds += week.Additions
			dels += week.Deletions
		}
		name := contributor.Author.Login
		if name == "" {
			name = "unknown"
		}
		lines = append(lines, fmt.Sprintf("- @%s — %d commits · +%d -%d", name, contributor.Total, adds, dels))
	}
	if note := githubLocalOmissionNote("contributors returned by GitHub", len(stats)-limit); note != "" {
		lines = append(lines, "", note)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubCommitActivityStats(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	resp, err := readGitHubStatistics(ctx, client, target, "commit_activity")
	if err != nil || resp.StatusCode == http.StatusAccepted {
		return renderStatisticsPending(target, "commit activity", resp), err
	}
	var weeks []githubCommitActivityWeek
	if err := json.Unmarshal(resp.Body, &weeks); err != nil {
		return "", fmt.Errorf("decode GitHub commit activity statistics: %w", err)
	}
	lines := statisticsFrontmatter(target, "commit_activity", len(weeks))
	lines = append(lines, "# Commit activity", "", statisticsFreshnessNote(), "")
	for _, week := range weeks {
		stamp := time.Unix(week.Week, 0).UTC().Format("2006-01-02")
		lines = append(lines, fmt.Sprintf("- %s — %d commits", stamp, week.Total))
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func readGitHubCodeFrequencyStats(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	resp, err := readGitHubStatistics(ctx, client, target, "code_frequency")
	if err != nil || resp.StatusCode == http.StatusAccepted {
		return renderStatisticsPending(target, "code frequency", resp), err
	}
	var weeks [][]int64
	if err := json.Unmarshal(resp.Body, &weeks); err != nil {
		return "", fmt.Errorf("decode GitHub code-frequency statistics: %w", err)
	}
	return renderGitHubCodeFrequencyStats(target, weeks), nil
}

func renderGitHubCodeFrequencyStats(target *GitHubTarget, rawWeeks [][]int64) string {
	weeks := make([][]int64, 0, len(rawWeeks))
	var additionsTotal, deletionsTotal int64
	for _, week := range rawWeeks {
		if len(week) < 3 {
			continue
		}
		weeks = append(weeks, week)
		additionsTotal += week[1]
		deletionsTotal += week[2]
	}
	sort.SliceStable(weeks, func(i, j int) bool { return weeks[i][0] < weeks[j][0] })
	limit := minInt(52, len(weeks))
	start := len(weeks) - limit
	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"view: code_frequency",
		fmt.Sprintf("weeks_returned: %d", len(weeks)),
		fmt.Sprintf("weeks_indexed: %d", limit),
		fmt.Sprintf("weeks_local_omitted: %d", start),
		fmt.Sprintf("additions_total: %d", additionsTotal),
		fmt.Sprintf("deletions_total: %d", deletionsTotal),
		"---",
		"",
		"# Code frequency",
		"",
		statisticsFreshnessNote(),
		"",
	}
	if len(weeks) == 0 {
		lines = append(lines, "_No code-frequency weeks returned by GitHub._")
	}
	if start > 0 {
		lines = append(lines, fmt.Sprintf("> Showing the most recent %d weekly buckets; %d older weekly buckets returned by GitHub are locally omitted from this overview. Aggregate totals above include every returned bucket.", limit, start), "")
	}
	for _, week := range weeks[start:] {
		stamp := time.Unix(week[0], 0).UTC().Format("2006-01-02")
		lines = append(lines, fmt.Sprintf("- %s — +%d %d", stamp, week[1], week[2]))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func readGitHubStatistics(ctx context.Context, client *GitHubClient, target *GitHubTarget, kind string) (GitHubResponse, error) {
	if target.Fragment != "" || len(target.Query) > 0 {
		return GitHubResponse{}, fmt.Errorf("GitHub statistics views do not support query/fragment selectors in the native reader")
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/stats/%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), kind)
	return client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
}

func renderStatisticsPending(target *GitHubTarget, view string, resp GitHubResponse) string {
	if resp.StatusCode != http.StatusAccepted {
		return ""
	}
	return fmt.Sprintf("---\nrepository: %s\nview: %s\nprovider_status: computing\n---\n\n# %s\n\nGitHub returned HTTP 202 while computing cached repository statistics. Retry the same URL later; webctx has no local cache and cannot make the provider compute them synchronously.", yamlScalar(target.Owner+"/"+target.Repo), yamlScalar(view), strings.Title(view))
}

func statisticsFrontmatter(target *GitHubTarget, view string, returned int) []string {
	return []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "view: " + yamlScalar(view), fmt.Sprintf("returned: %d", returned), "---", ""}
}

func statisticsFreshnessNote() string {
	return "_GitHub computes/caches repository statistics upstream. This command made a fresh provider request, but the statistics themselves may reflect GitHub's cached computation._"
}

func readGitHubDeployments(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub deployments fragment %q is not a supported native selector", target.Fragment)
	}
	allowed := map[string]struct{}{"sha": {}, "ref": {}, "task": {}, "environment": {}, "page": {}}
	for key := range target.Query {
		if _, ok := allowed[key]; !ok {
			return "", fmt.Errorf("GitHub deployments query parameter %q is not supported by the native reader", key)
		}
	}
	q := copySelectedQuery(target.Query, []string{"sha", "ref", "task", "environment", "page"})
	q.Set("per_page", strconv.Itoa(githubPageableListSize))
	resp, deployments, err := fetchGitHubDeploymentPage(ctx, client, target, q)
	if err != nil {
		return "", err
	}
	limit := len(deployments)
	lines := listFrontmatter(target, "deployments", len(deployments), limit)
	lines = append(lines, "# Deployments", "")
	lines = append(lines, renderDeploymentRows(target, deployments[:limit])...)
	return appendListNavigation(lines, target, resp.Links()), nil
}

func readGitHubDeploymentEnvironment(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub deployment environment fragment %q is not a supported native selector", target.Fragment)
	}
	for key := range target.Query {
		if key != "page" {
			return "", fmt.Errorf("GitHub deployment environment query parameter %q is not supported", key)
		}
	}
	envEndpoint := fmt.Sprintf("/repos/%s/%s/environments/%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), url.PathEscape(target.Name))
	envResp, err := client.REST(ctx, http.MethodGet, envEndpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var environment githubEnvironment
	if err := json.Unmarshal(envResp.Body, &environment); err != nil {
		return "", fmt.Errorf("decode GitHub environment: %w", err)
	}
	q := url.Values{"environment": []string{target.Name}, "per_page": []string{strconv.Itoa(githubPageableListSize)}}
	if page := target.Query.Get("page"); page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n <= 0 {
			return "", fmt.Errorf("invalid GitHub deployment environment page %q", page)
		}
		q.Set("page", page)
	}
	depResp, deployments, err := fetchGitHubDeploymentPage(ctx, client, target, q)
	if err != nil {
		return "", err
	}
	latestStatuses := make([]githubDeploymentLatestStatus, 0, len(deployments))
	for _, deployment := range deployments {
		latest, providerMore, err := fetchGitHubLatestDeploymentStatus(ctx, client, target, deployment.ID)
		if err != nil {
			return "", err
		}
		latestStatuses = append(latestStatuses, githubDeploymentLatestStatus{Status: latest, ProviderMore: providerMore})
	}
	return renderGitHubDeploymentEnvironment(target, environment, deployments, latestStatuses, depResp.Links()), nil
}

func renderGitHubDeploymentEnvironment(target *GitHubTarget, environment githubEnvironment, deployments []githubDeployment, latestStatuses []githubDeploymentLatestStatus, links GitHubLinkRelations) string {
	return renderGitHubDeploymentEnvironmentWithLimit(target, environment, deployments, latestStatuses, links, len(deployments))
}

func renderGitHubDeploymentEnvironmentWithLimit(target *GitHubTarget, environment githubEnvironment, deployments []githubDeployment, latestStatuses []githubDeploymentLatestStatus, links GitHubLinkRelations, limit int) string {
	limit = minInt(limit, len(deployments))
	olderCount := 0
	for _, latest := range latestStatuses {
		if latest.ProviderMore {
			olderCount++
		}
	}
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "environment: " + yamlScalar(target.Name), fmt.Sprintf("deployments_returned: %d", len(deployments)), fmt.Sprintf("deployments_indexed: %d", limit), fmt.Sprintf("deployments_local_omitted: %d", len(deployments)-limit), fmt.Sprintf("deployments_with_older_statuses: %d", olderCount)}
	if environment.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(environment.CreatedAt))
	}
	if environment.UpdatedAt != "" {
		lines = append(lines, "updated: "+yamlScalar(environment.UpdatedAt))
	}
	lines = append(lines, "---", "", "# Deployment environment: "+target.Name, "", "_GitHub controls deployment-status retention. webctx reads only the latest returned status for each deployment in this bounded overview; older status history may remain available upstream._", "")
	for i, deployment := range deployments[:limit] {
		lines = append(lines, renderDeploymentHeading(deployment), "")
		if i >= len(latestStatuses) || latestStatuses[i].Status == nil {
			lines = append(lines, "_No deployment statuses returned by GitHub._", "")
			continue
		}
		latest := latestStatuses[i]
		status := *latest.Status
		line := fmt.Sprintf("- latest status %d — %s", status.ID, status.State)
		if status.Description != "" {
			preview, truncated := githubOverviewPreview(status.Description, 160)
			line += " — " + preview
			if truncated {
				line += "…"
			}
		}
		if status.Creator.Login != "" {
			line += " — @" + status.Creator.Login
		}
		if status.CreatedAt != "" {
			line += " — " + status.CreatedAt
		}
		if status.LogURL != "" {
			line += " — logs " + status.LogURL
		}
		lines = append(lines, line)
		if status.EnvironmentURL != "" {
			lines = append(lines, "  Environment: "+status.EnvironmentURL)
		}
		if latest.ProviderMore {
			lines = append(lines, "  Older statuses: available upstream; not expanded in this overview.")
		}
		lines = append(lines, "")
	}
	if note := githubLocalOmissionNote("deployments returned on this environment page", len(deployments)-limit); note != "" {
		lines = append(lines, note, "")
	}
	if nav := renderGitHubUIPageNavigation(target, links); len(nav) > 0 {
		lines = append(lines, "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func fetchGitHubDeploymentPage(ctx context.Context, client *GitHubClient, target *GitHubTarget, q url.Values) (GitHubResponse, []githubDeployment, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/deployments?%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), q.Encode())
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return GitHubResponse{}, nil, err
	}
	var deployments []githubDeployment
	if err := json.Unmarshal(resp.Body, &deployments); err != nil {
		return GitHubResponse{}, nil, fmt.Errorf("decode GitHub deployments: %w", err)
	}
	return resp, deployments, nil
}

func fetchGitHubLatestDeploymentStatus(ctx context.Context, client *GitHubClient, target *GitHubTarget, deploymentID int64) (*githubDeploymentStatus, bool, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/deployments/%d/statuses?per_page=1", url.PathEscape(target.Owner), url.PathEscape(target.Repo), deploymentID)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, false, err
	}
	var statuses []githubDeploymentStatus
	if err := json.Unmarshal(resp.Body, &statuses); err != nil {
		return nil, false, fmt.Errorf("decode GitHub deployment statuses: %w", err)
	}
	if len(statuses) == 0 {
		return nil, resp.Links()["next"] != "", nil
	}
	return &statuses[0], resp.Links()["next"] != "", nil
}

func renderDeploymentRows(target *GitHubTarget, deployments []githubDeployment) []string {
	if len(deployments) == 0 {
		return []string{"_No deployments returned by GitHub._"}
	}
	lines := []string{}
	for _, deployment := range deployments {
		line := "- " + renderDeploymentHeading(deployment)
		if deployment.Environment != "" {
			href := fmt.Sprintf("https://github.com/%s/%s/deployments/%s", escapePathPreservingSlashes(target.Owner), escapePathPreservingSlashes(target.Repo), escapePathPreservingSlashes(deployment.Environment))
			environment, truncated := githubOverviewInlinePreview(deployment.Environment, 100)
			if truncated {
				environment += "…"
			}
			line += " — [" + escapeMarkdownLinkText(environment) + "](" + href + ")"
		}
		lines = append(lines, line)
	}
	return lines
}

func renderDeploymentHeading(deployment githubDeployment) string {
	line := fmt.Sprintf("Deployment %d", deployment.ID)
	if deployment.Ref != "" {
		ref, truncated := githubOverviewInlinePreview(deployment.Ref, 120)
		if truncated {
			ref += "…"
		}
		line += " — `" + ref + "`"
	}
	if deployment.SHA != "" {
		line += " @ `" + shortSHA(deployment.SHA) + "`"
	}
	if deployment.Creator.Login != "" {
		line += " — @" + deployment.Creator.Login
	}
	if deployment.CreatedAt != "" {
		line += " — " + deployment.CreatedAt
	}
	return line
}
