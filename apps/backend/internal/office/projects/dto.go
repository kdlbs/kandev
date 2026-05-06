package projects

// CreateProjectRequest is the request body for creating a project.
type CreateProjectRequest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	LeadAgentProfileID string   `json:"lead_agent_profile_id"`
	Color              string   `json:"color"`
	BudgetCents        int      `json:"budget_cents"`
	Repositories       []string `json:"repositories"`
	ExecutorConfig     string   `json:"executor_config"`
}

// UpdateProjectRequest is the request body for updating a project.
type UpdateProjectRequest struct {
	Name               *string   `json:"name,omitempty"`
	Description        *string   `json:"description,omitempty"`
	Status             *string   `json:"status,omitempty"`
	LeadAgentProfileID *string   `json:"lead_agent_profile_id,omitempty"`
	Color              *string   `json:"color,omitempty"`
	BudgetCents        *int      `json:"budget_cents,omitempty"`
	Repositories       *[]string `json:"repositories,omitempty"`
	ExecutorConfig     *string   `json:"executor_config,omitempty"`
}

// ProjectResponse wraps a single project.
type ProjectResponse struct {
	Project *Project `json:"project"`
}

// ProjectListResponse wraps a list of projects.
type ProjectListResponse struct {
	Projects []*ProjectWithCounts `json:"projects"`
}
