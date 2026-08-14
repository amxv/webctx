package app

import "testing"

func TestFinalRouteFidelityDoesNotClaimGlobalGitHubNamespaces(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/settings/profile",
		"https://github.com/settings/security",
		"https://github.com/organizations/plan",
		"https://github.com/codespaces/new",
		"https://github.com/marketplace/actions",
		"https://github.com/security/advisories",
		"https://github.com/sponsors/explore",
		"https://github.com/notifications/subscriptions",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("global GitHub namespace must stay outside native repository/profile routing: %s => %#v", raw, target)
		}
	}
}

func TestFinalRouteFidelityKeepsExcludedRepositorySurfacesGeneric(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/o/r/wiki",
		"https://github.com/o/r/settings",
		"https://github.com/o/r/settings/actions",
		"https://github.com/o/r/security",
		"https://github.com/o/r/security/code-scanning",
		"https://github.com/o/r/network/members",
		"https://github.com/o/r/community",
		"https://github.com/o/r/graphs/traffic",
		"https://github.com/o/r/archive/refs/heads/main.zip",
		"https://github.com/o/r/releases/download/v1/binary.zip",
		"https://github.com/o/r/issues/new",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("excluded/unproven repository surface must stay generic: %s => %#v", raw, target)
		}
	}
}

func TestFinalNativeRouteCoverageRepresentativeMatrix(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/o/r",
		"https://github.com/o/r/blob/main/README.md#L1-L2",
		"https://github.com/o/r/tree/main/docs",
		"https://github.com/o/r/issues/1",
		"https://github.com/o/r/pulls",
		"https://github.com/o/r/pull/1",
		"https://github.com/o/r/pull/1/files",
		"https://github.com/o/r/commit/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"https://github.com/o/r/compare/main...next",
		"https://github.com/o/r/commits/main/README.md",
		"https://github.com/o/r/blame/main/README.md",
		"https://github.com/o/r/actions",
		"https://github.com/o/r/actions/runs/1/job/2",
		"https://github.com/o/r/branches",
		"https://github.com/o/r/tags",
		"https://github.com/o/r/releases",
		"https://github.com/o/r/forks",
		"https://github.com/o/r/stargazers",
		"https://github.com/o/r/watchers",
		"https://github.com/o/r/discussions/1",
		"https://gist.github.com/alice/abcdef#file-readme-md-L1-L2",
		"https://github.com/search?q=webctx&type=repositories",
		"https://github.com/octocat",
		"https://github.com/o/r/activity",
		"https://github.com/o/r/graphs/contributors",
		"https://github.com/o/r/deployments/production",
		"https://github.com/orgs/acme/packages/container/package/widget",
		"https://github.com/orgs/acme/projects/7",
	} {
		if target := parseGitHubTarget(raw); target == nil {
			t.Fatalf("representative native route unexpectedly unsupported: %s", raw)
		}
	}
}
