package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

func TestParseGitHubIssueTargets(t *testing.T) {
	tests := []struct {
		rawURL   string
		kind     GitHubTargetKind
		number   int
		name     string
		fragment string
	}{
		{rawURL: "https://github.com/o/r/issues", kind: GitHubTargetIssueList},
		{rawURL: "https://github.com/o/r/issues?q=is%3Aissue+state%3Aclosed", kind: GitHubTargetIssueList},
		{rawURL: "https://github.com/o/r/issues/42", kind: GitHubTargetIssue, number: 42},
		{rawURL: "https://github.com/o/r/issues/42#issue-987", kind: GitHubTargetIssue, number: 42, fragment: "issue-987"},
		{rawURL: "https://github.com/o/r/issues/42#issuecomment-123", kind: GitHubTargetIssue, number: 42, fragment: "issuecomment-123"},
		{rawURL: "https://github.com/o/r/labels", kind: GitHubTargetLabelList},
		{rawURL: "https://github.com/o/r/labels/good%20first%20issue", kind: GitHubTargetLabel, name: "good first issue"},
		{rawURL: "https://github.com/o/r/milestones", kind: GitHubTargetMilestones},
		{rawURL: "https://github.com/o/r/milestone/7", kind: GitHubTargetMilestone, number: 7},
	}
	for _, tt := range tests {
		t.Run(tt.rawURL, func(t *testing.T) {
			target := parseGitHubTarget(tt.rawURL)
			if target == nil {
				t.Fatal("expected native target")
			}
			if target.Kind != tt.kind || target.Number != tt.number || target.Name != tt.name || target.Fragment != tt.fragment {
				t.Fatalf("unexpected target: %#v", target)
			}
		})
	}

	for _, rawURL := range []string{
		"https://github.com/o/r/issues/not-a-number",
		"https://github.com/o/r/issues/1/extra",
		"https://github.com/o/r/milestone/not-a-number",
		"https://github.com/o/r/security/dependabot",
	} {
		if target := parseGitHubTarget(rawURL); target != nil {
			t.Fatalf("expected %s to remain unsupported, got %#v", rawURL, target)
		}
	}
}

