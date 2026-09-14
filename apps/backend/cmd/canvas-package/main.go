// Command canvas-package inspects a portable canvas bundle without executing
// or extracting any package content. Registry tooling uses its JSON output as
// the trusted package descriptor.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/kandev/kandev/internal/plugins/webapp"
)

type packageDescriptor struct {
	ID               string   `json:"id"`
	Version          string   `json:"version"`
	Kind             string   `json:"kind"`
	DisplayName      string   `json:"display_name"`
	Description      string   `json:"description"`
	Author           string   `json:"author"`
	License          string   `json:"license"`
	SourceMode       string   `json:"source_mode"`
	MinKandevVersion string   `json:"min_kandev_version"`
	RepoURL          string   `json:"repo_url,omitempty"`
	Digest           string   `json:"sha256"`
	CompressedBytes  int64    `json:"compressed_bytes"`
	ExpandedBytes    int64    `json:"expanded_bytes"`
	Files            []string `json:"files"`
	APIRead          []string `json:"api_read,omitempty"`
	APIWrite         []string `json:"api_write,omitempty"`
	Events           []string `json:"events,omitempty"`
	State            bool     `json:"state,omitempty"`
	Secrets          bool     `json:"secrets,omitempty"`
	NetworkOrigins   []string `json:"network_origins,omitempty"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, input io.Reader, output, errorOutput io.Writer) int {
	flags := flag.NewFlagSet("canvas-package", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	fileName := flags.String("file", "-", "bundle path, or - for stdin")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(errorOutput, "canvas-package: unexpected argument")
		return 2
	}

	reader := input
	var file *os.File
	if *fileName != "-" {
		var err error
		file, err = os.Open(*fileName)
		if err != nil {
			_, _ = fmt.Fprintln(errorOutput, "canvas-package: unable to read package")
			return 1
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	pkg, err := webapp.ValidateDistributionPackage(reader)
	if err != nil {
		// Package paths and parser details are deliberately not returned to
		// registry callers. The inspector is an admission boundary.
		_, _ = fmt.Fprintln(errorOutput, "canvas-package: invalid distribution package")
		return 1
	}
	descriptor := describePackage(pkg)
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(descriptor); err != nil {
		_, _ = fmt.Fprintln(errorOutput, "canvas-package: unable to write descriptor")
		return 1
	}
	return 0
}

func describePackage(pkg *webapp.Package) packageDescriptor {
	manifest := pkg.Manifest
	files := make([]string, 0, len(pkg.Files))
	for name := range pkg.Files {
		files = append(files, name)
	}
	sort.Strings(files)
	networkOrigins := make([]string, 0)
	if len(manifest.UI.WebApps) == 1 {
		networkOrigins = append(networkOrigins, manifest.UI.WebApps[0].NetworkOrigins...)
	}
	return packageDescriptor{
		ID:               manifest.ID,
		Version:          manifest.Version,
		Kind:             manifest.Distribution.Kind,
		DisplayName:      manifest.DisplayName,
		Description:      manifest.Description,
		Author:           manifest.Author,
		License:          manifest.Distribution.License,
		SourceMode:       manifest.Distribution.SourceMode,
		MinKandevVersion: manifest.MinKandevVersion,
		RepoURL:          manifest.RepoURL,
		Digest:           pkg.Digest,
		CompressedBytes:  pkg.CompressedBytes,
		ExpandedBytes:    pkg.ExpandedBytes,
		Files:            files,
		APIRead:          append([]string(nil), manifest.Capabilities.APIRead...),
		APIWrite:         append([]string(nil), manifest.Capabilities.APIWrite...),
		Events:           append([]string(nil), manifest.Capabilities.Events...),
		State:            manifest.Capabilities.State,
		Secrets:          manifest.Capabilities.Secrets,
		NetworkOrigins:   networkOrigins,
	}
}
