package manifest

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// ThemeContribution names a portable palette within a data-only package.
type ThemeContribution struct {
	Key   string `yaml:"key" json:"key"`
	Label string `yaml:"label" json:"label"`
	Base  string `yaml:"base" json:"base"`
	Path  string `yaml:"path" json:"path"`
}

func (m *Manifest) IsThemeDistribution() bool {
	return m != nil && m.Distribution != nil && m.Distribution.Kind == DistributionKindTheme
}

var themeKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// New manifest contributions are denied unless explicitly admitted here.
var themeMetadataFields = map[string]bool{
	"ID": true, "APIVersion": true, "Version": true, "DisplayName": true,
	"Description": true, "Author": true, "Categories": true, "RepoURL": true,
	"MinKandevVersion": true, "Distribution": true, "Themes": true,
}

func (m *Manifest) validateThemes() []error {
	var errs []error
	value := reflect.ValueOf(*m)
	for i := 0; i < value.NumField(); i++ {
		name := value.Type().Field(i).Name
		if !themeMetadataFields[name] && !value.Field(i).IsZero() {
			errs = append(errs, fmt.Errorf("theme package cannot declare %s", name))
		}
	}
	if m.Distribution.SchemaVersion != DistributionSchemaVersion || m.Distribution.SourceMode != "" {
		errs = append(errs, errors.New("theme distribution requires schema_version 1 and no source_mode"))
	}
	if len(m.Themes) < 1 || len(m.Themes) > 16 {
		errs = append(errs, errors.New("themes must contain 1-16 variants"))
	}
	return append(errs, validateThemeVariants(m.Themes)...)
}

func validateThemeVariants(themes []ThemeContribution) []error {
	var errs []error
	keys, paths := map[string]bool{}, map[string]bool{}
	for _, theme := range themes {
		if !themeKeyPattern.MatchString(theme.Key) || keys[theme.Key] {
			errs = append(errs, fmt.Errorf("invalid or duplicate theme key %q", theme.Key))
		}
		keys[theme.Key] = true
		if strings.TrimSpace(theme.Label) == "" || len(theme.Label) > 128 || (theme.Base != "dark" && theme.Base != "light") {
			errs = append(errs, fmt.Errorf("theme %q requires a label and light/dark base", theme.Key))
		}
		if err := validateRelativePackagePath(theme.Path); err != nil || strings.ContainsAny(theme.Path, `\:`) || !strings.HasSuffix(theme.Path, ".json") || paths[theme.Path] {
			errs = append(errs, fmt.Errorf("invalid or duplicate theme path %q", theme.Path))
		}
		paths[theme.Path] = true
	}
	return errs
}
