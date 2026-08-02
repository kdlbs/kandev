package runtimeflags

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

type featureFieldBinding struct {
	fieldIndex int
	fieldName  string
	key        string
	profileKey string
	envVar     string
}

func TestDefinitionsIncludeOfficeExperimentalMetadata(t *testing.T) {
	def, ok := DefinitionByKey("features.office")
	if !ok {
		t.Fatal("features.office definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_OFFICE" {
		t.Fatalf("EnvVar = %q, want KANDEV_FEATURES_OFFICE", def.EnvVar)
	}
	if def.Stability != StabilityExperimental {
		t.Fatalf("Stability = %q, want %q", def.Stability, StabilityExperimental)
	}
	if def.RiskDescription == "" {
		t.Fatal("RiskDescription empty")
	}
	if !def.RestartRequired {
		t.Fatal("RestartRequired = false, want true")
	}
}

// TestDefinitionsExcludePlugins pins the graduation of the plugin system out
// of the feature-flag tier: plugins ship in the base product, so no toggle may
// reappear in Settings > System > Feature Toggles.
func TestDefinitionsExcludePlugins(t *testing.T) {
	if _, ok := DefinitionByKey("features.plugins"); ok {
		t.Fatal("features.plugins definition present; plugins are a base feature and must not be toggleable")
	}
	for _, def := range Definitions() {
		if def.EnvVar == "KANDEV_FEATURES_PLUGINS" {
			t.Fatalf("definition %q still binds KANDEV_FEATURES_PLUGINS", def.Key)
		}
	}
}

func TestRetiredRuntimeFlagIdentitiesIncludePlugins(t *testing.T) {
	for _, identity := range retiredRuntimeFlagIdentities {
		if identity.key == "features.plugins" && identity.envVar == "KANDEV_FEATURES_PLUGINS" {
			return
		}
	}
	t.Fatal("graduated plugins flag identity is missing from the retired identity set")
}

func TestDefinitionsIncludeAppStatusBarMetadata(t *testing.T) {
	def, ok := DefinitionByKey("features.appStatusBar")
	if !ok {
		t.Fatal("features.appStatusBar definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_APP_STATUS_BAR" {
		t.Fatalf("EnvVar = %q, want KANDEV_FEATURES_APP_STATUS_BAR", def.EnvVar)
	}
	if !def.RestartRequired {
		t.Fatal("RestartRequired = false, want true")
	}
}

func TestDefinitionsIncludeClaudeBackgroundPromptHandoffMetadata(t *testing.T) {
	def, ok := DefinitionByKey("features.claudeBackgroundPromptHandoff")
	if !ok {
		t.Fatal("features.claudeBackgroundPromptHandoff definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF" {
		t.Fatalf(
			"EnvVar = %q, want KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF",
			def.EnvVar,
		)
	}
	if def.Stability != StabilityExperimental {
		t.Fatalf("Stability = %q, want %q", def.Stability, StabilityExperimental)
	}
	if def.RiskLevel != RiskHigh {
		t.Fatalf("RiskLevel = %q, want %q", def.RiskLevel, RiskHigh)
	}
	if def.RiskDescription == "" {
		t.Fatal("RiskDescription empty")
	}
	if !def.RestartRequired {
		t.Fatal("RestartRequired = false, want true")
	}
	if !def.Mutable {
		t.Fatal("Mutable = false, want true")
	}
}
func TestDefinitionsExposeSingleUserFacingDebugToggle(t *testing.T) {
	def, ok := DefinitionByKey("debug.devMode")
	if !ok {
		t.Fatal("debug.devMode definition missing")
	}
	if def.EnvVar != "KANDEV_DEBUG_DEV_MODE" {
		t.Fatalf("EnvVar = %q, want KANDEV_DEBUG_DEV_MODE", def.EnvVar)
	}
	if len(def.ImpliedEnvVars) == 0 {
		t.Fatal("Debug mode should imply subordinate debug env vars")
	}
	if _, ok := DefinitionByKey("debug.agentMessages"); ok {
		t.Fatal("debug.agentMessages must not be a top-level user-facing toggle")
	}
}

func TestRuntimeFlagRegistrationsHaveCompleteMetadata(t *testing.T) {
	retiredKeys, retiredEnvVars := retiredIdentitySets(t)
	activeKeys := make(map[string]struct{}, len(registrations))
	activeEnvVars := make(map[string]string, len(registrations))

	for _, registration := range registrations {
		definition := registration.definition
		validateRuntimeFlagDefinition(t, definition)
		if registration.read == nil || registration.apply == nil {
			t.Fatalf("runtime flag registration %q is missing a config binding", definition.Key)
		}
		if _, exists := activeKeys[definition.Key]; exists {
			t.Fatalf("duplicate runtime flag registration for %q", definition.Key)
		}
		activeKeys[definition.Key] = struct{}{}
		if _, retired := retiredKeys[definition.Key]; retired {
			t.Fatalf("active runtime flag %q reuses a retired key", definition.Key)
		}
		registerActiveEnvVar(t, activeEnvVars, retiredEnvVars, definition.Key, definition.EnvVar)
		for _, envVar := range definition.ImpliedEnvVars {
			registerActiveEnvVar(t, activeEnvVars, retiredEnvVars, definition.Key, envVar)
		}
	}
}

func TestRuntimeFlagConfigBindingsAreIsolated(t *testing.T) {
	// Keep debug-mode's intentional environment side effects inert while this
	// test exercises only the typed config bindings.
	t.Setenv(envDebugAgentMessages, "explicit")
	t.Setenv(envDebugPprofEnabled, "explicit")

	for _, target := range registrations {
		cfg := &config.Config{}
		target.apply(cfg, true)
		for _, registration := range registrations {
			want := registration.definition.Key == target.definition.Key
			if got := registration.read(cfg); got != want {
				t.Fatalf(
					"applying %q changed read value for %q to %t; want %t",
					target.definition.Key,
					registration.definition.Key,
					got,
					want,
				)
			}
		}

		target.apply(cfg, false)
		for _, registration := range registrations {
			if registration.read(cfg) {
				t.Fatalf("disabling %q left %q enabled", target.definition.Key, registration.definition.Key)
			}
		}
	}
}

// TestFeatureBindingsExactlyMatchConfigAndProfiles keeps the typed config,
// profile defaults, and feature registry in lockstep in both directions.
func TestFeatureBindingsExactlyMatchConfigAndProfiles(t *testing.T) {
	defaults, err := profiles.FeatureFlagDefaults()
	if err != nil {
		t.Fatalf("FeatureFlagDefaults: %v", err)
	}
	fields := featureFieldBindings(t)
	registered := registeredFeatureBindings(t)

	for key, field := range fields {
		registration, ok := registered[key]
		if !ok {
			t.Fatalf("FeaturesConfig.%s has no runtime flag registration for %q", field.fieldName, key)
		}
		if registration.definition.EnvVar != field.envVar {
			t.Fatalf("FeaturesConfig.%s EnvVar = %q, want %q", field.fieldName, registration.definition.EnvVar, field.envVar)
		}
		if _, ok := defaults[field.profileKey]; !ok {
			t.Fatalf("FeaturesConfig.%s has no profile default for %q", field.fieldName, field.profileKey)
		}
	}
	for key := range registered {
		if _, ok := fields[key]; !ok {
			t.Fatalf("feature registration %q has no FeaturesConfig field", key)
		}
	}
	for profileKey := range defaults {
		if !hasProfileKey(fields, profileKey) {
			t.Fatalf("profile feature %q has no FeaturesConfig field", profileKey)
		}
	}
	if len(fields) != len(registered) || len(fields) != len(defaults) {
		t.Fatalf(
			"feature contract size mismatch: config=%d registry=%d profiles=%d",
			len(fields),
			len(registered),
			len(defaults),
		)
	}

	assertFeatureConfigRoundTrips(t, fields)
}

func retiredIdentitySets(t *testing.T) (map[string]struct{}, map[string]struct{}) {
	t.Helper()
	keys := make(map[string]struct{}, len(retiredRuntimeFlagIdentities))
	envVars := make(map[string]struct{}, len(retiredRuntimeFlagIdentities))
	for _, identity := range retiredRuntimeFlagIdentities {
		if strings.TrimSpace(identity.key) == "" || strings.TrimSpace(identity.envVar) == "" {
			t.Fatal("retired runtime flag identities must include both key and environment variable")
		}
		if _, exists := keys[identity.key]; exists {
			t.Fatalf("duplicate retired runtime flag key %q", identity.key)
		}
		keys[identity.key] = struct{}{}
		if _, exists := envVars[identity.envVar]; exists {
			t.Fatalf("duplicate retired runtime flag environment variable %q", identity.envVar)
		}
		envVars[identity.envVar] = struct{}{}
	}
	return keys, envVars
}

func registerActiveEnvVar(
	t *testing.T,
	active map[string]string,
	retired map[string]struct{},
	key string,
	envVar string,
) {
	t.Helper()
	if strings.TrimSpace(envVar) == "" {
		t.Fatalf("runtime flag %q has an empty environment variable", key)
	}
	if owner, exists := active[envVar]; exists {
		t.Fatalf("runtime flags %q and %q both use environment variable %q", owner, key, envVar)
	}
	if _, exists := retired[envVar]; exists {
		t.Fatalf("active runtime flag %q reuses retired environment variable %q", key, envVar)
	}
	active[envVar] = key
}

func validateRuntimeFlagDefinition(t *testing.T, definition RuntimeFlagDefinition) {
	t.Helper()
	wantKeyPrefix, wantEnvPrefix := definitionPrefixes(t, definition.Kind)
	if !strings.HasPrefix(definition.Key, wantKeyPrefix) || len(definition.Key) == len(wantKeyPrefix) {
		t.Fatalf("runtime flag key %q must start with %q and include a name", definition.Key, wantKeyPrefix)
	}
	if !strings.HasPrefix(definition.EnvVar, wantEnvPrefix) || len(definition.EnvVar) == len(wantEnvPrefix) {
		t.Fatalf("runtime flag %q environment variable %q must start with %q", definition.Key, definition.EnvVar, wantEnvPrefix)
	}
	for name, value := range map[string]string{
		"label": definition.Label, "description": definition.Description, "risk description": definition.RiskDescription,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("runtime flag %q has an empty %s", definition.Key, name)
		}
	}
	if !validStability(definition.Stability) {
		t.Fatalf("runtime flag %q has invalid stability %q", definition.Key, definition.Stability)
	}
	if !validRiskLevel(definition.RiskLevel) {
		t.Fatalf("runtime flag %q has invalid risk level %q", definition.Key, definition.RiskLevel)
	}
	if !definition.RestartRequired {
		t.Fatalf("runtime flag %q must declare restart_required", definition.Key)
	}
	if !definition.Mutable {
		t.Fatalf("runtime flag %q must declare mutable", definition.Key)
	}
}

func definitionPrefixes(t *testing.T, kind RuntimeFlagKind) (string, string) {
	t.Helper()
	switch kind {
	case KindFeature:
		return "features.", "KANDEV_FEATURES_"
	case KindDebug:
		return "debug.", "KANDEV_DEBUG_"
	default:
		t.Fatalf("invalid runtime flag kind %q", kind)
		return "", ""
	}
}

func validStability(stability RuntimeFlagStability) bool {
	switch stability {
	case StabilityStable, StabilityBeta, StabilityExperimental:
		return true
	default:
		return false
	}
}

func validRiskLevel(risk RuntimeFlagRiskLevel) bool {
	switch risk {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	default:
		return false
	}
}

func featureFieldBindings(t *testing.T) map[string]featureFieldBinding {
	t.Helper()
	typeOfFeatures := reflect.TypeOf(config.FeaturesConfig{})
	fields := make(map[string]featureFieldBinding, typeOfFeatures.NumField())
	for i := 0; i < typeOfFeatures.NumField(); i++ {
		field := typeOfFeatures.Field(i)
		if field.Type.Kind() != reflect.Bool {
			t.Fatalf("FeaturesConfig.%s has type %s; feature flags must be bool", field.Name, field.Type)
		}
		jsonName := requiredTagName(t, field, "json")
		profileKey := requiredTagName(t, field, "mapstructure")
		key := "features." + jsonName
		if _, exists := fields[key]; exists {
			t.Fatalf("duplicate FeaturesConfig JSON key %q", jsonName)
		}
		fields[key] = featureFieldBinding{
			fieldIndex: i,
			fieldName:  field.Name,
			key:        key,
			profileKey: profileKey,
			envVar:     "KANDEV_FEATURES_" + strings.ToUpper(profileKey),
		}
	}
	return fields
}

func requiredTagName(t *testing.T, field reflect.StructField, tag string) string {
	t.Helper()
	name := strings.Split(field.Tag.Get(tag), ",")[0]
	if name == "" || name == "-" {
		t.Fatalf("FeaturesConfig.%s is missing a %s name", field.Name, tag)
	}
	return name
}

func registeredFeatureBindings(t *testing.T) map[string]runtimeFlagRegistration {
	t.Helper()
	registered := make(map[string]runtimeFlagRegistration)
	for _, registration := range registrations {
		definition := registration.definition
		if definition.Kind != KindFeature && !strings.HasPrefix(definition.Key, "features.") {
			continue
		}
		if _, exists := registered[definition.Key]; exists {
			t.Fatalf("duplicate feature registration %q", definition.Key)
		}
		registered[definition.Key] = registration
	}
	return registered
}

func hasProfileKey(fields map[string]featureFieldBinding, profileKey string) bool {
	for _, field := range fields {
		if field.profileKey == profileKey {
			return true
		}
	}
	return false
}

func assertFeatureConfigRoundTrips(t *testing.T, fields map[string]featureFieldBinding) {
	t.Helper()
	typeOfFeatures := reflect.TypeOf(config.FeaturesConfig{})
	for key, field := range fields {
		cfg := &config.Config{}
		features := reflect.ValueOf(&cfg.Features).Elem()
		ApplyStatesToConfig(cfg, []RuntimeFlagState{{Key: key, EffectiveValue: true}})
		if !features.Field(field.fieldIndex).Bool() {
			t.Fatalf("ApplyStatesToConfig(%q) did not enable target field %s", key, field.fieldName)
		}
		for i := 0; i < features.NumField(); i++ {
			if i != field.fieldIndex && features.Field(i).Bool() {
				t.Fatalf("ApplyStatesToConfig(%q) changed unrelated field %s", key, typeOfFeatures.Field(i).Name)
			}
		}
		if got := ValuesFromConfig(cfg)[key]; !got {
			t.Fatalf("ValuesFromConfig(%q) = false after enabling %s", key, field.fieldName)
		}
		ApplyStatesToConfig(cfg, []RuntimeFlagState{{Key: key, EffectiveValue: false}})
		if features.Field(field.fieldIndex).Bool() || ValuesFromConfig(cfg)[key] {
			t.Fatalf("ApplyStatesToConfig(%q) did not disable target field %s", key, field.fieldName)
		}
	}
}
