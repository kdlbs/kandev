// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"github.com/kandev/kandev/internal/office/shared"
)

// ListPendingPermissions implements shared.PermissionLister.
// It returns a snapshot of all pending clarification requests as office-safe values.
func (s *Store) ListPendingPermissions() []shared.PendingPermission {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]shared.PendingPermission, 0, len(s.pending))
	for _, p := range s.pending {
		prompt := ""
		if len(p.Request.Questions) > 0 {
			prompt = p.Request.Questions[0].Prompt
		}
		out = append(out, shared.PendingPermission{
			PendingID: p.Request.PendingID,
			SessionID: p.Request.SessionID,
			TaskID:    p.Request.TaskID,
			Prompt:    prompt,
			Context:   p.Request.Context,
			CreatedAt: p.CreatedAt,
		})
	}
	return out
}
