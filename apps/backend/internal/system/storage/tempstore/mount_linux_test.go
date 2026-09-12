//go:build linux

package tempstore

import (
	"fmt"
	"strings"
	"testing"
)

func TestMountReaderRefreshesForNestedMountsBetweenScans(t *testing.T) {
	tables := [][]byte{
		[]byte(mountInfoLine("100", "1", "0:1", "/", "/tmp")),
		[]byte(mountInfoLine("100", "1", "0:1", "/", "/tmp") +
			mountInfoLine("200", "100", "0:2", "/", "/tmp/nested")),
	}
	reader := mountReader{readMountInfo: sequenceMountTables(t, tables)}

	parentIdentity, err := reader.Identity("/tmp/work")
	if err != nil {
		t.Fatalf("parent identity: %v", err)
	}
	nestedIdentity, err := reader.Identity("/tmp/nested/file")
	if err != nil {
		t.Fatalf("nested identity: %v", err)
	}
	if parentIdentity == nestedIdentity {
		t.Fatalf("nested mount identity = %q, want refreshed identity distinct from parent %q", nestedIdentity, parentIdentity)
	}
	if !strings.HasPrefix(nestedIdentity, "200\x00") {
		t.Fatalf("nested mount identity = %q, want mount ID 200", nestedIdentity)
	}
}

func TestMountReaderDetectsReplacementAtExistingMountpoint(t *testing.T) {
	tables := [][]byte{
		[]byte(mountInfoLine("100", "1", "0:1", "/", "/tmp")),
		[]byte(mountInfoLine("300", "1", "0:3", "/", "/tmp")),
	}
	reader := mountReader{readMountInfo: sequenceMountTables(t, tables)}

	oldIdentity, err := reader.Identity("/tmp/work")
	if err != nil {
		t.Fatalf("old identity: %v", err)
	}
	newIdentity, err := reader.Identity("/tmp/work")
	if err != nil {
		t.Fatalf("new identity: %v", err)
	}
	if oldIdentity == newIdentity {
		t.Fatalf("replacement identity remained %q", oldIdentity)
	}
	if !strings.HasPrefix(newIdentity, "300\x00") {
		t.Fatalf("replacement identity = %q, want mount ID 300", newIdentity)
	}
}

func TestMountReaderSnapshotReusesOneLoadedTable(t *testing.T) {
	reads := 0
	reader := mountReader{readMountInfo: func() ([]byte, error) {
		reads++
		return []byte(mountInfoLine("100", "1", "0:1", "/", "/tmp")), nil
	}}

	snapshot, err := reader.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	for _, path := range []string{"/tmp/one", "/tmp/two"} {
		if _, err := snapshot.Identity(path); err != nil {
			t.Fatalf("snapshot identity %s: %v", path, err)
		}
	}
	if reads != 1 {
		t.Fatalf("mount table reads = %d, want one snapshot read", reads)
	}
}

func sequenceMountTables(t *testing.T, tables [][]byte) func() ([]byte, error) {
	t.Helper()
	index := 0
	return func() ([]byte, error) {
		if index >= len(tables) {
			return nil, fmt.Errorf("mount table sequence exhausted at call %d", index)
		}
		table := tables[index]
		index++
		return table, nil
	}
}

func mountInfoLine(id, parent, device, root, mountpoint string) string {
	return fmt.Sprintf("%s %s %s %s %s rw - testfs tmpfs rw\n", id, parent, device, root, mountpoint)
}
