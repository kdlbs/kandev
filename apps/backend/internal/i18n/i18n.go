// Package i18n localizes the strings the backend renders directly to a browser.
//
// Scope is deliberately narrow. Almost every string the backend produces is
// diagnostic — API error payloads ("invalid payload", "wrong instance id"), log
// lines, and agent output. Those stay English: the SPA maps API failures onto its
// own translated copy, and translating a diagnostic would make logs and support
// harder without helping a user. See docs/i18n.md ("Backend").
//
// What this package covers is the surface a user reads straight from Go: the
// plain-text error pages served when the SPA bundle cannot be delivered, the
// shared-task artifacts (share.html, the gist README and description), and any
// future server-rendered copy.
//
// A shared task is a static file on GitHub — nobody makes a request to us when
// they open it — so its locale is fixed when the share is created, from the
// creator's request, and threaded down to the builders as an argument. See
// docs/decisions/2026-08-01-share-artifact-locale.md.
//
// Catalogs mirror the frontend's shape (`locales/<locale>.json`, flat
// `key: message`) and are embedded, so no runtime file access is needed.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

//go:embed locales/*.json
var catalogFS embed.FS

// DefaultLocale is the source locale; every other catalog falls back to it.
const DefaultLocale = "en"

// LocaleCookie is written by the SPA language switcher and is the source of
// truth for the active locale. Kept in sync with the frontend constant in
// apps/web/lib/i18n/cookie.ts.
const LocaleCookie = "kandev_locale"

// supportedLocales enumerates the locales with a committed catalog. Keep in sync
// with SUPPORTED_LOCALES in apps/web/lib/i18n/index.ts.
var supportedLocales = map[string]bool{
	"en":     true,
	"pseudo": true,
}

var (
	loadOnce sync.Once
	catalogs map[string]map[string]string
)

func load() {
	loadOnce.Do(func() {
		catalogs = make(map[string]map[string]string)
		entries, err := catalogFS.ReadDir("locales")
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			data, readErr := catalogFS.ReadFile("locales/" + entry.Name())
			if readErr != nil {
				continue
			}
			messages := map[string]string{}
			if json.Unmarshal(data, &messages) != nil {
				continue
			}
			catalogs[strings.TrimSuffix(entry.Name(), ".json")] = messages
		}
	})
}

// Supported reports whether locale has a committed catalog.
func Supported(locale string) bool { return supportedLocales[locale] }

// Normalize returns locale when it is supported, otherwise DefaultLocale. Used
// for both `<html lang>` and message lookup so they can never disagree.
func Normalize(locale string) string {
	if supportedLocales[locale] {
		return locale
	}
	return DefaultLocale
}

// FromRequest resolves the active locale: the kandev_locale cookie first (the
// SPA's explicit user choice), then the browser's Accept-Language, then the
// default. Unknown values coerce to DefaultLocale rather than erroring.
func FromRequest(r *http.Request) string {
	if r == nil {
		return DefaultLocale
	}
	if cookie, err := r.Cookie(LocaleCookie); err == nil && Supported(cookie.Value) {
		return cookie.Value
	}
	for _, tag := range parseAcceptLanguage(r.Header.Get("Accept-Language")) {
		if Supported(tag) {
			return tag
		}
		// Match "en-GB" against the "en" catalog.
		if base := strings.SplitN(tag, "-", 2)[0]; Supported(base) {
			return base
		}
	}
	return DefaultLocale
}

