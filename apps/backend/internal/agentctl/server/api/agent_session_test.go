package api

import (
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestAgentSessionAssociationRoundTripsThroughAuthenticatedClient(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{
		AuthToken:                 "association-token",
		InstanceID:                "execution-1",
		SessionID:                 "session-1",
		DeliveryIncarnationID:     "incarnation-1",
		DeliveryHarnessGeneration: 7,
		WorkDir:                   t.TempDir(),
	}
	procMgr := process.NewManager(cfg, log)
	procMgr.SetStatusForTest(process.StatusRunning)
	procMgr.SetAdapterForTest(&promptErrorAdapter{sessionID: "native-1"})
	server := httptest.NewServer(NewServer(cfg, procMgr, nil, nil, log).Router())
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("parse server address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	client := agentctlclient.NewClient(host, port, log,
		agentctlclient.WithAuthToken(cfg.AuthToken),
		agentctlclient.WithExecutionID(cfg.InstanceID),
	)
	identity, err := client.GetAgentSessionAssociation(context.Background())
	if err != nil {
		t.Fatalf("GetAgentSessionAssociation: %v", err)
	}
	if identity.InstanceID != cfg.InstanceID || identity.SessionID != cfg.SessionID ||
		identity.IncarnationID != cfg.DeliveryIncarnationID ||
		identity.HarnessGeneration != cfg.DeliveryHarnessGeneration ||
		identity.NativeSessionID != "native-1" || identity.AgentStatus != string(process.StatusRunning) {
		t.Fatalf("identity = %#v", identity)
	}

	unauthorized := agentctlclient.NewClient(host, port, log,
		agentctlclient.WithAuthToken("wrong-token"),
		agentctlclient.WithExecutionID(cfg.InstanceID),
	)
	if _, err := unauthorized.GetAgentSessionAssociation(context.Background()); err == nil {
		t.Fatal("unauthenticated identity request succeeded")
	} else {
		var statusErr *agentctlclient.DeliveryHTTPError
		if !errors.As(err, &statusErr) || statusErr.StatusCode != 401 {
			t.Fatalf("unauthorized error = %v, want HTTP 401", err)
		}
	}
}
