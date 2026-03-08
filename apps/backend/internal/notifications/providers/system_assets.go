package providers

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

//go:embed assets/toast-notification.ps1 assets/icon.png
var systemAssetsFS embed.FS

const assetDir = "kandev-notifications"

type systemAssets struct {
	scriptOnce sync.Once
	scriptPath string
	scriptErr  error

	iconOnce sync.Once
	iconPath string
	iconErr  error
}

func (a *systemAssets) ensureScript() (string, error) {
	a.scriptOnce.Do(func() {
		dir := filepath.Join(os.TempDir(), assetDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			a.scriptErr = fmt.Errorf("create notifications temp dir: %w", err)
			return
		}
		scriptPath := filepath.Join(dir, "toast-notification.ps1")
		if _, err := os.Stat(scriptPath); err == nil {
			a.scriptPath = scriptPath
			return
		}
		data, err := systemAssetsFS.ReadFile("assets/toast-notification.ps1")
		if err != nil {
			a.scriptErr = fmt.Errorf("read toast script: %w", err)
			return
		}
		if err := os.WriteFile(scriptPath, data, 0o644); err != nil {
			a.scriptErr = fmt.Errorf("write toast script: %w", err)
			return
		}
		a.scriptPath = scriptPath
	})
	return a.scriptPath, a.scriptErr
}

func (a *systemAssets) ensureIcon() (string, error) {
	a.iconOnce.Do(func() {
		dir := filepath.Join(os.TempDir(), assetDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			a.iconErr = fmt.Errorf("create notifications temp dir: %w", err)
			return
		}
		iconPath := filepath.Join(dir, "icon.png")
		if _, err := os.Stat(iconPath); err == nil {
			a.iconPath = iconPath
			return
		}
		data, err := systemAssetsFS.ReadFile("assets/icon.png")
		if err != nil {
			a.iconErr = fmt.Errorf("read icon: %w", err)
			return
		}
		if err := os.WriteFile(iconPath, data, 0o644); err != nil {
			a.iconErr = fmt.Errorf("write icon: %w", err)
			return
		}
		a.iconPath = iconPath
	})
	return a.iconPath, a.iconErr
}
