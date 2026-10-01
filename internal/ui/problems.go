package ui

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/i18n"
)

type languageProblem struct {
	diagnostic.Diagnostic
	utf16Column int
}

func projectFile(root, path string) bool {
	if root == "" || !filepath.IsAbs(path) || filepath.Ext(path) != ".go" {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (state *shellState) refreshProblems() {
	state.problems = nil
	state.languageDiagnostics = nil
	paths := make([]string, 0, len(state.languageReports))
	for path := range state.languageReports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		for _, item := range state.languageReports[path].Items {
			if item.Range.Start.Line < 0 || item.Range.Start.Character < 0 {
				continue
			}
			severity := diagnostic.Info
			switch item.Severity {
			case 1:
				severity = diagnostic.Error
			case 2:
				severity = diagnostic.Warning
			}
			problem := languageProblem{
				Diagnostic:  diagnostic.Diagnostic{Path: path, Line: item.Range.Start.Line + 1, Severity: severity, Source: "gopls", Message: strings.TrimSpace(item.Message)},
				utf16Column: item.Range.Start.Character,
			}
			if state.document != nil && state.buffer != nil && path == state.document.Path {
				lines := state.buffer.Lines()
				if item.Range.Start.Line >= len(lines) {
					continue
				}
				problem.Column = utf16ByteColumn(lines[item.Range.Start.Line], item.Range.Start.Character)
				state.languageDiagnostics = append(state.languageDiagnostics, problem.Diagnostic)
			}
			state.problems = append(state.problems, problem)
		}
	}
}

func (state *shellState) jumpToLanguage(problem languageProblem) {
	path, ok := state.resolvePath(problem.Diagnostic)
	if !ok {
		state.message = i18n.Format("msg_cannot_locate_s", problem.Path)
		return
	}
	state.openAtTarget(path, jumpTarget{line: problem.Line - 1, utf16Column: problem.utf16Column, fromUTF16: true})
}
