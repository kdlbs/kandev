package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// failingReader yields some bytes and then fails, to prove a mid-stream error
// leaves no file and no temporary artifact behind.
type failingReader struct {
	prefix []byte
	off    int
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.off < len(r.prefix) {
		n := copy(p, r.prefix[r.off:])
		r.off += n
		return n, nil
	}
	return 0, errors.New("stream broke")
}

func TestWriteFileStreamContainment(t *testing.T) {
	cases := []struct {
		name         string
		path         string
		absolutePath bool
	}{
		{name: "parent traversal", path: "../escaped.txt"},
		{name: "deep traversal", path: "../../etc/passwd"},
		{name: "absolute path", absolutePath: true},
		{name: "interior traversal segment", path: "nested/../../escaped.txt"},
		{name: "traversal through existing dir", path: "sub/../../escaped.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, wt := setupTestDir(t)
			if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			parent := filepath.Dir(dir)
			requestPath := tc.path
			outsidePath := filepath.Join(parent, "escaped.txt")
			if tc.absolutePath {
				// Use a platform-native absolute path. Unix accepts /etc/passwd,
				// but Windows treats that spelling as a relative path.
				requestPath = filepath.Join(parent, "escaped-absolute.txt")
				outsidePath = requestPath
			}

			_, _, err := wt.WriteFileStream(requestPath, UploadResolutionNone, strings.NewReader("payload"))
			if err == nil {
				t.Fatalf("expected containment rejection for %q, got nil", requestPath)
			}
			if _, statErr := os.Stat(outsidePath); statErr == nil {
				t.Fatalf("wrote outside the workspace for %q", requestPath)
			}
		})
	}
}

func TestWriteFileStreamSymlinkedDirRejected(t *testing.T) {
	dir, wt := setupTestDir(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, _, err := wt.WriteFileStream("link/escaped.txt", UploadResolutionNone, strings.NewReader("payload"))
	if err == nil {
		t.Fatal("expected rejection when writing through a symlinked directory")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "escaped.txt")); statErr == nil {
		t.Fatal("wrote through a symlink out of the workspace")
	}
}

func TestWriteFileStreamWritesNestedFile(t *testing.T) {
	dir, wt := setupTestDir(t)

	written, n, err := wt.WriteFileStream("nested/deep/file.txt", UploadResolutionNone, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("WriteFileStream: %v", err)
	}
	if written != "nested/deep/file.txt" {
		t.Errorf("written path = %q, want nested/deep/file.txt", written)
	}
	if n != 5 {
		t.Errorf("bytes = %d, want 5", n)
	}
	got, err := os.ReadFile(filepath.Join(dir, "nested", "deep", "file.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want hello", got)
	}
}

func TestWriteFileStreamResolutions(t *testing.T) {
	t.Run("absent resolution refuses to overwrite", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		writeFile(t, dir, "taken.txt", "original")

		_, _, err := wt.WriteFileStream("taken.txt", UploadResolutionNone, strings.NewReader("incoming"))
		if !errors.Is(err, ErrUploadConflict) {
			t.Fatalf("expected ErrUploadConflict, got %v", err)
		}
		got, _ := os.ReadFile(filepath.Join(dir, "taken.txt"))
		if string(got) != "original" {
			t.Errorf("existing file was modified: %q", got)
		}
	})

	t.Run("replace overwrites", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		writeFile(t, dir, "taken.txt", "original")

		written, _, err := wt.WriteFileStream("taken.txt", UploadResolutionReplace, strings.NewReader("incoming"))
		if err != nil {
			t.Fatalf("WriteFileStream: %v", err)
		}
		if written != "taken.txt" {
			t.Errorf("written path = %q, want taken.txt", written)
		}
		got, _ := os.ReadFile(filepath.Join(dir, "taken.txt"))
		if string(got) != "incoming" {
			t.Errorf("content = %q, want incoming", got)
		}
	})

	t.Run("replace refuses a directory", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		if err := os.Mkdir(filepath.Join(dir, "taken.txt"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		_, _, err := wt.WriteFileStream("taken.txt", UploadResolutionReplace, strings.NewReader("incoming"))
		if err == nil || !strings.Contains(err.Error(), "is a directory") {
			t.Fatalf("expected directory rejection, got %v", err)
		}
	})

	t.Run("keep both renames and preserves the original", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		writeFile(t, dir, "taken.txt", "original")

		written, _, err := wt.WriteFileStream("taken.txt", UploadResolutionKeepBoth, strings.NewReader("incoming"))
		if err != nil {
			t.Fatalf("WriteFileStream: %v", err)
		}
		if written != "taken-1.txt" {
			t.Errorf("written path = %q, want taken-1.txt", written)
		}
		original, _ := os.ReadFile(filepath.Join(dir, "taken.txt"))
		if string(original) != "original" {
			t.Errorf("original was modified: %q", original)
		}
		renamed, err := os.ReadFile(filepath.Join(dir, "taken-1.txt"))
		if err != nil {
			t.Fatalf("renamed file missing: %v", err)
		}
		if string(renamed) != "incoming" {
			t.Errorf("renamed content = %q, want incoming", renamed)
		}
	})

	t.Run("keep both skips names already taken", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		writeFile(t, dir, "taken.txt", "original")
		writeFile(t, dir, "taken-1.txt", "first")

		written, _, err := wt.WriteFileStream("taken.txt", UploadResolutionKeepBoth, strings.NewReader("incoming"))
		if err != nil {
			t.Fatalf("WriteFileStream: %v", err)
		}
		if written != "taken-2.txt" {
			t.Errorf("written path = %q, want taken-2.txt", written)
		}
	})

	t.Run("keep both on a free name uses it unchanged", func(t *testing.T) {
		_, wt := setupTestDir(t)

		written, _, err := wt.WriteFileStream("fresh.txt", UploadResolutionKeepBoth, strings.NewReader("incoming"))
		if err != nil {
			t.Fatalf("WriteFileStream: %v", err)
		}
		if written != "fresh.txt" {
			t.Errorf("written path = %q, want fresh.txt", written)
		}
	})
}

