package pluginpack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
)

func TestWritePackageWritesChecksumsForEveryInput(t *testing.T) {
	files := map[string][]byte{
		"manifest.yaml":             []byte("id: example\n"),
		"server/plugin-linux-amd64": []byte("binary"),
		"ui/bundle.js":              []byte("console.log('ok')"),
	}

	var archive bytes.Buffer
	if err := WritePackage(&archive, files); err != nil {
		t.Fatalf("WritePackage() error = %v", err)
	}

	entries := readEntries(t, archive.Bytes())
	checksums, ok := entries["checksums.txt"]
	if !ok {
		t.Fatal("archive does not contain checksums.txt")
	}
	listed := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(checksums)), "\n") {
		fields := strings.SplitN(line, "  ", 2)
		if len(fields) != 2 {
			t.Fatalf("invalid checksum line %q", line)
		}
		body, ok := entries[fields[1]]
		if !ok {
			t.Fatalf("checksum lists missing entry %q", fields[1])
		}
		want, err := hex.DecodeString(fields[0])
		if err != nil {
			t.Fatalf("decode checksum %q: %v", fields[0], err)
		}
		sum := sha256.Sum256(body)
		if !bytes.Equal(sum[:], want) {
			t.Fatalf("checksum mismatch for %q", fields[1])
		}
		listed[fields[1]] = true
	}
	for name := range files {
		if !listed[name] {
			t.Errorf("input %q missing from checksums", name)
		}
	}
}

func TestWritePackageRejectsCallerChecksums(t *testing.T) {
	err := WritePackage(io.Discard, map[string][]byte{"checksums.txt": nil})
	if err == nil || !strings.Contains(err.Error(), "checksums.txt") {
		t.Fatalf("WritePackage() error = %v, want generated-checksum rejection", err)
	}
}

func readEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	entries := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return entries
		}
		if err != nil {
			t.Fatalf("tar.Next() error = %v", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read %s: %v", hdr.Name, err)
		}
		entries[hdr.Name] = body
	}
}
