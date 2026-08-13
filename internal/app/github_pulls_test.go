package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseGitHubPullTargetsAndSelectors(t *testing.T) {
	for _, tt := range []struct {
		raw      string
		fragment string
	}{
		{raw: "https://github.com/o/r/pull/42"},
		{raw: "https://github.com/o/r/pull/42#issuecomment-99", fragment: "issuecomment-99"},
		{raw: "https://github.com/o/r/pull/42#discussion_r123", fragment: "discussion_r123"},
		{raw: "https://github.com/o/r/pull/42#pullrequestreview-456", fragment: "pullrequestreview-456"},
	} {
		target := parseGitHubTarget(tt.raw)
		if target == nil || target.Kind != GitHubTargetPull || target.Number != 42 || target.Fragment != tt.fragment {
			t.Fatalf("unexpected pull target for %s: %#v", tt.raw, target)
		}
	}
	for _, raw := range []string{
		"https://github.com/o/r/pull/nope",
		"https://github.com/o/r/pull/42/files",
		"https://github.com/o/r/pull/42/commits",
		"https://github.com/o/r/pull/42/checks",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("focused/later-phase route %s should remain unsupported, got %#v", raw, target)
		}
	}
}

func TestPullSelectorParsing(t *testing.T) {
	if id, ok, err := parsePullDiscussionSelector("discussion_r123"); !ok || err != nil || id != 123 {
		t.Fatalf("discussion selector failed: %d %v %v", id, ok, err)
	}
	if _, ok, err := parsePullDiscussionSelector("discussion_rx"); !ok || err == nil {
		t.Fatalf("invalid discussion selector not rejected: %v %v", ok, err)
	}
	if id, ok, err := parsePullReviewSelector("pullrequestreview-456"); !ok || err != nil || id != 456 {
		t.Fatalf("review selector failed: %d %v %v", id, ok, err)
	}
	if _, ok, err := parsePullReviewSelector("pullrequestreview-nope"); !ok || err == nil {
		t.Fatalf("invalid review selector not rejected: %v %v", ok, err)
	}
}

