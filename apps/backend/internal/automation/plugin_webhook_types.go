package automation

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// PluginAutomationProvider supplies a generation lease through verification and admission.
type PluginAutomationProvider interface {
	AutomationConditions(context.Context, string) []PluginConditionInfo
	AcquireAutomationAdapter(string, string) (manifest.AutomationCondition, string, pluginsdk.AutomationAdapter, func(), error)
	SetAutomationSecret(context.Context, string, string) error
	ReadAutomationSecret(context.Context, string) (string, error)
	DeleteAutomationSecret(context.Context, string) error
}
type PluginConditionInfo struct {
	ConfigOptions map[string][]string          `json:"config_options,omitempty"`
	PluginID      string                       `json:"plugin_id"`
	ProviderLabel string                       `json:"provider_label"`
	Condition     manifest.AutomationCondition `json:"condition"`
	Available     bool                         `json:"available"`
	Reason        string                       `json:"reason,omitempty"`
}
type PluginEventConfig struct {
	PluginID      string          `json:"plugin_id"`
	ConditionKey  string          `json:"condition_key"`
	ConfigVersion int             `json:"config_version"`
	Settings      json.RawMessage `json:"settings"`
}
type WebhookBinding struct {
	ID                 string `db:"id" json:"id"`
	TriggerID          string `db:"trigger_id" json:"trigger_id"`
	AutomationID       string `db:"automation_id" json:"-"`
	WorkspaceID        string `db:"workspace_id" json:"-"`
	Revision           string `db:"revision" json:"-"`
	ConnectionID       string `db:"connection_id" json:"-"`
	ConnectionRevision string `db:"connection_revision" json:"-"`
	SecretID           string `db:"secret_id" json:"-"`
}
type WebhookReceipt struct {
	TaskID       string `db:"linked_task_id" json:"task_id,omitempty"`
	ID           string `db:"id" json:"id"`
	BindingID    string `db:"binding_id" json:"-"`
	AutomationID string `db:"automation_id" json:"-"`
	Revision     string `db:"revision" json:"-"`
	Identity     string `db:"identity" json:"-"`
	State        string `db:"state" json:"state"`
	Reason       string `db:"reason" json:"reason"`
	Payload      string `db:"payload" json:"-"`
	RunID        string `db:"run_id" json:"run_id,omitempty"`
	FinishedAt   int64  `db:"finished_at" json:"-"`
	CreatedAt    int64  `db:"created_at" json:"created_at"`
}

func (s *Service) SetPluginAutomationProvider(p PluginAutomationProvider) { s.pluginAutomation = p }
