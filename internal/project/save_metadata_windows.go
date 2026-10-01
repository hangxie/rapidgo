package project

import (
	"os"

	"golang.org/x/sys/windows"

	"github.com/hangxie/rapidgo/internal/i18n"
)

func prepareReplacement(path string, info os.FileInfo, temporary *os.File) error {
	current, err := openRegularFile(path)
	if err != nil {
		return err
	}
	defer func() { _ = current.Close() }()
	currentInfo, err := current.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, currentInfo) {
		return ErrFileChanged
	}
	var details windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(current.Fd()), &details); err != nil {
		return i18n.Errorf("msg_inspect_hard_links_w", err)
	}
	if details.NumberOfLinks != 1 {
		return ErrMultipleLinks
	}
	return temporary.Chmod(info.Mode().Perm())
}
