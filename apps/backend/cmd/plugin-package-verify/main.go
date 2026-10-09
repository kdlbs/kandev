package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kandev/kandev/internal/plugins/pkgtar"
)

type output struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Signed  bool   `json:"signed"`
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("plugin-package-verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	archivePath := flags.String("archive", "", "path to the plugin package")
	expectedID := flags.String("expected-id", "", "curated plugin ID")
	expectedVersion := flags.String("expected-version", "", "normalized release version")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *archivePath == "" || *expectedID == "" || *expectedVersion == "" {
		writeError(stderr, "archive, expected-id, and expected-version are required\n")
		return 2
	}

	archive, err := os.Open(*archivePath)
	if err != nil {
		writeError(stderr, "open package: %v\n", err)
		return 1
	}
	verified, err := pkgtar.Verify(archive)
	closeErr := archive.Close()
	if err != nil {
		writeError(stderr, "verify package: %v\n", err)
		return 1
	}
	if closeErr != nil {
		writeError(stderr, "close package: %v\n", closeErr)
		return 1
	}
	if verified.Manifest.ID != *expectedID || verified.Manifest.Version != *expectedVersion {
		writeError(
			stderr,
			"verified manifest is %s@%s, expected %s@%s\n",
			verified.Manifest.ID,
			verified.Manifest.Version,
			*expectedID,
			*expectedVersion,
		)
		return 1
	}

	digest, err := fileSHA256(*archivePath)
	if err != nil {
		writeError(stderr, "hash package: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(output{
		ID:      verified.Manifest.ID,
		Version: verified.Manifest.Version,
		SHA256:  digest,
		Signed:  verified.Signed,
	}); err != nil {
		writeError(stderr, "encode result: %v\n", err)
		return 1
	}
	return 0
}

func writeError(stderr io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(stderr, format, args...)
}

func fileSHA256(archivePath string) (string, error) {
	archive, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = archive.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, archive); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
