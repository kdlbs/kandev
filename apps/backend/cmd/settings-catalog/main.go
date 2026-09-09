package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	agentcatalog "github.com/kandev/kandev/internal/agent/settings/catalog"
	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/settingscatalog"
	userdto "github.com/kandev/kandev/internal/user/dto"
)

const schemaVersion = "settings-catalog.v1"

type snapshot struct {
	SchemaVersion string                             `json:"schema_version"`
	GeneratedBy   string                             `json:"generated_by"`
	Domains       []settingscatalog.DomainDescriptor `json:"domains"`
	MutableFields map[string][]string                `json:"mutable_fields"`
}

func main() {
	check := flag.Bool("check", false, "check generated snapshots without writing them")
	flag.Parse()

	root, err := moduleRoot()
	if err != nil {
		fatal(err)
	}
	all, profile, err := snapshots()
	if err != nil {
		fatal(err)
	}
	outputs := map[string][]byte{
		filepath.Join(root, "../web/lib/settings-discovery/contract.generated.json"):         all,
		filepath.Join(root, "../web/lib/settings-discovery/profile-contract.generated.json"): profile,
	}
	for path, expected := range outputs {
		if *check {
			actual, readErr := os.ReadFile(path)
			if readErr != nil {
				fatal(fmt.Errorf("read %s: %w", path, readErr))
			}
			if !bytes.Equal(actual, expected) {
				fatal(fmt.Errorf("generated snapshot is stale: %s", path))
			}
			continue
		}
		if err := os.WriteFile(path, expected, 0o644); err != nil {
			fatal(fmt.Errorf("write %s: %w", path, err))
		}
	}
}

func snapshots() ([]byte, []byte, error) {
	profileDomains := agentcatalog.Descriptors()
	profileTypes := make(map[string]struct{}, len(profileDomains))
	for _, domain := range profileDomains {
		profileTypes[domain.ResourceType] = struct{}{}
	}
	allDomains := make([]settingscatalog.DomainDescriptor, 0)
	for _, domain := range settingscatalog.DefaultDomainDescriptors() {
		if _, exists := profileTypes[domain.ResourceType]; exists {
			continue
		}
		allDomains = append(allDomains, domain)
	}
	allDomains = append(allDomains, profileDomains...)
	allRegistry, err := settingscatalog.NewRegistry(allDomains)
	if err != nil {
		return nil, nil, err
	}
	profileRegistry, err := settingscatalog.NewRegistry(profileDomains)
	if err != nil {
		return nil, nil, err
	}
	mutableFields := mutableFieldInventory()
	all, err := marshalSnapshot(allRegistry, mutableFields)
	if err != nil {
		return nil, nil, err
	}
	profile, err := marshalSnapshot(profileRegistry, mutableFields)
	if err != nil {
		return nil, nil, err
	}
	return all, profile, nil
}

func marshalSnapshot(registry *settingscatalog.Registry, mutableFields map[string][]string) ([]byte, error) {
	domains := registry.Domains()
	sort.Slice(domains, func(i, j int) bool { return domains[i].ResourceType < domains[j].ResourceType })
	payload, err := json.MarshalIndent(snapshot{
		SchemaVersion: schemaVersion,
		GeneratedBy:   "cmd/settings-catalog",
		Domains:       domains,
		MutableFields: mutableFields,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func mutableFieldInventory() map[string][]string {
	userFields := jsonFieldPaths(reflect.TypeFor[userdto.UpdateUserSettingsRequest](), nil)
	// sidebar_task_colors is the public patch alias. The DTO stores its
	// normalized form as sidebar_task_color_patch before calling the service.
	userFields = append(userFields, "sidebar_task_colors")
	sort.Strings(userFields)
	return map[string][]string{
		"agent_profile": jsonFieldPaths(reflect.TypeFor[agentsettingsdto.ProfileUpdateRequest](), map[string]struct{}{
			"id": {}, "force": {},
		}),
		"user_settings": userFields,
	}
}

func jsonFieldPaths(valueType reflect.Type, excluded map[string]struct{}) []string {
	if valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	paths := make([]string, 0, valueType.NumField())
	for index := 0; index < valueType.NumField(); index++ {
		field := valueType.Field(index)
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" || jsonName == "-" {
			continue
		}
		if _, skip := excluded[jsonName]; skip {
			continue
		}
		paths = append(paths, jsonName)
	}
	sort.Strings(paths)
	return paths
}

func moduleRoot() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for current := workingDirectory; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod not found above %s", workingDirectory)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
