// Package i18n loads language packs independently of terminal rendering.
package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"sync/atomic"
)

//go:embed packs/*.json
var packs embed.FS

var localePattern = regexp.MustCompile(`^[A-Za-z]{2,3}[_-][A-Za-z]{2}$`)

// Catalog is an immutable selected pack with an English fallback.
type Catalog struct {
	locale   string
	english  map[string]string
	selected map[string]string
	formats  map[string]string
}

// Normalize removes encodings and modifiers and canonicalizes language and region.
func Normalize(locale string) string {
	locale = strings.SplitN(strings.SplitN(locale, "@", 2)[0], ".", 2)[0]
	if !localePattern.MatchString(locale) {
		return "en_US"
	}
	parts := strings.FieldsFunc(locale, func(r rune) bool { return r == '_' || r == '-' })
	return strings.ToLower(parts[0]) + "_" + strings.ToUpper(parts[1])
}

// Load reads a selected pack and the mandatory English fallback from a filesystem.
func Load(source fs.FS, locale string) (*Catalog, error) {
	english, err := readPack(source, "en_US")
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{locale: "en_US", english: english}
	locale = Normalize(locale)
	if locale == "en_US" {
		catalog.compileFormats()
		return catalog, nil
	}
	selected, err := readPack(source, locale)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			catalog.compileFormats()
			return catalog, nil
		}
		return nil, err
	}
	catalog.locale, catalog.selected = locale, selected
	catalog.compileFormats()
	return catalog, nil
}

func readPack(source fs.FS, locale string) (map[string]string, error) {
	data, err := fs.ReadFile(source, locale+".json")
	if err != nil {
		return nil, englishCatalog.Errorf("pack_read", locale, err)
	}
	var entries map[string]string
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, englishCatalog.Errorf("pack_decode", locale, err)
	}
	return entries, nil
}

// Locale returns the selected pack's normalized name.
func (c *Catalog) Locale() string { return c.locale }

// Text returns a translation or its English fallback.
func (c *Catalog) Text(key string) string {
	if key == "" {
		return ""
	}
	if text := c.lookup(key); text != "" {
		return text
	}
	if text := c.lookup("unknown_message"); text != "" {
		return text
	}
	return englishCatalog.english["unknown_message"]
}

func (c *Catalog) lookup(key string) string {
	if text := c.selected[key]; strings.TrimSpace(text) != "" {
		return text
	}
	if text := c.english[key]; strings.TrimSpace(text) != "" {
		return text
	}
	return ""
}

// Format interpolates printf placeholders using English for incompatible translations.
func (c *Catalog) Format(key string, args ...any) string {
	template := c.formatTemplate(key)
	if template == "" {
		return c.Text(key)
	}
	return fmt.Sprintf(template, args...)
}

// FromEnvironment selects the first nonempty LC_ALL, LC_MESSAGES, or LANG value.
func FromEnvironment(getenv func(string) string) (*Catalog, error) {
	locale := ""
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale = getenv(key); locale != "" {
			break
		}
	}
	source, err := fs.Sub(packs, "packs")
	if err != nil {
		return nil, err
	}
	return Load(source, locale)
}

var (
	active         atomic.Pointer[Catalog]
	englishCatalog = defaultCatalog()
)

func defaultCatalog() *Catalog {
	data, err := packs.ReadFile("packs/en_US.json")
	if err != nil {
		panic(err)
	}
	var entries map[string]string
	if err := json.Unmarshal(data, &entries); err != nil {
		panic(err)
	}
	catalog := &Catalog{locale: "en_US", english: entries}
	catalog.compileFormats()
	return catalog
}

// Use selects a catalog for the process and returns the previous selection.
func Use(catalog *Catalog) *Catalog { return active.Swap(catalog) }

// Text retrieves an application message from the active catalog.
func Text(key string) string { return currentCatalog().Text(key) }

func currentCatalog() *Catalog {
	if catalog := active.Load(); catalog != nil {
		return catalog
	}
	return englishCatalog
}

// Format interpolates an application message from the active catalog.
func Format(key string, args ...any) string { return currentCatalog().Format(key, args...) }

type messageError struct{ key string }

func (e *messageError) Error() string { return Text(e.key) }

func (e *messageError) Is(target error) bool {
	other, ok := target.(*messageError)
	return ok && other != nil && e != nil && other.key == e.key
}

// Error creates a localized error that errors.Is matches by message key.
func Error(key string) error { return &messageError{key: key} }
