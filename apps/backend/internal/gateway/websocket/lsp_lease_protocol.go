package websocket

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	lspControlKind       = "lsp"
	lspControlField      = "kandev"
	lspControlAttachSync = "attachmentReady"
	lspControlRelease    = "release"
	lspControlAck        = "released"
	lspLeaseStatusReady  = "ready"
	lspMethodInitialize  = "initialize"
	lspMethodDidOpen     = "textDocument/didOpen"
	lspMethodDidChange   = "textDocument/didChange"
	lspMethodDidClose    = "textDocument/didClose"
)

type jsonRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type jsonRPCResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

type lspControlMessage struct {
	Kind      string `json:"kandev"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	RequestID string `json:"requestId"`
}

func (l *lspLease) handleUpstreamMessage(message []byte) error {
	var status struct {
		Status             string   `json:"status"`
		WorkspacePath      string   `json:"workspacePath"`
		WorkspaceURI       string   `json:"workspaceUri"`
		RepositorySubpaths []string `json:"repoSubpaths"`
	}
	if json.Unmarshal(message, &status) == nil && status.Status != "" {
		return l.handleTaskHostStatus(message, status.Status, status.WorkspacePath, status.WorkspaceURI, status.RepositorySubpaths)
	}
	var rpc jsonRPCMessage
	if err := json.Unmarshal(message, &rpc); err != nil || rpc.JSONRPC != "2.0" {
		return fmt.Errorf("invalid upstream JSON-RPC frame")
	}
	if rpc.Method != "" && len(rpc.ID) != 0 {
		return l.handleServerRequest(rpc, message)
	}
	if rpc.Method == "" && len(rpc.ID) != 0 {
		return l.handleServerResponse(rpc, message)
	}
	if rpc.Method != "" {
		return l.handleServerNotification(rpc, message)
	}
	return fmt.Errorf("unrecognized JSON-RPC frame")
}

func (l *lspLease) handleTaskHostStatus(raw []byte, status, workspacePath, workspaceURI string, subpaths []string) error {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	l.mu.Lock()
	l.readyStatus = cloneAnyMap(value)
	if status == lspLeaseStatusReady {
		l.ready = true
		l.workspacePath = workspacePath
		l.workspaceURI = workspaceURI
		l.repoSubpaths = append([]string(nil), subpaths...)
	}
	if err := l.checkSnapshotLimitLocked(); err != nil {
		l.mu.Unlock()
		return err
	}
	generation, attached := l.generation, l.browser != nil
	resumed := len(l.initializeResult) > 0
	l.mu.Unlock()
	if !attached {
		return nil
	}
	if status == lspLeaseStatusReady {
		return l.writeBrowserOrDetach(generation, l.readyHandshake(resumed))
	}
	return l.writeBrowserOrDetach(generation, value)
}

func (l *lspLease) readyHandshake(resumed bool) map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	ready := cloneAnyMap(l.readyStatus)
	if ready == nil {
		ready = make(map[string]any)
	}
	ready["status"] = lspLeaseStatusReady
	ready["leaseId"] = l.id
	ready["resumed"] = resumed
	ready["initialized"] = l.initializedReceived
	ready["workspacePath"] = l.workspacePath
	ready["workspaceUri"] = l.workspaceURI
	ready["repoSubpaths"] = append([]string(nil), l.repoSubpaths...)
	ready["registrations"] = l.rawMapValues(l.registrations)
	ready["progressTokens"] = l.rawMapValues(l.progressTokens)
	ready["progress"] = l.rawMapValues(l.progress)
	if len(l.serverCapabilities) > 0 {
		var capabilities any
		if json.Unmarshal(l.serverCapabilities, &capabilities) == nil {
			ready["capabilities"] = capabilities
		}
	}
	return ready
}

func (l *lspLease) rawMapValues(values map[string][]byte) []any {
	result := make([]any, 0, len(values))
	for _, raw := range values {
		var value any
		if json.Unmarshal(raw, &value) == nil {
			result = append(result, value)
		}
	}
	return result
}

func (l *lspLease) handleServerRequest(rpc jsonRPCMessage, original []byte) error {
	brokered := false
	switch rpc.Method {
	case "workspace/configuration":
		return l.answerWorkspaceConfiguration(rpc.ID, rpc.Params)
	case "client/registerCapability":
		brokered = true
		if err := l.updateRegistrations(rpc.Method, rpc.Params); err != nil {
			return err
		}
	case "client/unregisterCapability":
		brokered = true
		if err := l.updateRegistrations(rpc.Method, rpc.Params); err != nil {
			return err
		}
	case "window/workDoneProgress/create":
		brokered = true
		if err := l.addProgressToken(rpc.Params); err != nil {
			return err
		}
	}

	l.mu.Lock()
	if err := l.checkSnapshotLimitLocked(); err != nil {
		l.mu.Unlock()
		return err
	}
	generation, browser := l.generation, l.browser
	if browser == nil {
		l.mu.Unlock()
		if brokered {
			return l.writeUpstream(jsonRPCResultResponse(rpc.ID, nil))
		}
		return l.writeUpstream(jsonRPCErrorResponseRaw(rpc.ID, -32601, "method unavailable while the editor is detached"))
	}
	l.requestCounter++
	forwardedID := jsonRPCStringID(fmt.Sprintf("kandev:server:%d", l.requestCounter))
	key := jsonRPCIDKey(forwardedID)
	l.serverRequests[key] = lspLeaseServerRequest{serverID: append([]byte(nil), rpc.ID...), generation: generation, method: rpc.Method}
	l.mu.Unlock()
	forwarded, err := replaceJSONRPCID(original, forwardedID)
	if err != nil {
		return err
	}
	if err = l.writeBrowser(generation, json.RawMessage(forwarded)); err != nil {
		l.detach(generation)
		return nil
	}
	return nil
}

func (l *lspLease) answerWorkspaceConfiguration(id, params json.RawMessage) error {
	var request struct {
		Items []struct {
			Section string `json:"section"`
		} `json:"items"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &request); err != nil {
			return l.writeUpstream(jsonRPCErrorResponseRaw(id, -32602, "invalid workspace/configuration parameters"))
		}
	}
	l.mu.Lock()
	configuration := cloneStringAnyMap(l.configuration)
	l.mu.Unlock()
	result := make([]any, len(request.Items))
	for index := range result {
		result[index] = configurationSection(configuration, request.Items[index].Section, l.language)
	}
	return l.writeUpstream(jsonRPCResultResponse(id, result))
}

