package plugins

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kdlbs/kandev/pluginsdk"
)

func mintLinkedReceipt(t *testing.T, f *linkedResponseFixture) pluginsdk.HumanResponseReceipt {
	t.Helper()
	f.link(t, "alice", "discord-alice")
	result, receipt, err := f.delegation.RequestHumanResponseReceipt(context.Background(), f.receiptRequest("discord-alice", "allow"))
	if err != nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("receipt = %+v err=%v", result, err)
	}
	return receipt
}

// The consumed marker is written after delivery. When that write is lost, the
// write-ahead claim still identifies the delivered receipt, so reads keep the
// attribution and repair the marker without repeating the native response.
func TestLinkedResponseAttributionSurvivesALostConsumedMarker(t *testing.T) {
	f := newLinkedResponseFixture(t, allLinkedGrants())
	receipt := mintLinkedReceipt(t, f)
	// The resolution lands after the receipt was claimed, as it does live.
	f.data.interactions.afterWrite.UpdatedAt = time.Now().UTC().Add(time.Second)
	if applied, _ := f.respond(f.interactions, "respond-1", receipt.ID, "allow"); applied.Status != pluginsdk.CommandApplied {
		t.Fatalf("respond = %+v", applied)
	}
	if _, err := f.connection.Exec(`UPDATE plugin_delegated_human_responses SET consumed_at = '' WHERE receipt_id = ?`, receipt.ID); err != nil {
		t.Fatalf("simulate a lost consumed marker: %v", err)
	}
	if _, err := f.store.ConsumedDelegatedResponse(context.Background(), f.interaction.ID); !errors.Is(err, state.ErrDelegatedResponseNotFound) {
		t.Fatalf("consumed lookup after the lost marker err = %v, want not found", err)
	}

	read, err := f.host.Interactions().Get(context.Background(), f.interaction.ID)
	want := pluginsdk.InteractionActor{UserID: "alice", DisplayName: "Display alice", Via: "discord"}
	if err != nil || read.ResolvedBy == nil || *read.ResolvedBy != want {
		t.Fatalf("attribution after the lost marker = %+v err=%v, want %+v", read, err, want)
	}
	if audit, err := f.store.ConsumedDelegatedResponse(context.Background(), f.interaction.ID); err != nil || audit.ReceiptID != receipt.ID {
		t.Fatalf("consumed marker after the read = %+v err=%v, want it repaired", audit, err)
	}
	if f.data.responder.permissionCalls != 1 {
		t.Fatalf("native responses = %d, want exactly the original delivery", f.data.responder.permissionCalls)
	}
}

// A response whose delivery failed leaves no claim behind, so a later
// resolution by someone else is never attributed to the linked user.
func TestLinkedResponseClaimIsReleasedWhenDeliveryFails(t *testing.T) {
	f := newLinkedResponseFixture(t, allLinkedGrants())
	receipt := mintLinkedReceipt(t, f)
	f.data.responder.err = errors.New("native responder failed")

	if failed, _ := f.respond(f.interactions, "respond-failed", receipt.ID, "allow"); failed.Status == pluginsdk.CommandApplied {
		t.Fatalf("respond = %+v, want a failure", failed)
	}
	if _, err := f.store.ClaimedDelegatedResponse(context.Background(), f.interaction.ID); !errors.Is(err, state.ErrDelegatedResponseNotFound) {
		t.Fatalf("claim after a failed delivery err = %v, want it released", err)
	}
	if _, err := f.store.ConsumedDelegatedResponse(context.Background(), f.interaction.ID); !errors.Is(err, state.ErrDelegatedResponseNotFound) {
		t.Fatalf("consumed marker after a failed delivery err = %v, want none", err)
	}
}

// Delivery needs the durable claim: with no audit row the response is refused
// before the native responder runs.
func TestLinkedResponseIsRefusedWhenTheClaimCannotBeRecorded(t *testing.T) {
	f := newLinkedResponseFixture(t, allLinkedGrants())
	receipt := mintLinkedReceipt(t, f)
	if _, err := f.connection.Exec(`DELETE FROM plugin_delegated_human_responses WHERE receipt_id = ?`, receipt.ID); err != nil {
		t.Fatalf("remove audit row: %v", err)
	}

	result, _ := f.respond(f.interactions, "respond-unaudited", receipt.ID, "allow")

	if result.Status != pluginsdk.CommandUnavailable || result.Reason != "human_response_audit_unavailable" || f.data.responder.permissionCalls != 0 {
		t.Fatalf("respond = %+v calls=%d, want a refusal before delivery", result, f.data.responder.permissionCalls)
	}
}
