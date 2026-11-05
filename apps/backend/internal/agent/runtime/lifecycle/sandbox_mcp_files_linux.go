//go:build linux

package lifecycle

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"golang.org/x/sys/unix"
)

const sandboxMCPConfigLimit = 1 << 20

// All file operations stay below the admitted root, including during symlink replacement.
func (m *Manager) materializeSandboxMCPFileLocked(execution *AgentExecution, file mcpconfig.PassthroughConfigFile) (bool, error) {
	rootPath, relative := sandboxMCPFileRoot(execution, file.Path)
	if rootPath == "" || len(file.Content) > sandboxMCPConfigLimit {
		return false, contract.ErrInvalidPolicy
	}
	canonical, err := filepath.EvalSymlinks(rootPath)
	if err != nil || canonical != rootPath {
		return false, contract.ErrInvalidPolicy
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return false, contract.ErrInvalidPolicy
	}
	defer func() { _ = root.Close() }()
	if err = root.MkdirAll(filepath.Dir(relative), 0700); err != nil {
		return false, contract.ErrInvalidPolicy
	}
	directory, err := root.OpenRoot(filepath.Dir(relative))
	if err != nil {
		return false, contract.ErrInvalidPolicy
	}
	defer func() { _ = directory.Close() }()
	name := filepath.Base(relative)
	content, owned, err := m.sandboxMCPFileContent(directory, name, file)
	if err != nil {
		return false, err
	}
	if err = writeSandboxMCPFile(directory, name, content); err != nil {
		return false, err
	}
	if owned {
		m.recordPassthroughMCPClaimLocked(execution, passthroughMCPFileClaim{Path: file.Path, Fingerprint: passthroughMCPFingerprint(content), Owned: true})
	} else {
		m.forgetPassthroughMCPClaimLocked(execution, file.Path)
	}
	return owned, nil
}

func (m *Manager) sandboxMCPFileContent(directory *os.Root, name string, file mcpconfig.PassthroughConfigFile) ([]byte, bool, error) {
	existing, err := readSandboxMCPFile(directory, name)
	if errors.Is(err, os.ErrNotExist) {
		return file.Content, true, nil
	}
	if err != nil {
		return nil, false, contract.ErrInvalidPolicy
	}
	if file.MergeKey == "" {
		return file.Content, true, nil
	}
	merged, err := mcpconfig.MergeJSONUnderKey(existing, file.Content, file.MergeKey)
	if err != nil || len(merged) > sandboxMCPConfigLimit {
		return nil, false, contract.ErrInvalidPolicy
	}
	owned := m.knownPassthroughMCPGenerationLocked(file.Path, passthroughMCPFingerprint(existing))
	if owned {
		m.adoptPassthroughMCPGenerationLocked(file.Path, passthroughMCPFingerprint(merged))
	}
	return merged, owned, nil
}

func readSandboxMCPFile(directory *os.Root, name string) ([]byte, error) {
	file, err := directory.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > sandboxMCPConfigLimit {
		return nil, contract.ErrInvalidPolicy
	}
	content, err := io.ReadAll(io.LimitReader(file, sandboxMCPConfigLimit+1))
	if err != nil || len(content) > sandboxMCPConfigLimit {
		return nil, contract.ErrInvalidPolicy
	}
	return content, nil
}

func writeSandboxMCPFile(directory *os.Root, name string, content []byte) error {
	temporary := ".kandev-mcp-" + uuid.NewString()
	file, err := directory.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return contract.ErrInvalidPolicy
	}
	defer func() { _ = directory.Remove(temporary) }()
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return contract.ErrInvalidPolicy
	}
	if err = directory.Rename(temporary, name); err != nil {
		return contract.ErrInvalidPolicy
	}
	return nil
}

func (m *Manager) cleanupSandboxMCPClaimLocked(execution *AgentExecution, claim passthroughMCPFileClaim) {
	rootPath, relative := sandboxMCPFileRoot(execution, claim.Path)
	if rootPath == "" {
		return
	}
	canonical, err := filepath.EvalSymlinks(rootPath)
	if err != nil || canonical != rootPath {
		return
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return
	}
	defer func() { _ = root.Close() }()
	directory, err := root.OpenRoot(filepath.Dir(relative))
	if err != nil {
		return
	}
	defer func() { _ = directory.Close() }()
	name := filepath.Base(relative)
	content, err := readSandboxMCPFile(directory, name)
	if err != nil || passthroughMCPFingerprint(content) != claim.Fingerprint {
		return
	}
	if m.hasOtherPassthroughMCPClaimLocked(claim.Path, execution.ID, claim.Fingerprint) {
		return
	}
	_ = directory.Remove(name)
}
