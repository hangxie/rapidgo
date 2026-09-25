//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package project

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func prepareReplacement(path string, info os.FileInfo, temporary *os.File) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("cannot inspect file links and ownership")
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
		return errors.New("cannot inspect temporary file ownership")
	}
	if stat.Uid != tempStat.Uid || stat.Gid != tempStat.Gid {
		if err := temporary.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
			return fmt.Errorf("restore owner and group: %w", err)
		}
	}
	// Chown can clear these bits, so apply them only after ownership is set.
	mode := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if err := temporary.Chmod(mode); err != nil {
		return fmt.Errorf("restore mode: %w", err)
	}
	if err := copyFileMetadata(path, info, temporary); err != nil {
		return err
	}
	return nil
}
