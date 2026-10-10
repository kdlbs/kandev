package statussummary

import "testing"

func TestProjectorPRObservationEqualityUsesPointerValuesAndAllFields(t *testing.T) {
	projector := &Projector{}
	state := newProjectionState()
	event := func(conflict bool, requiredReviews int) map[string]interface{} {
		return map[string]interface{}{
			"repository_id":       "repo-1",
			"pr_number":           42,
			"state":               "open",
			"checks_state":        "success",
			"has_merge_conflicts": conflict,
			"required_reviews":    requiredReviews,
		}
	}

	if !projector.applyPREventLocked(state, event(true, 2)) {
		t.Fatal("initial PR observation was not applied")
	}
	if projector.applyPREventLocked(state, event(true, 2)) {
		t.Fatal("equivalent conflict pointer value published a duplicate observation")
	}
	if !projector.applyPREventLocked(state, event(true, 3)) {
		t.Fatal("change to another observation field was ignored")
	}
	if !projector.applyPREventLocked(state, event(false, 3)) {
		t.Fatal("conflict transition was ignored")
	}
}
