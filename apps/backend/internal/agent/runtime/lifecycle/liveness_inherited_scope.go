package lifecycle

// InheritedRecordScope names what this launch found at the recorded control
// endpoint. It governs only records this backend did NOT create during this
// process lifetime -- the ones inherited from an earlier launch -- because a
// record this backend created is judged against whichever server it is using,
// adopted or freshly started.
//
// The distinction matters because a fresh server's enumeration is not
// evidence about an instance it never launched. Judging an inherited record
// against it would report a still-running agent as gone.
type InheritedRecordScope int

const (
	// InheritedRecordScopeNoServer is nothing answered at the recorded
	// endpoint, or no endpoint was recorded. No control server holds any
	// inherited instance, so the process-identifier probe applies and
	// genuinely dead records are still repaired. This is the zero value:
	// a launch that never attempted adoption is in exactly this position.
	InheritedRecordScopeNoServer InheritedRecordScope = iota

	// InheritedRecordScopeForeignServer is a server answered at the recorded
	// endpoint and was left running, which is every path where adoption did
	// not complete against a reachable server. A surviving instance runs on
	// a server this backend does not drive, so its absence from the one this
	// backend does drive proves nothing and every inherited record is
	// unknown. Classifying it dead here would repair or prune a record whose
	// agent is still running.
	InheritedRecordScopeForeignServer

	// InheritedRecordScopeAdopted is this backend adopted the recorded
	// server, so its enumeration is authoritative for inherited records.
	InheritedRecordScopeAdopted
)

// SetInheritedRecordScope installs what this launch found at the recorded
// control endpoint, so liveness classification of records inherited from an
// earlier launch can tell "no server holds these" from "a server this
// backend does not drive might". Set once during startup wiring, before
// Start runs; the zero value (no server) is correct for a launch that never
// attempted adoption.
func (m *Manager) SetInheritedRecordScope(scope InheritedRecordScope) {
	m.inheritedRecordScope = scope
}
