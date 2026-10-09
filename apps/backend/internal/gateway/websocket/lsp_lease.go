package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/lsp/protocol"
	"go.uber.org/zap"
)

const (
	lspLeaseSnapshotLimit                = 4 << 20
	lspLeaseDialTimeout                  = 15 * time.Second
	lspLeaseCloseTimeout                 = 5 * time.Second
	lspLeaseClientRequestTimeout         = 30 * time.Second
	lspLeaseMaxPendingClientRequests     = 256
	lspLeaseMaxClientRequestIDBytes      = 256
	lspLeaseMaxPendingClientRequestBytes = 64 << 10
	lspLeaseDetachedTimeout              = time.Hour
	lspCloseTransport                    = 4009
)

const (
	lspLeaseReleaseStop            = "stop"
	lspLeaseReleaseEditorIdle      = "editor_idle"
	lspLeaseReleaseDetachedTimeout = "detached_timeout"
	lspLeaseReleaseCapacity        = "capacity"
	lspLeaseReleaseServerExit      = "server_exit"
	lspLeaseReleaseRuntimeStop     = "runtime_stop"
	lspLeaseReleaseBackendStop     = "backend_stop"
)

type lspLeaseManager struct {
	mu              sync.Mutex
	admissionMu     sync.Mutex
	leases          map[string]*lspLease
	max             int
	detachedTimeout time.Duration
	closed          bool
	logger          *logger.Logger
}

type lspLeaseAgentCtlClient interface {
	DialLSP(context.Context, string, bool) (*websocket.Conn, *http.Response, error)
}

type lspLeaseExecution struct {
	ID                    string
	SessionID             string
	TaskID                string
	acquireAgentCtlClient func() (lspLeaseAgentCtlClient, func())
}

type lspLease struct {
	mu                            sync.Mutex
	manager                       *lspLeaseManager
	id                            string
	sessionID                     string
	taskID                        string
	executionID                   string
	userID                        string
	language                      string
	upstream                      *websocket.Conn
	upstreamWriteMu               sync.Mutex
	browserWriteMu                sync.Mutex
	browser                       *websocket.Conn
	generation                    uint64
	detachedAt                    time.Time
	expiryTimer                   *time.Timer
	closed                        bool
	expectedUpstreamClose         bool
	readDone                      chan struct{}
	ready                         bool
	readyStatus                   map[string]any
	workspacePath                 string
	workspaceURI                  string
	repoSubpaths                  []string
	initializeResult              []byte
	initializeServerID            []byte
	initializeResponse            []byte
	initializeWaiter              *lspLeaseInitializeWaiter
	initializedReceived           bool
	serverCapabilities            []byte
	registrations                 map[string][]byte
	progressTokens                map[string][]byte
	progress                      map[string][]byte
	configuration                 map[string]any
	configurationSnapshotBytes    int
	documentVersions              map[string]int64
	documentVersionsSnapshotBytes int
	registrationSnapshotBytes     int
	progressTokenSnapshotBytes    int
	progressSnapshotBytes         int
	openDocuments                 map[string]int64
	synchronizedDocs              map[string]bool
	clientRequests                map[string]lspLeaseClientRequest
	pendingClientRequestBytes     int
	clientRequestTimeout          time.Duration
	serverRequests                map[string]lspLeaseServerRequest
	brokerRequests                map[string]chan jsonRPCResponse
	requestCounter                uint64
	resumedGeneration             bool
}

type lspLeaseClientRequest struct {
	serverID   json.RawMessage
	generation uint64
	clientID   []byte
	byteSize   int
	timer      *time.Timer
}

type lspLeaseInitializeWaiter struct {
	generation uint64
	clientID   []byte
}

type lspLeaseServerRequest struct {
	serverID   []byte
	generation uint64
	method     string
}

func newLSPLeaseManager(max int, log *logger.Logger) *lspLeaseManager {
	if max <= 0 {
		max = defaultLSPMaxConnections
	}
	return &lspLeaseManager{leases: make(map[string]*lspLease), max: max, detachedTimeout: lspLeaseDetachedTimeout, logger: log.WithFields(zap.String("component", "lsp_lease_manager"))}
}

