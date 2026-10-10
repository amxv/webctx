package app

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Research deliberately works with source text, not generated answers. The
// coding agent gets exact excerpts and URLs it can independently inspect.
// One follow-up level and fixed budgets prevent uncontrolled crawls.
const (
	// Coding agents value complete code and request contracts more than tiny
	// snippets. Keep ordinary documentation pages whole when reasonably sized.
	focusedPrimaryBudget = 22000
	focusedRelatedBudget = 6500
	searchSourceBudget   = 4000
	maxRelatedSources    = 5
	maxFollowupSources   = 2
	maxSearchSources     = 3
)

func relatedExcerptBudget(rawURL string) int {
	lower := strings.ToLower(rawURL)
	if strings.Contains(lower, "/get-started/") || strings.Contains(lower, "/quickstart") {
		return 18000 // Preserve end-to-end runnable setup guides.
	}
	return focusedRelatedBudget
}

var (
	mdLinkPattern     = regexp.MustCompile(`\[([^\]\n]{2,120})\]\(([^\s)]+)(?:\s+"[^"]*")?\)`)
	wordPattern       = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9_-]{2,}`)
	sourceWordPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9]{1,}`)
	headingPattern    = regexp.MustCompile(`^#{1,6}\s+(.+)$`)
)

type researchSection struct {
	index   int
	heading string
	body    string
	score   int
}

type sourceCandidate struct {
	url   string
	label string
	score int
	order int
}

type sourceRead struct {
	url     string
	content string
	err     error
}

// ReadLinkFocused is backwards-compatible: URL-only calls still return the
// complete original source. A question makes the engine select excerpts and
// automatically inspect related documentation on the same host.
func ReadLinkFocused(rawURL, question string) (string, error) {
	return readLinkFocused(rawURL, question, ReadLink)
}

