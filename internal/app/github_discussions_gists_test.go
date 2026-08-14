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

func TestParseDiscussionAndGistTargets(t *testing.T) {
	for _, tt := range []struct {
		raw      string
		kind     GitHubTargetKind
		number   int
		owner    string
		name     string
		tail     string
		fragment string
	}{
		{raw: "https://github.com/o/r/discussions", kind: GitHubTargetDiscussions, owner: "o"},
		{raw: "https://github.com/o/r/discussions/42", kind: GitHubTargetDiscussion, number: 42, owner: "o"},
		{raw: "https://github.com/o/r/discussions/42#discussioncomment-123", kind: GitHubTargetDiscussion, number: 42, owner: "o", fragment: "discussioncomment-123"},
		{raw: "https://gist.github.com/alice/abcdef", kind: GitHubTargetGist, owner: "alice", name: "abcdef"},
		{raw: "https://gist.github.com/aa5a315d61ae9438b18d#gistcomment-456", kind: GitHubTargetGist, name: "aa5a315d61ae9438b18d", fragment: "gistcomment-456"},
		{raw: "https://gist.github.com/alice/abcdef/deadbeef", kind: GitHubTargetGist, owner: "alice", name: "abcdef", tail: "deadbeef"},
		{raw: "https://gist.github.com/alice/abcdef#file-demo-md-L2-L4", kind: GitHubTargetGist, owner: "alice", name: "abcdef", fragment: "file-demo-md-L2-L4"},
		{raw: "https://gist.github.com/alice/abcdef#gistcomment-456", kind: GitHubTargetGist, owner: "alice", name: "abcdef", fragment: "gistcomment-456"},
	} {
		target := parseGitHubTarget(tt.raw)
		if target == nil || target.Kind != tt.kind || target.Number != tt.number || target.Owner != tt.owner || target.Name != tt.name || target.Fragment != tt.fragment {
			t.Fatalf("target %s => %#v", tt.raw, target)
		}
		if tt.tail != "" && (len(target.Tail) != 1 || target.Tail[0] != tt.tail) {
			t.Fatalf("gist revision tail lost: %#v", target)
		}
	}
	for _, raw := range []string{
		"https://github.com/o/r/discussions/nope",
		"https://gist.github.com/alice",
		"https://gist.github.com/alice/abcdef/rev/extra",
		"https://gist.github.com/alice/abcdef/raw",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("unsupported Discussion/Gist route parsed: %s => %#v", raw, target)
		}
	}
}

func TestDiscussionsWithoutTokenFailBeforeGraphQL(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer server.Close()
	client := testGitHubClient(server.URL, server.URL, "")
	for _, raw := range []string{"https://github.com/o/r/discussions", "https://github.com/o/r/discussions/42", "https://github.com/o/r/discussions/42#discussioncomment-123"} {
		result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget(raw), client)
		if result.Outcome != GitHubNativeFailure || result.Err == nil || !strings.Contains(result.Err.Error(), "require authentication") || !strings.Contains(result.Err.Error(), "GH_TOKEN") {
			t.Fatalf("no-token Discussion result incorrect: %#v", result)
		}
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("no-token Discussions made provider calls")
	}
}

func TestDiscussionListIsBoundedAndAuthenticated(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/graphql" || r.Header.Get("Authorization") != "Bearer fake-token" {
			t.Fatalf("GraphQL auth/path wrong: %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "discussions(first:30") {
			t.Fatalf("Discussion list is not bounded: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"repository":{"discussions":{"nodes":[{"number":7,"title":"Question","url":"https://github.com/o/r/discussions/7","createdAt":"2026-08-01T00:00:00Z","updatedAt":"2026-08-02T00:00:00Z","upvoteCount":3,"locked":false,"author":{"login":"alice"},"category":{"name":"Q&A"}}],"pageInfo":{"hasNextPage":true,"endCursor":"cursor"}}}}}`)
	}))
	defer server.Close()
	out, err := readGitHubDiscussions(context.Background(), testGitHubClient(server.URL, server.URL, "fake-token"), parseGitHubTarget("https://github.com/o/r/discussions"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"view: discussions", "returned: 1", "more_available: true", "[#7 Question](https://github.com/o/r/discussions/7)", "Q&A", "@alice", "3 upvotes", "first 30"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Discussion list missing %q:\n%s", want, out)
		}
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected one bounded list GraphQL call")
	}
}

