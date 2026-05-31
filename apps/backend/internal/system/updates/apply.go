package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/persistence"
)

type applyRunner func(context.Context, applyRequest) (map[string]interface{}, error)

type applyRequest struct {
	IntentPath string
	Intent     updateIntent
}

const (
	applyResultStatusKey     = "status"
	applyResultStarted       = "started"
	applyResultRunnerKey     = "runner"
	applyResultIntentPathKey = "intent_path"
	applyRunnerFake          = "fake"
	applyRunnerSystemdRun    = "systemd-run"
	applyRunnerLaunchctl     = "launchctl"
)

type updateIntent struct {
	Version       int                    `json:"version"`
	TargetTag     string                 `json:"target_tag"`
	TargetVersion string                 `json:"target_version"`
	LatestURL     string                 `json:"latest_url,omitempty"`
	Install       serviceInstallMetadata `json:"install"`
	CreatedAt     string                 `json:"created_at"`
}

func (s *Service) applyPreflight() (UpdatesResponse, *serviceInstallMetadata, error) {
	version, releaseURL, checkedAt, err := persistence.ReadLatestVersion(s.pool.Reader())
	if err != nil {
		return UpdatesResponse{}, nil, err
	}
	// Read install state once so the ApplySupported gate below and the intent
	// file written by the caller both reflect the same snapshot.
	install, metadata := s.detectInstallState()
	resp := s.buildResponseFrom(install, version, releaseURL, checkedAt)
	if !resp.UpdateAvailable {
		return UpdatesResponse{}, nil, ErrNoUpdateAvailable
	}
	if !resp.ApplySupported {
		return UpdatesResponse{}, nil, fmt.Errorf("%w: %s", ErrApplyUnsupported, resp.ApplyUnsupportedReason)
	}
	if metadata == nil {
		return UpdatesResponse{}, nil, ErrApplyUnsupported
	}
	return resp, metadata, nil
}

func (s *Service) writeApplyIntent(resp UpdatesResponse, metadata *serviceInstallMetadata) (string, updateIntent, error) {
	if s.homeDir == "" {
		return "", updateIntent{}, errors.New("home dir is unknown")
	}
	intent := updateIntent{
		Version:       1,
		TargetTag:     resp.Latest,
		TargetVersion: strings.TrimPrefix(resp.Latest, "v"),
		LatestURL:     resp.LatestURL,
		Install:       *metadata,
		CreatedAt:     s.now().UTC().Format(time.RFC3339Nano),
	}
	dir := filepath.Join(s.homeDir, "service", "update-intents")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", updateIntent{}, err
	}
	name := fmt.Sprintf("%d.json", s.now().UTC().UnixNano())
	path := filepath.Join(dir, name)
	data, err := json.MarshalIndent(intent, "", "  ")
	if err != nil {
		return "", updateIntent{}, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", updateIntent{}, err
	}
	return path, intent, nil
}

