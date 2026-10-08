package app

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Reject non-public URL destinations before calling native readers or
// hosted scraping providers. Credentials embedded in URLs are never accepted.
func validateSourceURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("readable URL must be an absolute HTTP(S) URL without credentials")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("private/internal host is not a readable public URL")
	}
	if ip := net.ParseIP(host); ip != nil && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback()) {
		return fmt.Errorf("private or reserved IP address is not a readable public URL")
	}
	return nil
}
