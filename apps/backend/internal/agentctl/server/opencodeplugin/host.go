package opencodeplugin

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Committer is the durable delivery boundary. It returns nil only after the
// observation crossed Kandev's commit-before-publish boundary.
type Committer interface {
	CommitOpenCodePluginObservation(ctx context.Context, admission Admission) (publish func(), err error)
}

// Options configures one launch-scoped plugin host.
type Options struct {
	HomeDir        string
	ArtifactDir    string
	ArtifactHash   string
	RuntimeFamily  string
	RuntimeVersion string
	Committer      Committer
	Registry       *Registry
	Logger         *zap.Logger
	LeaseID        string
	Credential     string
}

type behaviorConfigState struct {
	raw   json.RawMessage
	state any
}

// Host owns the private loopback listener, launch lease, behavior
// configuration, and bounded receipt evidence for one agent process.
type Host struct {
	options  Options
	lease    Lease
	registry *Registry
	enabled  map[string]int
	logger   *zap.Logger

	listener      net.Listener
	server        *http.Server
	runtimeDir    string
	bootstrapPath string
	readyCh       chan struct{}
	readyOnce     sync.Once
	contactCh     chan struct{}
	contactOnce   sync.Once

	// fence is the admission/configuration ordering boundary. A commit holds it
	// across behavior admission, the durable committer call, and claim
	// retirement, so a revocation can never land between validation and the
	// journal write. It is separate from the lifecycle mutex below and is only
	// held for the bounded commit or a configuration snapshot.
	fence           sync.Mutex
	behaviorConfigs map[string]behaviorConfigState
	revision        uint64
	receipts        map[string]string
	receiptOrder    []string
	configWaiters   chan struct{}
	appliedConfig   map[string]uint64
	nextConfigSeq   atomic.Uint64
	admissionEpoch  atomic.Uint64

	mu     sync.Mutex
	ready  bool
	closed bool
}

// NewHost validates the launch inputs and constructs the host. Start binds the
// listener and writes the bootstrap file.
func NewHost(options Options) (*Host, error) {
	if strings.TrimSpace(options.HomeDir) == "" {
		return nil, errors.New("opencode plugin host home directory is required")
	}
	if strings.TrimSpace(options.ArtifactDir) == "" || strings.TrimSpace(options.ArtifactHash) == "" {
		return nil, errors.New("opencode plugin artifact identity is required")
	}
	if options.Committer == nil {
		return nil, errors.New("opencode plugin committer is required")
	}
	registry := options.Registry
	if registry == nil {
		registry = ProductionRegistry()
	}
	logger := options.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	enabled := make(map[string]int)
	for _, ref := range registry.Refs() {
		enabled[ref.ID] = ref.Version
	}
	return &Host{
		options:         options,
		registry:        registry,
		enabled:         enabled,
		logger:          logger,
		readyCh:         make(chan struct{}),
		contactCh:       make(chan struct{}),
		behaviorConfigs: make(map[string]behaviorConfigState),
		appliedConfig:   make(map[string]uint64),
		receipts:        make(map[string]string),
		configWaiters:   make(chan struct{}),
	}, nil
}

