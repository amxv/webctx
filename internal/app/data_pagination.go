package app

import (
	"net/url"
	"strconv"
	"strings"
)

// providerContinuation uses native cursor and page metadata without
// reconstructing provider URLs. All previously supplied filters remain
// unchanged; only the relevant paging parameter is added/replaced.
func providerContinuation(id string, original map[string]any, data any) map[string]any {
	value := mapField(data)
	if value == nil {
		return nil
	}
	makeNext := func(key string, token any) map[string]any {
		if key == "" || token == nil || token == "" {
			return nil
		}
		next := copyInputs(original)
		if previous, exists := next[key]; exists && previous == token {
			return nil
		}
		next[key] = token
		return map[string]any{"tool": "execute", "arguments": map[string]any{
			"calls": []any{map[string]any{"id": id, "inputs": next}},
		}}
	}
	for _, candidate := range []struct {
		location map[string]any
		names    []string
		input    string
	}{
		{value, []string{"next_page_token", "nextPageToken", "page_token_next"}, "page_token"},
		{mapField(value["pagination"]), []string{"next_page_token", "nextPageToken"}, "page_token"},
		{value, []string{"continuation", "next_continuation"}, "continuation"},
		{mapField(value["metadata"]), []string{"search_after"}, "search_after"},
		{mapField(value["meta"]), []string{"search_after"}, "search_after"},
		{value, []string{"search_after"}, "search_after"},
		{mapField(value["pagination"]), []string{"next_cursor", "nextCursor"}, "cursor"},
		{mapField(value["pageInfo"]), []string{"endCursor"}, "cursor"},
	} {
		for _, name := range candidate.names {
			token := candidate.location[name]
			if token == nil || token == "" {
				continue
			}
			key := candidate.input
			if key == "page_token" {
				for _, alias := range []string{"page_token", "pageToken", "pageTokenNext"} {
					if _, exists := original[alias]; exists {
						key = alias
						break
					}
				}
			}
			if key == "cursor" {
				for _, alias := range []string{"cursor", "after", "after_cursor"} {
					if _, exists := original[alias]; exists {
						key = alias
						break
					}
				}
			}
			if next := makeNext(key, token); next != nil {
				return next
			}
		}
	}
	// Treasury and other API-backed providers expose the following page as
	// a URL. We extract only recognized pagination parameters; we never
	// fetch or execute that URL directly.
	links := mapField(value["links"])
	for _, source := range []any{links["next"], mapField(value["paging"])["next"]} {
		target := stringField(source)
		if target == "" {
			continue
		}
		u, err := url.Parse(target)
		if err != nil {
			continue
		}
		q := u.Query()
		for _, candidate := range []struct{ From, To string }{
			{"page[number]", "page"}, {"pageNumber", "page"}, {"page", "page"},
			{"page_number", "page_number"}, {"page[size]", "page_size"},
			{"pageToken", "page_token"}, {"page_token", "page_token"},
			{"offset", "offset"}, {"cursor", "cursor"},
		} {
			raw := q.Get(candidate.From)
			if raw == "" {
				continue
			}
			key := candidate.To
			if candidate.From == "page[number]" {
				for _, alias := range []string{"page_number", "page", "pageNumber"} {
					if _, exists := original[alias]; exists {
						key = alias
						break
					}
				}
			}
			token := any(raw)
			if n, e := strconv.Atoi(raw); e == nil {
				token = n
			}
			if next := makeNext(key, token); next != nil {
				return next
			}
		}
	}
	// Some providers only return has_more and a current page number.
	if more, _ := value["has_more"].(bool); more {
		for _, name := range []string{"page", "page_number", "pageNumber"} {
			if n, ok := numberField(original[name]); ok && n >= 0 {
				return makeNext(name, int(n)+1)
			}
		}
	}
	if link := stringField(value["next"]); link != "" &&
		!strings.HasPrefix(strings.ToLower(link), "http") {
		return makeNext("cursor", link)
	}
	return nil
}
