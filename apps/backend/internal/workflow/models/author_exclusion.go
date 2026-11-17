package models

import "sort"

// ExcludeAuthorProviderConfigKey is the ensure_participant_seat config key
// that keeps the seat's runs off the provider that did the task's work
// (REQ-OFFICE-REVIEW-AUTHOR-EXCL-001).
const ExcludeAuthorProviderConfigKey = "exclude_author_provider"

// ExcludesAuthorProvider reports whether an ensure_participant_seat config
// sets exclude_author_provider. Only a boolean true enables it.
func ExcludesAuthorProvider(config map[string]interface{}) bool {
	v, _ := config[ExcludeAuthorProviderConfigKey].(bool)
	return v
}

// AuthorProviderExclusionRoles returns the participant roles whose
// ensure_participant_seat action on a step sets exclude_author_provider.
// Declarations without a role are ignored.
func AuthorProviderExclusionRoles(events StepEvents) map[string]bool {
	roles := map[string]bool{}
	for _, action := range events.OnEnter {
		if action.Type != OnEnterEnsureParticipantSeat || !ExcludesAuthorProvider(action.Config) {
			continue
		}
		if role, _ := action.Config["role"].(string); role != "" {
			roles[role] = true
		}
	}
	return roles
}

// AuthorExclusionDeclaration names one imported or synced step whose
// ensure_participant_seat action sets exclude_author_provider for a role.
type AuthorExclusionDeclaration struct {
	WorkflowName string
	StepName     string
	Role         string
}

// AuthorExclusionDeclarationsFor lists the declarations in workflows, in
// workflow, step position and role order.
func AuthorExclusionDeclarationsFor(workflows []WorkflowPortable) []AuthorExclusionDeclaration {
	var out []AuthorExclusionDeclaration
	for _, wf := range workflows {
		steps := append([]StepPortable(nil), wf.Steps...)
		sort.SliceStable(steps, func(i, j int) bool { return steps[i].Position < steps[j].Position })
		for _, step := range steps {
			roles := AuthorProviderExclusionRoles(step.Events)
			names := make([]string, 0, len(roles))
			for role := range roles {
				names = append(names, role)
			}
			sort.Strings(names)
			for _, role := range names {
				out = append(out, AuthorExclusionDeclaration{WorkflowName: wf.Name, StepName: step.Name, Role: role})
			}
		}
	}
	return out
}
