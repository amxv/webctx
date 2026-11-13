package app

import (
	"os"
	"strings"
)

// Firecrawl enforces these limits per team, not per individual API key.
// Source: https://docs.firecrawl.dev/rate-limits (November 2026).
// This intentionally supports only the five published self-service plans.
// Unknown/unset values fail closed to Free rather than guessing higher limits.
type firecrawlPlanLimits struct {
	scrape       int
	mapSite      int
	search       int
	crawl        int
	agent        int
	crawlStatus  int
	agentStatus  int
	interact     int
	interactExec int
	browserSlots int
}

var firecrawlPlans = map[string]firecrawlPlanLimits{
	"free": {
		scrape: 10, mapSite: 10, search: 10, crawl: 2, agent: 2,
		crawlStatus: 500, agentStatus: 500, interact: 2, interactExec: 10, browserSlots: 2,
	},
	"hobby": {
		scrape: 100, mapSite: 100, search: 100, crawl: 20, agent: 20,
		crawlStatus: 5000, agentStatus: 5000, interact: 20, interactExec: 100, browserSlots: 5,
	},
	"standard": {
		scrape: 500, mapSite: 500, search: 500, crawl: 100, agent: 100,
		crawlStatus: 25000, agentStatus: 25000, interact: 100, interactExec: 500, browserSlots: 25,
	},
	"growth": {
		scrape: 5000, mapSite: 5000, search: 5000, crawl: 1000, agent: 1000,
		crawlStatus: 250000, agentStatus: 250000, interact: 1000, interactExec: 5000, browserSlots: 50,
	},
	"scale": {
		scrape: 10000, mapSite: 10000, search: 10000, crawl: 2000, agent: 2000,
		crawlStatus: 500000, agentStatus: 500000, interact: 1500, interactExec: 7500, browserSlots: 100,
	},
}

const firecrawlPlanEnv = "WEBCTX_FIRECRAWL_PLAN"

func activeFirecrawlPlan() (string, firecrawlPlanLimits) {
	name := strings.ToLower(strings.TrimSpace(os.Getenv(firecrawlPlanEnv)))
	if limits, ok := firecrawlPlans[name]; ok {
		return name, limits
	}
	return "free", firecrawlPlans["free"]
}