func TestDiscussionDetailOverviewIsBoundedWithoutDeepPagination(t *testing.T) {
	var calls int32
	longBody := strings.Repeat("discussion body line\n", 500) + "<!-- hidden marker -->tail"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/graphql" || r.Header.Get("Authorization") != "Bearer fake-token" {
			t.Fatalf("GraphQL auth/path wrong")
		}
		var request struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if !strings.Contains(request.Query, "comments(first:30") || !strings.Contains(request.Query, "replies(first:5") || strings.Contains(request.Query, "after:$after") {
			t.Fatalf("Discussion overview query is not bounded: %s", request.Query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"data":{"repository":{"discussion":{"number":42,"title":"How?","body":%q,"url":"https://github.com/o/r/discussions/42","createdAt":"2026-08-01T00:00:00Z","updatedAt":"2026-08-03T00:00:00Z","upvoteCount":5,"locked":false,"author":{"login":"owner"},"category":{"name":"Q&A"},"answer":{"id":"C101","databaseId":101,"body":"Answer body","url":"https://github.com/o/r/discussions/42#discussioncomment-101","createdAt":"2026-08-02T00:00:00Z","upvoteCount":2,"author":{"login":"bob"}},"comments":{"totalCount":250,"nodes":[{"id":"C101","databaseId":101,"body":"Answer body","url":"https://github.com/o/r/discussions/42#discussioncomment-101","createdAt":"2026-08-02T00:00:00Z","upvoteCount":2,"author":{"login":"bob"},"replies":{"totalCount":80,"nodes":[{"id":"R201","databaseId":201,"body":"First reply","url":"https://github.com/o/r/discussions/42#discussioncomment-201","createdAt":"2026-08-02T12:00:00Z","author":{"login":"alice"}}],"pageInfo":{"hasNextPage":true,"endCursor":"r1"}}},{"id":"C102","databaseId":102,"body":"Another comment","url":"https://github.com/o/r/discussions/42#discussioncomment-102","createdAt":"2026-08-04T00:00:00Z","author":{"login":"dave"},"replies":{"totalCount":0,"nodes":[],"pageInfo":{"hasNextPage":false}}}],"pageInfo":{"hasNextPage":true,"endCursor":"c2"}}}}}}`, longBody))
	}))
	defer server.Close()
	out, err := readGitHubDiscussion(context.Background(), testGitHubClient(server.URL, server.URL, "fake-token"), parseGitHubTarget("https://github.com/o/r/discussions/42"))
	if err != nil {
		t.Fatal(err)
	}
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("Discussion overview exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{
		"overview: true", "comments_reported: 250", "comments_returned: 2", "comments_provider_more_available: true",
		"accepted_answer_comment_id: 101", "accepted_answer_url: \"https://github.com/o/r/discussions/42#discussioncomment-101\"",
		"### Accepted answer `101` by @bob", "#### Reply `201` by @alice", "More replies to comment `101` exist upstream",
		"Discussion body preview locally truncated", "More top-level Discussion comments exist upstream",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("Discussion overview missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hidden marker") || strings.Count(out, "Answer body") != 1 || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("Discussion root sanitized/pagination wrong calls=%d:\n%s", calls, out)
	}
}

func TestDiscussionExactCommentSelectorFindsDeferredReply(t *testing.T) {
	var detailCalls, replyCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Fatalf("GraphQL path wrong")
		}
		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(request.Query, "node(id:$id)") {
			atomic.AddInt32(&replyCalls, 1)
			if request.Variables["id"] != "C101" || request.Variables["after"] != "r1" {
				t.Fatalf("unexpected deferred reply vars: %#v", request.Variables)
			}
			_, _ = io.WriteString(w, `{"data":{"node":{"replies":{"nodes":[{"id":"R202","databaseId":202,"body":"Selected reply <!-- hidden --> kept in full","url":"https://github.com/o/r/discussions/42#discussioncomment-202","createdAt":"2026-08-03T00:00:00Z","updatedAt":"2026-08-03T01:00:00Z","author":{"login":"carol"}}],"pageInfo":{"hasNextPage":false}}}}}`)
			return
		}
		atomic.AddInt32(&detailCalls, 1)
		_, _ = io.WriteString(w, `{"data":{"repository":{"discussion":{"number":42,"title":"How?","url":"https://github.com/o/r/discussions/42","author":{"login":"owner"},"category":{"name":"Q&A"},"comments":{"totalCount":1,"nodes":[{"id":"C101","databaseId":101,"body":"Parent","url":"https://github.com/o/r/discussions/42#discussioncomment-101","author":{"login":"bob"},"replies":{"totalCount":2,"nodes":[{"id":"R201","databaseId":201,"body":"Not selected","url":"https://github.com/o/r/discussions/42#discussioncomment-201","author":{"login":"alice"}}],"pageInfo":{"hasNextPage":true,"endCursor":"r1"}}}],"pageInfo":{"hasNextPage":false}}}}}}`)
	}))
	defer server.Close()
	out, err := readGitHubDiscussion(context.Background(), testGitHubClient(server.URL, server.URL, "fake-token"), parseGitHubTarget("https://github.com/o/r/discussions/42#discussioncomment-202"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"selector: \"discussioncomment-202\"", "comment_id: 202", "parent_comment_id: 101", "url: \"https://github.com/o/r/discussions/42#discussioncomment-202\"", "Selected reply", "kept in full", "From: [#42 How?]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("exact Discussion reply missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hidden") || strings.Contains(out, "Not selected") || atomic.LoadInt32(&detailCalls) != 1 || atomic.LoadInt32(&replyCalls) != 1 {
		t.Fatalf("exact Discussion lookup fan-out/content wrong detail=%d replies=%d:\n%s", detailCalls, replyCalls, out)
	}
}

func TestDiscussionExactTopLevelCommentStopsBeforeReplyPagination(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var request struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if strings.Contains(request.Query, "node(id:$id)") {
			t.Fatal("top-level exact comment must not paginate replies")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"repository":{"discussion":{"number":42,"title":"How?","url":"https://github.com/o/r/discussions/42","comments":{"totalCount":1,"nodes":[{"id":"C101","databaseId":101,"body":"Exact top-level","url":"https://github.com/o/r/discussions/42#discussioncomment-101","author":{"login":"bob"},"replies":{"totalCount":500,"nodes":[],"pageInfo":{"hasNextPage":true,"endCursor":"r0"}}}],"pageInfo":{"hasNextPage":false}}}}}}`)
	}))
	defer server.Close()
	out, err := readGitHubDiscussion(context.Background(), testGitHubClient(server.URL, server.URL, "fake-token"), parseGitHubTarget("https://github.com/o/r/discussions/42#discussioncomment-101"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Exact top-level") || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("top-level exact comment did not stop early calls=%d:\n%s", calls, out)
	}
}

