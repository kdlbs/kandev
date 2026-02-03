package dto

import (
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/editors/models"
)

type EditorDTO struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Command   string          `json:"command,omitempty"`
	Scheme    string          `json:"scheme,omitempty"`
	Config    json.RawMessage `json:"config,omitempty"`
	Installed bool            `json:"installed"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type EditorsResponse struct {
	Editors []EditorDTO `json:"editors"`
}

type OpenEditorRequest struct {
	EditorID   string `json:"editor_id,omitempty"`
	EditorType string `json:"editor_type,omitempty"`
	FilePath   string `json:"file_path,omitempty"`
	Line       int    `json:"line,omitempty"`
	Column     int    `json:"column,omitempty"`
}

type OpenEditorResponse struct {
	URL string `json:"url,omitempty"`
}

func FromEditor(editor *models.Editor) EditorDTO {
	if editor == nil {
		return EditorDTO{}
	}
	return EditorDTO{
		ID:        editor.ID,
		Type:      editor.Type,
		Name:      editor.Name,
		Kind:      editor.Kind,
		Command:   editor.Command,
		Scheme:    editor.Scheme,
		Config:    editor.Config,
		Installed: editor.Installed,
		Enabled:   editor.Enabled,
		CreatedAt: editor.CreatedAt,
		UpdatedAt: editor.UpdatedAt,
	}
}

type CreateEditorRequest struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind"`
	Config  json.RawMessage `json:"config,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`
}

type UpdateEditorRequest struct {
	Name    *string         `json:"name,omitempty"`
	Kind    *string         `json:"kind,omitempty"`
	Config  json.RawMessage `json:"config,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`
}

type OpenFolderResponse struct {
	Success bool `json:"success"`
}
