package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// MapSiteInput retrieves an agent-sized page rather than dumping thousands
// of URLs. An omitted language prefers English; "all" allows all locales.
type MapSiteInput struct {
	URL        string `json:"url" jsonschema:"Website URL to map; HTTP(S) only"`
	Query      string `json:"query,omitempty" jsonschema:"Optional topic to filter and rank discovered pages"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Number of URLs to return, 1-100; defaults to 30"`
	Offset     int    `json:"offset,omitempty" jsonschema:"Pagination offset, zero-based"`
	PathPrefix string `json:"path_prefix,omitempty" jsonschema:"Optional URL path prefix, e.g. /api/"`
	Language   string `json:"language,omitempty" jsonschema:"Locale code such as en or fr; defaults to English, use all for all languages"`
}

type siteMapCacheValue struct {
	links   []any
	expires time.Time
}

var siteMapCache = struct {
	sync.Mutex
	items map[string]siteMapCacheValue
}{items: map[string]siteMapCacheValue{}}
var siteMapFlight singleflight.Group

func rawMappedLinks(ctx context.Context, rawURL string) ([]any, error) {
	if err := validateSourceURL(rawURL); err != nil {
		return nil, &DataError{Code: "invalid_url", Message: err.Error()}
	}
	credential := strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY"))
	if credential == "" {
		return nil, missingCredentialsError("Error mapping website", []string{"FIRECRAWL_API_KEY"}, "")
	}
	digest := sha256.Sum256([]byte(credential + "\x00" + rawURL))
	cacheKey := hex.EncodeToString(digest[:])
	useCache := !strings.HasPrefix(credential, "test")
	if useCache {
		siteMapCache.Lock()
		cached, ok := siteMapCache.items[cacheKey]
		siteMapCache.Unlock()
		if ok && time.Now().Before(cached.expires) {
			return cached.links, nil
		}
	}
	fetch := func() (any, error) {
		request := map[string]any{
			"url": rawURL, "sitemap": "include",
			"includeSubdomains": true, "ignoreQueryParameters": true, "limit": 5000,
		}
		body, err := doJSONRequest(ctx, http.MethodPost, "https://api.firecrawl.dev/v2/map", map[string]string{
			"Authorization": "Bearer " + credential, "Content-Type": "application/json",
		}, request)
		if err != nil {
			return nil, fmt.Errorf("Error mapping website: %w", err)
		}
		var result map[string]any
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("Error decoding map: %w", err)
		}
		if result["success"] != true {
			return nil, dataError("map_failed", fmt.Sprintf("The source could not be mapped: %v", result["error"]))
		}
		links := listField(result["links"])
		if useCache {
			siteMapCache.Lock()
			if len(siteMapCache.items) > 50 {
				siteMapCache.items = map[string]siteMapCacheValue{}
			}
			siteMapCache.items[cacheKey] = siteMapCacheValue{links: links, expires: time.Now().Add(3 * time.Minute)}
			siteMapCache.Unlock()
		}
		return links, nil
	}
	if !useCache {
		data, err := fetch()
		if err != nil {
			return nil, err
		}
		return data.([]any), nil
	}
	data, err, _ := siteMapFlight.Do(cacheKey, fetch)
	if err != nil {
		return nil, err
	}
	return data.([]any), nil
}

var knownSiteLocales = map[string]bool{
	"en": true, "fr": true, "es": true, "de": true, "ja": true, "zh": true, "zh-cn": true, "zh-tw": true,
	"pt": true, "pt-br": true, "it": true, "ko": true, "ru": true, "ar": true, "hi": true, "id": true,
	"nl": true, "pl": true, "tr": true, "vi": true, "th": true, "sv": true, "da": true,
	"es-es": true, "en-us": true, "en-gb": true, "fr-fr": true, "fr-ca": true,
}

