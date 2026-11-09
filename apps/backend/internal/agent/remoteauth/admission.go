package remoteauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/profileenv"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	methodTypeEnv   = "env"
	methodTypeFiles = "files"
)

// ErrAdmissionRefused is the stable class for a remote authentication
// admission failure. Callers may map it to a domain-specific workflow error;
// the wrapped text never contains credential values or secret identifiers.
var ErrAdmissionRefused = errors.New("remote authentication admission refused")

// AdmissionInput carries the authorized settings needed to prove that a
// remote executor has at least one usable authentication route.
type AdmissionInput struct {
	AgentID         string
	ProfileEnvVars  []models.ProfileEnvVar
	ExecutorEnvVars []models.ProfileEnvVar
	ExecutorConfig  map[string]string
	Catalog         Catalog
	SecretStore     secrets.SecretStore
}

// ValidateCredentialAdmission checks the selected agent profile and executor
// profile without resolving host credentials or contacting the provider.
//
// Every required environment binding is resolved and every explicitly
// configured authentication binding for the selected agent is validated.
// After those checks, at least one file route or nonempty catalog env route
// must remain.
func ValidateCredentialAdmission(ctx context.Context, input AdmissionInput) error {
	if err := admissionCanceled(ctx); err != nil {
		return err
	}
	spec, ok := specForAgent(input.Catalog, input.AgentID)
	if !ok {
		return refuse("agent family has no remote credential specification")
	}
	if len(spec.Methods) == 0 {
		return nil
	}
	profileEnvVars := admissionEnvVars(input.ProfileEnvVars)
	executorEnvVars := admissionEnvVars(input.ExecutorEnvVars)
	if err := profileenv.Validate(profileEnvVars); err != nil {
		return refuse("invalid agent profile environment: " + err.Error())
	}
	if err := profileenv.Validate(executorEnvVars); err != nil {
		return refuse("invalid executor profile environment: " + err.Error())
	}

	configuredFiles, configuredSecrets, err := decodeSelectedCredentials(input.ExecutorConfig)
	if err != nil {
		return err
	}
	if err := validateSelectedMethodReferences(spec, configuredFiles, configuredSecrets); err != nil {
		return err
	}

	definitions := admissionDefinitions(profileEnvVars, executorEnvVars)
	if err := runtimeenv.Validate(definitions); err != nil {
		return refuse("invalid effective environment: " + err.Error())
	}
	if needsCredentialReads(definitions, configuredSecrets) {
		if !credentialActorAvailable(ctx) {
			return refuse("current actor is unavailable for credential admission")
		}
		if input.SecretStore == nil {
			return refuse("secret store is unavailable")
		}
	}
	if err := validateAuthenticationBindings(ctx, spec, configuredSecrets, input.SecretStore); err != nil {
		return err
	}

	resolved, err := resolveAdmissionEnvironment(ctx, definitions, input.SecretStore)
	if err != nil {
		return err
	}
	if !hasAdmissionRoute(spec, configuredFiles, configuredSecrets, resolved) {
		return refuse("no usable authentication route is configured for the selected agent")
	}
	return admissionCanceled(ctx)
}

func refuse(reason string) error {
	return fmt.Errorf("%w: %s", ErrAdmissionRefused, reason)
}

func admissionCanceled(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: admission canceled: %w", ErrAdmissionRefused, err)
	}
	return nil
}

func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func specForAgent(catalog Catalog, agentID string) (*Spec, bool) {
	for i := range catalog.Specs {
		if catalog.Specs[i].ID == agentID {
			return &catalog.Specs[i], true
		}
	}
	return nil, false
}

func decodeSelectedCredentials(config map[string]string) ([]string, map[string]string, error) {
	files, err := decodeRemoteCredentialIDs(config["remote_credentials"])
	if err != nil {
		return nil, nil, refuse("invalid remote credential selection")
	}
	secretsByMethod, err := decodeRemoteCredentialSecrets(config["remote_auth_secrets"])
	if err != nil {
		return nil, nil, refuse("invalid remote authentication selection")
	}
	return files, secretsByMethod, nil
}

func validateSelectedMethodReferences(spec *Spec, configuredFiles []string, configuredSecrets map[string]string) error {
	methods := make(map[string]Method, len(spec.Methods))
	for _, method := range spec.Methods {
		methods[method.MethodID] = method
	}
	selectedPrefix := "agent:" + spec.ID + ":"
	for _, rawID := range configuredFiles {
		if rawID == "" {
			continue
		}
		if rawID != strings.TrimSpace(rawID) {
			return refuse("invalid remote credential selection")
		}
		if _, known := methods[rawID]; !known && strings.HasPrefix(rawID, selectedPrefix) {
			return refuse("unknown authentication method for the selected agent")
		}
	}
	for rawMethodID, rawSecretID := range configuredSecrets {
		if rawMethodID == "" {
			return refuse("invalid authentication method selection")
		}
		if rawMethodID != strings.TrimSpace(rawMethodID) {
			return refuse("invalid authentication method selection")
		}
		if rawSecretID != "" && rawSecretID != strings.TrimSpace(rawSecretID) {
			return refuse("invalid authentication binding")
		}
		if _, known := methods[rawMethodID]; !known && strings.HasPrefix(rawMethodID, selectedPrefix) {
			return refuse("unknown authentication method for the selected agent")
		}
	}
	return nil
}

