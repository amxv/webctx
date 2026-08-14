package app

import "testing"

type githubTargetContractAudit struct {
	kind     GitHubTargetKind
	class    string
	contract string
}

func TestFinalGitHubTargetContractAuditCoversEveryNativeKind(t *testing.T) {
	audits := []githubTargetContractAudit{
		{GitHubTargetRepository, "overview", "repository metadata + bounded README preview"},
		{GitHubTargetBlob, "exact", "selected file; optional copied line selector"},
		{GitHubTargetTree, "overview", "bounded one-level directory index + README preview"},
		{GitHubTargetIssue, "overview/exact", "adaptive Issue root; #issue-* / #issuecomment-* exact"},
		{GitHubTargetIssueList, "overview", "single provider page with copied page navigation"},
		{GitHubTargetPullList, "overview", "single provider page with copied page navigation"},
		{GitHubTargetLabel, "overview", "selected label metadata + one Issue page"},
		{GitHubTargetLabelList, "overview", "single provider label page"},
		{GitHubTargetMilestone, "overview", "selected milestone metadata + one Issue page"},
		{GitHubTargetMilestones, "overview", "single provider milestone page"},
		{GitHubTargetPull, "overview/exact", "bounded PR root; copied body/comment/review/thread selectors exact"},
		{GitHubTargetPullFiles, "overview/exact", "bounded files root; copied diff file/hunk selector exact"},
		{GitHubTargetPullCommits, "overview", "bounded commit index; provider endpoint ceiling retained"},
		{GitHubTargetPullChecks, "overview/exact", "bounded checks rollup; ?check_run_id exact machine detail"},
		{GitHubTargetPullDiff, "raw", "explicit complete PR diff"},
		{GitHubTargetPullPatch, "raw", "explicit complete PR patch"},
		{GitHubTargetCommit, "overview/exact", "bounded commit root; diff/comment selector exact"},
		{GitHubTargetCommitDiff, "raw", "explicit complete commit diff"},
		{GitHubTargetCommitPatch, "raw", "explicit complete commit patch"},
		{GitHubTargetCompare, "overview", "bounded compare metadata/index with provider ceiling truth"},
		{GitHubTargetCompareDiff, "raw", "explicit complete compare diff"},
		{GitHubTargetComparePatch, "raw", "explicit complete compare patch"},
		{GitHubTargetHistory, "overview", "one 30-commit provider page"},
		{GitHubTargetBlame, "exact", "selected file blame ranges"},
		{GitHubTargetActions, "overview", "bounded workflows/recent-runs landing"},
		{GitHubTargetWorkflows, "overview", "one workflow provider page"},
		{GitHubTargetWorkflow, "overview", "workflow metadata + one run page"},
		{GitHubTargetActionsRun, "overview", "bounded job/artifact rollup with exact job URLs"},
		{GitHubTargetActionsJob, "exact", "selected structured job + bounded log preview"},
		{GitHubTargetBranches, "overview", "single bounded provider page"},
		{GitHubTargetTags, "overview", "single bounded provider page"},
		{GitHubTargetReleases, "overview", "single bounded provider page"},
		{GitHubTargetRelease, "overview", "bounded release notes + first asset page"},
		{GitHubTargetReleaseLatest, "overview", "bounded latest release notes + first asset page"},
		{GitHubTargetForks, "overview", "single bounded provider page"},
		{GitHubTargetStargazers, "overview", "single bounded provider page"},
		{GitHubTargetWatchers, "overview", "single bounded provider page"},
		{GitHubTargetDiscussions, "overview", "first 30 GraphQL Discussions with upstream cursor truth"},
		{GitHubTargetDiscussion, "overview/exact", "bounded root; #discussioncomment-* exact"},
		{GitHubTargetGist, "overview/exact", "bounded root; #file-* / #gistcomment-* exact"},
		{GitHubTargetSearch, "overview", "one 30-result provider page + provider ceiling truth"},
		{GitHubTargetProfile, "overview", "profile metadata or one selected profile-tab page"},
		{GitHubTargetActivity, "overview", "one 30-item activity provider page"},
		{GitHubTargetStatsContributors, "overview", "bounded contributor ranking from provider aggregate"},
		{GitHubTargetStatsCommitActivity, "overview", "provider-defined last-year weekly aggregate"},
		{GitHubTargetStatsCodeFrequency, "overview", "full aggregates + recent 52-week index"},
		{GitHubTargetDeployments, "overview", "one deployment provider page; no status fan-out"},
		{GitHubTargetDeploymentEnvironment, "overview", "bounded deployments + latest returned status each"},
		{GitHubTargetPackage, "overview", "package metadata + one version provider page"},
		{GitHubTargetProjectV2, "overview", "project metadata + first bounded item page"},
	}

	if len(audits) != 50 {
		t.Fatalf("GitHub target contract audit has %d entries, want 50 current native kinds", len(audits))
	}
	seen := map[GitHubTargetKind]bool{}
	for _, audit := range audits {
		if seen[audit.kind] {
			t.Fatalf("GitHub target contract audit duplicates %q", audit.kind)
		}
		seen[audit.kind] = true
		t.Logf("%-24s %-14s %s", audit.kind, audit.class, audit.contract)
	}
	for _, kind := range []GitHubTargetKind{
		GitHubTargetRepository, GitHubTargetBlob, GitHubTargetTree, GitHubTargetIssue, GitHubTargetIssueList,
		GitHubTargetPullList, GitHubTargetLabel, GitHubTargetLabelList, GitHubTargetMilestone, GitHubTargetMilestones,
		GitHubTargetPull, GitHubTargetPullFiles, GitHubTargetPullCommits, GitHubTargetPullChecks, GitHubTargetPullDiff,
		GitHubTargetPullPatch, GitHubTargetCommit, GitHubTargetCommitDiff, GitHubTargetCommitPatch, GitHubTargetCompare,
		GitHubTargetCompareDiff, GitHubTargetComparePatch, GitHubTargetHistory, GitHubTargetBlame, GitHubTargetActions,
		GitHubTargetWorkflows, GitHubTargetWorkflow, GitHubTargetActionsRun, GitHubTargetActionsJob, GitHubTargetBranches,
		GitHubTargetTags, GitHubTargetReleases, GitHubTargetRelease, GitHubTargetReleaseLatest, GitHubTargetForks,
		GitHubTargetStargazers, GitHubTargetWatchers, GitHubTargetDiscussions, GitHubTargetDiscussion, GitHubTargetGist,
		GitHubTargetSearch, GitHubTargetProfile, GitHubTargetActivity, GitHubTargetStatsContributors, GitHubTargetStatsCommitActivity,
		GitHubTargetStatsCodeFrequency, GitHubTargetDeployments, GitHubTargetDeploymentEnvironment, GitHubTargetPackage, GitHubTargetProjectV2,
	} {
		if !seen[kind] {
			t.Fatalf("GitHub target contract audit omitted %q", kind)
		}
	}
}

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
