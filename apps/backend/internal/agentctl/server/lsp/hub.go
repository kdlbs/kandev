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
		attachment.Close()
		return attachment
	}
	h.nextID++
	attachment := newAttachment(h.nextID, h)
	h.attachments[attachment.id] = attachment
	h.mu.Unlock()
	h.documents.SetCapabilities(snapshot.Capabilities)
	attachment.enqueue(attachmentHandshake(snapshot))
	for _, diagnostic := range snapshot.Diagnostics {
		attachment.enqueue(serverNotification("textDocument/publishDiagnostics", diagnostic))
	}
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
