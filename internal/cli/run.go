// Package cli implements RapidGo's command-line boundary.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/hangxie/rapidgo/internal/buildinfo"
	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/ui"
)

type options struct {
	Help      bool   `help:"${help}" short:"h"`
	Version   bool   `help:"${version}" short:"v"`
	Directory string `arg:"" default:"." help:"${directory}" name:"directory" optional:""`
}

// Run parses arguments and runs the command. It returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	catalog, err := i18n.FromEnvironment(os.Getenv)
	if err != nil {
		writeError(stderr, "msg_rapidgo_v", err)
		return 1
	}
	i18n.Use(catalog)
	return run(args, stdout, stderr, ui.Run)
}

func run(args []string, stdout, stderr io.Writer, launch func(string) error) int {
	command := options{}
	parser, err := kong.New(
		&command,
		kong.Name("rapidgo"),
		kong.Vars{"help": i18n.Text("cli_help"), "version": i18n.Text("cli_version"), "directory": i18n.Text("cli_directory")},
		kong.Description(i18n.Text("msg_a_lightweight_keyboard_first_terminal_ide_for_go")),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
		kong.NoDefaultHelp(),
		kong.Writers(stdout, stderr),
	)
	if err != nil {
		writeError(stderr, "msg_rapidgo_initialize_command_parser_v", err)
		return 1
	}

	context, err := parser.Parse(args)
	if err != nil {
		writeError(stderr, "msg_rapidgo_v", err)
		return 2
	}
	if command.Help {
		if err := context.PrintUsage(false); err != nil {
			writeError(stderr, "msg_rapidgo_render_help_v", err)
			return 1
		}
		return 0
	}
	if command.Version {
		if _, err := fmt.Fprint(stdout, i18n.Format("msg_rapidgo_s", buildinfo.Describe())); err != nil {
			writeError(stderr, "msg_rapidgo_write_version_v", err)
			return 1
		}
		return 0
	}

	absoluteRoot, err := validateRoot(command.Directory)
	if err != nil {
		writeError(stderr, "msg_rapidgo_v", err)
		return 1
	}

	if err := launch(absoluteRoot); err != nil {
		writeError(stderr, "msg_rapidgo_start_terminal_ui_v", err)
		return 1
	}
	return 0
}

func writeError(writer io.Writer, key string, args ...any) {
	_, _ = fmt.Fprint(writer, i18n.Format(key, args...))
}

func validateRoot(root string) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", i18n.Errorf("msg_resolve_project_directory_w", err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return "", i18n.Errorf("msg_open_project_directory_w", err)
	}
	if !info.IsDir() {
		return "", i18n.Errorf("msg_project_path_is_not_a_directory_s", root)
	}
	return filepath.Clean(absoluteRoot), nil
}