func (m *lspLeaseManager) attachOrCreate(
	ctx context.Context,
	execution lspLeaseExecution,
	language, userID, leaseHint string,
	configuration map[string]any,
	autoInstall bool,
	browser *websocket.Conn,
) (*lspLease, uint64, bool, error) {
	m.admissionMu.Lock()
	defer m.admissionMu.Unlock()

	if execution.ID == "" {
		return nil, 0, false, errors.New("LSP execution has no stable identity")
	}
	m.expireDetachedLeases()

	if err := m.retireReplacedLeases(execution); err != nil {
		return nil, 0, false, err
	}

	if lease := m.detachedCandidate(execution, language, userID, leaseHint); lease != nil {
		return m.reattachLease(lease, language, configuration, browser)
	}

	if err := m.ensureCapacity(); err != nil {
		return nil, 0, false, err
	}

	return m.createLease(ctx, execution, language, userID, configuration, autoInstall, browser)
}

func (m *lspLeaseManager) retireReplacedLeases(execution lspLeaseExecution) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("LSP lease manager is closed")
	}
	var stale []*lspLease
	for _, lease := range m.leases {
		if lease.sessionID == execution.SessionID && lease.executionID != execution.ID {
			stale = append(stale, lease)
		}
	}
	m.mu.Unlock()
	for _, lease := range stale {
		lease.terminate(lspCloseTransport, "execution replaced", lspLeaseReleaseRuntimeStop)
		lease.waitForClose(lspLeaseCloseTimeout)
	}
	return nil
}

func (m *lspLeaseManager) reattachLease(
	lease *lspLease,
	language string,
	configuration map[string]any,
	browser *websocket.Conn,
) (*lspLease, uint64, bool, error) {
	if err := lease.updateConfiguration(configuration, true); err != nil {
		lease.terminate(lspCloseTransport, "LSP state exceeded its bound", lspLeaseReleaseRuntimeStop)
		return nil, 0, false, err
	}
	generation, resumed, err := lease.attach(browser)
	if err != nil {
		return nil, 0, false, err
	}
	m.logger.Debug("reattached language server lease", zap.String("language", language))
	return lease, generation, resumed, nil
}

func (m *lspLeaseManager) createLease(
	ctx context.Context,
	execution lspLeaseExecution,
	language, userID string,
	configuration map[string]any,
	autoInstall bool,
	browser *websocket.Conn,
) (*lspLease, uint64, bool, error) {
	if execution.acquireAgentCtlClient == nil {
		return nil, 0, false, errors.New("agentctl unavailable")
	}
	client, releaseClient := execution.acquireAgentCtlClient()
	if releaseClient == nil {
		releaseClient = func() {}
	}
	if client == nil {
		releaseClient()
		return nil, 0, false, errors.New("agentctl unavailable")
	}
	dialCtx, cancel := context.WithTimeout(ctx, lspLeaseDialTimeout)
	upstream, response, err := client.DialLSP(dialCtx, language, autoInstall)
	cancel()
	releaseClient()
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, 0, false, err
	}
	upstream.SetReadLimit(protocol.MaxMessageBytes)

	lease := newLSPLease(m, execution, language, userID, configuration, upstream)
	if err := m.add(lease); err != nil {
		_ = upstream.Close()
		return nil, 0, false, err
	}
	generation, _, err := lease.attach(browser)
	if err != nil {
		lease.terminate(lspCloseTransport, "browser attachment failed", lspLeaseReleaseRuntimeStop)
		return nil, 0, false, err
	}
	go lease.readUpstream()
	m.logger.Debug("created language server lease", zap.String("language", language))
	return lease, generation, false, nil
}