func TestReadGitHubPullRequestAnonymousCompleteConversation(t *testing.T) {
	var timelinePages, graphqlCalls int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{
                "number":42,"state":"open","draft":false,"merged":false,"title":"Improve things",
                "body":"Visible PR body\n<!-- hidden automation -->\nStill visible","html_url":"https://github.com/o/r/pull/42",
                "user":{"login":"author"},"created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-05T00:00:00Z",
                "comments":2,"review_comments":3,"commits":2,"additions":50,"deletions":10,"changed_files":4,
                "head":{"label":"fork:feature","ref":"feature","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
                "base":{"label":"o:main","ref":"main","sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
            }`)
		case "/repos/o/r/issues/42/timeline":
			atomic.AddInt32(&timelinePages, 1)
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `[
                    {"event":"committed","sha":"cccccccccccccccccccccccccccccccccccccccc","message":"Second commit\nmore"},
                    {"event":"ready_for_review","actor":{"login":"author"},"created_at":"2026-08-04T00:00:00Z"},
                    {"event":"review_request_removed","actor":{"login":"author"},"requested_reviewer":{"login":"reviewer"},"created_at":"2026-08-04T01:00:00Z"},
                    {"event":"mentioned","actor":{"login":"noise"},"created_at":"2026-08-04T02:00:00Z"}
                ]`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/issues/42/timeline?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[
                {"id":900,"event":"commented","body":"Normal bot comment\n<!-- hidden -->\nkept","user":{"login":"ci-bot","type":"Bot"},"author_association":"NONE","created_at":"2026-08-02T00:00:00Z","html_url":"https://github.com/o/r/pull/42#issuecomment-900"},
                {"id":500,"event":"reviewed","body":"Review duplicate body","state":"approved","user":{"login":"reviewer"},"created_at":"2026-08-02T01:00:00Z"},
                {"event":"head_ref_force_pushed","actor":{"login":"author"},"commit_id":"dddddddddddddddddddddddddddddddddddddddd","created_at":"2026-08-03T00:00:00Z"},
                {"event":"base_ref_changed","actor":{"login":"author"},"created_at":"2026-08-03T01:00:00Z"},
                {"event":"review_requested","actor":{"login":"author"},"requested_reviewer":{"login":"reviewer"},"created_at":"2026-08-03T02:00:00Z"}
            ]`)
		case "/repos/o/r/pulls/42/reviews":
			_, _ = io.WriteString(w, `[
                {"id":500,"state":"APPROVED","body":"Review duplicate body\n<!-- review hidden -->","commit_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","submitted_at":"2026-08-02T01:00:00Z","html_url":"https://github.com/o/r/pull/42#pullrequestreview-500","user":{"login":"reviewer"},"author_association":"MEMBER"},
                {"id":501,"state":"COMMENTED","body":"","commit_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","submitted_at":"2026-08-02T03:00:00Z","html_url":"https://github.com/o/r/pull/42#pullrequestreview-501","user":{"login":"outsider"},"author_association":"NONE"}
            ]`)
		case "/repos/o/r/pulls/42/comments":
			_, _ = io.WriteString(w, `[
                {"id":100,"pull_request_review_id":500,"path":"a.go","original_line":10,"side":"RIGHT","body":"Root comment","user":{"login":"reviewer"},"author_association":"MEMBER","created_at":"2026-08-02T01:10:00Z","html_url":"https://github.com/o/r/pull/42#discussion_r100"},
                {"id":101,"pull_request_review_id":501,"in_reply_to_id":100,"path":"a.go","original_line":10,"side":"RIGHT","body":"Reply from outsider","user":{"login":"outsider"},"author_association":"NONE","created_at":"2026-08-02T03:10:00Z","html_url":"https://github.com/o/r/pull/42#discussion_r101"},
                {"id":200,"pull_request_review_id":501,"path":"b.go","line":22,"side":"LEFT","body":"Second thread","user":{"login":"outsider"},"author_association":"NONE","created_at":"2026-08-02T04:00:00Z","html_url":"https://github.com/o/r/pull/42#discussion_r200"}
            ]`)
		case "/graphql":
			atomic.AddInt32(&graphqlCalls, 1)
			t.Fatal("anonymous PR read must not call GraphQL")
		default:
			t.Fatalf("unexpected request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()

	target := parseGitHubTarget("https://github.com/o/r/pull/42")
	out, err := readGitHubPullRequest(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`state: "open"`, `base: "o:main"`, `head: "fork:feature"`, `changed_files: 4`, `additions: 50`, `deletions: 10`,
		"Visible PR body", "Still visible", "Normal bot comment", "kept", "force-pushed the head ref to `dddddddddddd`", "changed the base ref", "requested review from @reviewer", "added commit `cccccccccccc` — Second commit", "marked the Pull Request ready for review", "removed review request from @reviewer",
		"APPROVED by @reviewer", "Review duplicate body", "COMMENTED by @outsider", "Thread 100 — `a.go` line 10 right", "Root comment", "Reply from outsider", "Thread 200 — `b.go` line 22 left", "Second thread",
		"Optional: set GH_TOKEN or GITHUB_TOKEN", "/pull/42/files", "/pull/42/commits", "/pull/42/checks",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("PR output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hidden automation") || strings.Contains(out, "review hidden") || strings.Contains(out, "@noise") {
		t.Fatalf("PR human-body sanitization/noise filtering regressed:\n%s", out)
	}
	if strings.Count(out, "Review duplicate body") != 1 {
		t.Fatalf("review duplicated from timeline and reviews endpoint:\n%s", out)
	}
	if got := atomic.LoadInt32(&timelinePages); got != 2 {
		t.Fatalf("expected two timeline pages, got %d", got)
	}
	if got := atomic.LoadInt32(&graphqlCalls); got != 0 {
		t.Fatalf("anonymous PR made %d GraphQL calls", got)
	}
}

func TestPullThreadGroupingFollowsReplyRootsAcrossReviews(t *testing.T) {
	root := int64(10)
	reply := int64(11)
	comments := []githubPullReviewComment{
		{ID: 12, InReplyToID: &reply, CreatedAt: "2026-08-03T00:00:00Z", Body: stringPtr("nested")},
		{ID: 10, CreatedAt: "2026-08-01T00:00:00Z", Body: stringPtr("root")},
		{ID: 11, InReplyToID: &root, CreatedAt: "2026-08-02T00:00:00Z", Body: stringPtr("reply")},
	}
	threads := groupGitHubPullThreads(comments)
	if len(threads) != 1 || threads[0].Root.ID != 10 || len(threads[0].Replies) != 2 || threads[0].Replies[0].ID != 11 || threads[0].Replies[1].ID != 12 {
		t.Fatalf("unexpected thread grouping: %#v", threads)
	}
}

func TestReadGitHubPullRequestGraphQLEnrichesThreadState(t *testing.T) {
	var graphQLCalls int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"state":"open","title":"x","html_url":"https://github.com/o/r/pull/42"}`)
		case "/repos/o/r/issues/42/timeline", "/repos/o/r/pulls/42/reviews":
			_, _ = io.WriteString(w, `[]`)
		case "/repos/o/r/pulls/42/comments":
			_, _ = io.WriteString(w, `[{"id":100,"path":"a.go","line":5,"side":"RIGHT","body":"thread","created_at":"2026-08-01T00:00:00Z"}]`)
		case "/graphql":
			atomic.AddInt32(&graphQLCalls, 1)
			if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
				t.Errorf("GraphQL auth mismatch: %q", got)
			}
			if r.Header.Get("X-GitHub-Api-Version") != "" {
				t.Errorf("GraphQL should not inherit REST version header")
			}
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if !strings.Contains(fmt.Sprint(payload["query"]), "reviewThreads") {
				t.Errorf("unexpected GraphQL query: %#v", payload)
			}
			_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"isResolved":true,"isOutdated":true,"resolvedBy":{"login":"resolver"},"comments":{"nodes":[{"fullDatabaseId":"100"}]}}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}}`)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := testGitHubClient(server.URL, server.URL, "fake-token")
	out, err := readGitHubPullRequest(context.Background(), client, parseGitHubTarget("https://github.com/o/r/pull/42"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "resolved · outdated · by @resolver") || strings.Contains(out, "Optional: set GH_TOKEN") {
		t.Fatalf("GraphQL thread state missing:\n%s", out)
	}
	if atomic.LoadInt32(&graphQLCalls) != 1 {
		t.Fatalf("expected one GraphQL call")
	}
}

func TestPullGraphQLEnrichmentFailurePreservesRESTCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"state":"open","title":"x"}`)
		case "/repos/o/r/issues/42/timeline", "/repos/o/r/pulls/42/reviews":
			_, _ = io.WriteString(w, `[]`)
		case "/repos/o/r/pulls/42/comments":
			_, _ = io.WriteString(w, `[{"id":100,"path":"a.go","line":5,"body":"REST survives"}]`)
		case "/graphql":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"message":"private graph detail must not surface"}`)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubPullRequest(context.Background(), testGitHubClient(server.URL, server.URL, "fake-token"), parseGitHubTarget("https://github.com/o/r/pull/42"))
	if err != nil {
		t.Fatalf("optional GraphQL failure destroyed REST core: %v", err)
	}
	if !strings.Contains(out, "REST survives") || !strings.Contains(out, "enrichment was unavailable") || strings.Contains(out, "private graph detail") {
		t.Fatalf("graceful enrichment failure incorrect:\n%s", out)
	}
}

