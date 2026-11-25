package lifecycle

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/sshconnections"
	"github.com/kandev/kandev/internal/sshtransport"
	"github.com/kandev/kandev/internal/task/models"
	"golang.org/x/crypto/ssh"
	"strconv"
	"testing"
)

type managedSSHTestResolver struct {
	admitted, dialed bool
	dialErr          error
}

func (m *managedSSHTestResolver) AdmitSavedRuntime(ctx context.Context, session string) (sshconnections.RuntimeSnapshot, error) {
	return m.AdmitRuntime(ctx, session, "connection", "")
}

func (m *managedSSHTestResolver) AdmitRuntime(_ context.Context, session, id, stamp string) (sshconnections.RuntimeSnapshot, error) {
	m.admitted = true
	return sshconnections.RuntimeSnapshot{SessionID: session, Route: sshconnections.Route{Destination: sshconnections.Connection{ID: id, Host: "saved.example", Port: 22, User: "saved", Fingerprint: "saved-pin"}, Stamp: "recorded"}, IdentityGeneration: "generation"}, nil
}
func (m *managedSSHTestResolver) DialRuntime(_ context.Context, s sshconnections.RuntimeSnapshot) (*ssh.Client, error) {
	m.dialed = true
	return nil, m.dialErr
}

func TestManagedSSHNeverFallsBackToInlineTarget(t *testing.T) {
	r := &SSHExecutor{}
	md := map[string]interface{}{"ssh_connection_id": "connection", "ssh_host": "attacker", "ssh_host_fingerprint": "forged"}
	if _, _, err := r.dialSession(context.Background(), "session", md, true); !errors.Is(err, sshconnections.ErrDisabled) {
		t.Fatalf("disabled selector fell back: %v", err)
	}
	resolver := &managedSSHTestResolver{}
	r.SetManagedConnections(resolver)
	target, _, err := r.dialSession(context.Background(), "session", md, true)
	if err != nil || !resolver.admitted || !resolver.dialed || target.Host != "saved.example" {
		t.Fatalf("managed target not used: %v %v", target, err)
	}
	if md["ssh_route_stamp"] != "recorded" || md["ssh_identity_generation"] != "generation" {
		t.Fatal("snapshot missing")
	}
	resolver.dialed = false
	md["ssh_identity_generation"] = "stale"
	if _, _, err = r.dialSession(context.Background(), "session", md, false); !errors.Is(err, sshconnections.ErrConflict) || resolver.dialed {
		t.Fatalf("stale identity reconnected: %v", err)
	}
}

func TestManagedSSHFailedAdmissionCanRetry(t *testing.T) {
	resolver := &managedSSHTestResolver{dialErr: errors.New("fixture dial")}
	r := &SSHExecutor{}
	r.SetManagedConnections(resolver)
	md := map[string]interface{}{"ssh_connection_id": "connection"}
	if _, _, err := r.dialSession(context.Background(), "session", md, true); err == nil {
		t.Fatal("expected dial failure")
	}
	if getMetadataString(md, MetadataKeySSHRouteStamp) != "" {
		t.Fatal("failed admission left a resume stamp without a retained runtime")
	}
	if getMetadataString(md, MetadataKeySSHIdentityGeneration) != "" {
		t.Fatal("failed admission left an identity generation")
	}
	md[MetadataKeySSHRouteStamp] = "recorded"
	md[MetadataKeySSHIdentityGeneration] = "generation"
	if _, _, err := r.dialSession(context.Background(), "session", md, false); err == nil {
		t.Fatal("expected reconnect failure")
	}
	if getMetadataString(md, MetadataKeySSHRouteStamp) != "recorded" {
		t.Fatal("reconnect failure discarded retained snapshot")
	}
}

func TestManagedSSHStartupRecoveryUsesRetainedIdentityWithoutLegacyFallback(t *testing.T) {
	for _, tc := range []struct {
		name       string
		stamp      string
		generation string
		disabled   bool
		wantDial   bool
	}{
		{name: "retained", stamp: "recorded", generation: "generation", wantDial: true},
		{name: "missing stamp", generation: "generation"},
		{name: "stale generation", stamp: "recorded", generation: "stale"},
		{name: "disabled", stamp: "recorded", generation: "generation", disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeSSHServer(t, func(_, _ string) sshExecResult {
				return sshExecResult{ExitCode: 1}
			})
			md := sshConnectionMetadata(t, server)
			md[MetadataKeySSHConnectionID] = "connection"
			md[MetadataKeySSHRouteStamp] = tc.stamp
			md[MetadataKeySSHIdentityGeneration] = tc.generation
			md[MetadataKeySSHRemoteAgentctlPID] = "4242"
			md[MetadataKeySSHRemoteAgentctlPort] = "41234"
			md[MetadataKeySSHRemoteSessionDir] = "/remote/session"
			md[MetadataKeySSHRemoteTaskDir] = "/remote/task"
			md[MetadataKeySSHAgentctlInstanceID] = "native-execution"
			record := &models.ExecutorRunning{
				ID: "session", SessionID: "session", TaskID: "task",
				AgentExecutionID: "execution", Runtime: agentruntime.RuntimeSSH,
				PID: 4242, TransientAuthToken: "fixture-auth", Metadata: md,
			}
			resolver := &managedSSHTestResolver{dialErr: errors.New("fixture unavailable")}
			executor := NewSSHExecutor(nil, nil, nil, newTestLogger())
			if !tc.disabled {
				executor.SetManagedConnections(resolver)
			}
			instances, outcomes, err := executor.RecoverInstancesDetailed(
				context.Background(), []*models.ExecutorRunning{record},
			)
			if err != nil || len(instances) != 0 || outcomes["session"] != RecoveryOutcomeUnknown {
				t.Fatalf("unavailable recovery must remain unknown: %v %v", outcomes, err)
			}
			if resolver.dialed != tc.wantDial {
				t.Fatalf("managed resolver dialed = %v, want %v", resolver.dialed, tc.wantDial)
			}
			if commands := server.commands(); len(commands) != 0 {
				t.Fatalf("managed recovery used inline legacy credentials: %v", commands)
			}
		})
	}
}

