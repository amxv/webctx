package app

import (
	"fmt"
	"regexp"
	"strings"
)

var githubLicenseField = regexp.MustCompile(`(?mi)^license:\s*["']?([A-Za-z0-9.+-]+)["']?\s*$`)
var repositoryLicenseURL = regexp.MustCompile(`https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/blob/[A-Za-z0-9_.-]+/(?:LICENSE|COPYING)(?:\.md|\.txt)?`)

func directlyAnsweredMetadata(rawURL, question, markdown string) (string, bool) {
	q := strings.ToLower(question)
	if !strings.Contains(q, "license") || !strings.Contains(strings.ToLower(rawURL), "github.com/") {
		return "", false
	}
	match := githubLicenseField.FindStringSubmatch(markdown)
	if len(match) != 2 {
		return "", false
	}
	value := match[1]
	evidenceURL := rawURL
	if source := repositoryLicenseURL.FindString(markdown); source != "" {
		evidenceURL = source
	}
	return fmt.Sprintf(
		"## Answer\n\nThe repository lists its license as **%s**.\n\n**Source:** %s\n\n**Evidence:** GitHub repository metadata lists `license: %q`.\n",
		value, evidenceURL, value,
	), true
}
