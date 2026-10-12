package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

const executorProfileSecretNamePrefix = "kandev-executor-profile:"

type executorProviderField struct {
	typeName string
	secret   bool
	enum     []any
	minimum  *float64
	maximum  *float64
}

// kandevExecutorProfileKeys are executor profile keys owned by Kandev rather than
// the provider: agent credential delivery and git identity, shared with the other
// remote executors. The value returns a target for the JSON shape a key holds,
// or is nil for plain text. They are validated here and never sent to the provider.
var kandevExecutorProfileKeys = map[string]func() any{
	"remote_credentials":      func() any { return &[]string{} },
	"remote_auth_secrets":     func() any { return &map[string]string{} },
	"agent_config_bundles":    func() any { return &[]string{} },
	"remote_auth_target_home": nil,
	"git_user_name":           nil,
	"git_user_email":          nil,
}

func validateKandevExecutorProfileValue(key, value string) error {
	shape := kandevExecutorProfileKeys[key]
	if shape == nil {
		return nil
	}
	if err := json.Unmarshal([]byte(value), shape()); err != nil {
		return fmt.Errorf("%w: %s has an invalid value", ErrInvalidExecutorConfig, key)
	}
	return nil
}

func executorProviderSchemaFields(schema map[string]any) (map[string]executorProviderField, []string, error) {
	properties, ok := schemaMap(schema["properties"])
	if !ok || len(properties) == 0 {
		return nil, nil, fmt.Errorf("%w: provider profile schema is unavailable", ErrInvalidExecutorConfig)
	}
	fields := make(map[string]executorProviderField, len(properties))
	for name, raw := range properties {
		if _, reserved := kandevExecutorProfileKeys[name]; reserved {
			return nil, nil, fmt.Errorf("%w: provider profile field %q is reserved by Kandev", ErrInvalidExecutorConfig, name)
		}
		property, ok := schemaMap(raw)
		if !ok {
			return nil, nil, fmt.Errorf("%w: invalid provider field schema", ErrInvalidExecutorConfig)
		}
		typeName, ok := property["type"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("%w: invalid provider field type", ErrInvalidExecutorConfig)
		}
		field := executorProviderField{typeName: typeName}
		field.secret, _ = property["secret"].(bool)
		field.enum, _ = schemaAnySlice(property["enum"])
		field.minimum = schemaNumber(property["minimum"])
		field.maximum = schemaNumber(property["maximum"])
		fields[name] = field
	}
	required, _ := schemaStringSlice(schema["required"])
	return fields, required, nil
}

func schemaMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			name, ok := key.(string)
			if !ok {
				return nil, false
			}
			out[name] = value
		}
		return out, true
	default:
		return nil, false
	}
}

func schemaStringSlice(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, len(typed))
		for index, item := range typed {
			name, ok := item.(string)
			if !ok {
				return nil, false
			}
			out[index] = name
		}
		return out, true
	default:
		return nil, false
	}
}

func schemaAnySlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []string:
		out := make([]any, len(typed))
		for index := range typed {
			out[index] = typed[index]
		}
		return out, true
	default:
		return nil, false
	}
}

func schemaNumber(value any) *float64 {
	var parsed float64
	switch typed := value.(type) {
	case int:
		parsed = float64(typed)
	case int64:
		parsed = float64(typed)
	case float64:
		parsed = typed
	default:
		return nil
	}
	return &parsed
}

func validatePluginExecutorProfileSchema(provider models.ExecutorProvider, config map[string]string) error {
	fields, required, err := executorProviderSchemaFields(provider.ProfileSchema)
	if err != nil {
		return err
	}
	for key, value := range config {
		if _, kandevKey := kandevExecutorProfileKeys[key]; kandevKey {
			if err := validateKandevExecutorProfileValue(key, value); err != nil {
				return err
			}
			continue
		}
		field, exists := fields[key]
		if !exists {
			return fmt.Errorf("%w: undeclared provider profile field %q", ErrInvalidExecutorConfig, key)
		}
		if field.secret {
			if _, ok := models.ExecutorProfileSecretID(value); !ok {
				return fmt.Errorf("%w: secret field %q is not stored in the vault", ErrInvalidExecutorConfig, key)
			}
			continue
		}
		if _, ok := models.ExecutorProfileSecretID(value); ok {
			return fmt.Errorf("%w: secret references are allowed only for declared secret fields", ErrInvalidExecutorConfig)
		}
		if err := validateExecutorProviderScalar(key, value, field); err != nil {
			return err
		}
	}
	for _, key := range required {
		value, exists := config[key]
		if !exists || value == "" {
			return fmt.Errorf("%w: required provider profile field %q is missing", ErrInvalidExecutorConfig, key)
		}
	}
	return nil
}