func TestWriteFileStreamFailedStreamLeavesNothing(t *testing.T) {
	dir, wt := setupTestDir(t)

	_, _, err := wt.WriteFileStream("partial.bin", UploadResolutionNone, &failingReader{prefix: []byte("abc")})
	if err == nil {
		t.Fatal("expected an error from the failing reader")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "partial.bin")); statErr == nil {
		t.Error("a partial file was left at the destination")
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("readdir: %v", readErr)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".kandev-upload-") {
			t.Errorf("temporary artifact left behind: %s", e.Name())
		}
	}
}

func TestWriteFileStreamDoesNotClobberExistingOnFailure(t *testing.T) {
	dir, wt := setupTestDir(t)
	writeFile(t, dir, "keep.txt", "original")

	_, _, err := wt.WriteFileStream("keep.txt", UploadResolutionReplace, &failingReader{prefix: []byte("abc")})
	if err == nil {
		t.Fatal("expected an error from the failing reader")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "keep.txt"))
	if string(got) != "original" {
		t.Errorf("existing file was clobbered by a failed replace: %q", got)
	}
}

func TestWriteFileStreamNotifiesChange(t *testing.T) {
	_, wt := setupTestDir(t)
	sub := make(types.WorkspaceStreamSubscriber, 8)
	wt.workspaceSubMu.Lock()
	if wt.workspaceStreamSubscribers == nil {
		wt.workspaceStreamSubscribers = make(map[types.WorkspaceStreamSubscriber]struct{})
	}
	wt.workspaceStreamSubscribers[sub] = struct{}{}
	wt.workspaceSubMu.Unlock()

	if _, _, err := wt.WriteFileStream("noted.txt", UploadResolutionNone, strings.NewReader("x")); err != nil {
		t.Fatalf("WriteFileStream: %v", err)
	}

	select {
	case msg := <-sub:
		if msg.FileChange == nil {
			t.Fatalf("expected a file-change notification, got %+v", msg)
		}
		if msg.FileChange.Path != "noted.txt" {
			t.Errorf("notified path = %q, want noted.txt", msg.FileChange.Path)
		}
	default:
		t.Fatal("expected a workspace change notification")
	}
}

func TestWriteFileStreamConcurrentKeepBothUsesUniqueNames(t *testing.T) {
	dir, wt := setupTestDir(t)
	writeFile(t, dir, "taken.txt", "original")

	const uploads = 8
	type uploadResult struct {
		path string
		err  error
	}
	results := make(chan uploadResult, uploads)
	for i := 0; i < uploads; i++ {
		go func(i int) {
			path, _, err := wt.WriteFileStream(
				"taken.txt",
				UploadResolutionKeepBoth,
				strings.NewReader(fmt.Sprintf("incoming-%d", i)),
			)
			results <- uploadResult{path: path, err: err}
		}(i)
	}
	seen := map[string]bool{}
	for i := 0; i < uploads; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent upload: %v", result.err)
		}
		if seen[result.path] {
			t.Fatalf("duplicate final path %q", result.path)
		}
		seen[result.path] = true
	}
	if len(seen) != uploads {
		t.Fatalf("got %d uploaded paths, want %d: %v", len(seen), uploads, seen)
	}
}

func TestCheckUploadConflicts(t *testing.T) {
	t.Run("reports only existing paths", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		writeFile(t, dir, "there.txt", "x")
		if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeFile(t, dir, filepath.Join("nested", "also.txt"), "x")

		conflicts, err := wt.CheckUploadConflicts([]string{"there.txt", "missing.txt", "nested/also.txt"})
		if err != nil {
			t.Fatalf("CheckUploadConflicts: %v", err)
		}
		got := map[string]bool{}
		for _, c := range conflicts {
			got[c.Path] = true
		}
		if !got["there.txt"] || !got["nested/also.txt"] {
			t.Errorf("missing expected conflicts, got %v", got)
		}
		if got["missing.txt"] {
			t.Error("reported a conflict for a path that does not exist")
		}
	})

	t.Run("marks directories", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		if err := os.MkdirAll(filepath.Join(dir, "adir"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		conflicts, err := wt.CheckUploadConflicts([]string{"adir"})
		if err != nil {
			t.Fatalf("CheckUploadConflicts: %v", err)
		}
		if len(conflicts) != 1 || !conflicts[0].IsDir {
			t.Fatalf("expected one directory conflict, got %+v", conflicts)
		}
	})

	t.Run("containment failure is an error, not a conflict", func(t *testing.T) {
		_, wt := setupTestDir(t)

		if _, err := wt.CheckUploadConflicts([]string{"../escaped.txt"}); err == nil {
			t.Fatal("expected an error for a path outside the workspace")
		}
	})
}

// io.Reader compile-time guard for the helper above.
var _ io.Reader = (*failingReader)(nil)
