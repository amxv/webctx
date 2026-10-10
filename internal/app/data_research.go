package app

import (
	"net/url"
	"sort"
	"strings"
)

func researchView(view, query string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(view)) {
	case "":
		if strings.TrimSpace(query) == "" {
			return "providers", nil
		}
		return "tools", nil
	case "sources", "providers":
		return "providers", nil
	case "groups":
		return "groups", nil
	case "operations", "tools":
		return "tools", nil
	default:
		return "", dataError("invalid_view", "Use view=sources, groups, or operations.")
	}
}

func expandContract(include []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(include))
	for _, value := range include {
		name := ""
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "inputs", "options":
			name = "options"
		case "output", "response":
			name = "response"
		case "examples":
			name = "examples"
		default:
			return nil, dataError("invalid_include", "Supported include values: inputs, output, examples.")
		}
		if !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	return out, nil
}

func normalizeResearchInput(input ResearchDataInput) (ResearchDataInput, string, []string, error) {
	switch input.Mode {
	case "", "ranked", "catalogue":
	default:
		return input, "", nil, dataError("invalid_mode", "Use mode=ranked or mode=catalogue.")
	}
	view, err := researchView(input.View, input.Query)
	if err != nil {
		return input, "", nil, err
	}
	expand, err := expandContract(input.Include)
	if err != nil {
		return input, "", nil, err
	}
	if input.Limit == 0 && !input.limitProvided {
		input.Limit = 10
	}
	if input.Limit < 1 || input.Limit > 100 {
		return input, "", nil, dataError("invalid_limit", "limit must be between 1 and 100.")
	}
	if input.Offset < 0 || input.Offset > 100000 {
		return input, "", nil, dataError("invalid_offset", "offset must be between 0 and 100000.")
	}
	for _, target := range input.URLs {
		if err := validateSourceURL(target); err != nil {
			return input, "", nil, dataError("invalid_url", "Website filters require public HTTP(S) URLs.")
		}
	}
	if len(input.URLs) > 0 && len(input.Sources) == 0 {
		for _, target := range input.URLs {
			u, _ := url.Parse(target)
			host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
			if host != "" {
				source := strings.ReplaceAll(host, ".", "-")
				input.Sources = append(input.Sources, source)
			}
		}
	}
	return input, view, expand, nil
}