func TestReadGitHubIssueStopsTimelinePaginationWhenProviderHasMore(t *testing.T) {
	var timelinePages int32
	var subIssuePages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/issues/42":
			_, _ = io.WriteString(w, `{
                "number":42,"state":"closed","state_reason":"completed","title":"A real issue",
                "body":"Visible body\n<!-- automation marker -->\nStill visible","html_url":"https://github.com/o/r/issues/42",
                "user":{"login":"alice"},"author_association":"MEMBER",
                "labels":[{"name":"bug"},{"name":"priority/high"}],"assignees":[{"login":"bob"}],
                "milestone":{"number":7,"title":"v2","state":"open"},"locked":true,"active_lock_reason":"too heated",
                "comments":2,"created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-02T00:00:00Z","closed_at":"2026-08-03T00:00:00Z",
                "type":{"name":"Bug"},
                "pinned_comment":{"id":2,"body":"Pinned answer","html_url":"https://github.com/o/r/issues/42#issuecomment-2","user":{"login":"alice"},"created_at":"2026-08-02T12:00:00Z"}
            }`)
		case "/repos/o/r/issues/42/timeline":
			atomic.AddInt32(&timelinePages, 1)
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `[
                    {"id":2,"event":"commented","body":"Pinned answer","html_url":"https://github.com/o/r/issues/42#issuecomment-2","user":{"login":"alice"},"created_at":"2026-08-02T12:00:00Z"},
                    {"event":"cross-referenced","actor":{"login":"carol"},"created_at":"2026-08-02T13:00:00Z","source":{"issue":{"number":55,"title":"Related","html_url":"https://github.com/o/r/issues/55"}}},
                    {"event":"renamed","actor":{"login":"alice"},"created_at":"2026-08-02T14:00:00Z","rename":{"from":"Old","to":"A real issue"}},
                    {"event":"subscribed","actor":{"login":"noise"},"created_at":"2026-08-02T15:00:00Z"}
                ]`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/issues/42/timeline?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[
                {"id":1,"event":"commented","body":"Bot content\n<!-- hidden -->\nkept","html_url":"https://github.com/o/r/issues/42#issuecomment-1","user":{"login":"build-bot","type":"Bot"},"author_association":"NONE","created_at":"2026-08-02T10:00:00Z"},
                {"event":"labeled","actor":{"login":"alice"},"created_at":"2026-08-02T11:00:00Z","label":{"name":"bug"}}
            ]`)
		case "/repos/o/r/issues/42/parent":
			_, _ = io.WriteString(w, `{"number":10,"title":"Parent","html_url":"https://github.com/o/r/issues/10"}`)
		case "/repos/o/r/issues/42/sub_issues":
			atomic.AddInt32(&subIssuePages, 1)
			if r.URL.Query().Get("page") != "" {
				t.Fatalf("Issue root followed relationship pagination: %s", r.URL.RawQuery)
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/issues/42/sub_issues?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"number":43,"title":"Child","html_url":"https://github.com/o/r/issues/43"}]`)
		case "/repos/o/r/issues/42/dependencies/blocked_by":
			_, _ = io.WriteString(w, `[{"number":41,"title":"Blocker","html_url":"https://github.com/o/r/issues/41"}]`)
		case "/repos/o/r/issues/42/dependencies/blocking":
			_, _ = io.WriteString(w, `[{"number":50,"title":"Downstream","html_url":"https://github.com/o/r/issues/50"}]`)
		case "/repos/o/r/issues/42/issue-field-values":
			_, _ = io.WriteString(w, `[
                {"issue_field_name":"Priority","data_type":"single_select","value":1,"single_select_option":{"name":"High"}},
                {"issue_field_name":"Points","data_type":"number","value":8}
            ]`)
		default:
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()

	target := parseGitHubTarget("https://github.com/o/r/issues/42")
	out, err := readGitHubIssue(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`state: "closed"`, `state_reason: "completed"`, `locked: true`, `lock_reason: "too heated"`,
		`labels: ["bug","priority/high"]`, `assignees: ["@bob"]`, `milestone: "v2"`, `type: "Bug"`,
		"overview: true", "timeline_provider_more_available: true", "relationships_provider_more_available: true", "comments_provider_complete: false",
		"Visible body", "Still visible", "Comment `1` by @build-bot", "Bot content", "kept", "pinned", "Pinned answer",
		"Parent: [#10 Parent]", "Sub-issue: [#43 Child]", "Blocked by: [#41 Blocker]", "Blocking: [#50 Downstream]",
		"Priority: High", "Points: 8", "More timeline pages may exist upstream", "More Issue relationship entries exist upstream",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "automation marker") || strings.Contains(out, "<!-- hidden -->") || strings.Contains(out, "@noise") {
		t.Fatalf("human-view sanitization/noise filtering regressed:\n%s", out)
	}
	if strings.Count(out, "Pinned answer") != 1 {
		t.Fatalf("pinned comment duplicated instead of being annotated in timeline:\n%s", out)
	}
	if strings.Contains(out, "cross-referenced [#55 Related]") || strings.Contains(out, "renamed the Issue") {
		t.Fatalf("Issue root followed later timeline pages despite bounded overview contract:\n%s", out)
	}
	if got := atomic.LoadInt32(&timelinePages); got != 1 {
		t.Fatalf("expected one bounded timeline page, got %d", got)
	}
	if got := atomic.LoadInt32(&subIssuePages); got != 1 {
		t.Fatalf("expected one bounded relationship page, got %d", got)
	}
	if strings.Contains(out, "GH_TOKEN") {
		t.Fatalf("successful anonymous Issue read nagged for auth:\n%s", out)
	}
}

