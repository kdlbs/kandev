package manifest

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// SidebarGrouping declares a complete workspace classification action, separate
// from synchronous page-local browser facets.
type SidebarGrouping struct {
	ID       string `yaml:"id" json:"id"`
	Label    string `yaml:"label" json:"label"`
	LabelKey string `yaml:"label_key,omitempty" json:"label_key,omitempty"`
	Action   string `yaml:"action" json:"action"`
}

func (m *Manifest) validateSidebarGroupings() []error {
	var errs []error
	if len(m.SidebarGroupings) > 0 && len(m.ID) > 128 {
		errs = append(errs, fmt.Errorf("sidebar grouping plugin id exceeds 128 bytes"))
	}
	if len(m.SidebarGroupings) > 16 {
		errs = append(errs, fmt.Errorf("sidebar grouping declarations exceed 16"))
	}
	seen := make(map[string]bool, len(m.SidebarGroupings))
	for _, grouping := range m.SidebarGroupings {
		if len(grouping.ID) > 128 || !idPattern.MatchString(grouping.ID) {
			errs = append(errs, fmt.Errorf("sidebar grouping has invalid id %q", grouping.ID))
		}
		if seen[grouping.ID] {
			errs = append(errs, fmt.Errorf("duplicate sidebar grouping id %q", grouping.ID))
		}
		seen[grouping.ID] = true
		if strings.TrimSpace(grouping.Label) == "" || len(grouping.Label) > 256 || !utf8.ValidString(grouping.Label) || len(grouping.LabelKey) > 256 {
			errs = append(errs, fmt.Errorf("sidebar grouping %q has invalid label", grouping.ID))
		}
		if !m.hasSidebarGroupingAction(grouping.Action) {
			errs = append(errs, fmt.Errorf("sidebar grouping %q requires a declared authenticated workspace action", grouping.ID))
		}
	}
	return errs
}

func (m *Manifest) hasSidebarGroupingAction(key string) bool {
	for _, action := range m.Actions {
		if action.Key == key && action.ResourceScope == ActionScopeWorkspace && action.EffectiveAccess() == ActionAccessAuthenticated {
			return true
		}
	}
	return false
}
