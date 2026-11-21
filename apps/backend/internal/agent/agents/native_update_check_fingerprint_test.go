package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestListDirFilesLimitCountsEveryEntry(t *testing.T) {
	dirPath := t.TempDir()
	for i := range nativeUpdateCheckDirEntryLimit + 1 {
		name := filepath.Join(dirPath, fmt.Sprintf("entry-%03d", i))
		if i%2 == 0 {
			if err := os.Mkdir(name, 0o700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(name, []byte("ordinary file"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	names, overLimit := listDirFiles(NativeUpdateCheckDir{
		Path:  dirPath,
		Match: func(string) bool { return false },
	})
	if !overLimit {
		t.Fatalf("overLimit = false with %d non-matching entries", nativeUpdateCheckDirEntryLimit+1)
	}
	if len(names) != 0 {
		t.Errorf("names = %v, want no partial enumeration after the limit", names)
	}
}