// Start listens on loopback and publishes the private bootstrap file. It must
// run before the OpenCode subprocess starts.
func (h *Host) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return errors.New("opencode plugin host is closed")
	}
	if h.server != nil {
		h.mu.Unlock()
		return errors.New("opencode plugin host is already started")
	}
	h.mu.Unlock()
	lease, err := NewLease(h.options.ArtifactHash)
	if err != nil {
		return err
	}
	if h.options.LeaseID != "" {
		lease.ID = h.options.LeaseID
	}
	if h.options.Credential != "" {
		lease.Credential = h.options.Credential
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for opencode plugin callbacks: %w", err)
	}
	runtimeDir := filepath.Join(h.options.HomeDir, "opencode-plugin", "launch", lease.ID)
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		_ = listener.Close()
		return fmt.Errorf("create opencode plugin bootstrap directory: %w", err)
	}
	if err := os.Chmod(runtimeDir, 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		_ = listener.Close()
		return fmt.Errorf("protect opencode plugin bootstrap directory: %w", err)
	}
	bootstrapPath := filepath.Join(runtimeDir, "bootstrap.json")
	bootstrap := Bootstrap{
		ProtocolVersion: ProtocolVersion,
		Endpoint:        "http://" + listener.Addr().String(),
		Credential:      lease.Credential,
		LeaseID:         lease.ID,
		ArtifactHash:    h.options.ArtifactHash,
		Behaviors:       h.registry.Refs(),
	}
	if err := writePrivateJSON(bootstrapPath, bootstrap); err != nil {
		_ = listener.Close()
		return err
	}
	server := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: RequestTimeout,
		ReadTimeout:       RequestTimeout * 2,
		WriteTimeout:      ConfigPollTimeout + RequestTimeout,
		IdleTimeout:       ConfigPollTimeout * 2,
	}
	h.mu.Lock()
	h.lease = lease
	h.listener = listener
	h.server = server
	h.runtimeDir = runtimeDir
	h.bootstrapPath = bootstrapPath
	h.mu.Unlock()
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			h.logger.Warn("opencode plugin host listener stopped", zap.Error(serveErr))
		}
	}()
	h.logger.Info("opencode plugin host started", zap.String("lease_id", lease.ID))
	return nil
}

// ArtifactDir returns the immutable plugin directory this launch supplies.
func (h *Host) ArtifactDir() string {
	return h.options.ArtifactDir
}

// BootstrapPath returns the private bootstrap file path written by Start.
func (h *Host) BootstrapPath() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.bootstrapPath
}

// FirstContact blocks until the plugin makes its first authenticated request.
func (h *Host) FirstContact() <-chan struct{} {
	return h.contactCh
}

// Ready reports whether the plugin completed a compatible hello.
func (h *Host) Ready() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ready
}

// WaitReady blocks until the plugin completes hello or ctx ends.
func (h *Host) WaitReady(ctx context.Context) error {
	select {
	case <-h.readyCh:
		if h.Ready() {
			return nil
		}
		return errors.New("opencode plugin host closed before readiness")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SetBehaviorConfig replaces one behavior's complete configuration. The
// behavior validates and normalizes its own shape. It returns true when the
// configuration revision advanced.
func (h *Host) SetBehaviorConfig(id string, raw json.RawMessage) bool {
	sequence := h.nextConfigSeq.Add(1)
	version, enabled := h.enabled[id]
	if !enabled {
		return false
	}
	behavior, ok := h.registry.Get(id, version)
	if !ok {
		return false
	}
	state, err := behavior.DecodeConfig(raw)
	if err != nil {
		h.logger.Debug("opencode plugin config rejected", zap.String("behavior", id), zap.Error(err))
		return false
	}
	normalized, err := behavior.EncodeConfig(state)
	if err != nil {
		h.logger.Debug("opencode plugin config encode failed", zap.String("behavior", id), zap.Error(err))
		return false
	}
	h.fence.Lock()
	defer h.fence.Unlock()
	if h.isClosed() || sequence <= h.appliedConfig[id] {
		return false
	}
	h.appliedConfig[id] = sequence
	current, exists := h.behaviorConfigs[id]
	if exists && string(current.raw) == string(normalized) {
		return false
	}
	h.behaviorConfigs[id] = behaviorConfigState{raw: normalized, state: state}
	h.revision++
	h.notifyConfigLocked()
	h.logger.Debug("opencode plugin config updated", zap.String("behavior", id), zap.Uint64("revision", h.revision))
	return true
}

// RevokeBehaviorConfig advances the admission epoch before applying a complete
// configuration that removes authority. An in-flight commit sees the new epoch
// before durable admission even though the state update waits for the fence.
func (h *Host) RevokeBehaviorConfig(id string, raw json.RawMessage) bool {
	h.admissionEpoch.Add(1)
	return h.SetBehaviorConfig(id, raw)
}

// Close revokes the lease, waits for an in-flight durable commit, releases
// configuration waiters, and removes only the Kandev-owned bootstrap and launch
// directory.
func (h *Host) Close(_ context.Context) error {
	h.admissionEpoch.Add(1)
	h.fence.Lock()
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		h.fence.Unlock()
		return nil
	}
	h.closed = true
	server := h.server
	runtimeDir := h.runtimeDir
	h.closeReadyLocked()
	h.notifyConfigLocked()
	h.mu.Unlock()
	h.fence.Unlock()
	var err error
	if server != nil {
		// Revoke the lease immediately; a configuration long poll must not
		// delay process teardown. The plugin sees a closed connection and
		// stops retrying after its own bounded backoff.
		if closeErr := server.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			err = errors.Join(err, closeErr)
		}
	}
	if runtimeDir != "" {
		if removeErr := os.RemoveAll(runtimeDir); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove opencode plugin launch directory: %w", removeErr))
		}
	}
	return err
}

