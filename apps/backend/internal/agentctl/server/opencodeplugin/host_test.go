package opencodeplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const testRuntimeVersion = "2.2.0"

type fakeCommitter struct {
	mu         sync.Mutex
	admissions []Admission
	err        error
}

func (c *fakeCommitter) CommitOpenCodePluginObservation(_ context.Context, admission Admission) (func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	c.admissions = append(c.admissions, admission)
	return nil, nil
}

func (c *fakeCommitter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.admissions)
}

func startTestHost(t *testing.T, committer Committer, options ...func(*Options)) (*Host, Bootstrap, *observer.ObservedLogs) {
	return startTestHostWithRegistry(t, committer, nil, options...)
}

func startTestHostWithRegistry(t *testing.T, committer Committer, registry *Registry, options ...func(*Options)) (*Host, Bootstrap, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zap.InfoLevel)
	opts := Options{
		HomeDir:        t.TempDir(),
		ArtifactDir:    t.TempDir(),
		ArtifactHash:   "artifact-hash",
		RuntimeFamily:  "opencode",
		RuntimeVersion: testRuntimeVersion,
		Committer:      committer,
		Registry:       registry,
		LeaseID:        "lease-test",
		Credential:     "credential-test-0001",
		Logger:         zap.New(core),
	}
	for _, apply := range options {
		apply(&opts)
	}
	host, err := NewHost(opts)
	if err != nil {
		t.Fatalf("NewHost() error = %v", err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = host.Close(ctx)
	})
	raw, err := os.ReadFile(host.BootstrapPath())
	if err != nil {
		t.Fatalf("read bootstrap: %v", err)
	}
	var bootstrap Bootstrap
	if err := json.Unmarshal(raw, &bootstrap); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	return host, bootstrap, observed
}

func doJSON(t *testing.T, host *Host, bootstrap Bootstrap, method, path string, body any, credential string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, bootstrap.Endpoint+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if credential == "" {
		credential = bootstrap.Credential
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, _ := io.ReadAll(response.Body)
	decoded := map[string]any{}
	_ = json.Unmarshal(raw, &decoded)
	return response.StatusCode, decoded
}

func helloBody(bootstrap Bootstrap, behaviors ...BehaviorRef) HelloRequest {
	if len(behaviors) == 0 {
		behaviors = []BehaviorRef{{ID: BehaviorShellOutcomes, Version: ShellOutcomesVersion}}
	}
	return HelloRequest{
		ProtocolVersion: ProtocolVersion,
		LeaseID:         bootstrap.LeaseID,
		ArtifactHash:    bootstrap.ArtifactHash,
		Runtime:         RuntimeInfo{Family: "opencode", Version: testRuntimeVersion},
		Behaviors:       behaviors,
	}
}

func setShellClaims(host *Host, claims []Claim) bool {
	raw, err := MarshalShellConfig(claims)
	if err != nil {
		return false
	}
	if len(claims) == 0 {
		return host.RevokeBehaviorConfig(BehaviorShellOutcomes, raw)
	}
	return host.SetBehaviorConfig(BehaviorShellOutcomes, raw)
}

func testClaim() Claim {
	return Claim{
		ShellID:          "shell-1",
		NativeSessionID:  "session-1",
		NativeGeneration: 1,
		WorkID:           "shell-1",
		RunID:            "shell-1",
		SourceCallID:     "call-1",
		Title:            "run tests",
		PromptGeneration: 3,
		SubmissionID:     "submission-1",
	}
}

func testEvent(claim Claim, status string, exit *int) ObservationEnvelope {
	payload, _ := json.Marshal(ShellTerminalPayload{Kind: "shell.terminal", ShellID: claim.ShellID, Status: status, ExitCode: exit})
	return ObservationEnvelope{
		EventID:          "shell-outcomes:1:" + claim.NativeSessionID + ":1:" + claim.ShellID + ":" + status,
		BehaviorID:       BehaviorShellOutcomes,
		BehaviorVersion:  ShellOutcomesVersion,
		Sequence:         1,
		NativeSessionID:  claim.NativeSessionID,
		NativeGeneration: claim.NativeGeneration,
		Payload:          payload,
	}
}

func eventBody(bootstrap Bootstrap, events ...ObservationEnvelope) EventRequest {
	return EventRequest{ProtocolVersion: ProtocolVersion, LeaseID: bootstrap.LeaseID, ProducerID: "producer-1", Events: events}
}

func TestOpenCodePluginLease(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHost(t, committer)
	status, _ := doJSON(t, host, bootstrap, http.MethodPost, "/v1/hello", helloBody(bootstrap), "wrong-credential-000")
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong credential status = %d, want 401", status)
	}
	wrongLease := helloBody(bootstrap)
	wrongLease.LeaseID = "other-lease"
	status, _ = doJSON(t, host, bootstrap, http.MethodPost, "/v1/hello", wrongLease, "")
	if status != http.StatusConflict {
		t.Fatalf("wrong lease status = %d, want 409", status)
	}
	wrongRuntime := helloBody(bootstrap)
	wrongRuntime.Runtime.Version = "9.9.9"
	status, _ = doJSON(t, host, bootstrap, http.MethodPost, "/v1/hello", wrongRuntime, "")
	if status != http.StatusConflict {
		t.Fatalf("wrong runtime status = %d, want 409", status)
	}
	status, _ = doJSON(t, host, bootstrap, http.MethodPost, "/v1/hello", helloBody(bootstrap), "")
	if status != http.StatusOK {
		t.Fatalf("hello status = %d, want 200", status)
	}
	if !host.Ready() {
		t.Fatal("host did not become ready after hello")
	}
	if got := host.BootstrapPath(); !strings.HasSuffix(got, "bootstrap.json") {
		t.Fatalf("bootstrap path = %q", got)
	}
}

