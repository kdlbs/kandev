package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	RepositoryCheckoutOptionsKey = "checkout_options"
	DownloadStandard             = "standard"
	DownloadOnDemand             = "on_demand"
)

type RepositoryCheckoutOptions = v1.RepositoryCheckoutOptions

// NormalizeRepositoryCheckoutOptions validates and copies task-owned options.
func NormalizeRepositoryCheckoutOptions(options *RepositoryCheckoutOptions) (*RepositoryCheckoutOptions, error) {
	if options == nil {
		return nil, nil
	}
	if options.Version != 1 {
		return nil, errors.New("unsupported checkout options version")
	}
	if options.DownloadMode != DownloadStandard && options.DownloadMode != DownloadOnDemand {
		return nil, errors.New("unsupported repository download mode")
	}
	encoded, err := json.Marshal(options)
	if err != nil || len(encoded) > 16*1024 || len(options.SparseDirectories) > 64 {
		return nil, errors.New("checkout options exceed size limit")
	}
	out := &RepositoryCheckoutOptions{Version: 1, DownloadMode: options.DownloadMode, SparseDirectories: []string{}}
	seen := make(map[string]bool)
	for _, directory := range options.SparseDirectories {
		if err := validateCheckoutDirectory(directory); err != nil {
			return nil, err
		}
		if !seen[directory] {
			out.SparseDirectories = append(out.SparseDirectories, directory)
			seen[directory] = true
		}
	}
	return out, nil
}

func validateCheckoutDirectory(directory string) error {
	if directory == "" || len(directory) > 4096 || !utf8.ValidString(directory) ||
		strings.ContainsAny(directory, "\\:*?[]{}") || strings.IndexFunc(directory, unicode.IsControl) >= 0 {
		return errors.New("invalid repository-relative checkout directory")
	}
	for _, segment := range strings.Split(directory, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("checkout directory must stay inside the repository")
		}
	}
	return nil
}

func PutRepositoryCheckoutOptions(metadata map[string]interface{}, options *RepositoryCheckoutOptions) error {
	value, err := NormalizeRepositoryCheckoutOptions(options)
	if err != nil {
		return err
	}
	if value != nil {
		metadata[RepositoryCheckoutOptionsKey] = value
	}
	return nil
}

func GetRepositoryCheckoutOptions(metadata map[string]interface{}) (*RepositoryCheckoutOptions, error) {
	raw, exists := metadata[RepositoryCheckoutOptionsKey]
	if !exists || raw == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil || len(encoded) > 16*1024 {
		return nil, errors.New("invalid stored checkout options")
	}
	var options RepositoryCheckoutOptions
	if err := json.Unmarshal(encoded, &options); err != nil {
		return nil, fmt.Errorf("invalid stored checkout options: %w", err)
	}
	return NormalizeRepositoryCheckoutOptions(&options)
}

// PublicRepositoryCheckoutOptions omits malformed metadata from the typed view.
// Runtime consumers use GetRepositoryCheckoutOptions and propagate its error.
func PublicRepositoryCheckoutOptions(metadata map[string]interface{}) *RepositoryCheckoutOptions {
	options, _ := GetRepositoryCheckoutOptions(metadata)
	return options
}
