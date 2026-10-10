package app

import (
	"strings"
)

var integerDataInputs = map[string]bool{
	"page": true, "page_number": true, "pageNumber": true,
	"page_size": true, "per_page": true, "offset": true,
	"limit": true, "k": true, "top_k": true, "topK": true,
	"count": true, "size": true, "max_results": true,
	"num_results": true, "max_records": true, "results_per_page": true,
}

// Return normalized, machine-readable hints alongside the untouched native
// field contract. Do not overwrite the upstream schema's declared types.
func normalizedDataInputFields(contract map[string]any) []map[string]any {
	fields := make([]map[string]any, 0)
	for _, v := range listField(contract["options"]) {
		item := mapField(v)
		if item == nil {
			continue
		}
		name := stringField(item["name"])
		if name == "" {
			continue
		}
		normalized := map[string]any{"name": name, "type": item["type"]}
		typ := strings.ToLower(stringField(item["type"]))
		description := strings.ToLower(stringField(item["about"]))
		if typ == "number" && (integerDataInputs[name] || strings.Contains(description, "integer")) {
			normalized["type"] = "integer"
			normalized["original_type"] = item["type"]
		}
		for _, key := range []string{"pattern", "required", "min", "max", "default", "oneOf", "nullable", "requires", "dependsOn", "mutuallyExclusiveWith"} {
			if value, ok := item[key]; ok {
				normalized[key] = value
			}
		}
		fields = append(fields, normalized)
	}
	return fields
}

func normalizedDataOutputFields(contract map[string]any) []map[string]any {
	response := mapField(contract["response"])
	fields := make([]map[string]any, 0)
	for _, v := range listField(response["fields"]) {
		item := mapField(v)
		if item == nil {
			continue
		}
		normalized := map[string]any{"name": item["name"], "type": item["type"]}
		about := strings.ToLower(stringField(item["about"]))
		nullable, _ := item["nullable"].(bool)
		if strings.Contains(about, "null") || strings.Contains(about, "missing value") {
			nullable = true
		}
		if nullable {
			normalized["nullable"] = true
		}
		fields = append(fields, normalized)
	}
	return fields
}
