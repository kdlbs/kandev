package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/db/sqlguard"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("sqlguard", flag.ContinueOnError)
	exemptionsPath := flags.String("exemptions", "internal/db/sqlguard/exemptions.json", "exact exemption registry")
	if err := flags.Parse(args); err != nil {
		return err
	}
	paths := flags.Args()
	if len(paths) == 0 {
		return errors.New("usage: sqlguard [-exemptions path] file-or-directory")
	}
	exemptions, err := sqlguard.LoadExemptions(*exemptionsPath)
	if err != nil {
		return err
	}
	files, err := goFiles(paths)
	if err != nil {
		return err
	}
	findings, err := sqlguard.CheckFiles(files, exemptions)
	if err != nil {
		return err
	}
	for _, finding := range findings {
		fmt.Printf("%s:%d:%d: %s [%s] (%s)\n", finding.File, finding.Line, finding.Column, finding.Message, finding.Rule, finding.Symbol)
	}
	if len(findings) > 0 {
		return fmt.Errorf("sqlguard found %d violation(s)", len(findings))
	}
	return nil
}

func goFiles(paths []string) ([]string, error) {
	files := make([]string, 0)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
		if !info.IsDir() {
			if filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
				files = append(files, filepath.ToSlash(path))
			}
			continue
		}
		err = filepath.WalkDir(path, func(candidate string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(candidate) != ".go" || strings.HasSuffix(candidate, "_test.go") {
				return nil
			}
			files = append(files, filepath.ToSlash(candidate))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", path, err)
		}
	}
	sort.Strings(files)
	return files, nil
}
