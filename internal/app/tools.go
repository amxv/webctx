package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SearchParams struct {
	Query          string
	ExcludeDomains []string
	IncludeKeyword string
}

type SearchDoc struct {
	URL      string
	Title    string
	Overview string
}

type SearchResult struct {
	Docs []SearchDoc
}

type providerDocs struct {
	Docs     []SearchDoc
	Provider string
	Err      error
}

type namedSearch struct {
	name string
	fn   func(context.Context) (SearchResult, error)
}

type scoredURL struct {
	Result         SearchDoc
	InsertionOrder int
	TotalScore     float64
	DuplicateCount int
	BestPosition   int
	FinalScore     float64
}

func Search(params SearchParams) (string, error) {
	config, err := currentProviderConfig()
	if err != nil {
		return "", err
	}
	if !config.enabled("brave") && !config.enabled("tavily") && !config.enabled("exa") {
		return "", errors.New("Error searching the web: all search providers are disabled; run 'webctx providers enable brave', 'webctx providers enable tavily', or 'webctx providers enable exa'")
	}
	defaultExcludedDomains := []string{"youtube.com", "vimeo.com", "dailymotion.com", "twitch.tv", "tiktok.com", "instagram.com", "facebook.com"}
	allExcludedDomains := append(append([]string{}, defaultExcludedDomains...), params.ExcludeDomains...)
	truncatedKeyword := truncateWords(params.IncludeKeyword, 5)

	// Free Alexandria tool discovery proceeds alongside normal web search.
	// It never executes a paid tool or replaces source-grounded web results.
	alexandriaDone := make(chan string, 1)
	go func() { alexandriaDone <- discoverAlexandria(params.Query) }()

	searches := []namedSearch{}
	keywordFallback := false
	if strings.TrimSpace(truncatedKeyword) != "" {
		if config.enabled("exa") && len(searchKeysForProvider("exa")) > 0 {
			searches = append(searches, namedSearch{name: "Exa", fn: func(ctx context.Context) (SearchResult, error) {
				return searchWithExa(ctx, params.Query, nil, truncatedKeyword)
			}})
		}
	} else {
		searches = append(searches, alternativeSearches(config, params.Query)...)
		if config.enabled("exa") {
			searches = append(searches, namedSearch{name: "Exa", fn: func(ctx context.Context) (SearchResult, error) {
				return searchWithExa(ctx, params.Query, nil, "")
			}})
		}
	}

	results := runSearches(searches)
	if truncatedKeyword != "" && (len(searches) == 0 || (len(results) == 1 && results[0].Err != nil)) {
		// Exa's includeText filter has no equivalent in Brave or Tavily. When Exa
		// is disabled, unconfigured, or fails, search for the quoted phrase instead
		// of making keyword searches unusable. Disclose the weaker semantics.
		fallbackQuery := params.Query + " " + strconv.Quote(truncatedKeyword)
		fallbackSearches := alternativeSearches(config, fallbackQuery)
		if len(fallbackSearches) > 0 {
			results = append(results, runSearches(fallbackSearches)...)
			keywordFallback = true
		} else if len(searches) == 0 && config.enabled("exa") {
			// Preserve the helpful missing-key error if Exa is the only enabled
			// provider and it has no key.
			results = append(results, providerDocs{Provider: "Exa", Err: errors.New("missing EXA_API_KEY")})
		}
	}
	if len(results) == 0 {
		return "", errors.New("Error searching the web: no enabled search providers are available for this query; use 'webctx status' to inspect settings")
	}

	total := countSearchDocs(results)
	if total == 0 {
		missingKeys := uniqueMissingCredentialKeys(results)
		if len(missingKeys) > 0 {
			return "", missingCredentialsError("Error searching the web", missingKeys, "")
		}

		failures := make([]string, 0, len(results))
		for _, r := range results {
			if r.Err == nil {
				continue
			}
			failures = append(failures, fmt.Sprintf("%s: %v", r.Provider, r.Err))
		}
		if len(failures) > 0 {
			return "", fmt.Errorf("Error searching the web: all search providers failed (%s)", strings.Join(failures, "; "))
		}

		return "", errors.New("Error searching the web: all search providers failed to return results")
	}

	filtered := filterExcludedDomains(results, allExcludedDomains)
	ranked, _ := scoreAndRankResults(filtered)
	if len(ranked) > 35 {
		ranked = ranked[:35]
	}

	parts := []string{fmt.Sprintf("Total Results: %d\n", len(ranked))}
	if keywordFallback {
		parts = append(parts, "> **Keyword fallback:** Exa was disabled or unavailable. Brave/Tavily searched for the quoted phrase; this is not a strict page-text inclusion filter.", "")
	}
	// The ranked index remains available, but agents also get useful source
	// context without having to issue several follow-up read-link calls.
	if context := enrichSearch(params.Query, ranked); context != "" {
		parts = append(parts, context, "")
	}
	if tools := <-alexandriaDone; tools != "" {
		parts = append(parts, "", "## Related data providers", tools, "")
	}
	parts = append(parts, "## Search results", "")
	for _, doc := range ranked {
		title := decodeHTML(doc.Title)
		overview := decodeHTML(doc.Overview)
		parts = append(parts, fmt.Sprintf("- [%s](%s)", title, doc.URL))
		if overview != "" {
			parts = append(parts, fmt.Sprintf("    - %s", overview))
		}
		parts = append(parts, "")
	}
	return strings.Join(parts, "\n"), nil
}

