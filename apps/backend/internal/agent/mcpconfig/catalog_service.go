package mcpconfig

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

var (
	ErrMCPServerDefinitionNotFound = errors.New("mcp server definition not found")
	ErrMCPRuntimeNameReserved      = errors.New("mcp runtime name is reserved")
	ErrMCPRuntimeNameConflict      = errors.New("mcp runtime name already exists")
	ErrMCPInvalidDefinition        = errors.New("invalid mcp server definition")
	ErrMCPDeleteConfirmation       = errors.New("mcp server deletion requires confirmation")
	ErrMCPRevisionConflict         = errors.New("mcp server definition revision conflict")
	ErrMCPWorkspaceAccess          = errors.New("mcp workspace access denied")
	ErrMCPDefinitionSelected       = errors.New("mcp server definition is selected")
)

// CatalogRepository stores workspace-owned MCP definitions.
type CatalogRepository interface {
	ListMCPServerDefinitions(context.Context, string) ([]*MCPServerDefinition, error)
	GetMCPServerDefinition(context.Context, string, string) (*MCPServerDefinition, error)
	CreateMCPServerDefinition(context.Context, *MCPServerDefinition) error
	UpdateMCPServerDefinition(context.Context, *MCPServerDefinition, int64) error
	DeleteMCPServerDefinition(context.Context, string, string, int64) error
}

// AtomicMCPDefinitionDeletion removes a definition and every typed selection
// that points to it in one database transaction. Stores without this optional
// extension can still use the service-level cleanup fallback.
type AtomicMCPDefinitionDeletion interface {
	DeleteMCPServerDefinitionWithSelections(context.Context, string, string, int64) error
}

// MCPRevisionConflictError describes a stale optimistic-concurrency write.
type MCPRevisionConflictError struct {
	Current *MCPServerDefinition
}

func (e *MCPRevisionConflictError) Error() string { return ErrMCPRevisionConflict.Error() }

func (e *MCPRevisionConflictError) Unwrap() error { return ErrMCPRevisionConflict }

// CatalogService validates and persists MCP definitions without materializing them.
type CatalogService struct {
	repo          CatalogRepository
	authorizer    WorkspaceAuthorizer
	selectionRepo SelectionRepository
}

func NewCatalogService(repo CatalogRepository) *CatalogService {
	return &CatalogService{repo: repo}
}

func (s *CatalogService) SetWorkspaceAuthorizer(authorizer WorkspaceAuthorizer) {
	s.authorizer = authorizer
}

// SetSelectionRepository enables selection-impact checks and guarded cleanup
// when a catalog definition is deleted.
func (s *CatalogService) SetSelectionRepository(repo SelectionRepository) {
	s.selectionRepo = repo
}