func TestIssueTimelineMinimizedShapeVariants(t *testing.T) {
	var events []githubTimelineEvent
	payload := `[
		{"id":1,"event":"commented","minimized":false,"body":"visible"},
		{"id":2,"event":"commented","minimized":null,"body":"visible"},
		{"id":3,"event":"commented","minimized":true,"minimized_reason":"outdated","body":"hidden"},
		{"id":4,"event":"commented","minimized":{"reason":"spam"},"minimized_reason":null,"body":"hidden"},
		{"id":5,"event":"commented","minimized":{"reason":{"future":"shape"}},"body":"hidden"},
		{"id":6,"event":"commented","minimized":"future-shape","body":"visible"}
	]`
	if err := json.Unmarshal([]byte(payload), &events); err != nil {
		t.Fatalf("polymorphic minimized data must not erase the timeline: %v", err)
	}
	if len(events) != 6 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
	if events[0].Minimized || events[1].Minimized {
		t.Fatal("false/null minimized states should remain visible")
	}
	if !events[2].Minimized || events[2].MinimizedReason != "outdated" {
		t.Fatalf("boolean minimized state lost reason: %#v", events[2])
	}
	if !events[3].Minimized || events[3].MinimizedReason != "spam" {
		t.Fatalf("object minimized state was not normalized: %#v", events[3])
	}
	if !events[4].Minimized || events[4].MinimizedReason != "" {
		t.Fatalf("unknown object reason shape should remain minimized without inventing a reason: %#v", events[4])
	}
	if events[5].Minimized {
		t.Fatalf("unknown scalar minimized shape should be tolerated without inventing minimized=true: %#v", events[5])
	}
}