func TestLegacySSHNewLaunchCannotDialInlineRoute(t *testing.T) {
	server := newFakeSSHServer(t, func(_, _ string) sshExecResult {
		return sshExecResult{ExitCode: 1}
	})
	executor := NewSSHExecutor(nil, nil, nil, newTestLogger())
	_, client, err := executor.dialSession(context.Background(), "session", sshConnectionMetadata(t, server), true)
	if client != nil {
		_ = client.Close()
	}
	if !errors.Is(err, sshconnections.ErrInvalidRoute) {
		t.Fatalf("new inline launch must fail before dialing, got %v", err)
	}
}

type retainedWireSSHTestResolver struct {
	*sshconnections.Service
}

func (r retainedWireSSHTestResolver) DialRetainedLegacySession(ctx context.Context, _ string, target sshtransport.SSHTarget) (*ssh.Client, error) {
	return r.DialRetainedLegacy(ctx, target)
}

func newRetainedWireSSHTestExecutor(log *logger.Logger) *SSHExecutor {
	executor := NewSSHExecutor(nil, nil, nil, log)
	executor.SetManagedConnections(retainedWireSSHTestResolver{Service: sshconnections.NewService(true, nil, nil)})
	return executor
}

type launchWireSSHTestResolver struct {
	*sshconnections.Service
	target sshtransport.SSHTarget
}

func (r launchWireSSHTestResolver) AdmitSavedRuntime(_ context.Context, sessionID string) (sshconnections.RuntimeSnapshot, error) {
	return sshconnections.RuntimeSnapshot{
		SessionID: sessionID,
		Route: sshconnections.Route{
			Destination: sshconnections.Connection{
				ID: "fixture-connection", Host: r.target.Host, Port: r.target.Port, User: r.target.User, Fingerprint: r.target.PinnedFingerprint,
			},
			Stamp: "fixture-route",
		},
		IdentityGeneration: "fixture-generation",
	}, nil
}

func (r launchWireSSHTestResolver) DialRuntime(ctx context.Context, _ sshconnections.RuntimeSnapshot) (*ssh.Client, error) {
	return DialSSH(ctx, &r.target)
}

func wireSavedSSHLaunch(executor *SSHExecutor, metadata map[string]interface{}) {
	port, _ := strconv.Atoi(getMetadataString(metadata, MetadataKeySSHPort))
	executor.SetManagedConnections(launchWireSSHTestResolver{target: sshtransport.SSHTarget{
		Host: getMetadataString(metadata, MetadataKeySSHHost), Port: port, User: getMetadataString(metadata, MetadataKeySSHUser),
		PinnedFingerprint: getMetadataString(metadata, MetadataKeySSHHostFingerprint),
		IdentitySource:    SSHIdentitySource(getMetadataString(metadata, MetadataKeySSHIdentitySource)),
		IdentityFile:      getMetadataString(metadata, MetadataKeySSHIdentityFile),
	}})
}

func TestRetainedSSHMetadataKeepsOriginalBinding(t *testing.T) {
	request := &LaunchRequest{
		ExecutorType:        string(models.ExecutorTypeSSH),
		PreviousExecutionID: "prior",
		Metadata: map[string]interface{}{
			MetadataKeySSHRemoteSessionDir: "/original/session",
			MetadataKeySSHConnectionID:     "original",
			MetadataKeySSHRouteStamp:       "original-route-stamp",
		},
		ExecutorConfig: map[string]string{MetadataKeySSHConnectionID: "replacement"},
	}
	metadata := buildLaunchMetadata(request, "", "", "")
	if metadata[MetadataKeySSHConnectionID] != "original" || metadata[MetadataKeySSHRouteStamp] != "original-route-stamp" {
		t.Fatalf("retained SSH route changed: %+v", metadata)
	}
	request.PreviousExecutionID = ""
	metadata = buildLaunchMetadata(request, "", "", "")
	if metadata[MetadataKeySSHConnectionID] != "replacement" || metadata[MetadataKeySSHRouteStamp] != nil {
		t.Fatalf("fresh SSH launch received retained route metadata: %+v", metadata)
	}
}

func TestRetainedSSHStopOwnerRejectsRouteChange(t *testing.T) {
	original := &models.ExecutorRunning{
		AgentExecutionID: "execution",
		TaskID:           "task",
		SessionID:        "session",
		ExecutorID:       "executor",
		Runtime:          agentruntime.RuntimeSSH,
		Metadata: map[string]interface{}{
			MetadataKeySSHConnectionID: "connection",
			MetadataKeySSHRouteStamp:   "original",
		},
	}
	replacement := *original
	replacement.Metadata = cloneSSHMetadata(original.Metadata)
	replacement.Metadata[MetadataKeySSHRouteStamp] = "replacement"
	if !samePersistedSSHStopOwner(original, original) || samePersistedSSHStopOwner(original, &replacement) {
		t.Fatalf("changed route was treated as the same SSH stop owner")
	}
}
