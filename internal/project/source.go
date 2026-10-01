package project

import (
	"bytes"
	"context"
	"io"
	"os"
	"unicode/utf8"

	"github.com/hangxie/rapidgo/internal/i18n"
)

const MaxOpenBytes = 4 << 20

// Document holds validated, unmodified UTF-8 source text for the editor.
type Document struct {
	Path string
	Text string
}

// Source allows filesystem work to run outside the terminal event loop.
type Source interface {
	List(context.Context, string) ([]Entry, error)
	Open(context.Context, string) (Document, error)
}

type DiskSource struct{}

func (DiskSource) List(ctx context.Context, path string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory, err := os.ReadDir(path)
	if err != nil {
		return nil, i18n.Errorf("msg_read_directory_q_w", path, err)
	}
	entries := make([]Entry, 0, len(directory))
	for _, item := range directory {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry := Entry{
			Name: item.Name(), IsDir: item.IsDir(), Symlink: item.Type()&os.ModeSymlink != 0,
			Regular: item.Type().IsRegular(),
		}
		if entry.Name == "go.mod" {
			info, err := item.Info()
			entry.Regular = err == nil && info.Mode().IsRegular()
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (DiskSource) Open(ctx context.Context, path string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Document{}, i18n.Errorf("msg_inspect_file_q_w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Document{}, i18n.Errorf("msg_file_q_is_not_a_regular_file", path)
	}
	file, err := openRegularFile(path)
	if err != nil {
		return Document{}, i18n.Errorf("msg_open_file_q_w", path, err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxOpenBytes+1))
	if err != nil {
		return Document{}, i18n.Errorf("msg_read_file_q_w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if len(data) > MaxOpenBytes {
		return Document{}, i18n.Errorf("msg_file_q_exceeds_d_byte_open_limit", path, MaxOpenBytes)
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return Document{}, i18n.Errorf("msg_file_q_is_not_utf_8_text", path)
	}
	return Document{Path: path, Text: string(data)}, nil
}

func checkRegularFile(file *os.File) (*os.File, error) {
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, i18n.Error("msg_not_a_regular_file")
	}
	return file, nil
}
