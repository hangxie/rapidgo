package debug

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Test is one test, benchmark, fuzz test, or example in the package at Dir.
type Test struct {
	Dir  string
	Name string
}

// FindTest returns the test function whose declaration spans the one-based line of a _test.go file.
func FindTest(path, source string, line int) (Test, bool) {
	if !strings.HasSuffix(path, "_test.go") {
		return Test{}, false
	}
	files := token.NewFileSet()
	// The buffer may be mid-edit, so use whatever declarations parse.
	file, _ := parser.ParseFile(files, path, source, parser.SkipObjectResolution|parser.ParseComments)
	if file == nil {
		return Test{}, false
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil {
			continue
		}
		if files.Position(function.Pos()).Line <= line && line <= files.Position(function.End()).Line {
			if !runsUnderGoTest(file, function) {
				return Test{}, false
			}
			return Test{Dir: filepath.Dir(path), Name: function.Name.Name}, true
		}
	}
	return Test{}, false
}

// outputComment matches the comment that makes go test run an example.
var outputComment = regexp.MustCompile(`(?i)^[[:space:]]*(unordered )?output:`)

// runsUnderGoTest applies go test's name and signature rules, so TestMain and helpers are not run alone.
func runsUnderGoTest(file *ast.File, function *ast.FuncDecl) bool {
	signature := function.Type
	if signature.Results != nil && len(signature.Results.List) > 0 {
		return false
	}
	name := function.Name.Name
	for _, test := range []struct{ prefix, parameter string }{{"Test", "T"}, {"Benchmark", "B"}, {"Fuzz", "F"}, {"Example", ""}} {
		rest, ok := strings.CutPrefix(name, test.prefix)
		if !ok {
			continue
		}
		if first, _ := utf8.DecodeRuneInString(rest); rest != "" && unicode.IsLower(first) {
			return false
		}
		if test.parameter == "" {
			return signature.Params.NumFields() == 0 && hasOutputComment(file, function)
		}
		return takesTestingPointer(signature.Params, test.parameter)
	}
	return false
}

// takesTestingPointer reports whether the only parameter is *testing.<name>, under any import name or a dot import.
func takesTestingPointer(params *ast.FieldList, name string) bool {
	if params.NumFields() != 1 {
		return false
	}
	pointer, ok := params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch parameter := pointer.X.(type) {
	case *ast.SelectorExpr:
		return parameter.Sel.Name == name
	case *ast.Ident:
		return parameter.Name == name
	}
	return false
}

// hasOutputComment checks the last comment in the body, which is where go test looks; without one the example only compiles.
func hasOutputComment(file *ast.File, function *ast.FuncDecl) bool {
	if function.Body == nil {
		return false
	}
	var last *ast.CommentGroup
	for _, group := range file.Comments {
		if function.Body.Pos() <= group.Pos() && group.End() <= function.Body.End() {
			last = group
		}
	}
	return last != nil && outputComment.MatchString(last.Text())
}

// Args selects only this test in the test binary.
func (t Test) Args() []string {
	pattern := "^" + t.Name + "$"
	if strings.HasPrefix(t.Name, "Benchmark") {
		return []string{"-test.run", "^$", "-test.bench", pattern}
	}
	return []string{"-test.run", pattern}
}