func TestIssueBodySelectorUsesOneProviderReadAndVerifiesIdentity(t *testing.T) {
	longBody := "Full exact description\n\n" + strings.Repeat("selected body text stays faithful even when long\n", 220)
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if r.URL.Path != "/repos/o/r/issues/42" {
			t.Fatalf("body selector fetched unrelated Issue data: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 987, "number": 42, "state": "open", "title": "Body selector", "body": longBody,
			"html_url": "https://github.com/o/r/issues/42", "user": map[string]any{"login": "alice"},
		})
	}))
	defer server.Close()

	out, err := readGitHubIssue(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/issues/42#issue-987"))
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("exact Issue-body selector should use one provider read, got %d", got)
	}
	if !strings.Contains(out, strings.TrimSpace(longBody)) || strings.Contains(out, "Timeline") || strings.Contains(out, "preview truncated") {
		t.Fatalf("exact Issue-body selector was not faithful/narrow:\n%s", out)
	}

	atomic.StoreInt32(&requests, 0)
	_, err = readGitHubIssue(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/issues/42#issue-986"))
	if err == nil || !strings.Contains(err.Error(), "does not belong") || !strings.Contains(err.Error(), "Issue id 987") {
		t.Fatalf("mismatched Issue-body selector was not rejected truthfully: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("mismatched Issue-body selector should stop after identity read, got %d", got)
	}
}

func TestLargeIssueUsesBoundedOverviewWithSelectorsAndTruthfulOmission(t *testing.T) {
	body := "Opening context that must survive.\n\n" + strings.Repeat("Body paragraph with enough detail to force a bounded preview.\n\n", 35) + "```go\n" + strings.Repeat("fmt.Println(\"inside body fence\")\n", 80) + "```\n\nTAIL BODY MUST NOT APPEAR"
	comments := make([]map[string]any, 0, 30)
	for i := 1; i <= 30; i++ {
		commentBody := fmt.Sprintf("Comment %02d preview marker.\n\n%s\nTAIL COMMENT %02d MUST NOT APPEAR", i, strings.Repeat("long comment paragraph with deterministic context. ", 24), i)
		if i == 2 {
			commentBody = "Pinned answer marker.\n\n" + strings.Repeat("pinned detail. ", 80)
		}
		comments = append(comments, map[string]any{
			"id": i, "event": "commented", "body": commentBody,
			"html_url":   fmt.Sprintf("https://github.com/o/r/issues/42#issuecomment-%d", i),
			"user":       map[string]any{"login": fmt.Sprintf("user-%02d", i)},
			"created_at": fmt.Sprintf("2026-08-%02dT00:00:00Z", (i%28)+1),
			"is_pinned":  i == 2,
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/issues/42":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 987, "number": 42, "state": "open", "title": "Very large Issue", "body": body,
				"html_url": "https://github.com/o/r/issues/42", "user": map[string]any{"login": "alice"},
				"comments":       35,
				"pinned_comment": map[string]any{"id": 2, "body": "Pinned answer marker.", "html_url": "https://github.com/o/r/issues/42#issuecomment-2", "user": map[string]any{"login": "user-02"}},
			})
		case "/repos/o/r/issues/42/timeline":
			_ = json.NewEncoder(w).Encode(comments)
		case "/repos/o/r/issues/42/parent":
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 10, "title": "Parent", "html_url": "https://github.com/o/r/issues/10"})
		case "/repos/o/r/issues/42/sub_issues":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"number": 43, "title": "Child", "html_url": "https://github.com/o/r/issues/43"}})
		case "/repos/o/r/issues/42/dependencies/blocked_by", "/repos/o/r/issues/42/dependencies/blocking", "/repos/o/r/issues/42/issue-field-values":
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	out, err := readGitHubIssue(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/issues/42"))
	if err != nil {
		t.Fatal(err)
	}
	runes := utf8.RuneCountInString(out)
	t.Logf("large Issue overview runes: %d", runes)
	if runes > githubOverviewRunes {
		t.Fatalf("large Issue overview exceeded shared target: %d runes\n%s", runes, out)
	}
	for _, want := range []string{
		"overview: true", "Opening context that must survive.", "Description preview locally truncated",
		"https://github.com/o/r/issues/42#issue-987", "Comment `1` by @user-01",
		"https://github.com/o/r/issues/42#issuecomment-1", "Pinned answer marker", "Parent: [#10 Parent]", "Sub-issue: [#43 Child]",
		"timeline_items_omitted:", "locally omitted from this overview", "comments_provider_complete: false", "Provider-incomplete comment data",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("bounded overview missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "TAIL BODY MUST NOT APPEAR") || strings.Contains(out, "TAIL COMMENT 01 MUST NOT APPEAR") {
		t.Fatalf("overview leaked full subordinate bodies:\n%s", out)
	}
	bodyPreviewStart := strings.Index(out, "## Body preview\n\n")
	if bodyPreviewStart < 0 {
		t.Fatalf("could not locate body preview boundaries:\n%s", out)
	}
	bodyPreviewEnd := strings.Index(out[bodyPreviewStart:], "\n\n> Description preview locally truncated")
	if bodyPreviewEnd < 0 {
		t.Fatalf("could not locate body preview end:\n%s", out)
	}
	preview := out[bodyPreviewStart+len("## Body preview\n\n") : bodyPreviewStart+bodyPreviewEnd]
	if strings.Count(preview, "```")%2 != 0 {
		t.Fatalf("body preview ended inside a Markdown fence:\n%s", preview)
	}
}

func TestReadGitHubIssueEmptyLockedAndUnavailableCommentBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/issues/8":
			_, _ = io.WriteString(w, `{"number":8,"state":"open","title":"Empty","body":null,"locked":true,"comments":1,"user":{"login":"u"}}`)
		case "/repos/o/r/issues/8/timeline":
			_, _ = io.WriteString(w, `[{"id":5,"event":"commented","body":null,"user":{"login":"gone"},"created_at":"2026-08-01T00:00:00Z"}]`)
		case "/repos/o/r/issues/8/parent", "/repos/o/r/issues/8/issue-field-values":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		case "/repos/o/r/issues/8/sub_issues", "/repos/o/r/issues/8/dependencies/blocked_by", "/repos/o/r/issues/8/dependencies/blocking":
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubIssue(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/issues/8"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "_No description provided._") || !strings.Contains(out, "_Comment body is unavailable or deleted._") || !strings.Contains(out, "locked: true") {
		t.Fatalf("ordinary empty/locked/deleted state malformed:\n%s", out)
	}
}