func TestOpenCodePluginClaims(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHost(t, committer)
	if status, _ := doJSON(t, host, bootstrap, http.MethodPost, "/v1/hello", helloBody(bootstrap), ""); status != http.StatusOK {
		t.Fatalf("hello status = %d", status)
	}
	claim := testClaim()
	if !setShellClaims(host, []Claim{claim}) {
		t.Fatal("SetBehaviorConfig() did not advance the revision")
	}
	status, config := doJSON(t, host, bootstrap, http.MethodGet, "/v1/config?lease="+bootstrap.LeaseID+"&revision=0", nil, "")
	if status != http.StatusOK {
		t.Fatalf("config status = %d", status)
	}
	behaviors, _ := config["behaviors"].(map[string]any)
	shell, _ := behaviors[BehaviorShellOutcomes].(map[string]any)
	if shell == nil || shell["version"] != float64(ShellOutcomesVersion) {
		t.Fatalf("shell config = %#v", shell)
	}
	data, _ := shell["data"].(map[string]any)
	claims, _ := data["claims"].([]any)
	if len(claims) != 1 {
		t.Fatalf("shell config claims = %#v", data)
	}
	foreign := testEvent(claim, "exited", intPtr(0))
	foreign.NativeSessionID = "foreign-session"
	status, response := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, foreign), "")
	if status != http.StatusOK || committer.count() != 0 {
		t.Fatalf("foreign session status=%d commits=%d response=%#v", status, committer.count(), response)
	}
	status, response = doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, testEvent(claim, "exited", intPtr(0))), "")
	if status != http.StatusOK || committer.count() != 1 {
		t.Fatalf("claimed event status=%d commits=%d response=%#v", status, committer.count(), response)
	}
}

func TestOpenCodePluginDurableAck(t *testing.T) {
	committer := &fakeCommitter{err: fmt.Errorf("journal unavailable")}
	host, bootstrap, _ := startTestHost(t, committer)
	claim := testClaim()
	setShellClaims(host, []Claim{claim})
	status, response := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, testEvent(claim, "exited", intPtr(0))), "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("failed commit status = %d, want 503 (%#v)", status, response)
	}
	committer.mu.Lock()
	committer.err = nil
	committer.mu.Unlock()
	status, _ = doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, testEvent(claim, "exited", intPtr(0))), "")
	if status != http.StatusOK || committer.count() != 1 {
		t.Fatalf("retried commit status=%d commits=%d", status, committer.count())
	}
}

func TestOpenCodePluginLostAck(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHost(t, committer)
	claim := testClaim()
	setShellClaims(host, []Claim{claim})
	event := testEvent(claim, "exited", intPtr(0))
	if status, _ := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, event), ""); status != http.StatusOK {
		t.Fatalf("first delivery failed")
	}
	setShellClaims(host, nil)
	status, response := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, event), "")
	if status != http.StatusOK {
		t.Fatalf("retry after claim removal status = %d (%#v)", status, response)
	}
	if committer.count() != 1 {
		t.Fatalf("retry after lost acknowledgement duplicated the commit (%d)", committer.count())
	}
}

