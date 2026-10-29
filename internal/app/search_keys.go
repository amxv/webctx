package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// A provider's legacy singular key remains supported. Additional keys are
// checked first so newly provisioned keys take precedence over exhausted ones.
// Plural values can be comma/newline-separated or a JSON string array (the
// latter is how the macOS Keychain stores them).
func searchKeysForProvider(name string) []string {
	name = strings.ToLower(strings.TrimSpace(name))
	if !validSearchProvider(name) {
		return nil
	}
	prefix := strings.ToUpper(name) + "_API_KEY"
	keys := parseSearchKeyList(os.Getenv(prefix + "S"))
	if key := strings.TrimSpace(os.Getenv(prefix)); key != "" {
		keys = append(keys, key)
	}
	return uniqueSearchKeys(keys)
}

func parseSearchKeyList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var keys []string
		if json.Unmarshal([]byte(raw), &keys) != nil {
			return nil // fail closed rather than sending malformed credential data
		}
		return uniqueSearchKeys(keys)
	}
	return uniqueSearchKeys(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}))
}

func uniqueSearchKeys(keys []string) []string {
	seen := make(map[string]bool, len(keys))
	unique := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key != "" && !seen[key] {
			seen[key] = true
			unique = append(unique, key)
		}
	}
	return unique
}

// Retrying with another key is appropriate for auth, quota, rate-limit, and
// server/transport failures. Request validation errors or malformed successful
// responses won't improve with another credential and should not be retried.
func retryWithNextSearchKey(err error) bool {
	var status *httpStatusError
	if errors.As(err, &status) {
		code := status.Code
		// Brave Search rejects invalid subscription tokens with HTTP 422,
		// rather than 401. Only retry that exact provider error code: a
		// different 422 can indicate a bad query that no key can fix.
		if code == 422 && strings.Contains(status.Message, "SUBSCRIPTION_TOKEN_INVALID") {
			return true
		}
		return code == 401 || code == 402 || code == 403 || code == 408 || code == 429 || code == 432 || code >= 500
	}
	var transport *url.Error
	return errors.As(err, &transport)
}

func redactSearchError(err error, keys []string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, key := range keys {
		if key != "" {
			message = strings.ReplaceAll(message, key, "[REDACTED]")
		}
	}
	return errors.New(message)
}

func useSearchKeys(ctx context.Context, name string, attempt func(string) ([]byte, error)) ([]byte, error) {
	keys := searchKeysForProvider(name)
	if len(keys) == 0 {
		return nil, fmt.Errorf("missing %s_API_KEY", strings.ToUpper(name))
	}
	var lastErr error
	for index, key := range keys {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		body, err := attempt(key)
		if err == nil {
			return body, nil
		}
		lastErr = redactSearchError(err, keys)
		if index+1 == len(keys) || !retryWithNextSearchKey(err) {
			break
		}
	}
	if len(keys) == 1 {
		return nil, lastErr
	}
	return nil, fmt.Errorf("%s: search failed after trying available API keys (up to %d): %w", name, len(keys), lastErr)
}

var keychainStore = storeKeychainSecret

// Additional keys are stored in one macOS Keychain entry per provider, without
// exposing secrets in argv, config.json, logs, or CLI output. The security CLI
// supports a prompt from stdin when -w is the final argument. It prompts for
// the password twice, even when updating an entry, so feed matching lines.
func storeKeychainSecret(account, secret string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("key management requires macOS Keychain; on other platforms set PROVIDER_API_KEYS in the environment")
	}
	cmd := exec.Command("security", "add-generic-password", "-U", "-s", keychainServiceName, "-a", account, "-w")
	cmd.Stdin = strings.NewReader(secret + "\n" + secret + "\n")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not save API keys to macOS Keychain: %w", err)
	}
	return nil
}

func managedKeychainAccount(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !validSearchProvider(name) {
		return "", fmt.Errorf("unknown search provider %q (choose brave, tavily, or exa)", name)
	}
	return strings.ToUpper(name) + "_API_KEYS", nil
}

func readManagedKeys(name string) ([]string, error) {
	account, err := managedKeychainAccount(name)
	if err != nil {
		return nil, err
	}
	raw, err := keychainLookup(account)
	if err != nil {
		return nil, fmt.Errorf("could not read API keys from macOS Keychain: %w", err)
	}
	if raw == "" {
		return nil, nil
	}
	var keys []string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return nil, fmt.Errorf("the stored %s key list is not valid JSON; no keys were changed", name)
	}
	return uniqueSearchKeys(keys), nil
}

func writeManagedKeys(name string, keys []string) error {
	account, err := managedKeychainAccount(name)
	if err != nil {
		return err
	}
	serialized, err := json.Marshal(uniqueSearchKeys(keys))
	if err != nil {
		return err
	}
	return keychainStore(account, string(serialized))
}

func addManagedSearchKey(name string, from io.Reader) (int, error) {
	if _, err := managedKeychainAccount(name); err != nil {
		return 0, err
	}
	input, err := io.ReadAll(io.LimitReader(from, 4097))
	if err != nil {
		return 0, fmt.Errorf("read API key from stdin: %w", err)
	}
	key := strings.TrimSpace(string(input))
	if key == "" || len(input) > 4096 || strings.ContainsAny(key, "\r\n\t ") {
		return 0, errors.New("API key must be a single nonempty token no longer than 4096 bytes")
	}
	keys, err := readManagedKeys(name)
	if err != nil {
		return 0, err
	}
	if slices.Contains(keys, key) {
		return len(keys), nil
	}
	// Newest keys take priority, so an old exhausted legacy key is never
	// attempted before a recently added working key.
	keys = append([]string{key}, keys...)
	if err := writeManagedKeys(name, keys); err != nil {
		return 0, err
	}
	return len(keys), nil
}

func removeManagedSearchKey(name, rawIndex string) (int, error) {
	keys, err := readManagedKeys(name)
	if err != nil {
		return 0, err
	}
	index, err := strconv.Atoi(rawIndex)
	if err != nil || index < 1 || index > len(keys) {
		return 0, fmt.Errorf("key index must be between 1 and %d (managed Keychain keys only)", len(keys))
	}
	keys = slices.Delete(keys, index-1, index)
	if err := writeManagedKeys(name, keys); err != nil {
		return 0, err
	}
	return len(keys), nil
}

func printManagedSearchKeySlots(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "\nManaged macOS Keychain keys (numbered newest-first; indexes start at 1):")
	for _, provider := range searchProviderKeys {
		keys, err := readManagedKeys(provider.name)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "  %-7s %d managed\n", provider.name, len(keys))
	}
	return nil
}
