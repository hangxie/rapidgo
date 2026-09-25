package project

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
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
		return fmt.Errorf("inspect hard links: %w", err)
	}
	if details.NumberOfLinks != 1 {
		return ErrMultipleLinks
	}
	return temporary.Chmod(info.Mode().Perm())
}
