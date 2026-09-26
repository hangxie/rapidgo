package jobs

import (
	"fmt"
	"unicode"
)

// ParseArguments splits a run-argument prompt without invoking a shell.
func ParseArguments(input string) ([]string, error) {
	var args []string
	var current []rune
	quote := rune(0)
	escape := false
	started := false
	for _, char := range input {
		switch {
		case escape:
			current = append(current, char)
			escape = false
		case char == '\\' && quote != '\'':
			escape, started = true, true
		case quote != 0 && char == quote:
			quote = 0
		case quote == 0 && (char == '\'' || char == '"'):
			quote, started = char, true
		case quote == 0 && unicode.IsSpace(char):
			if started {
				args = append(args, string(current))
				current, started = nil, false
			}
		default:
			current, started = append(current, char), true
		}
	}
	if escape {
		return nil, fmt.Errorf("trailing backslash")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote")
	}
	if started {
		args = append(args, string(current))
	}
	return args, nil
}
