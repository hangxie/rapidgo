//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package project

import (
	"os"
	"syscall"

	"github.com/hangxie/rapidgo/internal/i18n"
)

func prepareReplacement(path string, info os.FileInfo, temporary *os.File) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return i18n.Error("msg_cannot_inspect_file_links_and_ownership")
	}
	if stat.Nlink != 1 {
		return ErrMultipleLinks
	}
	tempInfo, err := temporary.Stat()
	if err != nil {
		return err
	}
	tempStat, ok := tempInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return i18n.Error("msg_cannot_inspect_temporary_file_ownership")
	}
	if stat.Uid != tempStat.Uid || stat.Gid != tempStat.Gid {
		if err := temporary.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
			return i18n.Errorf("msg_restore_owner_and_group_w", err)
		}
	}
	// Chown can clear these bits, so apply them only after ownership is set.
	mode := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if err := temporary.Chmod(mode); err != nil {
		return i18n.Errorf("msg_restore_mode_w", err)
	}
	if err := copyFileMetadata(path, info, temporary); err != nil {
		return err
	}
	return nil
}
