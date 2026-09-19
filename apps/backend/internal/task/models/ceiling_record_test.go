package models

import (
	"errors"
	"testing"
	"time"
)

func sampleDeferral() CeilingDeferral {
	return CeilingDeferral{
		Kind:       CeilingLaunchStartCreated,
		Payload:    map[string]interface{}{"session_id": "s1", "prompt": "do the thing"},
		Origin:     "automatic",
		ReasonCode: "ceiling",
		QueuedAt:   time.Date(2026, 9, 12, 8, 30, 0, 0, time.UTC),
	}
}

// TestCeilingLaunchKindsAreAClosedSet ranges over the canonical list so a kind
// added to the type without being classified is caught here rather than defaulting
// into the set.
func TestCeilingLaunchKindsAreAClosedSet(t *testing.T) {
	if len(AllCeilingLaunchKinds) != 7 {
		t.Fatalf("AllCeilingLaunchKinds has %d entries, want 7", len(AllCeilingLaunchKinds))
	}
	seen := map[CeilingLaunchKind]bool{}
	for _, kind := range AllCeilingLaunchKinds {
		if seen[kind] {
			t.Fatalf("%q appears twice in AllCeilingLaunchKinds", kind)
		}
		seen[kind] = true
		if !IsCeilingLaunchKind(kind) {
			t.Errorf("IsCeilingLaunchKind(%q) = false, want true", kind)
		}
	}
	for _, outside := range []CeilingLaunchKind{"", "start_task", "START", "relaunch"} {
		if IsCeilingLaunchKind(outside) {
			t.Errorf("IsCeilingLaunchKind(%q) = true, want false", outside)
		}
	}
}

// TestCeilingRecordKeysNestEveryReplayFieldUnderOneKey pins the confidentiality
// boundary: replay fields live under a single nested key, never spread across the
// record's top level, because that key is what the public projection redacts.
func TestCeilingRecordKeysNestEveryReplayFieldUnderOneKey(t *testing.T) {
	keys := CeilingRecordKeys(sampleDeferral())

	payload, ok := keys[CeilingLaunchPayloadKey].(map[string]interface{})
	if !ok {
		t.Fatalf("payload not nested under %s: %+v", CeilingLaunchPayloadKey, keys)
	}
	if payload["prompt"] != "do the thing" || payload["session_id"] != "s1" {
		t.Fatalf("payload lost its replay fields: %+v", payload)
	}
	for _, leaked := range []string{"prompt", "session_id"} {
		if _, present := keys[leaked]; present {
			t.Errorf("replay field %q leaked to the record's top level", leaked)
		}
	}
	if keys[CeilingDeferredKey] != true {
		t.Errorf("discriminator not set: %+v", keys)
	}
	if keys[CeilingLaunchKindKey] != string(CeilingLaunchStartCreated) {
		t.Errorf("launch kind = %v, want %q", keys[CeilingLaunchKindKey], CeilingLaunchStartCreated)
	}
	if keys[CeilingReasonCodeKey] != "ceiling" || keys[CeilingLaunchOriginKey] != "automatic" {
		t.Errorf("reason code or origin missing: %+v", keys)
	}
	if got := keys[CeilingQueuedAtKey]; got != "2026-09-12T08:30:00Z" {
		t.Errorf("ceiling_queued_at = %v, want RFC 3339 UTC", got)
	}
	for key := range keys {
		if !IsCeilingRecordKey(key) {
			t.Errorf("%q is not a ceiling-prefixed key; it would collide with the other writer", key)
		}
	}
}

// TestMergeCeilingRecordLeavesKeysItDoesNotOwn is the merge-not-replace rule, and
// the chain intent surviving it is the observable that matters.
func TestMergeCeilingRecordLeavesKeysItDoesNotOwn(t *testing.T) {
	existing := map[string]interface{}{
		DeferredLaunchStartWhenUnblockedKey: true,
		DeferredLaunchUserIDKey:             "user-9",
		DeferredLaunchRecordRecentUseKey:    true,
		"some_future_key":                   "keep me",
	}

	record, discarded, replaced := MergeCeilingRecord(existing, sampleDeferral())
	if replaced || discarded != nil {
		t.Fatalf("merging into an object must not replace it: replaced=%v discarded=%v", replaced, discarded)
	}
	for key, want := range existing {
		if record[key] != want {
			t.Errorf("merge dropped %q: got %v want %v", key, record[key], want)
		}
	}
	if record[CeilingDeferredKey] != true {
		t.Errorf("merge did not add the ceiling keys: %+v", record)
	}
	if !HasStartWhenUnblockedIntent(&Task{Metadata: map[string]interface{}{MetaKeyDeferredLaunch: record}}) {
		t.Error("HasStartWhenUnblockedIntent no longer reports the chain intent after a ceiling merge")
	}
	if !HasCeilingDeferredIntent(&Task{Metadata: map[string]interface{}{MetaKeyDeferredLaunch: record}}) {
		t.Error("HasCeilingDeferredIntent does not report the merged deferral")
	}
}

