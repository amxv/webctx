package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
		if integerDataInputs[name] {
			if n, ok := numberField(value); ok && n != math.Trunc(n) {
				issues = append(issues, fmt.Sprintf("field %q requires an integer value", name))
			}
		}
		for _, key := range []string{"requires", "dependsOn"} {
			if deps := listField(field[key]); len(deps) > 0 {
				for _, dep := range deps {
					dependent := stringField(dep)
					if dependent != "" && inputs[dependent] == nil {
						issues = append(issues, fmt.Sprintf("field %q requires accompanying field %q", name, dependent))
					}
				}
			}
		}
		for _, other := range listField(field["mutuallyExclusiveWith"]) {
			if alternate := stringField(other); alternate != "" && inputs[alternate] != nil {
				issues = append(issues, fmt.Sprintf("fields %q and %q cannot be combined", name, alternate))
			}
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
		for _, key := range []string{"limit", "k", "top_k", "topK", "per_page", "page_size", "max_results", "count", "size", "num_results", "max_records", "results_per_page"} {
			if n, ok := numberField(inputs[key]); ok && n >= 1 {
				count = n
				break
			}
		}
		// For bulk identifier lookups, a response may be charged per input.
		for _, key := range []string{"ids", "domains", "urls", "companies", "people", "symbols"} {
			if records := listField(inputs[key]); count == 0 && len(records) > 0 {
				count = float64(len(records))
			}
		}
		if count == 0 {
			for _, v := range listField(contract["options"]) {
				f := mapField(v)
				for _, key := range []string{"limit", "k", "top_k", "topK", "per_page", "page_size", "max_results", "count", "size", "num_results", "max_records", "results_per_page"} {
					if stringField(f["name"]) == key {
						if n, ok := numberField(f["default"]); ok && n >= 1 {
							count = n
						}
					}
				}
			}
		}
		if count == 0 {
			// An exact, non-paginated lookup with an individual identifier has
			// a single-record upper bound, even without a limit parameter.
			// Only apply this to known singleton operations, never searches.
			id := strings.ToLower(stringField(contract["id"]))
			isLookup := strings.HasSuffix(id, "/lookup") || strings.HasSuffix(id, "/get") ||
				strings.HasSuffix(id, "/detail") || strings.HasSuffix(id, "/details") ||
				strings.HasSuffix(id, "/resolve") || strings.HasSuffix(id, "/profile")
			hasIdentifier := false
			for _, key := range []string{"domain", "id", "company_id", "person_id", "symbol", "url", "linkedin_url", "repo", "doi"} {
				if s := stringField(inputs[key]); s != "" {
					hasIdentifier = true
					break
				}
			}
			if isLookup && hasIdentifier {
				if paginated, ok := mapField(contract["response"])["paginated"].(bool); !ok || !paginated {
					count = 1
				}
			}
		}
		if count == 0 {
			return 0, dataError("unbounded_per_record_cost",
				"This operation charges per record, but the contract does not establish a finite record bound. Inspect its output and supply a supported count parameter or smaller input set. Origo cannot safely invent a limit field.")
		}
		estimated = cost * count
	}
	if estimated > 1000000 {
		return 0, dataError("price_exceeds_limit", "Estimated cost is too large. Reduce the requested record limit.")
	}
	return int(math.Ceil(estimated)), nil
}

