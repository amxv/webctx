package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestStripeFocusedLinksPreferCheckoutAndWebhookDocs(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	index := "# Documentation\n\n" +
		"- [Climate order webhooks](https://docs.stripe.com/climate/orders/webhooks.md)\n" +
		"- [Create a Checkout Session](https://docs.stripe.com/api/checkout/sessions/create.md)\n" +
		"- [Build a Stripe-hosted checkout page](https://docs.stripe.com/checkout/quickstart.md)\n" +
		"- [Set up and deploy a webhook](https://docs.stripe.com/webhooks/quickstart.md)\n" +
		"- [Fulfill orders](https://docs.stripe.com/checkout/fulfillment.md)\n" +
		"- [Verify webhook signatures](https://docs.stripe.com/webhooks.md#verify-signature)\n"
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/llms.txt" {
			return testHTTPResponse(req, http.StatusOK, index, nil), nil
		}
		return testHTTPResponse(req, http.StatusNotFound, "", nil), nil
	})}
	question := "Implement Stripe-hosted Checkout: create a Checkout Session REST request and securely fulfill orders using webhooks, including signature verification"
	primary := "# Stripe Checkout\n\n[Start building your checkout integration](https://docs.stripe.com/checkout/quickstart.md)"
	got := researchCandidates("https://docs.stripe.com/payments/checkout", primary, question, 5)
	var links []string
	for _, c := range got {
		links = append(links, c.url)
	}
	output := strings.Join(links, "\n")
	for _, want := range []string{
		"/checkout/quickstart.md",
		"/api/checkout/sessions/create.md",
		"/webhooks/quickstart.md",
		"/checkout/fulfillment.md",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q among selected source links: %s", want, output)
		}
	}
	if strings.Contains(output, "/climate/") {
		t.Errorf("irrelevant Climate-specific webhook outranked Checkout sources: %s", output)
	}
}

func TestCloudflareNewSQLiteQuestionPrefersGettingStarted(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return testHTTPResponse(req, http.StatusForbidden, "", nil), nil
	})}
	primary := "# Cloudflare Durable Objects\n" +
		"[Durable Object alarms](https://developers.cloudflare.com/durable-objects/api/alarms/)\n" +
		"[SQLite storage](https://developers.cloudflare.com/durable-objects/api/sqlite-storage-api/)\n" +
		"[Get started](https://developers.cloudflare.com/durable-objects/get-started/)\n" +
		"> Documentation Index https://developers.cloudflare.com/durable-objects/llms.txt\n"
	question := "For a new Cloudflare Worker using a SQLite-backed Durable Object, show complete wrangler configuration, Worker binding, and SQL table code."
	got := researchCandidates("https://developers.cloudflare.com/durable-objects/", primary, question, 2)
	if len(got) != 2 {
		t.Fatalf("unexpected number of selected references: %+v", got)
	}
	var links []string
	for _, c := range got {
		links = append(links, c.url)
	}
	output := strings.Join(links, "\n")
	if !strings.Contains(output, "/get-started/") || strings.Contains(output, "/alarms/") {
		t.Fatalf("unrelated alarm material displaced getting-started guide: %s", output)
	}
}
