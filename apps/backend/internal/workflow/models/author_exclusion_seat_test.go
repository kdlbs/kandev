package models

import (
	"reflect"
	"testing"
)

func seatEvents() StepEvents {
	return StepEvents{OnEnter: []OnEnterAction{
		{Type: OnEnterClearDecisions},
		{Type: OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": "reviewer"}},
		{Type: OnEnterEnsureParticipantSeat, Config: map[string]interface{}{
			"role": "approver", ExcludeAuthorProviderConfigKey: true, "extra": "kept",
		}},
		{Type: OnEnterEnsureParticipantSeat, Config: map[string]interface{}{}},
	}}
}

func TestParticipantSeatRoles(t *testing.T) {
	got := ParticipantSeatRoles(seatEvents())
	want := map[string]bool{"reviewer": false, "approver": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParticipantSeatRoles = %v, want %v", got, want)
	}
}

func TestSetExcludeAuthorProvider_EnablesOnlyTheMatchingRole(t *testing.T) {
	original := seatEvents()
	got, found := SetExcludeAuthorProvider(original, "reviewer", true)
	if !found {
		t.Fatal("expected the reviewer seat action to match")
	}
	if !ExcludesAuthorProvider(got.OnEnter[1].Config) {
		t.Fatal("reviewer seat action should set exclude_author_provider")
	}
	if got.OnEnter[2].Config["extra"] != "kept" || !ExcludesAuthorProvider(got.OnEnter[2].Config) {
		t.Fatalf("approver action changed: %v", got.OnEnter[2].Config)
	}
	if got.OnEnter[0].Type != OnEnterClearDecisions {
		t.Fatalf("other actions changed: %v", got.OnEnter[0])
	}
	if ExcludesAuthorProvider(original.OnEnter[1].Config) {
		t.Fatal("input events were mutated")
	}
}

func TestSetExcludeAuthorProvider_DisableRemovesTheKey(t *testing.T) {
	got, found := SetExcludeAuthorProvider(seatEvents(), "approver", false)
	if !found {
		t.Fatal("expected the approver seat action to match")
	}
	if _, ok := got.OnEnter[2].Config[ExcludeAuthorProviderConfigKey]; ok {
		t.Fatalf("disable should drop the key: %v", got.OnEnter[2].Config)
	}
	if got.OnEnter[2].Config["role"] != "approver" || got.OnEnter[2].Config["extra"] != "kept" {
		t.Fatalf("other config keys changed: %v", got.OnEnter[2].Config)
	}
}

func TestSetExcludeAuthorProvider_UnknownRoleMatchesNothing(t *testing.T) {
	got, found := SetExcludeAuthorProvider(seatEvents(), "observer", true)
	if found {
		t.Fatal("no seat action declares the observer role")
	}
	if !reflect.DeepEqual(got, seatEvents()) {
		t.Fatalf("events changed for an unmatched role: %v", got)
	}
}