func newLSPLease(
	manager *lspLeaseManager,
	execution lspLeaseExecution,
	language, userID string,
	configuration map[string]any,
	upstream *websocket.Conn,
) *lspLease {
	lease := &lspLease{
		manager:              manager,
		id:                   uuid.NewString(),
		sessionID:            execution.SessionID,
		taskID:               execution.TaskID,
		executionID:          execution.ID,
		userID:               userID,
		language:             language,
		upstream:             upstream,
		readDone:             make(chan struct{}),
		registrations:        make(map[string][]byte),
		progressTokens:       make(map[string][]byte),
		progress:             make(map[string][]byte),
		configuration:        cloneStringAnyMap(configuration),
		documentVersions:     make(map[string]int64),
		openDocuments:        make(map[string]int64),
		synchronizedDocs:     make(map[string]bool),
		clientRequests:       make(map[string]lspLeaseClientRequest),
		serverRequests:       make(map[string]lspLeaseServerRequest),
		brokerRequests:       make(map[string]chan jsonRPCResponse),
		clientRequestTimeout: lspLeaseClientRequestTimeout,
	}
	if encoded, err := json.Marshal(lease.configuration); err == nil {
		lease.configurationSnapshotBytes = len(encoded)
	}
	return lease
}

func (m *lspLeaseManager) detachedCandidate(
	execution lspLeaseExecution,
	language, userID, leaseHint string,
) *lspLease {
	m.mu.Lock()
	defer m.mu.Unlock()
	var candidates []*lspLease
	for _, lease := range m.leases {
		if lease.sessionID != execution.SessionID || lease.executionID != execution.ID || lease.language != language || lease.userID != userID {
			continue
		}
		if leaseHint != "" && lease.id == leaseHint {
			if lease.isDetached() {
				return lease
			}
			// A copied tab may inherit an attached tab's hint. Give it a new
			// independent lease instead of claiming a different detached window.
			return nil
		}
		if leaseHint != "" {
			continue
		}
		if !lease.isDetached() {
			continue
		}
		candidates = append(candidates, lease)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].detachedTime().After(candidates[j].detachedTime())
	})
	if len(candidates) == 0 {
		return nil
	}
	return candidates[0]
}

func (m *lspLeaseManager) ensureCapacity() error {
	m.expireDetachedLeases()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("LSP lease manager is closed")
	}
	if len(m.leases) < m.max {
		m.mu.Unlock()
		return nil
	}
	var oldest *lspLease
	for _, lease := range m.leases {
		if !lease.isDetached() {
			continue
		}
		if oldest == nil || lease.detachedTime().Before(oldest.detachedTime()) {
			oldest = lease
		}
	}
	m.mu.Unlock()
	if oldest == nil {
		return errLSPCapacityExceeded
	}
	oldest.terminate(lspCloseTransport, "language server lease evicted at capacity", lspLeaseReleaseCapacity)
	oldest.waitForClose(lspLeaseCloseTimeout)
	return nil
}

func (m *lspLeaseManager) expireDetachedLeases() {
	m.mu.Lock()
	leases := make([]*lspLease, 0, len(m.leases))
	for _, lease := range m.leases {
		leases = append(leases, lease)
	}
	m.mu.Unlock()
	for _, lease := range leases {
		lease.expireDetached(lease.detachedTime())
	}
}

func (m *lspLeaseManager) add(lease *lspLease) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("LSP lease manager is closed")
	}
	m.leases[lease.id] = lease
	m.updateMetricsLocked()
	return nil
}

func (m *lspLeaseManager) remove(lease *lspLease, reason string) {
	m.mu.Lock()
	if current := m.leases[lease.id]; current == lease {
		delete(m.leases, lease.id)
		m.updateMetricsLocked()
		lspLeaseReleasedTotal.Add(reason, 1)
		if reason == lspLeaseReleaseCapacity {
			lspLeaseEvictedTotal.Add(1)
		}
	}
	m.mu.Unlock()
}

func (m *lspLeaseManager) attachmentChanged() {
	m.mu.Lock()
	m.updateMetricsLocked()
	m.mu.Unlock()
}

