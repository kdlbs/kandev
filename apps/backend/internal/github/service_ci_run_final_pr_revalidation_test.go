package github

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRequestFreshCIRunRejectsHeadDriftAfterFinalSourceRunRead(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	drifted := *client.pr
	drifted.HeadSHA = strings.Repeat("b", 40)
	client.runHook = func(call int) {
		if call == 2 {
			client.pr = &drifted
		}
	}

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	var ciErr *CIRunRequestError
	if !errors.As(err, &ciErr) || ciErr.Class != CIRunFailureHeadDrift {
		t.Fatalf("error = %#v, want head_drift", err)
	}
	if receipt == nil || receipt.Status != CIRunRequestFailed ||
		receipt.FailureClass != string(CIRunFailureHeadDrift) {
		t.Fatalf("receipt = %+v, want terminal head_drift", receipt)
	}
	if receipt.ObservedPRHeadSHA != drifted.HeadSHA {
		t.Fatalf("observed PR head = %q, want %q", receipt.ObservedPRHeadSHA, drifted.HeadSHA)
	}
	if client.reruns != 0 {
		t.Fatalf("provider reruns = %d, want zero after final PR head drift", client.reruns)
	}
}

func TestRequestFreshCIRunRejectsLinkDriftAfterFinalSourceRunRead(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Service, RequestFreshCIRunInput)
		want CIRunFailureClass
	}{
		{
			name: "PR association detached",
			edit: func(service *Service, input RequestFreshCIRunInput) {
				_, _ = service.store.db.Exec(`UPDATE github_task_prs SET detached_at = CURRENT_TIMESTAMP
					WHERE task_id = ? AND repository_id = ? AND pr_number = ?`,
					input.TargetTaskID, input.RepositoryID, input.PRNumber)
			},
			want: CIRunFailureUnlinkedPR,
		},
		{
			name: "repository attachment removed",
			edit: func(service *Service, input RequestFreshCIRunInput) {
				_, _ = service.store.db.Exec(`DELETE FROM task_repositories
					WHERE task_id = ? AND repository_id = ?`, input.TargetTaskID, input.RepositoryID)
			},
			want: CIRunFailureRepositoryMismatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, client, input := setupCIRunServiceTest(t, false)
			client.runHook = func(call int) {
				if call == 2 {
					tt.edit(service, input)
				}
			}

			_, err := service.RequestFreshCIRun(context.Background(), input)
			var ciErr *CIRunRequestError
			if !errors.As(err, &ciErr) || ciErr.Class != tt.want {
				t.Fatalf("error = %#v, want %q", err, tt.want)
			}
			if client.reruns != 0 {
				t.Fatalf("provider reruns = %d, want zero after link drift", client.reruns)
			}
		})
	}
}