func nativeBrowseOptions(input ResearchDataInput, view string, expand []string) map[string]any {
	o := map[string]any{"level": view, "limit": input.Limit, "offset": input.Offset}
	if input.Query != "" {
		o["query"] = input.Query
	}
	if len(input.URLs) > 0 && len(input.Sources) == 0 {
		o["urls"] = input.URLs
	}
	if len(input.Sources) > 0 {
		o["providers"] = input.Sources
	}
	if len(input.Categories) > 0 {
		o["categories"] = input.Categories
	}
	if len(input.Groups) > 0 {
		o["groups"] = input.Groups
	}
	if len(input.Operations) > 0 {
		var caps []string
		sourceSet := map[string]bool{}
		for _, source := range input.Sources {
			sourceSet[source] = true
		}
		for _, id := range input.Operations {
			provider, capability, err := parseOperationID(id)
			if err == nil {
				caps = append(caps, capability)
				sourceSet[provider] = true
			} else {
				caps = append(caps, id)
			}
		}
		o["capabilities"] = caps
		if len(input.Sources) == 0 {
			var providers []string
			for provider := range sourceSet {
				providers = append(providers, provider)
			}
			// Sorting keeps responses, cursors and test fixtures stable.
			sortStrings(providers)
			if len(providers) > 0 {
				o["providers"] = providers
			}
		}
	}
	if len(expand) > 0 {
		if containsString(expand, "examples") && !containsString(expand, "options") {
			expand = append(append([]string(nil), expand...), "options")
		}
		o["expand"] = expand
	}
	return o
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// ResearchData supports ranked semantic search and the complete filterable,
// pageable catalogue. Each result includes the exact next tool/arguments.
func ResearchData(input ResearchDataInput) (map[string]any, error) {
	input, view, expand, err := normalizeResearchInput(input)
	if err != nil {
		return nil, err
	}
	ctx, cancel := defaultDataContext()
	defer cancel()
	explicitExact := len(input.Operations) > 0
	// An exact operation ID is authoritative. A descriptive query helps
	// communicate intent, but must not exclude a known operation.
	note := ""
	if explicitExact && input.Query != "" {
		note = "Exact operation IDs take precedence over descriptive query terms; the query was not used as a restrictive upstream filter."
		input.Query = ""
	}
	semantic := input.Mode != "catalogue" && input.Query != "" && view == "tools" && input.Offset == 0 &&
		len(input.URLs) == 0 && len(input.Sources) == 0 && len(input.Categories) == 0 &&
		len(input.Groups) == 0 && len(input.Operations) == 0
	localFilteredRanking := view == "tools" && input.Query != "" &&
		(len(input.URLs) > 0 || len(input.Sources) > 0 || len(input.Groups) > 0 || len(input.Categories) > 0)
	var items []any
	var total any
	var next any
	var raw map[string]any
	mode := "catalogue"
	if semantic {
		mode = "ranked_search"
		// Ask for full metadata even when the agent requested concise output:
		// compact upstream matches may omit both price and operation name.
		// We still return compact result cards unless include was requested.
		detail := "full"
		fetchLimit := input.Limit * 4
		if fetchLimit < 20 {
			fetchLimit = 20
		}
		if fetchLimit > 100 {
			fetchLimit = 100
		}
		raw, err = doDataAPI(ctx, "/search", map[string]any{
			"query": input.Query, "sources": []string{"alexandria"},
			"limit": fetchLimit, "toolDetail": detail,
		}, "")
		if err != nil {
			return nil, err
		}
		data := mapField(raw["data"])
		items = listField(data["tools"])
		// Some upstream ranked matches arrive out of order despite including a
		// similarity score. Present the highest-scoring candidate first, retaining
		// upstream order for exact ties or missing scores.
		// Ranked search has no native cursor. Expose the free, complete
		// catalogue with matching semantic query as an optional expansion.
		next = map[string]any{"tool": "research", "arguments": map[string]any{
			"query": input.Query, "view": "operations", "mode": "catalogue", "limit": input.Limit,
		}, "note": "Browse all matching operations with pagination; ranked discovery itself has no cursor."}
	} else {
		options := nativeBrowseOptions(input, view, expand)
		if localFilteredRanking {
			// Provider-specified lexical filters can exclude operations whose
			// descriptions describe the requested data indirectly. Fetch a
			// catalogue slice from the actual source, then rank it ourselves.
			delete(options, "query")
			options["limit"] = 100
			options["offset"] = 0
			note = "The source/category filters select candidates first. The question ranks those candidates locally, rather than excluding matching source operations."
			if input.Mode != "catalogue" {
				mode = "ranked_filtered"
			}
		}
		if explicitExact {
			delete(options, "query")
			delete(options, "urls")
		}
		var catalogue map[string]any
		catalogue, raw, err = freeDataBrowse(ctx, options)
		if err != nil {
			return nil, err
		}
		items = listField(catalogue["items"])
		total = catalogue["total"]
		if native := mapField(catalogue["next"]); native != nil {
			next = continuationFromCatalogue(native)
		}
	}
	if view == "tools" {
		termsQuery := input.Query
		for _, op := range input.Operations {
			termsQuery += " " + op
		}
		if strings.TrimSpace(termsQuery) != "" {
			sort.SliceStable(items, func(i, j int) bool {
				return intentRelevance(mapField(items[i]), termsQuery) > intentRelevance(mapField(items[j]), termsQuery)
			})
		} else {
			sort.SliceStable(items, func(i, j int) bool {
				a, aok := numberField(mapField(items[i])["similarity"])
				b, bok := numberField(mapField(items[j])["similarity"])
				if aok != bok {
					return aok
				}
				return a > b
			})
		}
	}
	if semantic && len(items) > input.Limit {
		items = items[:input.Limit]
	}
	if localFilteredRanking {
		count := len(items)
		start := input.Offset
		if start > count {
			start = count
		}
		end := start + input.Limit
		if end > count {
			end = count
		}
		items = items[start:end]
		if count > end {
			next = map[string]any{"tool": "research", "arguments": ResearchDataInput{
				Query: input.Query, Mode: input.Mode, URLs: input.URLs, Sources: input.Sources,
				Categories: input.Categories, Groups: input.Groups, View: niceDataView(view),
				Include: input.Include, Limit: input.Limit, Offset: end,
			}}
		}
	}
	results := make([]map[string]any, 0, len(items))
	for _, value := range items {
		entry := mapField(value)
		if entry == nil {
			continue
		}
		id := stringField(entry["id"])
		if id == "" && view == "tools" {
			if provider := stringField(entry["provider"]); provider != "" {
				id = provider + "/" + stringField(entry["capability"])
			}
		}
		name := stringField(entry["name"])
		if name == "" {
			segments := strings.Split(id, "/")
			name = strings.ReplaceAll(strings.ReplaceAll(segments[len(segments)-1], "_", " "), "-", " ")
		}
		result := map[string]any{"id": id, "name": name, "description": entry["description"]}
		if provider := stringField(entry["provider"]); provider != "" {
			result["source_id"] = provider
		}
		if cost, ok := entry["creditsCost"]; ok {
			result["credits_per_call"] = cost
			if listed, ok := numberField(cost); ok {
				if listed > float64(dataCreditCap()) {
					result["executable"] = false
					result["availability"] = "exceeds_server_credit_limit"
					result["server_credit_limit"] = dataCreditCap()
				} else if view == "tools" {
					result["executable"] = true
				}
			}
		}
		if cost, ok := entry["perRecord"]; ok {
			result["per_record"] = cost
		}
		if count, ok := entry["toolCount"]; ok {
			result["operation_count"] = count
		}
		if input.Query != "" && view == "tools" {
			result["relevance"] = intentRelevance(entry, input.Query)
		}
		if rank, ok := entry["similarity"]; ok {
			result["semantic_similarity"] = rank
			if input.Query == "" {
				result["relevance"] = rank
			}
		}
		if len(expand) > 0 {
			if selected := entry["options"]; selected != nil && containsString(expand, "options") {
				result["inputs"] = selected
			}
			if selected := entry["response"]; selected != nil && containsString(expand, "response") {
				result["output"] = selected
			}
			if selected := entry["examples"]; selected != nil && containsString(expand, "examples") {
				result["examples"] = selected
			}
			if containsString(expand, "examples") && result["examples"] == nil {
				if entry["options"] != nil {
					result["examples"] = contractExamples(entry)
				}
			}
		}
		if view == "tools" && id != "" {
			result["next"] = map[string]any{"tool": "inspect", "arguments": map[string]any{"id": id}}
		} else if continuation := mapField(entry["next"]); continuation != nil {
			result["next"] = continuationFromCatalogue(continuation)
		} else if id != "" {
			result["next"] = map[string]any{"tool": "inspect", "arguments": map[string]any{"id": id}}
		}
		results = append(results, result)
	}
	output := map[string]any{
		"mode": mode, "view": niceDataView(view), "results": results,
		"count": len(results), "cost_credits": 0,
		"guidance": "Use inspect with an operation ID to see complete inputs, output, examples and the next execute call. To explore a source, inspect its source ID.",
	}
	if note != "" {
		output["mode_note"] = note
	}
	if len(input.URLs) > 0 && len(input.Sources) > 0 {
		output["resolved_url_sources"] = input.Sources
	}
	if total != nil {
		output["total"] = total
	}
	if next != nil {
		output["next"] = next
	}
	if input.Raw {
		output["raw"] = raw
	}
	return output, nil
}

// Rank operations by their ability to deliver the requested data, not just
// semantic similarity. Keep the provider similarity separately for auditing.
func intentRelevance(entry map[string]any, query string) float64 {
	if entry == nil {
		return 0
	}
	base, _ := numberField(entry["similarity"])
	if query == "" {
		return base
	}
	q := strings.ToLower(query)
	id := strings.ToLower(stringField(entry["id"]))
	name := strings.ToLower(stringField(entry["name"]))
	desc := strings.ToLower(stringField(entry["description"]))
	p := id + " " + name
	searchIntent := strings.Contains(q, "search") || strings.Contains(q, "find") ||
		strings.Contains(q, "papers") || strings.Contains(q, "podcast") ||
		strings.Contains(q, "trials") || strings.Contains(q, "about")
	historyIntent := strings.Contains(q, "historical") || strings.Contains(q, "history") ||
		strings.Contains(q, "time series") || strings.Contains(q, "over time") ||
		strings.Contains(q, "inflation")
	if historyIntent && (strings.Contains(p, "observations") ||
		strings.Contains(p, "history") || strings.Contains(p, "rates") ||
		strings.Contains(p, "timeseries")) {
		base += 0.23
	}
	if searchIntent && (strings.Contains(p, "/search") || strings.Contains(p, "/studies") ||
		strings.Contains(p, "episode-search")) {
		base += 0.20
	}
	if searchIntent && strings.Contains(p, "/list") && !strings.Contains(q, "list") {
		base -= 0.04
	}
	for _, ancillary := range []string{"suggest", "related", "directory", "releases", "/source", "metadata", "browse"} {
		if strings.Contains(p, ancillary) && !strings.Contains(q, strings.TrimLeft(ancillary, "/")) {
			base -= 0.14
		}
	}
	terms := queryTerms(query)
	if len(terms) > 0 {
		fit := termScore(name+" "+desc, terms)
		if fit > 8 {
			fit = 8
		}
		base += float64(fit) * 0.007
	}
	return float64(int(base*1000000+0.5)) / 1000000
}

func niceDataView(view string) string {
	switch view {
	case "providers":
		return "sources"
	case "tools":
		return "operations"
	default:
		return view
	}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func continuationFromCatalogue(native map[string]any) map[string]any {
	options := mapField(native["options"])
	if options == nil {
		return map[string]any{"native_request": native}
	}
	args := map[string]any{"mode": "catalogue"}
	for _, field := range []string{"query", "urls", "providers", "categories", "groups", "limit", "offset"} {
		if value, ok := options[field]; ok {
			name := field
			if field == "providers" {
				name = "sources"
			}
			args[name] = value
		}
	}
	if level := stringField(options["level"]); level != "" {
		args["view"] = niceDataView(level)
	}
	if caps := listField(options["capabilities"]); len(caps) > 0 {
		var ids []string
		sourceNames := listField(options["providers"])
		for _, capability := range caps {
			cap := stringField(capability)
			if cap == "" {
				continue
			}
			if len(sourceNames) == 1 {
				ids = append(ids, stringField(sourceNames[0])+"/"+cap)
			} else {
				ids = append(ids, cap)
			}
		}
		args["operations"] = ids
	}
	if exp := listField(options["expand"]); len(exp) > 0 {
		var include []string
		for _, value := range exp {
			switch stringField(value) {
			case "options":
				include = append(include, "inputs")
			case "response":
				include = append(include, "output")
			case "examples":
				include = append(include, "examples")
			}
		}
		args["include"] = include
	}
	return map[string]any{"tool": "research", "arguments": args, "native_request": native}
}
