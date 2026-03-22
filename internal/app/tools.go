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
}

type scoredURL struct {
	Result         SearchDoc
	TotalScore     float64
	DuplicateCount int
	BestPosition   int
	FinalScore     float64
}

func Search(params SearchParams) (string, error) {
	defaultExcludedDomains := []string{"youtube.com", "vimeo.com", "dailymotion.com", "twitch.tv", "tiktok.com", "instagram.com", "facebook.com"}
	allExcludedDomains := append(append([]string{}, defaultExcludedDomains...), params.ExcludeDomains...)
	truncatedKeyword := truncateWords(params.IncludeKeyword, 5)

	type namedSearch struct {
		name string
		fn   func(context.Context) (SearchResult, error)
	}

	searches := []namedSearch{}
	if strings.TrimSpace(truncatedKeyword) != "" {
		searches = append(searches, namedSearch{name: "Exa", fn: func(ctx context.Context) (SearchResult, error) {
			return searchWithExa(ctx, params.Query, nil, truncatedKeyword)
		}})
	} else {
		searches = append(searches,
			namedSearch{name: "Brave", fn: func(ctx context.Context) (SearchResult, error) { return searchWithBrave(ctx, params.Query) }},
			namedSearch{name: "Tavily", fn: func(ctx context.Context) (SearchResult, error) { return searchWithTavily(ctx, params.Query, nil) }},
			namedSearch{name: "Exa", fn: func(ctx context.Context) (SearchResult, error) { return searchWithExa(ctx, params.Query, nil, "") }},
		)
	}

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
				results[i] = providerDocs{Provider: s.name, Docs: []SearchDoc{}}
				return
			}
			results[i] = providerDocs{Provider: s.name, Docs: res.Docs}
		}(i, s)
	}
	wg.Wait()

	total := 0
	for _, r := range results {
		total += len(r.Docs)
	}
	if total == 0 {
		return "", errors.New("Error searching the web: All search providers failed to return results")
	}

	filtered := filterExcludedDomains(results, allExcludedDomains)
	ranked, _ := scoreAndRankResults(filtered)
	if len(ranked) > 35 {
		ranked = ranked[:35]
	}

	parts := []string{fmt.Sprintf("Total Results: %d\n", len(ranked))}
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

func ReadLink(rawURL string) (string, error) {
	if result, _ := fetchGitHubRawContent(rawURL); result != nil {
		return formatReadLink(result.Title, result.URL, result.Markdown), nil
	}

	if ok, _ := checkMarkdownAvailable(rawURL); ok {
		result, err := fetchMarkdownContent(rawURL)
		if err == nil {
			return formatReadLink(result.Title, result.URL, result.Markdown), nil
		}
	}

	apiKey := strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY"))
	if apiKey == "" {
		return "", errors.New("Error reading web page: FIRECRAWL_API_KEY environment variable is required for non-.md URLs")
	}

	requestBody := map[string]any{
		"url":                 rawURL,
		"formats":             []string{"markdown"},
		"onlyMainContent":     true,
		"skipTlsVerification": true,
		"blockAds":            true,
		"removeBase64Images":  true,
		"maxAge":              600000,
		"excludeTags":         []string{"script", "style", "meta", "noscript", "svg", "img", "nav", "footer", "header", "aside", ".advertisement", "#ad"},
	}
	if strings.HasSuffix(strings.ToLower(rawURL), ".pdf") {
		requestBody["parsers"] = []string{"pdf"}
	}

	data, err := getFirecrawlQueue().enqueue(rawURL, func() (map[string]any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		body, err := doJSONRequest(ctx, http.MethodPost, "https://api.firecrawl.dev/v2/scrape", map[string]string{
			"Authorization": "Bearer " + apiKey,
			"Content-Type":  "application/json",
		}, requestBody)
		if err != nil {
			return nil, err
		}
		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		if success, _ := parsed["success"].(bool); !success {
			return nil, fmt.Errorf("Scraping failed for %s: %v", rawURL, parsed["error"])
		}
		return parsed, nil
	})
	if err != nil {
		return "", fmt.Errorf("Error reading web page: %v", err)
	}

	dataMap, _ := data["data"].(map[string]any)
	metadata, _ := dataMap["metadata"].(map[string]any)
	title, _ := metadata["title"].(string)
	markdown, _ := dataMap["markdown"].(string)
	if markdown == "" {
		markdown = "No content extracted"
	}
	return formatReadLink(title, rawURL, markdown), nil
}

