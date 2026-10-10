package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMalformedAlexandriaJSONBecomesCopyable(t *testing.T) {
	source := "# REST example\n" + backtickFence + "bash\n" +
		"curl https://api.firecrawl.dev/v2/search \\\n" +
		" -H 'Content-Type: application/json' \\\n" +
		" -d '{\n  \"query\": \"podcast conversations\",\n  \"sources\": [\\\n    \"web\",\\\n    \"alexandria\"\\\n  ],\n  \"limit\": 2\n}'\n" +
		backtickFence
	got := copyableCurlExamples(source)
	if !strings.Contains(got, "Copy-ready correction") {
		t.Fatalf("lost truthful correction label: %s", got)
	}
	start := strings.Index(got, "-d '{")
	if start < 0 {
		t.Fatalf("lost source cURL: %s", got)
	}
	bodyStart := start + len("-d '")
	bodyEnd := bodyStart + strings.LastIndex(got[bodyStart:], "}'") + 1
	if !json.Valid([]byte(got[bodyStart:bodyEnd])) {
		t.Fatalf("JSON request still uncopyable: %s", got[bodyStart:bodyEnd])
	}
	if !strings.Contains(got, " -H 'Content-Type: application/json' \\") {
		t.Fatalf("source shell continuations should be preserved: %s", got)
	}
	if strings.Contains(got, "\"sources\": [\\") {
		t.Fatalf("malformed JSON continuation not repaired: %s", got)
	}
	if copyableCurlExamples(got) != got {
		t.Fatalf("correction should be idempotent")
	}
}

func TestCorrectSourceExamplesAreNeverRewritten(t *testing.T) {
	source := backtickFence + "bash\ncurl https://api.example.com/v2/search -d '{\"sources\":[\"alexandria\"]}'\n" + backtickFence
	if got := copyableCurlExamples(source); got != source {
		t.Fatalf("valid published snippet was modified: %s", got)
	}
}

func TestRESTContractsLeadAndExcerptsStayConcise(t *testing.T) {
	if focusedPrimaryBudget <= 5300 {
		t.Fatal("REST excerpt budget should be narrower than normal focused context")
	}
	source := "# Official API docs\n" + strings.Repeat("Large unrelated introduction about navigation.\n", 700) +
		"\n## REST API endpoint example\n" +
		backtickFence + "bash\ncurl https://api.example.com/v2/search -d '{\"sources\":[\"alexandria\"]}'\n" + backtickFence
	got := focusExcerpt(source, "What are the REST API endpoints and exact request examples?", 5300)
	if len(got) > 6000 || !strings.Contains(got, "/v2/search") {
		t.Fatalf("REST excerpt did not prioritize complete example under budget: len=%d", len(got))
	}
}
