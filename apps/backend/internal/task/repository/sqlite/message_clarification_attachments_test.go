package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func seedAnswerAttachmentBundle(t *testing.T, repo *Repository) (taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	const workspaceID = "ws-answer-attachments"
	const task = "task-answer-attachments"
	const session = "session-answer-attachments"
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: task, WorkspaceID: workspaceID, Title: "Answer attachments",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: session, TaskID: task, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	base := time.Now().UTC().Add(-time.Minute)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-answer-attachments", TaskSessionID: session, TaskID: task,
		StartedAt: base, CreatedAt: base,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	for i, questionID := range []string{"q1", "q2"} {
		createClarificationBundleMessage(
			t, repo, "message-answer-"+questionID, task, session,
			"turn-answer-attachments", "pending-answer-attachments", questionID,
			base.Add(time.Duration(i)*time.Second),
		)
	}
	return task, session
}

func createAnswerAttachment(
	t *testing.T,
	repo *Repository,
	id, ownerID, workspaceID, name, mime, kind string,
	size int64,
) {
	t.Helper()
	attachment := &models.TaskMessageAttachment{
		ID: id, OwnerID: ownerID, WorkspaceID: workspaceID,
		Name: name, MimeType: mime, Kind: kind, DeliveryMode: "path",
		SizeBytes: size, StorageKey: id, State: models.AttachmentStateStaged,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := repo.CreateMessageAttachment(context.Background(), attachment); err != nil {
		t.Fatalf("CreateMessageAttachment(%s): %v", id, err)
	}
}

func TestCompleteActiveClarificationBundleWithAttachmentsAdoptsCanonicalSet(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	taskID, sessionID := seedAnswerAttachmentBundle(t, repo)
	createAnswerAttachment(t, repo, "att-image", "owner-1", "ws-answer-attachments", "screen.png", "image/png", "image", 123)
	createAnswerAttachment(t, repo, "att-log", "owner-1", "ws-answer-attachments", "log.txt", "text/plain", "resource", 45)

	responses := map[string]interface{}{
		"q1": map[string]interface{}{"question_id": "q1", "selected_options": []string{"opt-a"}},
		"q2": map[string]interface{}{"question_id": "q2", "custom_text": "look"},
	}
	messages, claimed, released, err := repo.CompleteActiveClarificationBundleWithAttachments(
		ctx, "pending-answer-attachments", "answered", responses,
		[]models.ClarificationAttachmentClaim{
			{QuestionID: "q1", AttachmentIDs: []string{"att-image"}},
			{QuestionID: "q2", AttachmentIDs: []string{"att-log"}},
		},
		models.ClarificationAttachmentScope{
			OwnerID: "owner-1", WorkspaceID: "ws-answer-attachments",
			TaskID: taskID, SessionID: sessionID,
		},
	)
	if err != nil {
		t.Fatalf("CompleteActiveClarificationBundleWithAttachments: %v", err)
	}
	if !claimed {
		t.Fatal("expected the bundle to be claimed")
	}
	if len(released) != 0 {
		t.Fatalf("released = %v, want none", released)
	}
	byQuestion := messagesByQuestionID(messages)
	assertManifest(t, byQuestion["q1"], "att-image")
	assertManifest(t, byQuestion["q2"], "att-log")
	response, ok := byQuestion["q1"].Metadata["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("q1 response shape = %#v", byQuestion["q1"].Metadata["response"])
	}
	encoded, err := json.Marshal(response["attachments"])
	if err != nil {
		t.Fatalf("marshal response attachments: %v", err)
	}
	var responseAttachments []models.ClarificationAnswerAttachment
	if err := json.Unmarshal(encoded, &responseAttachments); err != nil {
		t.Fatalf("decode response attachments: %v", err)
	}
	if len(responseAttachments) != 1 || responseAttachments[0].AttachmentID != "att-image" {
		t.Fatalf("q1 response attachments = %#v", responseAttachments)
	}

	image, err := repo.GetMessageAttachment(ctx, "att-image")
	if err != nil {
		t.Fatalf("GetMessageAttachment: %v", err)
	}
	if image.State != models.AttachmentStateClaimed || image.MessageID != byQuestion["q1"].ID ||
		image.TaskID != taskID || image.SessionID != sessionID {
		t.Fatalf("claimed image row = %+v", image)
	}

	// A retryable delivery failure keeps the manifest but clears the response.
	restored, ok, err := repo.RestoreActiveClarificationBundle(
		ctx, "pending-answer-attachments", "answered", messages,
	)
	if err != nil || !ok {
		t.Fatalf("RestoreActiveClarificationBundle = %v, %v", ok, err)
	}
	restoredByQuestion := messagesByQuestionID(restored)
	assertManifest(t, restoredByQuestion["q1"], "att-image")
	if _, hasResponse := restoredByQuestion["q1"].Metadata["response"]; hasResponse {
		t.Fatal("restored message retained a response")
	}

	// The retry removes q2's file; the repository reports it for release and
	// clears that question's manifest.
	retryResponses := map[string]interface{}{
		"q1": map[string]interface{}{"question_id": "q1", "selected_options": []string{"opt-a"}},
		"q2": map[string]interface{}{"question_id": "q2", "custom_text": "no file"},
	}
	retried, claimed, released, err := repo.CompleteActiveClarificationBundleWithAttachments(
		ctx, "pending-answer-attachments", "answered", retryResponses,
		[]models.ClarificationAttachmentClaim{
			{QuestionID: "q1", AttachmentIDs: []string{"att-image"}},
			{QuestionID: "q2"},
		},
		models.ClarificationAttachmentScope{
			OwnerID: "owner-1", WorkspaceID: "ws-answer-attachments",
			TaskID: taskID, SessionID: sessionID,
		},
	)
	if err != nil || !claimed {
		t.Fatalf("retry claim = %v, %v", claimed, err)
	}
	if len(released) != 1 || released[0] != "att-log" {
		t.Fatalf("released = %v, want [att-log]", released)
	}
	removed, getErr := repo.GetMessageAttachment(ctx, "att-log")
	if getErr != nil {
		t.Fatalf("GetMessageAttachment(att-log): %v", getErr)
	}
	if removed.State != models.AttachmentStateExpired {
		t.Fatalf("removed attachment state = %q, want expired", removed.State)
	}
	retriedByQuestion := messagesByQuestionID(retried)
	assertManifest(t, retriedByQuestion["q1"], "att-image")
	if _, ok := retriedByQuestion["q2"].Metadata["attachments"]; ok {
		t.Fatal("q2 retained a removed attachment manifest")
	}
}

func TestCompleteActiveClarificationBundleWithAttachmentsRollsBackMixedInvalid(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	taskID, sessionID := seedAnswerAttachmentBundle(t, repo)
	createAnswerAttachment(t, repo, "att-valid", "owner-1", "ws-answer-attachments", "valid.txt", "text/plain", "resource", 5)
	createAnswerAttachment(t, repo, "att-foreign", "owner-2", "ws-answer-attachments", "foreign.txt", "text/plain", "resource", 5)

	_, _, _, err := repo.CompleteActiveClarificationBundleWithAttachments(
		ctx, "pending-answer-attachments", "answered",
		map[string]interface{}{
			"q1": map[string]interface{}{"question_id": "q1", "selected_options": []string{"opt-a"}},
			"q2": map[string]interface{}{"question_id": "q2", "selected_options": []string{"opt-b"}},
		},
		[]models.ClarificationAttachmentClaim{
			{QuestionID: "q1", AttachmentIDs: []string{"att-valid"}},
			{QuestionID: "q2", AttachmentIDs: []string{"att-foreign"}},
		},
		models.ClarificationAttachmentScope{
			OwnerID: "owner-1", WorkspaceID: "ws-answer-attachments",
			TaskID: taskID, SessionID: sessionID,
		},
	)
	if err == nil {
		t.Fatal("expected mixed valid/foreign claim to fail")
	}
	valid, getErr := repo.GetMessageAttachment(ctx, "att-valid")
	if getErr != nil {
		t.Fatalf("GetMessageAttachment: %v", getErr)
	}
	if valid.State != models.AttachmentStateStaged || valid.MessageID != "" {
		t.Fatalf("valid attachment was partially adopted: %+v", valid)
	}
	messages, findErr := repo.FindMessagesByPendingID(ctx, "pending-answer-attachments")
	if findErr != nil {
		t.Fatalf("FindMessagesByPendingID: %v", findErr)
	}
	for _, message := range messages {
		if status, _ := message.Metadata["status"].(string); status != "pending" {
			t.Fatalf("message %s status = %q, want pending after rollback", message.ID, status)
		}
	}
}

func TestCompleteActiveClarificationBundleWithAttachmentsRejectsExcessCount(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	taskID, sessionID := seedAnswerAttachmentBundle(t, repo)
	ids := make([]string, 0, models.MaxMessageAttachmentCount+1)
	for i := 0; i <= models.MaxMessageAttachmentCount; i++ {
		id := "att-count-" + time.Duration(i).String()
		createAnswerAttachment(t, repo, id, "owner-1", "ws-answer-attachments", id+".txt", "text/plain", "resource", 1)
		ids = append(ids, id)
	}
	_, _, _, err := repo.CompleteActiveClarificationBundleWithAttachments(
		ctx, "pending-answer-attachments", "answered",
		map[string]interface{}{
			"q1": map[string]interface{}{"question_id": "q1", "selected_options": []string{"opt-a"}},
			"q2": map[string]interface{}{"question_id": "q2", "selected_options": []string{"opt-b"}},
		},
		[]models.ClarificationAttachmentClaim{
			{QuestionID: "q1", AttachmentIDs: ids},
			{QuestionID: "q2"},
		},
		models.ClarificationAttachmentScope{
			OwnerID: "owner-1", WorkspaceID: "ws-answer-attachments",
			TaskID: taskID, SessionID: sessionID,
		},
	)
	if err == nil {
		t.Fatal("expected excess attachment count to fail")
	}
}

func messagesByQuestionID(messages []*models.Message) map[string]*models.Message {
	out := make(map[string]*models.Message, len(messages))
	for _, message := range messages {
		questionID, _ := message.Metadata["question_id"].(string)
		out[questionID] = message
	}
	return out
}

func assertManifest(t *testing.T, message *models.Message, attachmentID string) {
	t.Helper()
	if message == nil {
		t.Fatalf("missing message for attachment %s", attachmentID)
	}
	encoded, err := json.Marshal(message.Metadata["attachments"])
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	var manifest []models.ClarificationAnswerAttachment
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest) != 1 || manifest[0].AttachmentID != attachmentID {
		t.Fatalf("manifest = %#v, want %s", manifest, attachmentID)
	}
}

