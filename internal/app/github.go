package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	githubRESTAPIVersion   = "2026-03-10"
	githubDefaultAPIBase   = "https://api.github.com"
	githubDefaultRawBase   = "https://raw.githubusercontent.com"
	githubRootPreviewRunes = 5000
	githubBlobMaxBytes     = int64(100 * 1024 * 1024)
)

type GitHubTargetKind string

const (
	GitHubTargetRepository    GitHubTargetKind = "repository"
	GitHubTargetBlob          GitHubTargetKind = "blob"
	GitHubTargetTree          GitHubTargetKind = "tree"
	GitHubTargetIssue         GitHubTargetKind = "issue"
	GitHubTargetIssueList     GitHubTargetKind = "issue_list"
	GitHubTargetLabel         GitHubTargetKind = "label"
	GitHubTargetLabelList     GitHubTargetKind = "label_list"
	GitHubTargetMilestone     GitHubTargetKind = "milestone"
	GitHubTargetMilestones    GitHubTargetKind = "milestones"
	GitHubTargetPull          GitHubTargetKind = "pull"
	GitHubTargetPullFiles     GitHubTargetKind = "pull_files"
	GitHubTargetPullCommits   GitHubTargetKind = "pull_commits"
	GitHubTargetPullChecks    GitHubTargetKind = "pull_checks"
	GitHubTargetPullDiff      GitHubTargetKind = "pull_diff"
	GitHubTargetPullPatch     GitHubTargetKind = "pull_patch"
	GitHubTargetCommit        GitHubTargetKind = "commit"
	GitHubTargetCommitDiff    GitHubTargetKind = "commit_diff"
	GitHubTargetCommitPatch   GitHubTargetKind = "commit_patch"
	GitHubTargetCompare       GitHubTargetKind = "compare"
	GitHubTargetCompareDiff   GitHubTargetKind = "compare_diff"
	GitHubTargetComparePatch  GitHubTargetKind = "compare_patch"
	GitHubTargetHistory       GitHubTargetKind = "history"
	GitHubTargetBlame         GitHubTargetKind = "blame"
	GitHubTargetActions       GitHubTargetKind = "actions"
	GitHubTargetWorkflows     GitHubTargetKind = "actions_workflows"
	GitHubTargetWorkflow      GitHubTargetKind = "actions_workflow"
	GitHubTargetActionsRun    GitHubTargetKind = "actions_run"
	GitHubTargetActionsJob    GitHubTargetKind = "actions_job"
	GitHubTargetBranches      GitHubTargetKind = "branches"
	GitHubTargetTags          GitHubTargetKind = "tags"
	GitHubTargetReleases      GitHubTargetKind = "releases"
	GitHubTargetRelease       GitHubTargetKind = "release"
	GitHubTargetReleaseLatest GitHubTargetKind = "release_latest"
	GitHubTargetForks         GitHubTargetKind = "forks"
	GitHubTargetStargazers    GitHubTargetKind = "stargazers"
	GitHubTargetWatchers      GitHubTargetKind = "watchers"
	GitHubTargetDiscussions   GitHubTargetKind = "discussions"
	GitHubTargetDiscussion    GitHubTargetKind = "discussion"
	GitHubTargetGist          GitHubTargetKind = "gist"
)

// GitHubTarget is the semantic identity parsed from a GitHub URL. Blob/tree
// tails deliberately stay unresolved until a provider-backed reader needs to
// distinguish the ref from the repository path.
type GitHubTarget struct {
	Owner       string
	Repo        string
	Kind        GitHubTargetKind
	Tail        []string
	Number      int
	Name        string
	RunID       int64
	JobID       int64
	Fragment    string
	Query       url.Values
	OriginalURL string
}

type GitHubNativeOutcome int

const (
	GitHubNativeUnsupported GitHubNativeOutcome = iota
	GitHubNativeSuccess
	GitHubNativeFailure
)

type GitHubNativeResult struct {
	Outcome  GitHubNativeOutcome
	Markdown string
	Err      error
}

type githubHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// GitHubClient is the single GitHub provider boundary. It preserves status,
// headers, and response bodies, pins the REST version, owns optional token
// selection, and exposes Link pagination primitives without persistent cache.
type GitHubClient struct {
	httpClient githubHTTPDoer
	apiBase    string
	rawBase    string
	token      string
	userAgent  string
}

type GitHubResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	FinalURL   string
	TooLarge   bool
}

type GitHubErrorKind string

const (
	GitHubErrorNotFound       GitHubErrorKind = "not_found"
	GitHubErrorAuthentication GitHubErrorKind = "authentication"
	GitHubErrorForbidden      GitHubErrorKind = "forbidden"
	GitHubErrorRateLimit      GitHubErrorKind = "rate_limit"
	GitHubErrorProviderLimit  GitHubErrorKind = "provider_limit"
	GitHubErrorGone           GitHubErrorKind = "gone"
	GitHubErrorHTTP           GitHubErrorKind = "http"
)

type GitHubError struct {
	Kind          GitHubErrorKind
	StatusCode    int
	HasToken      bool
	RetryAfter    string
	RateReset     string
	RateResource  string
	ProviderLimit string
}

func (e *GitHubError) Error() string {
	if e == nil {
		return "GitHub request failed"
	}
	status := ""
	if e.StatusCode != 0 {
		status = fmt.Sprintf(" (%d)", e.StatusCode)
	}
	switch e.Kind {
	case GitHubErrorNotFound:
		if e.HasToken {
			return "GitHub resource was not found or is inaccessible with the configured token" + status + "."
		}
		return "GitHub resource was not found or may be private" + status + ". Hint: set GH_TOKEN or GITHUB_TOKEN to read resources your account can access."
	case GitHubErrorAuthentication:
		return "GitHub authentication failed" + status + ". Check GH_TOKEN or GITHUB_TOKEN."
	case GitHubErrorForbidden:
		if e.HasToken {
			return "GitHub denied access to this resource" + status + ". Check the configured token permissions."
		}
		return "GitHub denied access to this resource" + status + ". Hint: set GH_TOKEN or GITHUB_TOKEN if the resource requires authentication."
	case GitHubErrorRateLimit:
		parts := []string{"GitHub rate limit exceeded" + status + "."}
		if e.RateResource != "" {
			parts = append(parts, "Resource: "+e.RateResource+".")
		}
		if e.RetryAfter != "" {
			parts = append(parts, "Retry after: "+e.RetryAfter+".")
		}
		if e.RateReset != "" {
			parts = append(parts, "Reset: "+e.RateReset+".")
		}
		if !e.HasToken {
			parts = append(parts, "Hint: set GH_TOKEN or GITHUB_TOKEN for authenticated GitHub capacity.")
		}
		return strings.Join(parts, " ")
	case GitHubErrorProviderLimit:
		if e.ProviderLimit != "" {
			return "GitHub provider limit: " + e.ProviderLimit + "."
		}
		return "GitHub provider limit prevented a complete read" + status + "."
	case GitHubErrorGone:
		return "GitHub resource is no longer available" + status + "."
	default:
		return "GitHub request failed" + status + "."
	}
}

