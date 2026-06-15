package webapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
)

const bootPayloadGlobal = "window.__KANDEV_BOOT_PAYLOAD__"

var headCloseTag = []byte("</head>")

// RenderShell reads indexPath from assets and injects the boot payload before
// </head>. The assets FS can be an embedded Vite dist in production or an
// in-memory filesystem in tests.
func RenderShell(assets fs.FS, indexPath string, payload BootPayload) ([]byte, error) {
	indexHTML, err := fs.ReadFile(assets, indexPath)
	if err != nil {
		return nil, fmt.Errorf("read web shell: %w", err)
	}

	script, err := BootPayloadScript(payload)
	if err != nil {
		return nil, err
	}
	return injectBeforeHeadClose(indexHTML, script), nil
}

// BootPayloadScript serializes payload into an inline script safe to embed in
// HTML. encoding/json escapes '<', '>', and '&', which prevents a state value
// from closing the script tag.
func BootPayloadScript(payload BootPayload) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal boot payload: %w", err)
	}
	script := make([]byte, 0, len(data)+len(bootPayloadGlobal)+32)
	script = append(script, "<script>"...)
	script = append(script, bootPayloadGlobal...)
	script = append(script, "="...)
	script = append(script, data...)
	script = append(script, ";</script>"...)
	return script, nil
}

func injectBeforeHeadClose(indexHTML, script []byte) []byte {
	idx := bytes.Index(indexHTML, headCloseTag)
	if idx < 0 {
		out := make([]byte, 0, len(indexHTML)+len(script))
		out = append(out, script...)
		out = append(out, indexHTML...)
		return out
	}

	out := make([]byte, 0, len(indexHTML)+len(script))
	out = append(out, indexHTML[:idx]...)
	out = append(out, script...)
	out = append(out, indexHTML[idx:]...)
	return out
}
