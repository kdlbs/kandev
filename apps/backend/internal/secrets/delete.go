package secrets

import (
	"context"
	"fmt"
	"strings"
)

// Reference identifies a secret binding without carrying its value or secret ID.
// Inaccessible repositories expose only Kind.
type Reference struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

// InUseError prevents deletion of a secret with existing environment bindings.
type InUseError struct {
	References []Reference `json:"references"`
}

func (e *InUseError) Error() string {
	labels := make([]string, 0, len(e.References))
	for _, ref := range e.References {
		label := strings.ReplaceAll(ref.Kind, "_", " ")
		if ref.Name != "" {
			label += fmt.Sprintf(" %q", ref.Name)
		}
		if ref.Key != "" {
			label += fmt.Sprintf(" (%s)", ref.Key)
		}
		labels = append(labels, label)
	}
	return "secret is in use by " + strings.Join(labels, ", ") + ". Remove or replace these references before deleting it."
}

// SetReferenceChecker wires reference discovery from the owning repositories.
// An unavailable checker blocks ordinary user-facing deletion.
func (s *Service) SetReferenceChecker(checker func(context.Context, string) ([]Reference, error)) {
	s.referenceChecker = checker
}

func (s *Service) deleteChecked(ctx context.Context, id, workspaceID string, force bool) error {
	if force {
		return s.deleteStored(ctx, id, workspaceID)
	}
	refs, err := s.listReferences(ctx, id)
	if err != nil {
		return err
	}
	if len(refs) > 0 {
		return &InUseError{References: refs}
	}
	return s.deleteStored(ctx, id, workspaceID)
}

func (s *Service) listReferences(ctx context.Context, id string) ([]Reference, error) {
	if s.referenceChecker == nil {
		return nil, fmt.Errorf("secret reference checking is unavailable")
	}
	refs, err := s.referenceChecker(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("check secret references: %w", err)
	}
	return refs, nil
}

func (s *Service) deleteStored(ctx context.Context, id, workspaceID string) error {
	if scoped, ok := s.store.(WorkspaceSecretDeleter); ok && workspaceID != "" {
		return scoped.DeleteForWorkspace(ctx, id, workspaceID)
	}
	return s.store.Delete(ctx, id)
}
