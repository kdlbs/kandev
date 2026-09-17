package engine_dispatcher

import (
	"context"
	"github.com/kandev/kandev/internal/workflow/engine"
	"testing"
)

func TestConversationDispatchDoesNotRequireDeliverySession(t *testing.T) {
	d := &Dispatcher{sessions: &fakeSessions{activeErr: context.Canceled}}
	d.SetConversationHandler(func(_ context.Context, id string, _ engine.Trigger, _ any, op string) (bool, error) {
		if id != "conversation" || op != "comment-1" {
			t.Fatal("lost event identity")
		}
		return true, nil
	})
	handled, err := d.HandleTriggerHandled(context.Background(), "conversation", engine.TriggerOnComment, nil, "comment-1")
	if err != nil || !handled {
		t.Fatalf("standing conversation: handled=%v err=%v", handled, err)
	}
}
