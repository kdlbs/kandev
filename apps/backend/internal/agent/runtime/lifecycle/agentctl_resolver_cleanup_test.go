package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resolverCacheLeaseIsActive(cacheRoot, helperPath string) (bool, error) {
	activeDir := filepath.Join(cacheRoot, remoteHelperCacheActiveDir)
	entries, err := os.ReadDir(activeDir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read helper cache leases: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		marker := filepath.Join(activeDir, entry.Name())
		data, err := os.ReadFile(marker)
		if err != nil {
			return false, fmt.Errorf("read helper cache lease %s: %w", entry.Name(), err)
		}
		var lease remoteHelperCacheLeaseRecord
		if err := json.Unmarshal(data, &lease); err != nil {
			return false, fmt.Errorf("decode helper cache lease %s: %w", entry.Name(), err)
		}
		if filepath.Clean(lease.Path) == filepath.Clean(helperPath) {
			return true, nil
		}
	}
	return false, nil
}

func waitForResolverCacheLeaseRelease(t *testing.T, cacheRoot, helperPath string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		active, err := resolverCacheLeaseIsActive(cacheRoot, helperPath)
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			t.Errorf("inspect helper cache lease for %s during cleanup: %v", helperPath, err)
			return
		}
		if !active {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("helper cache lease for %s was not released before test cleanup", helperPath)
			return
		}
		time.Sleep(time.Millisecond)
	}
}
