// Package pkgtartest builds valid kandev plugin tar.gz packages for tests
// and fixtures. It is a small, dependency-free builder deliberately kept
// separate from production pkgtar code so other packages (fixture
// generators, e2e helpers) can import it without pulling in *_test.go
// files.
package pkgtartest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
)

// checksumsFileName mirrors pkgtar.checksumsFileName; duplicated here to
// avoid an import cycle (pkgtar's own tests import this package).
const checksumsFileName = "checksums.txt"

// WritePackage writes a valid kandev plugin tar.gz package to w. files maps
// archive-relative path (e.g. "manifest.yaml", "server/plugin-linux-amd64")
// to its contents. WritePackage computes a checksums.txt covering every
// entry in files and appends it as the final archive entry; callers must
// not include "checksums.txt" in files themselves.
//
// Entries under "server/" are written with mode 0755 (executable); every
// other entry is written with mode 0644. This only affects the mode stored
// in the tar header — pkgtar.Install always chmods declared runtime
// executables to 0755 regardless of the archived mode.
func WritePackage(w io.Writer, files map[string][]byte) error {
	if _, exists := files[checksumsFileName]; exists {
		return fmt.Errorf("pkgtartest: files must not include %q; it is generated", checksumsFileName)
	}

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	names := sortedNames(files)
	for _, name := range names {
		if err := writeEntry(tw, name, files[name]); err != nil {
			return err
		}
	}

	checksums := buildChecksums(names, files)
	if err := writeEntry(tw, checksumsFileName, checksums); err != nil {
		return err
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("pkgtartest: closing tar writer: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("pkgtartest: closing gzip writer: %w", err)
	}
	return nil
}

func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func buildChecksums(names []string, files map[string][]byte) []byte {
	var buf bytes.Buffer
	for _, name := range names {
		sum := sha256.Sum256(files[name])
		fmt.Fprintf(&buf, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	return buf.Bytes()
}

func writeEntry(tw *tar.Writer, name string, data []byte) error {
	mode := int64(0o644)
	if strings.HasPrefix(name, "server/") {
		mode = 0o755
	}
	hdr := &tar.Header{
		Name:     name,
		Typeflag: tar.TypeReg,
		Mode:     mode,
		Size:     int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("pkgtartest: writing header for %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("pkgtartest: writing data for %s: %w", name, err)
	}
	return nil
}
