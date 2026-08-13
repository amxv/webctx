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
		{raw: "https://gist.github.com/alice/abcdef", kind: GitHubTargetGist, owner: "alice", name: "abcdef"},
		{raw: "https://gist.github.com/alice/abcdef/deadbeef", kind: GitHubTargetGist, owner: "alice", name: "abcdef", tail: "deadbeef"},
		{raw: "https://gist.github.com/alice/abcdef#file-demo-md-L2-L4", kind: GitHubTargetGist, owner: "alice", name: "abcdef", fragment: "file-demo-md-L2-L4"},
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
	for _, raw := range []string{"https://github.com/o/r/discussions", "https://github.com/o/r/discussions/42"} {
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

func TestDiscussionDetailPaginatesCommentsAndRepliesWithoutDuplication(t *testing.T) {
	var detailCalls, replyCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" || r.Header.Get("Authorization") != "Bearer fake-token" {
			t.Fatalf("GraphQL auth/path wrong")
		}
		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(request.Query, "node(id:$id)") {
			atomic.AddInt32(&replyCalls, 1)
			if request.Variables["after"] == "r1" {
				_, _ = io.WriteString(w, `{"data":{"node":{"replies":{"nodes":[{"id":"R2","body":"Second reply <!-- hidden --> kept","url":"https://github.com/o/r/discussions/42#discussioncomment-r2","createdAt":"2026-08-03T00:00:00Z","upvoteCount":1,"author":{"login":"carol"}}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}`)
				return
			}
			t.Fatalf("unexpected reply cursor %#v", request.Variables["after"])
		}
		atomic.AddInt32(&detailCalls, 1)
		after := request.Variables["after"]
		if after == nil {
			_, _ = io.WriteString(w, `{"data":{"repository":{"discussion":{"number":42,"title":"How?","body":"Visible body <!-- hidden marker --> tail","url":"https://github.com/o/r/discussions/42","createdAt":"2026-08-01T00:00:00Z","updatedAt":"2026-08-03T00:00:00Z","upvoteCount":5,"locked":false,"author":{"login":"owner"},"category":{"name":"Q&A"},"answer":{"id":"C1"},"comments":{"nodes":[{"id":"C1","body":"Answer body","url":"https://github.com/o/r/discussions/42#discussioncomment-c1","createdAt":"2026-08-02T00:00:00Z","upvoteCount":2,"author":{"login":"bob"},"replies":{"nodes":[{"id":"R1","body":"First reply","url":"https://github.com/o/r/discussions/42#discussioncomment-r1","createdAt":"2026-08-02T12:00:00Z","author":{"login":"alice"}}],"pageInfo":{"hasNextPage":true,"endCursor":"r1"}}}],"pageInfo":{"hasNextPage":true,"endCursor":"c1"}}}}}}`)
			return
		}
		if after == "c1" {
			_, _ = io.WriteString(w, `{"data":{"repository":{"discussion":{"number":42,"title":"How?","body":"Visible body <!-- hidden marker --> tail","url":"https://github.com/o/r/discussions/42","createdAt":"2026-08-01T00:00:00Z","updatedAt":"2026-08-03T00:00:00Z","upvoteCount":5,"locked":false,"author":{"login":"owner"},"category":{"name":"Q&A"},"answer":{"id":"C1"},"comments":{"nodes":[{"id":"C2","body":"Another comment","url":"https://github.com/o/r/discussions/42#discussioncomment-c2","createdAt":"2026-08-04T00:00:00Z","author":{"login":"dave"},"replies":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":null}}}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}}`)
			return
		}
		t.Fatalf("unexpected comment cursor %#v", after)
	}))
	defer server.Close()
	out, err := readGitHubDiscussion(context.Background(), testGitHubClient(server.URL, server.URL, "fake-token"), parseGitHubTarget("https://github.com/o/r/discussions/42"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"discussion: 42", "# How?", "Visible body", "tail", "### Accepted answer by @bob", "Answer body", "#### Reply by @alice", "First reply", "#### Reply by @carol", "Second reply", "kept", "### Comment by @dave", "Another comment"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Discussion detail missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hidden marker") || strings.Contains(out, "<!-- hidden -->") || strings.Count(out, "Answer body") != 1 {
		t.Fatalf("Discussion sanitization/duplication wrong:\n%s", out)
	}
	if atomic.LoadInt32(&detailCalls) != 2 || atomic.LoadInt32(&replyCalls) != 1 {
		t.Fatalf("pagination calls detail=%d reply=%d", detailCalls, replyCalls)
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
}

func TestGistFullReadPreservesSourceCommentsSanitizesCommentsAndPaginates(t *testing.T) {
	var commentPages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gists/abc":
			_, _ = io.WriteString(w, `{"id":"abc","description":"demo","public":true,"html_url":"https://gist.github.com/alice/abc","created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-02T00:00:00Z","owner":{"login":"alice"},"files":{"demo.md":{"filename":"demo.md","type":"text/markdown","language":"Markdown","raw_url":"https://gist.githubusercontent.test/raw/demo","size":40,"truncated":false,"content":"# Demo\n<!-- source marker -->\nkept\nline4"},"z.txt":{"filename":"z.txt","type":"text/plain","raw_url":"https://gist.githubusercontent.test/raw/z","size":1,"content":"z"}},"history":[{"version":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","committed_at":"2026-08-02T00:00:00Z","change_status":{"total":2,"additions":1,"deletions":1}}]}`)
		case "/gists/abc/comments":
			atomic.AddInt32(&commentPages, 1)
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `[{"id":2,"body":null,"user":{"login":"gone"},"created_at":"2026-08-04T00:00:00Z"}]`)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/gists/abc/comments?per_page=100&page=2>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"id":1,"body":"Visible comment <!-- hidden --> tail","user":{"login":"bob"},"created_at":"2026-08-03T00:00:00Z"}]`)
		default:
			t.Fatalf("unexpected Gist request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://gist.github.com/alice/abc"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"gist: \"abc\"", "## demo.md", "<!-- source marker -->", "## z.txt", "## Comments", "Visible comment", "tail", "Gist comment body is unavailable", "## Revisions", "`aaaaaaaaaaaa`"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Gist output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<!-- hidden -->") || atomic.LoadInt32(&commentPages) != 2 {
		t.Fatalf("Gist comment sanitization/pagination wrong")
	}
}

func TestGistFileLineSelectorNarrowsBeforeComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/gists/abc" {
			_, _ = io.WriteString(w, `{"id":"abc","public":true,"files":{"demo.md":{"filename":"demo.md","content":"one\ntwo\nthree\nfour"},"other.txt":{"filename":"other.txt","content":"other"}}}`)
			return
		}
		if r.URL.Path == "/gists/abc/comments" {
			_, _ = io.WriteString(w, `[{"id":1,"body":"COMMENT MUST NOT RENDER"}]`)
			return
		}
		t.Fatalf("unexpected Gist selector request %s", r.URL.Path)
	}))
	defer server.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://gist.github.com/alice/abc#file-demo-md-L2-L3"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "## demo.md") || !strings.Contains(out, "Lines 2-3") || !strings.Contains(out, "two\nthree") || strings.Contains(out, "other") || strings.Contains(out, "COMMENT MUST NOT RENDER") || strings.Contains(out, "## Comments") {
		t.Fatalf("Gist selector failed to narrow:\n%s", out)
	}
}

func TestGistTruncatedFileUsesRawWithoutForwardingToken(t *testing.T) {
	var rawAuth string
	var raw *httptest.Server
	raw = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if r.URL.Path == "/gists/abc/comments" {
			_, _ = io.WriteString(w, `[]`)
			return
		}
	}))
	defer api.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(api.URL, api.URL, "fake-token"), parseGitHubTarget("https://gist.github.com/alice/abc"))
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
		_, _ = io.WriteString(w, `[]`)
	}))
	defer api.Close()
	out, err := readGitHubGist(context.Background(), testGitHubClient(api.URL, api.URL, ""), parseGitHubTarget("https://gist.github.com/alice/abc"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PARTIAL") || !strings.Contains(out, "marked this API file content truncated") || !strings.Contains(out, "Full raw file: http://127.0.0.1:1/unavailable") {
		t.Fatalf("truncated raw failure not truthful:\n%s", out)
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
