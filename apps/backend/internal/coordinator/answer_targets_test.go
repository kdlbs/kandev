package coordinator

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func bundleMsg(qid, prompt string, index int, opts ...string) *taskmodels.Message {
	options := []any{}
	for _, o := range opts {
		options = append(options, map[string]any{"option_id": o, "label": o})
	}
	return &taskmodels.Message{ID: "m-" + qid, TaskSessionID: "s1", TaskID: "t1", Metadata: map[string]any{
		"question_id": qid, "question_index": index, "pending_id": "p1",
		"question": map[string]any{"id": qid, "prompt": prompt, "options": options},
	}}
}

func TestCurrentTargetsReadsQuestionsAndPermission(t *testing.T) {
	f := newRelayFixture(t, true)
	f.reader.page = &taskmodels.ClarificationBundlePage{Bundles: []taskmodels.ClarificationBundleSummary{{PendingID: "p1"}}}
	f.reader.messages = map[string][]*taskmodels.Message{"p1": {bundleMsg("q2", "Second?", 1, "x"), bundleMsg("q1", "First?", 0, "a", "b")}}
	meta := fullPermissionMeta()
	meta["action_type"] = "execute"
	meta["action_details"] = map[string]any{"command": "ls"}
	f.reader.interacts = []*taskmodels.Message{permissionMsg(meta)}

	set, err := f.svc.CurrentTargets(context.Background(), "ws-1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if set.Clarification == nil || set.Clarification.PendingID != "p1" || len(set.Clarification.Questions) != 2 ||
		set.Clarification.Questions[0].ID != "q1" || len(set.Clarification.Questions[0].Options) != 2 {
		t.Fatalf("clarification = %+v", set.Clarification)
	}
	p := set.Permission
	if p == nil || p.RequestID != "r1" || p.PendingID != "p1" || p.SessionID != "s1" || p.Title != "Run ls" ||
		len(p.Options) != 1 || p.Options[0].Kind != "allow_once" ||
		p.Summary != `{"action_details":{"command":"ls"},"action_type":"execute"}` {
		t.Fatalf("permission = %+v", p)
	}
}

func TestCurrentTargetsErrors(t *testing.T) {
	f := newRelayFixture(t, true)
	ctx := context.Background()
	if _, err := f.svc.CurrentTargets(ctx, "ws-1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing task: %v", err)
	}
	if _, err := f.svc.CurrentTargets(ctx, "ws-1", "tx"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign task: %v", err)
	}
	f.tasks.primary = map[string]string{}
	if set, err := f.svc.CurrentTargets(ctx, "ws-1", "t1"); err != nil || set.Clarification != nil || set.Permission != nil {
		t.Fatalf("no session: %+v %v", set, err)
	}
	f.tasks.primary = map[string]string{"t1": "s1"}
	f.reader.page = &taskmodels.ClarificationBundlePage{Bundles: []taskmodels.ClarificationBundleSummary{{PendingID: "p1"}}}
	if _, err := f.svc.CurrentTargets(ctx, "ws-1", "t1"); !errors.Is(err, ErrAnswerTargetRead) {
		t.Fatalf("bundle with no messages: %v", err)
	}
	f.reader.page = nil
	f.reader.interactErr = errors.New("boom")
	if _, err := f.svc.CurrentTargets(ctx, "ws-1", "t1"); !errors.Is(err, ErrAnswerTargetRead) {
		t.Fatalf("read failure: %v", err)
	}
}