func (l *lspLease) handleServerResponse(rpc jsonRPCMessage, original []byte) error {
	key := jsonRPCIDKey(rpc.ID)
	if response := l.takeBrokerResponse(key); response != nil {
		response <- jsonRPCResponse{ID: append([]byte(nil), rpc.ID...), Result: append([]byte(nil), rpc.Result...), Error: append([]byte(nil), rpc.Error...)}
		return nil
	}
	if handled, err := l.handleInitializeResponse(key, rpc, original); handled {
		return err
	}
	return l.handleClientResponse(key, original)
}

func (l *lspLease) takeBrokerResponse(key string) chan jsonRPCResponse {
	l.mu.Lock()
	defer l.mu.Unlock()
	if responseCh, ok := l.brokerRequests[key]; ok {
		delete(l.brokerRequests, key)
		return responseCh
	}
	return nil
}

func (l *lspLease) handleInitializeResponse(
	key string,
	rpc jsonRPCMessage,
	original []byte,
) (bool, error) {
	l.mu.Lock()
	if len(l.initializeServerID) == 0 || jsonRPCIDKey(l.initializeServerID) != key {
		l.mu.Unlock()
		return false, nil
	}
	initializeFailed := len(rpc.Error) > 0 && string(rpc.Error) != "null"
	if initializeFailed {
		l.initializeResponse = nil
		l.initializeResult = nil
		l.serverCapabilities = nil
	} else {
		l.initializeResponse = append(l.initializeResponse[:0], original...)
	}
	l.initializeServerID = nil
	waiter := l.initializeWaiter
	l.initializeWaiter = nil
	if !initializeFailed && len(rpc.Result) > 0 {
		l.initializeResult = append(l.initializeResult[:0], rpc.Result...)
		var result struct {
			Capabilities json.RawMessage `json:"capabilities"`
		}
		if json.Unmarshal(rpc.Result, &result) == nil {
			l.serverCapabilities = append(l.serverCapabilities[:0], result.Capabilities...)
		}
	}
	if err := l.checkSnapshotLimitLocked(); err != nil {
		l.mu.Unlock()
		return true, err
	}
	l.mu.Unlock()
	if waiter == nil || !l.isActiveGeneration(waiter.generation) {
		return true, nil
	}
	forwarded, err := replaceJSONRPCID(original, waiter.clientID)
	if err != nil {
		return true, err
	}
	return true, l.writeBrowserOrDetach(waiter.generation, json.RawMessage(forwarded))
}

