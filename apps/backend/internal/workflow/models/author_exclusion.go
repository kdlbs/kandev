package models

import "sort"

// ExcludeAuthorProviderConfigKey is the ensure_participant_seat config key
// that keeps the seat's runs off the provider that did the task's work.
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

// ParticipantSeatRoles returns each role that has an ensure_participant_seat
// action on the step, mapped to whether that action sets
// exclude_author_provider. Seat actions without a role are ignored.
func ParticipantSeatRoles(events StepEvents) map[string]bool {
	roles := map[string]bool{}
	for _, action := range events.OnEnter {
		if action.Type != OnEnterEnsureParticipantSeat {
			continue
		}
		if role, _ := action.Config["role"].(string); role != "" {
			roles[role] = roles[role] || ExcludesAuthorProvider(action.Config)
		}
	}
	return roles
}

// SetExcludeAuthorProvider returns a copy of events whose
// ensure_participant_seat actions for role set exclude_author_provider to
// true (enabled) or drop the key (disabled). Every other action and config key
// is copied unchanged, and events itself is not modified. The boolean reports
// whether any seat action matched role.
func SetExcludeAuthorProvider(events StepEvents, role string, enabled bool) (StepEvents, bool) {
	out := events
	out.OnEnter = make([]OnEnterAction, len(events.OnEnter))
	found := false
	for i, action := range events.OnEnter {
		out.OnEnter[i] = action
		if action.Type != OnEnterEnsureParticipantSeat {
			continue
		}
		if actionRole, _ := action.Config["role"].(string); actionRole != role || role == "" {
			continue
		}
		found = true
		config := make(map[string]interface{}, len(action.Config)+1)
		for k, v := range action.Config {
			config[k] = v
		}
		if enabled {
			config[ExcludeAuthorProviderConfigKey] = true
		} else {
			delete(config, ExcludeAuthorProviderConfigKey)
		}
		out.OnEnter[i].Config = config
	}
	if events.OnEnter == nil {
		out.OnEnter = nil
	}
	return out, found
}