func readLinkFocused(rawURL, question string, reader func(string) (string, error)) (string, error) {
	if strings.TrimSpace(question) == "" {
		return reader(rawURL)
	}
	if err := validateSourceURL(rawURL); err != nil {
		return "", err
	}
	question = strings.TrimSpace(question)
	// REST contract discovery is independent of the page scrape. Start it
	// immediately so a slow Firecrawl read and a large OpenAPI download overlap
	// instead of approaching the hosted MCP function timeout sequentially.
	var openAPIDone chan openAPIContext
	if asksForAPIContracts(question) {
		openAPIDone = make(chan openAPIContext, 1)
		go func() { openAPIDone <- discoverOpenAPIContract(rawURL, question) }()
	}
	primary, err := reader(rawURL)
	if err != nil {
		return "", err
	}
	primary = copyableCurlExamples(primary)
	if answer, ok := directlyAnsweredMetadata(rawURL, question, primary); ok {
		return answer, nil
	}

	// Combine on-page links and authoritative Markdown index links instead of
	// filling the entire budget from whichever links happen to appear first.
	// In particular, an implementation question should prefer get-started,
	// SQLite, webhook, and API references over incidental alarm/concept pages.
	links := researchCandidates(rawURL, primary, question, maxRelatedSources)
	structuredDone := make(chan string, 1)
	if strings.Contains(primary, "**Markdown source:**") || !asksForAPIContracts(question) {
		structuredDone <- ""
	} else {
		go func() { structuredDone <- embeddedStructuredContext(rawURL, question) }()
	}
	more := readSources(links, reader)
	parts := []string{
		"# Source-grounded context",
		"",
		"**Question:** " + question,
		"",
		"## Primary source",
		"**Source:** " + rawURL,
		"",
	}
	primaryBudget := focusedPrimaryBudget
	if asksForAPIContracts(question) {
		primaryBudget = 5300
	}
	primaryExcerpt := focusExcerpt(primary, question, primaryBudget)
	parts = append(parts, primaryExcerpt)
	successful := 1
	relatedSuccessful := 0
	allRetrieved := primary
	outputEvidence := primaryExcerpt
	seenURLs := map[string]bool{rawURL: true}
	for _, source := range more {
		if source.err != nil {
			continue
		}
		successful++
		relatedSuccessful++
		seenURLs[source.url] = true
		allRetrieved += "\n" + source.content
		relatedBudget := relatedExcerptBudget(source.url)
		if asksForAPIContracts(question) && relatedBudget > 3000 {
			relatedBudget = 3000
		}
		excerpt := focusExcerpt(copyableCurlExamples(source.content), question, relatedBudget)
		outputEvidence += "\n" + excerpt
		parts = append(parts, "", "## Related source", "**Source:** "+source.url, "", excerpt)
	}
	// One additional bounded expansion if required code/API evidence remains
	// missing. This follows references from the implementation pages we actually
	// found, not arbitrary link hops across the entire site.
	missing := missingResearchEvidence(question, allRetrieved)
	if len(missing) > 0 {
		var next []sourceCandidate
		for _, source := range more {
			if source.err != nil {
				continue
			}
			if strings.Contains(strings.ToLower(source.content), "article has multiple variants") {
				for _, candidate := range relatedSourceLinks(source.content, source.url, question, 8) {
					if !seenURLs[candidate.url] {
						seenURLs[candidate.url] = true
						candidate.score += 50 // A published page variant is the actual documentation, not an index.
						if strings.Contains(strings.ToLower(question), "hosted") &&
							strings.Contains(strings.ToLower(candidate.url+" "+candidate.label), "hosted") {
							candidate.score += 20
						}
						next = append(next, candidate)
					}
				}
			}
			for _, candidate := range relatedSourceLinks(source.content, source.url, strings.Join(missing, " "), 8) {
				if !seenURLs[candidate.url] {
					seenURLs[candidate.url] = true
					candidate.score += implementationLinkBonus(candidate, question)
					next = append(next, candidate)
				}
			}
		}
		sort.SliceStable(next, func(i, j int) bool { return next[i].score > next[j].score })
		if len(next) > maxFollowupSources {
			next = next[:maxFollowupSources]
		}
		followups := readSources(next, reader)
		more = append(more, followups...)
		for _, source := range followups {
			if source.err != nil {
				continue
			}
			successful++
			relatedSuccessful++
			allRetrieved += "\n" + source.content
			followupBudget := relatedExcerptBudget(source.url)
			if asksForAPIContracts(question) && followupBudget > 3000 {
				followupBudget = 3000
			}
			excerpt := focusExcerpt(copyableCurlExamples(source.content), question, followupBudget)
			outputEvidence += "\n" + excerpt
			parts = append(parts, "", "## Supporting implementation reference", "**Source:** "+source.url, "", excerpt)
		}
	}
	if openAPIDone != nil {
		if contract := <-openAPIDone; contract.Content != "" {
			successful++
			outputEvidence += "\n" + contract.Content
			// Put exact REST contracts first so coding agents encounter the
			// implementation-ready operations before lengthy source excerpts.
			front := []string{"## REST API contract", "**Source:** " + contract.URL, "", contract.Content, ""}
			parts = append(append(append([]string(nil), parts[:4]...), front...), parts[4:]...)
		}
	}
	if structured := <-structuredDone; structured != "" {
		outputEvidence += "\n" + structured
		parts = append(parts, "", structured)
	}
	if unmet := missingResearchEvidence(question, outputEvidence); len(unmet) > 0 {
		parts = append(parts, "", "**Requested details not fully evidenced in returned excerpts:** "+strings.Join(unmet, ", ")+". Follow the cited source pages rather than assuming these details.")
	}
	parts = append(parts, "", fmt.Sprintf("**Coverage:** %d source document(s) inspected. Extracts are source-grounded, not an independently verified synthesis. REST commands assembled from an OpenAPI operation and its published example are labeled as such.", successful))
	if failed := len(more) - relatedSuccessful; failed > 0 {
		parts = append(parts, fmt.Sprintf("%d related page(s) could not be retrieved; the primary source is preserved.", failed))
	}
	return strings.Join(parts, "\n"), nil
}

// enrichSearch gives agents real documentation content in their first search
// response, with the original ranked source list retained for navigation.
func enrichSearch(query string, ranked []SearchDoc) string {
	return enrichSearchWithReader(query, ranked, ReadLink)
}

