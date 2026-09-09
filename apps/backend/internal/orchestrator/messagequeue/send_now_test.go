package messagequeue

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/entityrefs"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestBuildSendNowEnvelopeAggregatesInFIFOOrder(t *testing.T) {
	queuedAt := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	entries := []QueuedMessage{
		{
			ID: "first", SessionID: "session-1", TaskID: "task-1", Position: 10,
			Content: "first body", Model: "model-a", PlanMode: true,
			QueuedAt: queuedAt, QueuedBy: QueuedByUser,
			Attachments: []MessageAttachment{{Type: "image", Data: "aGVsbG8=", MimeType: "image/png", SizeBytes: 5}},
			Metadata: map[string]interface{}{
				MetadataEntityReferences: []apiv1.EntityReference{testEntityReference("1", "first")},
				"origin":                 "user-input",
			},
		},
		{
			ID: "second", SessionID: "session-1", TaskID: "task-2", Position: 20,
			Content: "second body", Model: "model-b", PlanMode: false,
			QueuedAt: queuedAt.Add(time.Minute), QueuedBy: QueuedByAgent,
			Attachments: []MessageAttachment{{Type: "image", Data: "d29ybGQ=", MimeType: "image/jpeg", SizeBytes: 5}},
			Metadata: map[string]interface{}{
				MetadataEntityReferences: []apiv1.EntityReference{
					testEntityReference("1", "duplicate"),
					testEntityReference("2", "second"),
				},
				MetadataSenderTaskID: "task-2",
			},
		},
	}

	envelope, err := BuildSendNowEnvelope(entries)
	if err != nil {
		t.Fatalf("BuildSendNowEnvelope() error = %v", err)
	}
	if envelope.SessionID != "session-1" || envelope.TaskID != "task-1" {
		t.Fatalf("envelope identity = (%q, %q), want oldest entry identity", envelope.SessionID, envelope.TaskID)
	}
	if envelope.Content != "first body\n\nsecond body" {
		t.Fatalf("envelope content = %q", envelope.Content)
	}
	if envelope.Model != "model-a" || !envelope.PlanMode || envelope.QueuedBy != QueuedByUser {
		t.Fatalf("envelope oldest fields = model %q, plan %t, queued_by %q", envelope.Model, envelope.PlanMode, envelope.QueuedBy)
	}
	if got := len(envelope.Attachments); got != 2 || envelope.Attachments[0].Data != "aGVsbG8=" || envelope.Attachments[1].Data != "d29ybGQ=" {
		t.Fatalf("envelope attachments = %#v", envelope.Attachments)
	}
	references := entityrefs.NormalizePersisted(envelope.Metadata[MetadataEntityReferences])
	if got := len(references); got != 2 || references[0].Ref != testEntityReference("1", "").Ref || references[1].Ref != testEntityReference("2", "").Ref {
		t.Fatalf("envelope references = %#v", references)
	}
	sources, ok := envelope.Metadata[MetadataSendNowSources].([]map[string]interface{})
	if !ok || len(sources) != 2 || sources[0]["id"] != "first" || sources[1]["queued_by"] != QueuedByAgent {
		t.Fatalf("envelope source provenance = %#v", envelope.Metadata[MetadataSendNowSources])
	}
}

func TestBuildSendNowEnvelopeAllowsAttachmentOnlyEntries(t *testing.T) {
	entries := []QueuedMessage{
		{ID: "first", SessionID: "session-1", Position: 1, Attachments: []MessageAttachment{{Data: "aA==", SizeBytes: 1}}},
		{ID: "second", SessionID: "session-1", Position: 2, Content: "body"},
	}

	envelope, err := BuildSendNowEnvelope(entries)
	if err != nil {
		t.Fatalf("BuildSendNowEnvelope() error = %v", err)
	}
	if envelope.Content != "body" {
		t.Fatalf("envelope content = %q, want body without a leading separator", envelope.Content)
	}
}

func TestBuildSendNowEnvelopeRejectsAggregateLimits(t *testing.T) {
	tests := []struct {
		name    string
		entries []QueuedMessage
		wantErr error
	}{
		{
			name: "attachments",
			entries: []QueuedMessage{{
				SessionID:   "session-1",
				Attachments: make([]MessageAttachment, 11),
			}},
			wantErr: ErrSendNowAttachmentOverflow,
		},
		{
			name: "references",
			entries: []QueuedMessage{
				{SessionID: "session-1", Metadata: map[string]interface{}{
					MetadataEntityReferences: makeEntityReferences(entityrefs.MaxReferencesPerMessage),
				}},
				{SessionID: "session-1", Metadata: map[string]interface{}{
					MetadataEntityReferences: makeEntityReferences(101)[100:],
				}},
			},
			wantErr: ErrSendNowReferenceOverflow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildSendNowEnvelope(tt.entries)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func testEntityReference(id, title string) apiv1.EntityReference {
	if title == "" {
		title = "Issue " + id
	}
	return apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      "mention:v1:github:issue:acme%2Frepo:" + id,
		Provider: "github",
		Kind:     "issue",
		ID:       id,
		Key:      "acme/repo#" + id,
		Title:    title,
		URL:      "https://github.com/acme/repo/issues/" + id,
		Scope:    "acme/repo",
	}
}

func makeEntityReferences(count int) []apiv1.EntityReference {
	references := make([]apiv1.EntityReference, count)
	for i := range references {
		id := strconv.Itoa(i)
		references[i] = testEntityReference(id, "Issue "+id)
	}
	return references
}
