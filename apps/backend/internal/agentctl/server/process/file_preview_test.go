package process

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// @covers AC-PLUGINS-FILE-VIEWERS-002.1
func TestFilePreviewLargeRead(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte{0x93}, 11<<20)
	if err := os.WriteFile(filepath.Join(dir, "model.stl"), data, 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := prepareFilePreview(dir, "model.stl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = preview.Close() }()
	if preview.External {
		t.Fatal("repository file classified as external")
	}
	var out bytes.Buffer
	if err := preview.CopyTo(t.Context(), &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Fatal("file bytes changed")
	}
}

// @covers AC-PLUGINS-FILE-VIEWERS-002.5
func TestFilePreviewReplacementAndCancellation(t *testing.T) {
	for _, change := range []string{"replacement", "cancel"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "model.stl")
			if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			preview, err := prepareFilePreview(dir, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = preview.Close() }()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if change == "cancel" {
				cancel()
			} else {
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			if err := preview.CopyTo(ctx, &out); err == nil {
				t.Fatal("invalid preview was read")
			}
			if out.Len() != 0 {
				t.Fatal("bytes delivered after invalidation")
			}
		})
	}
}

// @covers AC-PLUGINS-FILE-VIEWERS-006.5
func TestFilePreviewExternalClassification(t *testing.T) {
	dir, external := t.TempDir(), filepath.Join(t.TempDir(), "model.stl")
	if err := os.WriteFile(external, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := prepareFilePreview(dir, external)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = preview.Close() }()
	expected, err := filepath.EvalSymlinks(external)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.External || preview.Path != expected {
		t.Fatal("external target not identified")
	}
}

func TestFilePreviewRejectsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	large := filepath.Join(dir, "large.stl")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64<<20 + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	for _, path := range []string{"../escape", dir, large, ""} {
		if p, err := prepareFilePreview(dir, path); err == nil {
			_ = p.Close()
			t.Errorf("accepted %q", path)
		}
	}
	if _, err := prepareFilePreview(dir, large); !errors.Is(err, errFilePreviewLimit) {
		t.Fatalf("limit error = %v", err)
	}
}

func TestFilePreviewSymlinkRetarget(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "model.stl")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.stl")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	p, err := prepareFilePreview(dir, "link.stl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	if !p.External {
		t.Fatal("external symlink bypassed classification")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := p.CopyTo(t.Context(), &out); err == nil || out.Len() != 0 {
		t.Fatal("retargeted file was read")
	}
}

func TestFilePreviewSingleUse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model.stl"), []byte("mesh"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := prepareFilePreview(dir, "model.stl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	var out bytes.Buffer
	if err := p.CopyTo(t.Context(), &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := p.CopyTo(t.Context(), &out); err == nil || out.Len() != 0 {
		t.Fatal("preview replay succeeded")
	}
}

func FuzzFilePreviewPath(f *testing.F) {
	dir := f.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model.stl"), []byte("fixture"), 0600); err != nil {
		f.Fatal(err)
	}
	for _, path := range []string{"model.stl", "../model.stl", "", ".", "a/../../escape", "model.stl\x00"} {
		f.Add(path)
	}
	f.Fuzz(func(t *testing.T, path string) {
		// Keep arbitrary fuzzer inputs inside the disposable fixture namespace.
		if filepath.IsAbs(path) || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
			return
		}
		p, err := prepareFilePreview(dir, path)
		if err != nil {
			return
		}
		defer func() { _ = p.Close() }()
		if !filepath.IsLocal(path) {
			t.Fatal("accepted traversal")
		}
		if p.External {
			t.Fatal("external file without a fixture symlink")
		}
	})
}

func TestFilePreviewHomeSelector(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	if err := os.WriteFile(filepath.Join(home, "model.stl"), []byte("mesh"), 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := prepareFilePreview(root, "~/model.stl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = preview.Close() }()
	expected, err := os.Stat(filepath.Join(home, "model.stl"))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.Stat(preview.Path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows can normalize the path's case or resolve a temporary-directory
	// alias. Check the selected file's identity instead of its path spelling.
	if !preview.External || !os.SameFile(expected, actual) {
		t.Fatalf("home resolved to wrong file: %q", preview.Path)
	}
}