// ServeHTTP implements the private protocol. Every route requires the launch
// credential; ordinary application authentication is not consulted.
func (h *Host) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if !h.authorized(request) {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.contactOnce.Do(func() { close(h.contactCh) })
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/hello":
		h.handleHello(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/v1/config":
		h.handleConfig(writer, request)
	case request.Method == http.MethodPost && request.URL.Path == "/v1/events":
		h.handleEvents(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

func (h *Host) authorized(request *http.Request) bool {
	h.mu.Lock()
	lease := h.lease
	closed := h.closed
	h.mu.Unlock()
	if closed {
		return false
	}
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	return lease.Authorized(strings.TrimPrefix(header, "Bearer "))
}

func (h *Host) isClosed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

func (h *Host) handleHello(writer http.ResponseWriter, request *http.Request) {
	var hello HelloRequest
	if err := decodeLimitedJSON(request, &hello, MaxHelloRequestBytes); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_hello")
		return
	}
	h.mu.Lock()
	lease := h.lease
	closed := h.closed
	h.mu.Unlock()
	if closed || hello.ProtocolVersion != ProtocolVersion || hello.LeaseID != lease.ID ||
		hello.ArtifactHash != lease.ArtifactHash {
		h.logger.Warn("opencode plugin hello rejected", zap.String("reason", "lease_mismatch"))
		writeError(writer, http.StatusConflict, "lease_mismatch")
		return
	}
	if strings.TrimSpace(hello.Runtime.Family) != "opencode" || hello.Runtime.Version != h.options.RuntimeVersion {
		h.logger.Warn("opencode plugin hello rejected", zap.String("reason", "runtime_incompatible"),
			zap.String("reported_version", hello.Runtime.Version))
		writeError(writer, http.StatusConflict, "runtime_incompatible")
		return
	}
	negotiated := h.negotiate(hello.Behaviors)
	h.mu.Lock()
	h.ready = true
	h.mu.Unlock()
	h.readyOnce.Do(func() { close(h.readyCh) })
	h.logger.Info("opencode plugin hello accepted", zap.Int("behaviors", len(negotiated)))
	writeJSON(writer, http.StatusOK, HelloResponse{
		ProtocolVersion: ProtocolVersion,
		LeaseID:         lease.ID,
		Ready:           true,
		Behaviors:       negotiated,
	})
}

func (h *Host) negotiate(requested []BehaviorRef) []BehaviorRef {
	negotiated := make([]BehaviorRef, 0, len(requested))
	for _, ref := range requested {
		version, ok := h.enabled[ref.ID]
		if !ok || version != ref.Version {
			continue
		}
		negotiated = append(negotiated, BehaviorRef{ID: ref.ID, Version: version})
	}
	sort.Slice(negotiated, func(i, j int) bool { return negotiated[i].ID < negotiated[j].ID })
	return negotiated
}

func (h *Host) handleConfig(writer http.ResponseWriter, request *http.Request) {
	h.mu.Lock()
	leaseID := h.lease.ID
	h.mu.Unlock()
	if request.URL.Query().Get("lease") != leaseID {
		writeError(writer, http.StatusConflict, "lease_mismatch")
		return
	}
	revision, err := parseRevision(request.URL.Query().Get("revision"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_revision")
		return
	}
	response, ok := h.waitForConfig(request.Context(), revision)
	if !ok {
		return
	}
	h.logger.Debug("opencode plugin configuration served",
		zap.Uint64("revision", response.Revision),
		zap.Int("behaviors", len(response.Behaviors)))
	writeJSON(writer, http.StatusOK, response)
}

func (h *Host) waitForConfig(ctx context.Context, revision uint64) (ConfigResponse, bool) {
	deadline := time.NewTimer(ConfigPollTimeout)
	defer deadline.Stop()
	for {
		h.fence.Lock()
		if h.isClosed() {
			h.fence.Unlock()
			return ConfigResponse{}, false
		}
		if h.revision > revision {
			response := h.configResponseLocked()
			h.fence.Unlock()
			return response, true
		}
		waiter := h.configWaiters
		h.fence.Unlock()
		select {
		case <-waiter:
		case <-deadline.C:
			h.fence.Lock()
			response := h.configResponseLocked()
			h.fence.Unlock()
			return response, true
		case <-ctx.Done():
			return ConfigResponse{}, false
		}
	}
}

func (h *Host) configResponseLocked() ConfigResponse {
	response := ConfigResponse{
		ProtocolVersion: ProtocolVersion,
		LeaseID:         h.lease.ID,
		Revision:        h.revision,
		Behaviors:       make(map[string]BehaviorConfig, len(h.enabled)),
	}
	for id, version := range h.enabled {
		config := BehaviorConfig{Version: version}
		if state, ok := h.behaviorConfigs[id]; ok {
			config.Data = state.raw
		}
		response.Behaviors[id] = config
	}
	return response
}

func (h *Host) notifyConfigLocked() {
	close(h.configWaiters)
	h.configWaiters = make(chan struct{})
}

func (h *Host) closeReadyLocked() {
	h.readyOnce.Do(func() { close(h.readyCh) })
}

func (h *Host) handleEvents(writer http.ResponseWriter, request *http.Request) {
	var batch EventRequest
	if err := decodeLimitedJSON(request, &batch, MaxEventRequestBytes); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(writer, status, "invalid_events")
		return
	}
	h.mu.Lock()
	leaseID := h.lease.ID
	closed := h.closed
	h.mu.Unlock()
	if closed || batch.ProtocolVersion != ProtocolVersion || batch.LeaseID != leaseID {
		writeError(writer, http.StatusConflict, "lease_mismatch")
		return
	}
	if len(batch.Events) == 0 || len(batch.Events) > MaxEventsPerRequest || strings.TrimSpace(batch.ProducerID) == "" {
		writeError(writer, http.StatusBadRequest, "invalid_events")
		return
	}
	response, transient := h.commitEvents(request.Context(), batch.Events)
	if transient {
		writeJSON(writer, http.StatusServiceUnavailable, response)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (h *Host) commitEvents(ctx context.Context, events []ObservationEnvelope) (EventResponse, bool) {
	response := EventResponse{Accepted: make([]string, 0, len(events)), Rejected: make([]EventRejection, 0)}
	transient := false
	h.logger.Debug("opencode plugin observations received", zap.Int("events", len(events)))
	for _, event := range events {
		if transient {
			response.Rejected = append(response.Rejected, EventRejection{EventID: event.EventID, Code: "retry_later"})
			continue
		}
		if !validEnvelope(event) {
			response.Rejected = append(response.Rejected, EventRejection{EventID: event.EventID, Code: "invalid_envelope"})
			continue
		}
		behavior, ok := h.registry.Get(event.BehaviorID, event.BehaviorVersion)
		if !ok || h.enabled[event.BehaviorID] != event.BehaviorVersion {
			response.Rejected = append(response.Rejected, EventRejection{EventID: event.EventID, Code: "unknown_behavior"})
			continue
		}
		result, retry := h.commitOne(ctx, behavior, event)
		if retry {
			transient = true
			response.Rejected = append(response.Rejected, EventRejection{EventID: event.EventID, Code: "commit_failed"})
			continue
		}
		if result != "" {
			response.Rejected = append(response.Rejected, EventRejection{EventID: event.EventID, Code: result})
			continue
		}
		response.Accepted = append(response.Accepted, event.EventID)
	}
	return response, transient
}

// commitOne returns a rejection code, or retry=true for a transient failure.
func (h *Host) commitOne(ctx context.Context, behavior Behavior, event ObservationEnvelope) (string, bool) {
	eventHash := observationHash(event)
	h.fence.Lock()
	if h.isClosed() {
		h.fence.Unlock()
		return "lease_closed", false
	}
	if prior, ok := h.receipts[event.EventID]; ok {
		h.fence.Unlock()
		if prior != eventHash {
			return "identity_conflict", false
		}
		return "", false
	}
	config, ok := h.behaviorConfigs[event.BehaviorID]
	if !ok {
		h.fence.Unlock()
		return "config_unavailable", false
	}
	admission, err := behavior.Admit(config.state, event)
	if err != nil {
		h.fence.Unlock()
		h.logger.Debug("opencode plugin observation rejected",
			zap.String("behavior", event.BehaviorID), zap.Error(err))
		return "invalid_observation", false
	}
	epoch := h.admissionEpoch.Load()
	admission.Check = func() bool { return h.admissionEpoch.Load() == epoch }
	commitCtx, cancel := context.WithTimeout(ctx, RequestTimeout)
	publish, err := h.options.Committer.CommitOpenCodePluginObservation(commitCtx, admission)
	cancel()
	if err != nil {
		h.fence.Unlock()
		if errors.Is(err, ErrAdmissionRevoked) {
			return "admission_revoked", false
		}
		h.logger.Warn("opencode plugin observation commit failed", zap.Error(err))
		return "", true
	}
	if next, changed := behavior.Retire(config.state, admission); changed {
		raw, encodeErr := behavior.EncodeConfig(next)
		if encodeErr != nil {
			h.logger.Warn("opencode plugin claim retirement encode failed",
				zap.String("behavior", event.BehaviorID), zap.Error(encodeErr))
		} else {
			h.behaviorConfigs[event.BehaviorID] = behaviorConfigState{raw: raw, state: next}
			h.revision++
			h.notifyConfigLocked()
		}
	}
	h.recordReceiptLocked(event.EventID, eventHash)
	h.fence.Unlock()
	if publish != nil {
		publish()
	}
	return "", false
}

func validEnvelope(event ObservationEnvelope) bool {
	return event.EventID != "" && len(event.EventID) <= MaxIdentityBytes &&
		event.BehaviorID != "" && event.BehaviorVersion > 0 &&
		event.NativeSessionID != "" && event.NativeGeneration > 0
}

func observationHash(event ObservationEnvelope) string {
	var generation [8]byte
	binary.BigEndian.PutUint64(generation[:], event.NativeGeneration)
	sum := sha256.New()
	sum.Write([]byte(event.BehaviorID))
	sum.Write([]byte{0})
	sum.Write([]byte(strconv.Itoa(event.BehaviorVersion)))
	sum.Write([]byte{0})
	sum.Write([]byte(event.NativeSessionID))
	sum.Write([]byte{0})
	sum.Write(generation[:])
	sum.Write([]byte{0})
	sum.Write(event.Payload)
	return hex.EncodeToString(sum.Sum(nil))
}

func (h *Host) recordReceiptLocked(eventID, hash string) {
	if _, exists := h.receipts[eventID]; exists {
		return
	}
	h.receipts[eventID] = hash
	h.receiptOrder = append(h.receiptOrder, eventID)
	if len(h.receiptOrder) > 1024 {
		oldest := h.receiptOrder[0]
		h.receiptOrder = h.receiptOrder[1:]
		delete(h.receipts, oldest)
	}
}

func writePrivateJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode opencode plugin bootstrap: %w", err)
	}
	staging := path + ".tmp"
	if err := os.WriteFile(staging, data, 0o600); err != nil {
		return fmt.Errorf("write opencode plugin bootstrap: %w", err)
	}
	if err := os.Rename(staging, path); err != nil {
		_ = os.Remove(staging)
		return fmt.Errorf("publish opencode plugin bootstrap: %w", err)
	}
	return nil
}
