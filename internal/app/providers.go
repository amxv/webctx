package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// These are the configurable search providers. Firecrawl powers independent
// read-link, map-site, and Alexandria operations, not federated web search.
var searchProviderKeys = []struct {
	name string
	key  string
}{
	{"brave", "BRAVE_API_KEY"},
	{"tavily", "TAVILY_API_KEY"},
	{"exa", "EXA_API_KEY"},
}

type providerConfig struct {
	DisabledProviders []string `json:"disabled_providers,omitempty"`
}

func providerConfigPath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("WEBCTX_CONFIG_FILE")); custom != "" {
		return custom, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate WebCTX config directory: %w", err)
	}
	return filepath.Join(dir, "webctx", "config.json"), nil
}

func readProviderConfig(path string) (providerConfig, error) {
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return providerConfig{}, nil // all providers default to enabled
	}
	if err != nil {
		return providerConfig{}, fmt.Errorf("read provider settings %s: %w", path, err)
	}
	var config providerConfig
	if err := json.Unmarshal(contents, &config); err != nil {
		return providerConfig{}, fmt.Errorf("invalid provider settings %s: %w", path, err)
	}
	return config, nil
}

func currentProviderConfig() (providerConfig, error) {
	path, err := providerConfigPath()
	if err != nil {
		return providerConfig{}, err
	}
	return readProviderConfig(path)
}

func (c providerConfig) enabled(name string) bool {
	return !slices.Contains(c.DisabledProviders, name)
}

func validSearchProvider(name string) bool {
	for _, p := range searchProviderKeys {
		if p.name == name {
			return true
		}
	}
	return false
}

func setSearchProviderEnabled(name string, enabled bool) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !validSearchProvider(name) {
		return "", fmt.Errorf("unknown search provider %q (choose brave, tavily, or exa)", name)
	}
	path, err := providerConfigPath()
	if err != nil {
		return "", err
	}
	config, err := readProviderConfig(path)
	if err != nil {
		return "", err
	}
	if config.enabled(name) == enabled {
		return path, nil
	}
	if enabled {
		config.DisabledProviders = slices.DeleteFunc(config.DisabledProviders, func(p string) bool { return p == name })
	} else {
		config.DisabledProviders = append(config.DisabledProviders, name)
	}
	slices.Sort(config.DisabledProviders)
	if err := writeProviderConfig(path, config); err != nil {
		return "", err
	}
	return path, nil
}

func writeProviderConfig(path string, config providerConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create provider config directory: %w", err)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".webctx-config-*")
	if err != nil {
		return fmt.Errorf("write provider settings: %w", err)
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("save provider settings: %w", err)
	}
	return nil
}

func printProviderStatus(out io.Writer) error {
	path, err := providerConfigPath()
	if err != nil {
		return err
	}
	config, err := readProviderConfig(path)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Search providers (settings: %s)\n", path)
	for _, p := range searchProviderKeys {
		state, key := "enabled", "key missing"
		if !config.enabled(p.name) {
			state = "disabled"
		}
		count := len(searchKeysForProvider(p.name))
		if count > 0 {
			key = "key configured"
			if count > 1 {
				key = fmt.Sprintf("key configured (%d keys)", count)
			}
		}
		_, _ = fmt.Fprintf(out, "  %-7s %-9s %s\n", p.name, state, key)
	}
	_, _ = fmt.Fprintln(out, "\nFirecrawl (page reading, site maps, and Alexandria; separate from search):")
	if strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY")) != "" {
		_, _ = fmt.Fprintln(out, "  key configured")
	} else {
		_, _ = fmt.Fprintln(out, "  key missing")
	}
	_, _ = fmt.Fprintln(out, "\nKey presence is local configuration only; quotas and API availability are not checked.")
	return nil
}
