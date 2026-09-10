package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/db/dialect"
)

const mcpDefinitionColumns = `
	id, workspace_id, runtime_name, normalized_runtime_name, display_name,
	description, enabled, execution_mode, transport, configuration_json,
	secret_bindings_json, source, source_identity, revision, created_at, updated_at`

var _ mcpconfig.CatalogRepository = (*sqliteRepository)(nil)

func (r *sqliteRepository) ListMCPServerDefinitions(ctx context.Context, workspaceID string) ([]*mcpconfig.MCPServerDefinition, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`SELECT `+mcpDefinitionColumns+` FROM mcp_server_definitions WHERE workspace_id = ? ORDER BY normalized_runtime_name, id`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	definitions := make([]*mcpconfig.MCPServerDefinition, 0)
	for rows.Next() {
		definition, scanErr := scanMCPServerDefinition(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return definitions, nil
}

func (r *sqliteRepository) GetMCPServerDefinition(ctx context.Context, workspaceID, id string) (*mcpconfig.MCPServerDefinition, error) {
	row := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT `+mcpDefinitionColumns+` FROM mcp_server_definitions WHERE workspace_id = ? AND id = ?`), workspaceID, id)
	definition, err := scanMCPServerDefinition(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mcpconfig.ErrMCPServerDefinitionNotFound
	}
	return definition, err
}

func (r *sqliteRepository) CreateMCPServerDefinition(ctx context.Context, definition *mcpconfig.MCPServerDefinition) error {
	if err := prepareMCPServerDefinition(definition); err != nil {
		return err
	}
	configuration, bindings, err := marshalMCPDefinition(definition)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO mcp_server_definitions
		(id, workspace_id, runtime_name, normalized_runtime_name, display_name, description,
		 enabled, execution_mode, transport, configuration_json, secret_bindings_json,
		 source, source_identity, revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), definition.ID, definition.WorkspaceID, definition.RuntimeName,
		definition.NormalizedRuntimeName, definition.DisplayName, definition.Description,
		dialect.BoolToInt(definition.Enabled), definition.ExecutionMode, definition.Transport,
		configuration, bindings, definition.Source, definition.SourceIdentity,
		definition.Revision, definition.CreatedAt, definition.UpdatedAt)
	if isMCPDefinitionUniqueError(err) {
		return mcpconfig.ErrMCPRuntimeNameConflict
	}
	return err
}

func (r *sqliteRepository) UpdateMCPServerDefinition(ctx context.Context, definition *mcpconfig.MCPServerDefinition, expectedRevision int64) error {
	if err := prepareMCPServerDefinition(definition); err != nil {
		return err
	}
	configuration, bindings, err := marshalMCPDefinition(definition)
	if err != nil {
		return err
	}
	nextRevision := expectedRevision + 1
	if definition.Revision > 0 {
		nextRevision = definition.Revision
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE mcp_server_definitions
		SET runtime_name = ?, normalized_runtime_name = ?, display_name = ?, description = ?,
		    enabled = ?, execution_mode = ?, transport = ?, configuration_json = ?,
		    secret_bindings_json = ?, source = ?, source_identity = ?, revision = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ? AND revision = ?
	`), definition.RuntimeName, definition.NormalizedRuntimeName, definition.DisplayName,
		definition.Description, dialect.BoolToInt(definition.Enabled), definition.ExecutionMode,
		definition.Transport, configuration, bindings, definition.Source, definition.SourceIdentity,
		nextRevision, definition.UpdatedAt, definition.WorkspaceID, definition.ID, expectedRevision)
	if isMCPDefinitionUniqueError(err) {
		return mcpconfig.ErrMCPRuntimeNameConflict
	}
	if err != nil {
		return err
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return rowsErr
	}
	if rows > 0 {
		return nil
	}
	return r.mcpRevisionOrNotFound(ctx, definition.WorkspaceID, definition.ID)
}

func (r *sqliteRepository) DeleteMCPServerDefinition(ctx context.Context, workspaceID, id string, expectedRevision int64) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM mcp_server_definitions WHERE workspace_id = ? AND id = ? AND revision = ?`), workspaceID, id, expectedRevision)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}
	return r.mcpRevisionOrNotFound(ctx, workspaceID, id)
}

func (r *sqliteRepository) mcpRevisionOrNotFound(ctx context.Context, workspaceID, id string) error {
	current, err := r.GetMCPServerDefinition(ctx, workspaceID, id)
	if errors.Is(err, mcpconfig.ErrMCPServerDefinitionNotFound) {
		return mcpconfig.ErrMCPServerDefinitionNotFound
	}
	if err != nil {
		return err
	}
	return &mcpconfig.MCPRevisionConflictError{Current: current}
}

func prepareMCPServerDefinition(definition *mcpconfig.MCPServerDefinition) error {
	if definition == nil {
		return errors.New("mcp server definition is required")
	}
	if definition.ID == "" {
		definition.ID = uuid.New().String()
	}
	if definition.Revision <= 0 {
		definition.Revision = 1
	}
	if definition.CreatedAt.IsZero() {
		definition.CreatedAt = time.Now().UTC()
	}
	if definition.UpdatedAt.IsZero() {
		definition.UpdatedAt = definition.CreatedAt
	}
	return nil
}

func marshalMCPDefinition(definition *mcpconfig.MCPServerDefinition) (string, string, error) {
	configuration, err := json.Marshal(definition.Configuration)
	if err != nil {
		return "", "", fmt.Errorf("marshal MCP configuration: %w", err)
	}
	bindings, err := json.Marshal(definition.SecretBindings)
	if err != nil {
		return "", "", fmt.Errorf("marshal MCP secret bindings: %w", err)
	}
	return string(configuration), string(bindings), nil
}

type mcpDefinitionScanner interface {
	Scan(...any) error
}

func scanMCPServerDefinition(scanner mcpDefinitionScanner) (*mcpconfig.MCPServerDefinition, error) {
	var definition mcpconfig.MCPServerDefinition
	var enabled int
	var executionMode, transport string
	var configurationJSON, bindingsJSON string
	err := scanner.Scan(&definition.ID, &definition.WorkspaceID, &definition.RuntimeName,
		&definition.NormalizedRuntimeName, &definition.DisplayName, &definition.Description,
		&enabled, &executionMode, &transport, &configurationJSON, &bindingsJSON,
		&definition.Source, &definition.SourceIdentity, &definition.Revision,
		&definition.CreatedAt, &definition.UpdatedAt)
	if err != nil {
		return nil, err
	}
	definition.Enabled = enabled == 1
	definition.ExecutionMode = mcpconfig.ExecutionMode(executionMode)
	definition.Transport = mcpconfig.ServerType(transport)
	if err := json.Unmarshal([]byte(configurationJSON), &definition.Configuration); err != nil {
		return nil, fmt.Errorf("unmarshal MCP configuration: %w", err)
	}
	if err := json.Unmarshal([]byte(bindingsJSON), &definition.SecretBindings); err != nil {
		return nil, fmt.Errorf("unmarshal MCP secret bindings: %w", err)
	}
	return &definition, nil
}

func isMCPDefinitionUniqueError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "mcp_server_definitions") && strings.Contains(message, "unique")
}
