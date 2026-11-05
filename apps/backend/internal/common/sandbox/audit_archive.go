package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const maximumAuditArchiveBytes = 32 << 20
const maximumAuditArchiveSources = 32

// AuditSourceReceipt identifies frozen bytes. Name stays inside the runtime boundary.
type AuditSourceReceipt struct {
	ID       string
	Schema   string
	Name     string
	Digest   string
	Identity string
	Size     int64
}

type AuditArchive struct {
	Digest  string
	Sources []AuditSourceReceipt
	root    string
	files   []*os.File
}

// FreezeAuditSources copies declared private files before any provider RPC.
// Existing sources must retain their birth identity and every previously read byte.
func FreezeAuditSources(ctx context.Context, stateDir, scratchDir string, sources []RuntimeSource, previous []AuditSourceReceipt) (archive *AuditArchive, err error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !validRuntimeDirectory(stateDir) || !validRuntimeDirectory(scratchDir) || len(sources) > 8 || len(previous) > maximumAuditArchiveSources {
		return nil, ErrInvalidPolicy
	}
	if !validAuditDeclarations(sources) {
		return nil, ErrInvalidPolicy
	}
	directory, err := os.MkdirTemp(scratchDir, "audit-read-")
	if err != nil {
		return nil, err
	}
	result := &AuditArchive{root: directory}
	defer func() {
		if err != nil {
			err = errors.Join(err, result.Close())
		}
	}()
	for _, source := range sources {
		if err = result.captureSource(ctx, stateDir, source, previous); err != nil {
			return nil, err
		}
	}
	if !result.retainsPriorSources(previous) {
		return nil, ErrInvalidPolicy
	}
	result.Digest, err = AuditArtifactSetDigest(result.Sources)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (a *AuditArchive) captureSource(ctx context.Context, stateDir string, source RuntimeSource, previous []AuditSourceReceipt) error {
	directory := filepath.Join(stateDir, filepath.FromSlash(source.Directory))
	canonical, err := filepath.EvalSymlinks(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || canonical != directory {
		return ErrInvalidPolicy
	}
	root, err := os.OpenRoot(stateDir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	count := 0
	reader := runtimeSourceReader{root: root, source: source}
	reader.consume = func(ctx context.Context, name string) error {
		count++
		if count > int(source.MaxFiles) || len(a.Sources) >= maximumAuditArchiveSources {
			return ErrInvalidPolicy
		}
		return a.freezeFile(ctx, root, name, source, previous)
	}
	return reader.walk(ctx, source.Directory, 1)
}

// AuditArtifactSetDigest binds order, declaration, name, digest and byte count.
func AuditArtifactSetDigest(sources []AuditSourceReceipt) (string, error) {
	type binding struct {
		ID     string `json:"id"`
		Schema string `json:"schema"`
		Name   string `json:"name"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	}
	bindings := make([]binding, 0, len(sources))
	for _, source := range sources {
		bindings = append(bindings, binding{source.ID, source.Schema, source.Name, source.Digest, source.Size})
	}
	raw, err := json.Marshal(bindings)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (a *AuditArchive) ReadChunk(ctx context.Context, index int, offset int64, limit int) ([]byte, bool, error) {
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if index < 0 || index >= len(a.files) || offset < 0 || offset > a.Sources[index].Size || limit < 1 || limit > 256<<10 {
		return nil, false, ErrInvalidPolicy
	}
	size := min(int64(limit), a.Sources[index].Size-offset)
	content := make([]byte, int(size))
	count, err := a.files[index].ReadAt(content, offset)
	if err != nil || count != len(content) {
		return nil, false, ErrInvalidPolicy
	}
	return content, offset+size == a.Sources[index].Size, nil
}

func (a *AuditArchive) Close() error {
	var err error
	for _, file := range a.files {
		err = errors.Join(err, file.Close())
	}
	a.files = nil
	if a.root != "" {
		removeErr := os.RemoveAll(a.root)
		err = errors.Join(err, removeErr)
		if removeErr == nil {
			a.root = ""
		}
	}
	return err
}

func (a *AuditArchive) retainsPriorSources(previous []AuditSourceReceipt) bool {
	present := make(map[string]bool, len(a.Sources))
	for _, source := range a.Sources {
		present[source.ID] = true
	}
	for _, source := range previous {
		if !present[source.ID] {
			return false
		}
	}
	return true
}

func validAuditDeclarations(sources []RuntimeSource) bool {
	for _, source := range sources {
		if (source.Kind != RuntimeSourceAudit && source.Kind != RuntimeSourceAuditFinal) || source.ValidateForProtocol(2) != nil {
			return false
		}
	}
	return true
}
