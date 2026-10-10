package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
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
			_, _ = fmt.Fprintln(stdout, "Usage: webctx read-link <url> [--question 'what to find']")
			return 1
		}
		text, err := ReadLinkFocused(input, flags["question"])
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
	case "research":
		research := ResearchDataInput{
			Query:      strings.Join(positional, " "),
			Mode:       flags["mode"],
			URLs:       splitCSV(flags["urls"]),
			Sources:    splitCSV(flags["sources"]),
			Categories: splitCSV(flags["categories"]),
			Groups:     splitCSV(flags["groups"]),
			Operations: splitCSV(flags["operations"]),
			View:       flags["view"],
			Include:    splitCSV(flags["include"]),
			Raw:        flags["raw"] == "true",
		}
		if q := flags["query"]; q != "" {
			research.Query = q
		}
		if flag := flags["limit"]; flag != "" {
			n, e := strconv.Atoi(flag)
			if e != nil {
				return cliDataError(stderr, dataError("invalid_limit", "--limit requires an integer."))
			}
			research.Limit = n
		}
		if flag := flags["offset"]; flag != "" {
			n, e := strconv.Atoi(flag)
			if e != nil {
				return cliDataError(stderr, dataError("invalid_offset", "--offset requires an integer."))
			}
			research.Offset = n
		}
		result, e := ResearchData(research)
		if e != nil {
			return cliDataError(stderr, e)
		}
		writeDataJSON(stdout, result)
		return 0
	case "inspect":
		if strings.TrimSpace(input) == "" {
			return cliDataError(stderr, dataError("missing_id", "Usage: webctx inspect <operation-or-source-id> [--raw]"))
		}
		result, e := InspectData(InspectDataInput{ID: input, Include: splitCSV(flags["include"]), Raw: flags["raw"] == "true"})
		if e != nil {
			return cliDataError(stderr, e)
		}
		writeDataJSON(stdout, result)
		return 0
	case "execute":
		request := ExecuteDataInput{Raw: flags["raw"] == "true", RequestID: flags["request-id"]}
		if _, supplied := flags["max-credits"]; supplied {
			return cliDataError(stderr, dataError("unsupported_flag", "Credit limits are managed by the server; remove --max-credits."))
		}
		if jsonCalls := flags["calls"]; jsonCalls != "" {
			data, e := cliJSONString(jsonCalls)
			if e != nil {
				return cliDataError(stderr, e)
			}
			if e := json.Unmarshal([]byte(data), &request.Calls); e != nil {
				return cliDataError(stderr, dataError("invalid_calls", "--calls must be a JSON array of {id,inputs} objects."))
			}
		} else if strings.TrimSpace(input) != "" {
			var inputs map[string]any
			if source := flags["inputs"]; source != "" {
				data, e := cliJSONString(source)
				if e != nil {
					return cliDataError(stderr, e)
				}
				if e := json.Unmarshal([]byte(data), &inputs); e != nil {
					return cliDataError(stderr, dataError("invalid_inputs", "--inputs must be a JSON object."))
				}
			}
			request.Calls = []DataCall{{ID: input, Inputs: inputs}}
		} else {
			return cliDataError(stderr, dataError("missing_calls", "Usage: webctx execute <operation-id> --inputs JSON or --calls JSON."))
		}
		result, e := ExecuteData(request)
		if e != nil {
			return cliDataError(stderr, e)
		}
		writeDataJSON(stdout, result)
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
  webctx read-link <url> [--question 'what to find']
  webctx map-site <url>
  webctx research [question] [--view sources|groups|operations] [--sources IDs] [--include inputs,output,examples] [--limit N] [--offset N]
  webctx inspect <operation-or-source-id> [--raw]
  webctx execute <operation-id> --inputs '{"field":"value"}' [--request-id ID]
  webctx execute --calls '[{"id":"source/operation","inputs":{}}]'

Examples:
  webctx search "next.js server components"
  webctx search "react hooks" --exclude youtube.com,vimeo.com
  webctx search "drizzle orm" --keyword "migration guide"
  webctx research "podcast conversations about AI agents"
  webctx research --view sources --limit 5
  webctx inspect particle/podcasts/episodes/search
  webctx execute particle/podcasts/episodes/search --inputs '{"semantic_search":"AI agents","limit":2}'
  webctx read-link https://docs.example.com/guide
  webctx read-link https://docs.example.com/api --question "How do auth and pagination work?"
  webctx map-site https://example.com`, version)
}

func parseArgs(args []string) (map[string]string, []string) {
	flags := map[string]string{}
	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--raw" {
			flags["raw"] = "true"
			continue
		}
		if strings.HasPrefix(args[i], "--") {
			if k, v, ok := strings.Cut(strings.TrimPrefix(args[i], "--"), "="); ok {
				flags[k] = v
				continue
			}
		}
		if strings.HasPrefix(args[i], "--") && i+1 < len(args) {
			flags[strings.TrimPrefix(args[i], "--")] = args[i+1]
			i++
			continue
		}
		positional = append(positional, args[i])
	}
	return flags, positional
}

func cliJSONString(source string) (string, error) {
	if !strings.HasPrefix(source, "@") {
		return source, nil
	}
	path := strings.TrimPrefix(source, "@")
	if path == "" {
		return "", dataError("invalid_file", "The @file argument requires a path.")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", dataError("invalid_file", "Unable to read JSON input file: "+err.Error())
	}
	if len(b) > 1<<20 {
		return "", dataError("invalid_file", "JSON input file exceeds 1 MiB.")
	}
	return string(b), nil
}

func writeDataJSON(out io.Writer, value any) {
	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintln(out, "{}")
		return
	}
	_, _ = fmt.Fprintln(out, string(bytes))
}

func cliDataError(stderr io.Writer, err error) int {
	writeDataJSON(stderr, dataRawOrError(err))
	return 1
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
