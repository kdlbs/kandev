package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRuntimeBoundClientUsesSuccessorCredentialAndRetiresOldRequests(t *testing.T) {
	var oldRequests atomic.Int32
	var newRequests atomic.Int32
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldRequests.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer old-secret" {
			t.Errorf("old request Authorization = %q, want old credential", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer oldServer.Close()
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newRequests.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer new-secret" {
			t.Errorf("successor request Authorization = %q, want new credential", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer newServer.Close()

	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	oldHost, oldPort := splitTestServerHostPort(t, oldServer)
	oldCandidate, _ := owner.PrepareBinding()
	if err := oldCandidate.Configure(oldHost, oldPort, "old-secret", 11, nil); err != nil {
		t.Fatalf("configure old binding: %v", err)
	}
	if err := oldCandidate.Commit(); err != nil {
		t.Fatalf("commit old binding: %v", err)
	}
	oldLease, err := owner.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire old binding: %v", err)
	}
	oldClient := oldLease.NewInstanceClient(oldHost, oldPort, newTestLogger())
	oldLease.Close()
	if err := oldClient.Health(context.Background()); err != nil {
		t.Fatalf("old client initial health: %v", err)
	}

	if !owner.MarkUnavailableEpoch(oldCandidate.Epoch(), AvailabilityReasonAgentctlExited) {
		t.Fatal("retire old binding")
	}
	newHost, newPort := splitTestServerHostPort(t, newServer)
	newCandidate, _ := owner.PrepareBinding()
	if err := newCandidate.Configure(newHost, newPort, "new-secret", 22, nil); err != nil {
		t.Fatalf("configure successor binding: %v", err)
	}
	if err := newCandidate.Commit(); err != nil {
		t.Fatalf("commit successor binding: %v", err)
	}
	newLease, err := owner.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire successor binding: %v", err)
	}
	newClient := newLease.NewInstanceClient(newHost, newPort, newTestLogger())
	newLease.Close()
	if err := newClient.Health(context.Background()); err != nil {
		t.Fatalf("successor client health: %v", err)
	}
	if err := oldClient.Health(context.Background()); !errors.Is(err, ErrRuntimeLeaseRetired) {
		t.Fatalf("retired client health error = %v, want ErrRuntimeLeaseRetired", err)
	}
	if got := oldRequests.Load(); got != 1 {
		t.Fatalf("old server received %d requests, want only the initial request", got)
	}
	if got := newRequests.Load(); got != 1 {
		t.Fatalf("successor server received %d requests, want one", got)
	}
	oldClient.Close()
	newClient.Close()
	owner.Stop()
}

func TestRuntimeBoundClientCancelsInflightRequestOnRetirement(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	host, port := splitTestServerHostPort(t, server)
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	candidate, _ := owner.PrepareBinding()
	if err := candidate.Configure(host, port, "secret", 11, nil); err != nil {
		t.Fatalf("configure binding: %v", err)
	}
	if err := candidate.Commit(); err != nil {
		t.Fatalf("commit binding: %v", err)
	}
	lease, err := owner.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire binding: %v", err)
	}
	client := lease.NewInstanceClient(host, port, newTestLogger())
	lease.Close()
	requestDone := make(chan error, 1)
	go func() { requestDone <- client.Health(context.Background()) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not reach test server")
	}
	if !owner.MarkUnavailableEpoch(candidate.Epoch(), AvailabilityReasonAgentctlExited) {
		t.Fatal("retire binding")
	}
	select {
	case err := <-requestDone:
		if err == nil {
			t.Fatal("request succeeded after its runtime was retired")
		}
	case <-time.After(time.Second):
		t.Fatal("request was not canceled when its runtime was retired")
	}
	client.Close()
	owner.Stop()
}

func TestRuntimeBoundStreamClosesAndCancelsDispatchOnRetirement(t *testing.T) {
	accepted := make(chan struct{}, 1)
	serverClosed := make(chan struct{})
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		accepted <- struct{}{}
		_, _, _ = conn.ReadMessage()
		_ = conn.Close()
		close(serverClosed)
	}))
	defer server.Close()
	host, port := splitTestServerHostPort(t, server)
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	t.Cleanup(owner.Stop)
	candidate, _ := owner.PrepareBinding()
	if err := candidate.Configure(host, port, "secret", 11, nil); err != nil {
		t.Fatal(err)
	}
	if err := candidate.Commit(); err != nil {
		t.Fatal(err)
	}
	lease, err := owner.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	client := lease.NewBoundInstanceClient(port, newTestLogger())
	lease.Close()
	disconnected := make(chan error, 1)
	if err := client.StreamUpdates(context.Background(), nil, nil, func(err error) { disconnected <- err }); err != nil {
		t.Fatalf("open runtime-bound updates stream: %v", err)
	}
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("updates stream did not connect")
	}
	if !owner.MarkUnavailableEpoch(candidate.Epoch(), AvailabilityReasonAgentctlExited) {
		t.Fatal("retire runtime")
	}
	select {
	case <-serverClosed:
	case <-time.After(time.Second):
		t.Fatal("runtime retirement did not close the upstream websocket")
	}
	select {
	case err := <-disconnected:
		if err == nil {
			t.Fatal("updates stream disconnected without an error")
		}
	case <-time.After(time.Second):
		t.Fatal("runtime retirement did not cancel the updates read loop")
	}
	client.Close()
}
