//go:build plan9

package project

import "os"

func openRegularFile(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return checkRegularFile(file)
}
