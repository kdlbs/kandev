package linear

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestMockClient_DefaultsToSuccessfulAuth(t *testing.T) {
	m := NewMockClient()
	res, err := m.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("TestAuth: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true by default, got %+v", res)
	}
}

func TestMockClient_SetAuthResultIsReturned(t *testing.T) {
	m := NewMockClient()
	m.SetAuthResult(&TestConnectionResult{OK: false, Error: "401 Unauthorized"})
	res, _ := m.TestAuth(context.Background())
	if res.OK || res.Error != "401 Unauthorized" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestMockClient_TeamsAndStatesAreSeparate(t *testing.T) {
	m := NewMockClient()
	m.SetTeams([]LinearTeam{{ID: "t1", Key: "ENG", Name: "Engineering"}})
	m.SetStates("ENG", []LinearWorkflowState{{ID: "s1", Name: "Todo", Type: "unstarted"}})
	teams, _ := m.ListTeams(context.Background())
	if len(teams) != 1 || teams[0].Key != "ENG" {
		t.Fatalf("teams unexpected: %+v", teams)
	}
	states, _ := m.ListStates(context.Background(), "ENG")
	if len(states) != 1 || states[0].Name != "Todo" {
		t.Fatalf("states unexpected: %+v", states)
	}
	other, _ := m.ListStates(context.Background(), "DESIGN")
	if len(other) != 0 {
		t.Fatalf("states for unknown team should be empty, got %+v", other)
	}
}

func TestMockClient_GetIssueRoundtrip(t *testing.T) {
	m := NewMockClient()
	m.AddIssue(&LinearIssue{Identifier: "ENG-12", Title: "Test issue"})
	got, err := m.GetIssue(context.Background(), "ENG-12")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if got.Title != "Test issue" {
		t.Fatalf("unexpected title %q", got.Title)
	}
}

func TestMockClient_GetIssueUnknownReturns404(t *testing.T) {
	m := NewMockClient()
	_, err := m.GetIssue(context.Background(), "NOPE-1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %v", err)
	}
}

func TestMockClient_SetIssueStateRecordsCall(t *testing.T) {
	m := NewMockClient()
	if err := m.SetIssueState(context.Background(), "issue-id", "state-id"); err != nil {
		t.Fatalf("SetIssueState: %v", err)
	}
	calls := m.SetStateCalls()
	if len(calls) != 1 || calls[0].IssueID != "issue-id" || calls[0].StateID != "state-id" {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestMockClient_ResetClearsState(t *testing.T) {
	m := NewMockClient()
	m.AddIssue(&LinearIssue{Identifier: "X-1"})
	m.SetTeams([]LinearTeam{{Key: "X"}})
	m.SetAuthResult(&TestConnectionResult{OK: false, Error: "boom"})
	m.Reset()
	res, _ := m.TestAuth(context.Background())
	if !res.OK {
		t.Fatalf("Reset did not restore default auth result: %+v", res)
	}
	teams, _ := m.ListTeams(context.Background())
	if len(teams) != 0 {
		t.Fatalf("Reset did not clear teams")
	}
	if _, err := m.GetIssue(context.Background(), "X-1"); err == nil {
		t.Fatalf("Reset did not clear issues")
	}
}
