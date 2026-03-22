package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/amxv/webctx/internal/buildinfo"
)

const commandName = "webctx"

var version = buildinfo.CurrentVersion()

func Run(args []string, stdout, stderr io.Writer) int {
	loadEnvLocal()

	if len(args) == 0 || isHelpArg(args[0]) {
		_, _ = fmt.Fprintln(stdout, usageText())
		return 0
	}

	if args[0] == "--version" || args[0] == "-v" {
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	}

	tool := args[0]
	flags, positional := parseArgs(args[1:])
	input := ""
	if len(positional) > 0 {
		input = positional[0]
	}

	switch tool {
	case "search":
		query := strings.Join(positional, " ")
		if strings.TrimSpace(query) == "" {
			_, _ = fmt.Fprintln(stderr, "Error: search requires a query")
			_, _ = fmt.Fprintln(stdout, "Usage: webctx search <query> [--exclude domains] [--keyword phrase]")
			return 1
		}
		excludeDomains := splitCSV(flags["exclude"])
		text, err := Search(SearchParams{Query: query, ExcludeDomains: excludeDomains, IncludeKeyword: flags["keyword"]})
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return 1
		}
		_, _ = fmt.Fprintln(stdout, text)
		return 0
	case "read-link":
		if strings.TrimSpace(input) == "" {
			_, _ = fmt.Fprintln(stderr, "Error: read-link requires a URL")
			_, _ = fmt.Fprintln(stdout, "Usage: webctx read-link <url>")
			return 1
		}
		text, err := ReadLink(input)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return 1
		}
		_, _ = fmt.Fprintln(stdout, text)
		return 0
	case "map-site":
		if strings.TrimSpace(input) == "" {
			_, _ = fmt.Fprintln(stderr, "Error: map-site requires a URL")
			_, _ = fmt.Fprintln(stdout, "Usage: webctx map-site <url>")
			return 1
		}
		text, err := MapSite(input)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return 1
		}
		_, _ = fmt.Fprintln(stdout, text)
		return 0
	default:
		_, _ = fmt.Fprintln(stderr, "Unknown tool:", tool)
		_, _ = fmt.Fprintln(stdout, usageText())
		return 1
	}
}

func usageText() string {
	return fmt.Sprintf(`webctx v%s - Web search & browsing CLI

Usage:
  webctx search <query> [--exclude domain1,domain2] [--keyword phrase]
  webctx read-link <url>
  webctx map-site <url>

Examples:
  webctx search "next.js server components"
  webctx search "react hooks" --exclude youtube.com,vimeo.com
  webctx search "drizzle orm" --keyword "migration guide"
  webctx read-link https://docs.example.com/guide
  webctx map-site https://example.com`, version)
}

func parseArgs(args []string) (map[string]string, []string) {
	flags := map[string]string{}
	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") && i+1 < len(args) {
			flags[strings.TrimPrefix(args[i], "--")] = args[i+1]
			i++
			continue
		}
		positional = append(positional, args[i])
	}
	return flags, positional
}

func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isHelpArg(v string) bool {
	switch v {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}
