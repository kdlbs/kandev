package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateInstanceMaps409ToConflictError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"instance_cleaning_up"}`))
	}))
	defer srv.Close()
	_, err := newTestControlClient(t, srv).CreateInstance(context.Background(), &CreateInstanceRequest{ID: "i"})
	var conflict *CreateConflictError
	if !errors.As(err, &conflict) || conflict.Code != "instance_cleaning_up" {
		t.Fatalf("err = %v, want conflict instance_cleaning_up", err)
	}
}

func TestCreateInstanceRequestRedactsTokenInFormattedOutput(t *testing.T) {
	req := CreateInstanceRequest{ID: "i", SessionMCPToken: "secret-token-value"}
	for _, out := range []string{req.String(), strings.TrimSpace(fmt.Sprint(req)), fmt.Sprintf("%#v", req), fmt.Sprintf("%+v", &req)} {
		if strings.Contains(out, "secret-token-value") {
			t.Fatalf("token leaked in %q", out)
		}
	}
}

func TestDeleteInstanceMatchingSendsPreconditionHeaderOnlyWhenSet(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get(expectedTokenFingerprintHeader))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newTestControlClient(t, srv)
	if err := c.DeleteInstanceMatching(context.Background(), "i", "fp-1"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteInstanceMatching(context.Background(), "i", ""); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "fp-1" || got[1] != "" {
		t.Fatalf("headers = %q", got)
	}
}

func TestRevokeMCPTokenReportsStatus(t *testing.T) {
	status := http.StatusOK
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.Method + " " + r.URL.Path
		w.WriteHeader(status)
	}))
	defer srv.Close()
	c := newTestControlClient(t, srv)
	if err := c.RevokeMCPToken(context.Background(), "abc"); err != nil || path != "POST /api/v1/instances/abc/mcp-token/revoke" {
		t.Fatalf("err=%v path=%q", err, path)
	}
	status = http.StatusNotFound
	if err := c.RevokeMCPToken(context.Background(), "abc"); StatusOf(err) != http.StatusNotFound {
		t.Fatalf("err = %v, want 404 status error", err)
	}
}

func TestGetInstanceIdentityDecodesFingerprintsAndCapabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"capabilities":["session_mcp_token"],"mcp_token_fingerprint":"f1","broker_upstream_fingerprint":"b1"}`))
	}))
	defer srv.Close()
	host, port := splitTestServerHostPort(t, srv)
	id, err := NewClient(host, port, newTestLogger()).GetInstanceIdentity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !id.HasCapability("session_mcp_token") || id.HasCapability("other") || id.MCPTokenFingerprint != "f1" || id.BrokerUpstreamFingerprint != "b1" {
		t.Fatalf("identity = %+v", id)
	}
}

func TestCreateInstanceOtherConflictsKeepGenericError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"port range exhausted"}`))
	}))
	defer srv.Close()
	_, err := newTestControlClient(t, srv).CreateInstance(context.Background(), &CreateInstanceRequest{ID: "i"})
	var conflict *CreateConflictError
	if err == nil || errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a generic create failure", err)
	}
}
