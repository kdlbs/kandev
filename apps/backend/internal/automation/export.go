package automation

import "gopkg.in/yaml.v3"

// exportDocumentVersion and exportDocumentType are the fixed values of the export
// document's `version` and `type` keys. See docs/specs/office/requirements/automations-yaml-export.md
// § Data model.
const (
	exportDocumentVersion = 1
	exportDocumentType    = "kandev_automations"
)

// exportDocument is the top-level shape of a workspace's YAML automation export. Key
// order is pinned by AC-40: version, type, automations, warnings.
type exportDocument struct {
	Version     int                `yaml:"version"`
	Type        string             `yaml:"type"`
	Automations []exportAutomation `yaml:"automations"`
	Warnings    []string           `yaml:"warnings,omitempty"`
}

// exportAutomation is a single automation's exported form. Key order is pinned by
// AC-40: name, description, enabled, max_concurrent_runs, continuation_policy,
// task_mode, repository_mode, task_title_template, prompt,
// agent_profile, executor_profile, workflow, repositories, triggers.
//
// Prompt and every trigger's Config are *yaml.Node rather than string/json.RawMessage
// because both need explicit control over emitted Tag/Style that struct-tag marshaling
// cannot express: Prompt per the prompt-fidelity rules (AC-15/16/17/46/47/49), Config
// per the per-JSON-type node table (AC-8, AC-41).
type exportAutomation struct {
	Name               string                 `yaml:"name"`
	Description        string                 `yaml:"description,omitempty"`
	Enabled            bool                   `yaml:"enabled"`
	MaxConcurrentRuns  int                    `yaml:"max_concurrent_runs"`
	ContinuationPolicy ContinuationPolicy     `yaml:"continuation_policy"`
	TaskMode           TaskMode               `yaml:"task_mode"`
	RepositoryMode     RepositoryMode         `yaml:"repository_mode"`
	TaskTitleTemplate  string                 `yaml:"task_title_template,omitempty"`
	Prompt             *yaml.Node             `yaml:"prompt,omitempty"`
	AgentProfile       *exportAgentProfile    `yaml:"agent_profile,omitempty"`
	ExecutorProfile    *exportExecutorProfile `yaml:"executor_profile,omitempty"`
	Workflow           *exportWorkflow        `yaml:"workflow,omitempty"`
	Repositories       []string               `yaml:"repositories,omitempty"`
	Triggers           []exportTrigger        `yaml:"triggers"`
}

// exportAgentProfile is the portable {agent_name, model, mode} descriptor resolved
// from Automation.AgentProfileID. Defined locally rather than reusing
// workflow/models.AgentProfilePortable: that type's omitempty tags and shape were
// decided for a different feature (workflow export/import) and are not vetted against
// this spec's own AC-4/AC-22 disposition contract.
type exportAgentProfile struct {
	AgentName string `yaml:"agent_name"`
	Model     string `yaml:"model"`
	Mode      string `yaml:"mode"`
}

// exportExecutorProfile is the portable {executor, name} descriptor resolved from
// Automation.ExecutorProfileID.
type exportExecutorProfile struct {
	Executor string `yaml:"executor"`
	Name     string `yaml:"name"`
}

// exportWorkflow is the portable {name, step} descriptor resolved from
// Automation.WorkflowID / WorkflowStepID. Step is omitted (not merely empty) when the
// workflow resolves but the step does not, or when no step was ever referenced
// (AC-19).
type exportWorkflow struct {
	Name string `yaml:"name"`
	Step string `yaml:"step,omitempty"`
}

// exportTrigger is a single trigger's exported form. Key order is pinned by AC-40:
// type, enabled, config. Config is never omitted: an empty config is still emitted as
// an empty mapping (data model: "possibly empty").
type exportTrigger struct {
	Type    string     `yaml:"type"`
	Enabled bool       `yaml:"enabled"`
	Config  *yaml.Node `yaml:"config"`
}
