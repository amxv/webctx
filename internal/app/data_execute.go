package app

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

func requiredFields(contract map[string]any) []string {
	var fields []string
	for _, v := range listField(contract["options"]) {
		f := mapField(v)
		if yes, _ := f["required"].(bool); yes {
			fields = append(fields, stringField(f["name"]))
		}
	}
	return fields
}

func validateDataInputs(contract map[string]any, inputs map[string]any) error {
	if inputs == nil {
		inputs = map[string]any{}
	}
	id := stringField(contract["id"])
	fields := map[string]map[string]any{}
	for _, v := range listField(contract["options"]) {
		f := mapField(v)
		if name := stringField(f["name"]); name != "" {
			fields[name] = f
		}
	}
	var issues []string
	for _, name := range requiredFields(contract) {
		if inputs[name] == nil || inputs[name] == "" {
			issues = append(issues, fmt.Sprintf("required field %s is missing", name))
		}
	}
	for _, group := range listField(contract["requiresOneOf"]) {
		found := false
		var alternatives []string
		for _, v := range listField(group) {
			field := stringField(v)
			if field == "" {
				continue
			}
			alternatives = append(alternatives, field)
			if inputs[field] != nil && inputs[field] != "" {
				found = true
			}
		}
		if !found && len(alternatives) > 0 {
			issues = append(issues, "provide at least one of: "+strings.Join(alternatives, ", "))
		}
	}
	for name, value := range inputs {
		field, ok := fields[name]
		if !ok {
			issues = append(issues, fmt.Sprintf("unknown field %q", name))
			continue
		}
		if value == nil {
			issues = append(issues, fmt.Sprintf("field %q must be filled in", name))
			continue
		}
		if !dataTypeMatches(value, stringField(field["type"])) {
			issues = append(issues, fmt.Sprintf("field %q must be %s", name, stringField(field["type"])))
			continue
		}
		if enum := listField(field["oneOf"]); len(enum) > 0 {
			found := false
			for _, allowed := range enum {
				a, _ := json.Marshal(allowed)
				b, _ := json.Marshal(value)
				if string(a) == string(b) {
					found = true
					break
				}
				// Some upstream contracts declare a numeric type but encode
				// its enum entries as strings. Accept the equivalent number.
				if actual, ok := numberField(value); ok {
					if candidate, ok := allowed.(string); ok {
						if parsed, err := strconv.ParseFloat(candidate, 64); err == nil && parsed == actual {
							found = true
							break
						}
					}
				}
			}
			if !found {
				issues = append(issues, fmt.Sprintf("field %q must be one of the values in its inspected oneOf list", name))
			}
		}
		if number, ok := numberField(value); ok {
			if lower, ok := numberField(field["min"]); ok && number < lower {
				issues = append(issues, fmt.Sprintf("field %q must be >= %v", name, lower))
			}
			if upper, ok := numberField(field["max"]); ok && number > upper {
				issues = append(issues, fmt.Sprintf("field %q must be <= %v", name, upper))
			}
		}
		if pattern := stringField(field["pattern"]); pattern != "" {
			if re, err := regexp.Compile(pattern); err == nil && re != nil {
				if s, ok := value.(string); ok && !re.MatchString(s) {
					issues = append(issues, fmt.Sprintf("field %q does not match its required pattern", name))
				}
			}
		}
	}
	if len(issues) > 0 {
		names := make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		sortStrings(names)
		return &DataError{
			Code:    "invalid_inputs",
			Message: "Input validation failed for " + id + ": " + strings.Join(issues, "; ") + ".",
			Details: map[string]any{"available_inputs": names, "next": map[string]any{"tool": "inspect", "arguments": map[string]any{"id": id}}},
		}
	}
	return nil
}

func dataTypeMatches(value any, kind string) bool {
	switch strings.ToLower(kind) {
	case "", "any", "json":
		return true
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := numberField(value)
		return ok
	case "integer", "int":
		f, ok := numberField(value)
		return ok && !math.IsInf(f, 0) && f == math.Trunc(f)
	case "boolean", "bool":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		return listField(value) != nil
	case "string[]":
		list := listField(value)
		if list == nil {
			return false
		}
		for _, v := range list {
			if _, ok := v.(string); !ok {
				return false
			}
		}
		return true
	case "number[]":
		list := listField(value)
		if list == nil {
			return false
		}
		for _, v := range list {
			if _, ok := numberField(v); !ok {
				return false
			}
		}
		return true
	default:
		return true // Other upstream-specific types are validated by Firecrawl.
	}
}

