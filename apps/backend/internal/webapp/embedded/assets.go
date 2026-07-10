package embedded

import (
	"embed"
	"io/fs"
)

//go:embed fallback generated
var assets embed.FS

// FS returns embedded Vite web assets. Generated assets are embedded when the
// root build syncs apps/web/dist before compiling the backend; backend-only
// builds fall back to a minimal shell so the binary remains buildable.
func FS() (fs.FS, error) {
	if _, err := assets.Open("generated/index.html"); err == nil {
		return fs.Sub(assets, "generated")
	}
	return fs.Sub(assets, "fallback")
}
