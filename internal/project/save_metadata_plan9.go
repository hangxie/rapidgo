package project

import "os"

func prepareReplacement(_ string, info os.FileInfo, temporary *os.File) error {
	return temporary.Chmod(info.Mode().Perm())
}