func enrichSearchWithReader(query string, ranked []SearchDoc, reader func(string) (string, error)) string {
	chosen := chooseSearchSources(query, ranked, maxSearchSources)
	if len(chosen) == 0 {
		return ""
	}
	more := readSources(chosen, reader)
	parts := []string{}
	seen := make(map[string]bool)
	for _, source := range more {
		if source.err != nil {
			continue
		}
		seen[source.url] = true
		parts = append(parts, "### "+source.url, "", focusExcerpt(source.content, query, searchSourceBudget), "")
	}
	if len(parts) == 0 {
		return ""
	}
	// Expand high-relevance documentation references from retrieved results,
	// not arbitrary websites or unlimited recursive link trees.
	var follow []sourceCandidate
	for _, source := range more {
		if source.err != nil || len(follow) >= 2 {
			continue
		}
		for _, candidate := range relatedSourceLinks(source.content, source.url, query, 2) {
			if seen[candidate.url] {
				continue
			}
			seen[candidate.url] = true
			follow = append(follow, candidate)
			if len(follow) >= 2 {
				break
			}
		}
	}
	for _, source := range readSources(follow, reader) {
		if source.err == nil {
			parts = append(parts, "### Supporting reference: "+source.url, "", focusExcerpt(source.content, query, focusedRelatedBudget), "")
		}
	}
	return "## Retrieved source context\n\nThe following are excerpts from source pages, not an AI-generated answer. Compare versions and follow the URLs for full documents.\n\n" + strings.Join(parts, "\n")
}

func linksFromDocsIndex(rawURL, question string, limit int) []sourceCandidate {
	return linksFromDocsIndexWithPrimary(rawURL, "", question, limit)
}

