package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func newRuntimeDrainTestClient(t *testing.T, identity, token string) (*ControlServer, *agentctl.ControlClient, *instance.Manager) {
	t.Helper()
	log := logger.Default()
	cfg := &config.Config{
		AuthToken:         token,
		ServerIdentity:    identity,
		DiagnosticLogPath: t.TempDir() + "/diagnostic.log",
	}
	mgr := instance.NewManager(cfg, log)
	mgr.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		return http.NotFoundHandler()
	})
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })
	cs := NewControlServer(cfg, mgr, log)
	srv := httptest.NewServer(cs.Router())
	t.Cleanup(srv.Close)
	host, port := parseHostPort(t, srv.URL)
	return cs, agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken(token)), mgr
}

func TestRuntimeSealAndDrainReturnsCachedAuthenticatedReceipt(t *testing.T) {
	_, client, mgr := newRuntimeDrainTestClient(t, "runtime-a", "current-token")
	req := agentctl.RuntimeDrainRequest{
		RequestID:              "drain-1",
		ExpectedServerIdentity: "runtime-a",
		DeadlineMS:             5000,
	}

	first, err := client.SealAndDrainRuntime(t.Context(), req)
	if err != nil {
		t.Fatalf("SealAndDrainRuntime: %v", err)
	}
	if first.RequestID != req.RequestID || first.ServerIdentity != req.ExpectedServerIdentity ||
		!first.Sealed || !first.Drained || first.Scope != "managed_process_groups" || first.Version != "runtime-seal-drain.v1" {
		t.Fatalf("receipt = %+v, want successful scoped receipt", first)
	}
	second, err := client.SealAndDrainRuntime(t.Context(), req)
	if err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	if *second != *first {
		t.Fatalf("identical retry receipt = %+v, want cached %+v", second, first)
	}
	advertised := false
	for _, capability := range SurvivalCapabilities {
		advertised = advertised || capability == "runtime-seal-drain.v1"
	}
	if !advertised {
		t.Fatal("runtime-seal-drain.v1 not advertised")
	}
	if _, err := mgr.CreateInstance(t.Context(), &instance.CreateRequest{WorkspacePath: t.TempDir()}); err == nil {
		t.Fatal("instance creation succeeded after drain receipt")
	}
}

func TestRuntimeSealAndDrainRequiresCurrentCredentialAndExactIdentity(t *testing.T) {
	_, client, _ := newRuntimeDrainTestClient(t, "runtime-a", "current-token")
	req := agentctl.RuntimeDrainRequest{RequestID: "drain-1", ExpectedServerIdentity: "runtime-a", DeadlineMS: 5000}
	client.SetAuthToken("wrong-token")
	if _, err := client.SealAndDrainRuntime(t.Context(), req); err == nil {
		t.Fatal("wrong credential accepted")
	}
	client.SetAuthToken("current-token")
	rotation, err := client.RotateCredential(t.Context())
	if err != nil {
		t.Fatalf("RotateCredential: %v", err)
	}
	client.SetAuthToken("current-token") // the superseded credential remains adoption-only
	if _, err := client.SealAndDrainRuntime(t.Context(), req); err == nil {
		t.Fatal("superseded adoption-only credential accepted")
	}
	client.SetAuthToken(rotation.Credential)
	req.ExpectedServerIdentity = "runtime-b"
	_, mismatchErr := client.SealAndDrainRuntime(t.Context(), req)
	var rejected *agentctl.RuntimeDrainRejectedError
	if !errors.As(mismatchErr, &rejected) || rejected.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("wrong ServerIdentity = %v, want a 412 rejection distinct from an in-progress drain", mismatchErr)
	}
}

func TestRuntimeSealAndDrainFailsClosedWithoutConfiguredCredential(t *testing.T) {
	_, client, _ := newRuntimeDrainTestClient(t, "runtime-a", "")
	req := agentctl.RuntimeDrainRequest{RequestID: "drain-unauth", ExpectedServerIdentity: "runtime-a", DeadlineMS: 5000}
	if _, err := client.SealAndDrainRuntime(t.Context(), req); err == nil {
		t.Fatal("runtime drain without a configured control credential succeeded")
	}
}

func TestRuntimeSealAndDrainSameClaimReturnsStoredReceiptWhateverTheDeadline(t *testing.T) {
	_, client, _ := newRuntimeDrainTestClient(t, "runtime-a", "current-token")
	req := agentctl.RuntimeDrainRequest{RequestID: "claim-a", ExpectedServerIdentity: "runtime-a", DeadlineMS: 5000}
	first, err := client.SealAndDrainRuntime(t.Context(), req)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	req.DeadlineMS++
	again, err := client.SealAndDrainRuntime(t.Context(), req)
	if err != nil {
		t.Fatalf("same claim with another deadline: %v", err)
	}
	if *again != *first {
		t.Fatalf("same claim receipt = %+v, want the stored %+v", again, first)
	}
}

func TestRuntimeSealAndDrainSuccessorClaimOnSealedRuntimeGetsFreshReceipt(t *testing.T) {
	_, client, _ := newRuntimeDrainTestClient(t, "runtime-a", "current-token")
	first, err := client.SealAndDrainRuntime(t.Context(), agentctl.RuntimeDrainRequest{
		RequestID: "claim-a", ExpectedServerIdentity: "runtime-a", DeadlineMS: 5000,
	})
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	second, err := client.SealAndDrainRuntime(t.Context(), agentctl.RuntimeDrainRequest{
		RequestID: "claim-b", ExpectedServerIdentity: "runtime-a", DeadlineMS: 5000,
	})
	if err != nil {
		t.Fatalf("successor claim on a sealed runtime: %v", err)
	}
	if second.RequestID != "claim-b" || !second.Sealed || !second.Drained {
		t.Fatalf("successor receipt = %+v, want a receipt bound to claim-b", second)
	}
	if second.RequestID == first.RequestID {
		t.Fatal("successor claim received the earlier claim's receipt")
	}
}

