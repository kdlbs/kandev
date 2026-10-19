package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionPreview(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, _ := seedRetentionBoundsStream(t, engine, "preview", 3, 3)
			before := measureRetentionStorage(t, repo)
			preview, err := repo.AnalyzeAgentDeliveryRetention(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || !preview.Available || preview.EventPayloadCandidates != 2 || preview.EventReceiptCandidates != 2 {
				t.Fatalf("bounded preview: %+v error=%v", preview, err)
			}
			after := measureRetentionStorage(t, repo)
			if after.InboxRows != before.InboxRows || after.RawPayloadBytes != before.RawPayloadBytes ||
				after.GenericEffects != before.GenericEffects || after.CompactRows != before.CompactRows ||
				after.CanonicalBytes != before.CanonicalBytes {
				t.Fatalf("preview changed recovery evidence: before=%+v after=%+v", before, after)
			}
			result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.EventPayloadsPruned != preview.EventPayloadCandidates || result.EventRowsRetired != preview.EventReceiptCandidates || result.PayloadBytesPruned != preview.EventPayloadBytes {
				t.Fatalf("live batch differs from preview: preview=%+v result=%+v error=%v", preview, result, err)
			}
		})
	}
}

func TestDeliveryRetentionPreviewBoundsDeletedHistory(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, phase := range []string{"inbox", "effects", "cursor_attributed_effects"} {
			t.Run(engine+"/"+phase, func(t *testing.T) {
				repo, events := seedRetentionBoundsStream(t, engine, "preview-deleted", 7, 0)
				initializeRetentionDeletionQueue(t, repo)
				ctx := context.Background()
				session, err := repo.GetTaskSession(ctx, events[0].SessionID)
				if err != nil {
					t.Fatal(err)
				}
				if err := repo.DeleteTaskSession(ctx, session); err != nil {
					t.Fatal(err)
				}
				if phase != "inbox" {
					if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM agent_delivery_inbox WHERE session_id = ?`), session.ID); err != nil {
						t.Fatal(err)
					}
				}
				if phase == "cursor_attributed_effects" {
					if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_effects SET session_id = '' WHERE session_id = ?`), session.ID); err != nil {
						t.Fatal(err)
					}
				}
				before := measureRetentionStorage(t, repo)
				preview, err := repo.AnalyzeAgentDeliveryRetention(ctx, models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1})
				if err != nil || !preview.Available || preview.RetiredSessionEvidenceRows != 1 || preview.RetiredSessionEvidenceBytes <= 0 || !preview.SampleLimitReached {
					t.Fatalf("deleted history must be a truncated one-row sample: %+v %v", preview, err)
				}
				after := measureRetentionStorage(t, repo)
				if after.InboxRows != before.InboxRows || after.RawPayloadBytes != before.RawPayloadBytes || after.GenericEffects != before.GenericEffects {
					t.Fatalf("read-only deleted preview mutated history: before=%+v after=%+v", before, after)
				}
			})
		}
	}
}
