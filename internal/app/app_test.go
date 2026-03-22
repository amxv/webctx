package app

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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

func TestSearchMissingCredentialsErrorIsHelpful(t *testing.T) {
	t.Setenv("BRAVE_API_KEY", "")
	t.Setenv("TAVILY_API_KEY", "")
	t.Setenv("EXA_API_KEY", "")

	text, err := Search(SearchParams{Query: "openai api"})
	if err == nil {
		t.Fatalf("expected error, got text %q", text)
	}
	if !strings.Contains(err.Error(), "missing BRAVE_API_KEY, EXA_API_KEY, TAVILY_API_KEY") {
		t.Fatalf("unexpected error: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "macOS Keychain") {
		t.Fatalf("expected keychain guidance, got %q", err.Error())
	}
}

func TestMapSiteMissingCredentialErrorIsHelpful(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "")

	text, err := MapSite("https://example.com")
	if err == nil {
		t.Fatalf("expected error, got text %q", text)
	}
	if !strings.Contains(err.Error(), "missing FIRECRAWL_API_KEY") {
		t.Fatalf("unexpected error: %q", err.Error())
	}
	if !strings.Contains(err.Error(), ".env.local next to the binary") {
		t.Fatalf("expected .env.local guidance, got %q", err.Error())
	}
}

func TestLoadDotEnvFileDoesNotOverrideExistingEnv(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env.local")
	if err := os.WriteFile(envPath, []byte("BRAVE_API_KEY=from-file\nEXA_API_KEY=from-file\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("BRAVE_API_KEY", "from-env")
	t.Setenv("EXA_API_KEY", "")

	loadDotEnvFile(envPath)

	if got := os.Getenv("BRAVE_API_KEY"); got != "from-env" {
		t.Fatalf("expected existing env to win, got %q", got)
	}
	if got := os.Getenv("EXA_API_KEY"); got != "" {
		t.Fatalf("expected explicitly empty env to remain untouched, got %q", got)
	}
}

func TestEnvLocalCandidatesPreferExecutableDir(t *testing.T) {
	originalGetwd := getwdFunc
	originalExecutable := executableFunc
	t.Cleanup(func() {
		getwdFunc = originalGetwd
		executableFunc = originalExecutable
	})

	getwdFunc = func() (string, error) { return "/tmp/project", nil }
	executableFunc = func() (string, error) { return "/opt/webctx/bin/webctx", nil }

	got := envLocalCandidates()
	want := []string{
		filepath.Join("/opt/webctx/bin", ".env.local"),
		filepath.Join("/opt/webctx", ".env.local"),
		filepath.Join("/tmp/project", ".env.local"),
	}
	if runtime.GOOS == "windows" {
		for i := range want {
			want[i] = filepath.Clean(want[i])
		}
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected candidate count: got %v want %v", got, want)
	}
	for i := range want {
		if filepath.Clean(got[i]) != filepath.Clean(want[i]) {
			t.Fatalf("candidate %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestLoadKeychainEnvLoadsMissingOnly(t *testing.T) {
	originalLookup := keychainLookup
	t.Cleanup(func() { keychainLookup = originalLookup })

	keychainLookup = func(key string) (string, error) {
		return "from-keychain-" + key, nil
	}

	t.Setenv("BRAVE_API_KEY", "already-set")
	t.Setenv("TAVILY_API_KEY", "")
	t.Setenv("EXA_API_KEY", "")
	t.Setenv("FIRECRAWL_API_KEY", "")

	loadKeychainEnv()

	if got := os.Getenv("BRAVE_API_KEY"); got != "already-set" {
		t.Fatalf("expected existing env to remain, got %q", got)
	}
	if got := os.Getenv("TAVILY_API_KEY"); got != "" {
		t.Fatalf("expected explicitly empty env to remain untouched, got %q", got)
	}
}