func continuationForDataResult(id string, originalInputs map[string]any, data any) map[string]any {
	value := mapField(data)
	if value == nil {
		return nil
	}
	if next := providerContinuation(id, originalInputs, data); next != nil {
		return next
	}
	if next := mapField(value["next"]); next != nil {
		if provider := stringField(next["provider"]); provider != "" {
			if capability := stringField(next["capability"]); capability != "" {
				return map[string]any{"tool": "execute", "arguments": map[string]any{
					"calls": []any{map[string]any{"id": provider + "/" + capability, "inputs": mapField(next["options"])}},
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
			"calls": []any{map[string]any{"id": id, "inputs": next}},
		}}
	}
	if page, ok := numberField(value["next_page"]); ok && page > 0 {
		next := copyInputs(originalInputs)
		next["page"] = int(page)
		return map[string]any{"tool": "execute", "arguments": map[string]any{
			"calls": []any{map[string]any{"id": id, "inputs": next}},
		}}
	}
	if offset, ok := numberField(value["next_offset"]); ok && offset >= 0 {
		next := copyInputs(originalInputs)
		previous, _ := numberField(next["offset"])
		if offset > previous {
			next["offset"] = int(offset)
			return map[string]any{"tool": "execute", "arguments": map[string]any{
				"calls": []any{map[string]any{"id": id, "inputs": next}},
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

// A multi-operation request uses a distinct, deterministic provider request ID
// for every operation. The upstream batch endpoint is not transactionally
// isolated: one provider's terms rejection can abort unrelated operations.
func operationRequestID(root string, index int, id string, count int) string {
	if count == 1 {
		return root
	}
	digest := sha256.Sum256([]byte(id))
	if len(root) > 92 {
		root = root[:92]
	}
	return fmt.Sprintf("%s-%d-%s", root, index, hex.EncodeToString(digest[:6]))
}

func executeOneDataOperation(ctx context.Context, call DataCall, requestID string, index int, includeRaw bool) (map[string]any, float64, error) {
	provider, capability, err := parseOperationID(call.ID)
	if err != nil {
		return nil, 0, err
	}
	options := call.Inputs
	if options == nil {
		options = map[string]any{}
	}
	raw, err := doDataAPI(ctx, "/scrape", map[string]any{"alexandria": []any{
		map[string]any{"provider": provider, "capability": capability, "options": options},
	}}, requestID)
	if err != nil {
		return nil, 0, err
	}
	payload := mapField(raw["data"])
	results := listField(payload["alexandria"])
	if len(results) != 1 {
		return nil, 0, dataError("invalid_upstream_response", "The provider did not return exactly one result for the requested operation.")
	}
	entry := mapField(results[0])
	if entry == nil {
		return nil, 0, dataError("invalid_upstream_response", "The provider returned an unreadable result.")
	}
	if providerError := entry["error"]; providerError != nil {
		return nil, 0, &DataError{Code: "provider_operation_failed", Message: fmt.Sprint(providerError)}
	}
	credits, _ := numberField(entry["creditsCost"])
	if total, ok := numberField(payload["creditsCost"]); ok {
		credits = total
	}
	item := map[string]any{
		"operation_index": index, "id": call.ID, "source_id": provider,
		"ok": true, "data": entry["data"],
		"credits_used": credits, "receipt_scope": "upstream_request",
		"provider_request_id":  requestID,
		"provider_credit_note": "Provider-native fields named credits may use different units. credits_used is Firecrawl Alexandria billing.",
	}
	if token := entry["alexandriaId"]; token != nil {
		item["data_id"] = token
		item["receipt_scope"] = "provider_response_or_batch"
	}
	if note := dataMeasurementNote(entry["data"]); note != "" {
		item["measurement_note"] = note
	}
	if next := continuationForDataResult(call.ID, call.Inputs, entry["data"]); next != nil {
		item["next"] = next
	}
	if extra := raw["scrape_id"]; extra != nil {
		item["scrape_id"] = extra
	}
	if includeRaw {
		item["raw"] = raw
	}
	return item, credits, nil
}

func operationErrorResult(call DataCall, index int, requestID string, err error) map[string]any {
	typed, ok := err.(*DataError)
	if !ok {
		typed = &DataError{Code: "execution_failed", Message: err.Error()}
	}
	out := map[string]any{
		"id": call.ID, "operation_index": index, "ok": false,
		"provider_request_id": requestID, "error": typed,
		"credits_used": nil, "billing_status": "unknown_if_provider_accepted_request",
	}
	if typed.Code == "THIRD_PARTY_DATA_TERMS_REQUIRED" {
		out["billing_status"] = "blocked_pending_human_terms"
		out["credits_used"] = 0
	}
	return out
}

// ExecuteData validates contracts and credit estimates before calling any
// paid provider. The native API accepts at most ten executions per request.
func ExecuteData(input ExecuteDataInput) (map[string]any, error) {
	if len(input.Calls) == 0 || len(input.Calls) > 10 {
		return nil, dataError("invalid_calls", "Provide 1-10 operation calls with id and inputs.")
	}
	cap := dataCreditCap()
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
			return nil, &DataError{Code: "server_credit_limit_exceeded",
				Message: fmt.Sprintf("This request is estimated at %d credits, above Origo's %d-credit server limit. Reduce the number of operations or requested records.", estimated, cap),
				Details: map[string]any{"estimated_credits": estimated, "server_credit_limit": cap},
			}
		}
	}
	requestID := input.RequestID
	if requestID == "" {
		var err error
		requestID, err = newRequestID()
		if err != nil {
			return nil, dataError("internal_error", "Could not generate a request identifier.")
		}
	}
	if input.RequestID != "" {
		if cached, err := cachedPaidReplay(requestID, input.Calls); err != nil {
			return nil, err
		} else if cached != nil {
			return cached, nil
		}
	}
	results := make([]map[string]any, 0, len(input.Calls))
	creditsUsed := 0.0
	failed := 0
	var rawResponses []any
	for i, call := range input.Calls {
		operationID := operationRequestID(requestID, i, call.ID, len(input.Calls))
		item, charged, err := executeOneDataOperation(ctx, call, operationID, i, input.Raw)
		if err != nil {
			// Treat idempotency collisions and contract/terms failures as
			// independent operation errors. Do not suggest replaying a
			// conflicting payload with the same id.
			var typed *DataError
			if errorsAsData(err, &typed) {
				if typed.Code != "duplicate_request" && typed.Code != "THIRD_PARTY_DATA_TERMS_REQUIRED" {
					typed.Details = map[string]any{
						"request_id": operationID,
						"recovery":   "For an uncertain network failure, retry the exact same operation and request_id. For a different query or page, omit request_id.",
					}
				}
			}
			if len(input.Calls) == 1 {
				return nil, err
			}
			failed++
			results = append(results, operationErrorResult(call, i, operationID, err))
			continue
		}
		creditsUsed += charged
		results = append(results, item)
		if input.Raw {
			rawResponses = append(rawResponses, map[string]any{"operation_index": i, "provider_request_id": operationID, "result": item})
		}
	}
	output := map[string]any{
		"results": results, "partial": failed > 0, "failed_operations": failed,
		"request_id":                   requestID,
		"estimated_credits":            estimated,
		"credits_used":                 creditsUsed,
		"pricing_note":                 "credits_used totals reported Firecrawl charges from successful operations. Failed operations may have indeterminate billing; provider-native credits use their own units.",
		"guidance":                     "Use each result.next to fetch its next page. Retry an identical operation only with its provider_request_id. A new query/page must use a new request ID.",
		"replay_status":                "upstream_replay_unverified",
		"replayed":                     false,
		"credits_charged_this_request": nil,
		"original_credits_used":        nil,
		"billing_note":                 "Firecrawl idempotent replay may return the original credit amount; without a provider replay flag or durable accounting data, Origo cannot assert that a retry incurred a new charge.",
	}
	if failed > 0 {
		output["status"] = "partial_success"
	}
	if failed == len(input.Calls) {
		output["status"] = "all_failed"
	}
	if failed == 0 {
		output["status"] = "success"
	}
	if input.Raw {
		output["raw"] = rawResponses
	}
	if failed == 0 {
		storePaidReplay(requestID, input.Calls, output)
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

// Interpret only well-defined provider unit codes, and distinguish CPI index
// levels from annual inflation rates without transforming source values.
func dataMeasurementNote(raw any) string {
	data := mapField(raw)
	if data == nil {
		return ""
	}
	series := strings.ToUpper(stringField(data["series_id"]))
	if !strings.HasPrefix(series, "CPI") {
		return ""
	}
	switch strings.ToLower(stringField(data["units"])) {
	case "lin", "":
		return "CPI observations are price-index levels, not annual inflation percentages. Do not label them as inflation rates; a year-over-year percentage requires a 12-month comparison or an appropriate FRED transformation."
	case "pc1":
		return "FRED units=pc1 represents the percent change from one year ago, not an index level."
	default:
		return "CPI observations use the requested FRED units transformation; inspect the units field before interpreting values as percentages."
	}
}
