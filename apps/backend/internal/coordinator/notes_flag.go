package coordinator

// WithNotes turns agent-tier notes on or off. Callers pass the effective
// condition (coordinator, phase 2, phase 3, phase 3.1 and the notes flag),
// never the raw flag.
func WithNotes(on bool) ServiceOption {
	return func(s *Service) { s.notes, s.store.notesOn = on, on }
}

// NotesEnabled reports whether agent-tier notes are effective.
func (s *Service) NotesEnabled() bool { return s.notes }
