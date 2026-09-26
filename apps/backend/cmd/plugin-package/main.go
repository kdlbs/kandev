// Command plugin-package inspects a native Kandev plugin archive without
// executing or extracting package content. Registry tooling uses its JSON
// output as the package-derived descriptor for publisher evidence.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/kandev/kandev/internal/plugins/pkgtar"
)

type packageDescriptor struct {
	ID               string   `json:"id"`
	Version          string   `json:"version"`
	Kind             string   `json:"kind"`
	DisplayName      string   `json:"display_name"`
	Description      string   `json:"description"`
	Author           string   `json:"author"`
	Categories       []string `json:"categories"`
	Icon             string   `json:"icon,omitempty"`
	RepoURL          string   `json:"repo_url,omitempty"`
	MinKandevVersion string   `json:"min_kandev_version,omitempty"`
	Files            []string `json:"files"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, input io.Reader, output, errorOutput io.Writer) int {
	flags := flag.NewFlagSet("plugin-package", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	fileName := flags.String("file", "-", "plugin package path, or - for stdin")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(errorOutput, "plugin-package: unexpected argument")
		return 2
	}

	reader := input
	var file *os.File
	if *fileName != "-" {
		var err error
		file, err = os.Open(*fileName)
		if err != nil {
			_, _ = fmt.Fprintln(errorOutput, "plugin-package: unable to read package")
			return 1
		}
		defer func() { _ = file.Close() }()
		reader = file
	}

	inspection, err := pkgtar.InspectPackage(reader)
	if err != nil {
		// Package paths and parser details are deliberately not returned to
		// registry callers. The inspector is an admission boundary.
		_, _ = fmt.Fprintln(errorOutput, "plugin-package: invalid plugin package")
		return 1
	}

	descriptor := describePackage(inspection)
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(descriptor); err != nil {
		_, _ = fmt.Fprintln(errorOutput, "plugin-package: unable to write descriptor")
		return 1
	}
	return 0
}

func describePackage(inspection *pkgtar.Inspection) packageDescriptor {
	manifest := inspection.Manifest
	files := make([]string, 0, len(inspection.Files))
	for name := range inspection.Files {
		files = append(files, name)
	}
	sort.Strings(files)
	return packageDescriptor{
		ID:               manifest.ID,
		Version:          manifest.Version,
		Kind:             "plugin",
		DisplayName:      manifest.DisplayName,
		Description:      manifest.Description,
		Author:           manifest.Author,
		Categories:       append([]string(nil), manifest.Categories...),
		Icon:             manifest.Icon,
		RepoURL:          manifest.RepoURL,
		MinKandevVersion: manifest.MinKandevVersion,
		Files:            files,
	}
}
