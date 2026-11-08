package coordinator

import "context"

// ChangeLinks resolves which tasks a change is linked to. It is satisfied by an
// adapter over the provider integrations' task link tables.
type ChangeLinks interface {
	// TasksLinkedToChange returns the IDs of the workspace's tasks linked to
	// the change key (provider|host|repository ID|number).
	TasksLinkedToChange(ctx context.Context, workspaceID, changeKey string) ([]string, error)
}

// SetChangeLinks wires the change-to-task link reader; nil until then, and the
// merge wake duties do nothing without it.
func (s *Service) SetChangeLinks(l ChangeLinks) {
	s.wakeMu.Lock()
	s.changeLinks = l
	s.wakeMu.Unlock()
}

func (s *Service) changeLinkReader() ChangeLinks {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	return s.changeLinks
}