func linksFromDocsIndexWithPrimary(rawURL, primary, question string, limit int) []sourceCandidate {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "github.com" || directSourceLanguage(rawURL) != "" {
		return nil
	}
	if !strings.HasPrefix(u.Hostname(), "docs.") && !strings.Contains(u.Path, "/docs/") &&
		!strings.Contains(u.Path, "/reference/") && !strings.Contains(primary, "llms.txt") {
		return nil
	}
	indexURLs := documentIndexURLs(u, primary)
	var candidates []sourceCandidate
	seen := map[string]bool{}
	for _, indexURL := range indexURLs {
		if rawURL == indexURL {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		index, err := fetchPublicText(ctx, indexURL, 300000)
		cancel()
		if err != nil || !strings.Contains(index, "](") {
			continue
		}
		for _, item := range relatedSourceLinks(index, indexURL, question, 300) {
			item.score += implementationLinkBonus(item, question)
			item.score += sourceScopeBonus(item, rawURL, question)
			if !seen[item.url] {
				seen[item.url] = true
				candidates = append(candidates, item)
			}
		}
		if len(candidates) >= limit {
			break
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates
}

func readSources(candidates []sourceCandidate, reader func(string) (string, error)) []sourceRead {
	result := make([]sourceRead, len(candidates))
	var wg sync.WaitGroup
	for i, candidate := range candidates {
		wg.Add(1)
		go func(i int, candidate sourceCandidate) {
			defer wg.Done()
			content, err := reader(candidate.url)
			result[i] = sourceRead{url: candidate.url, content: content, err: err}
		}(i, candidate)
	}
	wg.Wait()
	return result
}

func chooseSearchSources(query string, ranked []SearchDoc, limit int) []sourceCandidate {
	items := make([]sourceCandidate, 0, limit)
	terms := queryTerms(query)
	for i, source := range ranked {
		if i >= 18 {
			break
		}
		if err := validateSourceURL(source.URL); err != nil || disallowedSourcePath(source.URL) {
			continue
		}
		u, _ := url.Parse(source.URL)
		score := 18 - i + termScore(source.Title, terms)*2 + termScore(u.Hostname()+" "+u.Path, terms)
		if strings.HasPrefix(u.Hostname(), "docs.") || strings.Contains(u.Path, "/docs/") || strings.Contains(u.Path, "/reference/") {
			score += 7
		}
		if strings.Contains(u.Path, "/specification/") || strings.Contains(u.Path, "/api-reference/") || strings.Contains(u.Path, "/sdk/") {
			score += 7
		}
		if u.Hostname() == "github.com" || strings.Contains(u.Path, "openapi") {
			score += 5
		}
		if strings.Contains(u.Path, "/community/") || strings.Contains(u.Path, "/interest-groups/") {
			score -= 16 // governance pages rarely contain usable API contracts
		}
		if strings.Contains(u.Path, "/blog/") || strings.HasPrefix(u.Hostname(), "blog.") {
			score -= 5
		}
		if strings.Contains(u.Path, "/internal/") && u.Hostname() == "pkg.go.dev" {
			score -= 12 // internal Go packages aren't public SDK contracts
		}
		if strings.Contains(strings.ToLower(query), "go sdk") && strings.Contains(strings.ToLower(source.Title+" "+u.Path), "go-sdk") {
			score += 7
		}
		items = append(items, sourceCandidate{url: source.URL, label: source.Title, score: score, order: i})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].score > items[j].score })
	selected := make([]sourceCandidate, 0, limit)
	seenHosts := make(map[string]int)
	for _, item := range items {
		if len(selected) == limit {
			break
		}
		u, _ := url.Parse(item.url)
		if seenHosts[u.Hostname()] >= 2 { // diversify while allowing two relevant docs pages
			continue
		}
		seenHosts[u.Hostname()]++
		selected = append(selected, item)
	}
	return selected
}

func relatedSourceLinks(markdown, baseURL, question string, limit int) []sourceCandidate {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	terms := queryTerms(question)
	seen := map[string]bool{canonicalSourceURL(base): true}
	var links []sourceCandidate
	for i, match := range mdLinkPattern.FindAllStringSubmatch(markdown, 1000) {
		destination := strings.TrimSpace(match[2])
		if strings.Contains(destination, "\\") { // malformed Markdown-escaped URLs
			continue
		}
		ref, err := url.Parse(destination)
		if err != nil {
			continue
		}
		u := base.ResolveReference(ref)
		if !strings.EqualFold(base.Hostname(), u.Hostname()) || base.Port() != u.Port() || u.User != nil {
			continue
		}
		if err := validateSourceURL(u.String()); err != nil || disallowedSourcePath(u.String()) || sensitiveURL(u) {
			continue
		}
		key := canonicalSourceURL(u)
		if seen[key] {
			continue
		}
		seen[key] = true
		label := strings.TrimSpace(match[1])
		score := termScore(label, terms)*4 + termScore(u.Path, terms)*3
		if score == 0 { // no relevant relationship to the user's question
			continue
		}
		if len(terms) >= 4 && len(matchedLinkTerms(label+" "+u.Path, terms)) == 1 {
			term := matchedLinkTerms(label+" "+u.Path, terms)[0]
			if len(term) <= 5 { // don't expand generic links matching only 'MCP' or 'tools'
				continue
			}
		}
		links = append(links, sourceCandidate{url: key, label: label, score: score, order: i})
	}
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].score == links[j].score {
			return links[i].order < links[j].order
		}
		return links[i].score > links[j].score
	})
	if len(links) > limit {
		links = links[:limit]
	}
	return links
}

func matchedLinkTerms(text string, terms []string) []string {
	var matched []string
	for _, term := range terms {
		if termScore(text, []string{term}) > 0 {
			matched = append(matched, term)
		}
	}
	return matched
}

func canonicalSourceURL(u *url.URL) string {
	copy := *u
	copy.Fragment = ""
	copy.RawFragment = ""
	return copy.String()
}

func sensitiveURL(u *url.URL) bool {
	for name := range u.Query() {
		name = strings.ToLower(name)
		if strings.Contains(name, "token") || strings.Contains(name, "secret") || strings.Contains(name, "key") || strings.Contains(name, "auth") || strings.Contains(name, "signature") {
			return true
		}
	}
	return false
}

