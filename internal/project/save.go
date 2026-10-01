package project

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hangxie/rapidgo/internal/i18n"
)

var (
	ErrFileChanged   = i18n.Error("msg_file_changed_on_disk_since_it_was_opened")
	ErrMultipleLinks = i18n.Error("msg_file_has_multiple_hard_links_replacement_would_break_them")
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

// Formatter and Writer are separate so each failure is testable alone.
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
			return SaveResult{}, i18n.Errorf("msg_format_failed_w", err)
		}
		content = formatted
	}
	writer := s.Writer
	if writer == nil {
		writer = AtomicWriter{}
	}
	if err := writer.Write(ctx, request.Path, request.Original, content); err != nil {
		return SaveResult{}, i18n.Errorf("msg_write_failed_w", err)
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
			return "", i18n.Errorf("msg_gofmt_s_w", bytes.TrimSpace(exit.Stderr), err)
		}
		return "", i18n.Errorf("msg_gofmt_w", err)
	}
	return string(output), nil
}

// AtomicWriter replaces the destination only after a complete write.
type AtomicWriter struct{}

func (AtomicWriter) Write(ctx context.Context, path, original, content string) error {
	info, err := verifyOriginal(ctx, path, original)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".rapidgo-*")
	if err != nil {
		return i18n.Errorf("msg_create_temporary_file_w", err)
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	if _, err := io.WriteString(temporary, content); err != nil {
		_ = temporary.Close()
		return i18n.Errorf("msg_write_temporary_file_w", err)
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
		return i18n.Errorf("msg_preserve_file_metadata_w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return i18n.Errorf("msg_sync_temporary_file_w", err)
	}
	if err := temporary.Close(); err != nil {
		return i18n.Errorf("msg_close_temporary_file_w", err)
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return i18n.Errorf("msg_replace_file_w", err)
	}
	return nil
}

func verifyOriginal(ctx context.Context, path, original string) (os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, err := openRegularFile(path)
	if err != nil {
		return nil, i18n.Errorf("msg_open_current_file_w", err)
	}
	info, err := current.Stat()
	if err != nil {
		_ = current.Close()
		return nil, i18n.Errorf("msg_stat_current_file_w", err)
	}
	data, err := io.ReadAll(io.LimitReader(current, MaxOpenBytes+1))
	closeErr := current.Close()
	if err != nil {
		return nil, i18n.Errorf("msg_read_current_file_w", err)
	}
	if closeErr != nil {
		return nil, i18n.Errorf("msg_close_current_file_w", closeErr)
	}
	if string(data) != original {
		return nil, ErrFileChanged
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return info, nil
}
