package app

import (
	"encoding/json"
	"regexp"
	"strings"
)

var trailingPayloadSlash = regexp.MustCompile("(?m)\\\\[ \\t]*(\\r?\\n)")

// Some source scrapers leave JavaScript-style line-continuation slashes
// inside shell-quoted JSON. Those slashes make the published cURL request
// uncopyable. Repair only payloads that are initially invalid JSON and become
// valid JSON after removing those continuation markers. Label every repair.
// The original source URL remains in the surrounding cited context.
func copyableCurlExamples(markdown string) string {
	parts := strings.Split(markdown, backtickFence)
	if len(parts) < 3 {
		return markdown
	}
	for i := 1; i < len(parts); i += 2 {
		code := parts[i]
		if !strings.Contains(code, "curl ") {
			continue
		}
		start := strings.Index(code, "-d '{")
		if start < 0 {
			continue
		}
		bodyStart := start + len("-d '")
		endRel := strings.LastIndex(code[bodyStart:], "}'")
		if endRel < 0 {
			continue
		}
		bodyEnd := bodyStart + endRel + 1
		original := code[bodyStart:bodyEnd]
		if json.Valid([]byte(original)) {
			continue
		}
		fixed := trailingPayloadSlash.ReplaceAllString(original, "$1")
		if fixed == original || !json.Valid([]byte(fixed)) {
			continue
		}
		parts[i] = code[:bodyStart] + fixed + code[bodyEnd:]
		parts[i-1] += "\n\n*[Copy-ready correction: removed stray line-continuation backslashes inside the published JSON payload; see the cited URL for the original.]*\n"
	}
	return strings.Join(parts, backtickFence)
}