func (s *CatalogService) List(ctx context.Context, workspaceID string) ([]*MCPServerDefinition, error) {
	if err := s.authorize(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.ListMCPServerDefinitions(ctx, workspaceID)
}

func (s *CatalogService) Get(ctx context.Context, workspaceID, id string) (*MCPServerDefinition, error) {
	if err := s.authorize(ctx, workspaceID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(id) == "" {
		return nil, ErrMCPServerDefinitionNotFound
	}
	definition, err := s.repo.GetMCPServerDefinition(ctx, workspaceID, id)
	if errors.Is(err, ErrMCPServerDefinitionNotFound) || definition == nil {
		return nil, ErrMCPServerDefinitionNotFound
	}
	if err != nil {
		return nil, err
	}
	return cloneDefinition(definition), nil
}

func (s *CatalogService) Create(ctx context.Context, input CreateDefinitionInput) (*MCPServerDefinition, error) {
	if err := s.authorize(ctx, input.WorkspaceID); err != nil {
		return nil, err
	}
	definition, err := newDefinition(input)
	if err != nil {
		return nil, err
	}
	if err := s.ensureRuntimeNameAvailable(ctx, definition.WorkspaceID, definition.NormalizedRuntimeName, ""); err != nil {
		return nil, err
	}
	if err := s.repo.CreateMCPServerDefinition(ctx, definition); err != nil {
		return nil, err
	}
	return cloneDefinition(definition), nil
}

func (s *CatalogService) Update(ctx context.Context, input UpdateDefinitionInput) (*MCPServerDefinition, error) {
	if err := s.authorize(ctx, input.WorkspaceID); err != nil {
		return nil, err
	}
	current, err := s.Get(ctx, input.WorkspaceID, input.ID)
	if err != nil {
		return nil, err
	}
	if current.Revision != input.ExpectedRevision {
		return nil, revisionConflict(current)
	}
	candidate := applyDefinitionUpdate(current, input)
	if err := validateDefinition(candidate); err != nil {
		return nil, err
	}
	if err := s.ensureRuntimeNameAvailable(ctx, candidate.WorkspaceID, candidate.NormalizedRuntimeName, candidate.ID); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateMCPServerDefinition(ctx, candidate, input.ExpectedRevision); err != nil {
		return nil, err
	}
	return cloneDefinition(candidate), nil
}

func (s *CatalogService) Delete(ctx context.Context, workspaceID, id string, expectedRevision int64, confirm bool) error {
	if err := s.authorize(ctx, workspaceID); err != nil {
		return err
	}
	current, err := s.Get(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return revisionConflict(current)
	}
	if s.selectionRepo != nil {
		impact, impactErr := s.selectionRepo.SelectionImpact(ctx, workspaceID, id)
		if impactErr != nil {
			return impactErr
		}
		if impact.Total() > 0 && !confirm {
			return &MCPSelectionImpactError{Impact: impact}
		}
	}
	if !confirm {
		return ErrMCPDeleteConfirmation
	}
	if atomic, ok := s.repo.(AtomicMCPDefinitionDeletion); ok && s.selectionRepo != nil {
		return atomic.DeleteMCPServerDefinitionWithSelections(ctx, workspaceID, id, expectedRevision)
	}
	if s.selectionRepo != nil {
		if err := s.selectionRepo.DeleteMCPSelectionsForDefinition(ctx, workspaceID, id); err != nil {
			return err
		}
	}
	return s.repo.DeleteMCPServerDefinition(ctx, workspaceID, id, expectedRevision)
}

func (s *CatalogService) authorize(ctx context.Context, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("%w: workspace id is required", ErrMCPInvalidDefinition)
	}
	if s.authorizer == nil {
		return nil
	}
	if err := s.authorizer(ctx, workspaceID); err != nil {
		return fmt.Errorf("%w: %w", ErrMCPWorkspaceAccess, err)
	}
	return nil
}

func (s *CatalogService) ensureRuntimeNameAvailable(ctx context.Context, workspaceID, normalized, excludeID string) error {
	definitions, err := s.repo.ListMCPServerDefinitions(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		if definition != nil && definition.ID != excludeID && definition.NormalizedRuntimeName == normalized {
			return ErrMCPRuntimeNameConflict
		}
	}
	return nil
}

func newDefinition(input CreateDefinitionInput) (*MCPServerDefinition, error) {
	runtimeName := strings.TrimSpace(input.RuntimeName)
	normalized := normalizeRuntimeName(runtimeName)
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	source := input.Source
	if source == "" {
		source = DefinitionSourceCustom
	}
	now := time.Now().UTC()
	definition := &MCPServerDefinition{
		ID:                    strings.TrimSpace(input.ID),
		WorkspaceID:           strings.TrimSpace(input.WorkspaceID),
		RuntimeName:           runtimeName,
		NormalizedRuntimeName: normalized,
		DisplayName:           strings.TrimSpace(input.DisplayName),
		Description:           strings.TrimSpace(input.Description),
		Enabled:               enabled,
		ExecutionMode:         input.ExecutionMode,
		Transport:             input.Transport,
		Configuration:         cloneConfiguration(input.Configuration),
		SecretBindings:        cloneSecretBindings(input.SecretBindings),
		Source:                source,
		SourceIdentity:        strings.TrimSpace(input.SourceIdentity),
		Revision:              1,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if definition.ID == "" {
		definition.ID = uuid.New().String()
	}
	if err := validateDefinition(definition); err != nil {
		return nil, err
	}
	return definition, nil
}

func applyDefinitionUpdate(current *MCPServerDefinition, input UpdateDefinitionInput) *MCPServerDefinition {
	updated := cloneDefinition(current)
	if input.RuntimeName != nil {
		updated.RuntimeName = strings.TrimSpace(*input.RuntimeName)
		updated.NormalizedRuntimeName = normalizeRuntimeName(updated.RuntimeName)
	}
	if input.DisplayName != nil {
		updated.DisplayName = strings.TrimSpace(*input.DisplayName)
	}
	if input.Description != nil {
		updated.Description = strings.TrimSpace(*input.Description)
	}
	if input.Enabled != nil {
		updated.Enabled = *input.Enabled
	}
	if input.ExecutionMode != nil {
		updated.ExecutionMode = *input.ExecutionMode
	}
	if input.Transport != nil {
		updated.Transport = *input.Transport
	}
	if input.Configuration != nil {
		updated.Configuration = cloneConfiguration(*input.Configuration)
	}
	if input.SecretBindings != nil {
		updated.SecretBindings = cloneSecretBindings(*input.SecretBindings)
	}
	if input.Source != nil {
		updated.Source = *input.Source
	}
	if input.SourceIdentity != nil {
		updated.SourceIdentity = strings.TrimSpace(*input.SourceIdentity)
	}
	updated.Revision = current.Revision + 1
	updated.UpdatedAt = time.Now().UTC()
	return updated
}

func validateDefinition(definition *MCPServerDefinition) error {
	if definition == nil || strings.TrimSpace(definition.WorkspaceID) == "" || definition.ID == "" {
		return ErrMCPInvalidDefinition
	}
	if err := validateNames(definition); err != nil {
		return err
	}
	if err := validateTransport(definition); err != nil {
		return err
	}
	if err := validateConfiguration(definition); err != nil {
		return err
	}
	return validateSecretBindings(definition.SecretBindings)
}

func validateNames(definition *MCPServerDefinition) error {
	if definition.RuntimeName == "" || len([]rune(definition.RuntimeName)) > 128 {
		return fmt.Errorf("%w: runtime name is required and must be at most 128 characters", ErrMCPInvalidDefinition)
	}
	if definition.NormalizedRuntimeName == "" || hasWhitespace(definition.RuntimeName) {
		return fmt.Errorf("%w: runtime name must not contain whitespace", ErrMCPInvalidDefinition)
	}
	if definition.NormalizedRuntimeName == "kandev" || strings.HasPrefix(definition.NormalizedRuntimeName, "kandev.") {
		return ErrMCPRuntimeNameReserved
	}
	if definition.DisplayName == "" || len([]rune(definition.DisplayName)) > 200 {
		return fmt.Errorf("%w: display name is required and must be at most 200 characters", ErrMCPInvalidDefinition)
	}
	if len([]rune(definition.Description)) > 5000 {
		return fmt.Errorf("%w: description is too long", ErrMCPInvalidDefinition)
	}
	return nil
}

func validateTransport(definition *MCPServerDefinition) error {
	switch definition.Transport {
	case ServerTypeStdio, ServerTypeHTTP, ServerTypeSSE, ServerTypeStreamableHTTP:
	default:
		return fmt.Errorf("%w: unsupported transport", ErrMCPInvalidDefinition)
	}
	if definition.ExecutionMode == ExecutionModeRemote && definition.Transport == ServerTypeStdio {
		return fmt.Errorf("%w: remote definitions require a network transport", ErrMCPInvalidDefinition)
	}
	if definition.ExecutionMode != ExecutionModeRemote && definition.Transport != ServerTypeStdio {
		return fmt.Errorf("%w: local definitions require stdio transport", ErrMCPInvalidDefinition)
	}
	return nil
}

func validateConfiguration(definition *MCPServerDefinition) error {
	configuration := definition.Configuration
	switch definition.ExecutionMode {
	case ExecutionModeRemote:
		if !validHTTPURL(configuration.URL) {
			return fmt.Errorf("%w: remote URL must use http or https", ErrMCPInvalidDefinition)
		}
	case ExecutionModeManagedPackage:
		if configuration.PackageType != "npm" || strings.TrimSpace(configuration.PackageName) == "" || !exactPackageVersion(configuration.PackageVersion) {
			return fmt.Errorf("%w: managed packages require an npm name and exact version", ErrMCPInvalidDefinition)
		}
		if configuration.PackageRegistry != "" && !validHTTPURL(configuration.PackageRegistry) {
			return fmt.Errorf("%w: package registry must use http or https", ErrMCPInvalidDefinition)
		}
	case ExecutionModeExistingExecutable:
		if strings.TrimSpace(configuration.Command) == "" {
			return fmt.Errorf("%w: executable command is required", ErrMCPInvalidDefinition)
		}
	default:
		return fmt.Errorf("%w: unsupported execution mode", ErrMCPInvalidDefinition)
	}
	return nil
}

func validateSecretBindings(bindings []MCPSecretBinding) error {
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		name := strings.TrimSpace(binding.InputName)
		secretID := strings.TrimSpace(binding.SecretID)
		if name == "" || secretID == "" {
			return fmt.Errorf("%w: secret bindings require input and secret identifiers", ErrMCPInvalidDefinition)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("%w: secret binding inputs must be unique", ErrMCPInvalidDefinition)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func exactPackageVersion(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "^~<>=*| ") || strings.HasPrefix(value, "v") {
		return false
	}
	parts := strings.SplitN(value, "+", 2)
	core := strings.SplitN(parts[0], "-", 2)[0]
	numbers := strings.Split(core, ".")
	if len(numbers) != 3 {
		return false
	}
	for _, number := range numbers {
		if number == "" || (len(number) > 1 && number[0] == '0') {
			return false
		}
		for _, character := range number {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func normalizeRuntimeName(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func hasWhitespace(value string) bool {
	for _, character := range value {
		if unicode.IsSpace(character) {
			return true
		}
	}
	return false
}

func revisionConflict(current *MCPServerDefinition) error {
	return &MCPRevisionConflictError{Current: cloneDefinition(current)}
}

func cloneDefinition(definition *MCPServerDefinition) *MCPServerDefinition {
	if definition == nil {
		return nil
	}
	clone := *definition
	clone.Configuration = cloneConfiguration(definition.Configuration)
	clone.SecretBindings = cloneSecretBindings(definition.SecretBindings)
	return &clone
}

func cloneConfiguration(configuration MCPServerConfiguration) MCPServerConfiguration {
	clone := configuration
	clone.Args = append([]string(nil), configuration.Args...)
	clone.Env = cloneStringMap(configuration.Env)
	clone.Headers = cloneStringMap(configuration.Headers)
	clone.Options = cloneAnyMap(configuration.Options)
	return clone
}

func cloneSecretBindings(bindings []MCPSecretBinding) []MCPSecretBinding {
	return append([]MCPSecretBinding(nil), bindings...)
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = cloneAny(value)
	}
	return clone
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneAny(item)
		}
		return clone
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
