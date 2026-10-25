package sandbox

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"time"
)

func (c *ApprovalChannel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if subtle.ConstantTimeCompare([]byte(r.URL.EscapedPath()), []byte(c.path)) != 1 || r.URL.RawQuery != "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	approval, err := c.accept(ctx, body)
	if err != nil {
		w.WriteHeader(approvalChannelStatus(err))
		return
	}
	defer c.release(approval.Snapshot.Operation.RequestID)
	c.waitForDecision(ctx, w, approval)
}

func approvalChannelStatus(err error) int {
	if err == ErrConflict {
		return http.StatusConflict
	}
	return http.StatusForbidden
}

func (c *ApprovalChannel) accept(ctx context.Context, body []byte) (*Approval, error) {
	attempt, err := c.owner.service.ReadAttempt(ctx, c.key.org, c.key.attempt)
	if err != nil {
		return nil, ErrAuthority
	}
	operation, err := c.owner.normalize(ctx, attempt, body)
	if err != nil {
		return nil, ErrAuthority
	}
	if c.owner.live(ctx, attempt) != nil {
		return nil, ErrAuthority
	}
	c.mu.Lock()
	if c.closed || c.ctx.Err() != nil || len(c.pending) >= 8 || c.pending[operation.RequestID] != nil {
		c.mu.Unlock()
		return nil, ErrConflict
	}
	c.pending[operation.RequestID] = ctx
	c.mu.Unlock()
	digest := sha256.Sum256(body)
	deadline, _ := ctx.Deadline()
	approval, err := c.owner.service.RecordApproval(ctx, c.key.org, c.key.attempt, operation, hex.EncodeToString(digest[:]), deadline)
	if err != nil || approval.State != ApprovalPending {
		c.release(operation.RequestID)
		return nil, ErrConflict
	}
	return approval, nil
}

func (c *ApprovalChannel) release(requestID string) {
	c.mu.Lock()
	delete(c.pending, requestID)
	c.mu.Unlock()
}

func (c *ApprovalChannel) waitForDecision(ctx context.Context, w http.ResponseWriter, approval *Approval) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			c.abandon(approval)
			_, _ = io.WriteString(w, c.replies.Timeout)
			return
		case <-ticker.C:
			latest, err := c.owner.service.ReadApproval(ctx, c.key.org, c.key.attempt, approval.Snapshot.Operation.RequestID)
			if err != nil {
				c.abandon(approval)
				_, _ = io.WriteString(w, c.replies.Deny)
				return
			}
			switch latest.State {
			case ApprovalApproved:
				c.deliver(ctx, w, latest)
				return
			case ApprovalPending:
				continue
			default:
				_, _ = io.WriteString(w, c.replies.Deny)
				return
			}
		}
	}
}

func (c *ApprovalChannel) deliver(ctx context.Context, w http.ResponseWriter, approval *Approval) {
	claimed, err := c.owner.service.ClaimApproval(ctx, c.key.org, c.key.attempt, approval.Snapshot.Operation.RequestID, approval.Version)
	if err != nil {
		c.abandon(approval)
		_, _ = io.WriteString(w, c.replies.Deny)
		return
	}
	c.mu.Lock()
	sent := false
	if !c.closed && c.ctx.Err() == nil && ctx.Err() == nil {
		n, writeErr := io.WriteString(w, c.replies.Allow)
		sent = writeErr == nil && n == len(c.replies.Allow)
		if sent {
			sent = http.NewResponseController(w).Flush() == nil
		}
	}
	c.mu.Unlock()
	finish, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = c.owner.service.FinishApproval(finish, c.key.org, c.key.attempt, approval.Snapshot.Operation.RequestID, claimed.Version, sent)
}

func (c *ApprovalChannel) abandon(approval *Approval) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.owner.service.abandonApproval(ctx, c.key.org, c.key.attempt, approval.Snapshot.Operation.RequestID)
}
