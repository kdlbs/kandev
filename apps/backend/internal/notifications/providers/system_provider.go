package providers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// OS name constants used for runtime.GOOS comparisons.
const (
	osDarwin  = "darwin"
	osLinux   = "linux"
	osWindows = "windows"
)

type SystemProvider struct {
	assets systemAssets
}

func NewSystemProvider() *SystemProvider {
	return &SystemProvider{}
}

func (p *SystemProvider) Available() bool {
	switch runtime.GOOS {
	case osDarwin:
		return true
	case osWindows:
		_, err := exec.LookPath("powershell.exe")
		return err == nil
	case osLinux:
		if isWSL() {
			_, err := exec.LookPath("powershell.exe")
			return err == nil
		}
		if _, err := exec.LookPath("notify-send"); err == nil {
			return true
		}
		if _, err := exec.LookPath("zenity"); err == nil {
			return true
		}
		return false
	default:
		return false
	}
}

func (p *SystemProvider) Validate(config map[string]interface{}) error {
	_, err := parseSystemConfig(config)
	return err
}

func (p *SystemProvider) Send(ctx context.Context, message Message) error {
	cfg, err := parseSystemConfig(message.Config)
	if err != nil {
		return err
	}
	if err := p.sendNotification(ctx, cfg, message.Title, message.Body); err != nil {
		return err
	}
	if cfg.SoundEnabled {
		_ = p.playSound(ctx, cfg)
	}
	return nil
}

type systemConfig struct {
	SoundEnabled bool
	SoundFile    string
	AppName      string
	IconPath     string
	TimeoutMS    int
}

func parseSystemConfig(raw map[string]interface{}) (systemConfig, error) {
	cfg := systemConfig{
		SoundEnabled: false,
		SoundFile:    "",
		AppName:      "Kandev",
		IconPath:     "",
		TimeoutMS:    10000,
	}
	if raw == nil {
		return cfg, nil
	}
	if err := applySystemConfigFields(&cfg, raw); err != nil {
		return cfg, err
	}
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = 10000
	}
	return cfg, nil
}

func applySystemConfigFields(cfg *systemConfig, raw map[string]interface{}) error {
	if err := parseSoundEnabled(cfg, raw); err != nil {
		return err
	}
	if err := parseSoundFile(cfg, raw); err != nil {
		return err
	}
	if err := parseAppName(cfg, raw); err != nil {
		return err
	}
	if err := parseIconPath(cfg, raw); err != nil {
		return err
	}
	return parseTimeoutMS(cfg, raw)
}

func parseSoundEnabled(cfg *systemConfig, raw map[string]interface{}) error {
	value, ok := raw["sound_enabled"]
	if !ok {
		return nil
	}
	enabled, ok := value.(bool)
	if !ok {
		return fmt.Errorf("sound_enabled must be a boolean")
	}
	cfg.SoundEnabled = enabled
	return nil
}

func parseSoundFile(cfg *systemConfig, raw map[string]interface{}) error {
	value, ok := raw["sound_file"]
	if !ok {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf("sound_file must be a string")
	}
	cfg.SoundFile = strings.TrimSpace(text)
	return nil
}

func parseAppName(cfg *systemConfig, raw map[string]interface{}) error {
	value, ok := raw["app_name"]
	if !ok {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf("app_name must be a string")
	}
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		cfg.AppName = trimmed
	}
	return nil
}

func parseIconPath(cfg *systemConfig, raw map[string]interface{}) error {
	value, ok := raw["icon_path"]
	if !ok {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf("icon_path must be a string")
	}
	cfg.IconPath = strings.TrimSpace(text)
	return nil
}

func parseTimeoutMS(cfg *systemConfig, raw map[string]interface{}) error {
	value, ok := raw["timeout_ms"]
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case float64:
		cfg.TimeoutMS = int(v)
	case int:
		cfg.TimeoutMS = v
	case int64:
		cfg.TimeoutMS = int(v)
	case string:
		if strings.TrimSpace(v) != "" {
			parsed, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return fmt.Errorf("timeout_ms must be a number")
			}
			cfg.TimeoutMS = parsed
		}
	default:
		return fmt.Errorf("timeout_ms must be a number")
	}
	return nil
}

