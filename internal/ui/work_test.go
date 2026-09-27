package ui

import (
	"context"
	"testing"
	"time"

	"github.com/hangxie/rapidgo/internal/project"
)

type readySource struct{ listed chan struct{} }

func (source readySource) List(context.Context, string) ([]project.Entry, error) {
	close(source.listed)
	return []project.Entry{{Name: "main.go"}}, nil
}

func (readySource) Open(context.Context, string) (project.Document, error) {
	return project.Document{}, nil
}

func TestProjectWorkerCancelsWhileResultIsUnread(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := readySource{listed: make(chan struct{})}
	requests := make(chan workRequest, 1)
	results := make(chan workResult)
	done := make(chan struct{})
	requests <- workRequest{kind: listDirectory, path: "/work"}
	go func() {
		projectWorker(ctx, source, nil, requests, results)
		close(done)
	}()
	select {
	case <-source.listed:
	case <-time.After(2 * time.Second):
		t.Fatal("directory scan did not finish")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop while its result was unread")
	}
	select {
	case result := <-results:
		t.Fatalf("cancelled result was delivered: %+v", result)
	default:
	}
}

func TestProjectWorkerStopsWhenRequestsClose(t *testing.T) {
	t.Parallel()

	requests := make(chan workRequest)
	close(requests)
	results := make(chan workResult)
	done := make(chan struct{})
	go func() {
		projectWorker(context.Background(), nil, nil, requests, results)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after the request channel closed")
	}
}