func TestPullDiscussionSelectorReturnsOnlySelectedThread(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/comments/101":
			_, _ = io.WriteString(w, `{"id":101,"pull_request_url":"https://api.github.com/repos/o/r/pulls/42","in_reply_to_id":100,"path":"a.go","line":9,"body":"Selected reply","created_at":"2026-08-02T00:00:00Z"}`)
		case "/repos/o/r/pulls/42/comments":
			_, _ = io.WriteString(w, `[
                    {"id":100,"path":"a.go","line":9,"body":"Thread root","created_at":"2026-08-01T00:00:00Z"},
                    {"id":101,"in_reply_to_id":100,"path":"a.go","line":9,"body":"Selected reply","created_at":"2026-08-02T00:00:00Z"},
                    {"id":200,"path":"b.go","line":2,"body":"Unrelated thread","created_at":"2026-08-01T00:00:00Z"}
                ]`)
		default:
			t.Fatalf("selector fetched unrelated endpoint %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubPullRequest(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42#discussion_r101"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Thread root") || !strings.Contains(out, "Selected reply") || !strings.Contains(out, "selected") || strings.Contains(out, "Unrelated thread") {
		t.Fatalf("exact discussion selector not scoped:\n%s", out)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("expected targeted comment + review-comment list only, got %d requests", got)
	}
}

func TestPullReviewSelectorReturnsReviewAndItsComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42/reviews/500":
			_, _ = io.WriteString(w, `{"id":500,"state":"APPROVED","body":"Selected review","user":{"login":"reviewer"},"submitted_at":"2026-08-01T00:00:00Z"}`)
		case "/repos/o/r/pulls/42/reviews/500/comments":
			_, _ = io.WriteString(w, `[{"id":100,"pull_request_review_id":500,"path":"a.go","line":3,"body":"Review comment"}]`)
		default:
			t.Fatalf("review selector fetched unrelated endpoint %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubPullRequest(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42#pullrequestreview-500"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "review_id: 500") || !strings.Contains(out, "Selected review") || !strings.Contains(out, "Review comment") {
		t.Fatalf("review selector missing context:\n%s", out)
	}
}