func MapSite(rawURL string) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY"))
	if apiKey == "" {
		return "", errors.New("Error mapping website: FIRECRAWL_API_KEY environment variable is required")
	}

	requestBody := map[string]any{
		"url":                   rawURL,
		"sitemap":               "include",
		"includeSubdomains":     true,
		"ignoreQueryParameters": true,
		"limit":                 5000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	body, err := doJSONRequest(ctx, http.MethodPost, "https://api.firecrawl.dev/v2/map", map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Content-Type":  "application/json",
	}, requestBody)
	if err != nil {
		return "", fmt.Errorf("Error mapping website: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("Error mapping website: %v", err)
	}
	if success, _ := parsed["success"].(bool); !success {
		return "", fmt.Errorf("Error mapping website: Mapping failed for %s: %v", rawURL, parsed["error"])
	}

	linksAny, _ := parsed["links"].([]any)
	parts := []string{fmt.Sprintf("total urls found: %d", len(linksAny)), ""}
	for _, item := range linksAny {
		switch link := item.(type) {
		case string:
			parts = append(parts, "- "+link, "")
		case map[string]any:
			parts = append(parts, "- "+stringValue(link["url"]))
			if v := stringValue(link["title"]); v != "" {
				parts = append(parts, "- "+v)
			}
			if v := stringValue(link["description"]); v != "" {
				parts = append(parts, "- "+v)
			}
			parts = append(parts, "")
		}
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

func searchWithBrave(ctx context.Context, query string) (SearchResult, error) {
	apiKey := strings.TrimSpace(os.Getenv("BRAVE_API_KEY"))
	if apiKey == "" {
		return SearchResult{}, errors.New("missing BRAVE_API_KEY")
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("text_decorations", "false")
	params.Set("result_filter", "web")
	params.Set("limit", "20")
	body, err := doRawRequest(ctx, http.MethodGet, "https://api.search.brave.com/res/v1/web/search?"+params.Encode(), map[string]string{
		"Accept":               "application/json",
		"Accept-Encoding":      "gzip",
		"x-subscription-token": apiKey,
	}, nil)
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
	apiKey := strings.TrimSpace(os.Getenv("TAVILY_API_KEY"))
	if apiKey == "" {
		return SearchResult{}, errors.New("missing TAVILY_API_KEY")
	}
	requestBody := map[string]any{
		"api_key":     apiKey,
		"query":       query,
		"max_results": 20,
	}
	if len(excludeDomains) > 0 {
		requestBody["exclude_domains"] = excludeDomains
	}
	body, err := doJSONRequest(ctx, http.MethodPost, "https://api.tavily.com/search", map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}, requestBody)
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
	apiKey := strings.TrimSpace(os.Getenv("EXA_API_KEY"))
	if apiKey == "" {
		return SearchResult{}, errors.New("missing EXA_API_KEY")
	}
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
	body, err := doJSONRequest(ctx, http.MethodPost, "https://api.exa.ai/search", map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"x-api-key":    apiKey,
	}, requestBody)
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
			urlScores[normalized] = &scoredURL{Result: doc, TotalScore: weightedScore, DuplicateCount: 1, BestPosition: position}
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
			return scored[i].Result.URL < scored[j].Result.URL
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
			return nil, fmt.Errorf("API request failed: %s - %s", resp.Status, strings.TrimSpace(string(respBody)))
		}
		return nil, fmt.Errorf("API request failed: %s", resp.Status)
	}
	return respBody, nil
}

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
