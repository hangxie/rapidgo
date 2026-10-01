package ui

import (
	"context"
	"path/filepath"
	"unicode/utf16"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/gopls"
	"github.com/hangxie/rapidgo/internal/i18n"
)

type languageSession interface {
	Open(string, string) error
	Change(string, string) error
	CloseDocument(string) error
	Hover(context.Context, string, gopls.Position) (string, error)
	Complete(context.Context, string, gopls.Position) ([]gopls.CompletionItem, error)
	Definition(context.Context, string, gopls.Position) ([]gopls.Location, error)
	References(context.Context, string, gopls.Position) ([]gopls.Location, error)
	Diagnostics() <-chan gopls.PublishedDiagnostics
	Close() error
}

type languageSnapshot struct {
	path, text string
	seq        uint64
}

type languageEventKind uint8

const (
	languageStarting languageEventKind = iota
	languageReady
	languageSynced
	languagePublished
	languageUnavailable
	languageHovered
	languageCompleted
	languageNavigated
)

type languageEvent struct {
	kind       languageEventKind
	path       string
	text       string
	version    int
	published  gopls.PublishedDiagnostics
	hover      languageHoverResult
	completion languageCompletionResult
	navigation languageNavigationResult
	err        error
}

func sendLanguageEvent(ctx context.Context, events chan<- languageEvent, event languageEvent) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

type languageConnection struct {
	session languageSession
	current languageSnapshot
	version int
	ctx     context.Context
	events  chan<- languageEvent
}

func (connection *languageConnection) apply(next languageSnapshot) error {
	if next.seq < connection.current.seq {
		return nil
	}
	if next == connection.current {
		return nil
	}
	if connection.current.path != "" && next.path != connection.current.path {
		if err := connection.session.CloseDocument(connection.current.path); err != nil {
			return i18n.Errorf("msg_close_s_in_gopls_w", connection.current.path, err)
		}
	}
	if next.path == "" {
		connection.current = next
		connection.version = 0
		return nil
	}
	if next.path != connection.current.path {
		if err := connection.session.Open(next.path, next.text); err != nil {
			return i18n.Errorf("msg_open_s_in_gopls_w", next.path, err)
		}
		connection.version = 1
	} else {
		if err := connection.session.Change(next.path, next.text); err != nil {
			return i18n.Errorf("msg_sync_s_with_gopls_w", next.path, err)
		}
		connection.version++
	}
	connection.current = next
	if !sendLanguageEvent(connection.ctx, connection.events, languageEvent{kind: languageSynced, path: next.path, text: next.text, version: connection.version}) {
		return connection.ctx.Err()
	}
	return nil
}

func languageWorker(ctx context.Context, root string, requests <-chan languageSnapshot, hovers <-chan languageHoverRequest, completions <-chan languageCompletionRequest, navigations <-chan languageNavigationRequest, events chan<- languageEvent, start func(context.Context, string) (languageSession, error)) {
	var first languageSnapshot
	for first.path == "" {
		select {
		case <-ctx.Done():
			return
		case first = <-requests:
		}
	}
	if !sendLanguageEvent(ctx, events, languageEvent{kind: languageStarting}) {
		return
	}
	session, err := start(ctx, root)
	if err != nil {
		sendLanguageEvent(ctx, events, languageEvent{kind: languageUnavailable, err: err})
		return
	}
	defer func() { _ = session.Close() }()
	if !sendLanguageEvent(ctx, events, languageEvent{kind: languageReady}) {
		return
	}
	connection := languageConnection{session: session, ctx: ctx, events: events}
	if err := connection.apply(first); err != nil {
		sendLanguageEvent(ctx, events, languageEvent{kind: languageUnavailable, err: err})
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case next := <-requests:
			if err := connection.apply(next); err != nil {
				sendLanguageEvent(ctx, events, languageEvent{kind: languageUnavailable, err: err})
				return
			}
		case request := <-hovers:
			connection.hover(request)
		case request := <-completions:
			connection.complete(request)
		case request := <-navigations:
			connection.navigate(request)
		case published, ok := <-session.Diagnostics():
			if !ok {
				sendLanguageEvent(ctx, events, languageEvent{kind: languageUnavailable, err: i18n.Error("msg_gopls_connection_closed")})
				return
			}
			if !sendLanguageEvent(ctx, events, languageEvent{kind: languagePublished, published: published}) {
				return
			}
		}
	}
}