func mapPageLocale(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "en"
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 {
		first := strings.ToLower(parts[0])
		if knownSiteLocales[first] {
			if strings.HasPrefix(first, "en") {
				return "en"
			}
			if strings.HasPrefix(first, "zh") {
				return "zh"
			}
			if strings.HasPrefix(first, "es") {
				return "es"
			}
			if strings.HasPrefix(first, "fr") {
				return "fr"
			}
			if strings.HasPrefix(first, "pt") {
				return "pt"
			}
			return first
		}
	}
	return "en"
}

func MapSitePage(input MapSiteInput) (map[string]any, error) {
	if input.Limit == 0 {
		input.Limit = 30
	}
	if input.Limit < 1 || input.Limit > 100 {
		return nil, dataError("invalid_limit", "map_site limit must be 1-100.")
	}
	if input.Offset < 0 || input.Offset > 100000 {
		return nil, dataError("invalid_offset", "map_site offset must be nonnegative.")
	}
	if input.Language == "" {
		input.Language = "en"
	}
	input.Language = strings.ToLower(strings.TrimSpace(input.Language))
	if input.PathPrefix != "" && !strings.HasPrefix(input.PathPrefix, "/") {
		input.PathPrefix = "/" + input.PathPrefix
	}
	if err := validateSourceURL(input.URL); err != nil {
		return nil, &DataError{Code: "invalid_url", Message: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	links, err := rawMappedLinks(ctx, input.URL)
	if err != nil {
		return nil, err
	}
	type located struct {
		entry map[string]any
		score int
		path  string
	}
	var matched []located
	seen := map[string]bool{}
	terms := queryTerms(input.Query)
	for _, v := range links {
		item := map[string]any{}
		if s, ok := v.(string); ok {
			item["url"] = s
		} else {
			item = mapField(v)
		}
		raw := stringField(item["url"])
		if raw == "" || seen[raw] {
			continue
		}
		seen[raw] = true
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" {
			continue
		}
		if input.Language != "all" && mapPageLocale(raw) != input.Language {
			continue
		}
		if input.PathPrefix != "" && !strings.HasPrefix(strings.ToLower(u.Path), strings.ToLower(input.PathPrefix)) {
			continue
		}
		name := stringField(item["title"])
		desc := stringField(item["description"])
		score := termScore(u.Path+" "+name, terms)*3 + termScore(desc, terms)
		if len(terms) > 0 && score == 0 {
			continue
		}
		clean := map[string]any{"url": raw}
		if name != "" {
			clean["title"] = boundedSiteText(name, 140)
		}
		if desc != "" {
			clean["description"] = boundedSiteText(desc, 230)
		}
		matched = append(matched, located{entry: clean, score: score, path: u.Path})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if len(terms) > 0 && matched[i].score != matched[j].score {
			return matched[i].score > matched[j].score
		}
		if depthI, depthJ := strings.Count(matched[i].path, "/"), strings.Count(matched[j].path, "/"); depthI != depthJ {
			return depthI < depthJ
		}
		return matched[i].path < matched[j].path
	})
	start := input.Offset
	if start > len(matched) {
		start = len(matched)
	}
	end := start + input.Limit
	if end > len(matched) {
		end = len(matched)
	}
	page := make([]map[string]any, 0, end-start)
	for _, entry := range matched[start:end] {
		page = append(page, entry.entry)
	}
	output := map[string]any{
		"url": input.URL, "total_discovered": len(links),
		"total_matching": len(matched), "links": page,
		"count": len(page), "offset": start, "limit": input.Limit,
		"language": input.Language, "provider_page_ceiling": 5000,
		"may_be_incomplete": len(links) >= 5000,
		"guidance":          "Use next to request another bounded page, or set query/path_prefix/language to narrow results. Language=all includes translated pages.",
	}
	if end < len(matched) {
		output["next"] = map[string]any{"tool": "map_site", "arguments": MapSiteInput{
			URL: input.URL, Query: input.Query, Limit: input.Limit, Offset: end,
			PathPrefix: input.PathPrefix, Language: input.Language,
		}}
	}
	return output, nil
}

func boundedSiteText(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > max {
		return value[:max] + "…"
	}
	return value
}
