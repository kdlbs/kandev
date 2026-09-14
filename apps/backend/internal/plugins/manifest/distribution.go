package manifest

import (
	"errors"
	"fmt"
	"strings"
)

const (
	DistributionSchemaVersion = 1
	DistributionKindCanvas    = "canvas"
	SourceModeStatic          = "static"
	SourceModeProject         = "project"
)

// Distribution is the package-owned declaration for a portable canvas. It
// intentionally contains no registry preview URLs or publisher metadata.
type Distribution struct {
	SchemaVersion int    `yaml:"schema_version" json:"schema_version"`
	Kind          string `yaml:"kind" json:"kind"`
	License       string `yaml:"license" json:"license"`
	SourceMode    string `yaml:"source_mode" json:"source_mode"`
}

// IsCanvasDistribution reports whether the manifest asserts the portable
// canvas profile. A nil declaration preserves legacy plugin behavior.
func (m *Manifest) IsCanvasDistribution() bool {
	return m != nil && m.Distribution != nil && m.Distribution.Kind == DistributionKindCanvas
}

func (m *Manifest) validateDistribution() []error {
	if m.Distribution == nil {
		return nil
	}
	errs := validateDistributionDeclaration(m)
	errs = append(errs, validateDistributionApplication(m)...)
	errs = append(errs, validateDistributionContributions(m)...)
	return errs
}

func validateDistributionDeclaration(m *Manifest) []error {
	d := m.Distribution
	var errs []error
	if d.SchemaVersion != DistributionSchemaVersion {
		errs = append(errs, fmt.Errorf("distribution.schema_version must be %d", DistributionSchemaVersion))
	}
	if d.Kind != DistributionKindCanvas {
		errs = append(errs, fmt.Errorf("distribution.kind %q is unsupported", d.Kind))
	}
	if strings.TrimSpace(d.License) == "" {
		errs = append(errs, errors.New("distribution.license must not be empty"))
	}
	if d.SourceMode != SourceModeStatic && d.SourceMode != SourceModeProject {
		errs = append(errs, fmt.Errorf("distribution.source_mode %q is unsupported", d.SourceMode))
	}
	if _, ok := NormalizeReleaseVersion(m.MinKandevVersion); !ok {
		errs = append(errs, errors.New("min_kandev_version is required for a distribution package"))
	}
	return errs
}

func validateDistributionApplication(m *Manifest) []error {
	var errs []error
	if !m.IsStaticWebAppOnly() {
		errs = append(errs, errors.New("distribution canvas must be a static web application"))
	}
	if len(m.UI.WebApps) != 1 {
		errs = append(errs, errors.New("distribution canvas must declare exactly one web app"))
	} else if !distributionContainsString(m.UI.WebApps[0].Placements, WebAppPlacementWorkspace) {
		errs = append(errs, errors.New("distribution canvas must support workspace-canvas placement"))
	}
	if m.UI.Bundle != "" || len(m.UI.Pages) > 0 || len(m.UI.Styles) > 0 || len(m.UI.Keybindings) > 0 {
		errs = append(errs, errors.New("distribution canvas cannot declare native UI contributions"))
	}
	return errs
}

func validateDistributionContributions(m *Manifest) []error {
	if len(m.Webhooks) == 0 && len(m.Actions) == 0 && len(m.RepositoryProviders) == 0 &&
		len(m.ReferenceSources) == 0 && len(m.AuthProviders) == 0 && len(m.AgentTools) == 0 &&
		len(m.ConfigSchema) == 0 && m.Runtime.Type == "" && len(m.Runtime.Executables) == 0 {
		return nil
	}
	return []error{errors.New("distribution canvas cannot declare backend, tool, or other plugin contributions")}
}

func distributionContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
