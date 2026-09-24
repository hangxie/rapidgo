package ui

import (
	"context"

	"github.com/hangxie/rapidgo/internal/project"
)

type workKind uint8

const (
	listDirectory workKind = iota
	openFile
)

type workRequest struct {
	kind workKind
	path string
	node *project.Node
	seq  uint64
}

type workResult struct {
	request  workRequest
	entries  []project.Entry
	document project.Document
	err      error
}

func projectWorker(ctx context.Context, source project.Source, requests <-chan workRequest, results chan<- workResult) {
	for {
		var request workRequest
		select {
		case <-ctx.Done():
			return
		case next, ok := <-requests:
			if !ok {
				return
			}
			request = next
		}
		result := workResult{request: request}
		if request.kind == listDirectory {
			result.entries, result.err = source.List(ctx, request.path)
		} else {
			result.document, result.err = source.Open(ctx, request.path)
		}
		select {
		case <-ctx.Done():
			return
		case results <- result:
		}
	}
}
