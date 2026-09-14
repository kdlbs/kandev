package dashboard

import (
	"expvar"
	"strings"
)

// officeSessionTermSuppressedTotal counts guarded terminations that did not
// end a session, exposed via stdlib's /debug/vars handler. The label model is
// "key=value;key=value..." so a Prometheus translation layer can split on `;`
// and `=` later, mirroring internal/office/scheduler/metrics_vars.go.
//
// A suppression leaves a session live where one used to end. Logged alone it
// is indistinguishable from a path that never ran, so a mistaken
// retained-capacity answer would look exactly like a quiet system.
//
// Counters only. Both dimensions are closed sets - the guarded reason is one
// of three and the outcome is one of three - so no task, agent, step or
// session identifier is ever a label.
var officeSessionTermSuppressedTotal = expvar.NewMap("office_session_term_suppressed_total")

// sessionTermSuppressReadFailed is the outcome label for a suppression caused
// by a capacity read that failed, as opposed to one caused by a capacity the
// agent genuinely still holds. A read fault and a retained capacity are
// different events and must stay countable apart.
const sessionTermSuppressReadFailed = "read_failed"

// officeSessionTermLabel builds a "k1=v1;k2=v2;..." expvar map key. Returns an
// empty label for an odd number of arguments rather than guessing.
func officeSessionTermLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// recordSessionTermSuppressed counts one suppressed termination. outcome is
// either the retained capacity or sessionTermSuppressReadFailed.
func recordSessionTermSuppressed(reason, outcome string) {
	officeSessionTermSuppressedTotal.Add(
		officeSessionTermLabel("reason", reason, "outcome", outcome), 1)
}