func validateExecutorProviderScalar(key, value string, field executorProviderField) error {
	numeric, hasNumeric, err := parseExecutorProviderScalar(key, value, field.typeName)
	if err != nil {
		return err
	}
	if err := validateExecutorProviderEnum(key, value, field.enum); err != nil {
		return err
	}
	return validateExecutorProviderBounds(key, numeric, hasNumeric, field)
}

func parseExecutorProviderScalar(key, value, typeName string) (float64, bool, error) {
	switch typeName {
	case "string":
		return 0, false, nil
	case "boolean":
		if value != "true" && value != "false" {
			return 0, false, fmt.Errorf("%w: provider profile field %q must be a boolean", ErrInvalidExecutorConfig, key)
		}
		return 0, false, nil
	case "integer":
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, false, fmt.Errorf("%w: provider profile field %q must be an integer", ErrInvalidExecutorConfig, key)
		}
		return float64(parsed), true, nil
	case "number":
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return 0, false, fmt.Errorf("%w: provider profile field %q must be a number", ErrInvalidExecutorConfig, key)
		}
		return parsed, true, nil
	default:
		return 0, false, fmt.Errorf("%w: provider profile field %q has an unsupported type", ErrInvalidExecutorConfig, key)
	}
}

func validateExecutorProviderEnum(key, value string, allowed []any) error {
	if len(allowed) == 0 {
		return nil
	}
	for _, candidate := range allowed {
		if fmt.Sprint(candidate) == value {
			return nil
		}
	}
	return fmt.Errorf("%w: provider profile field %q is not an allowed value", ErrInvalidExecutorConfig, key)
}

func validateExecutorProviderBounds(key string, numeric float64, hasNumeric bool, field executorProviderField) error {
	if field.minimum == nil && field.maximum == nil {
		return nil
	}
	if !hasNumeric {
		return fmt.Errorf("%w: numeric bounds require a number field", ErrInvalidExecutorConfig)
	}
	if field.minimum != nil && numeric < *field.minimum || field.maximum != nil && numeric > *field.maximum {
		return fmt.Errorf("%w: provider profile field %q is outside its allowed range", ErrInvalidExecutorConfig, key)
	}
	return nil
}

func executorProfileSecretName(profileID, key string) string {
	return executorProfileSecretNamePrefix + profileID + ":" + key
}

func (s *Service) normalizePluginExecutorProfileConfig(
	ctx context.Context,
	provider models.ExecutorProvider,
	profileID string,
	current map[string]string,
	incoming map[string]string,
) (map[string]string, []string, error) {
	fields, _, err := executorProviderSchemaFields(provider.ProfileSchema)
	if err != nil {
		return nil, nil, err
	}
	config := make(map[string]string, len(current)+len(incoming))
	for key, value := range current {
		config[key] = value
	}
	var createdSecrets []string
	for key, value := range incoming {
		if _, kandevKey := kandevExecutorProfileKeys[key]; kandevKey {
			if value == "" {
				delete(config, key)
			} else {
				config[key] = value
			}
			continue
		}
		field, exists := fields[key]
		if !exists {
			return nil, createdSecrets, fmt.Errorf("%w: undeclared provider profile field %q", ErrInvalidExecutorConfig, key)
		}
		created, err := s.normalizePluginExecutorProfileField(ctx, profileID, key, value, field, config)
		if err != nil {
			return nil, createdSecrets, err
		}
		createdSecrets = append(createdSecrets, created...)
	}
	if err := validatePluginExecutorProfileSchema(provider, config); err != nil {
		return nil, createdSecrets, err
	}
	return config, createdSecrets, nil
}

func (s *Service) normalizePluginExecutorProfileField(
	ctx context.Context,
	profileID string,
	key string,
	value string,
	field executorProviderField,
	config map[string]string,
) ([]string, error) {
	if field.secret {
		return s.normalizePluginExecutorSecretField(ctx, profileID, key, value, config)
	}
	if _, isReference := models.ExecutorProfileSecretID(value); isReference {
		return nil, fmt.Errorf("%w: vault references are allowed only for declared secret fields", ErrInvalidExecutorConfig)
	}
	if err := validateExecutorProviderScalar(key, value, field); err != nil {
		return nil, err
	}
	config[key] = value
	return nil, nil
}

