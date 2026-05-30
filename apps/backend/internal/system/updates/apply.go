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
	resp := s.buildResponse(version, releaseURL, checkedAt)
	if !resp.UpdateAvailable {
		return UpdatesResponse{}, nil, ErrNoUpdateAvailable
	}
	if !resp.ApplySupported {
		return UpdatesResponse{}, nil, fmt.Errorf("%w: %s", ErrApplyUnsupported, resp.ApplyUnsupportedReason)
	}
	_, metadata := s.detectInstallState()
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

func runLaunchdSelfUpdate(ctx context.Context, req applyRequest) (map[string]interface{}, error) {
	label := "com.kdlbs.kandev.self-update." + strconv.FormatInt(time.Now().UnixNano(), 10)
	args := launchdSelfUpdateArgs(req, label)
	out, err := exec.CommandContext(ctx, "launchctl", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("launchctl self-update helper: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return map[string]interface{}{
		applyResultStatusKey:     applyResultStarted,
		applyResultRunnerKey:     applyRunnerLaunchctl,
		"label":                  label,
		applyResultIntentPathKey: req.IntentPath,
	}, nil
}

func launchdSelfUpdateArgs(req applyRequest, label string) []string {
	args := []string{
		"submit",
		"-l", label,
		"--",
	}
	command := []string{
		req.Intent.Install.NodePath,
		req.Intent.Install.CLIEntry,
		"service",
		"self-update",
		"--intent",
		req.IntentPath,
	}
	if env := selfUpdateEnvironment(); len(env) > 0 {
		command = append(append([]string{"/usr/bin/env"}, env...), command...)
	}
	return append(args, command...)
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
