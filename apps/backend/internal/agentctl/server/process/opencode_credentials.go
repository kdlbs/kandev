package process

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	tools "github.com/kandev/kandev/internal/tools/installer"
)

// OpenCodeCredentialStatus returns only readiness, never credential data.
func (m *Manager) OpenCodeCredentialStatus(parent context.Context) (string, error) {
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
	defer func() { m.startMu.Lock(); m.credentialImport = false; m.startMu.Unlock() }()
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	marker, err := m.openCodePendingMarker(ctx)
	if err != nil {
		return "", err
	}
	if pending, err := inspectOpenCodePendingMarker(marker); err != nil {
		return "", err
	} else if pending {
		return "pending", nil
	}
	return m.inspectOpenCodeCredentials(ctx, args, dir)
}

func (m *Manager) inspectOpenCodeCredentials(ctx context.Context, args []string, dir string) (string, error) {
	data, _, err := m.runManagedCommandInput(ctx, tools.CommandSpec{Path: args[0], Args: args[1:], Dir: dir}, nil)
	var entries []struct {
		ID string `json:"id"`
	}
	// The generic command runner caps output. Reject a saturated capture rather
	// than mistaking a truncated JSON prefix for a successful empty store.
	if err != nil || len(data) >= managedCommandOutputLimit || json.Unmarshal(data, &entries) != nil || entries == nil {
		return "", errors.New("could not inspect OpenCode credentials")
	}
	if len(entries) == 0 {
		return "empty", nil
	}
	for _, entry := range entries {
		if entry.ID == "" {
			return "", errors.New("could not inspect OpenCode credentials")
		}
	}
	return "present", nil
}

// ImportOpenCodeCredentials provisions the configured session through the CLI's
// merge contract. Credential values are carried only through stdin.
func (m *Manager) ImportOpenCodeCredentials(ctx context.Context, data []byte) error {
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
		m.startMu.Lock()
		m.credentialImport = false
		m.startMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	marker, err := m.openCodePendingMarker(ctx)
	if err != nil {
		return err
	}
	ready, err := m.prepareOpenCodeBootstrap(ctx, marker, dir)
	if err != nil || ready {
		return err
	}
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
	if state == "present" {
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
// XDG overrides. The adjacent marker contains no credentials and survives restarts.
func (m *Manager) openCodePendingMarker(ctx context.Context) (string, error) {
	args, err := m.cfg.OpenCodeRuntime.CredentialStoreCommand()
	if err == nil {
		args, err = m.prepareSessionArgs(args)
	}
	if err != nil {
		return "", err
	}
	data, _, err := m.runManagedCommandInput(ctx, tools.CommandSpec{Path: args[0], Args: args[1:], Dir: m.cfg.WorkDir}, nil)
	path := strings.TrimSpace(string(data))
	if err != nil || len(data) >= managedCommandOutputLimit || !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n\x00") {
		return "", errors.New("could not resolve durable OpenCode credential store")
	}
	return path + ".kandev-bootstrap-v1.pending", nil
}
