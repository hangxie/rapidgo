// Package cli implements RapidGo's command-line boundary.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hangxie/rapidgo/internal/buildinfo"
)

const usage = `Usage: rapidgo [options] [directory]

Open a Go project in RapidGo. If directory is omitted, the current directory is used.

Options:
  -h, --help       Show this help
  -v, --version    Show version information
`

// Run parses arguments and runs the command. It returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("rapidgo", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }

	var showVersion bool
	flags.BoolVar(&showVersion, "version", false, "show version information")
	flags.BoolVar(&showVersion, "v", false, "show version information")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "rapidgo %s (commit %s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
		return 0
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(stderr, "rapidgo: expected at most one directory")
		return 2
	}

	root := "."
	if flags.NArg() == 1 {
		root = flags.Arg(0)
	}
	absoluteRoot, err := validateRoot(root)
	if err != nil {
		fmt.Fprintf(stderr, "rapidgo: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "RapidGo project: %s\n", absoluteRoot)
	fmt.Fprintln(stdout, "The interactive editor is not implemented yet; see docs/MVP.md for the v0.1 scope.")
	return 0
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