func TestPullIssueCommentSelectorUsesSharedIdentity(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if r.URL.Path != "/repos/o/r/issues/comments/99" {
			t.Fatalf("normal PR comment selector fetched %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":99,"issue_url":"https://api.github.com/repos/o/r/issues/42","body":"PR conversation comment","user":{"login":"u"}}`)
	}))
	defer server.Close()
	out, err := readGitHubPullRequest(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42#issuecomment-99"))
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&requests) != 1 || !strings.Contains(out, "pull_request: 42") || !strings.Contains(out, "Comment on Pull Request") || !strings.Contains(out, "PR conversation comment") {
		t.Fatalf("shared normal-comment selector regressed:\n%s", out)
	}
}

func TestPullConversationProviderFailureStaysNative(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"private detail"}`)
	}))
	defer server.Close()
	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/pull/42"), testGitHubClient(server.URL, server.URL, ""))
	if result.Outcome != GitHubNativeFailure || result.Err == nil || !strings.Contains(result.Err.Error(), "may be private") || strings.Contains(result.Err.Error(), "private detail") {
		t.Fatalf("unexpected PR native failure: %#v", result)
	}
}

func TestGraphQLRequiresTokenAndDoesNotLeakProviderErrors(t *testing.T) {
	client := testGitHubClient("https://api.github.test", "https://raw.github.test", "")
	if err := client.GraphQL(context.Background(), "query{viewer{login}}", nil, &struct{}{}); err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected no-token GraphQL auth error, got %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"errors":[{"message":"secret private graph error"}]}`)
	}))
	defer server.Close()
	err := testGitHubClient(server.URL, server.URL, "token-value").GraphQL(context.Background(), "query{x}", nil, &struct{}{})
	if err == nil || strings.Contains(err.Error(), "secret private graph error") || strings.Contains(err.Error(), "token-value") {
		t.Fatalf("GraphQL provider detail leaked: %v", err)
	}
}

func TestFetchGitHubPullThreadStatesPaginatesGraphQL(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"isResolved":false,"isOutdated":false,"comments":{"nodes":[{"fullDatabaseId":"100"}]}}],"pageInfo":{"hasNextPage":true,"endCursor":"cursor-1"}}}}}}`)
			return
		}
		var payload struct {
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Variables["after"] != "cursor-1" {
			t.Errorf("GraphQL pagination cursor lost: %#v", payload.Variables)
		}
		_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"isResolved":true,"isOutdated":true,"comments":{"nodes":[{"fullDatabaseId":"200"}]}}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}}`)
	}))
	defer server.Close()
	states, err := fetchGitHubPullThreadStates(context.Background(), testGitHubClient(server.URL, server.URL, "token"), &GitHubTarget{Owner: "o", Repo: "r", Number: 42})
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[100].Resolved || !states[200].Resolved || !states[200].Outdated || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("unexpected GraphQL states/calls: %#v calls=%d", states, calls)
	}
}

func TestPullDisplayState(t *testing.T) {
	for _, tt := range []struct {
		pr   githubPullRequest
		want string
	}{
		{pr: githubPullRequest{State: "open", Draft: true}, want: "draft"},
		{pr: githubPullRequest{State: "closed", Merged: true}, want: "merged"},
		{pr: githubPullRequest{State: "closed", MergedAt: "2026-01-01T00:00:00Z"}, want: "merged"},
		{pr: githubPullRequest{State: "closed"}, want: "closed"},
	} {
		if got := pullDisplayState(tt.pr); got != tt.want {
			t.Fatalf("pullDisplayState(%#v)=%q want %q", tt.pr, got, tt.want)
		}
	}
}

func TestPullTimelineSkipsReviewDuplicatesButKeepsMeaningfulEvents(t *testing.T) {
	if line, ok := renderPullTimelineState(githubTimelineEvent{Event: "reviewed", ID: 5}); ok || line != "" {
		t.Fatalf("reviewed event should defer to review endpoint")
	}
	if line, ok := renderPullTimelineState(githubTimelineEvent{Event: "committed", SHA: "abcdef1234567890", Message: "subject\nbody"}); !ok || !strings.Contains(line, "`abcdef123456`") || !strings.Contains(line, "subject") || strings.Contains(line, "body") {
		t.Fatalf("commit timeline rendering incorrect: %q", line)
	}
	if line, ok := renderPullTimelineState(githubTimelineEvent{Event: "renamed", Rename: &struct {
		From string `json:"from"`
		To   string `json:"to"`
	}{From: "old", To: "new"}}); !ok || !strings.Contains(line, "renamed the Pull Request") {
		t.Fatalf("PR rename inherited Issue wording: %q", line)
	}
	tevent := githubTimelineEvent{Event: "review_requested"}
	tevent.RequestedTeam = &struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}{Name: "code reviewers", Slug: "code-reviewers"}
	if line, ok := renderPullTimelineState(tevent); !ok || !strings.Contains(line, "team `code reviewers`") {
		t.Fatalf("team review request lost identity: %q", line)
	}
}

func stringPtr(s string) *string { return &s }