type GitHubLinkRelations map[string]string

func parseGitHubTarget(raw string) *GitHubTarget {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	if strings.EqualFold(parsed.Hostname(), "gist.github.com") {
		parts := splitGitHubPath(parsed.Path)
		if len(parts) < 2 || len(parts) > 3 {
			return nil
		}
		gistID := parts[1]
		if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(gistID) == "" || gistID == "raw" {
			return nil
		}
		target := &GitHubTarget{
			Host:         "gist.github.com",
			Owner:        parts[0],
			Kind:         GitHubTargetGist,
			Name:         gistID,
			OriginalURL:  raw,
			CanonicalURL: parsed.String(),
			Query:        cloneURLValues(parsed.Query()),
			Fragment:     parsed.Fragment,
		}
		if len(parts) == 3 {
			if parts[2] == "raw" {
				return nil
			}
			target.Tail = []string{parts[2]}
		}
		return target
	}
	if !strings.EqualFold(parsed.Hostname(), "github.com") {
		return nil
	}
	trimmed := strings.Trim(parsed.Path, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	target := &GitHubTarget{
		Owner:       parts[0],
		Repo:        parts[1],
		Fragment:    parsed.Fragment,
		Query:       parsed.Query(),
		OriginalURL: raw,
	}
	if len(parts) == 2 {
		target.Kind = GitHubTargetRepository
		return target
	}
	switch parts[2] {
	case "blob":
		target.Kind = GitHubTargetBlob
		target.Tail = append([]string(nil), parts[3:]...)
		return target
	case "tree":
		target.Kind = GitHubTargetTree
		target.Tail = append([]string(nil), parts[3:]...)
		return target
	case "issues":
		if len(parts) == 3 {
			target.Kind = GitHubTargetIssueList
			return target
		}
		if len(parts) == 4 {
			number, err := strconv.Atoi(parts[3])
			if err == nil && number > 0 {
				target.Kind = GitHubTargetIssue
				target.Number = number
				return target
			}
		}
		return nil
	case "labels":
		if len(parts) == 3 {
			target.Kind = GitHubTargetLabelList
			return target
		}
		if len(parts) == 4 {
			name, err := url.PathUnescape(parts[3])
			if err == nil && strings.TrimSpace(name) != "" {
				target.Kind = GitHubTargetLabel
				target.Name = name
				return target
			}
		}
		return nil
	case "milestones":
		if len(parts) == 3 {
			target.Kind = GitHubTargetMilestones
			return target
		}
		return nil
	case "milestone":
		if len(parts) == 4 {
			number, err := strconv.Atoi(parts[3])
			if err == nil && number > 0 {
				target.Kind = GitHubTargetMilestone
				target.Number = number
				return target
			}
		}
		return nil
	case "pull":
		if len(parts) == 4 {
			rawNumber := parts[3]
			kind := GitHubTargetPull
			switch {
			case strings.HasSuffix(rawNumber, ".diff"):
				kind = GitHubTargetPullDiff
				rawNumber = strings.TrimSuffix(rawNumber, ".diff")
			case strings.HasSuffix(rawNumber, ".patch"):
				kind = GitHubTargetPullPatch
				rawNumber = strings.TrimSuffix(rawNumber, ".patch")
			}
			number, err := strconv.Atoi(rawNumber)
			if err == nil && number > 0 {
				target.Kind = kind
				target.Number = number
				return target
			}
		}
		if len(parts) == 5 {
			number, err := strconv.Atoi(parts[3])
			if err != nil || number <= 0 {
				return nil
			}
			switch parts[4] {
			case "files":
				target.Kind = GitHubTargetPullFiles
			case "commits":
				target.Kind = GitHubTargetPullCommits
			case "checks":
				target.Kind = GitHubTargetPullChecks
			default:
				return nil
			}
			target.Number = number
			return target
		}
		return nil
	case "commit":
		if len(parts) != 4 {
			return nil
		}
		ref := parts[3]
		kind := GitHubTargetCommit
		switch {
		case strings.HasSuffix(ref, ".diff"):
			kind = GitHubTargetCommitDiff
			ref = strings.TrimSuffix(ref, ".diff")
		case strings.HasSuffix(ref, ".patch"):
			kind = GitHubTargetCommitPatch
			ref = strings.TrimSuffix(ref, ".patch")
		}
		if strings.TrimSpace(ref) == "" {
			return nil
		}
		target.Kind = kind
		target.Name = ref
		return target
	case "compare":
		if len(parts) < 4 {
			return nil
		}
		comparison := strings.Join(parts[3:], "/")
		kind := GitHubTargetCompare
		switch {
		case strings.HasSuffix(comparison, ".diff"):
			kind = GitHubTargetCompareDiff
			comparison = strings.TrimSuffix(comparison, ".diff")
		case strings.HasSuffix(comparison, ".patch"):
			kind = GitHubTargetComparePatch
			comparison = strings.TrimSuffix(comparison, ".patch")
		}
		base, head, ok := strings.Cut(comparison, "...")
		if !ok || strings.TrimSpace(base) == "" || strings.TrimSpace(head) == "" {
			return nil
		}
		target.Kind = kind
		target.Tail = []string{base, head}
		return target
	case "commits":
		if len(parts) < 4 {
			return nil
		}
		target.Kind = GitHubTargetHistory
		target.Tail = append([]string(nil), parts[3:]...)
		return target
	case "blame":
		if len(parts) < 5 {
			return nil
		}
		target.Kind = GitHubTargetBlame
		target.Tail = append([]string(nil), parts[3:]...)
		return target
	case "actions":
		if len(parts) == 3 {
			target.Kind = GitHubTargetActions
			return target
		}
		switch parts[3] {
		case "workflows":
			if len(parts) == 4 {
				target.Kind = GitHubTargetWorkflows
				return target
			}
			if len(parts) == 5 {
				workflow, err := url.PathUnescape(parts[4])
				if err == nil && strings.TrimSpace(workflow) != "" {
					target.Kind = GitHubTargetWorkflow
					target.Name = workflow
					return target
				}
			}
		case "runs":
			if len(parts) < 5 {
				return nil
			}
			runID, err := strconv.ParseInt(parts[4], 10, 64)
			if err != nil || runID <= 0 {
				return nil
			}
			if len(parts) == 5 {
				target.Kind = GitHubTargetActionsRun
				target.RunID = runID
				return target
			}
			if len(parts) == 7 && parts[5] == "job" {
				jobID, err := strconv.ParseInt(parts[6], 10, 64)
				if err == nil && jobID > 0 {
					target.Kind = GitHubTargetActionsJob
					target.RunID = runID
					target.JobID = jobID
					return target
				}
			}
		}
		return nil
	case "branches":
		if len(parts) == 3 {
			target.Kind = GitHubTargetBranches
			return target
		}
		return nil
	case "tags":
		if len(parts) == 3 {
			target.Kind = GitHubTargetTags
			return target
		}
		return nil
	case "releases":
		if len(parts) == 3 {
			target.Kind = GitHubTargetReleases
			return target
		}
		if len(parts) == 4 && parts[3] == "latest" {
			target.Kind = GitHubTargetReleaseLatest
			return target
		}
		if len(parts) >= 5 && parts[3] == "tag" {
			tag, err := url.PathUnescape(strings.Join(parts[4:], "/"))
			if err == nil && strings.TrimSpace(tag) != "" {
				target.Kind = GitHubTargetRelease
				target.Name = tag
				return target
			}
		}
		return nil
	case "forks":
		if len(parts) == 3 {
			target.Kind = GitHubTargetForks
			return target
		}
		return nil
	case "stargazers":
		if len(parts) == 3 {
			target.Kind = GitHubTargetStargazers
			return target
		}
		return nil
	case "watchers":
		if len(parts) == 3 {
			target.Kind = GitHubTargetWatchers
			return target
		}
		return nil
	case "discussions":
		if len(parts) == 3 {
			target.Kind = GitHubTargetDiscussions
			return target
		}
		if len(parts) == 4 {
			number, err := strconv.Atoi(parts[3])
			if err == nil && number > 0 {
				target.Kind = GitHubTargetDiscussion
				target.Number = number
				return target
			}
		}
		return nil
	default:
		// Security pages and every route family not yet implemented remain on
		// the existing generic markdown/Firecrawl fallback path.
		return nil
	}
}

func newGitHubClient() *GitHubClient {
	return &GitHubClient{
		httpClient: http.DefaultClient,
		apiBase:    githubDefaultAPIBase,
		rawBase:    githubDefaultRawBase,
		token:      githubTokenFromEnv(),
		userAgent:  "webctx/" + version,
	}
}

func githubTokenFromEnv() string {
	if value := strings.TrimSpace(os.Getenv("GH_TOKEN")); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
}

func (c *GitHubClient) hasToken() bool {
	return c != nil && strings.TrimSpace(c.token) != ""
}

func (c *GitHubClient) REST(ctx context.Context, method, endpoint, accept string) (GitHubResponse, error) {
	if c == nil {
		return GitHubResponse{}, fmt.Errorf("GitHub client is nil")
	}
	rawURL := endpoint
	if strings.HasPrefix(endpoint, "/") {
		rawURL = strings.TrimRight(c.apiBase, "/") + endpoint
	}
	if !sameOrigin(rawURL, c.apiBase) {
		return GitHubResponse{}, fmt.Errorf("refusing GitHub API request to an untrusted origin")
	}
	if accept == "" {
		accept = "application/vnd.github+json"
	}
	return c.request(ctx, method, rawURL, accept, true)
}

// RESTPages follows GitHub-provided Link rel=next URLs until the selected
// resource is complete. It deliberately does not synthesize page numbers.
func (c *GitHubClient) RESTPages(ctx context.Context, endpoint, accept string) ([]GitHubResponse, error) {
	pages := []GitHubResponse{}
	next := endpoint
	seen := map[string]struct{}{}
	for strings.TrimSpace(next) != "" {
		if _, ok := seen[next]; ok {
			return nil, fmt.Errorf("GitHub pagination returned a cycle at %s", next)
		}
		seen[next] = struct{}{}
		resp, err := c.REST(ctx, http.MethodGet, next, accept)
		if err != nil {
			return nil, err
		}
		pages = append(pages, resp)
		next = resp.Links()["next"]
	}
	return pages, nil
}

// GraphQL performs an authenticated GitHub GraphQL query using the same
// injected HTTP boundary as REST. GraphQL is capability-specific enrichment;
// callers decide whether a failure is fatal for the selected native resource.
func (c *GitHubClient) GraphQL(ctx context.Context, query string, variables map[string]any, result any) error {
	if c == nil {
		return fmt.Errorf("GitHub client is nil")
	}
	if !c.hasToken() {
		return &GitHubError{Kind: GitHubErrorAuthentication, StatusCode: http.StatusUnauthorized, HasToken: false}
	}
	endpoint := strings.TrimRight(c.apiBase, "/") + "/graphql"
	if !sameOrigin(endpoint, c.apiBase) {
		return fmt.Errorf("refusing GitHub GraphQL request to an untrusted origin")
	}
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.userAgent) != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, githubBlobMaxBytes+1))
	if err != nil {
		return err
	}
	providerResp := GitHubResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: body}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return classifyGitHubError(providerResp, true)
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode GitHub GraphQL response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		// Do not echo provider error bodies because auth/private responses can
		// contain details that should not become normal CLI output.
		return fmt.Errorf("GitHub GraphQL query failed")
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, result); err != nil {
		return fmt.Errorf("decode GitHub GraphQL data: %w", err)
	}
	return nil
}

