//go:build aix || dragonfly || freebsd || illumos || netbsd || openbsd || solaris

package project

import "os"

func copyFileMetadata(string, os.FileInfo, *os.File) error { return nil }
