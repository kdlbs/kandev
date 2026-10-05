package projects

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandlerWriteErrorMarksUnsupportedProfileAsCorrectable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	(&Handler{}).writeError(ctx, fmt.Errorf("validate project: %w: profile-1", ErrUnsupportedProjectAgentProfile))

	if recorder.Code != 422 {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] == "" {
		t.Fatal("expected actionable profile error")
	}
}
