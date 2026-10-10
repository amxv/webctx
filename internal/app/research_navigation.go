package app

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var documentationIndexPattern = regexp.MustCompile("https?://[^\\s<>\"')\\]]+/llms\\.txt")

func documentIndexURLs(base *url.URL, primary string) []string {
	var urls []string
	seen := map[string]bool{}
	for _, raw := range documentationIndexPattern.FindAllString(primary, 5) {
		target, err := url.Parse(raw)
		if err != nil || validateSourceURL(raw) != nil ||
			!strings.EqualFold(base.Hostname(), target.Hostname()) || target.Port() != base.Port() {
			continue
		}
		if !seen[raw] {
			urls = append(urls, raw)
			seen[raw] = true
		}
	}
	fallback := base.Scheme + "://" + base.Host + "/llms.txt"
	if !seen[fallback] {
		urls = append(urls, fallback)
	}
	return urls
}

func implementationQuestion(question string) bool {
	text := strings.ToLower(question)
	for _, signal := range []string{
		"implement", "integrat", "runnable", "configuration",
		"code example", "complete", "how to", "set up", "using a", "creating",
	} {
		if strings.Contains(text, signal) {
			return true
		}
	}
	return false
}

func implementationLinkBonus(candidate sourceCandidate, question string) int {
	lower := strings.ToLower(candidate.url + " " + candidate.label)
	q := strings.ToLower(question)
	score := 0
	if implementationQuestion(question) {
		switch {
		case strings.Contains(lower, "/get-started/"), strings.Contains(lower, "/quickstart"),
			strings.Contains(lower, "getting started"):
			score += 64 // Publisher's end-to-end guide, not isolated snippets.
		case strings.Contains(lower, "tutorial"), strings.Contains(lower, "integration"),
			strings.Contains(lower, "how-to"):
			score += 16
		case strings.Contains(lower, "example"):
			score += 6
		}
	}
	if strings.Contains(q, "sql") || strings.Contains(q, "sqlite") {
		if strings.Contains(lower, "sqlite") || strings.Contains(lower, "storage") {
			score += 26
		} else if strings.Contains(lower, "/examples/") {
			score -= 16
		}
	}
	if strings.Contains(q, "webhook") || strings.Contains(q, "fulfill") || strings.Contains(q, "fulfil") {
		if strings.Contains(lower, "webhook") || strings.Contains(lower, "fulfill") || strings.Contains(lower, "fulfil") {
			score += 30
		}
	}
	if strings.Contains(q, "checkout") && (strings.Contains(lower, "checkout") || strings.Contains(lower, "session")) {
		score += 6
	}
	if strings.Contains(q, "signature") && strings.Contains(lower, "signature") {
		score += 24
	}
	if strings.Contains(q, "curl") || strings.Contains(q, "rest") || strings.Contains(q, "endpoint") {
		if strings.Contains(lower, "/api/") || strings.Contains(lower, "api-reference") {
			score += 10
		}
	}
	if strings.Contains(q, "session") && strings.Contains(q, "creat") &&
		(strings.Contains(lower, "/sessions/create") || strings.Contains(lower, "create a checkout session")) {
		score += 38
	}
	if strings.Contains(q, "checkout") && strings.Contains(lower, "/checkout/quickstart") {
		score += 20
	}
	if strings.Contains(q, "fulfill") || strings.Contains(q, "fulfil") {
		if strings.Contains(lower, "/checkout/fulfillment") {
			score += 24
		}
	}
	if strings.Contains(lower, "/alarms/") && !strings.Contains(q, "alarm") {
		score -= 24
	}
	if strings.Contains(lower, "/concepts/") && implementationQuestion(question) {
		score -= 12
	}
	if strings.Contains(lower, "legacy") && (strings.Contains(q, "new ") || strings.Contains(q, "current")) && !strings.Contains(q, "legacy") {
		score -= 25
	}
	return score
}