func alternativeSearches(config providerConfig, query string) []namedSearch {
	searches := []namedSearch{}
	if config.enabled("brave") {
		searches = append(searches, namedSearch{name: "Brave", fn: func(ctx context.Context) (SearchResult, error) { return searchWithBrave(ctx, query) }})
	}
	if config.enabled("tavily") {
		searches = append(searches, namedSearch{name: "Tavily", fn: func(ctx context.Context) (SearchResult, error) { return searchWithTavily(ctx, query, nil) }})
	}
	return searches
}

func countSearchDocs(results []providerDocs) int {
	count := 0
	for _, r := range results {
		count += len(r.Docs)
	}
	return count
}

func runSearches(searches []namedSearch) []providerDocs {
	results := make([]providerDocs, len(searches))
	var wg sync.WaitGroup
	for i, s := range searches {
		wg.Add(1)
		go func(i int, s namedSearch) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			res, err := s.fn(ctx)
			if err != nil {
				results[i] = providerDocs{Provider: s.name, Docs: []SearchDoc{}, Err: err}
				return
			}
			results[i] = providerDocs{Provider: s.name, Docs: res.Docs}
		}(i, s)
	}
	wg.Wait()
	return results
}

func ReadLink(rawURL string) (string, error) {
	if err := validateSourceURL(rawURL); err != nil {
		return "", err
	}
	native := readGitHubNative(rawURL)
	switch native.Outcome {
	case GitHubNativeSuccess:
		return native.Markdown, nil
	case GitHubNativeFailure:
		if shouldBestEffortCrawlGitHubPackage(rawURL, native.Err) {
			title, markdown, crawlErr := scrapeLinkWithFirecrawl(rawURL)
			if crawlErr == nil && usefulGitHubPackageCrawl(title, markdown) {
				notice := "> **Best-effort GitHub Package page crawl:** GitHub's structured Package API was unavailable for the current credential, so webctx crawled the public GitHub page with Firecrawl instead. This output may be incomplete or contain GitHub UI noise."
				return formatReadLink(title, rawURL, notice+"\n\n"+markdown), nil
			}
			if crawlErr == nil {
				crawlErr = fmt.Errorf("Firecrawl did not return a recognizable public GitHub Package page")
			}
			return "", fmt.Errorf("%v Best-effort public GitHub Package crawl also failed: %v", native.Err, crawlErr)
		}
		return "", native.Err
	}

	if direct, ok := readDirectSource(rawURL); ok {
		return direct, nil
	}

	nativeStarted := time.Now()
	if result, err := readNativeMarkdown(rawURL); err == nil && result != nil {
		traceRetrieval(rawURL, "native-markdown", "success", nativeStarted, nil)
		return formatReadLink(result.Title, result.URL, "**Markdown source:** "+result.SourceURL+"\n\n"+result.Markdown), nil
	}
	traceRetrieval(rawURL, "native-markdown", "miss", nativeStarted, nil)

	title, markdown, err := scrapeLinkWithFirecrawl(rawURL)
	if err != nil {
		return "", err
	}
	return formatReadLink(title, rawURL, markdown), nil
}

func shouldBestEffortCrawlGitHubPackage(rawURL string, nativeErr error) bool {
	if strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY")) == "" {
		return false
	}
	target := parseGitHubTarget(rawURL)
	if target == nil || target.Kind != GitHubTargetPackage {
		return false
	}
	ghErr, ok := nativeErr.(*GitHubError)
	if !ok {
		return false
	}
	return ghErr.Kind == GitHubErrorAuthentication || ghErr.Kind == GitHubErrorForbidden
}

