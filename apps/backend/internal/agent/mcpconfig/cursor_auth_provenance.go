package mcpconfig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const (
	cursorMCPAuthProvenanceFilename = "kandev-mcp-auth-unified.json.provenance"
	cursorMCPAuthProvenanceVersion  = 1
	cursorMCPAuthProvenancePending  = "pending"
	cursorMCPAuthProvenanceReady    = "committed"
)

var errCursorMCPAuthMasterChanged = errors.New("cursor MCP auth master changed during publication")

type cursorMCPAuthProvenance struct {
	Version int                                     `json:"version"`
	State   string                                  `json:"state"`
	Servers map[string]cursorMCPAuthProvenanceEntry `json:"servers"`
}

type cursorMCPAuthProvenanceEntry struct {
	SourceID             string `json:"source_id"`
	SourceFingerprint    string `json:"source_fingerprint"`
	ProjectedFingerprint string `json:"projected_fingerprint"`
	NativeUpdated        bool   `json:"native_updated,omitempty"`
}

func publishCursorMCPAuth(cursorHome string, snapshot cursorMCPAuthSnapshot) error {
	return publishCursorMCPAuthWithHook(cursorHome, snapshot, nil)
}

func publishCursorMCPAuthWithHook(cursorHome string, snapshot cursorMCPAuthSnapshot, beforeMasterCompare func()) error {
	masterPath, err := cursorMasterPath(cursorHome)
	if err != nil {
		return err
	}
	provenancePath := masterPath + ".provenance"
	state, err := readCursorMCPAuthPublishState(masterPath, provenancePath)
	if err != nil {
		return err
	}
	servers, provenance := projectCursorMCPAuthSnapshot(snapshot, state)
	return commitCursorMCPAuthPublication(masterPath, provenancePath, servers, provenance, state, beforeMasterCompare)
}

type cursorMCPAuthPublishState struct {
	previous          cursorMCPAuthProvenance
	previousValid     bool
	master            map[string]json.RawMessage
	masterPresent     bool
	masterFingerprint string
}

func readCursorMCPAuthPublishState(masterPath, provenancePath string) (cursorMCPAuthPublishState, error) {
	if err := ensureReplaceableMaster(masterPath); err != nil {
		return cursorMCPAuthPublishState{}, err
	}
	if err := ensureReplaceableProvenance(provenancePath); err != nil {
		return cursorMCPAuthPublishState{}, err
	}
	previous, previousValid, err := readCursorMCPAuthProvenance(provenancePath)
	if err != nil {
		return cursorMCPAuthPublishState{}, err
	}
	master, masterPresent, masterFingerprint, err := readCursorMCPAuthMaster(masterPath)
	if err != nil {
		return cursorMCPAuthPublishState{}, err
	}
	if !masterPresent {
		master = map[string]json.RawMessage{}
	}
	return cursorMCPAuthPublishState{
		previous: previous, previousValid: previousValid, master: master,
		masterPresent: masterPresent, masterFingerprint: masterFingerprint,
	}, nil
}

func projectCursorMCPAuthSnapshot(snapshot cursorMCPAuthSnapshot, state cursorMCPAuthPublishState) (map[string]json.RawMessage, cursorMCPAuthProvenance) {
	servers := cloneCursorMCPAuthServers(snapshot.servers)
	provenance := cursorMCPAuthProvenance{
		Version: cursorMCPAuthProvenanceVersion,
		State:   cursorMCPAuthProvenanceReady,
		Servers: make(map[string]cursorMCPAuthProvenanceEntry, len(snapshot.provenance)),
	}
	for name, entry := range snapshot.provenance {
		if native, ok := preservedNativeCursorMCPAuth(name, entry, state); ok {
			servers[name] = native
			entry.NativeUpdated = true
		}
		entry.ProjectedFingerprint = cursorAuthFingerprint(servers[name])
		provenance.Servers[name] = entry
	}
	return servers, provenance
}

func preservedNativeCursorMCPAuth(name string, entry cursorMCPAuthProvenanceEntry, state cursorMCPAuthPublishState) (json.RawMessage, bool) {
	if !state.previousValid {
		return nil, false
	}
	previous, exists := state.previous.Servers[name]
	current, currentExists := state.master[name]
	if !exists || !currentExists || previous.SourceID != entry.SourceID || previous.SourceFingerprint != entry.SourceFingerprint {
		return nil, false
	}
	currentFingerprint := cursorAuthFingerprint(current)
	unchangedNative := previous.NativeUpdated && currentFingerprint == previous.ProjectedFingerprint
	changedNative := currentFingerprint != previous.ProjectedFingerprint && cursorMCPAuthHasCredential(current)
	if unchangedNative || changedNative {
		return append(json.RawMessage(nil), current...), true
	}
	return nil, false
}

func commitCursorMCPAuthPublication(masterPath, provenancePath string, servers map[string]json.RawMessage, provenance cursorMCPAuthProvenance, state cursorMCPAuthPublishState, beforeMasterCompare func()) error {
	data, err := marshalCursorMCPAuth(servers)
	if err != nil {
		return err
	}
	provenance.State = cursorMCPAuthProvenancePending
	if err := writeCursorMCPAuthProvenance(provenancePath, provenance); err != nil {
		return err
	}
	if err := writeCursorMCPAuthMasterIfUnchanged(masterPath, data, state.masterPresent, state.masterFingerprint, beforeMasterCompare); err != nil {
		if restoreErr := restoreCursorMCPAuthProvenance(provenancePath, state.previous, state.previousValid); restoreErr != nil {
			return errors.Join(err, restoreErr)
		}
		return err
	}
	provenance.State = cursorMCPAuthProvenanceReady
	return writeCursorMCPAuthProvenance(provenancePath, provenance)
}

