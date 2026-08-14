package app

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

func assertGitHubOverviewBound(t *testing.T, name, out string) {
	t.Helper()
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("%s exceeded shared overview target: %d runes\n%s", name, got, out)
	}
}

func TestPhase9AdversarialHumanTextCannotAmplifyMajorOverviews(t *testing.T) {
	long := strings.Repeat("very long human-authored metadata ", 600)
	body := long

	t.Run("issue-root", func(t *testing.T) {
		issue := githubIssue{ID: 123, Number: 42, State: "open", Title: long, Body: &body, HTMLURL: "https://github.com/o/r/issues/42", Comments: 500}
		for i := 0; i < 100; i++ {
			issue.Labels = append(issue.Labels, githubIssueLabel{Name: fmt.Sprintf("label-%03d-%s", i, strings.Repeat("x", 80))})
			issue.Assignees = append(issue.Assignees, githubUser{Login: fmt.Sprintf("assignee-%03d-%s", i, strings.Repeat("y", 80))})
		}
		rel := githubIssueRelationships{SubIssues: []githubIssue{{Number: 43, Title: long, HTMLURL: "https://github.com/o/r/issues/43"}}, Fields: []githubIssueFieldValue{{IssueFieldName: long, Value: long}}}
		out := renderGitHubIssue(parseGitHubTarget("https://github.com/o/r/issues/42"), issue, nil, rel, githubIssueAvailability{TimelineProviderMore: true})
		assertGitHubOverviewBound(t, "Issue root", out)
		for _, want := range []string{"title_preview_truncated: true", "labels_local_omitted: 95", "assignees_local_omitted: 95", "https://github.com/o/r/issues/42#issue-123"} {
			if !strings.Contains(out, want) {
				t.Fatalf("Issue overview missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("pull-root", func(t *testing.T) {
		pr := githubPullRequest{Number: 42, State: "open", Title: long, Body: &body, HTMLURL: "https://github.com/o/r/pull/42", Comments: 500, ReviewComments: 500, Commits: 400, ChangedFiles: 1000}
		pr.Base.Label = long
		pr.Head.Label = long
		out := renderGitHubPullRequest(parseGitHubTarget("https://github.com/o/r/pull/42"), pr, 99, nil, nil, nil, githubPullOverviewAvailability{TimelineProviderMore: true}, "", false)
		assertGitHubOverviewBound(t, "Pull Request root", out)
		if !strings.Contains(out, "title_preview_truncated: true") || !strings.Contains(out, "#issue-99") {
			t.Fatalf("PR overview lost bounded-title/exact-body navigation:\n%s", out)
		}
	})

	t.Run("release-root", func(t *testing.T) {
		release := githubRelease{TagName: long, Name: long, HTMLURL: "https://github.com/o/r/releases/tag/v1", Body: &body}
		out := renderGitHubRelease(parseGitHubTarget("https://github.com/o/r/releases/tag/v1"), release, githubReleaseAssetsAvailability{})
		assertGitHubOverviewBound(t, "Release root", out)
		if !strings.Contains(out, "tag_preview_truncated: true") || !strings.Contains(out, "name_preview_truncated: true") || !strings.Contains(out, "Full release notes:") {
			t.Fatalf("release overview lost truncation/deeper navigation:\n%s", out)
		}
	})

	t.Run("discussion-root", func(t *testing.T) {
		detail := githubDiscussionDetail{githubDiscussionSummary: githubDiscussionSummary{Number: 7, Title: long, URL: "https://github.com/o/r/discussions/7", Category: long}, Body: body, CommentsReported: 100, CommentsProviderMore: true}
		out := renderGitHubDiscussion(parseGitHubTarget("https://github.com/o/r/discussions/7"), detail)
		assertGitHubOverviewBound(t, "Discussion root", out)
		if !strings.Contains(out, "title_preview_truncated: true") || !strings.Contains(out, "category_preview_truncated: true") {
			t.Fatalf("Discussion overview lost bounded metadata:\n%s", out)
		}
	})
}

func TestPhase9AdversarialOverviewIndexesStayBounded(t *testing.T) {
	long := strings.Repeat("long index value ", 200)
	target := parseGitHubTarget("https://github.com/o/r")

	t.Run("issues", func(t *testing.T) {
		items := make([]githubIssue, 100)
		for i := range items {
			items[i] = githubIssue{Number: i + 1, State: "open", Title: long, HTMLURL: fmt.Sprintf("https://github.com/o/r/issues/%d", i+1)}
			for j := 0; j < 20; j++ {
				items[i].Labels = append(items[i].Labels, githubIssueLabel{Name: long})
			}
		}
		listTarget := parseGitHubTarget("https://github.com/o/r/issues?per_page=100")
		out := renderGitHubIssueList(listTarget, items, nil, 100, false)
		assertGitHubOverviewBound(t, "Issue list", out)
		if !strings.Contains(out, "results_local_omitted:") || !strings.Contains(out, "locally omitted") {
			t.Fatalf("Issue list omission not explicit:\n%s", out)
		}
	})

	t.Run("pulls", func(t *testing.T) {
		items := make([]githubPullListItem, 100)
		for i := range items {
			items[i] = githubPullListItem{Number: i + 1, State: "open", Title: long, HTMLURL: fmt.Sprintf("https://github.com/o/r/pull/%d", i+1)}
		}
		out := renderGitHubPullList(parseGitHubTarget("https://github.com/o/r/pulls"), items, nil, 100, false)
		assertGitHubOverviewBound(t, "Pull list", out)
	})

	t.Run("repository-lists", func(t *testing.T) {
		branches := make([]githubBranch, 100)
		tags := make([]githubTag, 100)
		releases := make([]githubRelease, 100)
		forks := make([]githubFork, 100)
		stars := make([]githubStar, 100)
		users := make([]githubUser, 100)
		for i := range branches {
			branches[i].Name = long
			tags[i].Name = long
			releases[i].Name, releases[i].TagName = long, long
			releases[i].HTMLURL = fmt.Sprintf("https://github.com/o/r/releases/tag/%d", i)
			forks[i].FullName, forks[i].HTMLURL = long, fmt.Sprintf("https://github.com/fork/%d", i)
			stars[i].User.Login = fmt.Sprintf("user-%d", i)
			users[i].Login = fmt.Sprintf("user-%d", i)
		}
		for name, out := range map[string]string{
			"branches":   renderGitHubBranches(target, branches, nil),
			"tags":       renderGitHubTags(target, tags, nil),
			"releases":   renderGitHubReleases(target, releases, nil),
			"forks":      renderGitHubForks(target, forks, nil),
			"stargazers": renderGitHubStargazers(target, stars, nil),
			"watchers":   renderGitHubWatchers(target, users, nil),
		} {
			assertGitHubOverviewBound(t, name, out)
		}
	})

	t.Run("search", func(t *testing.T) {
		items := make([]githubRepository, 30)
		for i := range items {
			items[i] = githubRepository{FullName: fmt.Sprintf("o/r-%d", i), HTMLURL: fmt.Sprintf("https://github.com/o/r-%d", i), Description: long}
		}
		raw, _ := json.Marshal(items)
		searchTarget := &GitHubTarget{Kind: GitHubTargetSearch, Query: url.Values{"q": []string{long}}}
		out, err := renderGitHubSearch(searchTarget, "repositories", githubSearchEnvelope{Items: raw, TotalCount: 5000}, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertGitHubOverviewBound(t, "Search", out)
		if !strings.Contains(out, "indexed: 15") || !strings.Contains(out, "query_preview_truncated: true") {
			t.Fatalf("Search bounds not explicit:\n%s", out)
		}
	})

	t.Run("package-project-workflow-history", func(t *testing.T) {
		pkg := githubPackage{Name: "pkg", PackageType: "container", HTMLURL: "https://github.com/users/o/packages/container/package/pkg"}
		pkg.Description = &long
		versions := make([]githubPackageVersion, 30)
		for i := range versions {
			versions[i].Name = long
			versions[i].PackageHTMLURL = fmt.Sprintf("https://github.com/pkg/%d", i)
			versions[i].Metadata.Container.Tags = make([]string, 30)
			for j := range versions[i].Metadata.Container.Tags {
				versions[i].Metadata.Container.Tags[j] = long
			}
		}
		assertGitHubOverviewBound(t, "Package", renderGitHubPackage(&GitHubTarget{Owner: "o", Query: url.Values{}}, pkg, versions, nil))

		project := githubProjectV2{Number: 1, Title: long, ShortDescription: long, URL: "https://github.com/orgs/o/projects/1", Items: make([]githubProjectV2Item, 50), MoreItems: true}
		for i := range project.Items {
			project.Items[i] = githubProjectV2Item{Title: long, URL: fmt.Sprintf("https://github.com/o/r/issues/%d", i+1), Type: long, Repository: long, State: long}
		}
		assertGitHubOverviewBound(t, "Project", renderGitHubProjectV2(&GitHubTarget{Owner: "o"}, project))

		workflows := make([]githubWorkflow, 30)
		for i := range workflows {
			workflows[i] = githubWorkflow{ID: int64(i + 1), Name: long, Path: long, HTMLURL: fmt.Sprintf("https://github.com/o/r/actions/workflows/%d", i+1)}
		}
		assertGitHubOverviewBound(t, "Workflow list", renderGitHubWorkflows(target, workflows, 30, nil))

		commits := make([]githubPullCommit, 30)
		for i := range commits {
			commits[i].SHA = fmt.Sprintf("%040x", i+1)
			commits[i].HTMLURL = fmt.Sprintf("https://github.com/o/r/commit/%040x", i+1)
			commits[i].Commit.Message = long
			commits[i].Commit.Author.Name = long
		}
		assertGitHubOverviewBound(t, "History", renderGitHubHistory(target, resolvedGitHubPath{Ref: "main"}, 1, commits, nil))
	})
}