func TestOpenCodePluginDeliveryGap(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHost(t, committer)
	status, response := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, testEvent(testClaim(), "exited", intPtr(0))), "")
	if status != http.StatusOK || committer.count() != 0 {
		t.Fatalf("unclaimed event status=%d commits=%d response=%#v", status, committer.count(), response)
	}
	rejected, _ := response["rejected"].([]any)
	if len(rejected) != 1 {
		t.Fatalf("unclaimed event was not rejected: %#v", response)
	}
}

func TestOpenCodePluginRedaction(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, logs := startTestHost(t, committer)
	doJSON(t, host, bootstrap, http.MethodPost, "/v1/hello", helloBody(bootstrap), "")
	doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, testEvent(testClaim(), "exited", intPtr(0))), "")
	for _, entry := range logs.All() {
		message := entry.Message
		for _, field := range entry.ContextMap() {
			message += fmt.Sprint(field)
		}
		if strings.Contains(message, bootstrap.Credential) {
			t.Fatal("plugin host log contains the launch credential")
		}
		if strings.Contains(message, "run tests") {
			t.Fatal("plugin host log contains a shell title")
		}
	}
}

func TestOpenCodePluginConcurrentClose(t *testing.T) {
	committer := &fakeCommitter{}
	host, _, _ := startTestHost(t, committer)
	var wg sync.WaitGroup
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = setShellClaims(host, []Claim{testClaim()})
			_ = host.Ready()
		}()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := host.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	wg.Wait()
	if _, err := os.Stat(host.BootstrapPath()); !os.IsNotExist(err) {
		t.Fatalf("bootstrap file survived close: %v", err)
	}
}

func intPtr(value int) *int { return &value }

type blockingCommitter struct {
	mu         sync.Mutex
	admissions []Admission
	started    chan struct{}
	release    chan struct{}
	callCheck  bool
}

func (c *blockingCommitter) CommitOpenCodePluginObservation(_ context.Context, admission Admission) (func(), error) {
	close(c.started)
	<-c.release
	if c.callCheck && admission.Check != nil && !admission.Check() {
		return nil, ErrAdmissionRevoked
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.admissions = append(c.admissions, admission)
	return nil, nil
}

func (c *blockingCommitter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.admissions)
}

func TestOpenCodePluginRevocationFence(t *testing.T) {
	committer := &blockingCommitter{started: make(chan struct{}), release: make(chan struct{}), callCheck: true}
	host, bootstrap, _ := startTestHost(t, committer)
	claim := testClaim()
	setShellClaims(host, []Claim{claim})
	event := testEvent(claim, "exited", intPtr(0))

	done := make(chan int, 1)
	go func() {
		status, _ := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, event), "")
		done <- status
	}()
	<-committer.started

	revoked := make(chan bool, 1)
	go func() { revoked <- setShellClaims(host, nil) }()
	select {
	case <-revoked:
		t.Fatal("configuration revocation completed while a durable commit was in flight")
	case <-time.After(50 * time.Millisecond):
	}
	close(committer.release)
	if status := <-done; status != http.StatusOK {
		t.Fatalf("in-flight commit status = %d, want 200", status)
	}
	if !<-revoked {
		t.Fatal("revocation did not advance after the fenced commit aborted")
	}
	if committer.count() != 0 {
		t.Fatalf("commit count = %d, want a revoked admission", committer.count())
	}
	status, response := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, event), "")
	if status != http.StatusOK || committer.count() != 0 {
		t.Fatalf("revoked event status=%d count=%d response=%#v", status, committer.count(), response)
	}
	rejected, _ := response["rejected"].([]any)
	if len(rejected) != 1 {
		t.Fatalf("revoked event was not rejected: %#v", response)
	}
}