func TestIssueCommentSelectorUsesOneProviderRead(t *testing.T) {
	var requests int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if r.URL.Path != "/repos/o/r/issues/comments/99" {
			t.Fatalf("selector fetched unrelated Issue data: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
            "id":99,"issue_url":"`+server.URL+`/repos/o/r/issues/42","html_url":"https://github.com/o/r/issues/42#issuecomment-99",
            "body":"Selected\n<!-- hidden -->\ncomment","user":{"login":"alice"},"author_association":"MEMBER",
            "created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-01T01:00:00Z","is_pinned":true
        }`)
	}))
	defer server.Close()
	target := parseGitHubTarget("https://github.com/o/r/issues/42#issuecomment-99")
	out, err := readGitHubIssue(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("exact selector should use one provider read, got %d", got)
	}
	if !strings.Contains(out, "Selected\n\ncomment") || strings.Contains(out, "hidden") || !strings.Contains(out, "pinned: true") {
		t.Fatalf("unexpected targeted comment output:\n%s", out)
	}
}

func TestIssueCommentSelectorRejectsCommentFromOtherIssue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":99,"issue_url":"https://api.github.com/repos/o/r/issues/41","body":"wrong"}`)
	}))
	defer server.Close()
	_, err := readGitHubIssue(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/issues/42#issuecomment-99"))
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("expected ownership error, got %v", err)
	}
}

func TestIssueListFiltersPullRequestsAndPreservesFiltersAndNavigation(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/issues" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		for key, want := range map[string]string{"state": "closed", "labels": "bug,ui", "sort": "updated", "direction": "asc", "page": "2"} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("query %s: got %q want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/issues?state=closed&labels=bug%%2Cui&page=1>; rel="prev", <%s/repos/o/r/issues?state=closed&labels=bug%%2Cui&page=3>; rel="next"`, server.URL, server.URL))
		_, _ = io.WriteString(w, `[
            {"number":20,"state":"closed","title":"Actual Issue","html_url":"https://github.com/o/r/issues/20","user":{"login":"alice"},"updated_at":"2026-08-01T00:00:00Z","labels":[{"name":"bug"}]},
            {"number":21,"state":"closed","title":"A PR","html_url":"https://github.com/o/r/pull/21","pull_request":{"html_url":"https://github.com/o/r/pull/21"}}
        ]`)
	}))
	defer server.Close()
	target := parseGitHubTarget("https://github.com/o/r/issues?state=closed&labels=bug%2Cui&sort=updated&direction=asc&page=2")
	out, err := readGitHubIssueList(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "#20 Actual Issue") || strings.Contains(out, "A PR") {
		t.Fatalf("Issue list failed PR exclusion:\n%s", out)
	}
	if !strings.Contains(out, "Previous: https://github.com/o/r/issues?") || !strings.Contains(out, "page=1") || !strings.Contains(out, "Next: https://github.com/o/r/issues?") || !strings.Contains(out, "page=3") {
		t.Fatalf("bounded page navigation missing:\n%s", out)
	}
	if !strings.Contains(out, "labels=bug%2Cui") || !strings.Contains(out, "state=closed") {
		t.Fatalf("UI filters were not preserved in navigation:\n%s", out)
	}
}

func TestIssueSearchUsesSearchResourceAndSurfacesIncompleteResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/issues" {
			t.Fatalf("unexpected search path: %s", r.URL.Path)
		}
		q := r.URL.Query().Get("q")
		if !strings.Contains(q, "repo:o/r") || !strings.Contains(q, "label:bug") || !strings.Contains(q, "is:issue") {
			t.Errorf("search qualifiers missing from %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Resource", "search")
		_, _ = io.WriteString(w, `{"total_count":90,"incomplete_results":true,"items":[
            {"number":1,"state":"open","title":"Issue","html_url":"https://github.com/o/r/issues/1"},
            {"number":2,"state":"open","title":"PR","pull_request":{"html_url":"https://github.com/o/r/pull/2"}}
        ]}`)
	}))
	defer server.Close()
	target := parseGitHubTarget("https://github.com/o/r/issues?q=is%3Aissue+label%3Abug&page=2")
	out, err := readGitHubIssueList(context.Background(), testGitHubClient(server.URL, server.URL, ""), target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `query: "is:issue label:bug"`) || !strings.Contains(out, "total_matches: 90") || !strings.Contains(out, "complete: false") || !strings.Contains(out, "marked this search result set as incomplete") {
		t.Fatalf("search truth missing:\n%s", out)
	}
	if strings.Contains(out, "PR") {
		t.Fatalf("search result leaked PR into Issues view:\n%s", out)
	}
}

func TestGitHubSearchResourceQualifierIsTokenAwareAndRejectsConflict(t *testing.T) {
	for _, tt := range []struct {
		query string
		want  string
	}{
		{query: "bug is:pr is:open", want: "pull_request"},
		{query: "bug IS:ISSUE", want: "issue"},
		{query: `"is:pr" bug`, want: ""},
		{query: `label:"is:pr" bug`, want: ""},
		{query: "this-is:pr bug", want: ""},
	} {
		got, err := githubSearchResourceQualifier(tt.query)
		if err != nil || got != tt.want {
			t.Fatalf("githubSearchResourceQualifier(%q)=%q,%v want %q,nil", tt.query, got, err, tt.want)
		}
	}
	if _, err := githubSearchResourceQualifier("is:issue bug is:pr"); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("conflicting explicit resource qualifiers were not rejected: %v", err)
	}
}

func TestIssueSearchConflictingResourceQualifiersFailBeforeProvider(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	_, err := readGitHubIssueList(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/issues?q=is%3Aissue+is%3Apr"))
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("conflicting Issue/PR search was not rejected truthfully: %v", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("conflicting search made %d provider calls", calls)
	}
}

func TestIssueRecognizedProviderFailuresStayNative(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		head   map[string]string
		want   string
	}{
		{name: "not found", status: http.StatusNotFound, want: "may be private"},
		{name: "forbidden", status: http.StatusForbidden, want: "denied access"},
		{name: "rate", status: http.StatusTooManyRequests, head: map[string]string{"Retry-After": "60", "X-RateLimit-Resource": "core"}, want: "rate limit exceeded"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, value := range tt.head {
					w.Header().Set(key, value)
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"message":"provider response"}`)
			}))
			defer server.Close()
			result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/o/r/issues/1"), testGitHubClient(server.URL, server.URL, ""))
			if result.Outcome != GitHubNativeFailure || result.Err == nil || !strings.Contains(strings.ToLower(result.Err.Error()), tt.want) {
				t.Fatalf("unexpected native failure: %#v", result)
			}
		})
	}
}

