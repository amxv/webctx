package app

import (
	"strings"
	"testing"
)

func TestFocusedExcerptPreservesWholeCurlExamples(t *testing.T) {
	code := backtickFence + "bash\ncurl -X POST https://api.example.com/v2/scrape \\\n" +
		" -H 'Content-Type: application/json' \\\n" +
		" -d '{\"alexandria\":{\"capability\":\"find-tools\"}}'\n" + backtickFence
	doc := "# Documentation\n\n" + strings.Repeat("Background prose unrelated to endpoints.\n", 200) +
		"\n## Inspect tools over REST\n" + strings.Repeat("Introductory filler.\n", 70) +
		"\n" + code + "\n" + strings.Repeat("Appendix filler.\n", 200)
	got := focusExcerpt(doc, "How do I inspect Alexandria tools with a REST request example?", 1600)
	if !strings.Contains(got, code) {
		t.Fatalf("the complete REST example must survive extraction:\n%s", got)
	}
	if strings.Count(got, backtickFence) != 2 {
		t.Fatalf("expected exactly one balanced code fence: %s", got)
	}
	if len(got) > 1800 {
		t.Fatalf("excerpt exceeded budget: %d", len(got))
	}
}

func TestOversizedCodeBlocksAreNeverPartiallyReturned(t *testing.T) {
	huge := backtickFence + "json\n{\"payload\":\"" + strings.Repeat("a", 3000) + "\"}\n" + backtickFence
	doc := "# API\n\n" + huge + "\n\n" + strings.Repeat("Further explanation.\n", 200)
	got := atomicFocusedSection(doc, "What is the API payload?", 700)
	if strings.Contains(got, "\"payload\"") || strings.Contains(got, backtickFence) {
		t.Fatalf("oversized example should be omitted, not truncated:\n%s", got)
	}
	if !strings.Contains(got, "omitted") && !strings.Contains(got, "source URL") {
		t.Fatalf("omission should be explicitly marked: %s", got)
	}
}

func TestPlainCodeFenceOpeningNotTreatedAsClosing(t *testing.T) {
	doc := "# API\n" + strings.Repeat("Preliminary text.\n", 80) + "\n" +
		backtickFence + "\nPOST /v2/scrape\n{}\n" + backtickFence + "\n" +
		strings.Repeat("End notes.\n", 150)
	got := atomicFocusedSection(doc, "scrape request", 650)
	if !strings.Contains(got, backtickFence+"\nPOST /v2/scrape\n{}\n"+backtickFence) {
		t.Fatalf("plain opening fence was not handled atomically: %s", got)
	}
}
