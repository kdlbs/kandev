package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/notifications/models"
)

func TestFromProviderNormalizesNilEventsToEmptyArray(t *testing.T) {
	data, err := json.Marshal(FromProvider(&models.Provider{
		ID:   "provider-1",
		Name: "No subscriptions",
		Type: models.ProviderTypeApprise,
	}, nil))
	if err != nil {
		t.Fatalf("marshal provider: %v", err)
	}
	var decoded struct {
		Events []string `json:"events"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal provider: %v", err)
	}
	if decoded.Events == nil {
		t.Fatalf("events = null, want empty array: %s", data)
	}
	if len(decoded.Events) != 0 {
		t.Fatalf("events = %#v, want empty array", decoded.Events)
	}
}
