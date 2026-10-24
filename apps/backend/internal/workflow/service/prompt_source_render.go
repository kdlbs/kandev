package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/workflow/models"
	"reflect"
	"regexp"
	"strings"
)

var sourceVariable = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
var sourceTemplateID = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func renderSourceString(text string, variables map[string]string, nativeStep bool) (string, error) {
	var missing string
	result := sourceVariable.ReplaceAllStringFunc(text, func(token string) string {
		if nativeStep && token == "{{task_prompt}}" {
			return token
		}
		key := sourceVariable.FindStringSubmatch(token)[1]
		value, ok := variables[key]
		if !ok {
			missing = key
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("missing prompt source variable %q", missing)
	}
	return result, nil
}

type sourceStepExtras struct {
	AgentProfileID string           `json:"agent_profile_id"`
	StageType      models.StageType `json:"stage_type"`
}
type sourceRendered struct {
	portable models.WorkflowPortable
	extras   map[int]sourceStepExtras
}

func renderSourceTemplate(template models.PromptSourceTemplate, variables map[string]string) (sourceRendered, error) {
	out := sourceRendered{portable: models.WorkflowPortable{Name: template.Name, Description: template.Description}, extras: make(map[int]sourceStepExtras)}
	var err error
	out.portable.Prompt, err = renderSourceString(template.Prompt, variables, false)
	if err != nil {
		return out, err
	}
	for _, raw := range template.Steps {
		step, extras, err := renderSourceStep(raw, variables)
		if err != nil {
			return out, err
		}
		out.portable.Steps = append(out.portable.Steps, step)
		out.extras[step.Position] = extras
	}
	validation := out.portable
	validation.Steps = append([]models.StepPortable(nil), out.portable.Steps...)
	for i := range validation.Steps {
		if out.extras[validation.Steps[i].Position].AgentProfileID != "" {
			validation.Steps[i].AgentProfile = &models.AgentProfilePortable{AgentName: "source-bound"}
		}
	}
	export := models.WorkflowExport{Version: models.ExportVersion, Type: models.ExportType, Workflows: []models.WorkflowPortable{validation}}
	if err := export.Validate(); err != nil {
		return out, err
	}
	return out, nil
}

func validateSourceEnvelope(request models.PromptSourceImport) (map[string]models.PromptSourceTemplate, error) {
	templates := make(map[string]models.PromptSourceTemplate)
	for _, template := range request.Templates {
		if !sourceTemplateID.MatchString(template.ID) || template.Revision < 1 || len(template.Steps) == 0 || strings.TrimSpace(template.Name) == "" {
			return nil, fmt.Errorf("invalid prompt source template identity or revision")
		}
		if _, ok := templates[template.ID]; ok {
			return nil, fmt.Errorf("duplicate prompt source template %q", template.ID)
		}
		templates[template.ID] = template
	}
	seen := make(map[string]bool)
	names := make(map[string]bool)
	for _, instance := range request.Instances {
		key := instance.WorkspaceID + "\x00" + instance.TemplateID
		if seen[key] || instance.WorkspaceID == "" || instance.SourceLocked == nil {
			return nil, fmt.Errorf("duplicate or incomplete prompt source instance")
		}
		seen[key] = true
		template, ok := templates[instance.TemplateID]
		if !ok {
			return nil, fmt.Errorf("instance refers to missing template %q", instance.TemplateID)
		}
		nameKey := instance.WorkspaceID + "\x00" + template.Name
		if names[nameKey] {
			return nil, fmt.Errorf("duplicate workflow name in prompt source instances")
		}
		names[nameKey] = true
	}
	if len(request.Instances) == 0 {
		return nil, fmt.Errorf("prompt source instances are required")
	}
	return templates, nil
}

func sourceVariablesEqual(a, b map[string]string) bool {
	one, _ := json.Marshal(a)
	two, _ := json.Marshal(b)
	return bytes.Equal(one, two)
}

func validateSourceStepFields(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	allowed := map[string]bool{"agent_profile_id": true, "stage_type": true}
	kind := reflect.TypeFor[models.StepPortable]()
	for i := 0; i < kind.NumField(); i++ {
		name := strings.Split(kind.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			allowed[name] = true
		}
	}
	for field := range fields {
		if !allowed[field] {
			return fmt.Errorf("unknown prompt source step field %q", field)
		}
	}
	return nil
}

func renderSourceStep(raw json.RawMessage, variables map[string]string) (models.StepPortable, sourceStepExtras, error) {
	var step models.StepPortable
	var extras sourceStepExtras
	var err error
	if err := validateSourceStepFields(raw); err != nil {
		return step, extras, err
	}
	if err := json.Unmarshal(raw, &step); err != nil {
		return step, extras, err
	}
	if err := json.Unmarshal(raw, &extras); err != nil {
		return step, extras, err
	}
	if step.AgentProfile != nil && extras.AgentProfileID != "" {
		return step, extras, fmt.Errorf("step %q supplies two profile bindings", step.Name)
	}
	if extras.StageType != "" && extras.StageType != models.StageTypeWork && extras.StageType != models.StageTypeReview && extras.StageType != models.StageTypeApproval && extras.StageType != models.StageTypeCustom {
		return step, extras, fmt.Errorf("invalid stage_type for step %q", step.Name)
	}
	step.Prompt, err = renderSourceString(step.Prompt, variables, true)
	if err != nil {
		return step, extras, err
	}
	return step, extras, nil
}
