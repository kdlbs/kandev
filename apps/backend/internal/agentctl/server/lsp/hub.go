package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

var (
	ErrRuntimeNotReady  = errors.New("task-host LSP generation is not ready for attachment")
	ErrAttachmentClosed = errors.New("task-host LSP attachment is closed")
)

type featureUpstream interface {
	Request(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error)
	Notify(method string, params json.RawMessage) error
}

type hub struct {
	language   string
	generation uint64
	upstream   featureUpstream
	documents  *documentBroker

	mu          sync.RWMutex
	nextID      uint64
	attachments map[uint64]*Attachment
	closed      bool
}

func newHub(language string, generation uint64, upstream featureUpstream) *hub {
	return &hub{
		language:    language,
		generation:  generation,
		upstream:    upstream,
		documents:   newDocumentBroker(upstream),
		attachments: make(map[uint64]*Attachment),
	}
}

func (h *hub) Attach(snapshot Snapshot) *Attachment {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		attachment := newAttachment(0, h)
		attachment.start(nil)
		attachment.Close()
		return attachment
	}
	h.nextID++
	attachment := newAttachment(h.nextID, h)
	h.documents.SetCapabilities(snapshot.Capabilities)
	replay := make([][]byte, 0, len(snapshot.Diagnostics)+1)
	replay = append(replay, attachmentHandshake(snapshot))
	for _, diagnostic := range snapshot.Diagnostics {
		replay = append(replay, serverNotification("textDocument/publishDiagnostics", diagnostic))
	}
	attachment.start(replay)
	// Make the attachment visible to fanout only after its generation-scoped
	// handshake and replay are staged. This is the attachment's publication
	// barrier: live notifications can never overtake recovery evidence.
	h.attachments[attachment.id] = attachment
	h.mu.Unlock()
	return attachment
}

func (h *hub) Broadcast(method string, params json.RawMessage) {
	message := serverNotification(method, params)
	h.mu.RLock()
	attachments := make([]*Attachment, 0, len(h.attachments))
	for _, attachment := range h.attachments {
		attachments = append(attachments, attachment)
	}
	h.mu.RUnlock()
	for _, attachment := range attachments {
		if !attachment.enqueue(message) {
			attachment.fail()
		}
	}
}

func (h *hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	attachments := make([]*Attachment, 0, len(h.attachments))
	for id, attachment := range h.attachments {
		attachments = append(attachments, attachment)
		delete(h.attachments, id)
	}
	h.mu.Unlock()
	for _, attachment := range attachments {
		attachment.close(false)
	}
}

func (h *hub) Closed() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.closed
}

func (h *hub) detach(id uint64) {
	h.mu.Lock()
	delete(h.attachments, id)
	h.mu.Unlock()
}

func attachmentHandshake(snapshot Snapshot) []byte {
	message, _ := json.Marshal(map[string]any{
		"status":              "attached",
		"language":            snapshot.Language,
		"generation":          snapshot.Generation,
		"workspaceUri":        snapshot.WorkspaceURI,
		fieldWorkspaceFolders: snapshot.WorkspaceFolders,
		"serverCapabilities":  snapshot.Capabilities,
	})
	return message
}

func serverNotification(method string, params json.RawMessage) []byte {
	message, _ := json.Marshal(rpcMessage{
		JSONRPC: rpcVersion,
		Method:  method,
		Params:  params,
	})
	return message
}
