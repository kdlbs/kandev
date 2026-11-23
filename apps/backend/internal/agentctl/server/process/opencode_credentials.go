package process

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/filelock"
	"github.com/kandev/kandev/internal/common/npmresolution"
	"github.com/kandev/kandev/internal/common/opencode"
	tools "github.com/kandev/kandev/internal/tools/installer"
)

const (
	openCodeCredentialStoreLockNamespace = "opencode-credential-stores"
	openCodeCredentialPreferOfflineArg   = "--prefer-offline"
	openCodeCredentialPreferOnlineArg    = "--prefer-online"
)

// OpenCodeCredentialStatus returns only readiness, never credential data.
func (m *Manager) OpenCodeCredentialStatus(parent context.Context) (state string, err error) {
	m.startMu.Lock()
	if m.credentialImport || m.commandActive() || m.passthrough.Live() {
		m.startMu.Unlock()
		return "", errors.New("OpenCode credential status requires an idle agent")
	}
	args, err := m.cfg.OpenCodeRuntime.CredentialCommand("export")
	if err == nil {
		args, err = m.prepareSessionArgs(args)
	}
	dir := m.cfg.WorkDir
	if err == nil {
		m.credentialImport = true
	}
	m.startMu.Unlock()
	if err != nil {
		return "", err
	}
	defer func() {
		if releaseErr := m.releaseOpenCodeCredentialAdmission(); releaseErr != nil {
			err = errors.Join(err, errors.New("could not release OpenCode credential store lock"))
		}
	}()
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	storePath, err := m.openCodeCredentialStorePath(ctx)
	if err != nil {
		return "", err
	}
	lock, err := filelock.Acquire(ctx, openCodeCredentialStoreLockNamespace, storePath)
	if err != nil {
		return "", errors.New("could not lock OpenCode credential store")
	}
	m.startMu.Lock()
	m.credentialStoreLock = lock
	m.startMu.Unlock()
	marker := openCodePendingMarker(storePath)
	if pending, err := inspectOpenCodePendingMarker(marker); err != nil {
		return "", err
	} else if pending {
		return opencode.CredentialStorePending, nil
	}
	return m.inspectOpenCodeCredentials(ctx, args, dir)
}

func (m *Manager) inspectOpenCodeCredentials(ctx context.Context, args []string, dir string) (string, error) {
	data, err := m.runOpenCodeCredentialReadCommand(ctx, args, dir)
	var entries []struct {
		ID string `json:"id"`
	}
	// The generic command runner caps output. Reject a saturated capture rather
	// than mistaking a truncated JSON prefix for a successful empty store.
	if err != nil || len(data) >= managedCommandOutputLimit || json.Unmarshal(data, &entries) != nil || entries == nil {
		return "", errors.New("could not inspect OpenCode credentials")
	}
	if len(entries) == 0 {
		return opencode.CredentialStoreEmpty, nil
	}
	for _, entry := range entries {
		if entry.ID == "" {
			return "", errors.New("could not inspect OpenCode credentials")
		}
	}
	return opencode.CredentialStorePresent, nil
}

// ImportOpenCodeCredentials provisions the configured session through the CLI's
// merge contract. Credential values are carried only through stdin.
func (m *Manager) ImportOpenCodeCredentials(ctx context.Context, data []byte) (err error) {
	m.startMu.Lock()
	if m.credentialImport || m.commandActive() || m.passthrough.Live() {
		m.startMu.Unlock()
		return errors.New("OpenCode credential import requires an idle agent")
	}
	args, err := m.cfg.OpenCodeRuntime.CredentialCommand("import")
	if err == nil {
		args, err = m.prepareSessionArgs(args)
	}
	dir := m.cfg.WorkDir
	if err == nil {
		m.credentialImport = true
	}
	m.startMu.Unlock()
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := m.releaseOpenCodeCredentialAdmission(); releaseErr != nil {
			err = errors.Join(err, errors.New("could not release OpenCode credential store lock"))
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	storePath, err := m.openCodeCredentialStorePath(ctx)
	if err != nil {
		return err
	}
	lock, err := filelock.Acquire(ctx, openCodeCredentialStoreLockNamespace, storePath)
	if err != nil {
		return errors.New("could not lock OpenCode credential store")
	}
	m.startMu.Lock()
	m.credentialStoreLock = lock
	m.startMu.Unlock()
	marker := openCodePendingMarker(storePath)
	ready, err := m.prepareOpenCodeBootstrap(ctx, marker, dir)
	if err != nil || ready {
		return err
	}
	// Import mutates the credential store. Do not replay it after an ambiguous failure.
	_, _, err = m.runManagedCommandInput(ctx, tools.CommandSpec{Path: args[0], Args: args[1:], Dir: dir}, data)
	if err != nil {
		return errors.New("OpenCode credential import failed")
	}
	if err := os.Remove(marker); err != nil {
		return errors.New("could not complete OpenCode bootstrap state")
	}
	return syncOpenCodeMarkerDirectory(marker)
}

func (m *Manager) prepareOpenCodeBootstrap(ctx context.Context, marker, dir string) (bool, error) {
	pending, markerErr := inspectOpenCodePendingMarker(marker)
	if markerErr != nil {
		return false, markerErr
	}
	if pending {
		return false, nil
	}
	exportArgs, exportErr := m.cfg.OpenCodeRuntime.CredentialCommand("export")
	if exportErr == nil {
		exportArgs, exportErr = m.prepareSessionArgs(exportArgs)
	}
	if exportErr != nil {
		return false, exportErr
	}
	state, inspectErr := m.inspectOpenCodeCredentials(ctx, exportArgs, dir)
	if inspectErr != nil {
		return false, inspectErr
	}
	if state == opencode.CredentialStorePresent {
		return true, nil
	}
	return false, persistOpenCodePendingMarker(marker)
}

func persistOpenCodePendingMarker(marker string) error {
	if err := os.MkdirAll(filepath.Dir(marker), 0700); err != nil {
		return errors.New("could not persist OpenCode bootstrap state")
	}
	file, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("could not persist OpenCode bootstrap state")
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil || closeErr != nil {
		return errors.New("could not persist OpenCode bootstrap state")
	}
	return syncOpenCodeMarkerDirectory(marker)
}

func inspectOpenCodePendingMarker(marker string) (bool, error) {
	info, err := os.Lstat(marker)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		return false, errors.New("could not inspect OpenCode bootstrap state")
	}
	return true, nil
}