func (c *GitHubClient) Raw(ctx context.Context, rawURL string) (GitHubResponse, error) {
	if c == nil {
		return GitHubResponse{}, fmt.Errorf("GitHub client is nil")
	}
	if !sameOrigin(rawURL, c.rawBase) {
		return GitHubResponse{}, fmt.Errorf("refusing GitHub raw request to an untrusted origin")
	}
	return c.request(ctx, http.MethodGet, rawURL, "", false)
}

func (c *GitHubClient) request(ctx context.Context, method, rawURL, accept string, rest bool) (GitHubResponse, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return GitHubResponse{}, err
	}
	if strings.TrimSpace(c.userAgent) != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if rest {
		req.Header.Set("Accept", accept)
		req.Header.Set("X-GitHub-Api-Version", githubRESTAPIVersion)
		if c.hasToken() {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return GitHubResponse{}, err
	}
	defer resp.Body.Close()

	out := GitHubResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone()}
	if resp.Request != nil && resp.Request.URL != nil {
		out.FinalURL = resp.Request.URL.String()
	} else {
		out.FinalURL = rawURL
	}
	if resp.ContentLength > githubBlobMaxBytes {
		out.TooLarge = true
	} else {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, githubBlobMaxBytes+1))
		if readErr != nil {
			return out, readErr
		}
		if int64(len(body)) > githubBlobMaxBytes {
			out.TooLarge = true
		} else {
			out.Body = body
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, classifyGitHubError(out, c.hasToken())
	}
	return out, nil
}

func sameOrigin(rawURL, baseURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, base.Scheme) && strings.EqualFold(u.Host, base.Host)
}

