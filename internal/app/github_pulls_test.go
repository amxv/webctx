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
	"unicode/utf8"
)

func TestParseGitHubPullTargetsAndSelectors(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/o/r/pulls",
		"https://github.com/o/r/pulls?page=2",
		"https://github.com/o/r/pulls?state=closed&base=main&sort=updated&direction=asc",
		"https://github.com/o/r/pulls?q=is%3Apr+is%3Aopen",
	} {
		target := parseGitHubTarget(raw)
		if target == nil || target.Kind != GitHubTargetPullList || target.Owner != "o" || target.Repo != "r" {
			t.Fatalf("unexpected pull-list target for %s: %#v", raw, target)
		}
	}
	for _, tt := range []struct {
		raw      string
		fragment string
	}{
		{raw: "https://github.com/o/r/pull/42"},
		{raw: "https://github.com/o/r/pull/42#issue-987", fragment: "issue-987"},
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
		"https://github.com/o/r/pull/42/unknown",
		"https://github.com/o/r/pulls/42",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("unsupported PR route %s should remain unsupported, got %#v", raw, target)
		}
	}
}

func TestGitHubPullListUsesRESTFiltersDraftStateAndNavigation(t *testing.T) {
	var calls int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/repos/o/r/pulls" {
			t.Fatalf("unexpected PR-list path: %s", r.URL.Path)
		}
		for key, want := range map[string]string{
			"state": "all", "head": "alice:feature", "base": "main", "sort": "updated", "direction": "asc", "per_page": "8", "page": "2",
		} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("PR-list query %s=%q want %q (%s)", key, got, want, r.URL.RawQuery)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/pulls?state=all&page=1>; rel="prev", <%s/repos/o/r/pulls?state=all&page=3>; rel="next"`, server.URL, server.URL))
		_, _ = io.WriteString(w, `[
			{"number":10,"state":"open","draft":false,"title":"Open PR","html_url":"https://github.com/o/r/pull/10","user":{"login":"alice"},"updated_at":"2026-08-10T00:00:00Z","head":{"label":"alice:feature"},"base":{"ref":"main"}},
			{"number":11,"state":"open","draft":true,"title":"Draft PR","html_url":"https://github.com/o/r/pull/11","user":{"login":"bob"},"created_at":"2026-08-09T00:00:00Z","head":{"label":"bob:wip"},"base":{"ref":"main"}},
			{"number":12,"state":"closed","draft":false,"title":"Closed PR","html_url":"https://github.com/o/r/pull/12","user":{"login":"carol"},"updated_at":"2026-08-08T00:00:00Z"}
		]`)
	}))
	defer server.Close()

	target := parseGitHubTarget("https://github.com/o/r/pulls?state=all&head=alice%3Afeature&base=main&sort=updated&direction=asc&page=2")
	out, err := readGitHubPullList(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`repository: "o/r"`, "view: pull_requests", `page: "2"`, "results_indexed: 3", "results_local_omitted: 0", "# Pull Requests",
		"[#10 Open PR](https://github.com/o/r/pull/10) — open · @alice · updated 2026-08-10T00:00:00Z",
		"[#11 Draft PR](https://github.com/o/r/pull/11) — draft · @bob · created 2026-08-09T00:00:00Z",
		"[#12 Closed PR](https://github.com/o/r/pull/12) — closed · @carol",
		"Previous: https://github.com/o/r/pulls?", "page=1", "Next: https://github.com/o/r/pulls?", "page=3",
		"base=main", "head=alice%3Afeature", "sort=updated", "direction=asc",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("native PR list missing %q:\n%s", want, out)
		}
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("PR list should use exactly one provider request, got %d", calls)
	}
}

func TestGitHubPullSearchAndIssuesPRSearchShareTruthfulSemantics(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/o/r/pulls?q=label%3Abug&page=2",
		"https://github.com/o/r/issues?q=is%3Apr+label%3Abug&page=2",
	} {
		t.Run(raw, func(t *testing.T) {
			var calls int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				if r.URL.Path != "/search/issues" {
					t.Fatalf("PR search used wrong endpoint: %s", r.URL.Path)
				}
				q := r.URL.Query().Get("q")
				if !strings.Contains(q, "repo:o/r") || !strings.Contains(q, "is:pr") || strings.Contains(q, "is:issue") || !strings.Contains(q, "label:bug") {
					t.Fatalf("PR search provider query lost semantics: %q", q)
				}
				if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "8" {
					t.Fatalf("PR search pagination wrong: %s", r.URL.RawQuery)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"total_count":1201,"incomplete_results":false,"items":[
					{"number":20,"state":"open","draft":true,"title":"PR result","html_url":"https://github.com/o/r/issues/20","pull_request":{"html_url":"https://github.com/o/r/pull/20"},"user":{"login":"alice"},"updated_at":"2026-08-12T00:00:00Z","labels":[{"name":"bug"}]}
				]}`)
			}))
			defer server.Close()

			target := parseGitHubTarget(raw)
			var (
				out string
				err error
			)
			if target.Kind == GitHubTargetPullList {
				out, err = readGitHubPullList(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
			} else {
				out, err = readGitHubIssueList(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"view: pull_requests", "total_matches: 1201", "provider_result_ceiling: 1000", "[#20 PR result](https://github.com/o/r/pull/20)", "draft", "@alice", "labels: bug",
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("PR search output missing %q:\n%s", want, out)
				}
			}
			if atomic.LoadInt32(&calls) != 1 {
				t.Fatalf("PR search should use one provider request, got %d", calls)
			}
		})
	}
}