func (m *lspLeaseManager) updateMetricsLocked() {
	active := int64(len(m.leases))
	detached := int64(0)
	for _, lease := range m.leases {
		if lease.isDetached() {
			detached++
		}
	}
	lspLeaseActiveGauge.Set(active)
	lspLeaseDetachedGauge.Set(detached)
}

func (m *lspLeaseManager) hasActiveSession(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, lease := range m.leases {
		if lease.sessionID == sessionID && !lease.isClosed() {
			return true
		}
	}
	return false
}

func (m *lspLeaseManager) hasActiveExecution(executionID string) bool {
	if executionID == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, lease := range m.leases {
		if lease.executionID == executionID && !lease.isClosed() {
			return true
		}
	}
	return false
}

func (m *lspLeaseManager) stopSession(sessionID string) {
	m.stopMatching(func(lease *lspLease) bool { return lease.sessionID == sessionID }, lspLeaseReleaseRuntimeStop)
}

func (m *lspLeaseManager) stopTask(taskID string) {
	m.stopMatching(func(lease *lspLease) bool { return lease.taskID == taskID }, lspLeaseReleaseRuntimeStop)
}

func (m *lspLeaseManager) stopExecution(executionID string) {
	m.stopMatching(func(lease *lspLease) bool { return lease.executionID == executionID }, lspLeaseReleaseRuntimeStop)
}

func (m *lspLeaseManager) updateUserConfiguration(userID string, configurations map[string]map[string]any) {
	if userID == "" {
		return
	}
	m.mu.Lock()
	leases := make([]*lspLease, 0)
	for _, lease := range m.leases {
		if lease.userID == userID {
			leases = append(leases, lease)
		}
	}
	m.mu.Unlock()
	for _, lease := range leases {
		if err := lease.updateConfiguration(configurations[lease.language], true); err != nil {
			lease.terminate(lspCloseTransport, "language server configuration exceeded its bound", lspLeaseReleaseRuntimeStop)
		}
	}
}

func (m *lspLeaseManager) stopMatching(match func(*lspLease) bool, reason string) {
	m.mu.Lock()
	leases := make([]*lspLease, 0)
	for _, lease := range m.leases {
		if match(lease) {
			leases = append(leases, lease)
		}
	}
	m.mu.Unlock()
	for _, lease := range leases {
		lease.terminate(lspCloseRuntimeStopped, "task runtime stopped", reason)
	}
}

func (m *lspLeaseManager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	leases := make([]*lspLease, 0, len(m.leases))
	for _, lease := range m.leases {
		leases = append(leases, lease)
	}
	m.mu.Unlock()
	for _, lease := range leases {
		lease.terminate(websocket.CloseGoingAway, "backend is shutting down", lspLeaseReleaseBackendStop)
	}
	deadline := time.Now().Add(lspLeaseCloseTimeout)
	for _, lease := range leases {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		lease.waitForClose(remaining)
	}
}

func (l *lspLease) attach(browser *websocket.Conn) (uint64, bool, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return 0, false, errors.New("LSP lease has ended")
	}
	if l.browser != nil {
		l.mu.Unlock()
		return 0, false, errors.New("LSP lease is already attached")
	}
	l.generation++
	generation := l.generation
	l.browser = browser
	if l.expiryTimer != nil {
		l.expiryTimer.Stop()
		l.expiryTimer = nil
	}
	l.detachedAt = time.Time{}
	resumed := len(l.initializeResult) > 0
	l.resumedGeneration = resumed
	ready, status := l.ready, cloneAnyMap(l.readyStatus)
	l.mu.Unlock()
	l.manager.attachmentChanged()
	if ready {
		if err := l.writeBrowser(generation, l.readyHandshake(resumed)); err != nil {
			l.detach(generation)
			return generation, resumed, err
		}
	} else if status != nil {
		if err := l.writeBrowser(generation, status); err != nil {
			l.detach(generation)
			return generation, resumed, err
		}
	}
	return generation, resumed, nil
}

