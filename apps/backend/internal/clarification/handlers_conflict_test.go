package clarification

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestWriteResolutionResultNotActiveConflictIncludesMachineReadableCode covers
// Defect 2: the frontend used to map every 409 from /respond to a silent
// "submitted successfully" state, on the now-disproven assumption that a 409
// only ever means "you already submitted". The only 409 this handler actually
// produces is IsNotActiveError (the bundle expired/was superseded), so the
// response must carry a stable machine-readable `code` the client can branch
// on -- additive alongside the existing human-readable `error` string, which
// must not change (older clients still read it).
func TestWriteResolutionResultNotActiveConflictIncludesMachineReadableCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handlers{}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	h.writeResolutionResult(c, "p1", nil, false, errClarificationNotActive)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response body: %v", err)
	}
	if got := body["code"]; got != clarificationConflictNotActive {
		t.Fatalf("code = %v, want %q", got, clarificationConflictNotActive)
	}
	if got := body["error"]; got != "clarification request is no longer active" {
		t.Fatalf("error = %v, want unchanged human-readable message", got)
	}
}