func TestGitHubPullListRejectsUnsupportedOrConflictingQueryBeforeProvider(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/o/r/pulls?unknown=x",
		"https://github.com/o/r/pulls?state=merged",
		"https://github.com/o/r/pulls?page=zero",
		"https://github.com/o/r/pulls?per_page=10",
		"https://github.com/o/r/pulls?per_page=101",
		"https://github.com/o/r/pulls?q=is%3Aissue",
		"https://github.com/o/r/pulls?q=is%3Apr+is%3Aissue",
		"https://github.com/o/r/pulls?q=is%3Apr&state=open",
	} {
		t.Run(raw, func(t *testing.T) {
			var calls int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			_, err := readGitHubPullList(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget(raw))
			if err == nil {
				t.Fatalf("unsupported/conflicting PR query was accepted: %s", raw)
			}
			if atomic.LoadInt32(&calls) != 0 {
				t.Fatalf("unsupported/conflicting PR query made %d provider calls: %s", calls, raw)
			}
		})
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

func TestReadGitHubPullRequestAnonymousBoundedOverview(t *testing.T) {
	var timelineCalls, reviewCalls, reviewCommentCalls, graphqlCalls int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{
                "id":123456789,"number":42,"state":"open","draft":false,"merged":false,"title":"Improve things",
                "body":"Visible PR body\n<!-- hidden automation -->\nStill visible","html_url":"https://github.com/o/r/pull/42",
                "user":{"login":"author"},"created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-05T00:00:00Z",
                "comments":2,"review_comments":3,"commits":2,"additions":50,"deletions":10,"changed_files":4,
                "head":{"label":"fork:feature","ref":"feature","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
                "base":{"label":"o:main","ref":"main","sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
            }`)
		case "/repos/o/r/issues/42":
			_, _ = io.WriteString(w, `{"id":987654321,"number":42,"title":"Improve things","body":"Visible PR body","html_url":"https://github.com/o/r/pull/42","pull_request":{"html_url":"https://github.com/o/r/pull/42"}}`)
		case "/repos/o/r/issues/42/timeline":
			atomic.AddInt32(&timelineCalls, 1)
			if r.URL.Query().Get("page") != "" {
				t.Fatal("PR overview followed timeline pagination")
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/issues/42/timeline?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[
                {"id":900,"event":"commented","body":"Normal bot comment\n<!-- hidden -->\nkept","user":{"login":"ci-bot","type":"Bot"},"author_association":"NONE","created_at":"2026-08-02T00:00:00Z","html_url":"https://github.com/o/r/pull/42#issuecomment-900"},
                {"id":500,"event":"reviewed","body":"Review duplicate body","state":"approved","user":{"login":"reviewer"},"created_at":"2026-08-02T01:00:00Z"},
                {"event":"head_ref_force_pushed","actor":{"login":"author"},"commit_id":"dddddddddddddddddddddddddddddddddddddddd","created_at":"2026-08-03T00:00:00Z"},
                {"event":"base_ref_changed","actor":{"login":"author"},"created_at":"2026-08-03T01:00:00Z"},
                {"event":"review_requested","actor":{"login":"author"},"requested_reviewer":{"login":"reviewer"},"created_at":"2026-08-03T02:00:00Z"},
                {"event":"committed","sha":"cccccccccccccccccccccccccccccccccccccccc","message":"Second commit\nmore","created_at":"2026-08-03T03:00:00Z"},
                {"event":"ready_for_review","actor":{"login":"author"},"created_at":"2026-08-04T00:00:00Z"},
                {"event":"review_request_removed","actor":{"login":"author"},"requested_reviewer":{"login":"reviewer"},"created_at":"2026-08-04T01:00:00Z"},
                {"event":"mentioned","actor":{"login":"noise"},"created_at":"2026-08-04T02:00:00Z"}
            ]`)
		case "/repos/o/r/pulls/42/reviews":
			atomic.AddInt32(&reviewCalls, 1)
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/pulls/42/reviews?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[
                {"id":500,"state":"APPROVED","body":"Review duplicate body\n<!-- review hidden -->","commit_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","submitted_at":"2026-08-02T01:00:00Z","html_url":"https://github.com/o/r/pull/42#pullrequestreview-500","user":{"login":"reviewer"},"author_association":"MEMBER"},
                {"id":501,"state":"COMMENTED","body":"","commit_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","submitted_at":"2026-08-02T03:00:00Z","html_url":"https://github.com/o/r/pull/42#pullrequestreview-501","user":{"login":"outsider"},"author_association":"NONE"}
            ]`)
		case "/repos/o/r/pulls/42/comments":
			atomic.AddInt32(&reviewCommentCalls, 1)
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/pulls/42/comments?per_page=100&page=2>; rel="next"`, server.URL))
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
		`state: "open"`, `overview: true`, `pull_request_id: 123456789`, `issue_id: 987654321`, `base: "o:main"`, `head: "fork:feature"`, `changed_files: 4`, `additions: 50`, `deletions: 10`,
		"Visible PR body", "Still visible", "Full description: https://github.com/o/r/pull/42#issue-987654321", "Comment `900` by @ci-bot", "Normal bot comment", "kept", "Selector: https://github.com/o/r/pull/42#issuecomment-900",
		"force-pushed the head ref to `dddddddddddd`", "changed the base ref", "requested review from @reviewer", "added commit `cccccccccccc` — Second commit", "marked the Pull Request ready for review", "removed review request from @reviewer",
		"Review `500` — APPROVED by @reviewer", "Review duplicate body", "Selector: https://github.com/o/r/pull/42#pullrequestreview-500", "Review `501` — COMMENTED by @outsider",
		"Thread `100` by @reviewer", "Coordinate: `a.go` line 10 right", "Root comment", "Replies: 1", "Selector: https://github.com/o/r/pull/42#discussion_r100", "Thread `200` by @outsider", "Second thread",
		"conversation_provider_more_available: true", "reviews_provider_more_available: true", "review_comments_provider_more_available: true", "thread_state_enrichment: unavailable_without_auth",
		"/pull/42/files", "/pull/42/commits", "/pull/42/checks",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("PR output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hidden automation") || strings.Contains(out, "review hidden") || strings.Contains(out, "@noise") || strings.Contains(out, "Reply from outsider") || strings.Contains(out, "#issue-123456789") {
		t.Fatalf("PR human-body sanitization/noise filtering regressed:\n%s", out)
	}
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("PR overview exceeded shared target: %d runes\n%s", got, out)
	}
	if got := atomic.LoadInt32(&timelineCalls); got != 1 {
		t.Fatalf("overview should fetch one timeline page, got %d", got)
	}
	if got := atomic.LoadInt32(&reviewCalls); got != 1 {
		t.Fatalf("overview should fetch one review page, got %d", got)
	}
	if got := atomic.LoadInt32(&reviewCommentCalls); got != 1 {
		t.Fatalf("overview should fetch one review-comment page, got %d", got)
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
		case "/repos/o/r/issues/42":
			_, _ = io.WriteString(w, `{"id":987,"number":42,"title":"x","pull_request":{"html_url":"https://github.com/o/r/pull/42"}}`)
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
			_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"isResolved":true,"isOutdated":true,"resolvedBy":{"login":"resolver"},"comments":{"nodes":[{"fullDatabaseId":"100"}]}}],"pageInfo":{"hasNextPage":true,"endCursor":"must-not-follow"}}}}}}`)
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
	if !strings.Contains(out, "State: resolved · outdated · by @resolver") || !strings.Contains(out, "thread_state_enrichment: partial_provider_page") || !strings.Contains(out, "first GraphQL page") || strings.Contains(out, "unavailable_without_auth") {
		t.Fatalf("GraphQL thread state missing:\n%s", out)
	}
	if atomic.LoadInt32(&graphQLCalls) != 1 {
		t.Fatalf("expected one GraphQL call")
	}
}

func TestPullOverviewReservesOrdinaryCommentSelectorsAheadOfStateEvents(t *testing.T) {
	timeline := []githubTimelineEvent{}
	for i := 0; i < 12; i++ {
		timeline = append(timeline, githubTimelineEvent{Event: "head_ref_force_pushed", CommitID: fmt.Sprintf("%040x", i+1), CreatedAt: fmt.Sprintf("2026-08-01T00:%02d:00Z", i)})
	}
	for i := 0; i < 3; i++ {
		id := int64(900 + i)
		body := fmt.Sprintf("comment-%d", id)
		timeline = append(timeline, githubTimelineEvent{ID: id, Event: "commented", Body: &body, HTMLURL: fmt.Sprintf("https://github.com/o/r/pull/42#issuecomment-%d", id), User: githubUser{Login: "u"}, CreatedAt: fmt.Sprintf("2026-08-02T00:%02d:00Z", i)})
	}
	pr := githubPullRequest{Number: 42, State: "open", Title: "mixed timeline", HTMLURL: "https://github.com/o/r/pull/42", Comments: 3}
	out := renderGitHubPullRequest(&GitHubTarget{Owner: "o", Repo: "r", Number: 42}, pr, 987, timeline, nil, nil, githubPullOverviewAvailability{}, "", false)
	for i := 0; i < 3; i++ {
		id := 900 + i
		if !strings.Contains(out, fmt.Sprintf("Comment `%d`", id)) || !strings.Contains(out, fmt.Sprintf("#issuecomment-%d", id)) {
			t.Fatalf("ordinary comment %d was crowded out by state events:\n%s", id, out)
		}
	}
	if !strings.Contains(out, "conversation_comments_indexed: 3") || !strings.Contains(out, "conversation_events_omitted:") {
		t.Fatalf("split conversation/event accounting missing:\n%s", out)
	}
}

func TestPullGraphQLEnrichmentFailurePreservesRESTCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/pulls/42":
			_, _ = io.WriteString(w, `{"number":42,"state":"open","title":"x"}`)
		case "/repos/o/r/issues/42":
			_, _ = io.WriteString(w, `{"id":987,"number":42,"title":"x","pull_request":{"html_url":"https://github.com/o/r/pull/42"}}`)
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

func TestPullBodySelectorUsesIssueSideIDAndOneProviderRead(t *testing.T) {
	longBody := "Exact Pull Request description\n\n" + strings.Repeat("selected PR body stays complete even when the root would preview it\n", 180)
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if r.URL.Path != "/repos/o/r/issues/42" {
			t.Fatalf("PR body selector fetched unrelated endpoint: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 987, "number": 42, "title": "Body selector", "body": longBody,
			"html_url": "https://github.com/o/r/pull/42", "user": map[string]any{"login": "alice"},
			"pull_request": map[string]any{"html_url": "https://github.com/o/r/pull/42"},
		})
	}))
	defer server.Close()

	out, err := readGitHubPullRequest(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42#issue-987"))
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("exact PR body selector should use one Issue-side provider read, got %d", got)
	}
	for _, want := range []string{"pull_request: 42", "issue_id: 987", "url: \"https://github.com/o/r/pull/42#issue-987\"", strings.TrimSpace(longBody)} {
		if !strings.Contains(out, want) {
			t.Fatalf("exact PR body selector missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "overview: true") || strings.Contains(out, "Conversation index") || strings.Contains(out, "preview truncated") {
		t.Fatalf("exact PR body selector expanded or truncated parent context:\n%s", out)
	}

	atomic.StoreInt32(&requests, 0)
	_, err = readGitHubPullRequest(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/pull/42#issue-123"))
	if err == nil || !strings.Contains(err.Error(), "does not belong") || !strings.Contains(err.Error(), "Issue-side id 987") {
		t.Fatalf("mismatched PR body selector was not rejected truthfully: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("mismatched PR body selector should stop after one identity read, got %d", got)
	}
}

func TestLargePullRootOverviewIsBoundedAndEveryIndexedChildHasSelector(t *testing.T) {
	body := "Opening PR description context.\n\n" + strings.Repeat("Description paragraph that should be previewed safely.\n\n", 30) + "```go\n" + strings.Repeat("fmt.Println(\"inside PR fence\")\n", 70) + "```\n\nTAIL PR BODY MUST NOT APPEAR"
	timeline := make([]githubTimelineEvent, 0, 20)
	reviews := make([]githubPullReview, 0, 12)
	threads := make([]githubPullThread, 0, 12)
	for i := 1; i <= 20; i++ {
		comment := fmt.Sprintf("Conversation %02d marker. %s TAIL CONVERSATION %02d MUST NOT APPEAR", i, strings.Repeat("comment detail. ", 50), i)
		timeline = append(timeline, githubTimelineEvent{
			ID: int64(i), Event: "commented", Body: stringPtr(comment), HTMLURL: fmt.Sprintf("https://github.com/o/r/pull/42#issuecomment-%d", i),
			User: githubUser{Login: fmt.Sprintf("commenter-%02d", i)}, CreatedAt: fmt.Sprintf("2026-08-%02dT00:00:00Z", (i%28)+1),
		})
	}
	for i := 1; i <= 12; i++ {
		reviewID := int64(1000 + i)
		reviewBody := fmt.Sprintf("Review %02d marker. %s TAIL REVIEW %02d MUST NOT APPEAR", i, strings.Repeat("review detail. ", 50), i)
		reviews = append(reviews, githubPullReview{
			ID: reviewID, State: "APPROVED", Body: stringPtr(reviewBody), HTMLURL: fmt.Sprintf("https://github.com/o/r/pull/42#pullrequestreview-%d", reviewID),
			User: githubUser{Login: fmt.Sprintf("reviewer-%02d", i)}, SubmittedAt: fmt.Sprintf("2026-08-%02dT01:00:00Z", (i%28)+1),
		})
		threadID := int64(2000 + i)
		line := i
		threadBody := fmt.Sprintf("Thread %02d marker. %s TAIL THREAD %02d MUST NOT APPEAR", i, strings.Repeat("thread detail. ", 50), i)
		threads = append(threads, githubPullThread{Root: githubPullReviewComment{
			ID: threadID, Body: stringPtr(threadBody), HTMLURL: fmt.Sprintf("https://github.com/o/r/pull/42#discussion_r%d", threadID), Path: "internal/large.go", Line: &line, Side: "RIGHT",
			User: githubUser{Login: fmt.Sprintf("threader-%02d", i)}, CreatedAt: fmt.Sprintf("2026-08-%02dT02:00:00Z", (i%28)+1),
		}})
	}
	pr := githubPullRequest{
		ID: 123, Number: 42, State: "open", Title: "Pathologically large PR", Body: stringPtr(body), HTMLURL: "https://github.com/o/r/pull/42",
		User: githubUser{Login: "author"}, Comments: 25, ReviewComments: 30, Commits: 9, ChangedFiles: 18, Additions: 1200, Deletions: 450,
		Head: githubPullRef{Label: "fork:feature"}, Base: githubPullRef{Label: "o:main"},
	}
	availability := githubPullOverviewAvailability{TimelineProviderMore: true, ReviewsProviderMore: true, ReviewCommentsProviderMore: true, ReviewCommentsReturned: 24}
	out := renderGitHubPullRequest(&GitHubTarget{Owner: "o", Repo: "r", Number: 42}, pr, 987, timeline, reviews, threads, availability, "", false)
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("large PR overview exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{
		"overview: true", "pull_request_id: 123", "issue_id: 987", "Opening PR description context.", "Description preview locally truncated",
		"Full description: https://github.com/o/r/pull/42#issue-987", "locally omitted from this overview",
		"conversation_provider_more_available: true", "reviews_provider_more_available: true", "review_comments_provider_more_available: true",
		"Conversation 01 marker", "Review 01 marker", "Thread 01 marker", "/pull/42/files", "/pull/42/commits", "/pull/42/checks",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("large PR overview missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"TAIL PR BODY MUST NOT APPEAR", "TAIL CONVERSATION 01 MUST NOT APPEAR", "TAIL REVIEW 01 MUST NOT APPEAR", "TAIL THREAD 01 MUST NOT APPEAR"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("large PR overview leaked full subordinate content %q:\n%s", forbidden, out)
		}
	}
	assertPullOverviewChildSelectors(t, out)
	previewStart := strings.Index(out, "## Description preview\n\n")
	previewEnd := strings.Index(out, "\n\n> Description preview locally truncated")
	if previewStart < 0 || previewEnd <= previewStart {
		t.Fatalf("could not locate PR description preview boundaries:\n%s", out)
	}
	preview := out[previewStart+len("## Description preview\n\n") : previewEnd]
	if strings.Count(preview, "```")%2 != 0 {
		t.Fatalf("PR description preview ended inside a Markdown fence:\n%s", preview)
	}
}

func assertPullOverviewChildSelectors(t *testing.T, out string) {
	t.Helper()
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		var fragmentPrefix string
		switch {
		case strings.HasPrefix(line, "### Comment `"):
			fragmentPrefix = "#issuecomment-"
		case strings.HasPrefix(line, "### Review `"):
			fragmentPrefix = "#pullrequestreview-"
		case strings.HasPrefix(line, "### Thread `"):
			fragmentPrefix = "#discussion_r"
		default:
			continue
		}
		parts := strings.Split(line, "`")
		if len(parts) < 3 || parts[1] == "" {
			t.Fatalf("indexed child lacks stable ID: %q", line)
		}
		want := fragmentPrefix + parts[1]
		found := false
		for j := i + 1; j < len(lines) && !strings.HasPrefix(lines[j], "### "); j++ {
			if strings.HasPrefix(lines[j], "Selector: ") && strings.HasSuffix(lines[j], want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("indexed child %q lacks matching exact selector %q:\n%s", line, want, out)
		}
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
	if err := client.GraphQL(context.Background(), "query{viewer{login}}", nil, &struct{}{}); err == nil || !strings.Contains(err.Error(), "authentication is required") {
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
