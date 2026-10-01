//go:build linux || darwin

package project

import (
	"os"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/hangxie/rapidgo/internal/i18n"
)

func copyFileMetadata(path string, info os.FileInfo, temporary *os.File) error {
	current, err := openRegularFile(path)
	if err != nil {
		return i18n.Errorf("msg_open_metadata_source_w", err)
	}
	defer func() { _ = current.Close() }()
	currentInfo, err := current.Stat()
	if err != nil {
		return i18n.Errorf("msg_stat_metadata_source_w", err)
	}
	if !os.SameFile(info, currentInfo) {
		return ErrFileChanged
	}
	size, err := unix.Flistxattr(int(current.Fd()), nil)
	if err != nil {
		return i18n.Errorf("msg_list_extended_attributes_w", err)
	}
	names := make([]byte, size)
	size, err = unix.Flistxattr(int(current.Fd()), names)
	if err != nil {
		return i18n.Errorf("msg_list_extended_attributes_w", err)
	}
	for _, name := range strings.Split(string(names[:size]), "\x00") {
		if name == "" {
			continue
		}
		length, err := unix.Fgetxattr(int(current.Fd()), name, nil)
		if err != nil {
			return i18n.Errorf("msg_read_extended_attribute_q_w", name, err)
		}
		value := make([]byte, length)
		length, err = unix.Fgetxattr(int(current.Fd()), name, value)
		if err != nil {
			return i18n.Errorf("msg_read_extended_attribute_q_w", name, err)
		}
		if err := unix.Fsetxattr(int(temporary.Fd()), name, value[:length], 0); err != nil {
			return i18n.Errorf("msg_restore_extended_attribute_q_w", name, err)
		}
	}
	return nil
}