func (s *Service) normalizePluginExecutorSecretField(ctx context.Context, profileID, key, value string, config map[string]string) ([]string, error) {
	if oldID, found := models.ExecutorProfileSecretID(config[key]); found {
		if err := s.validateOwnedExecutorProfileSecret(ctx, profileID, key, oldID); err != nil {
			return nil, err
		}
	}
	delete(config, key)
	if value == "" {
		return nil, nil
	}
	if _, isReference := models.ExecutorProfileSecretID(value); isReference {
		return nil, fmt.Errorf("%w: secret fields accept a value, not a vault reference", ErrInvalidExecutorConfig)
	}
	if s.secretStore == nil {
		return nil, fmt.Errorf("%w: secret vault is unavailable", ErrInvalidExecutorConfig)
	}
	secret := &secrets.SecretWithValue{
		Secret: secrets.Secret{Name: executorProfileSecretName(profileID, key), Scope: secrets.ScopeGlobal},
		Value:  value,
	}
	if err := s.secretStore.Create(ctx, secret); err != nil {
		return nil, fmt.Errorf("%w: cannot store provider secret", ErrInvalidExecutorConfig)
	}
	config[key] = models.ExecutorProfileSecretReference(secret.ID)
	return []string{secret.ID}, nil
}

func (s *Service) validateOwnedExecutorProfileSecret(ctx context.Context, profileID, key, secretID string) error {
	if s.secretStore == nil {
		return fmt.Errorf("%w: secret vault is unavailable", ErrInvalidExecutorConfig)
	}
	secret, err := s.secretStore.Get(ctx, secretID)
	if err != nil || secret == nil || secret.Scope != secrets.ScopeGlobal || secret.Name != executorProfileSecretName(profileID, key) {
		return fmt.Errorf("%w: provider profile secret reference is invalid", ErrInvalidExecutorConfig)
	}
	return nil
}

func (s *Service) executorProviderProfileSecrets(ctx context.Context, profile *models.ExecutorProfile, provider models.ExecutorProvider) (map[string]string, error) {
	fields, _, err := executorProviderSchemaFields(provider.ProfileSchema)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for key, field := range fields {
		if !field.secret {
			continue
		}
		secretID, configured := models.ExecutorProfileSecretID(profile.Config[key])
		if !configured {
			continue
		}
		if err := s.validateOwnedExecutorProfileSecret(ctx, profile.ID, key, secretID); err != nil {
			return nil, err
		}
		value, err := s.secretStore.Reveal(ctx, secretID)
		if err != nil {
			return nil, fmt.Errorf("%w: provider profile secret is unavailable", ErrInvalidExecutorConfig)
		}
		values[key] = value
	}
	return values, nil
}

// ExecutorProviderProfileForLaunch returns a host-authorized provider/profile
// snapshot for lifecycle dispatch. Cleartext secret values exist only in this
// transient return value and must not enter runtime metadata or inventory.
func (s *Service) ExecutorProviderProfileForLaunch(ctx context.Context, profileID, environmentID string) (*models.ExecutorProviderLaunchProfile, error) {
	profile, err := s.executors.GetExecutorProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	executor, err := s.GetExecutor(ctx, profile.ExecutorID)
	if err != nil {
		return nil, err
	}
	provider, err := availableExecutorProvider(executor)
	if err != nil {
		return nil, err
	}
	if err := validatePluginExecutorProfileSchema(*provider, profile.Config); err != nil {
		return nil, err
	}
	secretValues, err := s.executorProviderProfileSecrets(ctx, profile, *provider)
	if err != nil {
		return nil, err
	}
	secretReferences := make(map[string]string, len(secretValues))
	for key := range secretValues {
		if id, ok := models.ExecutorProfileSecretID(profile.Config[key]); ok {
			secretReferences[key] = id
		}
	}
	environment, err := s.taskEnvironments.GetTaskEnvironment(ctx, environmentID)
	if err != nil || environment == nil || environment.OwnershipGeneration <= 0 {
		return nil, fmt.Errorf("%w: plugin executor environment ownership is unavailable", ErrExecutorProviderUnavailable)
	}
	return &models.ExecutorProviderLaunchProfile{
		Provider: *provider, ProfileID: profile.ID, OwnershipGeneration: environment.OwnershipGeneration,
		Config:       publicExecutorProviderConfig(*provider, profile.Config),
		SecretValues: secretValues, SecretReferences: secretReferences,
	}, nil
}

