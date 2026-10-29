package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type MarkdownResult struct {
	URL       string
	SourceURL string
	Title     string
	Markdown  string
}

const keychainServiceName = "webctx"

var (
	credentialEnvKeys = []string{"BRAVE_API_KEY", "BRAVE_API_KEYS", "TAVILY_API_KEY", "TAVILY_API_KEYS", "EXA_API_KEY", "EXA_API_KEYS", "FIRECRAWL_API_KEY", "FIRECRAWL_API_KEYS", "GH_TOKEN", "GITHUB_TOKEN"}
	getwdFunc         = os.Getwd
	executableFunc    = os.Executable
	keychainLookup    = lookupKeychainSecret
)

func firstHeadingOrFallback(markdown, fallback string) string {
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	if strings.TrimSpace(fallback) == "" {
		return "Document"
	}
	return fallback
}

func loadEnvLocal() {
	for _, candidate := range envLocalCandidates() {
		loadDotEnvFile(candidate)
	}
	loadKeychainEnv()
}

func envLocalCandidates() []string {
	candidates := []string{}
	if exe, err := executableFunc(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, ".env.local"), filepath.Join(filepath.Dir(exeDir), ".env.local"))
	}
	if cwd, err := getwdFunc(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".env.local"))
	}
	seen := map[string]struct{}{}
	unique := []string{}
	for _, c := range candidates {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		unique = append(unique, c)
	}
	return unique
}

func loadDotEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "export ")
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" {
			if _, exists := os.LookupEnv(key); exists {
				continue
			}
			_ = os.Setenv(key, value)
		}
	}
}

func loadKeychainEnv() {
	for _, key := range credentialEnvKeys {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value, err := keychainLookup(key)
		if err != nil || strings.TrimSpace(value) == "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func lookupKeychainSecret(account string) (string, error) {
	if runtime.GOOS != "darwin" || strings.TrimSpace(account) == "" {
		return "", nil
	}

	out, err := exec.Command("security", "find-generic-password", "-s", keychainServiceName, "-a", account, "-w").Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
