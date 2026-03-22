package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRootHelp(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer

	code := Run([]string{"--help"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("Run returned code %d", code)
	}
	if !strings.Contains(out.String(), "webctx v") || !strings.Contains(out.String(), "read-link") {
		t.Fatalf("unexpected help output: %q", out.String())
	}
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer

	code := Run([]string{"--version"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("Run returned code %d", code)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatalf("unexpected empty version output")
	}
	if strings.Contains(out.String(), "webctx ") {
		t.Fatalf("expected bare version output, got: %q", out.String())
	}
}

func TestRunSearchWithoutQuery(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer

	code := Run([]string{"search"}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "search requires a query") {
		t.Fatalf("unexpected stderr: %q", errBuf.String())
	}
}

func TestNormalizeURL(t *testing.T) {
	got := normalizeURL("https://Example.com/docs/?utm_source=x&ref=y&q=go")
	want := "https://example.com/docs?q=go"
	if got != want {
		t.Fatalf("normalizeURL mismatch: got %q want %q", got, want)
	}
}

func TestScoreAndRankResultsDuplicateBoost(t *testing.T) {
	results, duplicatesRemoved := scoreAndRankResults([]providerDocs{
		{Provider: "Brave", Docs: []SearchDoc{{URL: "https://a.com", Title: "A"}, {URL: "https://b.com", Title: "B"}}},
		{Provider: "Tavily", Docs: []SearchDoc{{URL: "https://b.com", Title: "B2"}, {URL: "https://c.com", Title: "C"}}},
	})
	if duplicatesRemoved != 1 {
		t.Fatalf("expected 1 duplicate removed, got %d", duplicatesRemoved)
	}
	if len(results) == 0 || results[0].URL != "https://b.com" {
		t.Fatalf("expected duplicate URL to rank first, got %#v", results)
	}
}

func TestParseGitHubURL(t *testing.T) {
	info := parseGitHubURL("https://github.com/amxv/webctx-ts/blob/main/cli.ts")
	if info == nil || !info.IsFile || info.Owner != "amxv" || info.Repo != "webctx-ts" || info.Branch != "main" || info.Path != "cli.ts" {
		t.Fatalf("unexpected parse result: %#v", info)
	}
}
