package jobs

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/hangxie/rapidgo/internal/i18n"
)

// Toolchain is the Go installation RapidGo found and invokes.
type Toolchain struct {
	Path    string // absolute path to the go executable
	Version string // e.g. "go1.26.0", or the raw output when unrecognized
}

// Describe renders the toolchain for the help surface.
func (t Toolchain) Describe() string {
	switch {
	case t.Path == "":
		return i18n.Text("go_not_found")
	case t.Version == "":
		return t.Path
	default:
		return t.Version + " (" + t.Path + ")"
	}
}

type lookupFunc func(string) (string, error)

// Detect locates the user's go executable and asks it for its version.
func Detect(ctx context.Context) (Toolchain, error) {
	return detect(ctx, exec.LookPath)
}

func detect(ctx context.Context, lookPath lookupFunc) (Toolchain, error) {
	path, err := lookPath("go")
	if err != nil {
		return Toolchain{}, i18n.Errorf("msg_locate_go_executable_w", err)
	}
	command := exec.CommandContext(ctx, path, "version")
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return Toolchain{Path: path}, i18n.Errorf("format_go_version_s_w", strings.TrimSpace(string(exit.Stderr)), err)
		}
		return Toolchain{Path: path}, i18n.Errorf("format_go_version_w", err)
	}
	return Toolchain{Path: path, Version: parseVersion(string(output))}, nil
}

// parseVersion reads the token from "go version go1.26.0 linux/amd64".
func parseVersion(output string) string {
	line := strings.TrimSpace(output)
	if index := strings.IndexAny(line, "\r\n"); index >= 0 {
		line = strings.TrimSpace(line[:index])
	}
	fields := strings.Fields(line)
	if len(fields) >= 3 && fields[0] == "go" && fields[1] == "version" {
		return fields[2]
	}
	return line
}