func restoreCursorMCPAuthProvenance(path string, previous cursorMCPAuthProvenance, previousValid bool) error {
	if previousValid {
		return writeCursorMCPAuthProvenance(path, previous)
	}
	return removeIfPresent(path)
}

// clearCursorMCPAuthSnapshot removes the bridge-owned shared snapshot when
// no eligible sources remain. Existing project symlinks then fail closed.
func clearCursorMCPAuthSnapshot(cursorHome string) error {
	masterPath, err := cursorMasterPath(cursorHome)
	if err != nil {
		return err
	}
	provenancePath := masterPath + ".provenance"
	if err := ensureReplaceableMaster(masterPath); err != nil {
		return err
	}
	if err := removeIfPresent(masterPath); err != nil {
		return err
	}
	if err := ensureReplaceableProvenance(provenancePath); err != nil {
		return err
	}
	return removeIfPresent(provenancePath)
}

func readCursorMCPAuthProvenance(path string) (cursorMCPAuthProvenance, bool, error) {
	data, present, err := readRegularCursorAuthFile(path)
	if err != nil || !present {
		return cursorMCPAuthProvenance{}, false, err
	}
	var provenance cursorMCPAuthProvenance
	if err := json.Unmarshal(data, &provenance); err != nil {
		return cursorMCPAuthProvenance{}, false, nil
	}
	if !validCursorMCPAuthProvenance(provenance) {
		return cursorMCPAuthProvenance{}, false, nil
	}
	return provenance, true, nil
}

func validCursorMCPAuthProvenance(provenance cursorMCPAuthProvenance) bool {
	if provenance.Version != cursorMCPAuthProvenanceVersion || provenance.State != cursorMCPAuthProvenanceReady || provenance.Servers == nil {
		return false
	}
	for _, entry := range provenance.Servers {
		if entry.SourceID == "" || !validCursorAuthFingerprint(entry.SourceFingerprint) || !validCursorAuthFingerprint(entry.ProjectedFingerprint) {
			return false
		}
	}
	return true
}

func validCursorAuthFingerprint(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func cursorAuthFingerprint(value []byte) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		compact.Write(value)
	}
	digest := sha256.Sum256(compact.Bytes())
	return hex.EncodeToString(digest[:])
}

func marshalCursorMCPAuth(servers map[string]json.RawMessage) ([]byte, error) {
	if servers == nil {
		servers = map[string]json.RawMessage{}
	}
	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func cloneCursorMCPAuthServers(servers map[string]json.RawMessage) map[string]json.RawMessage {
	cloned := make(map[string]json.RawMessage, len(servers))
	for name, server := range servers {
		cloned[name] = append(json.RawMessage(nil), server...)
	}
	return cloned
}

func readCursorMCPAuthMaster(path string) (map[string]json.RawMessage, bool, string, error) {
	data, present, err := readRegularCursorAuthFile(path)
	if err != nil || !present {
		return nil, present, "", err
	}
	fingerprint := cursorAuthFingerprint(data)
	servers, valid := decodeCursorMCPAuth(data)
	if !valid {
		return nil, true, fingerprint, nil
	}
	return servers, true, fingerprint, nil
}

func readRegularCursorAuthFile(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, errors.New("cursor MCP auth file is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, false, errors.New("cursor MCP auth file changed while reading")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func ensureReplaceableProvenance(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("cursor MCP auth provenance path is not a regular file")
	}
	return nil
}

func writeCursorMCPAuthMasterIfUnchanged(path string, data []byte, expectedPresent bool, expectedFingerprint string, beforeCompare func()) error {
	return writeCursorAuthFileAtomicallyWithCheck(path, ".kandev-mcp-auth-", data, ensureReplaceableMaster, func() error {
		if beforeCompare != nil {
			beforeCompare()
		}
		currentData, currentPresent, err := readRegularCursorAuthFile(path)
		if err != nil {
			return err
		}
		if currentPresent != expectedPresent || currentPresent && cursorAuthFingerprint(currentData) != expectedFingerprint {
			return errCursorMCPAuthMasterChanged
		}
		return nil
	})
}

func writeCursorMCPAuthProvenance(path string, provenance cursorMCPAuthProvenance) error {
	data, err := json.Marshal(provenance)
	if err != nil {
		return err
	}
	return writeCursorAuthFileAtomically(path, ".kandev-mcp-auth-provenance-", append(data, '\n'), ensureReplaceableProvenance)
}

func writeCursorAuthFileAtomically(path, prefix string, data []byte, ensureReplaceable func(string) error) error {
	return writeCursorAuthFileAtomicallyWithCheck(path, prefix, data, ensureReplaceable, nil)
}

func writeCursorAuthFileAtomicallyWithCheck(path, prefix string, data []byte, ensureReplaceable func(string) error, beforeRename func() error) error {
	if err := ensureReplaceable(path); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), prefix)
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		return err
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := ensureReplaceable(path); err != nil {
		return err
	}
	if beforeRename != nil {
		if err := beforeRename(); err != nil {
			return err
		}
	}
	return os.Rename(tempPath, path)
}

func removeIfPresent(path string) error {
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else {
		return err
	}
}
