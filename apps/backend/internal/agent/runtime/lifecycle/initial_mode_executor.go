package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle/initialmode"
)

const (
	initialModeHomePlaceholder = "{home}"
	initialModeHomePrefix      = initialModeHomePlaceholder + "/"
)

type initialModeFilePlan struct {
	delivery agents.InitialModeDelivery
	value    string
	dir      string
}

func initialModeFilePlanForRequest(req *ExecutorCreateRequest) (initialModeFilePlan, error) {
	if req == nil || req.InitialMode == nil || req.AgentConfig == nil {
		return initialModeFilePlan{}, fmt.Errorf("initial mode request is incomplete")
	}
	runtimeCfg := req.AgentConfig.Runtime()
	if runtimeCfg == nil {
		return initialModeFilePlan{}, fmt.Errorf("agent runtime configuration is unavailable")
	}
	delivery := runtimeCfg.InitialMode
	value, ok := delivery.SettingsValue(req.InitialMode.Mode)
	if !ok || delivery.SettingsFileName == "" || len(delivery.ModeKeyPath) == 0 {
		return initialModeFilePlan{}, fmt.Errorf("agent cannot encode start mode %q", req.InitialMode.Mode)
	}
	dir, err := initialModeConfigRelativeDir(delivery)
	if err != nil {
		return initialModeFilePlan{}, err
	}
	return initialModeFilePlan{delivery: delivery, value: value, dir: dir}, nil
}

func markInitialModeDelivered(req *ExecutorCreateRequest, installedConfigDir, exportedConfigDir string) error {
	if req == nil || req.InitialMode == nil {
		return fmt.Errorf("initial mode request is incomplete")
	}
	installedConfigDir = path.Clean(installedConfigDir)
	exportedConfigDir = path.Clean(exportedConfigDir)
	if installedConfigDir == "." || exportedConfigDir == "." || installedConfigDir != exportedConfigDir {
		reason := fmt.Sprintf("installed start-mode directory %q does not match exported agent config directory %q", installedConfigDir, exportedConfigDir)
		req.InitialMode.Reason = reason
		return fmt.Errorf("%s", reason)
	}
	req.InitialMode.ConfigDir = installedConfigDir
	req.InitialMode.Delivered = true
	req.InitialMode.Reason = ""
	return nil
}

func exportedInitialModeConfigDir(req *ExecutorCreateRequest) (string, error) {
	plan, err := initialModeFilePlanForRequest(req)
	if err != nil {
		return "", err
	}
	if configured := strings.TrimSpace(req.Env[plan.delivery.ConfigDirEnvVar]); configured != "" {
		return path.Clean(configured), nil
	}
	if runtimeCfg := req.AgentConfig.Runtime(); runtimeCfg != nil && runtimeCfg.SessionConfig.SessionDirTarget != "" {
		return path.Clean(runtimeCfg.SessionConfig.SessionDirTarget), nil
	}
	return "", fmt.Errorf("agent config directory %s was not exported", plan.delivery.ConfigDirEnvVar)
}

func initialModeConfigRelativeDir(delivery agents.InitialModeDelivery) (string, error) {
	template := delivery.DefaultConfigDirTemplate
	if template == initialModeHomePlaceholder {
		return ".", nil
	}
	if !strings.HasPrefix(template, initialModeHomePrefix) {
		return "", fmt.Errorf("agent start-mode config path must be relative to its home")
	}
	relative := path.Clean(strings.TrimPrefix(template, initialModeHomePrefix))
	if !isSafePortableRelativePath(relative) {
		return "", fmt.Errorf("agent start-mode config path is unsafe")
	}
	return relative, nil
}

func initialModeSessionHome(home, instanceID string) (string, error) {
	if strings.TrimSpace(home) == "" || instanceID == "" || path.Base(instanceID) != instanceID || instanceID == "." || instanceID == ".." {
		return "", fmt.Errorf("initial mode session home is invalid")
	}
	return path.Join(home, ".kandev", "agent-sessions", instanceID), nil
}

func installInitialModeLocally(req *ExecutorCreateRequest, sessionHome string) error {
	plan, err := initialModeFilePlanForRequest(req)
	if err != nil {
		return err
	}
	settingsPath := filepath.Join(sessionHome, filepath.FromSlash(path.Join(plan.dir, plan.delivery.SettingsFileName)))
	settingsJSON, err := readPreparedSettings(settingsPath)
	if err != nil {
		return err
	}
	targetDir := filepath.Join(sessionHome, filepath.FromSlash(plan.dir))
	if _, err := initialmode.Materialize(initialmode.Request{
		SettingsJSON:     settingsJSON,
		TargetDir:        targetDir,
		SettingsFileName: plan.delivery.SettingsFileName,
		ModeKeyPath:      plan.delivery.ModeKeyPath,
		ModeValue:        plan.value,
	}); err != nil {
		return err
	}
	runtimeCfg := req.AgentConfig.Runtime()
	template := runtimeCfg.SessionConfig.SessionDirTemplate
	if !strings.HasPrefix(template, initialModeHomePrefix) {
		return fmt.Errorf("agent session directory is not relative to its home")
	}
	sessionConfigRel := path.Clean(strings.TrimPrefix(template, initialModeHomePrefix))
	expectedTargetDir := filepath.Join(sessionHome, filepath.FromSlash(sessionConfigRel))
	if sessionConfigRel != plan.dir || filepath.Clean(targetDir) != filepath.Clean(expectedTargetDir) {
		return fmt.Errorf("installed start-mode file does not match the agent session directory")
	}
	exportedConfigDir, err := exportedInitialModeConfigDir(req)
	if err != nil {
		return err
	}
	return markInitialModeDelivered(req, runtimeCfg.SessionConfig.SessionDirTarget, exportedConfigDir)
}

func readPreparedSettings(settingsPath string) ([]byte, error) {
	info, err := os.Lstat(settingsPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect selected settings: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("selected settings file is not a regular file")
	}
	settingsJSON, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("read selected settings: %w", err)
	}
	return settingsJSON, nil
}

func installInitialModeRemotely(
	ctx context.Context,
	uploader FileUploader,
	req *ExecutorCreateRequest,
	targetHome string,
	settingsJSON []byte,
) (string, error) {
	plan, err := initialModeFilePlanForRequest(req)
	if err != nil {
		return "", err
	}
	merged, err := initialmode.MergeSettings(settingsJSON, plan.delivery.ModeKeyPath, plan.value)
	if err != nil {
		return "", err
	}
	settingsPath := path.Join(targetHome, plan.dir, plan.delivery.SettingsFileName)
	if err := uploader.WriteFile(ctx, settingsPath, merged, 0o600); err != nil {
		return "", fmt.Errorf("install initial mode at %s: %w", settingsPath, err)
	}
	return path.Join(targetHome, plan.dir), nil
}

func selectedInitialModeSettings(req *ExecutorCreateRequest) ([]byte, error) {
	plan, err := initialModeFilePlanForRequest(req)
	if err != nil {
		return nil, err
	}
	selected := selectedPortableConfigBundleIDs(req.Metadata)
	return selectedPortableSettingsJSON(req.AgentConfig, selected, path.Join(plan.dir, plan.delivery.SettingsFileName))
}
