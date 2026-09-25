// Package highlight classifies source text without depending on terminal styles.
package highlight

import (
	"go/scanner"
	"go/token"
)

type Kind uint8

const (
	Keyword Kind = iota + 1
	String
	Number
	Comment
)

// Span uses half-open UTF-8 byte offsets into the original source text.
type Span struct {
	Start, End int
	Kind       Kind
}

// Go returns lexical spans even when the source is incomplete or invalid.
// The scanner's errors are deliberately ignored: an editor must keep coloring
// the rest of a file while the user is in the middle of typing it.
func Go(source string) []Span {
	set := token.NewFileSet()
	file := set.AddFile("", -1, len(source))
	var lexer scanner.Scanner
	lexer.Init(file, []byte(source), nil, scanner.ScanComments)
	var spans []Span
	for {
		position, symbol, literal := lexer.Scan()
		if symbol == token.EOF {
			return spans
		}
		var kind Kind
		switch {
		case symbol.IsKeyword():
			kind = Keyword
		case symbol == token.STRING || symbol == token.CHAR:
			kind = String
		case symbol == token.INT || symbol == token.FLOAT || symbol == token.IMAG:
			kind = Number
		case symbol == token.COMMENT:
			kind = Comment
		default:
			continue
		}
		start := file.Offset(position)
		spans = append(spans, Span{Start: start, End: start + len(literal), Kind: kind})
	}
}
