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

func TestParseGitHubRefReleaseSocialTargets(t *testing.T) {
	tests := []struct {
		raw  string
		kind GitHubTargetKind
		name string
	}{
		{raw: "https://github.com/o/r/branches", kind: GitHubTargetBranches},
		{raw: "https://github.com/o/r/tags", kind: GitHubTargetTags},
		{raw: "https://github.com/o/r/releases", kind: GitHubTargetReleases},
		{raw: "https://github.com/o/r/releases/latest", kind: GitHubTargetReleaseLatest},
		{raw: "https://github.com/o/r/releases/tag/v1.2.3", kind: GitHubTargetRelease, name: "v1.2.3"},
		{raw: "https://github.com/o/r/releases/tag/release/foo", kind: GitHubTargetRelease, name: "release/foo"},
		{raw: "https://github.com/o/r/forks", kind: GitHubTargetForks},
		{raw: "https://github.com/o/r/stargazers", kind: GitHubTargetStargazers},
		{raw: "https://github.com/o/r/watchers", kind: GitHubTargetWatchers},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			target := parseGitHubTarget(tt.raw)
			if target == nil || target.Kind != tt.kind || target.Name != tt.name {
				t.Fatalf("unexpected target: %#v", target)
			}
		})
	}
	for _, raw := range []string{
		"https://github.com/o/r/branches/main",
		"https://github.com/o/r/tags/v1",
		"https://github.com/o/r/releases/123",
		"https://github.com/o/r/stargazers/extra",
	} {
		if target := parseGitHubTarget(raw); target != nil {
			t.Fatalf("unstable/synthetic route should stay unsupported: %s => %#v", raw, target)
		}
	}
}

func TestBranchesBoundedPaginationAndProtection(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/branches" {
			t.Fatalf("unexpected branch path %s", r.URL.Path)
		}
		if r.URL.Query().Get("per_page") != "8" || r.URL.Query().Get("page") != "2" || r.URL.Query().Get("protected") != "true" {
			t.Fatalf("branch query lost: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/branches?protected=true&per_page=8&page=1>; rel="prev", <%s/repos/o/r/branches?protected=true&per_page=8&page=3>; rel="next"`, server.URL, server.URL))
		_, _ = io.WriteString(w, `[{"name":"release/next","protected":true,"commit":{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}]`)
	}))
	defer server.Close()
	out, err := readGitHubBranches(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/branches?protected=true&page=2"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"view: \"branches\"", "page: 2", "[release/next](https://github.com/o/r/tree/release/next)", "`aaaaaaaaaaaa`", "protected", "Previous", "page=1", "Next", "page=3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("branches output missing %q:\n%s", want, out)
		}
	}
}

func TestTagsBoundedAndLinkToTreeRef(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("unexpected Accept %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"name":"release/v1","commit":{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}]`)
	}))
	defer server.Close()
	out, err := readGitHubTags(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/tags"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[release/v1](https://github.com/o/r/tree/release/v1)") || !strings.Contains(out, "`bbbbbbbbbbbb`") {
		t.Fatalf("tags output incorrect:\n%s", out)
	}
}

func TestReleaseListDoesNotExpandBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
            {"id":1,"tag_name":"v2","name":"Version 2","body":"MUST NOT EXPAND","html_url":"https://github.com/o/r/releases/tag/v2","published_at":"2026-08-01T00:00:00Z"},
            {"id":2,"tag_name":"v1-rc","name":"","body":"MUST NOT EXPAND EITHER","html_url":"https://github.com/o/r/releases/tag/v1-rc","prerelease":true,"published_at":"2026-07-01T00:00:00Z"}
        ]`)
	}))
	defer server.Close()
	out, err := readGitHubReleases(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/releases"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[Version 2](https://github.com/o/r/releases/tag/v2)") || !strings.Contains(out, "[v1-rc](https://github.com/o/r/releases/tag/v1-rc)") || !strings.Contains(out, "prerelease") || strings.Contains(out, "MUST NOT EXPAND") {
		t.Fatalf("release list expanded body or lost metadata:\n%s", out)
	}
}

func TestReleaseDetailBoundsLongBodyAndStopsAssetPagination(t *testing.T) {
	longBody := strings.Repeat("release-notes-line\n", 500)
	assets := make([]githubReleaseAsset, 100)
	for i := range assets {
		assets[i] = githubReleaseAsset{
			ID:                 int64(i + 1),
			Name:               fmt.Sprintf("asset-%03d.zip", i),
			Size:               int64(1024 + i),
			ContentType:        "application/zip",
			DownloadCount:      i,
			BrowserDownloadURL: fmt.Sprintf("https://github.com/o/r/releases/download/release/v2/asset-%03d.zip", i),
		}
	}
	var assetPages int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/releases/tags/release%2Fv2", "/repos/o/r/releases/tags/release/v2":
			_, _ = io.WriteString(w, fmt.Sprintf(`{"id":10,"tag_name":"release/v2","target_commitish":"main","name":"Big Release","body":%q,"html_url":"https://github.com/o/r/releases/tag/release/v2","author":{"login":"maintainer"},"published_at":"2026-08-01T00:00:00Z"}`, longBody+"<!-- hidden -->\nvisible-tail"))
		case "/repos/o/r/releases/10/assets":
			atomic.AddInt32(&assetPages, 1)
			if r.URL.Query().Get("page") != "" || r.URL.Query().Get("per_page") != "100" {
				t.Fatalf("release asset overview followed/changed pagination: %s", r.URL.RawQuery)
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/releases/10/assets?per_page=100&page=2>; rel="next"`, server.URL))
			_ = json.NewEncoder(w).Encode(assets)
		default:
			t.Fatalf("unexpected release request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	out, err := readGitHubRelease(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/releases/tag/release/v2"), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := utf8.RuneCountInString(out); got > githubOverviewRunes {
		t.Fatalf("release overview exceeded shared target: %d runes\n%s", got, out)
	}
	for _, want := range []string{
		"overview: true", "assets_returned: 100", "assets_provider_more_available: true", "assets_local_omitted:",
		"Release notes preview locally truncated", "Canonical GitHub release page (complete notes in the browser): https://github.com/o/r/releases/tag/release/v2",
		"asset-000.zip", "https://github.com/o/r/releases/download/release/v2/asset-000.zip",
		"locally omitted from this overview", "more release assets beyond the provider page",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("release overview missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "assets_local_omitted: 0") || strings.Contains(out, "asset-099.zip") || strings.Contains(out, "visible-tail") || strings.Contains(out, "hidden") || atomic.LoadInt32(&assetPages) != 1 {
		t.Fatalf("release overview expanded subordinate content/provider pages; pages=%d\n%s", assetPages, out)
	}
}

