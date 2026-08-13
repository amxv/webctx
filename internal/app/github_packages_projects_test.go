package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParsePackageProjectTargetsAndLongTailFallback(t *testing.T) {
	for _, tt := range []struct {
		raw    string
		kind   GitHubTargetKind
		owner  string
		name   string
		number int
		tail   []string
	}{
		{raw: "https://github.com/orgs/acme/packages/container/package/widget", kind: GitHubTargetPackage, owner: "acme", name: "widget", tail: []string{"org", "container"}},
		{raw: "https://github.com/users/alice/packages/npm/package/pkg", kind: GitHubTargetPackage, owner: "alice", name: "pkg", tail: []string{"user", "npm"}},
		{raw: "https://github.com/orgs/acme/projects/7", kind: GitHubTargetProjectV2, owner: "acme", number: 7, tail: []string{"org"}},
		{raw: "https://github.com/users/alice/projects/3", kind: GitHubTargetProjectV2, owner: "alice", number: 3, tail: []string{"user"}},
	} {
		target := parseGitHubTarget(tt.raw)
		if target == nil || target.Kind != tt.kind || target.Owner != tt.owner || target.Name != tt.name || target.Number != tt.number || strings.Join(target.Tail, ",") != strings.Join(tt.tail, ",") {
			t.Fatalf("target %s => %#v", tt.raw, target)
		}
	}
	for _, raw := range []string{
		"https://github.com/orgs/acme/packages",
		"https://github.com/users/alice/packages/container",
		"https://github.com/orgs/acme/packages/unknown/package/widget",
		"https://github.com/o/r/wiki",
		"https://github.com/o/r/settings",
		"https://github.com/o/r/security",
		"https://github.com/o/r/archive/refs/heads/main.zip",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("excluded/unproven long-tail route was claimed: %s => %#v", raw, target)
		}
	}
}

func TestPackageDetailAndBoundedVersions(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/orgs/acme/packages/container/widget":
			_, _ = io.WriteString(w, `{"id":1,"name":"widget","package_type":"container","html_url":"https://github.com/orgs/acme/packages/container/package/widget","visibility":"public","description":"demo package","version_count":50,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-08-01T00:00:00Z","repository":{"full_name":"acme/widget","html_url":"https://github.com/acme/widget"}}`)
		case "/orgs/acme/packages/container/widget/versions":
			if r.URL.Query().Get("per_page") != "30" || r.URL.Query().Get("page") != "2" {
				t.Fatalf("version pagination lost: %s", r.URL.RawQuery)
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/orgs/acme/packages/container/widget/versions?per_page=30&page=1>; rel="prev", <%s/orgs/acme/packages/container/widget/versions?per_page=30&page=3>; rel="next"`, server.URL, server.URL))
			_, _ = io.WriteString(w, `[{"id":11,"name":"sha256:abc","package_html_url":"https://github.com/orgs/acme/packages/container/widget/11","updated_at":"2026-08-01T00:00:00Z","metadata":{"package_type":"container","container":{"tags":["latest","v1"]}}}]`)
		default:
			t.Fatalf("unexpected package request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubPackage(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/orgs/acme/packages/container/package/widget?page=2"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`package: "widget"`, `package_type: "container"`, "version_count: 50", "versions_page: 2", "demo package", "sha256:abc", "tags: latest, v1", "Previous", "page=1", "Next", "page=3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("package output missing %q:\n%s", want, out)
		}
	}
}

func TestPublicOrganizationProjectV2UsesRESTWithoutTokenAndBoundsItems(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("anonymous public Project unexpectedly sent auth")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/orgs/acme/projectsV2/7":
			_, _ = io.WriteString(w, `{"id":71,"number":7,"title":"Roadmap","short_description":"Ship it","public":true,"state":"open","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-08-01T00:00:00Z"}`)
		case "/orgs/acme/projectsV2/7/items":
			if r.URL.Query().Get("per_page") != "50" {
				t.Fatalf("Project items are not bounded: %s", r.URL.RawQuery)
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/orgs/acme/projectsV2/7/items?per_page=50&after=cursor>; rel="next"`, server.URL))
			_, _ = io.WriteString(w, `[{"id":101,"content_type":"Issue","content":{"number":12,"title":"Feature","html_url":"https://github.com/acme/r/issues/12","state":"open","repository_url":"https://api.github.com/repos/acme/r"}},{"id":102,"content_type":"DraftIssue","content":{"title":"Draft task","body":"details"}}]`)
		default:
			t.Fatalf("unexpected Project REST path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubProjectV2(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/orgs/acme/projects/7"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`project: 7`, `title: "Roadmap"`, "items_returned: 2", "more_items_available: true", "Ship it", "[Feature](https://github.com/acme/r/issues/12)", "acme/r", "#12", "Draft task", "first 50 items"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Project output missing %q:\n%s", want, out)
		}
	}
}

func TestPublicProjectV2RetriesWithoutNarrowFineGrainedToken(t *testing.T) {
	var authCalls, anonymousCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "" {
			atomic.AddInt32(&authCalls, 1)
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"Resource not accessible by personal access token"}`)
			return
		}
		atomic.AddInt32(&anonymousCalls, 1)
		switch r.URL.Path {
		case "/orgs/acme/projectsV2/7":
			_, _ = io.WriteString(w, `{"id":71,"number":7,"title":"Public roadmap","public":true,"state":"open"}`)
		case "/orgs/acme/projectsV2/7/items":
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Fatalf("unexpected Project REST path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubProjectV2(context.Background(), testGitHubClient(server.URL, server.URL, "fake-token"), parseGitHubTarget("https://github.com/orgs/acme/projects/7"))
	if err != nil || !strings.Contains(out, "# Public roadmap") {
		t.Fatalf("fine-grained public Project fallback failed err=%v:\n%s", err, out)
	}
	if atomic.LoadInt32(&authCalls) != 2 || atomic.LoadInt32(&anonymousCalls) != 2 {
		t.Fatalf("Project auth/anonymous retry counts auth=%d anon=%d", authCalls, anonymousCalls)
	}
}

func TestUserProjectV2UsesUserRESTPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/alice/projectsV2/3":
			_, _ = io.WriteString(w, `{"id":31,"number":3,"title":"Personal","public":true,"state":"open"}`)
		case "/users/alice/projectsV2/3/items":
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Fatalf("unexpected User Project path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubProjectV2(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/users/alice/projects/3"))
	if err != nil || !strings.Contains(out, "# Personal") {
		t.Fatalf("User Project failed err=%v:\n%s", err, out)
	}
}

func TestPackageProjectProviderFailuresStayNative(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"private body"}`)
	}))
	defer server.Close()
	result := readGitHubNativeWithClient(context.Background(), parseGitHubTarget("https://github.com/orgs/acme/packages/container/package/widget"), testGitHubClient(server.URL, server.URL, ""))
	if result.Outcome != GitHubNativeFailure || result.Err == nil || strings.Contains(result.Err.Error(), "private body") {
		t.Fatalf("Package native failure boundary wrong: %#v", result)
	}
}