func (l *lspLease) handleClientResponse(key string, original []byte) error {
	l.mu.Lock()
	var pendingID string
	var pending lspLeaseClientRequest
	for clientID, request := range l.clientRequests {
		if jsonRPCIDKey(request.serverID) == key {
			pendingID, pending = clientID, request
			l.removeClientRequestLocked(clientID)
			break
		}
	}
	if pendingID == "" {
		l.mu.Unlock()
		return nil
	}
	attached := l.browser != nil && l.generation == pending.generation
	generation := l.generation
	l.mu.Unlock()
	if !attached {
		return nil
	}
	forwarded, err := replaceJSONRPCID(original, pending.clientID)
	if err != nil {
		return err
	}
	return l.writeBrowserOrDetach(generation, json.RawMessage(forwarded))
}

func (l *lspLease) handleServerNotification(rpc jsonRPCMessage, original []byte) error {
	switch rpc.Method {
	case "$/progress":
		if err := l.updateProgress(rpc.Params); err != nil {
			return err
		}
	case "textDocument/publishDiagnostics":
		allowed, err := l.acceptDiagnostics(rpc.Params)
		if err != nil || !allowed {
			return err
		}
	}
	l.mu.Lock()
	generation, attached := l.generation, l.browser != nil
	l.mu.Unlock()
	if !attached {
		return nil
	}
	if err := l.writeBrowser(generation, json.RawMessage(original)); err != nil {
		l.detach(generation)
		return nil
	}
	return nil
}

func (l *lspLease) handleBrowserMessage(generation uint64, message []byte) error {
	var control lspControlMessage
	if json.Unmarshal(message, &control) == nil && control.Kind == lspControlKind {
		return l.handleControl(generation, control)
	}
	if !l.isActiveGeneration(generation) {
		return nil
	}
	var rpc jsonRPCMessage
	if err := json.Unmarshal(message, &rpc); err != nil || rpc.JSONRPC != "2.0" {
		return errors.New("invalid browser JSON-RPC frame")
	}
	if rpc.Method == "" && len(rpc.ID) > 0 {
		return l.handleBrowserResponse(generation, rpc, message)
	}
	if rpc.Method == "" {
		return nil
	}
	if handled, err := l.handleBrowserMethod(generation, rpc, message); handled {
		return err
	}
	forwarded, active, err := l.rewriteBrowserDocument(generation, rpc, message)
	if err != nil || !active {
		return err
	}
	if len(rpc.ID) == 0 {
		return l.writeUpstreamForGeneration(generation, forwarded)
	}
	return l.forwardBrowserRequest(generation, rpc, forwarded)
}

func (l *lspLease) handleBrowserMethod(
	generation uint64,
	rpc jsonRPCMessage,
	original []byte,
) (bool, error) {
	switch rpc.Method {
	case lspMethodInitialize:
		return true, l.handleInitializeRequest(generation, rpc, original)
	case "initialized":
		return true, l.handleInitializedNotification(generation, original)
	case "workspace/didChangeConfiguration":
		var params struct {
			Settings map[string]any `json:"settings"`
		}
		if json.Unmarshal(rpc.Params, &params) == nil && params.Settings != nil {
			return true, l.updateConfiguration(params.Settings, true)
		}
	case "$/cancelRequest":
		return true, l.forwardCancellation(generation, rpc, original)
	}
	return false, nil
}

func (l *lspLease) rewriteBrowserDocument(
	generation uint64,
	rpc jsonRPCMessage,
	original []byte,
) ([]byte, bool, error) {
	switch rpc.Method {
	case lspMethodDidOpen, lspMethodDidChange, lspMethodDidClose:
		return l.rewriteDocumentVersionForGeneration(generation, rpc.Method, original)
	default:
		return append([]byte(nil), original...), true, nil
	}
}

