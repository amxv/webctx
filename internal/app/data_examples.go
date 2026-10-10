package app

import (
	"regexp"
	"strings"
)

var backtickedIdentifier = regexp.MustCompile("`([A-Za-z][A-Za-z0-9_/-]{1,30})`")

// Upstream examples can legitimately be absent. Offer a clearly marked,
// locally validated sample input without claiming it was published by the
// provider or that it was executed.
func contractExamples(contract map[string]any) any {
	if published := contract["examples"]; published != nil {
		if len(listField(published)) > 0 {
			return published
		}
		if len(mapField(published)) > 0 {
			return published
		}
	}
	input, needed := nativeInputTemplate(contract)
	fieldInfo := map[string]map[string]any{}
	for _, v := range listField(contract["options"]) {
		if field := mapField(v); field != nil {
			fieldInfo[stringField(field["name"])] = field
		}
	}
	for _, name := range needed {
		field := fieldInfo[name]
		if value, ok := sampleFieldValue(name, field); ok {
			input[name] = value
		}
	}
	// An illustrative example should not accidentally fetch huge datasets
	// when copied verbatim. Users may adjust the inputs freely.
	for _, name := range []string{"limit", "per_page", "page_size", "max_results", "count"} {
		if n, ok := numberField(input[name]); ok && n > 5 {
			input[name] = 5
		}
	}
	if _, _, err := parseOperationID(stringField(contract["id"])); err != nil {
		return nil
	}
	validation := validateDataInputs(contract, input)
	example := map[string]any{
		"kind":              "generated_from_inspected_schema",
		"note":              "Illustrative Origo-generated inputs; not a published provider example or an executed query.",
		"inputs":            input,
		"valid_input_shape": validation == nil,
	}
	if validation != nil {
		example["note"] = "Illustrative input template; fill required fields and check source-specific constraints before execution."
		example["needs_input"] = needed
	} else {
		example["next"] = map[string]any{"tool": "execute", "arguments": map[string]any{
			"calls": []any{map[string]any{"id": contract["id"], "inputs": input}},
		}}
	}
	return []any{example}
}

func sampleFieldValue(name string, field map[string]any) (any, bool) {
	if field == nil {
		return nil, false
	}
	if values := listField(field["oneOf"]); len(values) > 0 {
		for _, v := range values {
			if v != nil && v != "" {
				return v, true
			}
		}
	}
	if sample, ok := field["example"]; ok && sample != nil {
		return sample, true
	}
	if samples := listField(field["examples"]); len(samples) > 0 && samples[0] != nil {
		return samples[0], true
	}
	if v, ok := field["default"]; ok && v != nil {
		return v, true
	}
	info := stringField(field["about"]) + " " + stringField(field["description"])
	lower := strings.ToLower(name + " " + info)
	switch {
	case strings.Contains(name, "series_id") && strings.Contains(info, "CPIAUCSL"):
		return "CPIAUCSL", true
	case strings.Contains(name, "series_id") && strings.Contains(info, "UNRATE"):
		return "UNRATE", true
	case strings.Contains(name, "series_id") && strings.Contains(info, "GDP"):
		return "GDP", true
	case strings.Contains(lower, "semantic_search"):
		return "AI agents", true
	case name == "query" || name == "search" || name == "keyword_search":
		return "example topic", true
	case name == "url" || strings.HasSuffix(name, "_url"):
		return "https://example.com", true
	}
	// Backticked descriptions sometimes contain sample codes or IDs.
	if strings.Contains(lower, "id") || strings.Contains(lower, "code") {
		for _, match := range backtickedIdentifier.FindAllStringSubmatch(info, 10) {
			candidate := match[1]
			if strings.Contains(candidate, "/") || strings.Contains(candidate, "_") {
				continue
			}
			if len(candidate) >= 3 && !strings.EqualFold(candidate, "required") {
				return candidate, true
			}
		}
	}
	return nil, false
}
