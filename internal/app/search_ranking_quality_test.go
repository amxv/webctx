package app

import "testing"

func TestCodingQueryPrefersOfficialDocsAndFirstPartyGitHub(t *testing.T) {
	results, removed := scoreAndRankResults([]providerDocs{{Provider: "Brave", Docs: []SearchDoc{
		{URL: "https://example.net/top", Title: "An unrelated discussion", Overview: "Some blog"},
		{URL: "https://docs.python.org/3/library/asyncio-task.html", Title: "Python asyncio TaskGroup documentation"},
		{URL: "https://github.com/python/cpython", Title: "Python CPython first-party implementation"},
		{URL: "https://github.com/unrelated/cpython", Title: "Python CPython fork"},
	}}}, "python asyncio TaskGroup documentation")
	if removed != 0 || len(results) != 4 {
		t.Fatalf("ranking changed result count: %+v, removed=%d", results, removed)
	}
	if results[0].URL != "https://docs.python.org/3/library/asyncio-task.html" {
		t.Errorf("official docs should outrank generic high-position source: %+v", results)
	}
	if firstPartyGitHubRepository("/unrelated/cpython", "python asyncio") {
		t.Error("third-party fork was mistaken for first-party source")
	}
	if !firstPartyGitHubRepository("/python/cpython", "python asyncio") {
		t.Error("official Python GitHub repository was not recognized")
	}
}

func TestGenericSearchDoesNotReceiveTechnicalSourceBonus(t *testing.T) {
	if technicalSearchQuery("restaurant reservation friday evening") {
		t.Fatal("unrelated consumer query was treated as technical")
	}
	if got := searchQualityBonus("restaurant reservation friday evening", SearchDoc{URL: "https://docs.example.com/api/", Title: "Unrelated documentation"}); got != 0 {
		t.Fatalf("unrelated docs received a technical boost: %v", got)
	}
}

func TestOfficialGitHubScoreRequiresMatchingEcosystem(t *testing.T) {
	if firstPartyGitHubRepository("/microsoft/typescript", "rust borrow checker") {
		t.Fatal("first-party status should be query-specific")
	}
	if !firstPartyGitHubRepository("/modelcontextprotocol/go-sdk", "MCP Go SDK") {
		t.Fatal("official MCP repository should score for an MCP query")
	}
	if firstPartyGitHubRepository("/microsoft/vscode", "TypeScript compiler API") {
		t.Fatal("unrelated repository from a broad upstream organization should not get a first-party boost")
	}
}