func TestIssueURLPointingToPullRequestDoesNotConsumePRConversation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/issues/5" {
			t.Fatalf("reader should stop after Issue identity reveals PR, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"number":5,"title":"PR identity","pull_request":{"html_url":"https://github.com/o/r/pull/5"}}`)
	}))
	defer server.Close()
	_, err := readGitHubIssue(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/issues/5"))
	if err == nil || !strings.Contains(err.Error(), "pull request") || !strings.Contains(err.Error(), "/pull/5") {
		t.Fatalf("expected PR ownership boundary, got %v", err)
	}
}

func TestLabelAndMilestoneReadersStayBoundedAndFilterPRs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/labels/bug":
			_, _ = io.WriteString(w, `{"name":"bug","color":"ff0000","description":"Something broke"}`)
		case "/repos/o/r/issues":
			if label := r.URL.Query().Get("labels"); label != "" && label != "bug" {
				t.Errorf("unexpected label filter: %q", label)
			}
			_, _ = io.WriteString(w, `[
                    {"number":1,"state":"open","title":"Issue","html_url":"https://github.com/o/r/issues/1"},
                    {"number":2,"state":"open","title":"PR","pull_request":{"html_url":"https://github.com/o/r/pull/2"}}
                ]`)
		case "/repos/o/r/milestones/7":
			_, _ = io.WriteString(w, `{"number":7,"state":"open","title":"v2","description":"Ship it","open_issues":1,"closed_issues":2,"due_on":"2026-09-01T00:00:00Z"}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := testGitHubClient(server.URL, server.URL, "")
	labelOut, err := readGitHubLabel(context.Background(), client, parseGitHubTarget("https://github.com/o/r/labels/bug"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(labelOut, "Something broke") || !strings.Contains(labelOut, "#1 Issue") || strings.Contains(labelOut, "PR") || !strings.Contains(labelOut, "Filtered Issues") {
		t.Fatalf("label output incorrect:\n%s", labelOut)
	}
	milestoneOut, err := readGitHubMilestone(context.Background(), client, parseGitHubTarget("https://github.com/o/r/milestone/7"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(milestoneOut, "Milestone: v2") || !strings.Contains(milestoneOut, "Ship it") || !strings.Contains(milestoneOut, "#1 Issue") || strings.Contains(milestoneOut, "PR") {
		t.Fatalf("milestone output incorrect:\n%s", milestoneOut)
	}
}

func TestLabelAndMilestoneListPaginationUsesUIURLs(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/labels":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/labels?page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"name":"bug","description":"Bug label"}]`)
		case "/repos/o/r/milestones":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/milestones?page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"number":7,"state":"open","title":"v2","open_issues":1,"closed_issues":2}]`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := testGitHubClient(server.URL, server.URL, "")
	labels, err := readGitHubLabelList(context.Background(), client, parseGitHubTarget("https://github.com/o/r/labels"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(labels, "Bug label") || !strings.Contains(labels, "Next: https://github.com/o/r/labels?page=2") {
		t.Fatalf("label list navigation incorrect:\n%s", labels)
	}
	milestones, err := readGitHubMilestones(context.Background(), client, parseGitHubTarget("https://github.com/o/r/milestones?state=open"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(milestones, "v2") || !strings.Contains(milestones, "Next: https://github.com/o/r/milestones?page=2&state=open") {
		t.Fatalf("milestone list navigation incorrect:\n%s", milestones)
	}
}

func TestGitHubRESTPagesRejectsPaginationCycle(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", fmt.Sprintf(`<%s/items>; rel="next"`, server.URL))
		_, _ = io.WriteString(w, `[]`)
	}))
	defer server.Close()
	_, err := testGitHubClient(server.URL, server.URL, "").RESTPages(context.Background(), "/items", "")
	if err == nil || !strings.Contains(err.Error(), "pagination returned a cycle") {
		t.Fatalf("expected pagination cycle error, got %v", err)
	}
}

func TestPageFromGitHubLink(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		page int
		ok   bool
	}{
		{raw: "https://api.github.com/x?page=3", page: 3, ok: true},
		{raw: "https://api.github.com/x", ok: false},
		{raw: "not a url%", ok: false},
	} {
		page, ok := pageFromGitHubLink(tt.raw)
		if page != tt.page || ok != tt.ok {
			t.Fatalf("pageFromGitHubLink(%q) = %d,%v want %d,%v", tt.raw, page, ok, tt.page, tt.ok)
		}
	}
}

func TestIssueSearchQueryRetainsUserText(t *testing.T) {
	target := parseGitHubTarget("https://github.com/o/r/issues?q=is%3Aissue+label%3A%22good+first+issue%22&page=2")
	if target == nil || target.Query.Get("q") != `is:issue label:"good first issue"` {
		t.Fatalf("query parse changed: %#v", target)
	}
	parsed, _ := url.Parse(githubTargetPageURL(target, 3))
	if parsed.Query().Get("q") != target.Query.Get("q") || parsed.Query().Get("page") != "3" {
		t.Fatalf("UI navigation lost query: %s", parsed.String())
	}
}

func TestRenderIssueTimelineMinimizedComment(t *testing.T) {
	body := "secret body"
	lines := renderTimelineComment(githubIssueComment{Body: &body, User: githubUser{Login: "u"}, Minimized: true, MinimizedReason: "outdated"})
	out := strings.Join(lines, "\n")
	if strings.Contains(out, "secret body") || !strings.Contains(out, "minimized by GitHub (outdated)") {
		t.Fatalf("minimized state not truthful: %s", out)
	}
}

func TestRenderIssueFieldValues(t *testing.T) {
	value := githubIssueFieldValue{IssueFieldName: "Tags", Value: []any{"a", "b"}}
	if got := renderIssueFieldValue(value); got != `["a","b"]` {
		t.Fatalf("unexpected field value: %q", got)
	}
}

func TestCopySelectedQueryDoesNotForwardUnknownUIParams(t *testing.T) {
	source := url.Values{"state": []string{"open"}, "evil": []string{"x"}, "labels": []string{"bug"}}
	got := copySelectedQuery(source, []string{"state", "labels"})
	if got.Get("state") != "open" || got.Get("labels") != "bug" || got.Get("evil") != "" {
		t.Fatalf("unexpected selected query: %#v", got)
	}
}

func TestIssueErrorDoesNotExposePrivateBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"private body: do-not-print"}`)
	}))
	defer server.Close()
	client := testGitHubClient(server.URL, server.URL, "fake-private-token")
	_, err := readGitHubIssue(context.Background(), client, parseGitHubTarget("https://github.com/o/r/issues/1"))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "do-not-print") || strings.Contains(err.Error(), "fake-private-token") {
		t.Fatalf("private provider content leaked: %q", err)
	}
}

