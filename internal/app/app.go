package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/amxv/webctx/internal/buildinfo"
)

const commandName = "webctx"

var version = buildinfo.CurrentVersion()

func Run(args []string, stdout, stderr io.Writer) error {
	_ = stderr

	if len(args) == 0 || isHelpArg(args[0]) {
		printRootHelp(stdout)
		return nil
	}
	if len(args) == 1 && isVersionArg(args[0]) {
		_, _ = fmt.Fprintf(stdout, "%s %s\n", commandName, version)
		return nil
	}

	switch args[0] {
	case "hello":
		if len(args) > 1 && isHelpArg(args[1]) {
			printHelloHelp(stdout)
			return nil
		}
		name := "world"
		if len(args) > 1 {
			name = strings.TrimSpace(args[1])
			if name == "" {
				name = "world"
			}
		}
		_, _ = fmt.Fprintf(stdout, "Hello, %s!\n", name)
		return nil
	default:
		return fmt.Errorf("unknown command %q (run `%s --help`)", args[0], commandName)
	}
}

func isVersionArg(v string) bool {
	return v == "--version"
}

func isHelpArg(v string) bool {
	switch v {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func printRootHelp(w io.Writer) {
	writeLines(w,
		"webctx - Go CLI template",
		"",
		"Usage:",
		"  webctx [--version]",
		"  webctx <command> [arguments]",
		"",
		"Commands:",
		"  hello [name]    print a greeting",
		"",
		"Examples:",
		"  webctx --version",
		"  webctx hello",
		"  webctx hello agent",
	)
}

func printHelloHelp(w io.Writer) {
	writeLines(w,
		"webctx hello - print a greeting",
		"",
		"Usage:",
		"  webctx hello [name]",
		"",
		"Examples:",
		"  webctx hello",
		"  webctx hello Alice",
	)
}

func writeLines(w io.Writer, lines ...string) {
	for _, line := range lines {
		_, _ = fmt.Fprintln(w, line)
	}
}