// admissionEnvVars removes declarations that the strict launch resolver also
// skips: blank keys and entries with neither a literal nor a secret value.
func admissionEnvVars(entries []models.ProfileEnvVar) []models.ProfileEnvVar {
	filtered := make([]models.ProfileEnvVar, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.Key) == "" || (entry.Value == "" && entry.SecretID == "") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func admissionDefinitions(profileEnvVars, executorEnvVars []models.ProfileEnvVar) []runtimeenv.Definition {
	definitions := make([]runtimeenv.Definition, 0, len(profileEnvVars)+len(executorEnvVars))
	definitions = appendProfileDefinitions(definitions, profileEnvVars, runtimeenv.OriginAgentProfile)
	return appendProfileDefinitions(definitions, executorEnvVars, runtimeenv.OriginExecutorProfile)
}

func appendProfileDefinitions(definitions []runtimeenv.Definition, entries []models.ProfileEnvVar, origin string) []runtimeenv.Definition {
	for _, entry := range entries {
		definitions = append(definitions, runtimeenv.Definition{
			Key:      entry.Key,
			Literal:  entry.Value,
			SecretID: entry.SecretID,
			Origin:   origin,
		})
	}
	return definitions
}

func needsCredentialReads(definitions []runtimeenv.Definition, configuredSecrets map[string]string) bool {
	for _, definition := range definitions {
		if definition.SecretID != "" {
			return true
		}
	}
	for _, secretID := range configuredSecrets {
		if secretID != "" {
			return true
		}
	}
	return false
}

func credentialActorAvailable(ctx context.Context) bool {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		return false
	}
	return identity.Synthetic || strings.TrimSpace(identity.UserID) != ""
}

func validateAuthenticationBindings(ctx context.Context, spec *Spec, configuredSecrets map[string]string, store secrets.SecretStore) error {
	if len(configuredSecrets) == 0 {
		return nil
	}
	methods := make(map[string]Method, len(spec.Methods))
	for _, method := range spec.Methods {
		methods[method.MethodID] = method
	}
	for methodID, secretID := range configuredSecrets {
		if secretID == "" {
			continue
		}
		method, known := methods[methodID]
		if !known || method.Type != methodTypeEnv {
			continue
		}
		if secretID != strings.TrimSpace(secretID) {
			return refuse("invalid authentication binding")
		}
		if store == nil {
			return refuse("secret store is unavailable")
		}
		if err := secrets.ValidateGlobalReference(ctx, store, secretID); err != nil {
			if isCancellation(err) {
				return fmt.Errorf("%w: authentication binding check canceled: %w", ErrAdmissionRefused, err)
			}
			return refuse("configured authentication binding is not usable")
		}
		value, err := revealGlobalSecret(ctx, store, secretID)
		if err != nil {
			if isCancellation(err) {
				return fmt.Errorf("%w: authentication binding check canceled: %w", ErrAdmissionRefused, err)
			}
			return refuse("configured authentication binding is empty or unreadable")
		}
		if strings.TrimSpace(value) == "" {
			return refuse("configured authentication binding is empty or unreadable")
		}
	}
	return nil
}

func resolveAdmissionEnvironment(ctx context.Context, definitions []runtimeenv.Definition, store secrets.SecretStore) (map[string]string, error) {
	if len(definitions) == 0 {
		return map[string]string{}, nil
	}
	resolved, _, err := runtimeenv.Resolve(ctx, definitions, func(ctx context.Context, definition runtimeenv.Definition) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("admission canceled: %w", err)
		}
		value, err := revealGlobalSecret(ctx, store, definition.SecretID)
		if err != nil {
			return "", err
		}
		if definition.SecretID != "" && strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("secret value is empty")
		}
		return value, nil
	})
	if err != nil {
		if isCancellation(err) {
			return nil, fmt.Errorf("%w: required environment binding check canceled: %w", ErrAdmissionRefused, err)
		}
		return nil, refuse("required environment binding is unavailable: " + err.Error())
	}
	return resolved, nil
}

func revealGlobalSecret(ctx context.Context, store secrets.SecretStore, id string) (string, error) {
	if store == nil {
		return "", fmt.Errorf("secret store is unavailable")
	}
	if scoped, ok := store.(secrets.ScopedSecretStore); ok {
		return scoped.RevealGlobal(ctx, id)
	}
	if err := secrets.ValidateGlobalReference(ctx, store, id); err != nil {
		return "", err
	}
	return store.Reveal(ctx, id)
}

func hasAdmissionRoute(
	spec *Spec,
	configuredFiles []string,
	configuredSecrets map[string]string,
	resolved map[string]string,
) bool {
	fileSelection := make(map[string]struct{}, len(configuredFiles))
	for _, methodID := range configuredFiles {
		fileSelection[methodID] = struct{}{}
	}
	for _, method := range spec.Methods {
		switch method.Type {
		case methodTypeEnv:
			if strings.TrimSpace(resolved[method.EnvVar]) != "" {
				return true
			}
			if configuredSecrets[method.MethodID] != "" {
				return true
			}
		case methodTypeFiles:
			if _, ok := fileSelection[method.MethodID]; ok {
				return true
			}
		}
	}
	return false
}

func decodeRemoteCredentialIDs(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func decodeRemoteCredentialSecrets(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var values map[string]*string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		if value != nil {
			result[key] = *value
		}
	}
	return result, nil
}
