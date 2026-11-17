package process

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/tailscale/hujson"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/server/opencodeplugin"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	commonopencodeplugin "github.com/kandev/kandev/internal/common/opencodeplugin"
)

const (
	openCodeConfigContentEnv = "OPENCODE_CONFIG_CONTENT"
	openCodePluginRootName   = "opencode-plugin"
)

var (
	errOpenCodePluginConfigInvalid  = errors.New("opencode inline configuration is not a JSON object")
	errOpenCodePluginConfigConflict = errors.New("opencode inline configuration already owns the reserved plugin entry")
)

// prepareOpenCodePlugin starts the private host and composes the child-only
// configuration layer before the OpenCode process is created. Every failure is
// bounded and leaves the ordinary ACP launch usable.
func (m *Manager) prepareOpenCodePlugin() {
	runtime, ok := m.openCodePluginRuntime()
	if !ok || m.openCodePlugin() != nil {
		return
	}
	artifactDir, artifactHash, ok := m.stageOpenCodePluginArtifact()
	if !ok {
		return
	}
	host, ok := m.startOpenCodePluginHost(runtime, artifactDir, artifactHash)
	if !ok {
		return
	}
	if !m.applyOpenCodePluginEnvironment(host, artifactDir) {
		_ = host.Close(context.Background())
		m.logOpenCodePluginUnavailable("configuration_conflict")
		return
	}
	m.SetOpenCodePluginHost(host)
	m.logger.Info("opencode integration plugin prepared")
}

func (m *Manager) openCodePluginRuntime() (*agentctltypes.OpenCodePluginRuntime, bool) {
	runtime := m.cfg.OpenCodePluginRuntime
	if !m.cfg.OpenCodePluginEnabled || runtime == nil || !m.cfg.AgentBackgroundWorkEnabled {
		return nil, false
	}
	if runtime.Family != "v2" || runtime.Version != commonopencodeplugin.SupportedRuntimeVersion {
		return nil, false
	}
	if m.cfg.AgentType != agents.OpenCodeACPAgentID {
		return nil, false
	}
	if string(m.cfg.Protocol) != "acp" || m.cfg.TaskID == "" || m.cfg.SessionID == "" {
		return nil, false
	}
	if envKeyStripped(m.cfg.StripEnv, openCodeConfigContentEnv) || m.cfg.HomeDir == "" {
		return nil, false
	}
	return runtime, true
}

func (m *Manager) stageOpenCodePluginArtifact() (string, string, bool) {
	manifest, err := opencodeplugin.ArtifactManifestValue()
	if err != nil {
		m.logOpenCodePluginUnavailable("artifact_manifest")
		return "", "", false
	}
	artifactDir, err := opencodeplugin.StageArtifact(filepath.Join(m.cfg.HomeDir, openCodePluginRootName, "artifacts"))
	if err != nil {
		m.logOpenCodePluginUnavailable("artifact_stage")
		return "", "", false
	}
	return artifactDir, manifest.SHA256, true
}

func (m *Manager) startOpenCodePluginHost(
	runtime *agentctltypes.OpenCodePluginRuntime,
	artifactDir, artifactHash string,
) (*opencodeplugin.Host, bool) {
	host, err := opencodeplugin.NewHost(opencodeplugin.Options{
		HomeDir:        m.cfg.HomeDir,
		ArtifactDir:    artifactDir,
		ArtifactHash:   artifactHash,
		RuntimeFamily:  runtime.Family,
		RuntimeVersion: runtime.Version,
		Committer:      m,
		Logger:         m.logger.Zap(),
	})
	if err != nil {
		m.logOpenCodePluginUnavailable("host_configuration")
		return nil, false
	}
	if err := host.Start(context.Background()); err != nil {
		m.logOpenCodePluginUnavailable("host_start")
		return nil, false
	}
	return host, true
}

func (m *Manager) applyOpenCodePluginEnvironment(host *opencodeplugin.Host, artifactDir string) bool {
	existing := processEnvValue(m.cfg.AgentEnv, openCodeConfigContentEnv)
	composed, err := composeOpenCodePluginConfig(existing, artifactDir, host.BootstrapPath())
	if err != nil {
		return false
	}
	m.cfg.AgentEnv = upsertEnvValue(m.cfg.AgentEnv, openCodeConfigContentEnv, composed)
	return true
}

// watchOpenCodePluginReadiness requires the subscription barrier before claims
// can settle jobs. A timeout revokes the lease and keeps ACP usable.
func (m *Manager) watchOpenCodePluginReadiness() {
	host := m.openCodePlugin()
	if host == nil {
		return
	}
	go func() {
		select {
		case <-host.FirstContact():
		case <-time.After(opencodeplugin.BootBudget):
			m.disableOpenCodePluginHost(host, "boot_timeout")
			return
		case <-m.lifetimeCtx.Done():
			return
		}
		ctx, cancel := context.WithTimeout(m.lifetimeCtx, opencodeplugin.ReadinessBudget)
		defer cancel()
		if err := host.WaitReady(ctx); err != nil {
			m.disableOpenCodePluginHost(host, "readiness_timeout")
		}
	}()
}