func TestRuntimeSealAndDrainOtherClaimDuringDrainIsConflict(t *testing.T) {
	cs, client, mgr := newRuntimeDrainTestClient(t, "runtime-a", "current-token")
	startupReached := make(chan struct{})
	startupRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseStartup := func() { releaseOnce.Do(func() { close(startupRelease) }) }
	mgr.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		close(startupReached)
		<-startupRelease
		return http.NotFoundHandler()
	})
	t.Cleanup(releaseStartup)
	go func() {
		_, _ = mgr.CreateInstance(context.Background(), &instance.CreateRequest{ID: "held", WorkspacePath: t.TempDir()})
	}()
	select {
	case <-startupReached:
	case <-time.After(time.Second):
		t.Fatal("creation did not reach startup gate")
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := client.SealAndDrainRuntime(t.Context(), agentctl.RuntimeDrainRequest{
			RequestID: "claim-a", ExpectedServerIdentity: "runtime-a", DeadlineMS: 3000,
		})
		firstDone <- err
	}()
	// The first drain is blocked on the admitted creation; a second claim must be refused.
	require.Eventually(t, func() bool {
		cs.runtimeDrain.mu.Lock()
		defer cs.runtimeDrain.mu.Unlock()
		return cs.runtimeDrain.running == "claim-a"
	}, 2*time.Second, time.Millisecond, "first drain never became the running drain")
	_, err := client.SealAndDrainRuntime(t.Context(), agentctl.RuntimeDrainRequest{
		RequestID: "claim-b", ExpectedServerIdentity: "runtime-a", DeadlineMS: 50,
	})
	var rejected *agentctl.RuntimeDrainRejectedError
	if !errors.As(err, &rejected) || rejected.StatusCode != http.StatusConflict {
		t.Fatalf("second claim during a running drain = %v, want a 409 rejection", err)
	}
	releaseStartup()
	if err := receiveAPIError(firstDone); err != nil {
		t.Fatalf("first drain after release: %v", err)
	}
}

func TestRuntimeSealAndDrainSerializesWithMainLoopShutdown(t *testing.T) {
	cs, client, mgr := newRuntimeDrainTestClient(t, "runtime-a", "current-token")
	if _, err := mgr.CreateInstance(t.Context(), &instance.CreateRequest{WorkspacePath: t.TempDir()}); err != nil {
		t.Fatalf("CreateInstance before drain: %v", err)
	}
	req := agentctl.RuntimeDrainRequest{RequestID: "drain-main-race", ExpectedServerIdentity: "runtime-a", DeadlineMS: 5000}
	start := make(chan struct{})
	apiResult := make(chan error, 1)
	mainResult := make(chan error, 1)
	go func() {
		<-start
		_, err := client.SealAndDrainRuntime(t.Context(), req)
		apiResult <- err
	}()
	go func() {
		<-start
		mainResult <- cs.ShutdownInstances(t.Context())
	}()
	close(start)
	if err := receiveAPIError(apiResult); err != nil {
		t.Fatalf("API drain racing main shutdown: %v", err)
	}
	if err := receiveAPIError(mainResult); err != nil {
		t.Fatalf("main shutdown racing API drain: %v", err)
	}
	if _, err := client.SealAndDrainRuntime(t.Context(), req); err != nil {
		t.Fatalf("identical retry after serialized shutdown: %v", err)
	}
}

func TestRuntimeDrainAdmissionReturnsDeadlineAndSameRequestCanRetry(t *testing.T) {
	_, client, mgr := newRuntimeDrainTestClient(t, "runtime-a", "current-token")
	startupReached := make(chan struct{})
	startupRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseStartup := func() { releaseOnce.Do(func() { close(startupRelease) }) }
	mgr.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		close(startupReached)
		<-startupRelease
		return http.NotFoundHandler()
	})
	t.Cleanup(releaseStartup)
	createDone := make(chan error, 1)
	go func() {
		_, err := mgr.CreateInstance(context.Background(), &instance.CreateRequest{ID: "drain-race", WorkspacePath: t.TempDir()})
		createDone <- err
	}()
	select {
	case <-startupReached:
	case <-time.After(time.Second):
		t.Fatal("creation did not reach startup gate")
	}

	req := agentctl.RuntimeDrainRequest{RequestID: "drain-timeout", ExpectedServerIdentity: "runtime-a", DeadlineMS: 20}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := client.SealAndDrainRuntime(ctx, req); err == nil {
		t.Fatal("drain returned success while an admitted creation remained blocked")
	}

	releaseStartup()
	if err, ok := receiveAPIResult(createDone); !ok || (err != nil && !errors.Is(err, instance.ErrManagerShuttingDown)) {
		t.Fatalf("admitted creation result = %v (received=%t)", err, ok)
	}
	if _, err := client.SealAndDrainRuntime(t.Context(), req); err != nil {
		t.Fatalf("retry after the admitted creation completed: %v", err)
	}
}

func receiveAPIResult(ch <-chan error) (error, bool) {
	select {
	case err := <-ch:
		return err, true
	case <-time.After(time.Second):
		return nil, false
	}
}

func receiveAPIError(ch <-chan error) error {
	select {
	case err := <-ch:
		return err
	case <-time.After(5 * time.Second):
		return context.DeadlineExceeded
	}
}