func (state *shellState) syncLanguage() {
	if state.enqueueLanguage == nil {
		return
	}
	next := languageSnapshot{}
	if state.document != nil && state.buffer != nil && filepath.Ext(state.document.Path) == ".go" {
		next = languageSnapshot{path: state.document.Path, text: state.buffer.Text()}
		if state.languageStatus == "" {
			state.languageStatus = "starting"
		}
	}
	if next.path == state.languageQueued.path && next.text == state.languageQueued.text {
		return
	}
	next.seq = state.languageQueued.seq + 1
	state.languageQueued = next
	state.languageDiagnostics = nil
	state.languageVersion = 0
	if next.path != "" {
		delete(state.languageReports, next.path)
		state.refreshProblems()
	}
	state.enqueueLanguage(next)
}

func (state *shellState) applyLanguageEvent(event languageEvent) {
	switch event.kind {
	case languageStarting:
		state.languageStatus = "starting"
	case languageReady:
		state.languageStatus = "ready"
	case languageUnavailable:
		state.languageStatus = "unavailable"
		state.languageErr = event.err
		state.completionPending = false
		state.completionVisible = false
		state.languageDiagnostics = nil
		state.languageReports = nil
		state.refreshProblems()
	case languageSynced:
		if state.document != nil && state.buffer != nil && event.path == state.document.Path && event.text == state.buffer.Text() {
			state.languageVersion = event.version
			state.languageSyncedText = event.text
		}
	case languagePublished:
		state.applyLanguageDiagnostics(event.published)
	case languageHovered:
		state.applyHoverResult(event.hover)
	case languageCompleted:
		state.applyCompletionResult(event.completion)
	case languageNavigated:
		state.applyNavigationResult(event.navigation)
	}
}

func (state *shellState) applyLanguageDiagnostics(published gopls.PublishedDiagnostics) {
	if !projectFile(state.projectRoot, published.Path) {
		return
	}
	if state.document != nil && published.Path == state.document.Path && (state.buffer == nil || published.Version != state.languageVersion || state.languageSyncedText != state.buffer.Text()) {
		return
	}
	if state.languageReports == nil {
		state.languageReports = make(map[string]gopls.PublishedDiagnostics)
	}
	if previous, ok := state.languageReports[published.Path]; ok && published.Version > 0 && previous.Version > published.Version {
		return
	}
	if len(published.Items) == 0 {
		delete(state.languageReports, published.Path)
	} else {
		state.languageReports[published.Path] = published
	}
	state.refreshProblems()
}

func utf16ByteColumn(line string, character int) int {
	units := 0
	for offset, value := range line {
		if units+utf16.RuneLen(value) > character {
			return offset + 1
		}
		units += utf16.RuneLen(value)
	}
	return len(line) + 1
}

func (state *shellState) languageSummary() string {
	if state.languageStatus == "unavailable" {
		return i18n.Text("msg_gopls_unavailable")
	}
	if state.languageStatus == "starting" {
		return i18n.Text("msg_gopls_starting")
	}
	if state.languageStatus != "ready" {
		return ""
	}
	errors, warnings := 0, 0
	for _, item := range state.languageDiagnostics {
		switch item.Severity {
		case diagnostic.Error:
			errors++
		case diagnostic.Warning:
			warnings++
		}
	}
	if errors == 0 && warnings == 0 {
		return i18n.Text("msg_gopls_ready")
	}
	return i18n.Format("msg_gopls_d_error_d_warning", errors, warnings)
}

func (state *shellState) languageLineMessage() string {
	if state.document == nil || state.buffer == nil || filepath.Ext(state.document.Path) != ".go" {
		return ""
	}
	line := state.buffer.Cursor().Line + 1
	for _, item := range state.languageDiagnostics {
		if item.Line == line {
			return i18n.Format("msg_gopls_d_d_s", item.Line, item.Column, item.Message)
		}
	}
	return ""
}

func (state *shellState) languageMarker(line int) rune {
	for _, item := range state.languageDiagnostics {
		if item.Line == line+1 {
			return '!'
		}
	}
	return ' '
}