func TestLatestReleaseUsesLatestEndpoint(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			_, _ = io.WriteString(w, `{"id":10,"tag_name":"v2","name":"Latest"}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/releases/10/assets") {
			_, _ = io.WriteString(w, `[]`)
			return
		}
		t.Fatalf("unexpected latest endpoint %s", r.URL.Path)
	}))
	defer server.Close()
	_, err := readGitHubRelease(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/releases/latest"), true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotPath, "/releases/10/assets") {
		t.Fatalf("latest release did not proceed through expected endpoints; final=%s", gotPath)
	}
}

func TestForksUseStargazersCountNotWatchersAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"full_name":"u/r","html_url":"https://github.com/u/r","stargazers_count":12,"watchers_count":999,"language":"Go","updated_at":"2026-08-01T00:00:00Z","owner":{"login":"u"}}]`)
	}))
	defer server.Close()
	out, err := readGitHubForks(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/forks?sort=stargazers"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "12 stars") || strings.Contains(out, "999") || strings.Contains(out, "999 watchers") {
		t.Fatalf("fork star/watcher semantics wrong:\n%s", out)
	}
}

func TestStargazersUseStarMediaTypeAndDates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/vnd.github.star+json" {
			t.Errorf("stargazer media type=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"starred_at":"2026-08-01T00:00:00Z","user":{"login":"star-user"}}]`)
	}))
	defer server.Close()
	out, err := readGitHubStargazers(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/stargazers"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[@star-user](https://github.com/star-user)") || !strings.Contains(out, "starred 2026-08-01") || !strings.Contains(out, "not the subscriber count") {
		t.Fatalf("stargazer output incorrect:\n%s", out)
	}
}

func TestWatchersUseSubscribersEndpointAndNaming(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"login":"actual-watcher"}]`)
	}))
	defer server.Close()
	out, err := readGitHubWatchers(context.Background(), testGitHubClient(server.URL, server.URL, ""), parseGitHubTarget("https://github.com/o/r/watchers"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotPath, "/subscribers") || !strings.Contains(out, "# Watchers / subscribers") || !strings.Contains(out, "actual repository watchers/subscribers, not stars") || !strings.Contains(out, "@actual-watcher") {
		t.Fatalf("watchers/subscribers output wrong path=%s:\n%s", gotPath, out)
	}
}

func TestRefReleaseSocialQueriesRejectUnmappedParameters(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&calls, 1) }))
	defer server.Close()
	client := testGitHubClient(server.URL, server.URL, "")
	for _, target := range []*GitHubTarget{
		parseGitHubTarget("https://github.com/o/r/branches?query=foo"),
		parseGitHubTarget("https://github.com/o/r/tags?sort=newest"),
		parseGitHubTarget("https://github.com/o/r/stargazers?foo=bar"),
	} {
		result := readGitHubNativeWithClient(context.Background(), target, client)
		if result.Outcome != GitHubNativeFailure || result.Err == nil || !strings.Contains(result.Err.Error(), "not supported") {
			t.Fatalf("unmapped list query not rejected: %#v", result)
		}
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("unmapped queries made provider calls")
	}
}