func (l *lspLease) detach(generation uint64) {
	// Serialize the generation change with browser-originated upstream writes.
	// This prevents a frame already read from a stale tab from landing after
	// the lease has detached or been claimed by another tab.
	l.upstreamWriteMu.Lock()
	defer l.upstreamWriteMu.Unlock()
	l.mu.Lock()
	if l.closed || l.browser == nil || l.generation != generation {
		l.mu.Unlock()
		return
	}
	browser := l.browser
	l.browser = nil
	l.detachedAt = time.Now()
	detachedAt := l.detachedAt
	l.expiryTimer = time.AfterFunc(l.manager.detachedTimeout, func() {
		l.manager.admissionMu.Lock()
		defer l.manager.admissionMu.Unlock()
		l.expireDetached(detachedAt)
	})
	documents := make([]string, 0, len(l.openDocuments))
	for uri := range l.openDocuments {
		documents = append(documents, uri)
		l.synchronizedDocs[uri] = false
	}
	clear(l.openDocuments)
	clientRequests := make([]lspLeaseClientRequest, 0)
	for key, request := range l.clientRequests {
		if request.generation == generation {
			if removed, ok := l.removeClientRequestLocked(key); ok {
				clientRequests = append(clientRequests, removed)
			}
		}
	}
	serverRequests := make([]lspLeaseServerRequest, 0)
	for key, request := range l.serverRequests {
		if request.generation == generation {
			serverRequests = append(serverRequests, request)
			delete(l.serverRequests, key)
		}
	}
	l.mu.Unlock()
	_ = browser.Close()
	for _, request := range clientRequests {
		_ = l.writeUpstreamLocked(jsonRPCNotification("$/cancelRequest", map[string]any{"id": request.serverID}))
	}
	for _, request := range serverRequests {
		_ = l.writeUpstreamLocked(jsonRPCErrorResponseRaw(request.serverID, -32800, "request cancelled because the browser detached"))
	}
	for _, uri := range documents {
		_ = l.writeUpstreamLocked(jsonRPCNotification("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": uri}}))
	}
	l.manager.attachmentChanged()
	l.manager.logger.Debug("detached language server lease", zap.String("language", l.language))
}

func (l *lspLease) isDetached() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.closed && l.browser == nil
}

func (l *lspLease) isClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

func (l *lspLease) detachedTime() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.detachedAt
}

func (l *lspLease) readUpstream() {
	defer close(l.readDone)
	for {
		messageType, message, err := l.upstream.ReadMessage()
		if err != nil {
			l.mu.Lock()
			expectedClose := l.expectedUpstreamClose
			l.mu.Unlock()
			if expectedClose {
				return
			}
			code, text, reason := classifyLSPUpstreamClose(err)
			l.terminate(code, text, reason)
			return
		}
		if messageType != websocket.TextMessage {
			continue
		}
		if err := l.handleUpstreamMessage(message); err != nil {
			l.manager.logger.Warn("LSP lease broker rejected upstream frame", zap.String("language", l.language), zap.Error(err))
			l.terminate(lspCloseTransport, "language server protocol state exceeded its bound", lspLeaseReleaseRuntimeStop)
			return
		}
	}
}

func classifyLSPUpstreamClose(err error) (int, string, string) {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		if closeErr.Code == 4006 {
			return 4006, closeErr.Text, lspLeaseReleaseServerExit
		}
		if closeErr.Code != websocket.CloseNoStatusReceived && closeErr.Code != websocket.CloseAbnormalClosure {
			return closeErr.Code, closeErr.Text, lspLeaseReleaseRuntimeStop
		}
	}
	return lspCloseTransport, "LSP transport failed", lspLeaseReleaseRuntimeStop
}

func (l *lspLease) terminate(code int, text, reason string) {
	l.terminateIfDetachedAt(code, text, reason, time.Time{})
}

func (l *lspLease) expireDetached(detachedAt time.Time) {
	if detachedAt.IsZero() {
		return
	}
	l.terminateIfDetachedAt(lspCloseTransport, "detached language server lease expired", lspLeaseReleaseDetachedTimeout, detachedAt)
}

