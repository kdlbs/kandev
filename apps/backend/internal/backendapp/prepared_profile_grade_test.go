package backendapp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

func TestPreparedProfileGradeAdapter(t *testing.T) {
	profile := &lifecycle.AgentProfileInfo{ProfileID: "profile-grade", AgentName: "test-agent", Model: "unrelated-model"}
	if err := json.Unmarshal([]byte(`{"ExecutionGrade":"Grade-A"}`), profile); err != nil {
		t.Fatal(err)
	}
	mgr := lifecycle.NewManager(nil, nil, nil, nil, fixedLifecycleProfileResolver{profile: profile}, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), newTestLogger())
	adapter := newLifecycleAdapter(mgr, nil, newTestLogger())
	info, err := adapter.ResolveAgentProfile(context.Background(), profile.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["ExecutionGrade"] != "Grade-A" || info.ProfileID != profile.ProfileID {
		t.Fatalf("adapter lost grade/identity: %s", data)
	}
}