func (l *lspLease) forwardBrowserRequest(
	generation uint64,
	rpc jsonRPCMessage,
	message []byte,
) error {
	l.mu.Lock()
	if l.closed || l.generation != generation || l.browser == nil {
		l.mu.Unlock()
		return nil
	}
	l.requestCounter++
	serverID := jsonRPCStringID(fmt.Sprintf("kandev:client:%d", l.requestCounter))
	clientKey := jsonRPCIDKey(rpc.ID)
	if err := l.addClientRequestLocked(generation, rpc.ID, serverID); err != nil {
		l.mu.Unlock()
		return err
	}
	if err := l.checkSnapshotLimitLocked(); err != nil {
		l.removeClientRequestLocked(clientKey)
		l.mu.Unlock()
		return err
	}
	l.mu.Unlock()
	forwarded, err := replaceJSONRPCID(message, serverID)
	if err != nil {
		return err
	}
	if err = l.writeUpstreamForGeneration(generation, forwarded); err != nil {
		l.mu.Lock()
		l.removeClientRequestLocked(clientKey)
		l.mu.Unlock()
	}
	return err
}

func (l *lspLease) handleBrowserResponse(generation uint64, rpc jsonRPCMessage, original []byte) error {
	key := jsonRPCIDKey(rpc.ID)
	l.mu.Lock()
	pending, ok := l.serverRequests[key]
	if !ok || pending.generation != generation || l.generation != generation || l.browser == nil {
		l.mu.Unlock()
		return nil
	}
	delete(l.serverRequests, key)
	l.mu.Unlock()
	forwarded, err := replaceJSONRPCID(original, pending.serverID)
	if err != nil {
		return err
	}
	return l.writeUpstreamForGeneration(generation, forwarded)
}

func (l *lspLease) handleInitializeRequest(generation uint64, rpc jsonRPCMessage, original []byte) error {
	if len(rpc.ID) == 0 {
		return errors.New("initialize request has no ID")
	}
	l.upstreamWriteMu.Lock()
	l.mu.Lock()
	if l.closed || l.browser == nil || l.generation != generation {
		l.mu.Unlock()
		l.upstreamWriteMu.Unlock()
		return nil
	}
	if len(l.initializeResponse) > 0 {
		response := append([]byte(nil), l.initializeResponse...)
		l.mu.Unlock()
		l.upstreamWriteMu.Unlock()
		forwarded, err := replaceJSONRPCID(response, rpc.ID)
		if err != nil {
			return err
		}
		return l.writeBrowserOrDetach(generation, json.RawMessage(forwarded))
	}
	if len(l.initializeServerID) > 0 {
		l.initializeWaiter = &lspLeaseInitializeWaiter{generation: generation, clientID: append([]byte(nil), rpc.ID...)}
		l.mu.Unlock()
		l.upstreamWriteMu.Unlock()
		return nil
	}
	l.requestCounter++
	serverID := jsonRPCStringID(fmt.Sprintf("kandev:initialize:%d", l.requestCounter))
	l.initializeServerID = append([]byte(nil), serverID...)
	l.initializeWaiter = &lspLeaseInitializeWaiter{generation: generation, clientID: append([]byte(nil), rpc.ID...)}
	var params struct {
		WorkDoneToken json.RawMessage `json:"workDoneToken"`
	}
	if len(rpc.Params) > 0 && json.Unmarshal(rpc.Params, &params) == nil && len(params.WorkDoneToken) > 0 {
		storeSnapshotByteSlice(l.progressTokens, jsonRPCIDKey(params.WorkDoneToken), params.WorkDoneToken, &l.progressTokenSnapshotBytes)
	}
	if err := l.checkSnapshotLimitLocked(); err != nil {
		l.initializeServerID = nil
		l.initializeWaiter = nil
		l.mu.Unlock()
		l.upstreamWriteMu.Unlock()
		return err
	}
	l.mu.Unlock()
	forwarded, err := replaceJSONRPCID(original, serverID)
	if err == nil {
		err = l.writeUpstreamLocked(forwarded)
	}
	l.upstreamWriteMu.Unlock()
	return err
}

