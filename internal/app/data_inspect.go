package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var dataSourceIDPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$")
var dataCapabilityPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_./-]{0,249}$")

func parseOperationID(id string) (string, string, error) {
	provider, capability, ok := strings.Cut(strings.TrimSpace(id), "/")
	if !ok || !dataSourceIDPattern.MatchString(provider) ||
		!dataCapabilityPattern.MatchString(capability) || strings.Contains(capability, "..") ||
		strings.Contains(capability, "//") {
		return "", "", dataError("invalid_operation", "Use an exact operation ID from research, such as particle/podcasts/episodes/search.")
	}
	return provider, capability, nil
}

func exactDataContract(ctx context.Context, id string, expanded []string) (map[string]any, map[string]any, error) {
	provider, capability, err := parseOperationID(id)
	if err != nil {
		return nil, nil, err
	}
	opts := map[string]any{
		"providers":    []string{provider},
		"capabilities": []string{capability},
		"level":        "tools", "expand": expanded, "limit": 10,
	}
	catalogue, raw, err := freeDataBrowse(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range listField(catalogue["items"]) {
		item := mapField(entry)
		if item == nil {
			continue
		}
		actual := stringField(item["id"])
		if actual == "" {
			actual = stringField(item["provider"]) + "/" + stringField(item["capability"])
		}
		if actual == id {
			return item, raw, nil
		}
	}
	return nil, nil, &DataError{
		Code:    "operation_not_found",
		Message: fmt.Sprintf("Operation %q was not found. Use research to discover currently available sources and operations.", id),
	}
}

func nativeInputTemplate(contract map[string]any) (map[string]any, []string) {
	template := map[string]any{}
	var need []string
	seen := map[string]bool{}
	for _, field := range listField(contract["options"]) {
		option := mapField(field)
		name := stringField(option["name"])
		if name == "" {
			continue
		}
		if value, exists := option["default"]; exists {
			template[name] = value
		}
		if required, _ := option["required"].(bool); required {
			template[name] = nil
			need = append(need, name)
			seen[name] = true
		}
	}
	return template, need
}

func InspectData(input InspectDataInput) (map[string]any, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return nil, dataError("missing_id", "Provide an operation or source ID returned by research.")
	}
	expanded := []string{"options", "response", "examples"}
	if len(input.Include) > 0 {
		var err error
		expanded, err = expandContract(input.Include)
		if err != nil {
			return nil, err
		}
		// A useful execute template always requires the real input contract,
		// even if the caller only asked to expand examples or output details.
		if !containsString(expanded, "options") {
			expanded = append(expanded, "options")
		}
	}
	if !strings.Contains(id, "/") {
		if !dataSourceIDPattern.MatchString(id) {
			return nil, dataError("invalid_source", "Use an exact source ID returned by research.")
		}
		include := input.Include
		// Provider-level discovery is intentionally lightweight: inspect a
		// particular operation for full field-by-field contracts.
		list, err := ResearchData(ResearchDataInput{Sources: []string{id}, View: "operations", Include: include, Limit: 12, Raw: input.Raw})
		if err != nil {
			return nil, err
		}
		list["source_id"] = id
		list["guidance"] = "Choose an operation ID and call inspect again for its exact contract and an execute template."
		return list, nil
	}
	ctx, cancel := defaultDataContext()
	defer cancel()
	item, raw, err := exactDataContract(ctx, id, expanded)
	if err != nil {
		return nil, err
	}
	provider, _, _ := parseOperationID(id)
	inputs, missing := nativeInputTemplate(item)
	execArguments := map[string]any{
		"calls": []any{map[string]any{"id": id, "inputs": inputs}},
	}
	response := map[string]any{
		"id":           id,
		"name":         item["name"],
		"source_id":    provider,
		"description":  item["description"],
		"cost_credits": 0,
		"price": map[string]any{
			"credits_per_call": item["creditsCost"],
			"per_record":       item["perRecord"],
		},
		"inputs":                   map[string]any{"fields": item["options"], "normalized_fields": normalizedDataInputFields(item), "requires_one_of": item["requiresOneOf"]},
		"output":                   item["response"],
		"normalized_output_fields": normalizedDataOutputFields(item),
		"examples":                 contractExamples(item),
		"input_template":           inputs,
		"fill_one_of":              item["requiresOneOf"],
		"next":                     map[string]any{"tool": "execute", "arguments": execArguments},
		"guidance":                 "Copy next.arguments into execute, fill any blank required inputs and adjust values according to inputs.fields. Execute uses credits.",
	}
	if cost, ok := numberField(item["creditsCost"]); ok && cost > float64(dataCreditCap()) {
		response["availability"] = "exceeds_server_credit_limit"
		response["executable"] = false
		response["server_credit_limit"] = dataCreditCap()
		delete(response, "next")
		response["guidance"] = fmt.Sprintf("This operation costs %v credits per call and exceeds Origo's %d-credit request ceiling. It cannot execute under this server configuration.", cost, dataCreditCap())
	} else {
		response["executable"] = true
	}
	if len(missing) > 0 {
		response["fill_before_execution"] = missing
	}
	if input.Raw {
		response["raw"] = raw
	}
	return response, nil
}
