package i18n

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type formatArgument struct {
	index int
	verb  byte
}

type formatParser struct {
	text      string
	pos, next int
	arguments []formatArgument
}

func compatibleFormat(english, translated string) bool {
	expected, valid := formatArguments(english)
	if !valid {
		return false
	}
	actual, valid := formatArguments(translated)
	return valid && slices.Equal(expected, actual)
}

func formatArguments(text string) ([]formatArgument, bool) {
	parser := formatParser{text: text, next: 1}
	for parser.pos < len(text) {
		offset := strings.IndexByte(text[parser.pos:], '%')
		if offset < 0 {
			break
		}
		parser.pos += offset + 1
		if !parser.directive() {
			return nil, false
		}
	}
	slices.SortFunc(parser.arguments, func(left, right formatArgument) int {
		if order := cmp.Compare(left.index, right.index); order != 0 {
			return order
		}
		return cmp.Compare(left.verb, right.verb)
	})
	return parser.arguments, true
}

func (p *formatParser) directive() bool {
	for p.pos < len(p.text) && strings.ContainsRune("#0+- ", rune(p.text[p.pos])) {
		p.pos++
	}
	indexed, valid := p.index()
	if !valid {
		return false
	}
	if p.consume('*') {
		p.argument('*')
		indexed = false
	} else {
		start := p.pos
		if !p.number() || indexed && p.pos != start {
			return false
		}
	}
	if p.consume('.') {
		if indexed {
			return false
		}
		indexed, valid = p.index()
		if !valid {
			return false
		}
		if p.consume('*') {
			p.argument('.')
			indexed = false
		} else if !p.number() {
			return false
		}
	}
	if !indexed {
		_, valid = p.index()
		if !valid {
			return false
		}
	}
	if p.pos >= len(p.text) {
		return false
	}
	verb := p.text[p.pos]
	p.pos++
	if verb == '%' {
		return true
	}
	if !strings.ContainsRune("vTtbcdoOxXUeEfFgGsqpw", rune(verb)) {
		return false
	}
	p.argument(verb)
	return true
}

func (p *formatParser) consume(char byte) bool {
	if p.pos >= len(p.text) || p.text[p.pos] != char {
		return false
	}
	p.pos++
	return true
}

func (p *formatParser) index() (bool, bool) {
	if !p.consume('[') {
		return false, true
	}
	end := strings.IndexByte(p.text[p.pos:], ']')
	if end < 0 {
		return true, false
	}
	for _, char := range p.text[p.pos : p.pos+end] {
		if char < '0' || char > '9' {
			return true, false
		}
	}
	index, err := strconv.Atoi(p.text[p.pos : p.pos+end])
	if err != nil || index < 1 || index > 1_000_000 {
		return true, false
	}
	p.next = index
	p.pos += end + 1
	return true, true
}

func (p *formatParser) number() bool {
	start := p.pos
	for p.pos < len(p.text) && p.text[p.pos] >= '0' && p.text[p.pos] <= '9' {
		p.pos++
	}
	if start == p.pos {
		return true
	}
	number, err := strconv.Atoi(p.text[start:p.pos])
	return err == nil && number <= 1_000_000
}

func (p *formatParser) argument(verb byte) {
	p.arguments = append(p.arguments, formatArgument{p.next, verb})
	p.next++
}

func (c *Catalog) formatTemplate(key string) string {
	if c.formats != nil {
		return c.formats[key]
	}
	english, selected := c.english[key], c.selected[key]
	if strings.TrimSpace(selected) != "" {
		if strings.TrimSpace(english) != "" && compatibleFormat(english, selected) {
			return selected
		}
		if english == "" {
			if _, valid := formatArguments(selected); valid {
				return selected
			}
		}
	}
	return english
}

func (c *Catalog) compileFormats() {
	formats := make(map[string]string, len(c.english)+len(c.selected))
	for key := range c.english {
		formats[key] = c.formatTemplate(key)
	}
	for key := range c.selected {
		if _, exists := formats[key]; !exists {
			formats[key] = c.formatTemplate(key)
		}
	}
	c.formats = formats
}

// Errorf formats a localized error with fallback while preserving %w error chains.
func (c *Catalog) Errorf(key string, args ...any) error {
	template := c.formatTemplate(key)
	if template == "" {
		return errors.New(c.Text("unknown_message"))
	}
	return fmt.Errorf(template, args...)
}

// Errorf formats an application error from the active catalog.
func Errorf(key string, args ...any) error { return currentCatalog().Errorf(key, args...) }