func syncOpenCodeMarkerDirectory(marker string) error {
	dir, err := os.Open(filepath.Dir(marker))
	if err != nil {
		return errors.New("could not persist OpenCode bootstrap state")
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil || closeErr != nil {
		return errors.New("could not persist OpenCode bootstrap state")
	}
	return nil
}

// Ask the public CLI for the effective store path, including OPENCODE_DB and
// XDG overrides. Kandev uses this path only as the identity for a file lock and
// the adjacent retry marker.
func (m *Manager) openCodeCredentialStorePath(ctx context.Context) (string, error) {
	args, err := m.cfg.OpenCodeRuntime.CredentialStoreCommand()
	if err == nil {
		args, err = m.prepareSessionArgs(args)
	}
	if err != nil {
		return "", err
	}
	data, err := m.runOpenCodeCredentialReadCommand(ctx, args, m.cfg.WorkDir)
	path := strings.TrimSpace(string(data))
	if err != nil || len(data) >= managedCommandOutputLimit || !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n\x00") {
		return "", errors.New("could not resolve durable OpenCode credential store")
	}
	return path, nil
}

func (m *Manager) runOpenCodeCredentialReadCommand(ctx context.Context, args []string, dir string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("OpenCode credential command is empty")
	}
	spec := tools.CommandSpec{Path: args[0], Args: args[1:], Dir: dir}
	stdout, _, err := runOpenCodeCredentialReadCommandWithRetry(
		ctx, m.cfg.OpenCodeRuntime, spec,
		func(ctx context.Context, spec tools.CommandSpec) ([]byte, []byte, error) {
			return m.runManagedCommandInput(ctx, spec, nil)
		},
	)
	return stdout, err
}

type openCodeCredentialCommandRunner func(context.Context, tools.CommandSpec) ([]byte, []byte, error)

func runOpenCodeCredentialReadCommandWithRetry(
	ctx context.Context,
	runtime opencode.Runtime,
	spec tools.CommandSpec,
	run openCodeCredentialCommandRunner,
) ([]byte, []byte, error) {
	stdout, stderr, err := run(ctx, spec)
	if err == nil || ctx.Err() != nil {
		return stdout, stderr, err
	}
	retrySpec, ok := openCodeCredentialOnlineRetrySpec(runtime, spec, stderr)
	if !ok {
		return stdout, stderr, err
	}
	return run(ctx, retrySpec)
}

func openCodeCredentialOnlineRetrySpec(
	runtime opencode.Runtime,
	spec tools.CommandSpec,
	stderr []byte,
) (tools.CommandSpec, bool) {
	diagnostic := string(stderr)
	packageSpec := opencode.PackageV2 + "@" + runtime.Version
	summary := npmresolution.AnalyzeManagedStartupDiagnostics(diagnostic)
	if !summary.Complete || summary.UnclassifiedCode || len(summary.Codes) != 1 {
		return tools.CommandSpec{}, false
	}
	switch summary.Codes[0] {
	case "ETARGET":
		if !npmresolution.MatchesExactPackage(diagnostic, packageSpec) ||
			npmresolution.MatchesReleaseAgePolicy(diagnostic, packageSpec) {
			return tools.CommandSpec{}, false
		}
	default:
		if !npmresolution.IsTransientManagedStartupCode(summary.Codes[0]) {
			return tools.CommandSpec{}, false
		}
	}

	args := append([]string(nil), spec.Args...)
	preferenceIndex := -1
	for i, arg := range args {
		if arg != openCodeCredentialPreferOfflineArg {
			continue
		}
		if preferenceIndex >= 0 {
			return tools.CommandSpec{}, false
		}
		preferenceIndex = i
	}
	if preferenceIndex < 0 {
		return tools.CommandSpec{}, false
	}
	args[preferenceIndex] = openCodeCredentialPreferOnlineArg
	spec.Args = args
	return spec, true
}

func openCodePendingMarker(storePath string) string {
	return storePath + ".kandev-bootstrap-v1.pending"
}