func TestCompleteActiveClarificationBundleWithAttachmentsLosingCallerDoesNotAdopt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	taskID, sessionID := seedAnswerAttachmentBundle(t, repo)
	createAnswerAttachment(t, repo, "att-winner", "owner-1", "ws-answer-attachments", "winner.txt", "text/plain", "resource", 5)
	createAnswerAttachment(t, repo, "att-loser", "owner-1", "ws-answer-attachments", "loser.txt", "text/plain", "resource", 5)
	scope := models.ClarificationAttachmentScope{
		OwnerID: "owner-1", WorkspaceID: "ws-answer-attachments",
		TaskID: taskID, SessionID: sessionID,
	}
	_, claimed, _, err := repo.CompleteActiveClarificationBundleWithAttachments(
		ctx, "pending-answer-attachments", "answered",
		map[string]interface{}{
			"q1": map[string]interface{}{"question_id": "q1", "selected_options": []string{"opt-a"}},
			"q2": map[string]interface{}{"question_id": "q2", "selected_options": []string{"opt-b"}},
		},
		[]models.ClarificationAttachmentClaim{
			{QuestionID: "q1", AttachmentIDs: []string{"att-winner"}},
			{QuestionID: "q2"},
		},
		scope,
	)
	if err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}

	_, claimed, released, err := repo.CompleteActiveClarificationBundleWithAttachments(
		ctx, "pending-answer-attachments", "answered",
		map[string]interface{}{
			"q1": map[string]interface{}{"question_id": "q1", "selected_options": []string{"opt-b"}},
			"q2": map[string]interface{}{"question_id": "q2", "selected_options": []string{"opt-a"}},
		},
		[]models.ClarificationAttachmentClaim{
			{QuestionID: "q1", AttachmentIDs: []string{"att-loser"}},
			{QuestionID: "q2"},
		},
		scope,
	)
	if err != nil {
		t.Fatalf("losing claim: %v", err)
	}
	if claimed || len(released) != 0 {
		t.Fatalf("losing claim = claimed %v released %v", claimed, released)
	}
	loser, err := repo.GetMessageAttachment(ctx, "att-loser")
	if err != nil {
		t.Fatalf("GetMessageAttachment: %v", err)
	}
	if loser.State != models.AttachmentStateStaged || loser.MessageID != "" {
		t.Fatalf("losing file was adopted: %+v", loser)
	}
	winner, err := repo.GetMessageAttachment(ctx, "att-winner")
	if err != nil {
		t.Fatalf("GetMessageAttachment: %v", err)
	}
	if winner.State != models.AttachmentStateClaimed {
		t.Fatalf("winner file = %+v", winner)
	}
}

