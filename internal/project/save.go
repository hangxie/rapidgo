package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	ErrFileChanged   = errors.New("file changed on disk since it was opened")
	ErrMultipleLinks = errors.New("file has multiple hard links; replacement would break them")
)

type SaveRequest struct {
	Path, Original, Content string
}

type SaveResult struct {
	Content string // bytes written to disk, including any gofmt changes
}

type Saver interface {
	Save(context.Context, SaveRequest) (SaveResult, error)
}

// Formatter and Writer are separate so formatting and write failures can be
// tested without mutating a real file or depending on the installed gofmt.
type Formatter interface {
	Format(context.Context, string) (string, error)
}

type Writer interface {
	Write(context.Context, string, string, string) error
}

type DiskSaver struct {
	Formatter Formatter
	Writer    Writer
}

func (s DiskSaver) Save(ctx context.Context, request SaveRequest) (SaveResult, error) {
	if err := ctx.Err(); err != nil {
		return SaveResult{}, err
	}
	content := request.Content
	if filepath.Ext(request.Path) == ".go" {
		formatter := s.Formatter
		if formatter == nil {
			formatter = GoFmt{}
		}
		formatted, err := formatter.Format(ctx, content)
		if err != nil {
			return SaveResult{}, fmt.Errorf("format failed: %w", err)
		}
		content = formatted
	}
	writer := s.Writer
	if writer == nil {
		writer = AtomicWriter{}
	}
	if err := writer.Write(ctx, request.Path, request.Original, content); err != nil {
		return SaveResult{}, fmt.Errorf("write failed: %w", err)
	}
	return SaveResult{Content: content}, nil
}

// GoFmt invokes the user's gofmt executable without a shell.
type GoFmt struct{}

func (GoFmt) Format(ctx context.Context, content string) (string, error) {
	command := exec.CommandContext(ctx, "gofmt")
	command.Stdin = bytes.NewBufferString(content)
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("gofmt: %s: %w", bytes.TrimSpace(exit.Stderr), err)
		}
		return "", fmt.Errorf("gofmt: %w", err)
	}
	return string(output), nil
}

// AtomicWriter checks the originally opened bytes, writes a sibling temporary
// file, and replaces the destination only after a complete successful write.
type AtomicWriter struct{}

func (AtomicWriter) Write(ctx context.Context, path, original, content string) error {
	info, err := verifyOriginal(ctx, path, original)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".rapidgo-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	if _, err := io.WriteString(temporary, content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	latest, err := verifyOriginal(ctx, path, original)
	if err != nil {
		_ = temporary.Close()
		return err
	}
	if !os.SameFile(info, latest) {
		_ = temporary.Close()
		return ErrFileChanged
	}
	if err := prepareReplacement(path, latest, temporary); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("preserve file metadata: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}

func verifyOriginal(ctx context.Context, path, original string) (os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, err := openRegularFile(path)
	if err != nil {
		return nil, fmt.Errorf("open current file: %w", err)
	}
	info, err := current.Stat()
	if err != nil {
		_ = current.Close()
		return nil, fmt.Errorf("stat current file: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(current, MaxOpenBytes+1))
	closeErr := current.Close()
	if err != nil {
		return nil, fmt.Errorf("read current file: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close current file: %w", closeErr)
	}
	if string(data) != original {
		return nil, ErrFileChanged
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return info, nil
}