func classifyGitHubError(resp GitHubResponse, hasToken bool) error {
	message := strings.ToLower(githubErrorMessage(resp.Body))
	kind := GitHubErrorHTTP
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		kind = GitHubErrorAuthentication
	case http.StatusNotFound:
		kind = GitHubErrorNotFound
	case http.StatusGone:
		kind = GitHubErrorGone
	case http.StatusForbidden:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != "" || strings.Contains(message, "rate limit") || strings.Contains(message, "abuse detection") {
			kind = GitHubErrorRateLimit
		} else if githubMessageIsProviderLimit(message) {
			kind = GitHubErrorProviderLimit
		} else {
			kind = GitHubErrorForbidden
		}
	case http.StatusTooManyRequests:
		kind = GitHubErrorRateLimit
	default:
		if githubMessageIsProviderLimit(message) {
			kind = GitHubErrorProviderLimit
		}
	}

	ghErr := &GitHubError{
		Kind:         kind,
		StatusCode:   resp.StatusCode,
		HasToken:     hasToken,
		RetryAfter:   strings.TrimSpace(resp.Header.Get("Retry-After")),
		RateResource: strings.TrimSpace(resp.Header.Get("X-RateLimit-Resource")),
	}
	if reset := strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")); reset != "" {
		if epoch, err := strconv.ParseInt(reset, 10, 64); err == nil {
			ghErr.RateReset = time.Unix(epoch, 0).UTC().Format(time.RFC3339)
		} else {
			ghErr.RateReset = reset
		}
	}
	if kind == GitHubErrorProviderLimit {
		ghErr.ProviderLimit = "GitHub does not expose this source through the selected API because it exceeds the provider's supported content size"
	}
	return ghErr
}

func githubErrorMessage(body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		return payload.Message
	}
	return ""
}

func githubMessageIsProviderLimit(message string) bool {
	return strings.Contains(message, "too large") || strings.Contains(message, "100 mb") || strings.Contains(message, "100mb") || strings.Contains(message, "blob size")
}

func ParseGitHubLinkHeader(header string) GitHubLinkRelations {
	links := GitHubLinkRelations{}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "<") {
			continue
		}
		end := strings.Index(part, ">")
		if end <= 1 {
			continue
		}
		target := part[1:end]
		params := strings.Split(part[end+1:], ";")
		for _, param := range params {
			param = strings.TrimSpace(param)
			if !strings.HasPrefix(param, "rel=") {
				continue
			}
			rel := strings.Trim(strings.TrimPrefix(param, "rel="), `"`)
			if rel != "" {
				links[rel] = target
			}
		}
	}
	return links
}

func (r GitHubResponse) Links() GitHubLinkRelations {
	return ParseGitHubLinkHeader(r.Header.Get("Link"))
}

