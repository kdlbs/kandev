package webapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/kandev/kandev/internal/i18n"
)

const bootPayloadGlobal = "window.__KANDEV_BOOT_PAYLOAD__"
const debugGlobalAssignment = "window.__KANDEV_DEBUG=true;"
const maxInt = int(^uint(0) >> 1)

var headCloseTag = []byte("</head>")

var htmlOpenTag = []byte("<html")

// DefaultLocale is the source locale shipped as the message catalog.
const DefaultLocale = i18n.DefaultLocale

// NormalizeLocale returns locale if it is supported, otherwise DefaultLocale.
// Locale support lives in internal/i18n so `<html lang>` and message lookup can
// never disagree.
func NormalizeLocale(locale string) string { return i18n.Normalize(locale) }

// RenderShell reads indexPath from assets and injects the boot payload before
// </head>. The assets FS can be an embedded Vite dist in production or an
// in-memory filesystem in tests.
func RenderShell(assets fs.FS, indexPath string, payload BootPayload) ([]byte, error) {
	indexHTML, err := fs.ReadFile(assets, indexPath)
	if err != nil {
		return nil, fmt.Errorf("read web shell: %w", err)
	}

	return RenderShellHTML(indexHTML, payload)
}

// RenderShellHTML injects the boot payload into an already-loaded HTML shell.
// Dev mode uses this with Vite's in-memory index.html.
func RenderShellHTML(indexHTML []byte, payload BootPayload) ([]byte, error) {
	script, err := BootPayloadScript(payload)
	if err != nil {
		return nil, err
	}
	localized := rewriteHTMLLang(indexHTML, NormalizeLocale(payload.Runtime.Locale))
	return injectBeforeHeadClose(localized, script), nil
}

// rewriteHTMLLang sets the lang="" attribute on the shell's opening <html> tag
// to locale so first paint matches the SPA's active locale. If the tag has no
// lang attribute, one is inserted. Only the opening <html ...> tag is touched.
func rewriteHTMLLang(indexHTML []byte, locale string) []byte {
	start := bytes.Index(indexHTML, htmlOpenTag)
	if start < 0 {
		return indexHTML
	}
	end := bytes.IndexByte(indexHTML[start:], '>')
	if end < 0 {
		return indexHTML
	}
	end += start
	tag := indexHTML[start : end+1]
	langAttr := []byte(` lang="` + locale + `"`)

	out := make([]byte, 0, len(indexHTML)+len(langAttr))
	out = append(out, indexHTML[:start]...)
	out = append(out, replaceLangInTag(tag, locale, langAttr)...)
	out = append(out, indexHTML[end+1:]...)
	return out
}

func replaceLangInTag(tag []byte, locale string, langAttr []byte) []byte {
	idx := bytes.Index(tag, []byte(` lang="`))
	if idx < 0 {
		// No lang attribute: insert one right after "<html".
		insertAt := len(htmlOpenTag)
		result := make([]byte, 0, len(tag)+len(langAttr))
		result = append(result, tag[:insertAt]...)
		result = append(result, langAttr...)
		result = append(result, tag[insertAt:]...)
		return result
	}
	valueStart := idx + len(` lang="`)
	valueEnd := bytes.IndexByte(tag[valueStart:], '"')
	if valueEnd < 0 {
		return tag
	}
	valueEnd += valueStart
	result := make([]byte, 0, len(tag)+len(locale))
	result = append(result, tag[:valueStart]...)
	result = append(result, locale...)
	result = append(result, tag[valueEnd:]...)
	return result
}

// BootPayloadScript serializes payload into an inline script safe to embed in
// HTML. encoding/json escapes '<', '>', and '&', which prevents a state value
// from closing the script tag.
func BootPayloadScript(payload BootPayload) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal boot payload: %w", err)
	}
	debugPrefix := ""
	if payload.Runtime.Debug {
		debugPrefix = debugGlobalAssignment
	}
	script := make([]byte, 0, bytesCapacity(len(data), len(debugPrefix), len(bootPayloadGlobal), 32))
	script = append(script, "<script>"...)
	script = append(script, debugPrefix...)
	script = append(script, bootPayloadGlobal...)
	script = append(script, "="...)
	script = append(script, data...)
	script = append(script, ";</script>"...)
	return script, nil
}

func injectBeforeHeadClose(indexHTML, script []byte) []byte {
	idx := bytes.Index(indexHTML, headCloseTag)
	if idx < 0 {
		out := make([]byte, 0, bytesCapacity(len(indexHTML), len(script)))
		out = append(out, script...)
		out = append(out, indexHTML...)
		return out
	}

	out := make([]byte, 0, bytesCapacity(len(indexHTML), len(script)))
	out = append(out, indexHTML[:idx]...)
	out = append(out, script...)
	out = append(out, indexHTML[idx:]...)
	return out
}

func bytesCapacity(parts ...int) int {
	total := 0
	for _, part := range parts {
		if part > maxInt-total {
			return 0
		}
		total += part
	}
	return total
}
