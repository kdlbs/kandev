package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/sessionmcp"
	"slices"
)

// codexMinVersion is the lowest Codex release whose configuration accepts
// env_http_headers under mcp_servers. Source: openai/codex commit a5d48a7
// ("[MCP] Allow specifying custom headers with streamable http servers",
// #5241, 2025-10-17), first contained in the stable tag rust-v0.47.0.
var codexMinVersion = [3]int{0, 47, 0}

const (
	codexProbeTimeout  = 20 * time.Second
	codexProbePoll     = 250 * time.Millisecond
	codexProbeCacheTTL = 10 * time.Minute
)

var codexVersionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// codexProbeFunc runs the version probe command on the host that runs the
// agent and returns its combined output.
type codexProbeFunc func(ctx context.Context, execution *AgentExecution, command string) (string, error)

type codexProbeCache struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

var codexVerified = &codexProbeCache{entries: map[string]time.Time{}}

func (c *codexProbeCache) fresh(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	at, ok := c.entries[key]
	return ok && time.Since(at) < codexProbeCacheTTL
}

func (c *codexProbeCache) store(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = time.Now()
}

// checkCodexCarrier verifies, for a command that carries the Codex token
// override, that the carrier will work: the command starts with npx or codex
// with no launch wrapper, and the resolved Codex is at least codexMinVersion.
func (m *Manager) checkCodexCarrier(ctx context.Context, execution *AgentExecution, argv []string, wrapper launchwrapper.Spec) error {
	if sandboxedMCPRoute(execution) || !slices.Contains(argv, sessionmcp.CodexTokenArg) {
		return nil
	}
	if _, ok := m.sessionKeyStore(); !ok {
		return nil
	}
	resolver, ok := codexResolverArgs(argv)
	if !ok || !wrapper.IsZero() {
		return &LaunchRefusal{Code: CodeMCPTokenCarrierUnavailable, Cause: CauseCodexWrapper}
	}
	command := shellJoin(append(resolver, "--version"))
	key := string(execution.RuntimeName) + "|" + codexProbeHost(execution) + "|" + command
	if codexVerified.fresh(key) {
		return nil
	}
	probe := m.codexProbe
	if probe == nil {
		probe = probeCodexOnInstance
	}
	probeCtx, cancel := context.WithTimeout(ctx, codexProbeTimeout)
	defer cancel()
	out, err := probe(probeCtx, execution, command)
	if err != nil {
		return &LaunchRefusal{Code: CodeMCPTokenCarrierUnavailable, Cause: CauseCodexProbeFailed, Err: err}
	}
	version, ok := parseCodexVersion(out)
	if !ok {
		return &LaunchRefusal{Code: CodeMCPTokenCarrierUnavailable, Cause: CauseCodexProbeFailed, Err: errors.New("unparseable codex version output")}
	}
	if versionLess(version, codexMinVersion) {
		return &LaunchRefusal{Code: CodeMCPTokenCarrierUnavailable, Cause: CauseCodexVersion}
	}
	codexVerified.store(key)
	return nil
}

func codexProbeHost(execution *AgentExecution) string {
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return ""
	}
	return client.Host()
}

// codexResolverArgs returns the command's own resolver form: "codex" or
// "npx [flags] @openai/codex". ok is false when the command does not start
// with npx or codex by basename.
func codexResolverArgs(argv []string) ([]string, bool) {
	if len(argv) == 0 {
		return nil, false
	}
	switch filepath.Base(argv[0]) {
	case "codex":
		return []string{argv[0]}, true
	case "npx":
		for i := 1; i < len(argv); i++ {
			if strings.HasPrefix(argv[i], "@openai/codex") {
				return append([]string(nil), argv[:i+1]...), true
			}
		}
	}
	return nil, false
}

func parseCodexVersion(out string) ([3]int, bool) {
	match := codexVersionPattern.FindStringSubmatch(out)
	if match == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := range v {
		n, err := strconv.Atoi(match[i+1])
		if err != nil {
			return [3]int{}, false
		}
		v[i] = n
	}
	return v, true
}

func versionLess(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

// probeCodexOnInstance runs command through the execution's agentctl instance
// so it resolves Codex on the host and with the environment the agent uses.
func probeCodexOnInstance(ctx context.Context, execution *AgentExecution, command string) (string, error) {
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return "", errors.New("agentctl client unavailable")
	}
	info, err := client.StartProcess(ctx, agentctl.StartProcessRequest{
		SessionID:  execution.SessionID,
		Kind:       streams.ProcessKindCustom,
		Command:    command,
		WorkingDir: execution.WorkspacePath,
	})
	if err != nil {
		return "", err
	}
	ticker := time.NewTicker(codexProbePoll)
	defer ticker.Stop()
	for {
		current, err := client.GetProcess(ctx, info.ID, true)
		if err != nil {
			return "", err
		}
		switch current.Status {
		case streams.ProcessStatusStarting, streams.ProcessStatusRunning:
		default:
			if current.ExitCode == nil || *current.ExitCode != 0 {
				return "", fmt.Errorf("codex version probe exited with status %s", current.Status)
			}
			var out strings.Builder
			for _, chunk := range current.Output {
				out.WriteString(chunk.Data)
			}
			return out.String(), nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}
