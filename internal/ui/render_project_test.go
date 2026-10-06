package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/editor"
)

func TestDiagnosticLineHighlights(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		severities             []diagnostic.Severity
		foreground, background tcell.Color
	}{
		{"error", []diagnostic.Severity{diagnostic.Error}, turboWhite, turboRed},
		{"warning", []diagnostic.Severity{diagnostic.Warning}, turboWhite, turboBlack},
		{"info", []diagnostic.Severity{diagnostic.Info}, turboWhite, turboBlue},
		{"warning then error", []diagnostic.Severity{diagnostic.Warning, diagnostic.Error}, turboWhite, turboRed},
		{"error then warning", []diagnostic.Severity{diagnostic.Error, diagnostic.Warning}, turboWhite, turboRed},
		{"info then warning", []diagnostic.Severity{diagnostic.Info, diagnostic.Warning}, turboWhite, turboBlack},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			screen := tcell.NewSimulationScreen("")
			require.NoError(t, screen.Init())
			t.Cleanup(screen.Fini)
			screen.SetSize(40, 6)
			state := shellState{focus: focusEditor}
			setTestDocument(t, &state, "/tmp/main.go", "var n = 42 // comment\n\nplain")
			for _, severity := range test.severities {
				for _, line := range []int{1, 2} {
					state.languageDiagnostics = append(state.languageDiagnostics, diagnostic.Diagnostic{Line: line, Severity: severity})
				}
			}
			area := rectangle{width: 40, height: 6}
			renderDocument(screen, area, state)
			assertCellColors(t, screen, 6, 1, test.foreground, test.background)
			for y := 1; y <= 2; y++ {
				for x := 6; x < 39; x++ {
					_, style, _ := screen.Get(x, y)
					_, background, _ := style.Decompose()
					assert.Equal(t, test.background, background, "cell %d,%d", x, y)
				}
				assertCellRune(t, screen, 5, y, "!")
			}
			if test.background == turboRed {
				assertCellColors(t, screen, 14, 1, turboWhite, turboRed)
				assertCellColors(t, screen, 17, 1, turboWhite, turboRed)
			} else {
				assertCellColors(t, screen, 14, 1, turboLightCyan, test.background)
				assertCellColors(t, screen, 17, 1, turboLightGray, test.background)
			}
			assertCellColors(t, screen, 6, 3, turboYellow, turboBlue)
			x, y, visible := screen.GetCursor()
			assert.True(t, visible)
			assert.Equal(t, 6, x)
			assert.Equal(t, 1, y)

			require.NoError(t, state.buffer.Select(editor.Position{}, editor.Position{Column: 3}))
			renderDocument(screen, area, state)
			assertCellColors(t, screen, 6, 1, turboBlack, turboLightCyan)
			assertCellColors(t, screen, 8, 1, turboBlack, turboLightCyan)
			foreground := turboYellow
			if test.background == turboRed {
				foreground = turboWhite
			}
			assertCellColors(t, screen, 9, 1, foreground, test.background)
			x, y, visible = screen.GetCursor()
			assert.True(t, visible)
			assert.Equal(t, 9, x)
			assert.Equal(t, 1, y)

			state.languageDiagnostics = nil
			renderDocument(screen, area, state)
			assertCellColors(t, screen, 38, 1, turboYellow, turboBlue)
			assertCellColors(t, screen, 6, 2, turboYellow, turboBlue)
			assertCellRune(t, screen, 5, 1, " ")
		})
	}
}

func TestDiagnosticHighlightsScrolledAndNarrowLines(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, text    string
		width, scroll int
		wantText      string
	}{
		{"horizontal scroll", "abcdef", 20, 3, "def"},
		{"past end", "abc", 20, 10, ""},
		{"partial tab", "\tx", 20, 2, "  x"},
		{"partial wide rune", "界x", 20, 1, " x"},
		{"narrow", "界x", 4, 0, "界"},
		{"no gutter", "abc", 10, 0, "abc"},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := tcell.NewSimulationScreen("")
			require.NoError(t, screen.Init())
			t.Cleanup(screen.Fini)
			screen.SetSize(test.width, 4)
			state := shellState{fileColumn: test.scroll, focus: focusEditor}
			setTestDocument(t, &state, "/tmp/main.go", test.text)
			state.languageDiagnostics = []diagnostic.Diagnostic{{Line: 1, Severity: diagnostic.Error}}
			area := rectangle{width: test.width, height: 4}
			renderDocument(screen, area, state)
			for x := 1 + editorGutterWidth(area, 1); x < test.width-1; x++ {
				assertCellColors(t, screen, x, 1, turboWhite, turboRed)
			}
			assert.Equal(t, test.wantText, strings.TrimRight(rowText(screen, 1, 1+editorGutterWidth(area, 1), test.width-1), " "))
			assertCellRune(t, screen, 0, 1, "║")
			assertCellRune(t, screen, test.width-1, 1, "║")
		})
	}
}
