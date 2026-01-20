package providers

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

//go:embed assets/toast-notification.ps1
var systemAssetsFS embed.FS

type systemAssets struct {
	once       sync.Once
	scriptPath string
	err        error
}

func (a *systemAssets) ensureScript() (string, error) {
	a.once.Do(func() {
		dir := filepath.Join(os.TempDir(), "kandev-notifications")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			a.err = fmt.Errorf("create notifications temp dir: %w", err)
			return
		}
		const scriptName = "toast-notification.ps1"
		scriptPath := filepath.Join(dir, scriptName)
		if _, err := os.Stat(scriptPath); err == nil {
			a.scriptPath = scriptPath
			return
		}
		data, err := systemAssetsFS.ReadFile("assets/toast-notification.ps1")
		if err != nil {
			a.err = fmt.Errorf("read toast script: %w", err)
			return
		}
		if err := os.WriteFile(scriptPath, data, 0o644); err != nil {
			a.err = fmt.Errorf("write toast script: %w", err)
			return
		}
		a.scriptPath = scriptPath
	})
	return a.scriptPath, a.err
}
