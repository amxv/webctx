package app

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
	"time"
)

func TestLongCAPTCHAPayloadCannotPassAsDocumentation(t *testing.T) {
	longChallenge := "hCaptcha - Please verify you are human. " +
		strings.Repeat("Deutsch Français 日本語 中文 Hindi Español English ", 1000)
	if usableScrapedMarkdown(longChallenge) || usableNativeMarkdown(longChallenge) {
		t.Fatal("large CAPTCHA payload must be rejected even when >2KB")
	}
	if challengeReason(longChallenge) != "browser verification challenge" {
		t.Fatalf("missing browser challenge reason: %q", challengeReason(longChallenge))
	}
}

func TestNormalDocumentationAboutCAPTCHAsIsNotAutomaticallyBlocked(t *testing.T) {
	doc := "# hCaptcha integration guide\n\nUse the hCaptcha SDK to add a verification widget to your website.\n" +
		strings.Repeat("Install the client library and validate tokens on your server.\n", 50)
	if !usableScrapedMarkdown(doc) {
		t.Fatal("ordinary documentation about captcha should not be rejected")
	}
}

func TestExplicitEvidenceGapReportsMissingFields(t *testing.T) {
	q := "Create a Checkout Session with line_items, mode, success_url and cancel_url and explain webhook signature verification"
	excerpts := "Use line_items and success_url in a session. Verify webhooks using Stripe-Signature."
	gaps := missingResearchEvidence(q, excerpts)
	if !strings.Contains(strings.Join(gaps, ","), "cancel_url") {
		t.Fatalf("missing required request field was not reported: %+v", gaps)
	}
}

func TestEmbeddedJSONExampleDoesNotBypassExcerptBudget(t *testing.T) {
	doc := "**URL:** https://example.com/guide\n\n# Guide\n" +
		strings.Repeat("This guide describes a public API and its fields.\n", 1000) +
		"\n" + backtickFence + "json\n" + `{"example":{"id":"ok"}}` + "\n" + backtickFence
	out := focusExcerpt(doc, "public API fields", 3500)
	if len(out) > 4000 {
		t.Fatalf("returned source bypassed excerpt budget: size=%d", len(out))
	}
}

func TestRetrievalDebugTraceDoesNotLogCredentials(t *testing.T) {
	original := log.Writer()
	t.Cleanup(func() { log.SetOutput(original) })
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Setenv("WEBCTX_DEBUG", "1")
	traceRetrieval("https://docs.example.com/guide?api_key=secret-value&token=sensitive",
		"browser", "blocked", time.Now(), errors.New("unauthorized token sensitive"))
	result := output.String()
	if !strings.Contains(result, `stage="browser"`) || !strings.Contains(result, `error="source-unavailable"`) ||
		strings.Contains(result, "secret-value") || strings.Contains(result, "sensitive") {
		t.Fatalf("debug trace leaked query parameters or credentials: %s", result)
	}
}