// ExecutorProviderProfileForRecovery rehydrates only the recorded public
// configuration and same-profile vault references. A later profile edit does
// not redirect an already allocated resource.
func (s *Service) ExecutorProviderProfileForRecovery(
	ctx context.Context,
	profileID, taskID, environmentID, providerIdentity string,
	ownershipGeneration int64,
	config, secretReferences map[string]string,
) (*models.ExecutorProviderLaunchProfile, error) {
	profile, err := s.executors.GetExecutorProfile(ctx, profileID)
	if err != nil || profile == nil {
		return nil, fmt.Errorf("%w: recorded provider profile is unavailable", ErrExecutorProviderUnavailable)
	}
	executor, err := s.GetExecutor(ctx, profile.ExecutorID)
	if err != nil {
		return nil, err
	}
	provider, err := availableExecutorProvider(executor)
	if err != nil || provider.Identity != providerIdentity {
		return nil, fmt.Errorf("%w: recorded provider installation is unavailable", ErrExecutorProviderUnavailable)
	}
	environment, err := s.taskEnvironments.GetTaskEnvironment(ctx, environmentID)
	if err != nil || environment == nil || environment.TaskID != taskID || environment.OwnershipGeneration != ownershipGeneration {
		return nil, fmt.Errorf("%w: recorded environment ownership changed", ErrExecutorProviderUnavailable)
	}
	fields, required, err := executorProviderSchemaFields(provider.ProfileSchema)
	if err != nil {
		return nil, err
	}
	publicConfig := cloneExecutorProfileConfig(config)
	validationConfig := cloneExecutorProfileConfig(publicConfig)
	secretValues, err := s.loadRecordedExecutorProviderSecrets(ctx, profileID, fields, secretReferences, validationConfig)
	if err != nil {
		return nil, err
	}
	if err := validatePluginExecutorProfileSchema(*provider, validationConfig); err != nil {
		return nil, err
	}
	if err := validateRequiredRecordedExecutorProviderSecrets(fields, required, secretValues); err != nil {
		return nil, err
	}
	return &models.ExecutorProviderLaunchProfile{
		Provider: *provider, ProfileID: profileID, OwnershipGeneration: ownershipGeneration,
		Config: publicConfig, SecretValues: secretValues, SecretReferences: cloneExecutorProfileConfig(secretReferences),
	}, nil
}

func (s *Service) loadRecordedExecutorProviderSecrets(
	ctx context.Context,
	profileID string,
	fields map[string]executorProviderField,
	secretReferences map[string]string,
	validationConfig map[string]string,
) (map[string]string, error) {
	secretValues := make(map[string]string, len(secretReferences))
	for key, secretID := range secretReferences {
		field, declared := fields[key]
		if !declared || !field.secret || secretID == "" {
			return nil, fmt.Errorf("%w: recorded provider secret reference is invalid", ErrInvalidExecutorConfig)
		}
		if err := s.validateOwnedExecutorProfileSecret(ctx, profileID, key, secretID); err != nil {
			return nil, err
		}
		value, err := s.secretStore.Reveal(ctx, secretID)
		if err != nil {
			return nil, fmt.Errorf("%w: recorded provider secret is unavailable", ErrInvalidExecutorConfig)
		}
		secretValues[key] = value
		validationConfig[key] = models.ExecutorProfileSecretReference(secretID)
	}
	return secretValues, nil
}

func validateRequiredRecordedExecutorProviderSecrets(fields map[string]executorProviderField, required []string, secretValues map[string]string) error {
	for _, key := range required {
		if fields[key].secret && secretValues[key] == "" {
			return fmt.Errorf("%w: required recorded provider secret is unavailable", ErrInvalidExecutorConfig)
		}
	}
	return nil
}

func (s *Service) validatePluginExecutorProfile(ctx context.Context, profile *models.ExecutorProfile, provider models.ExecutorProvider) error {
	if err := validatePluginExecutorProfileSchema(provider, profile.Config); err != nil {
		return err
	}
	if s.executorProviderCatalog == nil || !provider.Available {
		return ErrExecutorProviderUnavailable
	}
	secretValues, err := s.executorProviderProfileSecrets(ctx, profile, provider)
	if err != nil {
		return err
	}
	fieldErrors, err := s.executorProviderCatalog.ValidateExecutorProviderProfile(ctx, provider.ExecutorID, models.ExecutorProviderProfile{
		ProfileID: profile.ID, Config: publicExecutorProviderConfig(provider, profile.Config), SecretValues: secretValues,
	})
	if err != nil {
		return fmt.Errorf("%w: provider profile validation is unavailable", ErrExecutorProviderUnavailable)
	}
	if len(fieldErrors) > 0 {
		field := fieldErrors[0]
		return fmt.Errorf("%w: provider profile field %q failed validation (%s)", ErrInvalidExecutorConfig, field.Field, field.Code)
	}
	return nil
}

