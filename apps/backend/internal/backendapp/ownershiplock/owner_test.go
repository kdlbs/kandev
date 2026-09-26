package ownershiplock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteOwnerRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })

	if err := writeOwner(file); err != nil {
		t.Fatalf("writeOwner: %v", err)
	}
	owner := readOwner(path)
	if owner == nil {
		t.Fatal("readOwner returned nil")
		return
	}
	if owner.PID != int64(os.Getpid()) {
		t.Fatalf("owner PID = %d, want %d", owner.PID, os.Getpid())
	}
	if owner.Executable == "" {
		t.Fatal("owner executable is empty")
	}
	if owner.StartedAt == "" {
		t.Fatal("owner start time is empty")
	}
}

func TestReadOwnerWhileLockIsHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	file, err := openLockFile(path)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	t.Cleanup(func() {
		_ = unlockFile(file)
		_ = file.Close()
	})
	if err := lockFile(file); err != nil {
		t.Fatalf("lockFile: %v", err)
	}
	if err := writeOwner(file); err != nil {
		t.Fatalf("writeOwner: %v", err)
	}

	owner := readOwner(path)
	if owner == nil {
		t.Fatal("readOwner returned nil while lock was held")
		return
	}
	if owner.PID != int64(os.Getpid()) {
		t.Fatalf("owner PID = %d, want %d", owner.PID, os.Getpid())
	}
}

func TestWriteOwnerTruncatesPreviousRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if _, err := file.WriteString(`{"pid":123456789,"executable":"` + string(make([]byte, 4096)) + `"}`); err != nil {
		t.Fatalf("write previous record: %v", err)
	}

	if err := writeOwner(file); err != nil {
		t.Fatalf("writeOwner: %v", err)
	}
	owner := readOwner(path)
	if owner == nil {
		t.Fatal("readOwner returned nil after truncating previous metadata")
		return
	}
	if owner.PID != int64(os.Getpid()) {
		t.Fatalf("owner PID = %d, want %d", owner.PID, os.Getpid())
	}
}

func TestReadOwnerRejectsInvalidMetadata(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "missing", data: ""},
		{name: "invalid json", data: "not json"},
		{name: "missing pid", data: `{"executable":"kandev"}`},
		{name: "non-positive pid", data: `{"pid":0}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "lock")
			if test.name != "missing" {
				if err := os.WriteFile(path, ownerMetadataFixture(test.data), 0o600); err != nil {
					t.Fatalf("write metadata: %v", err)
				}
			}
			if owner := readOwner(path); owner != nil {
				t.Fatalf("readOwner = %+v, want nil", owner)
			}
		})
	}
}

func ownerMetadataFixture(data string) []byte {
	return ownerMetadataBytes([]byte(data))
}

func ownerMetadataBytes(data []byte) []byte {
	return append([]byte{0}, data...)
}
