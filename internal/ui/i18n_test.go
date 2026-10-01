package ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/diagnostic"
	"github.com/hangxie/rapidgo/internal/i18n"
)

func useChinese(t *testing.T) {
	t.Helper()
	catalog, err := i18n.FromEnvironment(func(string) string { return "zh_CN.UTF-8" })
	require.NoError(t, err)
	previous := i18n.Use(catalog)
	t.Cleanup(func() { i18n.Use(previous) })
}

func TestTranslatedLayouts(t *testing.T) {
	useChinese(t)
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	for _, size := range [][2]int{{100, 30}, {30, 12}, {12, 5}} {
		screen.SetSize(size[0], size[1])
		state := shellState{menuOpen: true, menuIndex: menuFile}
		render(screen, state)
		row := 2
		if size[0] == 12 {
			row = 1
		}
		assert.Contains(t, translatedRow(screen, row, size[0]), "保存")
		assertCellColors(t, screen, 0, size[1]-1, turboBlack, turboLightGray)
		state.menuOpen, state.helpVisible = false, true
		render(screen, state)
		if helpFits(size[0], size[1]) {
			assert.Contains(t, paneText(screen, 0, size[1]-1), "RapidGo")
		}
	}
	labels := menuLabels()
	assert.Equal(t, "文件 (F)", labels[menuFile])
	assert.GreaterOrEqual(t, menuX(menuSearch), menuX(menuFile)+uniseg.StringWidth(labels[menuFile])+2)
	for _, width := range []int{1, 8, 26, 52} {
		for _, row := range helpRows(shellState{}, width, 100, 30) {
			var text strings.Builder
			for _, segment := range row {
				text.WriteString(segment.text)
			}
			assert.LessOrEqual(t, uniseg.StringWidth(text.String()), width)
		}
	}
}

func TestTranslatedSaveFailureKeepsMessage(t *testing.T) {
	useChinese(t)
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	state := newShellState("/tmp/project", func(workRequest) bool { return true })
	setTestDocument(t, &state, "main.go", "package main")
	state.languageDiagnostics = []diagnostic.Diagnostic{{Line: 1, Message: "diagnostic"}}
	state.saveSeq = 1
	state.applySaveResult(workResult{request: workRequest{seq: 1, path: "main.go"}, err: assert.AnError})
	render(screen, state)
	assert.Contains(t, translatedRow(screen, 22, 80), "保存失败")
}

func translatedRow(screen tcell.Screen, row, width int) string {
	var text strings.Builder
	for col := 0; col < width; {
		cell, _, cells := screen.Get(col, row)
		text.WriteString(cell)
		col += max(1, cells)
	}
	return text.String()
}

func TestLongTranslatedHelpHeader(t *testing.T) {
	english, err := os.ReadFile("../i18n/packs/en_US.json")
	require.NoError(t, err)
	source := fstest.MapFS{
		"en_US.json": {Data: english},
		"zh_CN.json": {Data: []byte(`{"msg_shortcut":"非常非常非常非常非常非常非常非常非常非常长的快捷键标题","msg_action":"非常非常非常非常长的操作描述","msg_file":"文件管理及工作目录选择","msg_save":"保存"}`)},
	}
	catalog, err := i18n.Load(source, "zh_CN")
	require.NoError(t, err)
	previous := i18n.Use(catalog)
	t.Cleanup(func() { i18n.Use(previous) })
	for _, row := range helpRows(shellState{}, 52, 100, 30) {
		var line strings.Builder
		for _, segment := range row {
			line.WriteString(segment.text)
		}
		assert.LessOrEqual(t, uniseg.StringWidth(line.String()), 52)
	}
	screen := tcell.NewSimulationScreen("")
	require.NoError(t, screen.Init())
	t.Cleanup(screen.Fini)
	for _, width := range []int{100, 30, 12} {
		screen.SetSize(width, 24)
		render(screen, shellState{menuOpen: true, menuIndex: menuFile})
		assert.Contains(t, translatedRow(screen, 2, width), "保存")
	}
}

func TestExplicitMenuMnemonic(t *testing.T) {
	english, err := os.ReadFile("../i18n/packs/en_US.json")
	require.NoError(t, err)
	for _, test := range []struct {
		name, label, index, want string
		cell                     int
	}{
		{"incidental letter", "SaFe", "-1", "SaFe (F)", 6},
		{"explicit later letter", "FiF", "2", "FiF", 2},
		{"wide and combining prefix", "界e\u0301F", "2", "界e\u0301F", 3},
		{"wrong character", "SaFe", "0", "SaFe (F)", 6},
		{"out of bounds", "SaFe", "99", "SaFe (F)", 6},
		{"invalid metadata", "SaFe", "invalid", "SaFe (F)", 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			translated, err := json.Marshal(map[string]string{"msg_file": test.label, "menu_file_mnemonic_index": test.index})
			require.NoError(t, err)
			catalog, err := i18n.Load(fstest.MapFS{"en_US.json": {Data: english}, "zh_CN.json": {Data: translated}}, "zh_CN")
			require.NoError(t, err)
			previous := i18n.Use(catalog)
			t.Cleanup(func() { i18n.Use(previous) })
			assert.Equal(t, test.want, menuLabels()[menuFile])
			screen := tcell.NewSimulationScreen("")
			require.NoError(t, screen.Init())
			t.Cleanup(screen.Fini)
			screen.SetSize(100, 24)
			renderMenuBar(screen, 100, 0, shellState{})
			assertCellColors(t, screen, 2+test.cell, 0, turboRed, turboLightGray)
			if test.name == "explicit later letter" {
				assertCellColors(t, screen, 2, 0, turboBlack, turboLightGray)
			}
		})
	}
}
