package instance

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestInferenceOwnerFreshnessRequiresMintedInstanceID(t *testing.T) {
	for _, id := range []string{"", "caller-supplied"} {
		t.Run(id, func(t *testing.T) {
			manager := NewManager(&config.Config{Ports: config.PortConfig{Base: 0, Max: 0}}, newTestLogger(t))
			admission, err := manager.admitCreate(context.Background(), &CreateRequest{ID: id, WorkspacePath: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer manager.activeCreations.Done()
			defer func() {
				if err := manager.abandonPartialInstance(admission.bundle); err != nil {
					t.Error(err)
				}
			}()
			if admission.freshInferenceOwner != (id == "") {
				t.Fatalf("fresh=%t caller id=%q", admission.freshInferenceOwner, id)
			}
		})
	}
	cfg := config.InstanceConfig{}
	if err := json.Unmarshal([]byte(`{"InferenceOwnerFresh":true,"inference_owner_fresh":true}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.InferenceOwnerFresh {
		t.Fatal("external configuration minted trusted freshness")
	}
}