// parseAcceptLanguage returns the header's tags in descending q-value order.
// Malformed entries are skipped rather than failing the request.
func parseAcceptLanguage(header string) []string {
	if header == "" {
		return nil
	}
	type weighted struct {
		tag string
		q   float64
	}
	var items []weighted
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		tag := strings.ToLower(strings.TrimSpace(fields[0]))
		if tag == "" {
			continue
		}
		q := 1.0
		for _, field := range fields[1:] {
			field = strings.TrimSpace(field)
			if !strings.HasPrefix(field, "q=") {
				continue
			}
			if _, err := fmt.Sscanf(field, "q=%f", &q); err != nil {
				q = 1.0
			}
		}
		items = append(items, weighted{tag: tag, q: q})
	}
	// Stable insertion sort: preserves header order among equal q-values.
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].q > items[j-1].q; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	tags := make([]string, 0, len(items))
	for _, item := range items {
		tags = append(tags, item.tag)
	}
	return tags
}

// T returns the message for key in locale, falling back to DefaultLocale and
// finally to the key itself. A missing key therefore degrades to
// visible-but-wrong rather than blank — the same contract as the frontend.
func T(locale, key string) string {
	load()
	normalized := Normalize(locale)
	if messages, ok := catalogs[normalized]; ok {
		if message, found := messages[key]; found && message != "" {
			return message
		}
	}
	if normalized != DefaultLocale {
		if messages, ok := catalogs[DefaultLocale]; ok {
			if message, found := messages[key]; found && message != "" {
				return message
			}
		}
	}
	return key
}

// Tf returns the message for key in locale with every `{{name}}` placeholder
// replaced by the matching entry in vars. It is the only supported way to build
// a message that carries a value: `fmt.Sprintf` on the result of T would freeze
// the argument order in English, and a translator cannot move a placeholder that
// is not in the catalog.
//
// When vars carries a "count", the i18next plural suffixes are resolved first —
// `key_one` for exactly 1, `key_other` for anything else — falling back to the
// unsuffixed key when the catalog defines no plural forms. That keeps the
// contract identical to the frontend's `t("key", { count })` and is why no call
// site ever appends an English "s" itself.
//
// Values are substituted verbatim; escaping (HTML, Markdown) belongs to the call
// site, which is the only place that knows the target syntax.
func Tf(locale, key string, vars map[string]any) string {
	return interpolate(T(locale, pluralKey(locale, key, vars)), vars)
}

// pluralKey picks the plural variant of key that the catalog actually defines,
// so a message with only a base form still resolves.
func pluralKey(locale, key string, vars map[string]any) string {
	count, ok := countVar(vars)
	if !ok {
		return key
	}
	// Only an exact 1 is singular. Comparing the original value rather than a
	// truncated integer is what stops 1.5 from rendering as "1.5 message".
	suffixed := key + "_other"
	if count == 1 {
		suffixed = key + "_one"
	}
	// T echoes the key back when it is missing — that is how we detect a catalog
	// that never defined the plural forms.
	if T(locale, suffixed) != suffixed {
		return suffixed
	}
	return key
}

// countVar reads the plural selector out of vars as a float64, which is wide
// enough for every Go numeric type a caller might hold (a proto int32, a store
// aggregate uint64, a JSON-decoded float64) and preserves the fractional part
// that decides singular-vs-plural. Non-numeric values are ignored rather than
// erroring: a bad "count" degrades to the unsuffixed key.
func countVar(vars map[string]any) (float64, bool) {
	switch n := vars["count"].(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// interpolate replaces `{{name}}` with vars[name]. Placeholders with no matching
// var are left in place, which makes the omission visible instead of silently
// producing a sentence with a hole in it.
func interpolate(message string, vars map[string]any) string {
	if len(vars) == 0 || !strings.Contains(message, "{{") {
		return message
	}
	pairs := make([]string, 0, len(vars)*2)
	for name, value := range vars {
		pairs = append(pairs, "{{"+name+"}}", fmt.Sprint(value))
	}
	return strings.NewReplacer(pairs...).Replace(message)
}

// Keys lists every key in the source catalog; used by the drift test that keeps
// translations in step with `en`.
func Keys() []string {
	load()
	messages := catalogs[DefaultLocale]
	keys := make([]string, 0, len(messages))
	for key := range messages {
		keys = append(keys, key)
	}
	return keys
}
