package backendapp

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/orchestrator"
)

func TestAnswerDeliveryErrorMapping(t *testing.T) {
	other := errors.New("boom")
	cases := []struct {
		in   error
		want error
	}{
		{nil, nil},
		{orchestrator.ErrPermissionNotFound, coordinator.ErrDeliveryStale},
		{fmt.Errorf("wrapped: %w", orchestrator.ErrPermissionStale), coordinator.ErrDeliveryStale},
		{orchestrator.ErrPermissionAlreadyResolved, coordinator.ErrDeliveryStale},
		{orchestrator.ErrPermissionTaskOrSessionNotFound, coordinator.ErrDeliveryTaskOrSessionGone},
		{orchestrator.ErrPermissionOptionNotOffered, coordinator.ErrDeliveryOptionNotOffered},
		{orchestrator.ErrPermissionResolutionInProgress, coordinator.ErrDeliveryInProgress},
		{orchestrator.ErrPermissionHumanInputRequired, coordinator.ErrDeliveryNotDelivered},
		{other, other},
		{orchestrator.ErrPermissionDeliveryFailed, orchestrator.ErrPermissionDeliveryFailed},
		{orchestrator.ErrPermissionAuditFailed, orchestrator.ErrPermissionAuditFailed},
	}
	for _, tc := range cases {
		if got := answerDeliveryError(tc.in); !errors.Is(got, tc.want) && got != tc.want {
			t.Errorf("answerDeliveryError(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
