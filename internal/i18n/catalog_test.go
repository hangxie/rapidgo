package i18n

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocale(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"empty", nil, "en_US"},
		{"precedence", map[string]string{"LC_ALL": "zh_CN.UTF-8", "LC_MESSAGES": "en_US", "LANG": "en_US"}, "zh_CN"},
		{"messages", map[string]string{"LC_MESSAGES": "zh-cn@variant", "LANG": "en_US"}, "zh_CN"},
		{"language", map[string]string{"LANG": "zh_CN.utf8@variant"}, "zh_CN"},
		{"unsupported", map[string]string{"LANG": "fr_FR"}, "en_US"},
		{"invalid", map[string]string{"LANG": "../../zh_CN"}, "en_US"},
		{"C overrides", map[string]string{"LC_ALL": "C.UTF-8", "LANG": "zh_CN"}, "en_US"},
		{"POSIX", map[string]string{"LANG": "POSIX"}, "en_US"},
		{"empty all", map[string]string{"LC_ALL": "", "LANG": "zh_CN"}, "zh_CN"},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog, err := FromEnvironment(func(key string) string { return test.env[key] })
			require.NoError(t, err)
			assert.Equal(t, test.want, catalog.Locale())
		})
	}
}

func TestCatalog(t *testing.T) {
	packs := fstest.MapFS{
		"en_US.json": {Data: []byte(`{"greeting":"Hello %[1]s, %[2]d files", "fallback":"English", "blank":"Default"}`)},
		"zh_CN.json": {Data: []byte(`{"greeting":"%[2]d 个文件：%[1]s", "blank":""}`)},
	}
	catalog, err := Load(packs, "zh_CN.UTF-8")
	require.NoError(t, err)
	assert.Equal(t, "2 个文件：/tmp/界", catalog.Format("greeting", "/tmp/界", 2))
	assert.Equal(t, "English", catalog.Text("fallback"))
	assert.Equal(t, "Default", catalog.Text("blank"))
	packs["zh_CN.json"].Data = []byte(`broken`)
	_, err = Load(packs, "zh_CN")
	require.Error(t, err)
	delete(packs, "en_US.json")
	_, err = Load(packs, "en_US")
	require.Error(t, err)
}

func TestActiveCatalogAndSentinel(t *testing.T) {
	sentinel := Error("msg_editor_text_is_not_valid_utf_8")
	assert.Equal(t, "editor text is not valid UTF-8", sentinel.Error())
	catalog, err := FromEnvironment(func(string) string { return "zh_CN" })
	require.NoError(t, err)
	previous := Use(catalog)
	t.Cleanup(func() { Use(previous) })
	assert.Equal(t, "文件", Text("msg_file"))
	assert.Equal(t, "找到：/tmp/界", Format("msg_found_s", "/tmp/界"))
	assert.ErrorIs(t, fmt.Errorf("wrapped: %w", sentinel), sentinel)
	assert.ErrorIs(t, fmt.Errorf("wrapped: %w", sentinel), Error("msg_editor_text_is_not_valid_utf_8"))
	assert.ErrorIs(t, sentinel, Error("msg_editor_text_is_not_valid_utf_8"))
	assert.NotErrorIs(t, sentinel, Error("msg_file"))
	assert.NotErrorIs(t, sentinel, fmt.Errorf("different error"))
	var nilTarget *messageError
	assert.NotErrorIs(t, sentinel, nilTarget)
	assert.Equal(t, "Message unavailable", catalog.Text("absent"))
	Use(nil)
	assert.Equal(t, "File", Text("msg_file"))
}

func TestEmbeddedPacks(t *testing.T) {
	source, err := fs.Sub(packs, "packs")
	require.NoError(t, err)
	files, err := fs.ReadDir(source, ".")
	require.NoError(t, err)
	for _, file := range files {
		entries, err := readPack(source, strings.TrimSuffix(file.Name(), ".json"))
		require.NoError(t, err)
		for key, value := range entries {
			english, exists := englishCatalog.english[key]
			require.True(t, exists, "unknown key %s in %s", key, file.Name())
			require.NotEmpty(t, strings.TrimSpace(english), key)
			assert.True(t, compatibleFormat(english, value), "argument mismatch for %s in %s", key, file.Name())
		}
	}
}

func TestApplicationKeysHaveEnglishMessages(t *testing.T) {
	source := os.DirFS("..")
	err := fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, data, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			key, err := strconv.Unquote(literal.Value)
			require.NoError(t, err)
			if strings.HasPrefix(key, "msg_") || strings.HasPrefix(key, "cli_") || strings.HasPrefix(key, "format_") || strings.HasPrefix(key, "pack_") || strings.HasPrefix(key, "menu_") {
				require.NotEmpty(t, englishCatalog.english[key], "missing English message %s in %s", key, path)
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
}

func TestNormalize(t *testing.T) {
	for _, test := range []struct{ locale, want string }{
		{"en_US.UTF-8", "en_US"},
		{"ZH-cn.utf8@variant", "zh_CN"},
		{"zh_CN@variant.UTF-8", "zh_CN"},
		{"C", "en_US"},
		{"POSIX.UTF-8", "en_US"},
		{" ", "en_US"},
		{"zh", "en_US"},
		{"zh_123", "en_US"},
		{"../../zh_CN", "en_US"},
	} {
		t.Run(test.locale, func(t *testing.T) { assert.Equal(t, test.want, Normalize(test.locale)) })
	}
}

func TestLoadIndependentOfActiveCatalog(t *testing.T) {
	source := fstest.MapFS{"en_US.json": {Data: []byte(`{"greeting":"Hello"}`)}}
	catalog, err := Load(source, "en_US")
	require.NoError(t, err)
	previous := Use(catalog)
	t.Cleanup(func() { Use(previous) })
	fallback, err := Load(source, "fr_FR")
	require.NoError(t, err)
	assert.Equal(t, "en_US", fallback.Locale())
}

func TestFormatFallback(t *testing.T) {
	for _, test := range []struct{ name, template string }{
		{"wrong type", "%d"},
		{"missing argument", "%s %d"},
		{"unused argument", "no placeholders"},
		{"bad index", "%[2]s"},
		{"bad verb", "%Q"},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog := &Catalog{english: map[string]string{"message": "English %s"}, selected: map[string]string{"message": test.template}}
			assert.Equal(t, "English /tmp/界", catalog.Format("message", "/tmp/界"))
			previous := Use(catalog)
			t.Cleanup(func() { Use(previous) })
			assert.Equal(t, "English /tmp/界", Format("message", "/tmp/界"))
		})
	}
}

func TestUnknownMessageFallback(t *testing.T) {
	for _, test := range []struct {
		name              string
		english, selected map[string]string
		want              string
	}{
		{"translated", map[string]string{"unknown_message": "English"}, map[string]string{"unknown_message": "未知消息"}, "未知消息"},
		{"blank translation", map[string]string{"unknown_message": "English"}, map[string]string{"unknown_message": " "}, "English"},
		{"absent generic", nil, nil, "Message unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog := &Catalog{english: test.english, selected: test.selected}
			assert.Equal(t, test.want, catalog.Text("missing"))
			assert.Equal(t, test.want, catalog.Format("missing", "unused runtime argument"))
			assert.Empty(t, catalog.Text(""))
		})
	}
}