func publicExecutorProviderConfig(provider models.ExecutorProvider, stored map[string]string) map[string]string {
	fields, _, _ := executorProviderSchemaFields(provider.ProfileSchema)
	config := make(map[string]string, len(stored))
	for key, value := range stored {
		if _, kandevKey := kandevExecutorProfileKeys[key]; kandevKey || fields[key].secret {
			continue
		}
		config[key] = value
	}
	return config
}

func cleanupProfileSecrets(ctx context.Context, store secrets.SecretStore, ids []string) {
	if store == nil {
		return
	}
	for _, id := range ids {
		if id != "" {
			_ = store.Delete(ctx, id)
		}
	}
}

func collectProfileSecretIDs(config map[string]string) []string {
	var ids []string
	for _, value := range config {
		if id, ok := models.ExecutorProfileSecretID(value); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *Service) retainedPluginExecutorReferences(ctx context.Context, profileID string, secretIDs []string) (bool, map[string]bool, error) {
	records, err := s.executors.ListExecutorsRunning(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("read retained plugin executor references: %w", err)
	}
	wantedSecrets := make(map[string]struct{}, len(secretIDs))
	for _, id := range secretIDs {
		if id != "" {
			wantedSecrets[id] = struct{}{}
		}
	}
	usedSecrets := make(map[string]bool, len(wantedSecrets))
	profileInUse := false
	for _, record := range records {
		if record == nil || record.Runtime != agentruntime.RuntimePluginRemote {
			continue
		}
		encoded, err := json.Marshal(record.Metadata)
		if err != nil {
			return false, nil, fmt.Errorf("encode retained plugin executor inventory: %w", err)
		}
		var metadata struct {
			PluginExecutor struct {
				ProfileID        string            `json:"profile_id"`
				Phase            string            `json:"phase"`
				SecretReferences map[string]string `json:"secret_references"`
			} `json:"plugin_executor"`
		}
		if err := json.Unmarshal(encoded, &metadata); err != nil || metadata.PluginExecutor.ProfileID == "" {
			return false, nil, fmt.Errorf("decode retained plugin executor inventory for session %s", record.SessionID)
		}
		phase := strings.TrimSpace(metadata.PluginExecutor.Phase)
		if phase == "absent" || phase == "expired" || metadata.PluginExecutor.ProfileID != profileID {
			continue
		}
		profileInUse = true
		for _, id := range metadata.PluginExecutor.SecretReferences {
			if _, found := wantedSecrets[id]; found {
				usedSecrets[id] = true
			}
		}
	}
	return profileInUse, usedSecrets, nil
}

func (s *Service) executorProfileSecretIDs(ctx context.Context, profile *models.ExecutorProfile) ([]string, error) {
	ids := collectProfileSecretIDs(profile.Config)
	if s.secretStore == nil {
		return ids, nil
	}
	items, err := s.secretStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list provider profile secrets: %w", err)
	}
	prefix := executorProfileSecretNamePrefix + profile.ID + ":"
	for _, item := range items {
		if item != nil && strings.HasPrefix(item.Name, prefix) {
			ids = append(ids, item.ID)
		}
	}
	return ids, nil
}

func newExecutorProfileID() string { return uuid.NewString() }

func hasPluginProfileLocalOptions(prepareScript, cleanupScript string, envVars []models.ProfileEnvVar) bool {
	return prepareScript != "" || cleanupScript != "" || len(envVars) > 0
}

func hasPluginProfileLocalUpdateOptions(req *UpdateExecutorProfileRequest) bool {
	return (req.PrepareScript != nil && *req.PrepareScript != "") ||
		(req.CleanupScript != nil && *req.CleanupScript != "") ||
		len(req.EnvVars) > 0
}

func applyPluginExecutorProfileUpdate(profile *models.ExecutorProfile, req *UpdateExecutorProfileRequest) {
	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.McpPolicy != nil {
		profile.McpPolicy = *req.McpPolicy
	}
	if req.PrepareScript != nil {
		profile.PrepareScript = *req.PrepareScript
	}
	if req.CleanupScript != nil {
		profile.CleanupScript = *req.CleanupScript
	}
	if req.EnvVars != nil {
		profile.EnvVars = req.EnvVars
	}
}

