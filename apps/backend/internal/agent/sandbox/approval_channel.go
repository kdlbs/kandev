package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ApprovalReplies contains the provider's bounded, static response protocol.
type ApprovalReplies struct{ Allow, Deny, Timeout string }
type approvalChannelKey struct{ org, attempt string }
type ApprovalNormalizer func(context.Context, *Attempt, []byte) (ApprovalOperation, error)
type ApprovalLiveCheck func(context.Context, *Attempt) error

type ApprovalChannels struct {
	service   *Service
	normalize ApprovalNormalizer
	live      ApprovalLiveCheck
	mu        sync.Mutex
	channels  map[approvalChannelKey]*ApprovalChannel
	closed    bool
}

type ApprovalChannel struct {
	owner     *ApprovalChannels
	key       approvalChannelKey
	replies   ApprovalReplies
	url       string
	path      string
	server    *http.Server
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	pending   map[string]context.Context
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func NewApprovalChannels(service *Service, normalize ApprovalNormalizer, live ApprovalLiveCheck) *ApprovalChannels {
	channels := &ApprovalChannels{service: service, normalize: normalize, live: live, channels: map[approvalChannelKey]*ApprovalChannel{}}
	service.SetApprovalGuard(channels.checkLive)
	return channels
}

// Open reserves a private callback before the immutable launch is materialized.
// Its URL must be written only to a protected runtime artifact.
func (b *ApprovalChannels) Open(ctx context.Context, orgID, attemptID string, excludedPorts []int, replies ApprovalReplies) (*ApprovalChannel, error) {
	if ctx.Err() != nil || orgID == "" || attemptID == "" || b.normalize == nil || b.live == nil || !validApprovalReplies(replies) {
		return nil, ErrConflict
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := approvalChannelKey{orgID, attemptID}
	if b.closed || b.channels[key] != nil {
		return nil, ErrConflict
	}
	listener, err := privateApprovalListener(excludedPorts)
	if err != nil {
		return nil, err
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		_ = listener.Close()
		return nil, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	channel := &ApprovalChannel{owner: b, key: key, replies: replies, path: "/" + base64.RawURLEncoding.EncodeToString(token), ctx: lifetime, cancel: cancel, pending: map[string]context.Context{}}
	channel.url = "http://" + listener.Addr().String() + channel.path
	channel.server = &http.Server{Handler: channel, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 55 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 4096, ErrorLog: log.New(io.Discard, "", 0)}
	channel.server.SetKeepAlivesEnabled(false)
	b.channels[key] = channel
	go func() { _ = channel.server.Serve(listener); cancel() }()
	go func() { <-lifetime.Done(); _ = channel.Close() }()
	return channel, nil
}

func validApprovalReplies(replies ApprovalReplies) bool {
	for _, body := range []string{replies.Allow, replies.Deny, replies.Timeout} {
		if len(body) == 0 || len(body) > 4096 || !json.Valid([]byte(body)) {
			return false
		}
	}
	return true
}

func privateApprovalListener(excluded []int) (net.Listener, error) {
	ports := make(map[int]bool, len(excluded))
	for _, port := range excluded {
		ports[port] = true
	}
	for range 32 {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, ErrConflict
		}
		_, text, err := net.SplitHostPort(listener.Addr().String())
		port, parseErr := strconv.Atoi(text)
		if err == nil && parseErr == nil && !ports[port] {
			return listener, nil
		}
		_ = listener.Close()
	}
	return nil, ErrConflict
}

func (c *ApprovalChannel) URL() string { return c.url }

func (b *ApprovalChannels) checkLive(ctx context.Context, attempt *Attempt, approval *Approval) error {
	b.mu.Lock()
	channel := b.channels[approvalChannelKey{approval.Snapshot.OrgID, attempt.ID}]
	b.mu.Unlock()
	if channel == nil {
		return ErrConflict
	}
	channel.mu.Lock()
	request := channel.pending[approval.Snapshot.Operation.RequestID]
	valid := !channel.closed && channel.ctx.Err() == nil && request != nil && request.Err() == nil
	channel.mu.Unlock()
	if !valid {
		return ErrConflict
	}
	return b.live(ctx, attempt)
}

func (c *ApprovalChannel) Close() error {
	c.closeOnce.Do(func() {
		c.cancel()
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		c.closeErr = c.server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		c.closeErr = errors.Join(c.closeErr, c.owner.service.CloseApprovals(ctx, c.key.org, c.key.attempt))
		c.owner.mu.Lock()
		delete(c.owner.channels, c.key)
		c.owner.mu.Unlock()
	})
	return c.closeErr
}

func (b *ApprovalChannels) Close() error {
	b.mu.Lock()
	b.closed = true
	channels := make([]*ApprovalChannel, 0, len(b.channels))
	for _, channel := range b.channels {
		channels = append(channels, channel)
	}
	b.mu.Unlock()
	var err error
	for _, channel := range channels {
		err = errors.Join(err, channel.Close())
	}
	return err
}