func (l *lspLease) terminateIfDetachedAt(code int, text, reason string, detachedAt time.Time) {
	l.mu.Lock()
	if l.closed || (!detachedAt.IsZero() && (l.browser != nil || !l.detachedAt.Equal(detachedAt) || time.Since(detachedAt) < l.manager.detachedTimeout)) {
		l.mu.Unlock()
		return
	}
	l.closed = true
	if l.expiryTimer != nil {
		l.expiryTimer.Stop()
		l.expiryTimer = nil
	}
	browser := l.browser
	l.browser = nil
	l.openDocuments = make(map[string]int64)
	l.synchronizedDocs = make(map[string]bool)
	for key := range l.clientRequests {
		l.removeClientRequestLocked(key)
	}
	l.clientRequests = make(map[string]lspLeaseClientRequest)
	l.pendingClientRequestBytes = 0
	l.serverRequests = make(map[string]lspLeaseServerRequest)
	l.mu.Unlock()
	if browser != nil {
		l.browserWriteMu.Lock()
		closeLSPConnWithCode(browser, code, text)
		l.browserWriteMu.Unlock()
	}
	if l.upstream != nil {
		_ = l.upstream.Close()
	}
	l.manager.remove(l, reason)
}

func (l *lspLease) waitForClose(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-l.readDone:
	case <-timer.C:
	}
}

func (l *lspLease) writeUpstream(message []byte) error {
	l.upstreamWriteMu.Lock()
	defer l.upstreamWriteMu.Unlock()
	return l.writeUpstreamLocked(message)
}

func (l *lspLease) writeUpstreamForGeneration(generation uint64, message []byte) error {
	l.upstreamWriteMu.Lock()
	defer l.upstreamWriteMu.Unlock()
	l.mu.Lock()
	active := !l.closed && l.browser != nil && l.generation == generation
	l.mu.Unlock()
	if !active {
		return nil
	}
	return l.writeUpstreamLocked(message)
}

func (l *lspLease) writeUpstreamLocked(message []byte) error {
	if err := l.upstream.SetWriteDeadline(time.Now().Add(lspProxyWriteTimeout)); err != nil {
		return err
	}
	return l.upstream.WriteMessage(websocket.TextMessage, message)
}

func (l *lspLease) isActiveGeneration(generation uint64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.closed && l.browser != nil && l.generation == generation
}

func (l *lspLease) writeBrowserOrDetach(generation uint64, value any) error {
	if err := l.writeBrowser(generation, value); err != nil {
		l.detach(generation)
	}
	return nil
}

func (l *lspLease) writeBrowser(generation uint64, value any) error {
	message, err := marshalBoundedJSON(value, protocol.MaxMessageBytes)
	if err != nil {
		return err
	}
	l.browserWriteMu.Lock()
	defer l.browserWriteMu.Unlock()
	l.mu.Lock()
	conn := l.browser
	current := !l.closed && conn != nil && l.generation == generation
	l.mu.Unlock()
	if !current {
		return errors.New("browser attachment generation is no longer active")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(lspProxyWriteTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, message)
}

func (h *LSPHandler) HasActiveLSPLease(sessionID string) bool {
	return h.leases != nil && h.leases.hasActiveSession(sessionID)
}

func (h *LSPHandler) HasActiveLSPLeaseForExecution(executionID string) bool {
	return h.leases != nil && h.leases.hasActiveExecution(executionID)
}

func (h *LSPHandler) StopLSPLeasesForSession(sessionID string) {
	if h.leases != nil {
		h.leases.stopSession(sessionID)
	}
}

func (h *LSPHandler) StopLSPLeasesForTask(taskID string) {
	if h.leases != nil {
		h.leases.stopTask(taskID)
	}
}

func (h *LSPHandler) StopLSPLeasesForExecution(executionID string) {
	if h.leases != nil {
		h.leases.stopExecution(executionID)
	}
}