// TestMergeCeilingRecordReplacesANonObjectValue covers the three reachable
// malformed shapes. No reader can read an intent out of any of them, so replacing
// that one key is correct, and the discarded value is reported so it stays
// recoverable.
func TestMergeCeilingRecordReplacesANonObjectValue(t *testing.T) {
	for name, existing := range map[string]interface{}{
		"scalar": "nonsense",
		"array":  []interface{}{1, 2},
		"number": float64(7),
	} {
		t.Run(name, func(t *testing.T) {
			record, discarded, replaced := MergeCeilingRecord(existing, sampleDeferral())
			if !replaced {
				t.Fatalf("replaced = false for a %s value", name)
			}
			if discarded == nil {
				t.Error("the discarded value was not reported, so the corruption is unrecoverable from the log")
			}
			if record[CeilingDeferredKey] != true {
				t.Fatalf("replacement is not a well-formed ceiling record: %+v", record)
			}
		})
	}
}

func TestMergeCeilingRecordCreatesTheRecordWhenAbsent(t *testing.T) {
	record, discarded, replaced := MergeCeilingRecord(nil, sampleDeferral())
	if replaced || discarded != nil {
		t.Fatalf("an absent value is not a replacement: replaced=%v discarded=%v", replaced, discarded)
	}
	if record[CeilingLaunchKindKey] != string(CeilingLaunchStartCreated) {
		t.Fatalf("record not created: %+v", record)
	}
}

func TestReadCeilingDeferralRoundTrips(t *testing.T) {
	want := sampleDeferral()
	record, _, _ := MergeCeilingRecord(nil, want)

	got, err := ReadCeilingDeferral(record)
	if err != nil {
		t.Fatalf("ReadCeilingDeferral: %v", err)
	}
	if got.Kind != want.Kind || got.Origin != want.Origin || got.ReasonCode != want.ReasonCode {
		t.Fatalf("round trip lost discriminators: %+v", got)
	}
	if !got.QueuedAt.Equal(want.QueuedAt) {
		t.Fatalf("queued_at = %v, want %v", got.QueuedAt, want.QueuedAt)
	}
	if got.Payload["prompt"] != "do the thing" {
		t.Fatalf("round trip lost the payload: %+v", got.Payload)
	}
}

// TestReadCeilingDeferralRefusesAnUnknownKind is the criterion's point: an absent
// or unrecognised kind must not fall back to "start", which is the only kind an
// older reader understands, because that answers a resume with a fresh launch.
func TestReadCeilingDeferralRefusesAnUnknownKind(t *testing.T) {
	for name, kind := range map[string]interface{}{
		"absent":      nil,
		"unknown":     "teleport",
		"wrong case":  "START",
		"wrong type":  42,
		"future kind": "start_v2",
	} {
		t.Run(name, func(t *testing.T) {
			record := map[string]interface{}{
				CeilingDeferredKey:      true,
				CeilingLaunchPayloadKey: map[string]interface{}{"session_id": "s1"},
			}
			if kind != nil {
				record[CeilingLaunchKindKey] = kind
			}

			got, err := ReadCeilingDeferral(record)
			if err == nil {
				t.Fatalf("ReadCeilingDeferral accepted kind %v, returning %+v", kind, got)
			}
			var unreplayable *UnreplayableCeilingRecordError
			if !errors.As(err, &unreplayable) {
				t.Fatalf("error %v is not distinguishable as unreplayable, so it cannot be dropped rather than retried", err)
			}
			if got.Kind != "" {
				t.Fatalf("a rejected record still yielded kind %q", got.Kind)
			}
		})
	}
}

func TestReadCeilingDeferralRejectsARecordWithoutTheDiscriminator(t *testing.T) {
	if _, err := ReadCeilingDeferral(map[string]interface{}{
		DeferredLaunchStartWhenUnblockedKey: true,
	}); err == nil {
		t.Fatal("a WIP-only record was read as a ceiling deferral")
	}
	if _, err := ReadCeilingDeferral(nil); err == nil {
		t.Fatal("an absent record was read as a ceiling deferral")
	}
}
