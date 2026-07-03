package jira

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

func TestMockClient_GetTicketReturnsSeeded(t *testing.T) {
	m := NewMockClient()
	m.AddTicket(&JiraTicket{Key: "PROJ-12", Summary: "Hello"})
	got, err := m.GetTicket(context.Background(), "PROJ-12")
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if got.Summary != "Hello" {
		t.Fatalf("expected Hello, got %q", got.Summary)
	}
}

func TestMockClient_GetTicketUnknownKeyReturns404(t *testing.T) {
	m := NewMockClient()
	_, err := m.GetTicket(context.Background(), "NOPE-1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected APIError 404, got %v", err)
	}
}

func TestMockClient_SetGetTicketErrorOverridesLookup(t *testing.T) {
	m := NewMockClient()
	m.AddTicket(&JiraTicket{Key: "PROJ-1"})
	m.SetGetTicketError(&APIError{StatusCode: http.StatusUnauthorized, Message: "expired"})
	_, err := m.GetTicket(context.Background(), "PROJ-1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected forced 401, got %v", err)
	}
}

func TestMockClient_DoTransitionRecordsCall(t *testing.T) {
	m := NewMockClient()
	if err := m.DoTransition(context.Background(), "PROJ-1", "31"); err != nil {
		t.Fatalf("DoTransition: %v", err)
	}
	calls := m.TransitionCalls()
	if len(calls) != 1 || calls[0].TicketKey != "PROJ-1" || calls[0].TransitionID != "31" {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestMockClient_ProjectStatusesSeedAndRead(t *testing.T) {
	m := NewMockClient()
	m.SetProjectStatuses("CLIP", []JiraStatus{
		{ID: "1", Name: "In Development", StatusCategory: "indeterminate"},
	})
	got, err := m.ListProjectStatuses(context.Background(), "CLIP")
	if err != nil {
		t.Fatalf("ListProjectStatuses: %v", err)
	}
	if len(got) != 1 || got[0].Name != "In Development" {
		t.Fatalf("unexpected statuses: %+v", got)
	}
	// Unseeded project returns empty, not nil-panic.
	if s, _ := m.ListProjectStatuses(context.Background(), "OTHER"); len(s) != 0 {
		t.Fatalf("expected no statuses for unseeded project, got %+v", s)
	}
}

func TestMockClient_SearchTickets_FiltersByStatusName(t *testing.T) {
	m := NewMockClient()
	m.SetSearchHits([]JiraTicket{
		{Key: "CLIP-1", ProjectKey: "CLIP", StatusName: "In Development"},
		{Key: "CLIP-2", ProjectKey: "CLIP", StatusName: "Ready for review"},
		{Key: "CLIP-3", ProjectKey: "CLIP", StatusName: "In Development"},
	})
	res, err := m.SearchTickets(
		context.Background(),
		`project in ("CLIP") AND status in ("Ready for review") ORDER BY updated DESC`,
		"",
		50,
	)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Tickets) != 1 || res.Tickets[0].Key != "CLIP-2" {
		t.Fatalf("expected only CLIP-2, got %+v", res.Tickets)
	}
}

func TestMockClient_SearchTickets_FiltersByProjectKey(t *testing.T) {
	m := NewMockClient()
	m.SetSearchHits([]JiraTicket{
		{Key: "CLIP-1", ProjectKey: "CLIP", StatusName: "In Development"},
		{Key: "OPS-9", ProjectKey: "OPS", StatusName: "In Development"},
	})
	res, err := m.SearchTickets(
		context.Background(),
		`project in ("CLIP") AND assignee = currentUser() ORDER BY updated DESC`,
		"",
		50,
	)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Tickets) != 1 || res.Tickets[0].Key != "CLIP-1" {
		t.Fatalf("expected only CLIP-1, got %+v", res.Tickets)
	}
}

func TestMockClient_ResetClearsState(t *testing.T) {
	m := NewMockClient()
	m.AddTicket(&JiraTicket{Key: "X-1"})
	m.SetProjectStatuses("CLIP", []JiraStatus{{ID: "1", Name: "Open"}})
	m.SetAuthResult(&TestConnectionResult{OK: false, Error: "boom"})
	m.Reset()
	if s, _ := m.ListProjectStatuses(context.Background(), "CLIP"); len(s) != 0 {
		t.Fatalf("Reset did not clear statuses: %+v", s)
	}
	res, _ := m.TestAuth(context.Background())
	if !res.OK {
		t.Fatalf("Reset did not restore default auth result: %+v", res)
	}
	if _, err := m.GetTicket(context.Background(), "X-1"); err == nil {
		t.Fatalf("Reset did not clear tickets")
	}
}
