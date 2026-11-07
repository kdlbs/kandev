package manifest

import (
	"fmt"
	"slices"
)

// NotificationFormatter declares a provider-bound content extension, not a sender.
type NotificationFormatter struct {
	Key               string   `yaml:"key" json:"key"`
	ContractVersion   int      `yaml:"contract_version" json:"contract_version"`
	ProviderTypes     []string `yaml:"provider_types" json:"provider_types"`
	ManagementActions []string `yaml:"management_actions" json:"management_actions"`
}

func (m *Manifest) validateNotificationFormatters() []error {
	seen := map[string]bool{}
	var errs []error
	for _, formatter := range m.NotificationFormatters {
		if !actionKeyPattern.MatchString(formatter.Key) || seen[formatter.Key] || formatter.ContractVersion != 1 || len(formatter.ProviderTypes) == 0 {
			errs = append(errs, fmt.Errorf("invalid notification formatter declaration"))
		}
		seen[formatter.Key] = true
		for _, kind := range formatter.ProviderTypes {
			if !validFormatterProviderType(kind) {
				errs = append(errs, fmt.Errorf("notification formatter has invalid provider type"))
			}
		}
		errs = append(errs, m.validateFormatterManagementActions(formatter)...)
	}
	for _, action := range m.Actions {
		if action.ResourceScope != ActionScopeNotificationProvider {
			continue
		}
		if !m.declaresFormatterAction(action.Key) {
			errs = append(errs, fmt.Errorf("notification provider action requires a formatter declaration"))
		}
	}
	if len(m.NotificationFormatters) > 0 && m.Runtime.Type != "binary" {
		errs = append(errs, fmt.Errorf("notification formatters require managed binary runtime"))
	}
	return errs
}

func validFormatterProviderType(kind string) bool {
	return kind == "apprise" || kind == "webhook" || kind == "webpush" || kind == "system" || kind == "local"
}

func (m *Manifest) declaresFormatterAction(key string) bool {
	for _, formatter := range m.NotificationFormatters {
		if slices.Contains(formatter.ManagementActions, key) {
			return true
		}
	}
	return false
}

func (m *Manifest) validateFormatterManagementActions(formatter NotificationFormatter) []error {
	var errs []error
	for _, key := range formatter.ManagementActions {
		found := false
		for _, action := range m.Actions {
			if action.Key == key && action.ResourceScope == ActionScopeNotificationProvider && action.MaxBodyBytes <= 128*1024 {
				found = true
			}
		}
		if !found {
			errs = append(errs, fmt.Errorf("notification formatter action %q is undeclared or has invalid scope/size", key))
		}
	}
	return errs
}
