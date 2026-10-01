package i18n

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompatibleFormat(t *testing.T) {
	for _, test := range []struct {
		name, english, translated string
		want                      bool
	}{
		{"bad index", "File: %s", "文件：%[2]s", false},
		{"reordered", "%s %d", "%[2]d %[1]s", true},
		{"swapped types", "%s %d", "%[1]d %[2]s", false},
		{"implicit after index", "%s %d", "%[2]d %s", false},
		{"width", "%*d", "%[1]*[2]d", true},
		{"precision", "%.*s", "%.[1]*[2]s", true},
		{"width and precision", "%*.*f", "%[1]*.[2]*[3]f", true},
		{"wrong width argument", "%*d", "%[2]*[1]d", false},
		{"escaped percent", "100%%s %s", "100%%s %[1]s", true},
		{"dropped wrap", "%s: %w", "%[1]s: %[2]v", false},
		{"signed index", "%s", "%[+1]s", false},
		{"invalid unsigned verb", "%d", "%u", false},
		{"zero index", "%s", "%[0]s", false},
		{"malformed index", "%s", "%[x]s", false},
		{"missing verb", "%s", "%", false},
		{"unsupported verb", "%s", "%Q", false},
	} {
		t.Run(test.name, func(t *testing.T) { assert.Equal(t, test.want, compatibleFormat(test.english, test.translated)) })
	}
}

func TestErrorfFallbackPreservesWrapping(t *testing.T) {
	cause := errors.New("disk failure")
	for _, test := range []struct{ name, translated, want string }{
		{"bad index", "%[3]w", "File /tmp/界: disk failure"},
		{"dropped wrap", "%s: %v", "File /tmp/界: disk failure"},
		{"reordered", "%[2]w：%[1]s", "disk failure：/tmp/界"},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog := &Catalog{english: map[string]string{"message": "File %s: %w"}, selected: map[string]string{"message": test.translated}}
			err := catalog.Errorf("message", "/tmp/界", cause)
			require.ErrorIs(t, err, cause)
			assert.Equal(t, test.want, err.Error())
			previous := Use(catalog)
			t.Cleanup(func() { Use(previous) })
			assert.ErrorIs(t, Errorf("message", "/tmp/界", cause), cause)
		})
	}
}

func TestLiteralFormattingMarker(t *testing.T) {
	catalog := &Catalog{english: map[string]string{"message": "File %s"}, selected: map[string]string{"message": "文件：%[1]s"}}
	assert.Equal(t, "文件：/tmp/%!s(BADINDEX)", catalog.Format("message", "/tmp/%!s(BADINDEX)"))
}

func TestLoadedFormats(t *testing.T) {
	source := fstest.MapFS{
		"en_US.json": {Data: []byte(`{"path":"File: %s", "wrapped":"File %s: %w", "unknown_message":"Unavailable"}`)},
		"zh_CN.json": {Data: []byte(`{"path":"文件：%[2]s", "wrapped":"%[1]s: %[2]v", "unknown_message":"未知消息"}`)},
	}
	catalog, err := Load(source, "zh_CN")
	require.NoError(t, err)
	assert.Equal(t, "File: /tmp/界", catalog.Format("path", "/tmp/界"))
	cause := errors.New("disk failure")
	err = catalog.Errorf("wrapped", "/tmp/界", cause)
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, "File /tmp/界: disk failure", err.Error())
	assert.Equal(t, "未知消息", catalog.Errorf("missing").Error())
}

func TestFormattingCallersUseCatalog(t *testing.T) {
	err := fs.WalkDir(os.DirFS(".."), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), "../"+path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if function := localizedFormatCall(node); function != "" {
				t.Errorf("%s: use catalog formatting instead of fmt.%s(i18n.Text(...))", path, function)
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
}

func localizedFormatCall(node ast.Node) string {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return ""
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "fmt" {
		return ""
	}
	argumentIndex := 0
	switch selector.Sel.Name {
	case "Fprintf", "Appendf":
		argumentIndex = 1
	case "Sprintf", "Errorf", "Printf":
	default:
		return ""
	}
	if len(call.Args) <= argumentIndex {
		return ""
	}
	lookup, ok := call.Args[argumentIndex].(*ast.CallExpr)
	if !ok {
		return ""
	}
	function, ok := lookup.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	packageName, ok := function.X.(*ast.Ident)
	if ok && packageName.Name == "i18n" && function.Sel.Name == "Text" {
		return selector.Sel.Name
	}
	return ""
}