func readGitHubNative(rawURL string) GitHubNativeResult {
	target := parseGitHubTarget(rawURL)
	if target == nil {
		return GitHubNativeResult{Outcome: GitHubNativeUnsupported}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return readGitHubNativeWithClient(ctx, target, newGitHubClient())
}

func readGitHubNativeWithClient(ctx context.Context, target *GitHubTarget, client *GitHubClient) GitHubNativeResult {
	if target == nil {
		return GitHubNativeResult{Outcome: GitHubNativeUnsupported}
	}
	var (
		markdown string
		err      error
	)
	switch target.Kind {
	case GitHubTargetRepository:
		markdown, err = readGitHubRepository(ctx, client, target)
	case GitHubTargetBlob:
		markdown, err = readGitHubBlob(ctx, client, target)
	case GitHubTargetTree:
		markdown, err = readGitHubTree(ctx, client, target)
	case GitHubTargetIssue:
		markdown, err = readGitHubIssue(ctx, client, target)
	case GitHubTargetIssueList:
		markdown, err = readGitHubIssueList(ctx, client, target)
	case GitHubTargetLabel:
		markdown, err = readGitHubLabel(ctx, client, target)
	case GitHubTargetLabelList:
		markdown, err = readGitHubLabelList(ctx, client, target)
	case GitHubTargetMilestone:
		markdown, err = readGitHubMilestone(ctx, client, target)
	case GitHubTargetMilestones:
		markdown, err = readGitHubMilestones(ctx, client, target)
	case GitHubTargetPull:
		markdown, err = readGitHubPullRequest(ctx, client, target)
	case GitHubTargetPullFiles:
		markdown, err = readGitHubPullFiles(ctx, client, target)
	case GitHubTargetPullCommits:
		markdown, err = readGitHubPullCommits(ctx, client, target)
	case GitHubTargetPullChecks:
		markdown, err = readGitHubPullChecks(ctx, client, target)
	case GitHubTargetPullDiff:
		markdown, err = readGitHubPullRawDiff(ctx, client, target, false)
	case GitHubTargetPullPatch:
		markdown, err = readGitHubPullRawDiff(ctx, client, target, true)
	case GitHubTargetCommit:
		markdown, err = readGitHubCommit(ctx, client, target)
	case GitHubTargetCommitDiff:
		markdown, err = readGitHubCommitRawDiff(ctx, client, target, false)
	case GitHubTargetCommitPatch:
		markdown, err = readGitHubCommitRawDiff(ctx, client, target, true)
	case GitHubTargetCompare:
		markdown, err = readGitHubCompare(ctx, client, target)
	case GitHubTargetCompareDiff:
		markdown, err = readGitHubCompareRawDiff(ctx, client, target, false)
	case GitHubTargetComparePatch:
		markdown, err = readGitHubCompareRawDiff(ctx, client, target, true)
	case GitHubTargetHistory:
		markdown, err = readGitHubHistory(ctx, client, target)
	case GitHubTargetBlame:
		markdown, err = readGitHubBlame(ctx, client, target)
	case GitHubTargetActions:
		markdown, err = readGitHubActionsOverview(ctx, client, target)
	case GitHubTargetWorkflows:
		markdown, err = readGitHubWorkflows(ctx, client, target)
	case GitHubTargetWorkflow:
		markdown, err = readGitHubWorkflow(ctx, client, target)
	case GitHubTargetActionsRun:
		markdown, err = readGitHubActionsRun(ctx, client, target)
	case GitHubTargetActionsJob:
		markdown, err = readGitHubActionsJob(ctx, client, target)
	case GitHubTargetBranches:
		markdown, err = readGitHubBranches(ctx, client, target)
	case GitHubTargetTags:
		markdown, err = readGitHubTags(ctx, client, target)
	case GitHubTargetReleases:
		markdown, err = readGitHubReleases(ctx, client, target)
	case GitHubTargetRelease:
		markdown, err = readGitHubRelease(ctx, client, target, false)
	case GitHubTargetReleaseLatest:
		markdown, err = readGitHubRelease(ctx, client, target, true)
	case GitHubTargetForks:
		markdown, err = readGitHubForks(ctx, client, target)
	case GitHubTargetStargazers:
		markdown, err = readGitHubStargazers(ctx, client, target)
	case GitHubTargetWatchers:
		markdown, err = readGitHubWatchers(ctx, client, target)
	case GitHubTargetDiscussions:
		markdown, err = readGitHubDiscussions(ctx, client, target)
	case GitHubTargetDiscussion:
		markdown, err = readGitHubDiscussion(ctx, client, target)
	case GitHubTargetGist:
		markdown, err = readGitHubGist(ctx, client, target)
	default:
		return GitHubNativeResult{Outcome: GitHubNativeUnsupported}
	}
	if err != nil {
		return GitHubNativeResult{Outcome: GitHubNativeFailure, Err: err}
	}
	return GitHubNativeResult{Outcome: GitHubNativeSuccess, Markdown: markdown}
}

type githubRepository struct {
	FullName        string   `json:"full_name"`
	HTMLURL         string   `json:"html_url"`
	Description     string   `json:"description"`
	Language        string   `json:"language"`
	DefaultBranch   string   `json:"default_branch"`
	StargazersCount int      `json:"stargazers_count"`
	Topics          []string `json:"topics"`
	Fork            bool     `json:"fork"`
	Archived        bool     `json:"archived"`
	Private         bool     `json:"private"`
	Visibility      string   `json:"visibility"`
	License         *struct {
		SPDXID string `json:"spdx_id"`
		Name   string `json:"name"`
	} `json:"license"`
}

type githubContent struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int64  `json:"size"`
	Encoding    string `json:"encoding"`
	Content     string `json:"content"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
}

func readGitHubRepository(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub repository-root fragment %q is not a supported native selector", target.Fragment)
	}
	baseEndpoint := fmt.Sprintf("/repos/%s/%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo))
	resp, err := client.REST(ctx, http.MethodGet, baseEndpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var repo githubRepository
	if err := json.Unmarshal(resp.Body, &repo); err != nil {
		return "", fmt.Errorf("decode GitHub repository metadata: %w", err)
	}
	if repo.FullName == "" {
		repo.FullName = target.Owner + "/" + target.Repo
	}
	if repo.HTMLURL == "" {
		repo.HTMLURL = "https://github.com/" + escapePathPreservingSlashes(repo.FullName)
	}

	readmeResp, readmeErr := client.REST(ctx, http.MethodGet, baseEndpoint+"/readme", "application/vnd.github+json")
	var readme *githubContent
	var readmeText string
	if readmeErr != nil {
		var ghErr *GitHubError
		if !asGitHubError(readmeErr, &ghErr) || ghErr.Kind != GitHubErrorNotFound {
			return "", readmeErr
		}
	} else {
		var content githubContent
		if err := json.Unmarshal(readmeResp.Body, &content); err != nil {
			return "", fmt.Errorf("decode GitHub README metadata: %w", err)
		}
		readme = &content
		readmeText, err = githubContentText(ctx, client, target.Owner, target.Repo, repo.DefaultBranch, &content)
		if err != nil {
			return "", err
		}
	}

	return renderGitHubRepository(repo, readme, readmeText), nil
}

func renderGitHubRepository(repo githubRepository, readme *githubContent, readmeText string) string {
	lines := []string{"---", "repository: " + yamlScalar(repo.FullName)}
	if strings.TrimSpace(repo.Description) != "" {
		lines = append(lines, "description: "+yamlScalar(repo.Description))
	}
	if strings.TrimSpace(repo.Language) != "" {
		lines = append(lines, "language: "+yamlScalar(repo.Language))
	}
	if strings.TrimSpace(repo.DefaultBranch) != "" {
		lines = append(lines, "default_branch: "+yamlScalar(repo.DefaultBranch))
	}
	if repo.License != nil {
		license := strings.TrimSpace(repo.License.SPDXID)
		if license == "" || strings.EqualFold(license, "NOASSERTION") {
			license = strings.TrimSpace(repo.License.Name)
		}
		if license != "" {
			lines = append(lines, "license: "+yamlScalar(license))
		}
	}
	lines = append(lines, fmt.Sprintf("stars: %d", repo.StargazersCount))
	if len(repo.Topics) > 0 {
		sortedTopics := append([]string(nil), repo.Topics...)
		sort.Strings(sortedTopics)
		encoded, _ := json.Marshal(sortedTopics)
		lines = append(lines, "topics: "+string(encoded))
	}
	if repo.Fork {
		lines = append(lines, "fork: true")
	}
	if repo.Archived {
		lines = append(lines, "archived: true")
	}
	visibility := strings.TrimSpace(repo.Visibility)
	if visibility == "" && repo.Private {
		visibility = "private"
	}
	if visibility != "" && !strings.EqualFold(visibility, "public") {
		lines = append(lines, "visibility: "+yamlScalar(visibility))
	}
	lines = append(lines, "---", "", "## README preview", "")

	if readme == nil {
		lines = append(lines, "No repository README was found.")
	} else {
		humanReadme := stripInvisibleHTMLComments(readmeText)
		preview, truncated := truncateMarkdownSafe(humanReadme, githubRootPreviewRunes)
		if strings.TrimSpace(preview) == "" {
			lines = append(lines, "The repository README is empty.")
		} else {
			lines = append(lines, preview)
		}
		if truncated {
			lines = append(lines, "", "> README preview truncated near 5,000 characters.")
			if readme.HTMLURL != "" {
				lines = append(lines, "> Full README: "+readme.HTMLURL)
			}
		}
	}

	hints := githubRootHints(repo, readme)
	if len(hints) > 0 {
		lines = append(lines, "", "## Useful GitHub URLs", "")
		for _, hint := range hints {
			lines = append(lines, "- "+hint)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func githubRootHints(repo githubRepository, readme *githubContent) []string {
	base := strings.TrimRight(repo.HTMLURL, "/")
	if base == "" {
		base = "https://github.com/" + escapePathPreservingSlashes(repo.FullName)
	}
	ref := strings.TrimSpace(repo.DefaultBranch)
	if ref == "" {
		ref = "HEAD"
	}
	refPath := escapePathPreservingSlashes(ref)
	hints := []string{}
	if readme != nil && readme.HTMLURL != "" {
		hints = append(hints, "Full README source: "+readme.HTMLURL)
	}
	hints = append(hints,
		"Select source lines: "+base+"/blob/"+refPath+"/path/to/file#L20-L40",
		"List one directory level: "+base+"/tree/"+refPath+"/path/to/directory",
		"Browse Issues: "+base+"/issues",
		"Read a Pull Request: "+base+"/pull/123",
	)
	return hints
}

func readGitHubBlob(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if len(target.Tail) < 2 {
		return "", fmt.Errorf("GitHub blob URL must include a ref and file path")
	}

	// Preserve the existing cheapest faithful path for public blobs. The raw
	// host resolves the complete unresolved tail, including slash refs, without
	// spending a REST request or requiring gh/authentication.
	rawURL := githubRawURL(client.rawBase, target.Owner, target.Repo, target.Tail)
	rawResp, rawErr := client.Raw(ctx, rawURL)
	if rawErr == nil {
		if rawResp.TooLarge {
			return "", githubBlobProviderLimitError()
		}
		return renderGitHubBlobSelection(target, rawResp.Body, rawResp.Header.Get("Content-Type"), rawURL)
	}

	var rawGHError *GitHubError
	if !asGitHubError(rawErr, &rawGHError) || rawGHError.Kind != GitHubErrorNotFound || !client.hasToken() {
		return "", rawErr
	}

	// A raw 404 can be a private source. With an optional token configured,
	// resolve ref/path through Contents and fetch the authenticated source.
	resolved, err := resolveGitHubRefPath(ctx, client, target, "file")
	if err != nil {
		return "", err
	}
	var item githubContent
	if err := json.Unmarshal(resolved.Response.Body, &item); err != nil {
		return "", fmt.Errorf("decode GitHub source metadata: %w", err)
	}
	content, contentType, err := githubContentBytes(ctx, client, target.Owner, target.Repo, resolved.Ref, &item)
	if err != nil {
		return "", err
	}
	downloadURL := item.DownloadURL
	if downloadURL == "" {
		downloadURL = target.OriginalURL
	}
	return renderGitHubBlobSelection(target, content, contentType, downloadURL)
}

func renderGitHubBlobSelection(target *GitHubTarget, content []byte, contentType, downloadURL string) (string, error) {
	if int64(len(content)) > githubBlobMaxBytes {
		return "", githubBlobProviderLimitError()
	}
	name := path.Base(strings.Join(target.Tail, "/"))
	if name == "." || name == "/" || name == "" {
		name = target.Repo
	}
	if !isTextContent(content, contentType) {
		parts := []string{"# " + name, "", "**URL:** " + target.OriginalURL, "", "Binary/non-text source is not emitted as terminal bytes."}
		if downloadURL != "" {
			parts = append(parts, "", "Raw/download: "+downloadURL)
		}
		return strings.Join(parts, "\n"), nil
	}
	text := string(content)
	if target.Fragment == "" {
		title := firstHeadingOrFallback(text, name)
		return formatReadLink(title, target.OriginalURL, text), nil
	}

	if start, end, ok, err := parseGitHubLineSelector(target.Fragment); ok {
		if err != nil {
			return "", err
		}
		selected, total, err := selectSourceLines(text, start, end)
		if err != nil {
			return "", err
		}
		coord := fmt.Sprintf("**Lines:** %d", start)
		if end != start {
			coord = fmt.Sprintf("**Lines:** %d-%d", start, end)
		}
		coord += fmt.Sprintf(" of %d", total)
		return strings.Join([]string{"# " + name, "", "**URL:** " + target.OriginalURL, coord, "", selected}, "\n"), nil
	}

	if !isMarkdownPath(name) {
		return "", fmt.Errorf("GitHub source fragment %q is not a supported selector for %s", target.Fragment, name)
	}
	section, err := selectMarkdownHeadingSection(text, target.Fragment)
	if err != nil {
		return "", err
	}
	return formatReadLink(firstHeadingOrFallback(section, name), target.OriginalURL, section), nil
}

func githubBlobProviderLimitError() error {
	return &GitHubError{Kind: GitHubErrorProviderLimit, ProviderLimit: "source exceeds GitHub's 100 MB Contents/Git blob support; use the GitHub blob/raw URL as a download instead"}
}

type resolvedGitHubPath struct {
	Ref      string
	Path     string
	Endpoint string
	Response GitHubResponse
}

type githubAmbiguousRefError struct {
	candidates []resolvedGitHubPath
}

func (e *githubAmbiguousRefError) Error() string {
	parts := make([]string, 0, len(e.candidates))
	for _, candidate := range e.candidates {
		if candidate.Path == "" {
			parts = append(parts, candidate.Ref+" (repository root)")
		} else {
			parts = append(parts, candidate.Ref+" + "+candidate.Path)
		}
	}
	return "GitHub ref/path is ambiguous; multiple provider-valid splits exist: " + strings.Join(parts, ", ")
}

func resolveGitHubRefPath(ctx context.Context, client *GitHubClient, target *GitHubTarget, wantKind string) (resolvedGitHubPath, error) {
	if wantKind == "history" {
		return resolveGitHubHistoryRefPath(ctx, client, target)
	}
	if target == nil || len(target.Tail) == 0 {
		return resolvedGitHubPath{}, fmt.Errorf("GitHub source URL is missing a ref")
	}
	maxSplit := len(target.Tail)
	if wantKind == "file" {
		maxSplit--
	}
	if maxSplit < 1 {
		return resolvedGitHubPath{}, fmt.Errorf("GitHub source URL is missing a repository path")
	}

	matches := []resolvedGitHubPath{}
	var lastNotFound error
	for split := 1; split <= maxSplit; split++ {
		ref := strings.Join(target.Tail[:split], "/")
		filePath := strings.Join(target.Tail[split:], "/")
		endpoint := githubContentsEndpoint(target.Owner, target.Repo, filePath, ref)
		resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github+json")
		if err != nil {
			var ghErr *GitHubError
			if asGitHubError(err, &ghErr) && ghErr.Kind == GitHubErrorNotFound {
				lastNotFound = err
				continue
			}
			return resolvedGitHubPath{}, err
		}
		kind, err := githubContentsKind(resp.Body)
		if err != nil {
			return resolvedGitHubPath{}, err
		}
		if wantKind != "any" && kind != wantKind {
			continue
		}
		matches = append(matches, resolvedGitHubPath{Ref: ref, Path: filePath, Endpoint: endpoint, Response: resp})
	}

	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return resolvedGitHubPath{}, &githubAmbiguousRefError{candidates: matches}
	}
	if lastNotFound != nil {
		return resolvedGitHubPath{}, lastNotFound
	}
	return resolvedGitHubPath{}, &GitHubError{Kind: GitHubErrorNotFound, StatusCode: http.StatusNotFound, HasToken: client.hasToken()}
}

func githubContentsKind(body []byte) (string, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("GitHub Contents API returned an empty response")
	}
	if trimmed[0] == '[' {
		var entries []githubContent
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return "", fmt.Errorf("decode GitHub directory contents: %w", err)
		}
		return "dir", nil
	}
	var item githubContent
	if err := json.Unmarshal(trimmed, &item); err != nil {
		return "", fmt.Errorf("decode GitHub source content: %w", err)
	}
	if item.Type == "dir" {
		return "dir", nil
	}
	return "file", nil
}

func githubContentsEndpoint(owner, repo, filePath, ref string) string {
	endpoint := fmt.Sprintf("/repos/%s/%s/contents", url.PathEscape(owner), url.PathEscape(repo))
	if strings.TrimSpace(filePath) != "" {
		endpoint += "/" + escapePathPreservingSlashes(filePath)
	}
	if ref != "" {
		endpoint += "?ref=" + url.QueryEscape(ref)
	}
	return endpoint
}

func githubContentBytes(ctx context.Context, client *GitHubClient, owner, repo, ref string, item *githubContent) ([]byte, string, error) {
	if item == nil {
		return nil, "", fmt.Errorf("GitHub source metadata is missing")
	}
	if item.Size > githubBlobMaxBytes {
		return nil, "", githubBlobProviderLimitError()
	}
	if strings.EqualFold(item.Encoding, "base64") && strings.TrimSpace(item.Content) != "" {
		compact := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, item.Content)
		decoded, err := base64.StdEncoding.DecodeString(compact)
		if err != nil {
			return nil, "", fmt.Errorf("decode GitHub base64 source: %w", err)
		}
		return decoded, http.DetectContentType(decoded), nil
	}
	endpoint := githubContentsEndpoint(owner, repo, item.Path, ref)
	resp, err := client.REST(ctx, http.MethodGet, endpoint, "application/vnd.github.raw+json")
	if err != nil {
		return nil, "", err
	}
	if resp.TooLarge {
		return nil, "", githubBlobProviderLimitError()
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

func githubContentText(ctx context.Context, client *GitHubClient, owner, repo, ref string, item *githubContent) (string, error) {
	content, contentType, err := githubContentBytes(ctx, client, owner, repo, ref, item)
	if err != nil {
		return "", err
	}
	if !isTextContent(content, contentType) {
		return "", fmt.Errorf("GitHub README is not text content")
	}
	return string(content), nil
}

func readGitHubTree(ctx context.Context, client *GitHubClient, target *GitHubTarget) (string, error) {
	if len(target.Tail) < 1 {
		return "", fmt.Errorf("GitHub tree URL must include a ref")
	}
	if target.Fragment != "" {
		return "", fmt.Errorf("GitHub tree fragment %q is not a supported native selector", target.Fragment)
	}
	resolved, err := resolveGitHubRefPath(ctx, client, target, "dir")
	if err != nil {
		return "", err
	}
	var entries []githubContent
	if err := json.Unmarshal(resolved.Response.Body, &entries); err != nil {
		return "", fmt.Errorf("decode GitHub directory listing: %w", err)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	lines := []string{
		"---",
		"repository: " + yamlScalar(target.Owner+"/"+target.Repo),
		"ref: " + yamlScalar(resolved.Ref),
		"path: " + yamlScalar(treeDisplayPath(resolved.Path)),
		fmt.Sprintf("entries: %d", len(entries)),
	}
	providerCeiling := len(entries) >= 1000
	if providerCeiling {
		lines = append(lines, "complete: false")
	}
	lines = append(lines, "---", "", "# "+treeDisplayPath(resolved.Path), "")
	if providerCeiling {
		lines = append(lines, "> GitHub Contents API returns at most 1,000 entries for a directory; this one-level listing may be incomplete.", "")
	}
	if len(entries) == 0 {
		lines = append(lines, "_Directory is empty._")
	}
	for _, entry := range entries {
		label := entry.Name
		if entry.Type == "dir" {
			label += "/"
		}
		kind := entry.Type
		if kind == "" {
			kind = "file"
		}
		if entry.HTMLURL != "" {
			lines = append(lines, fmt.Sprintf("- %s [%s](%s)", kind, escapeMarkdownLinkText(label), entry.HTMLURL))
		} else {
			lines = append(lines, fmt.Sprintf("- %s %s", kind, label))
		}
	}

	if readme := chooseDirectoryREADME(entries); readme != nil {
		readmeText, err := githubContentText(ctx, client, target.Owner, target.Repo, resolved.Ref, readme)
		if err != nil {
			return "", err
		}
		preview, truncated := truncateMarkdownSafe(stripInvisibleHTMLComments(readmeText), githubRootPreviewRunes)
		lines = append(lines, "", "## Directory README", "", preview)
		if truncated {
			lines = append(lines, "", "> Directory README preview truncated near 5,000 characters.")
			if readme.HTMLURL != "" {
				lines = append(lines, "> Full README: "+readme.HTMLURL)
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func chooseDirectoryREADME(entries []githubContent) *githubContent {
	best := -1
	bestScore := 100
	for i := range entries {
		if entries[i].Type != "file" {
			continue
		}
		name := strings.ToLower(entries[i].Name)
		score := 100
		switch name {
		case "readme.md":
			score = 0
		case "readme":
			score = 1
		default:
			if strings.HasPrefix(name, "readme.") {
				score = 2
			}
		}
		if score < bestScore {
			best = i
			bestScore = score
		}
	}
	if best < 0 {
		return nil
	}
	return &entries[best]
}

func treeDisplayPath(filePath string) string {
	if strings.TrimSpace(filePath) == "" {
		return "/"
	}
	return filePath
}

func parseGitHubLineSelector(fragment string) (start, end int, ok bool, err error) {
	matches := githubLineSelectorRE.FindStringSubmatch(fragment)
	if matches == nil {
		return 0, 0, false, nil
	}
	start, _ = strconv.Atoi(matches[1])
	end = start
	if matches[2] != "" {
		end, _ = strconv.Atoi(matches[2])
	}
	if start <= 0 || end <= 0 {
		return 0, 0, true, fmt.Errorf("GitHub line selector must use positive line numbers")
	}
	if end < start {
		return 0, 0, true, fmt.Errorf("GitHub line selector range is reversed: L%d-L%d", start, end)
	}
	return start, end, true, nil
}

func selectSourceLines(text string, start, end int) (string, int, error) {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	total := len(lines)
	if start > total || end > total {
		return "", total, fmt.Errorf("GitHub line selector L%d-L%d is outside the file's %d lines", start, end, total)
	}
	return strings.Join(lines[start-1:end], "\n"), total, nil
}

type markdownHeading struct {
	Level int
	Slug  string
	Line  int
}

func selectMarkdownHeadingSection(text, fragment string) (string, error) {
	headings := markdownHeadings(text)
	matchIndex := -1
	for i, heading := range headings {
		if heading.Slug == fragment {
			matchIndex = i
			break
		}
	}
	if matchIndex < 0 {
		return "", fmt.Errorf("GitHub Markdown heading selector #%s was not found", fragment)
	}
	lines := strings.Split(text, "\n")
	start := headings[matchIndex].Line
	end := len(lines)
	for _, heading := range headings[matchIndex+1:] {
		if heading.Level <= headings[matchIndex].Level {
			end = heading.Line
			break
		}
	}
	return strings.TrimRight(strings.Join(lines[start:end], "\n"), "\n"), nil
}

func markdownHeadings(text string) []markdownHeading {
	lines := strings.Split(text, "\n")
	counts := map[string]int{}
	out := []markdownHeading{}
	inFence := false
	fenceMarker := ""
	for lineNo, line := range lines {
		trimmed := strings.TrimSpace(line)
		if marker, ok := markdownFenceMarker(trimmed); ok {
			if !inFence {
				inFence = true
				fenceMarker = marker
			} else if strings.HasPrefix(trimmed, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			continue
		}
		if inFence {
			continue
		}
		level, headingText, ok := parseATXHeading(line)
		if !ok {
			continue
		}
		base := githubHeadingSlug(headingText)
		if base == "" {
			continue
		}
		count := counts[base]
		counts[base] = count + 1
		slug := base
		if count > 0 {
			slug = fmt.Sprintf("%s-%d", base, count)
		}
		out = append(out, markdownHeading{Level: level, Slug: slug, Line: lineNo})
	}
	return out
}

func parseATXHeading(line string) (int, string, bool) {
	trimmedLeft := strings.TrimLeft(line, " ")
	if len(line)-len(trimmedLeft) > 3 {
		return 0, "", false
	}
	level := 0
	for level < len(trimmedLeft) && level < 6 && trimmedLeft[level] == '#' {
		level++
	}
	if level == 0 || level >= len(trimmedLeft) || (trimmedLeft[level] != ' ' && trimmedLeft[level] != '\t') {
		return 0, "", false
	}
	text := strings.TrimSpace(trimmedLeft[level:])
	text = strings.TrimSpace(strings.TrimRight(text, "#"))
	if text == "" {
		return 0, "", false
	}
	return level, text, true
}

func githubHeadingSlug(text string) string {
	text = markdownImageRE.ReplaceAllString(text, "$1")
	text = markdownLinkRE.ReplaceAllString(text, "$1")
	text = htmlTagRE.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "`", "")
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune('-')
		}
	}
	return b.String()
}