func (s *Service) defaultApplyRunner(ctx context.Context, req applyRequest) (map[string]interface{}, error) {
	if s.getenv("KANDEV_E2E_MOCK") == "true" {
		return map[string]interface{}{
			applyResultStatusKey:     applyResultStarted,
			applyResultRunnerKey:     applyRunnerFake,
			applyResultIntentPathKey: req.IntentPath,
		}, nil
	}
	install := req.Intent.Install
	switch install.Manager {
	case serviceManagerSystemd:
		return runSystemdSelfUpdate(ctx, req)
	case serviceManagerLaunchd:
		return runLaunchdSelfUpdate(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported service manager %q", install.Manager)
	}
}

func runSystemdSelfUpdate(ctx context.Context, req applyRequest) (map[string]interface{}, error) {
	unitName := "kandev-self-update-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	args := systemdSelfUpdateArgs(req, unitName)
	out, err := exec.CommandContext(ctx, "systemd-run", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("systemd-run self-update helper: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return map[string]interface{}{
		applyResultStatusKey:     applyResultStarted,
		applyResultRunnerKey:     applyRunnerSystemdRun,
		"unit":                   unitName,
		applyResultIntentPathKey: req.IntentPath,
	}, nil
}

func systemdSelfUpdateArgs(req applyRequest, unitName string) []string {
	args := []string{
		"--user",
		"--unit", unitName,
		"--collect",
	}
	for _, env := range selfUpdateEnvironment() {
		args = append(args, "--setenv="+env)
	}
	return append(args,
		req.Intent.Install.NodePath,
		req.Intent.Install.CLIEntry,
		"service",
		"self-update",
		"--intent",
		req.IntentPath,
	)
}

const selfUpdateHelperDirName = "self-update"

// runLaunchdSelfUpdate spawns the self-update helper as a transient launchd job.
//
// The previous implementation used `launchctl submit`, which registers the job
// with an implicit KeepAlive — launchd re-ran the one-shot updater every ~15s
// forever, so the service reinstalled and restarted in an endless loop. A
// bootstrapped plist with KeepAlive=false runs exactly once (matching what
// `systemd-run --collect` gives us on Linux). The job lives outside the kandev
// service so reinstalling/restarting the service mid-update doesn't kill it.
func runLaunchdSelfUpdate(ctx context.Context, req applyRequest) (map[string]interface{}, error) {
	label := "com.kdlbs.kandev.self-update." + strconv.FormatInt(time.Now().UnixNano(), 10)
	uid := os.Getuid()
	domain := launchdSelfUpdateDomain(req, uid)

	dir := filepath.Join(req.Intent.Install.HomeDir, "service", selfUpdateHelperDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("self-update helper dir: %w", err)
	}
	// Boot out and delete any idle helper job left loaded by a prior update so
	// they don't accumulate (a KeepAlive=false job stays registered after it
	// exits until it's booted out).
	cleanupStaleLaunchdHelpers(ctx, dir, domain)

	plistPath := filepath.Join(dir, label+".plist")
	if err := os.WriteFile(plistPath, []byte(renderLaunchdHelperPlist(label, req)), 0o600); err != nil {
		return nil, fmt.Errorf("write self-update helper plist: %w", err)
	}

	out, err := exec.CommandContext(ctx, "launchctl", "bootstrap", domain, plistPath).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("launchctl bootstrap self-update helper: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return map[string]interface{}{
		applyResultStatusKey:     applyResultStarted,
		applyResultRunnerKey:     applyRunnerLaunchctl,
		"label":                  label,
		applyResultIntentPathKey: req.IntentPath,
	}, nil
}

func launchdSelfUpdateDomain(req applyRequest, uid int) string {
	if req.Intent.Install.Mode == installModeSystem {
		return "system"
	}
	return fmt.Sprintf("gui/%d", uid)
}

func cleanupStaleLaunchdHelpers(ctx context.Context, dir, domain string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".plist") {
			continue
		}
		label := strings.TrimSuffix(name, ".plist")
		_ = exec.CommandContext(ctx, "launchctl", "bootout", domain+"/"+label).Run()
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// renderLaunchdHelperPlist builds a transient LaunchAgent plist for the helper:
// RunAtLoad fires it once on bootstrap, KeepAlive=false stops launchd from
// re-running it. Update env (PATH/npm prefix) is carried via EnvironmentVariables
// so the helper can resolve npm; stdout/stderr are captured to the log dir as a
// fallback alongside the helper's own self-update-<ts>.log.
func renderLaunchdHelperPlist(label string, req applyRequest) string {
	args := []string{
		req.Intent.Install.NodePath,
		req.Intent.Install.CLIEntry,
		"service",
		"self-update",
		"--intent",
		req.IntentPath,
	}
	var argsXML strings.Builder
	for _, arg := range args {
		argsXML.WriteString("    <string>" + escapeXML(arg) + "</string>\n")
	}
	var envXML strings.Builder
	for _, kv := range selfUpdateEnvironment() {
		key, value, _ := strings.Cut(kv, "=")
		envXML.WriteString("    <key>" + escapeXML(key) + "</key>\n")
		envXML.WriteString("    <string>" + escapeXML(value) + "</string>\n")
	}
	logPath := filepath.Join(req.Intent.Install.LogDir, "self-update-launchd.log")
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + escapeXML(label) + `</string>
  <key>ProgramArguments</key>
  <array>
` + argsXML.String() + `  </array>
  <key>EnvironmentVariables</key>
  <dict>
` + envXML.String() + `  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <false/>
  <key>ProcessType</key>
  <string>Background</string>
  <key>StandardOutPath</key>
  <string>` + escapeXML(logPath) + `</string>
  <key>StandardErrorPath</key>
  <string>` + escapeXML(logPath) + `</string>
</dict>
</plist>
`
}

func selfUpdateEnvironment() []string {
	keys := []string{"PATH", "npm_config_prefix", "NPM_CONFIG_PREFIX"}
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func sameOriginOrNoOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	requestHost := r.Host
	if requestHost == "" {
		requestHost = r.URL.Host
	}
	return strings.EqualFold(originURL.Host, requestHost)
}
