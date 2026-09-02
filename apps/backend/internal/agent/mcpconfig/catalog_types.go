package mcpconfig

import (
	"context"
	"time"
)

// ExecutionMode controls how Kandev materializes an MCP server definition.
type ExecutionMode string

const (
	ExecutionModeRemote             ExecutionMode = "remote"
	ExecutionModeManagedPackage     ExecutionMode = "managed_package"
	ExecutionModeExistingExecutable ExecutionMode = "existing_executable"
)

// DefinitionSource identifies where an MCP definition came from.
type DefinitionSource string

const (
	DefinitionSourceCurated      DefinitionSource = "curated"
	DefinitionSourceRegistry     DefinitionSource = "registry"
	DefinitionSourceCustom       DefinitionSource = "custom"
	DefinitionSourceLegacyImport DefinitionSource = "legacy_import"
)

// MCPServerConfiguration contains non-secret materialization settings.
type MCPServerConfiguration struct {
	Command         string            `json:"command,omitempty"`
	Args            []string          `json:"args,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	URL             string            `json:"url,omitempty"`
	PackageType     string            `json:"package_type,omitempty"`
	PackageName     string            `json:"package_name,omitempty"`
	PackageVersion  string            `json:"package_version,omitempty"`
	PackageRegistry string            `json:"package_registry,omitempty"`
	Options         map[string]any    `json:"options,omitempty"`
}

// MCPSecretBinding references a secret without storing its value in the catalog.
type MCPSecretBinding struct {
	InputName string `json:"input_name"`
	SecretID  string `json:"secret_id"`
}

// MCPServerDefinition is a workspace-owned MCP server catalog entry.
type MCPServerDefinition struct {
	ID                    string                 `json:"id"`
	WorkspaceID           string                 `json:"workspace_id"`
	RuntimeName           string                 `json:"runtime_name"`
	NormalizedRuntimeName string                 `json:"normalized_runtime_name"`
	DisplayName           string                 `json:"display_name"`
	Description           string                 `json:"description,omitempty"`
	Enabled               bool                   `json:"enabled"`
	ExecutionMode         ExecutionMode          `json:"execution_mode"`
	Transport             ServerType             `json:"transport"`
	Configuration         MCPServerConfiguration `json:"configuration"`
	SecretBindings        []MCPSecretBinding     `json:"secret_bindings,omitempty"`
	Source                DefinitionSource       `json:"source"`
	SourceIdentity        string                 `json:"source_identity,omitempty"`
	Revision              int64                  `json:"revision"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

// CreateDefinitionInput is the caller-owned input for a new catalog entry.
type CreateDefinitionInput struct {
	// ID is optional for normal creates. Compatibility importers can provide a
	// deterministic ID so a retried profile-workspace import does not create a
	// second definition.
	ID             string
	WorkspaceID    string
	RuntimeName    string
	DisplayName    string
	Description    string
	Enabled        *bool
	ExecutionMode  ExecutionMode
	Transport      ServerType
	Configuration  MCPServerConfiguration
	SecretBindings []MCPSecretBinding
	Source         DefinitionSource
	SourceIdentity string
}

// UpdateDefinitionInput contains optional changes to one catalog entry.
type UpdateDefinitionInput struct {
	WorkspaceID      string
	ID               string
	ExpectedRevision int64
	RuntimeName      *string
	DisplayName      *string
	Description      *string
	Enabled          *bool
	ExecutionMode    *ExecutionMode
	Transport        *ServerType
	Configuration    *MCPServerConfiguration
	SecretBindings   *[]MCPSecretBinding
	Source           *DefinitionSource
	SourceIdentity   *string
}

// WorkspaceAuthorizer checks that the current caller can access a workspace.
type WorkspaceAuthorizer func(ctx context.Context, workspaceID string) error
