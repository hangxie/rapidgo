package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxPreviewBytes = 4 << 20

// Document is a read-only UTF-8 preview. Editing state is a later milestone.
type Document struct {
	Path  string
	Lines []string
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
		return nil, fmt.Errorf("read directory %q: %w", path, err)
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
		return Document{}, fmt.Errorf("inspect file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Document{}, fmt.Errorf("file %q is not a regular file", path)
	}
	file, err := openRegularFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("open file %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxPreviewBytes+1))
	if err != nil {
		return Document{}, fmt.Errorf("read file %q: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if len(data) > MaxPreviewBytes {
		return Document{}, fmt.Errorf("file %q exceeds %d-byte preview limit", path, MaxPreviewBytes)
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return Document{}, fmt.Errorf("file %q is not UTF-8 text", path)
	}
	lines := strings.Split(string(data), "\n")
	for index, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		lines[index] = strings.Map(func(char rune) rune {
			if char == '\t' {
				return char
			}
			if unicode.IsControl(char) {
				return '�'
			}
			return char
		}, line)
	}
	return Document{Path: path, Lines: lines}, nil
}

func checkRegularFile(file *os.File) (*os.File, error) {
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("not a regular file")
	}
	return file, nil
}
