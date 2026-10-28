package service

import (
	"expvar"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	profileCodeRequired        = "required"
	profileCodeInvalidType     = "invalid_type"
	profileCodeUnknownProperty = "unknown_property"
	profileCodeTooLarge        = "profile_too_large"
	profileMessageIDPrefix     = "executor_plugin_profile_"
)

// executorPluginProfileInvalidTotal counts host-side profile rejections by
// field error code; no profile, property, or secret label exists.
var executorPluginProfileInvalidTotal = expvar.NewMap("executor_plugin_profile_invalid_total")

// ProfileFieldErrors is the host-side validation result of a provider profile.
// It carries stable codes and property names only, never a submitted value.
type ProfileFieldErrors struct {
	Errors []models.ExecutorProviderFieldError
}

func (e *ProfileFieldErrors) Error() string {
	first := e.Errors[0]
	if first.Field == "" {
		return fmt.Sprintf("%s: provider profile failed validation (%s)", ErrInvalidExecutorConfig, first.Code)
	}
	return fmt.Sprintf("%s: provider profile field %q failed validation (%s)", ErrInvalidExecutorConfig, first.Field, first.Code)
}

func (e *ProfileFieldErrors) Unwrap() error { return ErrInvalidExecutorConfig }

func profileFieldError(field, code string) models.ExecutorProviderFieldError {
	return models.ExecutorProviderFieldError{Field: field, Code: code, MessageID: profileMessageIDPrefix + code}
}

// providerDeclaresCapabilities reports whether the provider's current manifest
// requires any host capability, which switches on the stricter profile rules.
func providerDeclaresCapabilities(provider models.ExecutorProvider) bool {
	return len(provider.RequiredCapabilities) > 0
}

// hostOwnedProfileKey reports the profile keys that Kandev itself reads
// besides the six in kandevExecutorProfileKeys.
func hostOwnedProfileKey(key string) bool {
	switch key {
	case "launch_policy", "policy_id", "checkout_kind", "checkout_value", "sparse_paths":
		return true
	}
	return strings.HasPrefix(key, "hint.")
}

// validatePluginExecutorProfileSchema validates a profile at save, including
// the aggregate size limit.
func validatePluginExecutorProfileSchema(provider models.ExecutorProvider, config map[string]string) error {
	_, err := validateProfileAgainstSchema(provider, config, true)
	return err
}

// validateRecordedPluginExecutorProfile validates the snapshot recorded at
// allocation. The size limit is applied by the caller from the recorded
// negotiated set, so a later upgrade cannot fail an existing environment.
func validateRecordedPluginExecutorProfile(provider models.ExecutorProvider, config map[string]string) error {
	_, err := validateProfileAgainstSchema(provider, config, false)
	return err
}

// fillPluginExecutorProfile validates the profile, applies the aggregate size
// limit, and returns it with absent or empty optional values replaced by their
// defaults. This is the profile that a launch snapshots.
func fillPluginExecutorProfile(provider models.ExecutorProvider, config map[string]string) (map[string]string, error) {
	return validateProfileAgainstSchema(provider, config, true)
}

func validateProfileAgainstSchema(provider models.ExecutorProvider, config map[string]string, measure bool) (map[string]string, error) {
	fields, required, err := executorProviderSchemaFields(provider.ProfileSchema)
	if err != nil {
		return nil, err
	}
	filled := cloneExecutorProfileConfig(config)
	if filled == nil {
		filled = map[string]string{}
	}
	declares := providerDeclaresCapabilities(provider)
	var problems []models.ExecutorProviderFieldError
	for _, key := range sortedProfileKeys(config) {
		if code := checkProfileKey(key, config[key], fields, declares); code != "" {
			problems = append(problems, profileFieldError(key, code))
		}
	}
	fillProfileDefaults(fields, filled)
	for _, key := range required {
		if profileValueAbsent(fields[key], filled[key]) {
			problems = append(problems, profileFieldError(key, profileCodeRequired))
		}
	}
	if len(problems) == 0 && measure && declares &&
		pluginExecutorProfileContractBytes(provider, filled) > models.ExecutorProfileContractBudgetBytes {
		problems = append(problems, profileFieldError("", profileCodeTooLarge))
	}
	if len(problems) > 0 {
		for _, problem := range problems {
			executorPluginProfileInvalidTotal.Add(problem.Code, 1)
		}
		return nil, &ProfileFieldErrors{Errors: problems}
	}
	return filled, nil
}

func sortedProfileKeys(config map[string]string) []string {
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// checkProfileKey returns the field error code for one stored entry.
func checkProfileKey(key, value string, fields map[string]executorProviderField, declares bool) string {
	if _, kandevKey := kandevExecutorProfileKeys[key]; kandevKey {
		if validateKandevExecutorProfileValue(key, value) != nil {
			return profileCodeInvalidType
		}
		return ""
	}
	field, exists := fields[key]
	if !exists {
		if declares && hostOwnedProfileKey(key) {
			return ""
		}
		return profileCodeUnknownProperty
	}
	_, isReference := models.ExecutorProfileSecretID(value)
	if field.secret {
		if !isReference {
			return profileCodeInvalidType
		}
		return ""
	}
	if isReference {
		return profileCodeInvalidType
	}
	if profileValueAbsent(field, value) {
		return ""
	}
	return field.property.CheckValue(value)
}

// profileValueAbsent applies the absent-equals-empty rule: only a string with
// an explicit minLength of 0 keeps an empty value as a real value.
func profileValueAbsent(field executorProviderField, value string) bool {
	return value == "" && !field.property.AllowsEmpty()
}

func fillProfileDefaults(fields map[string]executorProviderField, filled map[string]string) {
	for key, field := range fields {
		if field.secret || !profileValueAbsent(field, filled[key]) {
			continue
		}
		if text, ok := field.property.DefaultText(); ok {
			filled[key] = text
		}
	}
}

// pluginExecutorProfileContractBytes measures the provider-facing config and
// the secret store IDs of a filled profile with the shared models function.
func pluginExecutorProfileContractBytes(provider models.ExecutorProvider, filled map[string]string) int {
	fields, _, _ := executorProviderSchemaFields(provider.ProfileSchema)
	public := publicExecutorProviderConfig(provider, filled)
	var ids []string
	for _, key := range sortedProfileKeys(filled) {
		if fields[key].secret {
			if id, ok := models.ExecutorProfileSecretID(filled[key]); ok {
				ids = append(ids, id)
			}
		}
	}
	return models.ExecutorProfileContractBytes(public, ids)
}
