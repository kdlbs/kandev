package routines

// CreateRoutineRequest is the request body for creating a routine.
type CreateRoutineRequest struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	TaskTemplate           string `json:"task_template"`
	AssigneeAgentProfileID string `json:"assignee_agent_profile_id"`
	ConcurrencyPolicy      string `json:"concurrency_policy"`
	CatchUpPolicy          string `json:"catch_up_policy"`
	CatchUpMax             int    `json:"catch_up_max"`
	Variables              string `json:"variables"`
}

// UpdateRoutineRequest is the request body for updating a routine.
type UpdateRoutineRequest struct {
	Name                   *string `json:"name,omitempty"`
	Description            *string `json:"description,omitempty"`
	TaskTemplate           *string `json:"task_template,omitempty"`
	AssigneeAgentProfileID *string `json:"assignee_agent_profile_id,omitempty"`
	Status                 *string `json:"status,omitempty"`
	ConcurrencyPolicy      *string `json:"concurrency_policy,omitempty"`
	CatchUpPolicy          *string `json:"catch_up_policy,omitempty"`
	CatchUpMax             *int    `json:"catch_up_max,omitempty"`
	Variables              *string `json:"variables,omitempty"`
}

// RunRoutineRequest is the request body for manually firing a routine.
type RunRoutineRequest struct {
	Variables map[string]string `json:"variables"`
}

// RoutineResponse wraps a single routine.
type RoutineResponse struct {
	Routine *Routine `json:"routine"`
}

// RoutineListResponse wraps a list of routines.
type RoutineListResponse struct {
	Routines []*Routine `json:"routines"`
}

// CreateTriggerRequest is the request body for creating a routine trigger.
type CreateTriggerRequest struct {
	Kind           string `json:"kind"`
	CronExpression string `json:"cron_expression"`
	Timezone       string `json:"timezone"`
	PublicID       string `json:"public_id"`
	SigningMode    string `json:"signing_mode"`
	Secret         string `json:"secret"`
}

// TriggerResponse wraps a single routine trigger.
type TriggerResponse struct {
	Trigger *RoutineTrigger `json:"trigger"`
}

// TriggerListResponse wraps a list of routine triggers.
type TriggerListResponse struct {
	Triggers []*RoutineTrigger `json:"triggers"`
}

// RoutineRunResponse wraps a single routine run.
type RoutineRunResponse struct {
	Run *RoutineRun `json:"run"`
}

// RunListResponse wraps a list of routine runs.
type RunListResponse struct {
	Runs []*RoutineRun `json:"runs"`
}
