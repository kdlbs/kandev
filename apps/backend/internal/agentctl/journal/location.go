package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ownerDirectoryName = "agentctl-journals"

const (
	ownerMarkerName = ".owner"
	lostMarkerName  = ".lost"
)

// StorageCapability is the protocol-facing result of checking retained
// storage. A missing or invalid root is not silently downgraded to legacy
// delivery when the caller requested durable delivery.
type StorageCapability struct {
	Version    uint32 `json:"version"`
	Durable    bool   `json:"durable"`
	Unresolved bool   `json:"unresolved,omitempty"`
	Path       string `json:"path,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// Location identifies the stable owner-scoped journal path. The path is
// independent of a worktree, native harness home, process ID, and binary path.
type Location struct {
	Root    string
	OwnerID string
	Path    string
}

func ResolveLocation(root, ownerID string) (Location, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || !filepath.IsAbs(root) {
		return Location{}, fmt.Errorf("journal root must be an absolute path")
	}
	if !validOwnerID(ownerID) {
		return Location{}, fmt.Errorf("journal owner id is invalid")
	}
	return Location{
		Root:    root,
		OwnerID: ownerID,
		Path:    filepath.Join(root, ownerDirectoryName, ownerID, "delivery.bbolt"),
	}, nil
}

// CheckStorage verifies that the owner root can be retained. It creates only
// the owner directory; journal.Open remains the operation that initializes or
// validates the database format and reports corruption.
func CheckStorage(root, ownerID string) StorageCapability {
	location, err := ResolveLocation(root, ownerID)
	if err != nil {
		return StorageCapability{Version: CurrentVersion, Reason: "storage_not_durable"}
	}
	dir := filepath.Dir(location.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return StorageCapability{Version: CurrentVersion, Path: location.Path, Reason: "storage_unavailable"}
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return StorageCapability{Version: CurrentVersion, Path: location.Path, Reason: "storage_unavailable"}
	}
	if _, err := os.Stat(filepath.Join(dir, lostMarkerName)); err == nil {
		return StorageCapability{Version: CurrentVersion, Path: location.Path, Reason: "journal_lost"}
	}
	if _, err := os.Stat(filepath.Join(dir, ownerMarkerName)); err != nil {
		if !os.IsNotExist(err) {
			return StorageCapability{Version: CurrentVersion, Path: location.Path, Reason: "storage_unavailable"}
		}
		if err := os.WriteFile(filepath.Join(dir, ownerMarkerName), []byte(ownerID+"\n"), 0o600); err != nil {
			return StorageCapability{Version: CurrentVersion, Path: location.Path, Reason: "storage_unavailable"}
		}
	}
	return StorageCapability{Version: CurrentVersion, Durable: true, Path: location.Path}
}

// MarkStorageLost is the explicit destructive cleanup operation. It removes
// the journal but leaves a tombstone so a recreated executor cannot advertise
// continuity for the old owner.
func MarkStorageLost(root, ownerID string) error {
	location, err := ResolveLocation(root, ownerID)
	if err != nil {
		return err
	}
	dir := filepath.Dir(location.Path)
	if err := os.Remove(location.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(filepath.Join(dir, lostMarkerName), []byte(ownerID+"\n"), 0o600)
}

func validOwnerID(ownerID string) bool {
	if ownerID == "" || ownerID == "." || ownerID == ".." || strings.TrimSpace(ownerID) != ownerID {
		return false
	}
	return !strings.ContainsAny(ownerID, `/\\`) && !strings.ContainsRune(ownerID, '\x00')
}