func disallowedSourcePath(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	path := strings.ToLower(u.Path)
	for _, suffix := range []string{".png", ".jpg", ".jpeg", ".svg", ".gif", ".webp", ".zip", ".mp4", ".mov", ".mp3", ".woff", ".woff2", ".pdf", ".dmg", ".exe"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func queryTerms(query string) []string {
	stop := map[string]bool{"the": true, "and": true, "for": true, "from": true, "with": true, "that": true, "this": true, "what": true, "how": true, "when": true, "where": true, "does": true, "which": true, "using": true, "about": true, "into": true, "can": true, "are": true, "get": true, "use": true, "you": true, "your": true, "its": true, "have": true}
	seen := make(map[string]bool)
	var terms []string
	for _, term := range sourceWordPattern.FindAllString(strings.ToLower(query), 40) {
		if len(term) < 3 {
			continue
		}
		if stop[term] || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	return terms
}

func termScore(text string, terms []string) int {
	// Compare words instead of substrings. Without this, an "api" search
	// incorrectly matches "scraping" and chooses unrelated pages.
	words := sourceWordPattern.FindAllString(strings.ToLower(text), -1)
	count := 0
	for _, term := range terms {
		// Approximate inflection matching: pagination / paginate, authenticated /
		// authentication, etc. Exact matches are weighted slightly higher.
		root := term
		if len(root) > 5 {
			root = root[:5]
		}
		for _, word := range words {
			if word == term {
				count += 2
				break
			}
			if len(root) >= 4 && strings.HasPrefix(word, root) {
				count++
				break
			}
		}
	}
	return count
}

func splitResearchSections(markdown string, terms []string) []researchSection {
	lines := strings.Split(markdown, "\n")
	sections := make([]researchSection, 0, 20)
	current := researchSection{heading: "Introduction"}
	var body strings.Builder
	inFence := false
	flush := func() {
		current.body = strings.TrimSpace(body.String())
		if current.body != "" {
			current.index = len(sections)
			current.score = 5*termScore(current.heading, terms) + termScore(current.body, terms)
			if strings.Contains(current.body, "```") && current.score > 0 {
				current.score += 2
			}
			sections = append(sections, current)
		}
		body.Reset()
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inFence {
			if heading := headingPattern.FindStringSubmatch(trimmed); len(heading) == 2 {
				flush()
				current = researchSection{heading: heading[1]}
			}
		}
		body.WriteString(line)
		body.WriteByte('\n')
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
	}
	flush()
	return sections
}

func focusExcerpt(markdown, question string, budget int) string {
	markdown = strings.TrimSpace(markdown)
	if len(markdown) <= budget {
		return markdown
	}
	if focused := focusedRawJSON(markdown, question, budget); focused != "" {
		return focused
	}
	sections := splitResearchSections(markdown, queryTerms(question))
	if len(sections) == 0 {
		return boundedMarkdown(markdown, budget)
	}
	sorted := append([]researchSection(nil), sections...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].score == sorted[j].score {
			return sorted[i].index < sorted[j].index
		}
		return sorted[i].score > sorted[j].score
	})
	selected := make([]researchSection, 0, 5)
	for _, section := range sorted {
		if len(selected) >= 5 || (section.score == 0 && len(selected) > 0) {
			break
		}
		selected = append(selected, section)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].index < selected[j].index })
	var parts []string
	remaining := budget
	for i, section := range selected {
		if remaining < 160 {
			break
		}
		// Allocate budget across selected sections so one huge section doesn't
		// crowd out exact schemas or examples from the other sections.
		allocation := remaining / (len(selected) - i)
		if allocation < 500 && remaining >= 500 {
			allocation = 500
		}
		text := atomicFocusedSection(section.body, question, allocation)
		parts = append(parts, text)
		remaining -= len(text)
	}
	result := strings.Join(parts, "\n\n")
	return result + "\n\n*[Selected excerpts; open the source URL to inspect omitted sections.]*"
}

func boundedMarkdown(markdown string, budget int) string {
	if len(markdown) <= budget {
		return markdown
	}
	if budget <= 80 {
		return "*[Section too large; consult source.]*"
	}
	lines := strings.Split(markdown, "\n")
	var out strings.Builder
	inFence := false
	fence := "```"
	for _, line := range lines {
		if out.Len()+len(line)+len("\n…\n```") > budget {
			break
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if !inFence {
				fence = trimmed[:3]
			}
			inFence = !inFence
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	if inFence {
		out.WriteString(fence)
		out.WriteByte('\n')
	}
	out.WriteString("*[Excerpt shortened; see source for full content.]*")
	return out.String()
}
