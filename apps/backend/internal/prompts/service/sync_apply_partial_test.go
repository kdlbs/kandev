package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/prompts/models"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

// failingReferenceRepo saves writes normally but fails the reference read the
// removal guard performs afterward.
type failingReferenceRepo struct {
	promptstore.Repository
}

var errReferenceRead = errors.New("reference read failed")

func (failingReferenceRepo) ListPromptsForReferenceExpansion(context.Context, int, int, int, int, int) ([]*models.Prompt, bool, error) {
	return nil, false, errReferenceRead
}

func TestApplySyncedPromptsPublishesSavedWritesWhenALaterStepFails(t *testing.T) {
	base, cleanup := createService(t)
	t.Cleanup(cleanup)
	applySynced(t, base, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "keep", Content: "v1", Path: "p/keep.md"},
		{Name: "gone", Content: "x", Path: "p/gone.md"},
	}})

	svc := NewService(failingReferenceRepo{Repository: base.repo})
	events := subscribePromptEvents(t, svc)
	_, err := svc.ApplySyncedPrompts(context.Background(), SyncApplyInput{
		Desired:        []SyncedPromptFile{{Name: "keep", Content: "v2", Path: "p/keep.md"}},
		ReferenceTexts: []PromptReferenceText{{Owner: "wf", Text: "@gone"}},
	})
	require.ErrorIs(t, err, errReferenceRead)
	require.Len(t, *events, 1, "the saved update must still invalidate open prompt lists")
}