func TestGistSelectorSlugAndLines(t *testing.T) {
	if got := gistFileSlug("Hello.World_test.md"); got != "file-hello-world-test-md" {
		t.Fatalf("gist slug=%q", got)
	}
	selector, ok, err := parseGistFileSelector("file-hello-world-test-md-L2-L3")
	if err != nil || !ok || selector.Slug != "file-hello-world-test-md" || selector.Start != 2 || selector.End != 3 {
		t.Fatalf("selector=%#v ok=%v err=%v", selector, ok, err)
	}
	if _, _, err := parseGistFileSelector("file-a-L4-L2"); err == nil {
		t.Fatal("reversed Gist line range should fail")
	}
	id, ok, err := parseGistCommentSelector("gistcomment-3793344")
	if err != nil || !ok || id != 3793344 {
		t.Fatalf("gist comment selector id=%d ok=%v err=%v", id, ok, err)
	}
}

func TestGistRootIsBoundedAndDoesNotFollowCommentPagination(t *testing.T) {
	var commentPages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gists/abc":
			_, _ = io.WriteString(w, `{"id":"abc","description":"demo","public":true,"html_url":"https://gist.github.com/alice/abc","created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-02T00:00:00Z","comments":2,"owner":{"login":"alice"},"files":{"demo.md":{"filename":"demo.md","type":"text/markdown","language":"Markdown","raw_url":"https://gist.githubusercontent.test/raw/demo","size":40,"truncated":false,"content":"# Demo\n<!-- source marker -->\nkept\nline4"},"z.txt":{"filename":"z.txt","type":"text/plain","raw_url":"https://gist.githubusercontent.test/raw/z","size":1,"content":"z"}},"history":[{"version":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","committed_at":"2026-08-02T00:00:00Z","change_status":{"total":2,"additions":1,"deletions":1}}]}`)
		case "/gists/abc/comments":
			atomic.AddInt32(&commentPages, 1)
			if r.URL.Query().Get("page") != "" || r.URL.Query().Get("per_page") != "30" {
				t.Fatalf("Gist root followed/changed comment pagination: %s", r.URL.RawQuery)
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/gists/abc/comments?per_page=30&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"id":1,"body":"Visible comment <!-- hidden --> tail","user":{"login":"bob"},"created_at":"2026-08-03T00:00:00Z","url":"https://api.github.com/gists/abc/comments/1"}]`)
		default:
			t.Fatalf("unexpected Gist request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://gist.github.com/alice/abc"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"overview: true", "## File index", "[demo.md](https://gist.github.com/alice/abc#file-demo-md)", "<!-- source marker -->", "## Comment index", "[Comment 1](https://gist.github.com/alice/abc#gistcomment-1)", "Visible comment", "tail", "comments_provider_more_available: true", "## Revision index", "`aaaaaaaaaaaa`"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Gist root missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<!-- hidden -->") || strings.Contains(out, "## demo.md") || atomic.LoadInt32(&commentPages) != 1 {
		t.Fatalf("Gist root sanitization/pagination wrong pages=%d:\n%s", commentPages, out)
	}
}

func TestLargeGistRootStaysBoundedWithoutRawFanout(t *testing.T) {
	gist := githubGist{ID: "big", Public: true, HTMLURL: "https://gist.github.com/alice/big", Comments: 500, Files: map[string]githubGistFile{}}
	gist.Owner.Login = "alice"
	for i := 0; i < 100; i++ {
		gist.Files[fmt.Sprintf("file-%03d.txt", i)] = githubGistFile{Filename: fmt.Sprintf("file-%03d.txt", i), RawURL: fmt.Sprintf("RAW-%03d", i), Size: 10000, Content: strings.Repeat(fmt.Sprintf("content-%03d ", i), 300), Truncated: i%3 == 0}
	}
	gist.History = make([]githubGistHistory, 100)
	for i := range gist.History {
		gist.History[i].Version = fmt.Sprintf("%040x", i+1)
		gist.History[i].CommittedAt = "2026-08-01T00:00:00Z"
	}
	comments := make([]githubGistComment, 30)
	for i := range comments {
		body := strings.Repeat(fmt.Sprintf("comment-%02d ", i), 100)
		comments[i] = githubGistComment{ID: int64(i + 1), Body: &body, CreatedAt: "2026-08-01T00:00:00Z"}
		comments[i].User.Login = "user"
	}
	var commentCalls, rawCalls int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/gists/big":
			_ = json.NewEncoder(w).Encode(gist)
		case r.URL.Path == "/gists/big/comments":
			atomic.AddInt32(&commentCalls, 1)
			w.Header().Set("Link", fmt.Sprintf(`<%s/gists/big/comments?per_page=30&page=2>; rel="next"`, server.URL))
			_ = json.NewEncoder(w).Encode(comments)
		default:
			atomic.AddInt32(&rawCalls, 1)
			t.Fatalf("Gist root unexpectedly fetched subordinate raw URL %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://gist.github.com/alice/big"))
	if err != nil {
		t.Fatal(err)
	}
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("large Gist overview exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{"files_returned: 100", "files_local_omitted:", "comments_reported: 500", "comments_returned: 30", "comments_local_omitted:", "revisions_returned: 100", "locally omitted from this overview", "comments_provider_more_available: true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("large Gist overview missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "files_local_omitted: 0") || strings.Contains(out, "comments_local_omitted: 0") || strings.Contains(out, "revisions_local_omitted: 0") || atomic.LoadInt32(&commentCalls) != 1 || atomic.LoadInt32(&rawCalls) != 0 {
		t.Fatalf("large Gist root fan-out/omission wrong comments=%d raw=%d", commentCalls, rawCalls)
	}
}

func TestGistFileLineSelectorNarrowsBeforeComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/gists/abc" {
			_, _ = io.WriteString(w, `{"id":"abc","public":true,"html_url":"https://gist.github.com/alice/abc","files":{"demo.md":{"filename":"demo.md","content":"one\ntwo\nthree\nfour"},"other.txt":{"filename":"other.txt","content":"other"}}}`)
			return
		}
		t.Fatalf("exact Gist file selector must not fetch comments/other resources: %s", r.URL.Path)
	}))
	defer server.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://gist.github.com/alice/abc#file-demo-md-L2-L3"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# demo.md") || !strings.Contains(out, "Lines 2-3") || !strings.Contains(out, "two\nthree") || !strings.Contains(out, `url: "https://gist.github.com/alice/abc#file-demo-md-L2-L3"`) || strings.Contains(out, "other") || strings.Contains(out, "## Comment") {
		t.Fatalf("Gist selector failed to narrow:\n%s", out)
	}
}

func TestGistTruncatedFileSelectorUsesRawWithoutForwardingToken(t *testing.T) {
	var rawAuth string
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "FULL RAW\n<!-- source preserved -->\n")
	}))
	defer raw.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/gists/abc" {
			_, _ = io.WriteString(w, fmt.Sprintf(`{"id":"abc","files":{"big.txt":{"filename":"big.txt","raw_url":%q,"size":999,"truncated":true,"content":"PARTIAL"}}}`, raw.URL))
			return
		}
		t.Fatalf("unexpected exact Gist file request %s", r.URL.Path)
	}))
	defer api.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(api.URL, api.URL, "fake-token"), parseGitHubTarget("https://gist.github.com/alice/abc#file-big-txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "FULL RAW") || strings.Contains(out, "PARTIAL") || !strings.Contains(out, "<!-- source preserved -->") || rawAuth != "" {
		t.Fatalf("Gist raw fallback/token safety wrong auth=%q:\n%s", rawAuth, out)
	}
}

func TestGistTruncatedRawFailurePointsToRawURL(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/gists/abc" {
			_, _ = io.WriteString(w, `{"id":"abc","files":{"big.txt":{"filename":"big.txt","raw_url":"http://127.0.0.1:1/unavailable","truncated":true,"content":"PARTIAL"}}}`)
			return
		}
		t.Fatalf("unexpected exact Gist file request %s", r.URL.Path)
	}))
	defer api.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(api.URL, api.URL, ""), parseGitHubTarget("https://gist.github.com/alice/abc#file-big-txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PARTIAL") || !strings.Contains(out, "marked this API file content truncated") || !strings.Contains(out, "Full raw file: http://127.0.0.1:1/unavailable") {
		t.Fatalf("truncated raw failure not truthful:\n%s", out)
	}
}

func TestGistExactCommentUsesSingleCommentEndpointAndCanonicalFragment(t *testing.T) {
	var listCalls, exactCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gists/abc":
			_, _ = io.WriteString(w, `{"id":"abc","html_url":"https://gist.github.com/alice/abc","files":{"a.txt":{"filename":"a.txt","content":"DO NOT RENDER"}}}`)
		case "/gists/abc/comments/3793344":
			atomic.AddInt32(&exactCalls, 1)
			_, _ = io.WriteString(w, `{"id":3793344,"url":"https://api.github.com/gists/abc/comments/3793344","body":"Exact Gist comment <!-- hidden --> kept","user":{"login":"bob"},"created_at":"2026-08-01T00:00:00Z"}`)
		case "/gists/abc/comments":
			atomic.AddInt32(&listCalls, 1)
			t.Fatal("exact Gist comment must not list comments")
		default:
			t.Fatalf("unexpected Gist exact-comment request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://gist.github.com/alice/abc#gistcomment-3793344"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"selector: \"gistcomment-3793344\"", "comment_id: 3793344", `url: "https://gist.github.com/alice/abc#gistcomment-3793344"`, "@bob", "Exact Gist comment", "kept"} {
		if !strings.Contains(out, want) {
			t.Fatalf("exact Gist comment missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hidden") || strings.Contains(out, "DO NOT RENDER") || atomic.LoadInt32(&exactCalls) != 1 || atomic.LoadInt32(&listCalls) != 0 {
		t.Fatalf("exact Gist comment endpoint/content wrong exact=%d list=%d:\n%s", exactCalls, listCalls, out)
	}
}

func TestGistRevisionUsesRevisionEndpoint(t *testing.T) {
	var gistPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/gists/abc/") && !strings.HasSuffix(r.URL.Path, "/comments") {
			gistPath = r.URL.Path
			_, _ = io.WriteString(w, `{"id":"abc","files":{"a.txt":{"filename":"a.txt","content":"revision"}}}`)
			return
		}
		if r.URL.Path == "/gists/abc/comments" {
			_, _ = io.WriteString(w, `[]`)
			return
		}
		t.Fatalf("unexpected Gist revision request %s", r.URL.Path)
	}))
	defer server.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://gist.github.com/alice/abc/deadbeef"))
	if err != nil {
		t.Fatal(err)
	}
	if gistPath != "/gists/abc/deadbeef" || !strings.Contains(out, `revision: "deadbeef"`) || !strings.Contains(out, "revision") {
		t.Fatalf("Gist revision endpoint/output wrong path=%s:\n%s", gistPath, out)
	}
}
