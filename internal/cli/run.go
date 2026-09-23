// Package cli implements RapidGo's command-line boundary.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/hangxie/rapidgo/internal/buildinfo"
)

type options struct {
	Help      bool   `help:"Show this help." short:"h"`
	Version   bool   `help:"Show version information." short:"v"`
	Directory string `arg:"" default:"." help:"Project directory to open." name:"directory" optional:""`
}

// Run parses arguments and runs the command. It returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	command := options{}
	parser, err := kong.New(
		&command,
		kong.Name("rapidgo"),
		kong.Description("A lightweight, keyboard-first terminal IDE for Go."),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
		kong.NoDefaultHelp(),
		kong.Writers(stdout, stderr),
	)
	if err != nil {
		writeError(stderr, "rapidgo: initialize command parser: %v\n", err)
		return 1
	}

	context, err := parser.Parse(args)
	if err != nil {
		writeError(stderr, "rapidgo: %v\n", err)
		return 2
	}
	if command.Help {
		if err := context.PrintUsage(false); err != nil {
			writeError(stderr, "rapidgo: render help: %v\n", err)
			return 1
		}
		return 0
	}
	if command.Version {
		if _, err := fmt.Fprintf(stdout, "rapidgo %s (commit %s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date); err != nil {
			writeError(stderr, "rapidgo: write version: %v\n", err)
			return 1
		}
		return 0
	}

	absoluteRoot, err := validateRoot(command.Directory)
	if err != nil {
		writeError(stderr, "rapidgo: %v\n", err)
		return 1
	}

	if _, err := fmt.Fprintf(stdout, "RapidGo project: %s\n", absoluteRoot); err != nil {
		writeError(stderr, "rapidgo: write project path: %v\n", err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, "The interactive editor is not implemented yet; see docs/MVP.md for the v0.1 scope."); err != nil {
		writeError(stderr, "rapidgo: write status: %v\n", err)
		return 1
	}
	return 0
}

func writeError(writer io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(writer, format, args...)
}

func validateRoot(root string) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return "", fmt.Errorf("open project directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory: %s", root)
	}
	return filepath.Clean(absoluteRoot), nil
}
