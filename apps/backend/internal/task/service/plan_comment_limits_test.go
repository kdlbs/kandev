// @covers AC-TASKS-PLAN-COMMENTS-001.8
package service

import (
	"strings"
	"testing"
)

func TestPlanCommentFieldLimits(t *testing.T) {
	for _, tc := range []struct {
		name            string
		body, selection int
		rejected        bool
	}{
		{name: "boundary", body: 64 << 10, selection: 256 << 10},
		{name: "body overflow", body: (64 << 10) + 1, selection: 4, rejected: true},
		{name: "selection overflow", body: 4, selection: (256 << 10) + 1, rejected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := CreatePlanCommentRequest{
				TaskID: "task", PlanID: "plan", ID: "d8d97d5b-2663-45aa-aad4-64580a9ae07f",
				Body: strings.Repeat("x", tc.body), SelectedText: strings.Repeat("x", tc.selection),
				AnchorFrom: 0, AnchorTo: 4,
			}
			if err := validatePlanCommentCreate(req); (err != nil) != tc.rejected {
				t.Fatalf("create validation error = %v, rejected=%v", err, tc.rejected)
			}
		})
	}
	if err := validatePlanCommentMutation(
		"task", "plan", "d8d97d5b-2663-45aa-aad4-64580a9ae07f",
		strings.Repeat("x", (64<<10)+1), 1, true,
	); err == nil {
		t.Fatal("oversized update was accepted")
	}
}