func TestIssueListSearchRateErrorReportsSearchResource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Resource", "search")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"API rate limit exceeded"}`)
	}))
	defer server.Close()
	_, err := readGitHubIssueList(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/issues?q=is%3Aissue"))
	if err == nil || !strings.Contains(err.Error(), "Resource: search") {
		t.Fatalf("search quota truth missing: %v", err)
	}
}

func TestIssueCommentSelectorParsing(t *testing.T) {
	if id, ok, err := parseIssueCommentSelector("issuecomment-123"); !ok || err != nil || id != 123 {
		t.Fatalf("valid selector failed: %d %v %v", id, ok, err)
	}
	if _, ok, err := parseIssueCommentSelector("issuecomment-nope"); !ok || err == nil {
		t.Fatalf("invalid selector not rejected: %v %v", ok, err)
	}
	if _, ok, err := parseIssueCommentSelector("discussion_r123"); ok || err != nil {
		t.Fatalf("non-Issue selector should be unclaimed: %v %v", ok, err)
	}
}

func TestIssueTimelineKeepsUnknownPotentiallySubstantiveEvents(t *testing.T) {
	line, ok := renderIssueTimelineState(githubTimelineEvent{Event: "future_meaningful_event", Actor: githubUser{Login: "alice"}, CreatedAt: "2026-08-01T00:00:00Z"})
	if !ok || !strings.Contains(line, "future meaningful event") || !strings.Contains(line, "@alice") {
		t.Fatalf("unknown substantive event was lost: %q %v", line, ok)
	}
}

func TestIssueTimelineOmitsKnownProviderBookkeeping(t *testing.T) {
	for _, event := range []string{"mentioned", "subscribed", "unsubscribed"} {
		if line, ok := renderIssueTimelineState(githubTimelineEvent{Event: event}); ok || line != "" {
			t.Fatalf("bookkeeping %q should be omitted, got %q", event, line)
		}
	}
}

func TestGitHubIssueTimeHelper(t *testing.T) {
	if got := parseGitHubTime("2026-08-01T00:00:00Z"); got.IsZero() {
		t.Fatal("expected RFC3339 time to parse")
	}
}

func TestIssueSearchResponseJSONShape(t *testing.T) {
	var result githubIssueSearchResponse
	if err := json.Unmarshal([]byte(`{"total_count":1,"incomplete_results":false,"items":[{"number":1,"title":"x"}]}`), &result); err != nil {
		t.Fatal(err)
	}
	if result.TotalCount != 1 || len(result.Items) != 1 || result.Items[0].Number != 1 {
		t.Fatalf("unexpected decode: %#v", result)
	}
}