func (l *lspLease) handleInitializedNotification(generation uint64, message []byte) error {
	l.upstreamWriteMu.Lock()
	defer l.upstreamWriteMu.Unlock()
	l.mu.Lock()
	if l.closed || l.browser == nil || l.generation != generation || l.initializedReceived {
		l.mu.Unlock()
		return nil
	}
	l.mu.Unlock()
	if err := l.writeUpstreamLocked(message); err != nil {
		return err
	}
	l.mu.Lock()
	if !l.closed && l.browser != nil && l.generation == generation {
		l.initializedReceived = true
	}
	l.mu.Unlock()
	return nil
}

func (l *lspLease) forwardCancellation(generation uint64, rpc jsonRPCMessage, original []byte) error {
	var params struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(rpc.Params, &params); err != nil || len(params.ID) == 0 {
		return nil
	}
	key := jsonRPCIDKey(params.ID)
	l.mu.Lock()
	pending, ok := l.clientRequests[key]
	if ok && pending.generation == generation {
		pending, ok = l.removeClientRequestLocked(key)
	}
	l.mu.Unlock()
	if !ok || pending.generation != generation {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(original, &raw); err != nil {
		return err
	}
	var cancelParams map[string]json.RawMessage
	if err := json.Unmarshal(raw["params"], &cancelParams); err != nil {
		return err
	}
	cancelParams["id"] = append([]byte(nil), pending.serverID...)
	encoded, err := json.Marshal(cancelParams)
	if err != nil {
		return err
	}
	raw["params"] = encoded
	forwarded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return l.writeUpstreamForGeneration(generation, forwarded)
}

func (l *lspLease) rewriteDocumentVersion(method string, original []byte) ([]byte, error) {
	message, _, err := l.rewriteDocumentVersionInternal(0, method, original, false)
	return message, err
}

func (l *lspLease) rewriteDocumentVersionForGeneration(generation uint64, method string, original []byte) ([]byte, bool, error) {
	return l.rewriteDocumentVersionInternal(generation, method, original, true)
}

func (l *lspLease) rewriteDocumentVersionInternal(
	generation uint64,
	method string,
	original []byte,
	checkGeneration bool,
) ([]byte, bool, error) {
	raw, params, document, uri, err := decodeDocumentSyncMessage(original)
	if err != nil {
		return nil, false, err
	}
	l.mu.Lock()
	if checkGeneration && (l.closed || l.browser == nil || l.generation != generation) {
		l.mu.Unlock()
		return nil, false, nil
	}
	switch method {
	case lspMethodDidOpen, lspMethodDidChange:
		previous := l.documentVersions[uri]
		version := previous + 1
		if previous > 0 {
			l.documentVersionsSnapshotBytes -= snapshotDocumentVersionEntrySize(uri, previous)
		}
		l.documentVersions[uri] = version
		l.documentVersionsSnapshotBytes += snapshotDocumentVersionEntrySize(uri, version)
		l.openDocuments[uri] = version
		l.synchronizedDocs[uri] = true
		encodedVersion, _ := json.Marshal(version)
		document["version"] = encodedVersion
	case lspMethodDidClose:
		delete(l.openDocuments, uri)
		delete(l.synchronizedDocs, uri)
	}
	if err := l.checkSnapshotLimitLocked(); err != nil {
		l.mu.Unlock()
		return nil, false, err
	}
	l.mu.Unlock()
	encodedDocument, err := json.Marshal(document)
	if err != nil {
		return nil, false, err
	}
	params["textDocument"] = encodedDocument
	encodedParams, err := json.Marshal(params)
	if err != nil {
		return nil, false, err
	}
	raw["params"] = encodedParams
	message, err := json.Marshal(raw)
	return message, true, err
}

func decodeDocumentSyncMessage(
	original []byte,
) (map[string]json.RawMessage, map[string]json.RawMessage, map[string]json.RawMessage, string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(original, &raw); err != nil {
		return nil, nil, nil, "", err
	}
	var params map[string]json.RawMessage
	if err := json.Unmarshal(raw["params"], &params); err != nil {
		return nil, nil, nil, "", err
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(params["textDocument"], &document); err != nil {
		return nil, nil, nil, "", err
	}
	var uri string
	if err := json.Unmarshal(document["uri"], &uri); err != nil || uri == "" {
		return nil, nil, nil, "", errors.New("document synchronization has no URI")
	}
	return raw, params, document, uri, nil
}

func (l *lspLease) acceptDiagnostics(params json.RawMessage) (bool, error) {
	var diagnostic struct {
		URI     string `json:"uri"`
		Version *int64 `json:"version"`
	}
	if err := json.Unmarshal(params, &diagnostic); err != nil {
		return false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	current, open := l.openDocuments[diagnostic.URI]
	if !open || !l.synchronizedDocs[diagnostic.URI] {
		return false, nil
	}
	if diagnostic.Version == nil {
		return !l.resumedGeneration, nil
	}
	return *diagnostic.Version == current, nil
}

func (l *lspLease) updateRegistrations(method string, params json.RawMessage) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(params, &payload); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if method == "client/registerCapability" {
		var registrations []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload["registrations"], &registrations); err != nil {
			return err
		}
		var rawItems []json.RawMessage
		if err := json.Unmarshal(payload["registrations"], &rawItems); err != nil {
			return err
		}
		for index, registration := range registrations {
			if registration.ID != "" && index < len(rawItems) {
				storeSnapshotByteSlice(l.registrations, registration.ID, rawItems[index], &l.registrationSnapshotBytes)
			}
		}
		return l.checkSnapshotLimitLocked()
	}
	items := payload["unregistrations"]
	if len(items) == 0 {
		items = payload["unregisterations"]
	}
	var unregistrations []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(items, &unregistrations); err != nil {
		return err
	}
	for _, registration := range unregistrations {
		deleteSnapshotByteSlice(l.registrations, registration.ID, &l.registrationSnapshotBytes)
	}
	return l.checkSnapshotLimitLocked()
}

func (l *lspLease) addProgressToken(params json.RawMessage) error {
	var payload struct {
		Token json.RawMessage `json:"token"`
	}
	if err := json.Unmarshal(params, &payload); err != nil || len(payload.Token) == 0 {
		return errors.New("progress creation request has no token")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	storeSnapshotByteSlice(l.progressTokens, jsonRPCIDKey(payload.Token), payload.Token, &l.progressTokenSnapshotBytes)
	return l.checkSnapshotLimitLocked()
}

func (l *lspLease) updateProgress(params json.RawMessage) error {
	var payload struct {
		Token json.RawMessage `json:"token"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(params, &payload); err != nil || len(payload.Token) == 0 || len(payload.Value) == 0 {
		return nil
	}
	key := jsonRPCIDKey(payload.Token)
	var value struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(payload.Value, &value)
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, registered := l.progressTokens[key]; !registered {
		return nil
	}
	switch value.Kind {
	case "end":
		deleteSnapshotByteSlice(l.progress, key, &l.progressSnapshotBytes)
	case "report":
		storeSnapshotByteSlice(l.progress, key, mergeProgressReport(l.progress[key], params, payload.Value), &l.progressSnapshotBytes)
	default:
		storeSnapshotByteSlice(l.progress, key, params, &l.progressSnapshotBytes)
	}
	return l.checkSnapshotLimitLocked()
}

func (l *lspLease) handleControl(generation uint64, control lspControlMessage) error {
	l.mu.Lock()
	active := !l.closed && l.browser != nil && l.generation == generation
	if active && control.Action == lspControlAttachSync {
		for uri := range l.openDocuments {
			l.synchronizedDocs[uri] = true
		}
	}
	l.mu.Unlock()
	if !active {
		return nil
	}
	if control.Action == lspControlAttachSync {
		return l.writeBrowser(generation, map[string]any{
			lspControlField: lspControlKind,
			"action":        lspControlAttachSync,
			"requestId":     control.RequestID,
		})
	}
	if control.Action != lspControlRelease {
		return errors.New("unknown LSP broker control action")
	}
	reason := control.Reason
	if reason != lspLeaseReleaseEditorIdle {
		reason = lspLeaseReleaseStop
	}
	if err := l.gracefulRelease(generation, reason, control.RequestID); err != nil {
		return err
	}
	return nil
}