func TestCompleteActiveClarificationBundleWithAttachmentsPreservesMixedTerminalSibling(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const workspaceID = "ws-mixed-answer"
	const taskID = "task-mixed-answer"
	const sessionID = "session-mixed-answer"
	const pendingID = "pending-mixed-answer"
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Mixed"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	base := time.Now().UTC().Add(-time.Minute)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-mixed-answer", TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: base, CreatedAt: base,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	terminal := &models.Message{
		ID: "message-terminal", TaskSessionID: sessionID, TaskID: taskID,
		TurnID: "turn-mixed-answer", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeClarificationRequest, CreatedAt: base,
		Metadata: map[string]interface{}{
			"pending_id": pendingID, "question_id": "q1", "status": "answered",
			"response": map[string]interface{}{
				"question_id": "q1", "selected_options": []string{"opt-a"},
				"attachments": []models.ClarificationAnswerAttachment{{
					AttachmentID: "att-terminal", Type: "resource", Name: "terminal.txt",
					MimeType: "text/plain", SizeBytes: 5, DeliveryMode: "path",
				}},
			},
			"attachments": []models.ClarificationAnswerAttachment{{
				AttachmentID: "att-terminal", Type: "resource", Name: "terminal.txt",
				MimeType: "text/plain", SizeBytes: 5, DeliveryMode: "path",
			}},
		},
	}
	pendingMessage := &models.Message{
		ID: "message-pending", TaskSessionID: sessionID, TaskID: taskID,
		TurnID: "turn-mixed-answer", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeClarificationRequest, CreatedAt: base.Add(time.Second),
		Metadata: map[string]interface{}{
			"pending_id": pendingID, "question_id": "q2", "status": "pending",
		},
	}
	for _, message := range []*models.Message{terminal, pendingMessage} {
		if err := repo.CreateMessage(ctx, message); err != nil {
			t.Fatalf("CreateMessage(%s): %v", message.ID, err)
		}
	}
	terminalAttachment := &models.TaskMessageAttachment{
		ID: "att-terminal", OwnerID: "owner-1", WorkspaceID: workspaceID,
		TaskID: taskID, SessionID: sessionID, MessageID: terminal.ID,
		Name: "terminal.txt", MimeType: "text/plain", Kind: "resource",
		DeliveryMode: "path", SizeBytes: 5, StorageKey: "att-terminal",
		State: models.AttachmentStateClaimed, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := repo.CreateMessageAttachment(ctx, terminalAttachment); err != nil {
		t.Fatalf("CreateMessageAttachment(terminal): %v", err)
	}
	createAnswerAttachment(t, repo, "att-terminal-incoming", "owner-1", workspaceID, "incoming.txt", "text/plain", "resource", 5)
	createAnswerAttachment(t, repo, "att-pending", "owner-1", workspaceID, "pending.txt", "text/plain", "resource", 5)

	responses := map[string]interface{}{
		"q1": map[string]interface{}{"question_id": "q1", "selected_options": []string{"opt-a"}},
		"q2": map[string]interface{}{"question_id": "q2", "selected_options": []string{"opt-b"}},
	}
	messages, claimed, _, err := repo.CompleteActiveClarificationBundleWithAttachments(
		ctx, pendingID, "answered", responses,
		[]models.ClarificationAttachmentClaim{
			{QuestionID: "q1", AttachmentIDs: []string{"att-terminal-incoming"}},
			{QuestionID: "q2", AttachmentIDs: []string{"att-pending"}},
		},
		models.ClarificationAttachmentScope{
			OwnerID: "owner-1", WorkspaceID: workspaceID,
			TaskID: taskID, SessionID: sessionID,
		},
	)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	byQuestion := messagesByQuestionID(messages)
	if _, ok := byQuestion["q1"]; ok {
		t.Fatalf("terminal sibling was returned in the claimed set: %v", messageIDsForTest(messages))
	}
	if byQuestion["q2"] == nil {
		t.Fatal("pending sibling was not claimed")
	}
	assertManifest(t, byQuestion["q2"], "att-pending")

	storedTerminal, err := repo.FindMessageByPendingIDAndQuestion(ctx, sessionID, pendingID, "q1")
	if err != nil {
		t.Fatalf("FindMessageByPendingIDAndQuestion: %v", err)
	}
	assertManifest(t, storedTerminal, "att-terminal")
	if response, ok := storedTerminal.Metadata["response"].(map[string]interface{}); !ok ||
		response["question_id"] != "q1" {
		t.Fatalf("terminal response changed: %#v", storedTerminal.Metadata["response"])
	}
	incoming, err := repo.GetMessageAttachment(ctx, "att-terminal-incoming")
	if err != nil {
		t.Fatalf("GetMessageAttachment(incoming): %v", err)
	}
	if incoming.State != models.AttachmentStateStaged {
		t.Fatalf("terminal sibling incoming file was adopted: %+v", incoming)
	}
}

func messageIDsForTest(messages []*models.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func TestCompleteActiveClarificationBundleWithAttachmentsHandlesMixedSiblingsWithoutFiles(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const workspaceID = "ws-mixed-no-files"
	const taskID = "task-mixed-no-files"
	const sessionID = "session-mixed-no-files"
	const pendingID = "pending-mixed-no-files"
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Mixed"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	base := time.Now().UTC().Add(-time.Minute)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-mixed-no-files", TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: base, CreatedAt: base,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	for _, message := range []*models.Message{
		{
			ID: "message-terminal-no-files", TaskSessionID: sessionID, TaskID: taskID,
			TurnID: "turn-mixed-no-files", AuthorType: models.MessageAuthorAgent,
			Type: models.MessageTypeClarificationRequest, CreatedAt: base,
			Metadata: map[string]interface{}{
				"pending_id": pendingID, "question_id": "q1", "status": "answered",
				"response": map[string]interface{}{
					"question_id": "q1", "selected_options": []string{"opt-a"},
				},
			},
		},
		{
			ID: "message-pending-no-files", TaskSessionID: sessionID, TaskID: taskID,
			TurnID: "turn-mixed-no-files", AuthorType: models.MessageAuthorAgent,
			Type: models.MessageTypeClarificationRequest, CreatedAt: base.Add(time.Second),
			Metadata: map[string]interface{}{
				"pending_id": pendingID, "question_id": "q2", "status": "pending",
			},
		},
	} {
		if err := repo.CreateMessage(ctx, message); err != nil {
			t.Fatalf("CreateMessage(%s): %v", message.ID, err)
		}
	}
	messages, claimed, released, err := repo.CompleteActiveClarificationBundleWithAttachments(
		ctx, pendingID, "answered",
		map[string]interface{}{
			"q1": map[string]interface{}{"question_id": "q1", "selected_options": []string{"opt-a"}},
			"q2": map[string]interface{}{"question_id": "q2", "selected_options": []string{"opt-b"}},
		},
		[]models.ClarificationAttachmentClaim{{QuestionID: "q1"}, {QuestionID: "q2"}},
		models.ClarificationAttachmentScope{
			OwnerID: "owner-1", WorkspaceID: workspaceID,
			TaskID: taskID, SessionID: sessionID,
		},
	)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	if len(released) != 0 || len(messages) != 1 {
		t.Fatalf("claimed messages = %v, released = %v", messageIDsForTest(messages), released)
	}
	storedTerminal, err := repo.FindMessageByPendingIDAndQuestion(ctx, sessionID, pendingID, "q1")
	if err != nil {
		t.Fatalf("FindMessageByPendingIDAndQuestion: %v", err)
	}
	if status, _ := storedTerminal.Metadata["status"].(string); status != "answered" {
		t.Fatalf("terminal sibling status = %q", status)
	}
}