func (s *Service) createPluginExecutorProfile(ctx context.Context, executor *models.Executor, req *CreateExecutorProfileRequest) (*models.ExecutorProfile, error) {
	provider, err := availableExecutorProvider(executor)
	if err != nil {
		return nil, err
	}
	if hasPluginProfileLocalOptions(req.PrepareScript, req.CleanupScript, req.EnvVars) {
		return nil, fmt.Errorf("%w: provider profiles accept schema fields only", ErrInvalidExecutorConfig)
	}
	profile := &models.ExecutorProfile{
		ID: newExecutorProfileID(), ExecutorID: executor.ID, Name: req.Name,
		McpPolicy: req.McpPolicy, PrepareScript: "", CleanupScript: "",
	}
	config, created, err := s.normalizePluginExecutorProfileConfig(ctx, *provider, profile.ID, nil, req.Config)
	if err != nil {
		cleanupProfileSecrets(ctx, s.secretStore, created)
		return nil, err
	}
	profile.Config = config
	if err := s.validatePluginExecutorProfile(ctx, profile, *provider); err != nil {
		cleanupProfileSecrets(ctx, s.secretStore, created)
		return nil, err
	}
	if err := s.executors.CreateExecutorProfile(ctx, profile); err != nil {
		cleanupProfileSecrets(ctx, s.secretStore, created)
		return nil, err
	}
	s.publishExecutorProfileEvent(ctx, events.ExecutorProfileCreated, profile)
	return profile, nil
}

func (s *Service) updatePluginExecutorProfile(ctx context.Context, profile *models.ExecutorProfile, executor *models.Executor, req *UpdateExecutorProfileRequest) (*models.ExecutorProfile, error) {
	provider, err := availableExecutorProvider(executor)
	if err != nil {
		return nil, err
	}
	if hasPluginProfileLocalUpdateOptions(req) {
		return nil, fmt.Errorf("%w: provider profiles accept schema fields only", ErrInvalidExecutorConfig)
	}
	oldConfig := cloneExecutorProfileConfig(profile.Config)
	config, created, err := s.normalizePluginExecutorProfileConfig(ctx, *provider, profile.ID, profile.Config, req.Config)
	if err != nil {
		cleanupProfileSecrets(ctx, s.secretStore, created)
		return nil, err
	}
	for key, value := range req.Config {
		if value != "" {
			continue
		}
		secretID, found := models.ExecutorProfileSecretID(oldConfig[key])
		if !found {
			continue
		}
		profileInUse, _, readErr := s.retainedPluginExecutorReferences(ctx, profile.ID, []string{secretID})
		if readErr != nil || profileInUse {
			cleanupProfileSecrets(ctx, s.secretStore, created)
			profile.Config = oldConfig
			if readErr != nil {
				return nil, readErr
			}
			return nil, ErrExecutorProfileInUse
		}
	}
	profile.Config = config
	applyPluginExecutorProfileUpdate(profile, req)
	if err := s.validatePluginExecutorProfile(ctx, profile, *provider); err != nil {
		cleanupProfileSecrets(ctx, s.secretStore, created)
		return nil, err
	}
	var updateErr error
	if req.ExpectedUpdatedAt != nil {
		updateErr = s.executors.UpdateExecutorProfileIfUnmodified(ctx, profile, *req.ExpectedUpdatedAt)
	} else {
		updateErr = s.executors.UpdateExecutorProfile(ctx, profile)
	}
	if updateErr != nil {
		cleanupProfileSecrets(ctx, s.secretStore, created)
		profile.Config = oldConfig
		return nil, updateErr
	}
	// Keep previous credentials available for an execution that loaded the old
	// profile snapshot before this update and persists inventory afterward.
	s.publishExecutorProfileEvent(ctx, events.ExecutorProfileUpdated, profile)
	return profile, nil
}

func availableExecutorProvider(executor *models.Executor) (*models.ExecutorProvider, error) {
	if executor == nil || executor.Type != models.ExecutorTypePluginRemote || executor.Provider == nil || !executor.Provider.Available {
		return nil, ErrExecutorProviderUnavailable
	}
	return executor.Provider, nil
}

func cloneExecutorProfileConfig(config map[string]string) map[string]string {
	if config == nil {
		return nil
	}
	clone := make(map[string]string, len(config))
	for key, value := range config {
		clone[key] = value
	}
	return clone
}