func usefulGitHubPackageCrawl(title, markdown string) bool {
	title = strings.ToLower(strings.TrimSpace(title))
	markdown = strings.TrimSpace(markdown)
	if markdown == "" || strings.Contains(title, "page not found") || strings.Contains(title, "404") {
		return false
	}
	return strings.Contains(title, "package") && len(markdown) >= 100
}

func scrapeLinkWithFirecrawl(rawURL string) (string, string, error) {
	apiKey := strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY"))
	if apiKey == "" {
		return "", "", missingCredentialsError("Error reading web page", []string{"FIRECRAWL_API_KEY"}, "for non-.md URLs")
	}
	// Firecrawl's default auto proxy already tries basic then enhanced when the
	// transport fails. A second explicit enhanced attempt is only useful when a
	// scrape responds successfully but returns a challenge/empty document.
	started := time.Now()
	first, err := firecrawlScrape(rawURL, apiKey, "")
	if err == nil && usableScrapedMarkdown(first.Markdown) {
		traceRetrieval(rawURL, "firecrawl-auto", "success", started, nil)
		return first.Title, withSourceTools(first.Markdown, first.ToolSummary), nil
	}
	lastError := err
	traceRetrieval(rawURL, "firecrawl-auto", "escalate", started, err)
	if err == nil {
		lastError = fmt.Errorf("Firecrawl returned empty or blocked page content: %s", challengeReason(first.Markdown))
		started = time.Now()
		second, enhancedErr := firecrawlScrape(rawURL, apiKey, "enhanced")
		if enhancedErr == nil && usableScrapedMarkdown(second.Markdown) {
			traceRetrieval(rawURL, "firecrawl-enhanced", "success", started, nil)
			return second.Title, withSourceTools(second.Markdown, second.ToolSummary), nil
		}
		traceRetrieval(rawURL, "firecrawl-enhanced", "escalate", started, enhancedErr)
		if enhancedErr != nil {
			lastError = enhancedErr
		}
	}
	started = time.Now()
	title, markdown, browserErr := firecrawlBrowserFallback(rawURL, apiKey)
	if browserErr == nil && usableScrapedMarkdown(markdown) {
		traceRetrieval(rawURL, "browser", "success", started, nil)
		return title, markdown, nil
	}
	if browserErr == nil {
		browserErr = fmt.Errorf("browser produced no usable content: %s", challengeReason(markdown))
	}
	traceRetrieval(rawURL, "browser", "blocked", started, browserErr)
	return "", "", fmt.Errorf("Error reading web page: scrape: %v; browser fallback: %w", lastError, browserErr)
}

func MapSite(rawURL string) (string, error) {
	data, err := MapSitePage(MapSiteInput{URL: rawURL})
	if err != nil {
		return "", err
	}
	parts := []string{
		fmt.Sprintf("total urls found: %v", data["total_discovered"]),
		fmt.Sprintf("matching pages: %v; displayed: %v (offset %v)", data["total_matching"], data["count"], data["offset"]),
		"",
	}
	for _, entry := range data["links"].([]map[string]any) {
		parts = append(parts, "- "+stringField(entry["url"]))
		if title := stringField(entry["title"]); title != "" {
			parts = append(parts, "  - "+title)
		}
		if desc := stringField(entry["description"]); desc != "" {
			parts = append(parts, "  - "+desc)
		}
	}
	if data["next"] != nil {
		parts = append(parts, "", "More pages available; use --offset with webctx map-site for the next page.")
	}
	return strings.Join(parts, "\n"), nil
}

func formatReadLink(title, rawURL, markdown string) string {
	parts := []string{}
	if strings.TrimSpace(title) != "" {
		parts = append(parts, "# "+title, "")
	}
	parts = append(parts, "**URL:** "+rawURL, "", markdown)
	return strings.Join(parts, "\n")
}

