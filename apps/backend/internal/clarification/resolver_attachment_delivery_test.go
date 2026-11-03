package clarification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type attachmentAwareDeliveryCreator struct {
	*resolverDeliveryMessageCreator
	attachment taskmodels.ClarificationAnswerAttachment
}

func (c *attachmentAwareDeliveryCreator) CompleteActiveClarificationBundleWithAttachments(
	ctx context.Context,
	pendingID, status string,
	responses map[string]interface{},
	_ []taskmodels.ClarificationAttachmentClaim,
) ([]*taskmodels.Message, bool, error) {
	messages, claimed, err := c.CompleteActiveClarificationBundle(ctx, pendingID, status, responses)
	if err != nil || !claimed {
		return messages, claimed, err
	}
	manifest := []taskmodels.ClarificationAnswerAttachment{c.attachment}
	for _, message := range messages {
		message.Metadata["attachments"] = manifest
		if response, ok := message.Metadata["response"].(map[string]any); ok {
			response["attachments"] = manifest
		}
		stored := resolverMessageByID(c.repo.messages[pendingID], message.ID)
		if stored != nil {
			stored.Metadata["attachments"] = manifest
			if response, ok := stored.Metadata["response"].(map[string]any); ok {
				response["attachments"] = manifest
			}
		}
	}
	return messages, true, nil
}

type recordingAnswerMaterializer struct {
	delay            time.Duration
	started          chan struct{}
	deadlineRemained time.Duration
	err              error
	paths            map[string]string
}

func (m *recordingAnswerMaterializer) MaterializeAnswerAttachments(
	ctx context.Context,
	_ string,
	_ []taskmodels.ClarificationAnswerAttachment,
) (map[string]string, error) {
	if deadline, ok := ctx.Deadline(); ok {
		m.deadlineRemained = time.Until(deadline)
	}
	if m.started != nil {
		close(m.started)
	}
	if m.err != nil {
		return nil, m.err
	}
	timer := time.NewTimer(m.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		if m.paths != nil {
			return m.paths, nil
		}
		return map[string]string{"att-1": "/workspace/.kandev/attachments/session/screen.png"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestMaterializationUsesPostClaimDeliveryBudget(t *testing.T) {
	const pendingID = "pending-post-claim-materialization"
	message := resolverDeliveryMessage(pendingID, "message-post-claim", "turn-1")
	store := NewStore(time.Minute)
	repo := &stubMessageStore{
		messages:      map[string][]*taskmodels.Message{pendingID: {message}},
		currentTurnID: "turn-1",
	}
	baseCreator := &resolverDeliveryMessageCreator{repo: repo}
	creator := &attachmentAwareDeliveryCreator{
		resolverDeliveryMessageCreator: baseCreator,
		attachment: taskmodels.ClarificationAnswerAttachment{
			AttachmentID: "att-1", Type: "image", Name: "screen.png",
			MimeType: "image/png", SizeBytes: 123, DeliveryMode: answerAttachmentDeliveryModePath,
		},
	}
	eventBus := &stubEventBus{}
	resolver := NewResolver(store, repo, creator, &stubAuthorizer{}, eventBus, eventBus, nil, logger.Default())
	materializer := &recordingAnswerMaterializer{delay: 80 * time.Millisecond}
	resolver.SetAnswerAttachmentMaterializer(materializer)
	store.CreateRequest(&Request{
		PendingID: pendingID, SessionID: "session-1", TaskID: "task-1",
		Questions: []Question{{
			ID: "q1", Prompt: "Continue?",
			Options: []Option{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}},
		}},
	})
	t.Cleanup(func() { store.CancelRequest(pendingID) })
	waitDone := make(chan error, 1)
	go func() {
		_, err := store.WaitForResponse(context.Background(), pendingID)
		waitDone <- err
	}()

	resolution, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, Outcome{Answers: []Answer{{
		QuestionID:      "q1",
		SelectedOptions: []string{"yes"},
		Attachments: []AnswerAttachment{{
			AttachmentID: "att-1", Type: "image", Name: "screen.png",
			MimeType: "image/png", SizeBytes: 123, DeliveryMode: answerAttachmentDeliveryModePath,
		}},
	}}})
	if err != nil || !claimed || resolution == nil {
		t.Fatalf("ResolveBundle = %#v, %v, %v", resolution, claimed, err)
	}
	// The pre-claim gate is generous; the materializer must receive a strictly
	// longer post-claim budget instead of inheriting the pre-claim deadline.
	if materializer.deadlineRemained <= clarificationPreClaimTimeout+time.Second {
		t.Fatalf("materialization deadline = %v, want the post-claim delivery budget", materializer.deadlineRemained)
	}
	if len(resolution.Response.Answers) != 1 ||
		len(resolution.Response.Answers[0].Attachments) != 1 ||
		resolution.Response.Answers[0].Attachments[0].Path == "" {
		t.Fatalf("resolution attachments = %#v", resolution.Response.Answers)
	}
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("waiter: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter did not receive the response")
	}
}

