package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	q.Set("per_page", "30")
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
	lines := listFrontmatter(target, "activity", len(items))
	lines = append(lines, "# Repository activity", "")
	if len(items) == 0 {
		lines = append(lines, "_No activity returned by GitHub on this page._")
	}
	for _, item := range items {
		line := "- " + item.ActivityType
		if item.Actor.Login != "" {
			line += " by @" + item.Actor.Login
		}
		if item.Ref != "" {
			line += " — `" + item.Ref + "`"
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
	lines := statisticsFrontmatter(target, "contributors", len(stats))
	lines = append(lines, "# Contributor statistics", "", statisticsFreshnessNote(), "")
	for _, contributor := range stats {
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
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
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
	lines := statisticsFrontmatter(target, "code_frequency", len(weeks))
	lines = append(lines, "# Code frequency", "", statisticsFreshnessNote(), "")
	for _, week := range weeks {
		if len(week) < 3 {
			continue
		}
		stamp := time.Unix(week[0], 0).UTC().Format("2006-01-02")
		lines = append(lines, fmt.Sprintf("- %s — +%d %d", stamp, week[1], week[2]))
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
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
	q.Set("per_page", "30")
	resp, deployments, err := fetchGitHubDeploymentPage(ctx, client, target, q)
	if err != nil {
		return "", err
	}
	lines := listFrontmatter(target, "deployments", len(deployments))
	lines = append(lines, "# Deployments", "")
	lines = append(lines, renderDeploymentRows(target, deployments)...)
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
	q := url.Values{"environment": []string{target.Name}, "per_page": []string{"10"}}
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
	lines := []string{"---", "repository: " + yamlScalar(target.Owner+"/"+target.Repo), "environment: " + yamlScalar(target.Name), fmt.Sprintf("deployments_returned: %d", len(deployments))}
	if environment.CreatedAt != "" {
		lines = append(lines, "created: "+yamlScalar(environment.CreatedAt))
	}
	if environment.UpdatedAt != "" {
		lines = append(lines, "updated: "+yamlScalar(environment.UpdatedAt))
	}
	lines = append(lines, "---", "", "# Deployment environment: "+target.Name, "", "_GitHub controls deployment-status retention. webctx follows every status page GitHub returns for each deployment on this bounded environment page, but does not imply older statuses remain available indefinitely._", "")
	for _, deployment := range deployments {
		lines = append(lines, renderDeploymentHeading(deployment), "")
		statuses, err := fetchGitHubDeploymentStatuses(ctx, client, target, deployment.ID)
		if err != nil {
			return "", err
		}
		if len(statuses) == 0 {
			lines = append(lines, "_No deployment statuses returned by GitHub._", "")
		}
		for _, status := range statuses {
			line := "- " + status.State
			if status.Description != "" {
				line += " — " + status.Description
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
		}
		lines = append(lines, "")
	}
	if nav := renderGitHubUIPageNavigation(target, depResp.Links()); len(nav) > 0 {
		lines = append(lines, "## Navigation", "")
		lines = append(lines, nav...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
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

func fetchGitHubDeploymentStatuses(ctx context.Context, client *GitHubClient, target *GitHubTarget, deploymentID int64) ([]githubDeploymentStatus, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/deployments/%d/statuses?per_page=100", url.PathEscape(target.Owner), url.PathEscape(target.Repo), deploymentID)
	pages, err := client.RESTPages(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	statuses := []githubDeploymentStatus{}
	for _, page := range pages {
		var batch []githubDeploymentStatus
		if err := json.Unmarshal(page.Body, &batch); err != nil {
			return nil, fmt.Errorf("decode GitHub deployment statuses: %w", err)
		}
		statuses = append(statuses, batch...)
	}
	return statuses, nil
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
			line += " — [" + deployment.Environment + "](" + href + ")"
		}
		lines = append(lines, line)
	}
	return lines
}

func renderDeploymentHeading(deployment githubDeployment) string {
	line := fmt.Sprintf("Deployment %d", deployment.ID)
	if deployment.Ref != "" {
		line += " — `" + deployment.Ref + "`"
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
