package store

import "errors"

// ErrAgentProfileDeleted is returned by lookups that find a soft-deleted row.
// Distinguishes "the profile was removed" (recoverable: pick a new one) from
// "the profile never existed" (caller passed a bad ID). Wrapped — caller uses
// errors.Is.
var ErrAgentProfileDeleted = errors.New("agent profile soft-deleted")