func expectedCallCredits(contract map[string]any, inputs map[string]any) (int, error) {
	cost, valid := numberField(contract["creditsCost"])
	if !valid || cost < 0 {
		return 0, dataError("price_unavailable", "The data source did not supply a usable credit price.")
	}
	if cost == 0 {
		return 0, nil
	}
	estimated := cost
	if perRecord, _ := contract["perRecord"].(bool); perRecord {
		// A per-record price has no hard upper bound without a record limit.
		count := 0.0
		for _, key := range []string{"limit", "per_page", "page_size", "max_results", "count"} {
			if n, ok := numberField(inputs[key]); ok && n >= 1 {
				count = n
				break
			}
		}
		if count == 0 {
			for _, v := range listField(contract["options"]) {
				f := mapField(v)
				for _, key := range []string{"limit", "per_page", "page_size", "max_results", "count"} {
					if stringField(f["name"]) == key {
						if n, ok := numberField(f["default"]); ok && n >= 1 {
							count = n
						}
					}
				}
			}
		}
		if count == 0 {
			return 0, dataError("unbounded_per_record_cost",
				"This operation charges per record but exposes no bounded record count. Inspect it and supply an explicit limit before execution.")
		}
		estimated = cost * count
	}
	if estimated > 1000000 {
		return 0, dataError("price_exceeds_limit", "Estimated cost is too large. Reduce the requested record limit.")
	}
	return int(math.Ceil(estimated)), nil
}

func continuationForDataResult(id string, originalInputs map[string]any, data any, maxCredits int) map[string]any {
	value := mapField(data)
	if value == nil {
		return nil
	}
	if next := mapField(value["next"]); next != nil {
		if provider := stringField(next["provider"]); provider != "" {
			if capability := stringField(next["capability"]); capability != "" {
				return map[string]any{"tool": "execute", "arguments": map[string]any{
					"calls":       []any{map[string]any{"id": provider + "/" + capability, "inputs": mapField(next["options"])}},
					"max_credits": maxCredits,
				}}
			}
		}
	}
	cursor := stringField(value["next_cursor"])
	if cursor == "" {
		cursor = stringField(value["nextCursor"])
	}
	if cursor == "" {
		cursor = stringField(mapField(value["pagination"])["next_cursor"])
	}
	if cursor != "" {
		next := copyInputs(originalInputs)
		if _, exists := next["page_token"]; exists {
			next["page_token"] = cursor
		} else {
			next["cursor"] = cursor
		}
		return map[string]any{"tool": "execute", "arguments": map[string]any{
			"calls":       []any{map[string]any{"id": id, "inputs": next}},
			"max_credits": maxCredits,
		}}
	}
	if page, ok := numberField(value["next_page"]); ok && page > 0 {
		next := copyInputs(originalInputs)
		next["page"] = int(page)
		return map[string]any{"tool": "execute", "arguments": map[string]any{
			"calls":       []any{map[string]any{"id": id, "inputs": next}},
			"max_credits": maxCredits,
		}}
	}
	if offset, ok := numberField(value["next_offset"]); ok && offset >= 0 {
		next := copyInputs(originalInputs)
		previous, _ := numberField(next["offset"])
		if offset > previous {
			next["offset"] = int(offset)
			return map[string]any{"tool": "execute", "arguments": map[string]any{
				"calls":       []any{map[string]any{"id": id, "inputs": next}},
				"max_credits": maxCredits,
			}}
		}
	}
	return nil
}

func copyInputs(original map[string]any) map[string]any {
	out := make(map[string]any, len(original))
	for k, v := range original {
		out[k] = v
	}
	return out
}