func stripInvisibleHTMLComments(markdown string) string {
	lines := strings.SplitAfter(markdown, "\n")
	var out strings.Builder
	inFence := false
	fenceMarker := ""
	inComment := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if marker, ok := markdownFenceMarker(trimmed); ok && !inComment {
			if !inFence {
				inFence = true
				fenceMarker = marker
			} else if strings.HasPrefix(trimmed, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			out.WriteString(line)
			continue
		}
		if inFence {
			out.WriteString(line)
			continue
		}
		out.WriteString(stripHTMLCommentsFromLine(line, &inComment))
	}
	return strings.TrimSpace(out.String())
}

func stripHTMLCommentsFromLine(line string, inComment *bool) string {
	var out strings.Builder
	rest := line
	for rest != "" {
		if *inComment {
			end := strings.Index(rest, "-->")
			if end < 0 {
				return out.String()
			}
			*inComment = false
			rest = rest[end+3:]
			continue
		}
		start := strings.Index(rest, "<!--")
		if start < 0 {
			out.WriteString(rest)
			break
		}
		out.WriteString(rest[:start])
		rest = rest[start+4:]
		*inComment = true
	}
	return out.String()
}

func truncateMarkdownSafe(markdown string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		return "", strings.TrimSpace(markdown) != ""
	}
	if utf8.RuneCountInString(markdown) <= maxRunes {
		return strings.TrimSpace(markdown), false
	}
	lines := strings.SplitAfter(markdown, "\n")
	runeCount := 0
	byteCount := 0
	lastSafeLine := 0
	lastParagraph := 0
	inFence := false
	fenceMarker := ""
	for _, line := range lines {
		lineRunes := utf8.RuneCountInString(line)
		if runeCount+lineRunes > maxRunes {
			break
		}
		trimmed := strings.TrimSpace(line)
		if marker, ok := markdownFenceMarker(trimmed); ok {
			if !inFence {
				inFence = true
				fenceMarker = marker
			} else if strings.HasPrefix(trimmed, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
		}
		runeCount += lineRunes
		byteCount += len(line)
		if !inFence {
			lastSafeLine = byteCount
			if trimmed == "" {
				lastParagraph = byteCount
			}
		}
	}
	end := lastSafeLine
	if lastParagraph > 0 && utf8.RuneCountInString(markdown[:lastParagraph]) >= maxRunes*2/3 {
		end = lastParagraph
	}
	if end <= 0 {
		// No safe line boundary exists before the budget. This only happens for
		// a single enormous line; a rune boundary is still preferable to an
		// invalid UTF-8 byte cut.
		runes := []rune(markdown)
		if len(runes) > maxRunes {
			runes = runes[:maxRunes]
		}
		return strings.TrimSpace(string(runes)), true
	}
	return strings.TrimSpace(markdown[:end]), true
}

