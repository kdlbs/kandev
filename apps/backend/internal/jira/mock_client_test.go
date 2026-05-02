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

func TestMockClient_ResetClearsState(t *testing.T) {
	m := NewMockClient()
	m.AddTicket(&JiraTicket{Key: "X-1"})
	m.SetAuthResult(&TestConnectionResult{OK: false, Error: "boom"})
	m.Reset()
	res, _ := m.TestAuth(context.Background())
	if !res.OK {
		t.Fatalf("Reset did not restore default auth result: %+v", res)
	}
	if _, err := m.GetTicket(context.Background(), "X-1"); err == nil {
		t.Fatalf("Reset did not clear tickets")
	}
}
