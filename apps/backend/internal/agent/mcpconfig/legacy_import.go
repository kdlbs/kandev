package mcpconfig

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

const (
	LegacyImportStatusPending  = "pending"
	LegacyImportStatusComplete = "complete"
)

var (
	ErrMCPLegacyImportStateNotFound  = errors.New("legacy MCP import state not found")
	ErrMCPLegacyImportRequiresRebind = errors.New("legacy MCP secrets require workspace rebind")
	ErrMCPLegacyImportFailed         = errors.New("legacy MCP import failed")
)

// LegacyMCPConfigReader is implemented by the existing raw profile MCP
// service. Keeping it as a narrow interface lets the compatibility importer
// run without making the catalog depend on legacy storage.
type LegacyMCPConfigReader interface {
	GetConfigByProfileID(context.Context, string) (*ProfileConfig, error)
}

// LegacyProfileWorkspaceLister returns workspaces that use a profile. Global
// profiles may have more than one workspace binding, so the workspace ID is
// intentionally part of the import key.
type LegacyProfileWorkspaceLister interface {
	ListMCPProfileWorkspaces(context.Context, string) ([]string, error)
}

type LegacyImportState struct {
	WorkspaceID   string    `json:"workspace_id"`
	ProfileID     string    `json:"profile_id"`
	Status        string    `json:"status"`
	FailureCode   string    `json:"failure_code,omitempty"`
	FailureReason string    `json:"failure_reason,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type LegacyImportStateRepository interface {
	GetMCPImportState(context.Context, string, string) (LegacyImportState, error)
	SaveMCPImportState(context.Context, LegacyImportState) error
}

// AtomicLegacyMCPImportRepository is the settings-store fast path. It commits
// new definitions, the profile selection, and the import marker together.
type AtomicLegacyMCPImportRepository interface {
	ImportLegacyMCPProfileWorkspace(context.Context, string, string, []*MCPServerDefinition, []string, LegacyImportState) error
}

type LegacyImportResult struct {
	WorkspaceID   string
	ProfileID     string
	DefinitionIDs []string
	Complete      bool
	Fallback      bool
	FailureCode   string
}

type LegacyImporter struct {
	reader     LegacyMCPConfigReader
	workspaces LegacyProfileWorkspaceLister
	catalog    *CatalogService
	selections *SelectionService
	states     LegacyImportStateRepository
}

func NewLegacyImporter(
	reader LegacyMCPConfigReader,
	workspaces LegacyProfileWorkspaceLister,
	catalog *CatalogService,
	selections *SelectionService,
	states LegacyImportStateRepository,
) *LegacyImporter {
	return &LegacyImporter{
		reader: reader, workspaces: workspaces, catalog: catalog,
		selections: selections, states: states,
	}
}

// ImportProfile imports every workspace binding for one legacy profile.
func (i *LegacyImporter) ImportProfile(ctx context.Context, profileID string) ([]LegacyImportResult, error) {
	if i.workspaces == nil {
		return nil, fmt.Errorf("%w: workspace lister is unavailable", ErrMCPLegacyImportFailed)
	}
	workspaceIDs, err := i.workspaces.ListMCPProfileWorkspaces(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("%w: list profile workspaces: %v", ErrMCPLegacyImportFailed, err)
	}
	workspaceIDs = uniqueSortedStrings(workspaceIDs)
	results := make([]LegacyImportResult, 0, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		result, importErr := i.ImportProfileWorkspace(ctx, workspaceID, profileID)
		results = append(results, result)
		if importErr != nil {
			return results, importErr
		}
	}
	return results, nil
}

// ImportProfileWorkspace is idempotent. The complete marker is written only
// after all definitions and the profile selection have succeeded.
func (i *LegacyImporter) ImportProfileWorkspace(ctx context.Context, workspaceID, profileID string) (LegacyImportResult, error) {
	result := LegacyImportResult{WorkspaceID: workspaceID, ProfileID: profileID}
	if err := i.validateImport(workspaceID, profileID); err != nil {
		return result, err
	}
	complete, err := i.importComplete(ctx, workspaceID, profileID)
	if err != nil {
		return result, err
	}
	if complete {
		result.Complete = true
		return result, nil
	}
	inputs, definitionIDs, unsafeSecrets, err := i.readLegacyInputs(ctx, workspaceID, profileID)
	if err != nil {
		return i.fail(ctx, result, "invalid_legacy_server", err)
	}
	result.DefinitionIDs = append([]string(nil), definitionIDs...)
	if unsafeSecrets {
		return i.persistUnsafeImport(ctx, result, workspaceID, inputs)
	}
	failureCode, err := i.persistSafeImport(ctx, workspaceID, profileID, inputs, definitionIDs)
	if err != nil {
		return i.fail(ctx, result, failureCode, err)
	}
	result.Complete = true
	return result, nil
}

func (i *LegacyImporter) validateImport(workspaceID, profileID string) error {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(profileID) == "" {
		return fmt.Errorf("%w: workspace and profile are required", ErrMCPLegacyImportFailed)
	}
	if i.reader == nil || i.catalog == nil || i.selections == nil {
		return fmt.Errorf("%w: importer dependencies are unavailable", ErrMCPLegacyImportFailed)
	}
	return nil
}

func (i *LegacyImporter) importComplete(ctx context.Context, workspaceID, profileID string) (bool, error) {
	if i.states == nil {
		return false, nil
	}
	state, err := i.states.GetMCPImportState(ctx, workspaceID, profileID)
	if err == nil {
		return state.Status == LegacyImportStatusComplete, nil
	}
	if errors.Is(err, ErrMCPLegacyImportStateNotFound) {
		return false, nil
	}
	return false, err
}

func (i *LegacyImporter) readLegacyInputs(
	ctx context.Context,
	workspaceID, profileID string,
) ([]CreateDefinitionInput, []string, bool, error) {
	config, err := i.reader.GetConfigByProfileID(ctx, profileID)
	if err != nil {
		return nil, nil, false, err
	}
	if config == nil {
		config = &ProfileConfig{}
	}
	return legacyDefinitionInputs(workspaceID, profileID, config)
}

func (i *LegacyImporter) persistUnsafeImport(
	ctx context.Context,
	result LegacyImportResult,
	workspaceID string,
	inputs []CreateDefinitionInput,
) (LegacyImportResult, error) {
	if err := i.persistDefinitions(ctx, workspaceID, inputs); err != nil {
		return i.fail(ctx, result, "persist_definition", err)
	}
	result.Fallback = true
	return i.fail(ctx, result, "secret_rebind_required", ErrMCPLegacyImportRequiresRebind)
}

func (i *LegacyImporter) persistSafeImport(
	ctx context.Context,
	workspaceID, profileID string,
	inputs []CreateDefinitionInput,
	definitionIDs []string,
) (string, error) {
	if atomicRepo, ok := i.selections.Repository().(AtomicLegacyMCPImportRepository); ok {
		definitions, err := i.prepareDefinitions(ctx, workspaceID, inputs)
		if err != nil {
			return "prepare_definition", err
		}
		state := completeLegacyImportState(workspaceID, profileID)
		if err := atomicRepo.ImportLegacyMCPProfileWorkspace(ctx, workspaceID, profileID, definitions, definitionIDs, state); err != nil {
			return "persist_import", err
		}
		return "", nil
	}
	if err := i.persistDefinitions(ctx, workspaceID, inputs); err != nil {
		return "persist_definition", err
	}
	if err := i.selections.Replace(ctx, SelectionScopeProfile, workspaceID, profileID, definitionIDs); err != nil {
		return "persist_selection", err
	}
	if err := i.saveCompleteImportState(ctx, workspaceID, profileID); err != nil {
		return "persist_import_state", err
	}
	return "", nil
}

func completeLegacyImportState(workspaceID, profileID string) LegacyImportState {
	return LegacyImportState{
		WorkspaceID: workspaceID, ProfileID: profileID,
		Status: LegacyImportStatusComplete,
	}
}

func (i *LegacyImporter) saveCompleteImportState(ctx context.Context, workspaceID, profileID string) error {
	if i.states == nil {
		return nil
	}
	return i.states.SaveMCPImportState(ctx, completeLegacyImportState(workspaceID, profileID))
}

func (i *LegacyImporter) prepareDefinitions(
	ctx context.Context,
	workspaceID string,
	inputs []CreateDefinitionInput,
) ([]*MCPServerDefinition, error) {
	if err := i.catalog.authorize(ctx, workspaceID); err != nil {
		return nil, err
	}
	definitions := make([]*MCPServerDefinition, 0, len(inputs))
	seenNames := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if existing, err := i.catalog.Get(ctx, workspaceID, input.ID); err == nil && existing != nil {
			continue
		} else if err != nil && !errors.Is(err, ErrMCPServerDefinitionNotFound) {
			return nil, err
		}
		definition, err := newDefinition(input)
		if err != nil {
			return nil, err
		}
		if _, exists := seenNames[definition.NormalizedRuntimeName]; exists {
			return nil, ErrMCPRuntimeNameConflict
		}
		seenNames[definition.NormalizedRuntimeName] = struct{}{}
		if err := i.catalog.ensureRuntimeNameAvailable(ctx, workspaceID, definition.NormalizedRuntimeName, definition.ID); err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func (i *LegacyImporter) persistDefinitions(ctx context.Context, workspaceID string, inputs []CreateDefinitionInput) error {
	for _, input := range inputs {
		existing, err := i.catalog.Get(ctx, workspaceID, input.ID)
		if err == nil && existing != nil {
			continue
		}
		if err != nil && !errors.Is(err, ErrMCPServerDefinitionNotFound) {
			return err
		}
		if _, err := i.catalog.Create(ctx, input); err != nil {
			if errors.Is(err, ErrMCPRuntimeNameConflict) {
				if existing, getErr := i.catalog.Get(ctx, workspaceID, input.ID); getErr == nil && existing != nil {
					continue
				}
			}
			return err
		}
	}
	return nil
}

func (i *LegacyImporter) fail(ctx context.Context, result LegacyImportResult, code string, cause error) (LegacyImportResult, error) {
	result.FailureCode = code
	if i.states != nil {
		state := LegacyImportState{
			WorkspaceID: result.WorkspaceID, ProfileID: result.ProfileID,
			Status: LegacyImportStatusPending, FailureCode: code,
			FailureReason: code,
		}
		if err := i.states.SaveMCPImportState(ctx, state); err != nil {
			return result, fmt.Errorf("%w: %v; save state: %v", ErrMCPLegacyImportFailed, cause, err)
		}
	}
	return result, fmt.Errorf("%w (%s): %w", ErrMCPLegacyImportFailed, code, cause)
}

func legacyDefinitionInputs(workspaceID, profileID string, config *ProfileConfig) ([]CreateDefinitionInput, []string, bool, error) {
	names := make([]string, 0, len(config.Servers))
	for name := range config.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	inputs := make([]CreateDefinitionInput, 0, len(names))
	ids := make([]string, 0, len(names))
	unsafeSecrets := false
	for _, name := range names {
		input, unsafe, err := legacyDefinitionInput(workspaceID, profileID, name, config.Servers[name])
		if err != nil {
			return nil, nil, false, err
		}
		inputs = append(inputs, input)
		ids = append(ids, input.ID)
		unsafeSecrets = unsafeSecrets || unsafe
	}
	return inputs, ids, unsafeSecrets, nil
}

func legacyDefinitionInput(workspaceID, profileID, name string, server ServerDef) (CreateDefinitionInput, bool, error) {
	runtimeName := legacyRuntimeName(name)
	if runtimeName == "" {
		return CreateDefinitionInput{}, false, fmt.Errorf("legacy MCP server name is empty")
	}
	serverType := server.Type
	if serverType == "" {
		serverType = ServerTypeStdio
		if server.URL != "" {
			serverType = ServerTypeHTTP
		}
	}
	mode := ExecutionModeExistingExecutable
	if serverType != ServerTypeStdio {
		mode = ExecutionModeRemote
	}
	configuration := MCPServerConfiguration{
		Command: server.Command, Args: append([]string(nil), server.Args...),
		URL: server.URL,
		Env: map[string]string{}, Headers: map[string]string{},
	}
	bindings := make([]MCPSecretBinding, 0)
	unsafeSecrets := false
	for key, value := range server.Env {
		if value == "" {
			configuration.Env[key] = value
			continue
		}
		unsafeSecrets = true
		bindings = append(bindings, MCPSecretBinding{
			InputName: "env:" + key,
			SecretID:  legacySecretID(workspaceID, profileID, name, "env:"+key),
		})
	}
	for key, value := range server.Headers {
		if value == "" {
			configuration.Headers[key] = value
			continue
		}
		unsafeSecrets = true
		bindings = append(bindings, MCPSecretBinding{
			InputName: "header:" + key,
			SecretID:  legacySecretID(workspaceID, profileID, name, "header:"+key),
		})
	}
	sort.Slice(bindings, func(left, right int) bool { return bindings[left].InputName < bindings[right].InputName })
	enabled := true
	return CreateDefinitionInput{
		ID:          workspaceProfileServerID(workspaceID, profileID, name),
		WorkspaceID: workspaceID, RuntimeName: runtimeName,
		DisplayName: strings.TrimSpace(name), Enabled: &enabled,
		ExecutionMode: mode, Transport: serverType, Configuration: configuration,
		SecretBindings: bindings, Source: DefinitionSourceLegacyImport,
		SourceIdentity: "legacy:" + profileID + ":" + name,
	}, unsafeSecrets, nil
}

func workspaceProfileServerID(workspaceID, profileID, serverName string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev:legacy-mcp:"+workspaceID+":"+profileID+":"+serverName)).String()
}

func legacySecretID(workspaceID, profileID, serverName, input string) string {
	return "legacy-" + uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev:legacy-secret:"+workspaceID+":"+profileID+":"+serverName+":"+input)).String()
}

func legacyRuntimeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, character := range value {
		switch {
		case unicode.IsLetter(character), unicode.IsDigit(character), character == '.', character == '-', character == '_':
			builder.WriteRune(character)
		case unicode.IsSpace(character):
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-._")
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