func (m *Manager) disableOpenCodePluginHost(expected *opencodeplugin.Host, reason string) {
	if expected == nil || m.detachOpenCodePluginHost(expected) == nil {
		return
	}
	m.logger.Warn("opencode integration plugin disabled", zap.String("reason", reason))
	ctx, cancel := context.WithTimeout(context.Background(), opencodeplugin.ReadinessBudget)
	defer cancel()
	_ = expected.Close(ctx)
}

func (m *Manager) closeOpenCodePlugin(ctx context.Context) {
	host := m.detachOpenCodePluginHost(nil)
	if host == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = host.Close(ctx)
}

// detachOpenCodePluginHost removes the expected host and its admission binding
// in one critical section. A nil expected value detaches whatever is attached.
func (m *Manager) detachOpenCodePluginHost(expected *opencodeplugin.Host) *opencodeplugin.Host {
	m.openCodePluginMu.Lock()
	defer m.openCodePluginMu.Unlock()
	host := m.openCodePluginHost
	if host == nil || (expected != nil && host != expected) {
		return nil
	}
	m.openCodePluginHost = nil
	m.openCodePluginValidator = nil
	m.openCodePluginGuard = nil
	m.openCodePluginRetirer = nil
	return host
}

func (m *Manager) logOpenCodePluginUnavailable(reason string) {
	m.logger.Warn("opencode integration plugin unavailable", zap.String("reason", reason))
}

func composeOpenCodePluginConfig(existing, artifactDir, bootstrapPath string) (string, error) {
	document := map[string]any{}
	if strings.TrimSpace(existing) != "" {
		standardized, err := hujson.Standardize([]byte(existing))
		if err != nil {
			return "", errOpenCodePluginConfigInvalid
		}
		if err := json.Unmarshal(standardized, &document); err != nil {
			return "", errOpenCodePluginConfigInvalid
		}
		if document == nil {
			document = map[string]any{}
		}
	}
	entry := map[string]any{
		"package": artifactDir,
		"options": map[string]any{"bootstrapFile": bootstrapPath},
	}
	plugins, changed, err := mergeOpenCodePluginEntry(document["plugins"], entry)
	if err != nil {
		return "", err
	}
	if changed {
		document["plugins"] = plugins
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", errOpenCodePluginConfigInvalid
	}
	return string(encoded), nil
}

func mergeOpenCodePluginEntry(current any, entry map[string]any) ([]any, bool, error) {
	if current == nil {
		return []any{entry}, true, nil
	}
	list, ok := current.([]any)
	if !ok {
		return nil, false, errOpenCodePluginConfigConflict
	}
	artifactDir := entry["package"].(string)
	for _, item := range list {
		reserved, exact := classifyPluginEntry(item, artifactDir, entry["options"])
		if exact {
			return list, false, nil
		}
		if reserved {
			return nil, false, errOpenCodePluginConfigConflict
		}
	}
	replaced := make([]any, 0, len(list)+1)
	for _, item := range list {
		if !isOwnedPluginEntry(item, artifactDir) {
			replaced = append(replaced, item)
		}
	}
	return append(replaced, entry), true, nil
}

func classifyPluginEntry(item any, artifactDir string, options any) (reserved, exact bool) {
	switch value := item.(type) {
	case string:
		return value == injectedKandevMCPServerName || value == artifactDir, false
	case map[string]any:
		pkg, _ := value["package"].(string)
		if pkg == injectedKandevMCPServerName {
			return true, false
		}
		if pkg != artifactDir {
			return false, false
		}
		if samePluginOptions(value["options"], options) {
			return false, true
		}
		return !ownedPluginOptions(value["options"]), false
	default:
		return false, false
	}
}

func isOwnedPluginEntry(item any, artifactDir string) bool {
	value, ok := item.(map[string]any)
	if !ok || value["package"] != artifactDir {
		return false
	}
	return ownedPluginOptions(value["options"])
}

func ownedPluginOptions(value any) bool {
	options, ok := value.(map[string]any)
	if !ok || len(options) != 1 {
		return false
	}
	_, ok = options["bootstrapFile"].(string)
	return ok
}

func samePluginOptions(left, right any) bool {
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftRaw) == string(rightRaw)
}

func processEnvValue(env []string, key string) string {
	prefix := key + "="
	value := ""
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			value = strings.TrimPrefix(item, prefix)
		}
	}
	return value
}

func envKeyStripped(stripEnv []string, key string) bool {
	for _, item := range stripEnv {
		if item == key {
			return true
		}
	}
	return false
}
