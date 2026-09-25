package ui

import (
	"context"

	"github.com/hangxie/rapidgo/internal/project"
)

type workKind uint8

const (
	listDirectory workKind = iota
	openFile
	saveFile
)

type workRequest struct {
	kind            workKind
	path            string
	node            *project.Node
	seq             uint64
	focusSeq        uint64
	discardApproved bool
	approvedText    string
	save            project.SaveRequest
	revision        int
}

type workResult struct {
	request  workRequest
	entries  []project.Entry
	document project.Document
	saved    project.SaveResult
	err      error
}

func projectWorker(ctx context.Context, source project.Source, saver project.Saver, requests <-chan workRequest, results chan<- workResult) {
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
		switch request.kind {
		case listDirectory:
			result.entries, result.err = source.List(ctx, request.path)
		case openFile:
			result.document, result.err = source.Open(ctx, request.path)
		case saveFile:
			result.saved, result.err = saver.Save(ctx, request.save)
		}
		select {
		case <-ctx.Done():
			return
		case results <- result:
		}
	}
}