func TestOpenCodePluginGenericBehaviorDeliversWithoutShellBranches(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHostWithRegistry(t, committer, NewRegistry(ShellOutcomesBehavior{}, testBehaviorModule{}))
	hello := helloBody(bootstrap,
		BehaviorRef{ID: BehaviorShellOutcomes, Version: ShellOutcomesVersion},
		BehaviorRef{ID: "test-behavior", Version: 7},
	)
	if status, _ := doJSON(t, host, bootstrap, http.MethodPost, "/v1/hello", hello, ""); status != http.StatusOK {
		t.Fatalf("hello status = %d", status)
	}
	if !host.SetBehaviorConfig("test-behavior", json.RawMessage(`{"resource":"r1"}`)) {
		t.Fatal("generic behavior config was rejected")
	}
	status, config := doJSON(t, host, bootstrap, http.MethodGet, "/v1/config?lease="+bootstrap.LeaseID+"&revision=0", nil, "")
	if status != http.StatusOK {
		t.Fatalf("config status = %d", status)
	}
	behaviors := config["behaviors"].(map[string]any)
	generic := behaviors["test-behavior"].(map[string]any)
	data := generic["data"].(map[string]any)
	if data["resource"] != "r1" {
		t.Fatalf("generic config = %#v", generic)
	}
	payload, _ := json.Marshal(map[string]string{"kind": "test.completed", "resource": "r1"})
	event := ObservationEnvelope{
		EventID:          "test:event-1",
		BehaviorID:       "test-behavior",
		BehaviorVersion:  7,
		Sequence:         1,
		NativeSessionID:  "native-session",
		NativeGeneration: 3,
		Payload:          payload,
	}
	status, response := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, event), "")
	if status != http.StatusOK || committer.count() != 1 {
		t.Fatalf("generic event status=%d count=%d response=%#v", status, committer.count(), response)
	}
	committer.mu.Lock()
	admission := committer.admissions[0]
	committer.mu.Unlock()
	if admission.BehaviorID != "test-behavior" || admission.Observation.WorkID != "r1" {
		t.Fatalf("generic admission = %#v", admission)
	}
}

func TestOpenCodePluginRetiresAcceptedClaimsBeyondCapacity(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHost(t, committer)
	for index := 0; index < MaxClaims+44; index++ {
		claim := testClaim()
		claim.ShellID = fmt.Sprintf("shell-%d", index)
		claim.WorkID = claim.ShellID
		claim.RunID = claim.ShellID
		setShellClaims(host, []Claim{claim})
		event := testEvent(claim, "exited", intPtr(0))
		event.EventID = fmt.Sprintf("plugin:capacity:%d", index)
		status, response := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, event), "")
		if status != http.StatusOK {
			t.Fatalf("job %d status=%d response=%#v", index, status, response)
		}
	}
	if committer.count() != MaxClaims+44 {
		t.Fatalf("accepted jobs = %d, want %d", committer.count(), MaxClaims+44)
	}
}

func TestOpenCodePluginCloseWaitsForInFlightCommit(t *testing.T) {
	committer := &blockingCommitter{started: make(chan struct{}), release: make(chan struct{}), callCheck: true}
	host, bootstrap, _ := startTestHost(t, committer)
	claim := testClaim()
	setShellClaims(host, []Claim{claim})
	done := make(chan int, 1)
	go func() {
		body, _ := json.Marshal(eventBody(bootstrap, testEvent(claim, "exited", intPtr(0))))
		request, err := http.NewRequest(http.MethodPost, bootstrap.Endpoint+"/v1/events", bytes.NewReader(body))
		if err != nil {
			done <- 0
			return
		}
		request.Header.Set("Authorization", "Bearer "+bootstrap.Credential)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			done <- 0
			return
		}
		defer func() { _ = response.Body.Close() }()
		done <- response.StatusCode
	}()
	<-committer.started
	closed := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		closed <- host.Close(ctx)
	}()
	select {
	case <-closed:
		t.Fatal("Close returned while a durable commit was in flight")
	case <-time.After(50 * time.Millisecond):
	}
	close(committer.release)
	<-done
	if err := <-closed; err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if committer.count() != 0 {
		t.Fatalf("commit count = %d, want a revoked admission", committer.count())
	}
}