func markdownFenceMarker(trimmed string) (string, bool) {
	if strings.HasPrefix(trimmed, "```") {
		return "```", true
	}
	if strings.HasPrefix(trimmed, "~~~") {
		return "~~~", true
	}
	return "", false
}

func isMarkdownPath(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") || strings.HasSuffix(lower, ".mdown") || strings.HasSuffix(lower, ".mkdn")
}

func isTextContent(content []byte, contentType string) bool {
	if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return false
	}
	contentType = strings.ToLower(contentType)
	if isKnownBinaryContentType(contentType) {
		return false
	}
	if strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "json") || strings.Contains(contentType, "xml") || strings.Contains(contentType, "javascript") || strings.Contains(contentType, "yaml") || strings.Contains(contentType, "toml") {
		return true
	}
	detected := strings.ToLower(http.DetectContentType(content))
	if isKnownBinaryContentType(detected) {
		return false
	}
	if strings.HasPrefix(detected, "text/") || strings.Contains(detected, "json") || strings.Contains(detected, "xml") {
		return true
	}
	// GitHub may serve extensionless source as application/octet-stream. Valid
	// UTF-8 without NULs is safe to treat as textual source for terminal output.
	return true
}

func isKnownBinaryContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if strings.HasPrefix(contentType, "image/") || strings.HasPrefix(contentType, "audio/") || strings.HasPrefix(contentType, "video/") || strings.HasPrefix(contentType, "font/") {
		return true
	}
	switch contentType {
	case "application/pdf", "application/zip", "application/gzip", "application/x-gzip", "application/x-rar-compressed", "application/vnd.rar", "application/x-7z-compressed", "application/wasm":
		return true
	default:
		return false
	}
}

func githubRawURL(rawBase, owner, repo string, tail []string) string {
	parts := []string{url.PathEscape(owner), url.PathEscape(repo)}
	for _, segment := range tail {
		parts = append(parts, url.PathEscape(segment))
	}
	return strings.TrimRight(rawBase, "/") + "/" + strings.Join(parts, "/")
}

func escapePathPreservingSlashes(value string) string {
	parts := strings.Split(value, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func yamlScalar(value string) string {
	return strconv.Quote(value)
}

func escapeMarkdownLinkText(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "[", "\\[")
	value = strings.ReplaceAll(value, "]", "\\]")
	return value
}

func asGitHubError(err error, target **GitHubError) bool {
	if err == nil {
		return false
	}
	ghErr, ok := err.(*GitHubError)
	if !ok {
		return false
	}
	*target = ghErr
	return true
}

var (
	githubLineSelectorRE = regexp.MustCompile(`^L([0-9]+)(?:-L([0-9]+))?$`)
	htmlTagRE            = regexp.MustCompile(`<[^>]+>`)
	markdownImageRE      = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	markdownLinkRE       = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
)
