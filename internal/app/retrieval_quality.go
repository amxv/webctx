package app

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"
)

// CAPTCHA pages can contain thousands of bytes of translated prompts. Page
// length is not evidence of useful content, so challenge detection must not
// be gated on the size of the extracted Markdown.
func challengeReason(markdown string) string {
	text := strings.ToLower(strings.TrimSpace(markdown))
	if text == "" {
		return "empty source"
	}
	prefix := text[:min(len(text), 4000)]
	for _, marker := range []string{
		"just a moment...",
		"checking your browser",
		"access denied | cloudflare",
		"attention required! | cloudflare",
		"enable javascript and cookies",
		"no content extracted",
		"please complete the security check",
		"verify you are human",
		"verifying you are human",
		"verify that you are human",
		"please verify you are a human",
		"we need to verify your browser",
		"challenge-platform",
	} {
		if strings.Contains(prefix, marker) {
			return "browser verification challenge"
		}
	}
	if strings.Contains(prefix, "hcaptcha") || strings.Contains(prefix, "h-captcha") {
		for _, secondary := range []string{
			"select all", "please verify", "i am human",
			"privacy - terms", "please complete", "security check",
			"captcha challenge", "verify your", "please solve",
		} {
			if strings.Contains(text, secondary) {
				return "hCaptcha challenge"
			}
		}
	}
	if (strings.Count(text, "captcha") > 2 || strings.Count(text, "verify you are human") > 0) &&
		(strings.Contains(text, "try again") || strings.Contains(text, "challenge") || strings.Contains(text, "i am human")) {
		return "CAPTCHA challenge"
	}
	return ""
}

func usableSourceContent(markdown string) bool {
	text := strings.TrimSpace(markdown)
	if len(text) < 10 || challengeReason(text) != "" {
		return false
	}
	// Guard against a 200 status containing an HTML block/error page.
	prefix := strings.ToLower(text[:min(len(text), 350)])
	return !strings.Contains(prefix, "<!doctype html") &&
		!strings.Contains(prefix, "<html")
}

// WEBCTX_DEBUG=1 enables stage traces. Only host + URL path are included;
// query parameters and credentials are intentionally not logged.
func traceRetrieval(rawURL, stage, outcome string, start time.Time, err error) {
	if os.Getenv("WEBCTX_DEBUG") != "1" {
		return
	}
	source := "invalid-url"
	if u, parseErr := url.Parse(rawURL); parseErr == nil && u.Hostname() != "" {
		source = u.Hostname() + u.EscapedPath()
	}
	detail := ""
	if err != nil {
		detail = fmt.Sprintf(" error=%q", sanitizedRetrievalError(err))
	}
	log.Printf("webctx retrieval source=%q stage=%q outcome=%q duration_ms=%d%s",
		source, stage, outcome, time.Since(start).Milliseconds(), detail)
}

func sanitizedRetrievalError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "403"), strings.Contains(message, "forbidden"):
		return "forbidden"
	case strings.Contains(message, "429"), strings.Contains(message, "rate limit"):
		return "rate-limited"
	case strings.Contains(message, "timeout"), strings.Contains(message, "deadline"):
		return "timeout"
	case strings.Contains(message, "captcha"), strings.Contains(message, "challenge"):
		return "challenge"
	case strings.Contains(message, "404"), strings.Contains(message, "not found"):
		return "not-found"
	default:
		return "source-unavailable"
	}
}