func uniqueMissingCredentialKeys(results []providerDocs) []string {
	keys := map[string]struct{}{}
	for _, r := range results {
		if r.Err == nil {
			continue
		}
		msg := r.Err.Error()
		if !strings.HasPrefix(msg, "missing ") {
			continue
		}
		key := strings.TrimSpace(strings.TrimPrefix(msg, "missing "))
		if key != "" {
			keys[key] = struct{}{}
		}
	}

	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func missingCredentialsError(prefix string, keys []string, detail string) error {
	sort.Strings(keys)
	keysText := strings.Join(keys, ", ")
	suffix := ""
	if strings.TrimSpace(detail) != "" {
		suffix = " " + strings.TrimSpace(detail)
	}

	return fmt.Errorf(
		"%s: missing %s%s. Set them as environment variables, place them in a .env.local next to the binary, or store them in macOS Keychain with service %q and account names matching the env vars",
		prefix,
		keysText,
		suffix,
		keychainServiceName,
	)
}

func searchWithBrave(ctx context.Context, query string) (SearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("text_decorations", "false")
	params.Set("result_filter", "web")
	params.Set("limit", "20")
	body, err := useSearchKeys(ctx, "brave", func(apiKey string) ([]byte, error) {
		return doRawRequest(ctx, http.MethodGet, "https://api.search.brave.com/res/v1/web/search?"+params.Encode(), map[string]string{
			"Accept":               "application/json",
			"x-subscription-token": apiKey,
		}, nil)
	})
	if err != nil {
		return SearchResult{}, err
	}
	var parsed struct {
		Web struct {
			Results []struct {
				URL         string `json:"url"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return SearchResult{}, err
	}
	docs := make([]SearchDoc, 0, min(20, len(parsed.Web.Results)))
	for i, r := range parsed.Web.Results {
		if i >= 20 {
			break
		}
		docs = append(docs, SearchDoc{URL: r.URL, Title: r.Title, Overview: r.Description})
	}
	return SearchResult{Docs: docs}, nil
}

func searchWithTavily(ctx context.Context, query string, excludeDomains []string) (SearchResult, error) {
	requestBody := map[string]any{
		"query":       query,
		"max_results": 20,
	}
	if len(excludeDomains) > 0 {
		requestBody["exclude_domains"] = excludeDomains
	}
	body, err := useSearchKeys(ctx, "tavily", func(apiKey string) ([]byte, error) {
		requestBody["api_key"] = apiKey
		return doJSONRequest(ctx, http.MethodPost, "https://api.tavily.com/search", map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		}, requestBody)
	})
	if err != nil {
		return SearchResult{}, err
	}
	var parsed struct {
		Results []struct {
			URL     string `json:"url"`
			Title   string `json:"title"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return SearchResult{}, err
	}
	docs := make([]SearchDoc, 0, min(20, len(parsed.Results)))
	for i, r := range parsed.Results {
		if i >= 20 {
			break
		}
		docs = append(docs, SearchDoc{URL: r.URL, Title: r.Title, Overview: r.Content})
	}
	return SearchResult{Docs: docs}, nil
}

func searchWithExa(ctx context.Context, query string, excludeDomains []string, includeKeyword string) (SearchResult, error) {
	requestBody := map[string]any{
		"query":      query,
		"type":       "auto",
		"numResults": 25,
		"contents": map[string]any{
			"livecrawl": "preferred",
		},
	}
	if len(excludeDomains) > 0 {
		requestBody["excludeDomains"] = excludeDomains
	}
	if strings.TrimSpace(includeKeyword) != "" {
		requestBody["includeText"] = []string{includeKeyword}
	}
	body, err := useSearchKeys(ctx, "exa", func(apiKey string) ([]byte, error) {
		return doJSONRequest(ctx, http.MethodPost, "https://api.exa.ai/search", map[string]string{
			"Accept":       "application/json",
			"Content-Type": "application/json",
			"x-api-key":    apiKey,
		}, requestBody)
	})
	if err != nil {
		return SearchResult{}, err
	}
	var parsed struct {
		Results []struct {
			URL     string `json:"url"`
			Title   string `json:"title"`
			Text    string `json:"text"`
			Summary string `json:"summary"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return SearchResult{}, err
	}
	docs := make([]SearchDoc, 0, min(25, len(parsed.Results)))
	for i, r := range parsed.Results {
		if i >= 25 {
			break
		}
		overview := r.Text
		if overview == "" {
			overview = r.Summary
		}
		docs = append(docs, SearchDoc{URL: r.URL, Title: r.Title, Overview: overview})
	}
	return SearchResult{Docs: docs}, nil
}

func scoreAndRankResults(providerResults []providerDocs) ([]SearchDoc, int) {
	urlScores := map[string]*scoredURL{}
	totalResultsBeforeDedup := 0

	for _, providerResult := range providerResults {
		totalResultsBeforeDedup += len(providerResult.Docs)
		for idx, doc := range providerResult.Docs {
			normalized := normalizeURL(doc.URL)
			position := idx + 1
			weightedScore := float64(getPositionPoints(position)) * getProviderWeight(providerResult.Provider)
			if existing, ok := urlScores[normalized]; ok {
				existing.TotalScore += weightedScore
				existing.DuplicateCount++
				if position < existing.BestPosition {
					existing.BestPosition = position
					existing.Result = doc
				}
				continue
			}
			urlScores[normalized] = &scoredURL{
				Result:         doc,
				InsertionOrder: len(urlScores),
				TotalScore:     weightedScore,
				DuplicateCount: 1,
				BestPosition:   position,
			}
		}
	}

	scored := make([]*scoredURL, 0, len(urlScores))
	for _, item := range urlScores {
		duplicateBonus := 3.0
		if item.BestPosition <= 5 {
			duplicateBonus = 5.0
		}
		duplicatePenalty := 0.0
		if item.DuplicateCount > 3 {
			duplicatePenalty = -2.0
		}
		item.FinalScore = item.TotalScore + float64(item.DuplicateCount-1)*duplicateBonus + duplicatePenalty
		scored = append(scored, item)
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].FinalScore == scored[j].FinalScore {
			return scored[i].InsertionOrder < scored[j].InsertionOrder
		}
		return scored[i].FinalScore > scored[j].FinalScore
	})

	results := make([]SearchDoc, 0, len(scored))
	for _, item := range scored {
		results = append(results, item.Result)
	}
	return results, totalResultsBeforeDedup - len(scored)
}

func getPositionPoints(position int) int {
	scores := map[int]int{1: 30, 2: 27, 3: 24, 4: 21, 5: 19, 6: 16, 7: 13, 8: 11, 9: 9, 10: 7, 11: 5, 12: 4, 13: 3, 14: 2}
	if score, ok := scores[position]; ok {
		return score
	}
	return 1
}

func getProviderWeight(provider string) float64 {
	switch provider {
	case "Ref":
		return 1.25
	case "Exa", "Tavily", "Brave":
		return 1.0
	default:
		return 1.0
	}
}

func filterExcludedDomains(providerResults []providerDocs, excludeDomains []string) []providerDocs {
	if len(excludeDomains) == 0 {
		return providerResults
	}
	normalizedExclude := make(map[string]struct{}, len(excludeDomains))
	for _, domain := range excludeDomains {
		normalizedExclude[strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "www.")] = struct{}{}
	}
	filtered := make([]providerDocs, 0, len(providerResults))
	for _, providerResult := range providerResults {
		docs := make([]SearchDoc, 0, len(providerResult.Docs))
		for _, doc := range providerResult.Docs {
			if _, blocked := normalizedExclude[extractDomain(doc.URL)]; !blocked {
				docs = append(docs, doc)
			}
		}
		filtered = append(filtered, providerDocs{Provider: providerResult.Provider, Docs: docs})
	}
	return filtered
}

func normalizeURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return strings.Split(strings.TrimSuffix(strings.ToLower(raw), "/"), "?")[0]
	}
	base := strings.ToLower(parsed.Scheme + "://" + parsed.Host + strings.TrimSuffix(parsed.EscapedPath(), "/"))
	tracking := map[string]struct{}{"utm_source": {}, "utm_medium": {}, "utm_campaign": {}, "utm_term": {}, "utm_content": {}, "ref": {}, "fbclid": {}, "gclid": {}}
	vals := url.Values{}
	for key, vs := range parsed.Query() {
		if _, skip := tracking[strings.ToLower(key)]; skip {
			continue
		}
		for _, v := range vs {
			vals.Add(key, v)
		}
	}
	if encoded := vals.Encode(); encoded != "" {
		return base + "?" + encoded
	}
	return base
}

func extractDomain(raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Hostname() != "" {
		return strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	}
	cleaned := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(raw), "https://"), "http://")
	cleaned = strings.TrimPrefix(cleaned, "www.")
	return strings.Split(strings.Split(cleaned, "/")[0], "?")[0]
}

func decodeHTML(text string) string {
	replacer := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#x27;", "'", "&#39;", "'", "&apos;", "'")
	return replacer.Replace(text)
}

func truncateWords(s string, maxWords int) string {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) <= maxWords {
		return strings.Join(fields, " ")
	}
	return strings.Join(fields[:maxWords], " ")
}

func doJSONRequest(ctx context.Context, method, rawURL string, headers map[string]string, payload any) ([]byte, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return doRawRequest(ctx, method, rawURL, headers, bodyBytes)
}

func doRawRequest(ctx context.Context, method, rawURL string, headers map[string]string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(respBody) > 0 {
			return nil, &httpStatusError{Code: resp.StatusCode, Message: fmt.Sprintf("API request failed: %s - %s", resp.Status, strings.TrimSpace(string(respBody)))}
		}
		return nil, &httpStatusError{Code: resp.StatusCode, Message: "API request failed: " + resp.Status}
	}
	return respBody, nil
}

type httpStatusError struct {
	Code    int
	Message string
}

func (e *httpStatusError) Error() string { return e.Message }

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
