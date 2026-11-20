package orchestrator

import "maps"

// The existing reservation freezes the workflow frame with its selected grade.
// Copies let launch preparation own its session/route values without mutating
// the frame retained under the common admission lock.
func cloneAdmittedStartSelection(selection *explicitWorkflowStartSelection) *explicitWorkflowStartSelection {
	if selection == nil || !selection.resolved {
		return nil
	}
	clone := *selection
	if selection.session != nil {
		session := *selection.session
		session.Metadata = maps.Clone(session.Metadata)
		clone.session = &session
	}
	if selection.route != nil {
		route := *selection.route
		clone.route = &route
	}
	return &clone
}

func carryAdmittedStartSelection(req admissionRequest, held *ceilingReservation) {
	if req.profileExplicit || req.startSelection == nil {
		return
	}
	if selection := cloneAdmittedStartSelection(held.startSelection); selection != nil {
		*req.startSelection = *selection
	}
}