// Search indices on a large documentation host often contain links for many
// unrelated products. Prefer links in the original section or with a product
// named in the question. This prevents "webhooks for Climate Orders" from
// outranking generic Checkout webhooks solely because both contain "webhook".
func sourceScopeBonus(candidate sourceCandidate, rawURL, question string) int {
	base, err1 := url.Parse(rawURL)
	target, err2 := url.Parse(candidate.url)
	if err1 != nil || err2 != nil {
		return 0
	}
	q := strings.ToLower(question)
	sourceSegments := strings.Split(strings.Trim(base.Path, "/"), "/")
	targetSegments := strings.Split(strings.Trim(target.Path, "/"), "/")
	if len(targetSegments) == 0 {
		return 0
	}
	score := 0
	if len(sourceSegments) > 0 && targetSegments[0] == sourceSegments[0] {
		score += 8
	}
	if len(targetSegments) > 0 && targetSegments[0] != "" &&
		!strings.Contains(q, strings.TrimSuffix(targetSegments[0], "s")) &&
		!strings.Contains(q, targetSegments[0]) &&
		targetSegments[0] != "api" && targetSegments[0] != "webhooks" &&
		targetSegments[0] != "events" &&
		(len(sourceSegments) == 0 || targetSegments[0] != sourceSegments[0]) {
		score -= 24
	}
	// A general Worker overview is less relevant than Durable Objects'
	// focused setup instructions even if both mention "Worker".
	if strings.Contains(q, "durable object") && strings.Contains(target.Path, "/workers/") &&
		!strings.Contains(target.Path, "/durable-objects/") {
		score -= 14
	}
	return score
}

func normalizedDocumentationURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	p := strings.TrimSuffix(u.Path, "/index.md")
	p = strings.TrimSuffix(p, ".md")
	p = strings.TrimSuffix(p, "/")
	u.Path = p
	u.Fragment = ""
	u.RawFragment = ""
	u.RawPath = ""
	return u.String()
}

func researchCandidates(rawURL, primary, question string, limit int) []sourceCandidate {
	baseCandidates := relatedSourceLinks(primary, rawURL, question, limit*4)
	indexCandidates := linksFromDocsIndexWithPrimary(rawURL, primary, question, limit*5)
	seen := map[string]bool{normalizedDocumentationURL(rawURL): true}
	var candidates []sourceCandidate
	for _, c := range append(baseCandidates, indexCandidates...) {
		key := normalizedDocumentationURL(c.url)
		if seen[key] {
			continue
		}
		seen[key] = true
		c.score += implementationLinkBonus(c, question)
		c.score += sourceScopeBonus(c, rawURL, question)
		candidates = append(candidates, c)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].order < candidates[j].order
		}
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > limit {
		return candidates[:limit]
	}
	return candidates
}

type evidenceNeed struct {
	name         string
	alternatives []string
}

// Check for explicit source evidence; do not claim that a plausible guess
// supplies missing request fields or code.
func evidenceNeeds(question string) []evidenceNeed {
	q := strings.ToLower(question)
	var needs []evidenceNeed
	for _, literal := range []string{"line_items", "success_url", "cancel_url", "sql.exec", "wrangler", "sqlite"} {
		if strings.Contains(q, literal) {
			needs = append(needs, evidenceNeed{literal, []string{literal}})
		}
	}
	if strings.Contains(q, "webhook") {
		needs = append(needs, evidenceNeed{"webhook events", []string{"webhook", "checkout.session.completed"}})
		if strings.Contains(q, "signature") || strings.Contains(q, "secure") {
			needs = append(needs, evidenceNeed{"webhook signature verification", []string{"stripe-signature", "constructevent", "signature verification", "signatureverification"}})
		}
	}
	if strings.Contains(q, "sql table") || strings.Contains(q, "creating a sql") {
		needs = append(needs, evidenceNeed{"SQL table creation", []string{"create table"}})
	}
	if strings.Contains(q, "accesses the binding") || strings.Contains(q, "access the binding") {
		needs = append(needs, evidenceNeed{"Worker binding access", []string{"getbyname", "env.my_durable_object", "env.durable_object"}})
	}
	if strings.Contains(q, "checkout") && (strings.Contains(q, "fulfill") || strings.Contains(q, "fulfil")) {
		needs = append(needs, evidenceNeed{"Checkout fulfillment event", []string{"checkout.session.completed"}})
	}
	return needs
}

func missingResearchEvidence(question, text string) []string {
	lower := strings.ToLower(text)
	var missing []string
	for _, need := range evidenceNeeds(question) {
		found := false
		for _, candidate := range need.alternatives {
			if strings.Contains(lower, candidate) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, need.name)
		}
	}
	return missing
}
