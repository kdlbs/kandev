package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/opencode"
)

const openCodeCredentialMethodID = "agent:opencode-acp:files:0"
const openCodeCredentialAgentID = "opencode-acp"

type openCodeCredentialSource struct {
	env        []string
	executable string
}

func captureOpenCodeCredentialSource() openCodeCredentialSource {
	path, _ := exec.LookPath("npx")
	return openCodeCredentialSource{env: os.Environ(), executable: path}
}

func (s openCodeCredentialSource) export(parent context.Context, runtime managedruntime.OpenCodeRuntime) ([]byte, error) {
	args, err := runtime.CredentialCommand("export")
	if err != nil {
		return nil, err
	}
	if s.executable == "" {
		return nil, errors.New("OpenCode credential export requires npx on the controller source PATH")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	cmd := exec.Command(s.executable, args[1:]...)
	cmd.Env = append([]string(nil), s.env...)
	return collectOpenCodeCredentialExport(ctx, cancel, cmd)
}

func (m *Manager) provisionOpenCodeCredentials(ctx context.Context, execution *AgentExecution, client *agentctl.Client) error {
	if execution.AgentID != openCodeCredentialAgentID || execution.RuntimeName == agentruntime.RuntimeStandalone {
		return nil
	}
	if execution.OpenCodeRuntime.Family != managedruntime.OpenCodeFamilyV2 {
		return nil
	}
	selected := execution.RuntimeName == agentruntime.RuntimeDocker
	var ids []string
	if raw := getMetadataString(execution.MetadataSnapshot(), "remote_credentials"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			return errors.New("invalid remote credential selection")
		}
		selected = slices.Contains(ids, openCodeCredentialMethodID)
	}
	if !selected {
		return nil
	}
	if _, err := execution.OpenCodeRuntime.CredentialCommand("import"); err != nil {
		return err
	}
	if err := client.CheckOpenCodeCredentialCapability(ctx); err != nil {
		return err
	}
	state, err := client.OpenCodeCredentialStatus(ctx)
	if err != nil {
		return err
	}
	if state == opencode.CredentialStorePresent {
		return nil
	}
	data, err := m.openCodeCredentialSource.export(ctx, execution.OpenCodeRuntime)
	if err != nil {
		return err
	}
	return client.ImportOpenCodeCredentials(ctx, data)
}

func collectOpenCodeCredentialExport(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd) ([]byte, error) {
	if err := managedruntime.PrepareNPMProjectPrefix(cmd.Args); err != nil {
		return nil, errors.New("could not prepare OpenCode credential export")
	}
	cmd.WaitDelay = 5 * time.Second
	output := credentialExportBuffer{cancel: cancel}
	cmd.Stdout = &output
	err := runOpenCodeExport(ctx, cmd)
	if err != nil {
		return nil, errors.New("OpenCode credential export failed; sign in on the Kandev host with a supported V2 CLI")
	}
	data := output.buffer.Bytes()
	var entries []json.RawMessage
	if len(data) > 1<<20 || json.Unmarshal(data, &entries) != nil || len(entries) == 0 {
		return nil, errors.New("OpenCode credential export did not contain accounts")
	}
	return data, nil
}

type credentialExportBuffer struct {
	buffer bytes.Buffer
	cancel context.CancelFunc
}

func (b *credentialExportBuffer) Write(data []byte) (int, error) {
	if len(data) > (1<<20)-b.buffer.Len() {
		b.cancel()
		return 0, errors.New("OpenCode credential export exceeded its limit")
	}
	return b.buffer.Write(data)
}

func runOpenCodeExport(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	configureCredentialExportProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	stop, err := captureCredentialExportProcess(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	defer stop()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		stop()
		<-done //nolint:errcheck // Cancellation is authoritative after the child has been reaped.
		return ctx.Err()
	}
}
