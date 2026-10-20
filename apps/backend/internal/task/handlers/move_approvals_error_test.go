package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/service"
)

// @covers AC-OFFICE-HUMAN-DONE-002.1
func TestHandleSelectedMoveError_ApprovalsPending409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	err := fmt.Errorf("move: %w", &service.ApprovalsPendingMoveError{
		Pending: []service.PendingApprover{{AgentProfileID: "agent-ceo", Name: "CEO"}},
	})

	handleSelectedMoveError(c, newTestLogger(t), err)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body struct {
		Code             string `json:"code"`
		PendingApprovers []struct {
			AgentProfileID string `json:"agent_profile_id"`
			Name           string `json:"name"`
		} `json:"pending_approvers"`
	}
	if decodeErr := json.Unmarshal(rec.Body.Bytes(), &body); decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	if body.Code != "task_approvals_pending" {
		t.Fatalf("code = %q, want task_approvals_pending", body.Code)
	}
	if len(body.PendingApprovers) != 1 || body.PendingApprovers[0].AgentProfileID != "agent-ceo" ||
		body.PendingApprovers[0].Name != "CEO" {
		t.Fatalf("pending_approvers = %+v, want CEO", body.PendingApprovers)
	}
}