func TestMaterializationFailureRestoresClaim(t *testing.T) {
	const pendingID = "pending-materialization-restore"
	message := resolverDeliveryMessage(pendingID, "message-materialization-restore", "turn-1")
	store := NewStore(time.Minute)
	repo := &stubMessageStore{
		messages:      map[string][]*taskmodels.Message{pendingID: {message}},
		currentTurnID: "turn-1",
	}
	baseCreator := &resolverDeliveryMessageCreator{repo: repo}
	creator := &attachmentAwareDeliveryCreator{
		resolverDeliveryMessageCreator: baseCreator,
		attachment: taskmodels.ClarificationAnswerAttachment{
			AttachmentID: "att-1", Type: "image", Name: "screen.png",
			MimeType: "image/png", SizeBytes: 123, DeliveryMode: answerAttachmentDeliveryModePath,
		},
	}
	eventBus := &stubEventBus{}
	resolver := NewResolver(store, repo, creator, &stubAuthorizer{}, eventBus, eventBus, nil, logger.Default())
	resolver.SetAnswerAttachmentMaterializer(&recordingAnswerMaterializer{err: errors.New("transfer failed")})
	store.CreateRequest(&Request{
		PendingID: pendingID, SessionID: "session-1", TaskID: "task-1",
		Questions: []Question{{
			ID: "q1", Prompt: "Continue?",
			Options: []Option{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}},
		}},
	})
	t.Cleanup(func() { store.CancelRequest(pendingID) })
	waitDone := make(chan error, 1)
	go func() {
		_, err := store.WaitForResponse(context.Background(), pendingID)
		waitDone <- err
	}()

	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, Outcome{Answers: []Answer{{
		QuestionID:      "q1",
		SelectedOptions: []string{"yes"},
		Attachments: []AnswerAttachment{{
			AttachmentID: "att-1", Type: "image", Name: "screen.png",
			MimeType: "image/png", SizeBytes: 123, DeliveryMode: answerAttachmentDeliveryModePath,
		}},
	}}})
	if !claimed || err == nil {
		t.Fatalf("ResolveBundle = claimed %v, err %v", claimed, err)
	}
	if baseCreator.restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1", baseCreator.restoreCalls)
	}
	detail := repo.messages[pendingID][0]
	if status := stringFromMetadata(detail.Metadata, metaStatusKey); status != string(StatusPending) {
		t.Fatalf("restored status = %q", status)
	}
	// Manifest preservation on restore is repository behavior, covered by
	// TestCompleteActiveClarificationBundleWithAttachmentsAdoptsCanonicalSet.
	store.CancelRequest(pendingID)
	select {
	case <-waitDone:
	case <-time.After(time.Second):
	}
}

func TestMaterializationRejectsSupersededConversationalTurn(t *testing.T) {
	const pendingID = "pending-materialization-superseded"
	message := resolverDeliveryMessage(pendingID, "message-materialization-superseded", "turn-1")
	store := NewStore(time.Minute)
	repo := &stubMessageStore{
		messages:      map[string][]*taskmodels.Message{pendingID: {message}},
		currentTurnID: "turn-2",
	}
	baseCreator := &resolverDeliveryMessageCreator{repo: repo}
	creator := &attachmentAwareDeliveryCreator{
		resolverDeliveryMessageCreator: baseCreator,
		attachment: taskmodels.ClarificationAnswerAttachment{
			AttachmentID: "att-1", Type: "image", Name: "screen.png",
			MimeType: "image/png", SizeBytes: 123, DeliveryMode: answerAttachmentDeliveryModePath,
		},
	}
	eventBus := &stubEventBus{}
	resolver := NewResolver(store, repo, creator, &stubAuthorizer{}, eventBus, eventBus, nil, logger.Default())
	resolver.SetAnswerAttachmentMaterializer(&recordingAnswerMaterializer{
		paths: map[string]string{"att-1": "/workspace/.kandev/attachments/session/screen.png"},
	})
	store.CreateRequest(&Request{
		PendingID: pendingID, SessionID: "session-1", TaskID: "task-1",
		Questions: []Question{{
			ID: "q1", Prompt: "Continue?",
			Options: []Option{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}},
		}},
	})
	t.Cleanup(func() { store.CancelRequest(pendingID) })
	go func() { _, _ = store.WaitForResponse(context.Background(), pendingID) }()

	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, Outcome{Answers: []Answer{{
		QuestionID:      "q1",
		SelectedOptions: []string{"yes"},
		Attachments: []AnswerAttachment{{
			AttachmentID: "att-1", Type: "image", Name: "screen.png",
			MimeType: "image/png", SizeBytes: 123, DeliveryMode: answerAttachmentDeliveryModePath,
		}},
	}}})
	if !claimed || err == nil {
		t.Fatalf("ResolveBundle = claimed %v, err %v", claimed, err)
	}
	if baseCreator.restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1", baseCreator.restoreCalls)
	}
}
