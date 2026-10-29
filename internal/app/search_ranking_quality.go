package app

import (
	"net/url"
	"strings"
)

// Search result quality is a modest, deterministic complement to provider
// agreement. It never fetches content or makes a network request. Coding
// queries prefer source documentation, API/SDK references and first-party
// repositories, without applying those preferences to unrelated searches.
func searchQualityBonus(query string, doc SearchDoc) float64 {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return 0
	}

	// Exact title-term matches matter, but don't let title keyword stuffing
	// overwhelm a page independently surfaced by multiple search providers.
	score := min(16, termScore(doc.Title, terms)*2)
	if !technicalSearchQuery(query) {
		return float64(score)
	}

	u, err := url.Parse(doc.URL)
	if err != nil || u.Hostname() == "" {
		return float64(score)
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	path := strings.ToLower(u.EscapedPath())
	if strings.HasPrefix(host, "docs.") || strings.HasPrefix(host, "developer.") || strings.HasPrefix(host, "developers.") ||
		strings.Contains(path, "/docs/") || strings.HasPrefix(path, "/docs") ||
		strings.Contains(path, "/documentation/") {
		score += 9
	}
	if strings.Contains(path, "/reference/") || strings.Contains(path, "/api/") ||
		strings.Contains(path, "/api-reference/") || strings.Contains(path, "/sdk/") ||
		strings.Contains(path, "/specification/") || strings.Contains(path, "/pkg/") {
		score += 9
	}
	if strings.Contains(path, "/quickstart") || strings.Contains(path, "/getting-started") || strings.Contains(path, "/guides/") {
		score += 4
	}
	if firstPartyDocsHost(host, query) {
		score += 9
	}
	if host == "github.com" && firstPartyGitHubRepository(path, query) {
		score += 19
	}
	return float64(score)
}

func technicalSearchQuery(query string) bool {
	terms := sourceWordPattern.FindAllString(strings.ToLower(query), -1)
	for _, term := range terms {
		switch term {
		case "api", "sdk", "docs", "documentation", "guide", "reference", "implement", "implementation", "integrate", "integration", "code", "programming", "developer", "typescript", "javascript", "python", "golang", "go", "rust", "react", "nextjs", "kubernetes", "docker", "postgres", "postgresql", "sql", "git", "github", "mcp", "cli", "library", "package", "async", "http", "server", "client", "bug", "error", "webhook", "configuration":
			return true
		}
	}
	return false
}

// These are publisher-operated docs hosts, rather than simply popular
// programming sites. Only favor them when the requested ecosystem matches.
func firstPartyDocsHost(host, query string) bool {
	aliases := map[string][]string{
		"go.dev": {"go", "golang"}, "pkg.go.dev": {"go", "golang"},
		"react.dev": {"react"}, "typescriptlang.org": {"typescript"},
		"python.org": {"python"}, "docs.python.org": {"python"},
		"rust-lang.org": {"rust"}, "doc.rust-lang.org": {"rust"},
		"nextjs.org":              {"nextjs", "next.js", "next js"},
		"kubernetes.io":           {"kubernetes", "k8s"},
		"developer.mozilla.org":   {"javascript", "typescript", "web", "html", "css", "http", "mdn"},
		"modelcontextprotocol.io": {"mcp", "model context protocol"},
		"docs.github.com":         {"git", "github", "actions"},
	}
	for domain, names := range aliases {
		if (host == domain || strings.HasSuffix(host, "."+domain)) && searchMentionsAny(query, names) {
			return true
		}
	}
	return false
}

// A random GitHub repository is not first-party. Require a known upstream
// organization AND an ecosystem-specific term in the search query.
func firstPartyGitHubRepository(path, query string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	owners := map[string][]string{
		"golang":    {"go", "golang"},
		"rust-lang": {"rust", "cargo"},
		"python":    {"python", "cpython"},
		"microsoft": {"typescript", "vscode", "dotnet", "azure"},
		"facebook":  {"react"}, "reactjs": {"react"},
		"vercel":               {"next.js", "nextjs", "next js", "vercel"},
		"nodejs":               {"nodejs", "node.js", "node js"},
		"kubernetes":           {"kubernetes", "k8s"},
		"modelcontextprotocol": {"mcp", "model context protocol"},
		"tokio-rs":             {"tokio"},
		"postgres":             {"postgres", "postgresql"},
		"openai":               {"openai", "codex"},
		"anthropics":           {"anthropic", "claude"},
		"cloudflare":           {"cloudflare", "wrangler"},
		"tavily-ai":            {"tavily"},
	}
	names, ok := owners[parts[0]]
	if !ok || !searchMentionsAny(query, names) {
		return false
	}
	repo := parts[1]
	// The organization alone is insufficient: Microsoft, Vercel and others
	// maintain many projects unrelated to the named ecosystem. Require the
	// repository itself to match a query term, including Go's two-letter name
	// and Python's canonical `cpython` repository.
	if parts[0] == "python" && repo == "cpython" && searchMentionsAny(query, []string{"python"}) {
		return true
	}
	if repo == "go" && searchMentionsAny(query, []string{"go", "golang"}) {
		return true
	}
	return termScore(strings.ReplaceAll(repo, "-", " "), queryTerms(query)) > 0
}

func searchMentionsAny(query string, names []string) bool {
	text := " " + strings.ToLower(query) + " "
	for _, name := range names {
		needle := strings.ToLower(name)
		for _, boundary := range []string{" ", "\"", "(", ")", "/", "-", ":"} {
			if strings.Contains(text, boundary+needle+" ") || strings.Contains(text, " "+needle+boundary) {
				return true
			}
		}
	}
	return false
}
