package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestInterruptedNativeRestoreRetiresOnlyObservedSubmission(t *testing.T) {
	retired := 0
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/submissions/old/retire":
			var identity struct {
				StreamID   string `json:"stream_id"`
				Generation uint64 `json:"harness_generation"`
			}
			if err := json.NewDecoder(r.Body).Decode(&identity); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if identity.StreamID != "stream" || identity.Generation != 3 {
				w.WriteHeader(409)
				return
			}
			retired++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	execution := &AgentExecution{SessionID: "session", RequiredNativeConversationID: "native", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 4, InterruptedSubmissionID: "old", InterruptedStreamID: "stream", InterruptedHarnessGeneration: 3}
	if err := retireInterruptedDeliverySubmission(context.Background(), client, execution); err != nil {
		t.Fatal(err)
	}
	if retired != 1 {
		t.Fatalf("retire calls=%d, want only observed submission", retired)
	}
	execution.InterruptedStreamID = "other-owner"
	if err := retireInterruptedDeliverySubmission(context.Background(), client, execution); err == nil {
		t.Fatal("retired mismatched submission")
	}
	if retired != 1 {
		t.Fatalf("retired mismatched submission: %d", retired)
	}
}
