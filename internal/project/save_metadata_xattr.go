//go:build linux || darwin

package project

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func copyFileMetadata(path string, info os.FileInfo, temporary *os.File) error {
	current, err := openRegularFile(path)
	if err != nil {
		return fmt.Errorf("open metadata source: %w", err)
	}
	defer func() { _ = current.Close() }()
	currentInfo, err := current.Stat()
	if err != nil {
		return fmt.Errorf("stat metadata source: %w", err)
	}
	if !os.SameFile(info, currentInfo) {
		return ErrFileChanged
	}
	size, err := unix.Flistxattr(int(current.Fd()), nil)
	if err != nil {
		return fmt.Errorf("list extended attributes: %w", err)
	}
	names := make([]byte, size)
	size, err = unix.Flistxattr(int(current.Fd()), names)
	if err != nil {
		return fmt.Errorf("list extended attributes: %w", err)
	}
	for _, name := range strings.Split(string(names[:size]), "\x00") {
		if name == "" {
			continue
		}
		length, err := unix.Fgetxattr(int(current.Fd()), name, nil)
		if err != nil {
			return fmt.Errorf("read extended attribute %q: %w", name, err)
		}
		value := make([]byte, length)
		length, err = unix.Fgetxattr(int(current.Fd()), name, value)
		if err != nil {
			return fmt.Errorf("read extended attribute %q: %w", name, err)
		}
		if err := unix.Fsetxattr(int(temporary.Fd()), name, value[:length], 0); err != nil {
			return fmt.Errorf("restore extended attribute %q: %w", name, err)
		}
	}
	return nil
}