// ExecuteData validates contracts and credit estimates before calling any
// paid provider. The native API accepts at most ten executions per request.
func ExecuteData(input ExecuteDataInput) (map[string]any, error) {
	if len(input.Calls) == 0 || len(input.Calls) > 10 {
		return nil, dataError("invalid_calls", "Provide 1-10 operation calls with id and inputs.")
	}
	if !paidDataEnabled() {
		return nil, dataError("paid_data_disabled", "Paid data queries are disabled by WEBCTX_ALEXANDRIA_PAID=off.")
	}
	cap := dataCreditCap()
	if input.MaxCredits > 0 && input.MaxCredits < cap {
		cap = input.MaxCredits
	}
	if input.MaxCredits < 0 || input.MaxCredits > dataCreditCap() {
		return nil, dataError("invalid_budget", fmt.Sprintf("max_credits must be between 1 and the configured maximum of %d.", dataCreditCap()))
	}
	if input.RequestID != "" && !safeRequestID.MatchString(input.RequestID) {
		return nil, dataError("invalid_request_id", "request_id must have 8-128 letters, digits, hyphens or underscores.")
	}
	ctx, cancel := defaultDataContext()
	defer cancel()
	type reviewed struct {
		Contract map[string]any
		Err      error
	}
	checked := make([]reviewed, len(input.Calls))
	var wg sync.WaitGroup
	for i, call := range input.Calls {
		if _, _, err := parseOperationID(call.ID); err != nil {
			return nil, err
		}
		wg.Add(1)
		go func(i int, call DataCall) {
			defer wg.Done()
			contract, _, err := exactDataContract(ctx, call.ID, []string{"options", "response", "examples"})
			checked[i] = reviewed{Contract: contract, Err: err}
		}(i, call)
	}
	wg.Wait()
	var estimated int
	var calls []map[string]any
	for i, call := range input.Calls {
		if checked[i].Err != nil {
			return nil, checked[i].Err
		}
		contract := checked[i].Contract
		if err := validateDataInputs(contract, call.Inputs); err != nil {
			return nil, err
		}
		cost, err := expectedCallCredits(contract, call.Inputs)
		if err != nil {
			return nil, err
		}
		estimated += cost
		if estimated > cap {
			return nil, &DataError{Code: "credit_budget_exceeded",
				Message: fmt.Sprintf("Preflight estimate %d credits exceeds the %d-credit request cap. Reduce calls/record limits, or configure WEBCTX_ALEXANDRIA_MAX_CREDITS.", estimated, cap),
				Details: map[string]any{"estimated_credits": estimated, "max_credits": cap},
			}
		}
		provider, capability, _ := parseOperationID(call.ID)
		options := call.Inputs
		if options == nil {
			options = map[string]any{}
		}
		calls = append(calls, map[string]any{"provider": provider, "capability": capability, "options": options})
	}
	requestID := input.RequestID
	if requestID == "" {
		var err error
		requestID, err = newRequestID()
		if err != nil {
			return nil, dataError("internal_error", "Could not generate a request identifier.")
		}
	}
	response, err := doDataAPI(ctx, "/scrape", map[string]any{"alexandria": calls}, requestID)
	if err != nil {
		var typed *DataError
		if errorsAsData(err, &typed) {
			typed.Details = map[string]any{"request_id": requestID,
				"retry": map[string]any{"tool": "execute", "arguments": ExecuteDataInput{
					Calls: input.Calls, MaxCredits: cap, RequestID: requestID, Raw: input.Raw,
				}},
			}
		}
		return nil, err
	}
	payload := mapField(response["data"])
	nativeResults := listField(payload["alexandria"])
	if len(nativeResults) == 0 {
		return nil, dataError("invalid_upstream_response", "Paid request returned no Alexandria execution results.")
	}
	results := make([]map[string]any, 0, len(nativeResults))
	for i, value := range nativeResults {
		entry := mapField(value)
		if entry == nil {
			continue
		}
		id := stringField(entry["provider"]) + "/" + stringField(entry["capability"])
		item := map[string]any{
			"id":           id,
			"source_id":    entry["provider"],
			"data":         entry["data"],
			"credits_used": entry["creditsCost"],
		}
		if token := entry["alexandriaId"]; token != nil {
			item["data_id"] = token
		}
		if errValue := entry["error"]; errValue != nil {
			item["error"] = errValue
		}
		if i < len(input.Calls) {
			if next := continuationForDataResult(id, input.Calls[i].Inputs, entry["data"], cap); next != nil {
				item["next"] = next
			}
		}
		results = append(results, item)
	}
	output := map[string]any{
		"results":           results,
		"request_id":        requestID,
		"estimated_credits": estimated,
		"pricing_note":      "max_credits is a preflight estimate; actual per-record charges are determined by the upstream provider and the account's Firecrawl limits.",
		"guidance":          "Use next to fetch additional pages where available. Reuse request_id when retrying the same paid request, but generate a new ID for the next page.",
	}
	if cost := payload["creditsCost"]; cost != nil {
		output["credits_used"] = cost
	} else if cost := response["creditsUsed"]; cost != nil {
		output["credits_used"] = cost
	}
	if input.Raw {
		output["raw"] = response
	}
	return output, nil
}

func errorsAsData(err error, into **DataError) bool {
	e, ok := err.(*DataError)
	if ok {
		*into = e
	}
	return ok
}
