package clarification

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// stubAuthorizer is a taskAccessAuthorizer double. Its zero value permits
// every task_id (the auth-disabled/unscoped-caller stand-in); set err to
// deny every call.
type stubAuthorizer struct {
	err error
}

func (a *stubAuthorizer) AuthorizeTaskAccess(_ context.Context, _ string) error {
	return a.err
}

// clarificationMessage builds a single-question clarification message row for
// tests, with metadata shaped the way bundle_question.go expects.
func clarificationMessage(id, pendingID, questionID string, index int, prompt, optionID string) *taskmodels.Message {
	return &taskmodels.Message{
		ID:            id,
		TaskSessionID: "s1",
		TaskID:        "t1",
		Metadata: map[string]any{
			metaPendingIDKey:  pendingID,
			metaQuestionIDKey: questionID,
			"question_index":  index,
			metaStatusKey:     string(StatusPending),
			metaQuestionKey: map[string]any{
				"id":     questionID,
				"prompt": prompt,
				"options": []any{
					map[string]any{"id": optionID, "label": optionID},
				},
			},
		},
	}
}

// stubMessageCreator is a MessageCreator double. Zero value is a
// no-op/success stub suitable for tests that only exercise authorization or
// read paths; fields let a test opt into specific claim/delivery outcomes.
type stubMessageCreator struct {
	createErr        error
	completeMessages []*taskmodels.Message
	completeClaimed  bool
	completeErr      error
}

func (s *stubMessageCreator) CreateClarificationRequestMessages(
	context.Context, string, string, string, []Question, string,
) ([]string, error) {
	return nil, s.createErr
}

func (s *stubMessageCreator) UpdateClarificationMessage(
	context.Context, string, string, string, string, *Answer,
) error {
	return nil
}

func (s *stubMessageCreator) CompleteActiveClarificationBundle(
	context.Context, string, string, map[string]interface{},
) ([]*taskmodels.Message, bool, error) {
	return s.completeMessages, s.completeClaimed, s.completeErr
}

func (s *stubMessageCreator) FinalizeClarificationResponseDelivery(
	context.Context, string, string, []*taskmodels.Message,
) ([]*taskmodels.Message, bool, error) {
	return nil, true, nil
}

func (s *stubMessageCreator) RestoreActiveClarificationBundle(
	context.Context, string, string, []*taskmodels.Message,
) ([]*taskmodels.Message, bool, error) {
	return nil, true, nil
}

func (s *stubMessageCreator) PublishClarificationBundleUpdates(context.Context, []*taskmodels.Message) error {
	return nil
}

func runRespond(t *testing.T, h *Handlers, pendingID string, body RespondBody) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal respond body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clarification/"+pendingID+"/respond", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{gin.Param{Key: "id", Value: pendingID}}
	h.httpRespond(c)
	return rec
}

func runCancel(t *testing.T, h *Handlers, pendingID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clarification/"+pendingID+"/cancel", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{gin.Param{Key: "id", Value: pendingID}}
	h.httpCancelRequest(c)
	return rec
}
