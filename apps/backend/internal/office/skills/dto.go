package skills

import "github.com/kandev/kandev/internal/office/models"

// CreateSkillRequest is the request body for creating a skill.
type CreateSkillRequest struct {
	Name                    string `json:"name"`
	Slug                    string `json:"slug"`
	Description             string `json:"description"`
	SourceType              string `json:"source_type"`
	SourceLocator           string `json:"source_locator"`
	Content                 string `json:"content"`
	FileInventory           string `json:"file_inventory"`
	CreatedByAgentProfileID string `json:"created_by_agent_profile_id"`
}

// UpdateSkillRequest is the request body for updating a skill.
type UpdateSkillRequest struct {
	Name          *string `json:"name,omitempty"`
	Slug          *string `json:"slug,omitempty"`
	Description   *string `json:"description,omitempty"`
	SourceType    *string `json:"source_type,omitempty"`
	SourceLocator *string `json:"source_locator,omitempty"`
	Content       *string `json:"content,omitempty"`
	FileInventory *string `json:"file_inventory,omitempty"`
}

// ImportSkillRequest is the request body for importing a skill from a URL or path.
type ImportSkillRequest struct {
	Source     string `json:"source"`
	SourceType string `json:"source_type,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Key        string `json:"key,omitempty"`
}

// SkillResponse wraps a single skill.
type SkillResponse struct {
	Skill *models.Skill `json:"skill"`
}

// SkillListResponse wraps a list of skills.
type SkillListResponse struct {
	Skills []*models.Skill `json:"skills"`
}

// DiscoveredUserSkillListResponse wraps user-home skill discovery results.
type DiscoveredUserSkillListResponse struct {
	Skills []DiscoveredUserSkill `json:"skills"`
}

// ImportSkillResponse wraps the result of a skill import.
type ImportSkillResponse struct {
	Skills   []*models.Skill `json:"skills"`
	Warnings []string        `json:"warnings"`
}

// SkillFileResponse wraps the content of a skill file.
type SkillFileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