func (p *SystemProvider) sendNotification(ctx context.Context, cfg systemConfig, title, body string) error {
	switch runtime.GOOS {
	case osDarwin:
		return runCommand(ctx, "osascript", "-e", buildAppleScript(title, body))
	case osLinux:
		if isWSL() {
			return p.sendWindowsNotification(ctx, cfg, title, body)
		}
		return sendLinuxNotification(ctx, cfg, title, body)
	case osWindows:
		return p.sendWindowsNotification(ctx, cfg, title, body)
	default:
		return fmt.Errorf("system notifications not supported on %s", runtime.GOOS)
	}
}

func (p *SystemProvider) sendWindowsNotification(ctx context.Context, cfg systemConfig, title, body string) error {
	scriptPath, err := p.assets.ensureScript()
	if err != nil {
		return err
	}
	path := scriptPath
	iconPath := cfg.IconPath
	if isWSL() {
		if converted, err := wslPathToWindows(scriptPath); err == nil {
			path = converted
		}
		if iconPath != "" {
			if converted, err := wslPathToWindows(iconPath); err == nil {
				iconPath = converted
			}
		}
	}
	args := []string{
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		path,
		"-Title",
		title,
		"-Message",
		body,
		"-AppName",
		cfg.AppName,
		"-TimeoutMs",
		strconv.Itoa(cfg.TimeoutMS),
	}
	if iconPath != "" {
		args = append(args, "-IconPath", iconPath)
	}
	return runCommand(ctx, "powershell.exe", args...)
}

func sendLinuxNotification(ctx context.Context, cfg systemConfig, title, body string) error {
	if _, err := exec.LookPath("notify-send"); err == nil {
		return runCommand(ctx, "notify-send", "-t", strconv.Itoa(cfg.TimeoutMS), title, body)
	}
	if _, err := exec.LookPath("zenity"); err == nil {
		message := strings.TrimSpace(fmt.Sprintf("%s\n%s", title, body))
		return runCommand(ctx, "zenity", "--notification", "--text", message)
	}
	return fmt.Errorf("notify-send or zenity is required for system notifications")
}

func (p *SystemProvider) playSound(ctx context.Context, cfg systemConfig) error {
	switch runtime.GOOS {
	case osDarwin:
		soundPath := cfg.SoundFile
		if soundPath == "" {
			soundPath = "/System/Library/Sounds/Glass.aiff"
		}
		return runCommand(ctx, "afplay", soundPath)
	case osLinux:
		if isWSL() {
			return p.playWindowsSound(ctx, cfg)
		}
		if cfg.SoundFile != "" {
			if _, err := exec.LookPath("paplay"); err == nil {
				return runCommand(ctx, "paplay", cfg.SoundFile)
			}
			if _, err := exec.LookPath("aplay"); err == nil {
				return runCommand(ctx, "aplay", cfg.SoundFile)
			}
		}
		return runCommand(ctx, "sh", "-c", "printf '\\a'")
	case osWindows:
		return p.playWindowsSound(ctx, cfg)
	default:
		return nil
	}
}

func (p *SystemProvider) playWindowsSound(ctx context.Context, cfg systemConfig) error {
	if cfg.SoundFile != "" {
		soundPath := cfg.SoundFile
		if isWSL() {
			if converted, err := wslPathToWindows(soundPath); err == nil {
				soundPath = converted
			}
		}
		script := fmt.Sprintf(`(New-Object Media.SoundPlayer "%s").PlaySync()`, escapePowerShell(soundPath))
		return runCommand(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-c", script)
	}
	return runCommand(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-c", "[console]::beep(800,200)")
}

func buildAppleScript(title, body string) string {
	escapedTitle := strings.ReplaceAll(title, `"`, `\"`)
	escapedBody := strings.ReplaceAll(body, `"`, `\"`)
	return fmt.Sprintf(`display notification "%s" with title "%s"`, escapedBody, escapedTitle)
}

func escapePowerShell(value string) string {
	return strings.ReplaceAll(value, `"`, "`\"")
}

func runCommand(ctx context.Context, name string, args ...string) error {
	timeout := 5 * time.Second
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Start()
}

func isWSL() bool {
	if runtime.GOOS != osLinux {
		return false
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

func wslPathToWindows(path string) (string, error) {
	cmd := exec.Command("wslpath", "-w", path)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
