package dto

import "github.com/kandev/kandev/internal/orchestrate/models"

// PermissionMeta describes a single permission for frontend rendering.
type PermissionMeta struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Type        string `json:"type"` // "bool" or "int"
}

// MetaResponse contains all orchestrate metadata for the frontend.
type MetaResponse struct {
	Statuses           []models.StatusMeta               `json:"statuses"`
	Priorities         []models.PriorityMeta             `json:"priorities"`
	Roles              []models.RoleMeta                 `json:"roles"`
	ExecutorTypes      []models.ExecutorTypeMeta         `json:"executorTypes"`
	SkillSourceTypes   []models.SkillSourceTypeMeta      `json:"skillSourceTypes"`
	ProjectStatuses    []models.ProjectStatusMeta        `json:"projectStatuses"`
	AgentStatuses      []models.AgentStatusMeta          `json:"agentStatuses"`
	RoutineRunStatuses []models.RoutineRunStatusMeta     `json:"routineRunStatuses"`
	InboxItemTypes     []models.InboxItemTypeMeta        `json:"inboxItemTypes"`
	Permissions        []PermissionMeta                  `json:"permissions"`
	PermissionDefaults map[string]map[string]interface{} `json:"permissionDefaults"`
}