func TestOpenCodePluginLiveClaimCapacityIsBounded(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHost(t, committer)
	claims := make([]Claim, 0, MaxClaims+4)
	for index := 0; index < MaxClaims+4; index++ {
		claim := testClaim()
		claim.ShellID = fmt.Sprintf("live-%d", index)
		claim.WorkID = claim.ShellID
		claim.RunID = claim.ShellID
		claims = append(claims, claim)
	}
	setShellClaims(host, claims)
	status, config := doJSON(t, host, bootstrap, http.MethodGet, "/v1/config?lease="+bootstrap.LeaseID+"&revision=0", nil, "")
	if status != http.StatusOK {
		t.Fatalf("config status = %d", status)
	}
	data := config["behaviors"].(map[string]any)[BehaviorShellOutcomes].(map[string]any)["data"].(map[string]any)
	stored := data["claims"].([]any)
	if len(stored) != MaxClaims {
		t.Fatalf("stored live claims = %d, want %d", len(stored), MaxClaims)
	}
	dropped := claims[len(claims)-1]
	event := testEvent(dropped, "exited", intPtr(0))
	event.EventID = "plugin:dropped-live"
	status, _ = doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, event), "")
	if status != http.StatusOK || committer.count() != 0 {
		t.Fatalf("excess live claim status=%d commits=%d, want no effects", status, committer.count())
	}
}

func TestOpenCodePluginReceiptRejectsConflictingPayload(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHost(t, committer)
	claim := testClaim()
	setShellClaims(host, []Claim{claim})
	event := testEvent(claim, "exited", intPtr(0))
	if status, _ := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, event), ""); status != http.StatusOK {
		t.Fatal("first delivery failed")
	}
	conflicting := testEvent(claim, "exited", intPtr(3))
	status, response := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, conflicting), "")
	if status != http.StatusOK {
		t.Fatalf("conflicting duplicate status = %d", status)
	}
	rejected, _ := response["rejected"].([]any)
	if len(rejected) != 1 || committer.count() != 1 {
		t.Fatalf("conflicting duplicate response=%#v commits=%d", response, committer.count())
	}
}

func TestOpenCodePluginCommitBeforeRevocationIsAccepted(t *testing.T) {
	committer := &blockingCommitter{started: make(chan struct{}), release: make(chan struct{}), callCheck: true}
	host, bootstrap, _ := startTestHost(t, committer)
	claim := testClaim()
	setShellClaims(host, []Claim{claim})
	done := make(chan int, 1)
	go func() {
		status, _ := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, testEvent(claim, "exited", intPtr(0))), "")
		done <- status
	}()
	<-committer.started
	close(committer.release)
	if status := <-done; status != http.StatusOK {
		t.Fatalf("commit status = %d, want 200", status)
	}
	if committer.count() != 1 {
		t.Fatalf("commit count = %d, want 1", committer.count())
	}
}

type delayedConfigBehavior struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (*delayedConfigBehavior) ID() string   { return "delayed-config" }
func (*delayedConfigBehavior) Version() int { return 1 }

func (b *delayedConfigBehavior) DecodeConfig(raw json.RawMessage) (any, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == "slow" {
		b.once.Do(func() { close(b.started) })
		<-b.release
	}
	return value, nil
}

func (*delayedConfigBehavior) EncodeConfig(state any) (json.RawMessage, error) {
	return json.Marshal(state)
}

func (*delayedConfigBehavior) Admit(any, ObservationEnvelope) (Admission, error) {
	return Admission{}, nil
}

func (*delayedConfigBehavior) Retire(state any, _ Admission) (any, bool) {
	return state, false
}

func TestOpenCodePluginConfigUpdatesAreOrderedByCallSequence(t *testing.T) {
	behavior := &delayedConfigBehavior{started: make(chan struct{}), release: make(chan struct{})}
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHostWithRegistry(t, committer, NewRegistry(behavior))
	done := make(chan bool, 1)
	go func() {
		done <- host.SetBehaviorConfig("delayed-config", json.RawMessage(`"slow"`))
	}()
	<-behavior.started
	if !host.SetBehaviorConfig("delayed-config", json.RawMessage(`"fast"`)) {
		t.Fatal("newer config was rejected")
	}
	close(behavior.release)
	if <-done {
		t.Fatal("older config replaced a newer applied snapshot")
	}
	if host.SetBehaviorConfig("delayed-config", json.RawMessage(`"fast"`)) {
		t.Fatal("re-sending the applied snapshot was not deduplicated")
	}
	status, config := doJSON(t, host, bootstrap, http.MethodGet, "/v1/config?lease="+bootstrap.LeaseID+"&revision=0", nil, "")
	if status != http.StatusOK {
		t.Fatalf("config status = %d", status)
	}
	data := config["behaviors"].(map[string]any)["delayed-config"].(map[string]any)["data"]
	if data != "fast" {
		t.Fatalf("applied config = %#v, want fast", data)
	}
}
