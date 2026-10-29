package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSandboxApprovalChannelBindsOneOperatorDecision(t *testing.T) {
	service, attempt, file := approvalStoreFixture(t, sqlitePolicyDB(t))
	normalize := func(_ context.Context, _ *Attempt, body []byte) (ApprovalOperation, error) {
		var operation ApprovalOperation
		err := json.Unmarshal(body, &operation)
		return operation, err
	}
	channels := NewApprovalChannels(service, normalize, func(context.Context, *Attempt) error { return nil })
	t.Cleanup(func() { require.NoError(t, channels.Close()) })
	channel, err := channels.Open(t.Context(), "org", attempt.ID, nil, ApprovalReplies{Allow: `{"status":"granted"}`, Deny: `{"status":"denied"}`, Timeout: `{"status":"timeout"}`})
	require.NoError(t, err)
	parsed, err := url.Parse(channel.URL())
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", parsed.Hostname())
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	require.NotZero(t, port)
	require.GreaterOrEqual(t, len(parsed.Path), 43)
	client := &http.Client{Timeout: 3 * time.Second}
	request := ApprovalOperation{RequestID: "one", ProviderSessionID: "provider-session", ChildPID: 101, Access: "read", Target: file}
	body, err := json.Marshal(request)
	require.NoError(t, err)
	results := make(chan string, 1)
	go func() {
		response, requestErr := client.Post(channel.URL(), "application/json", bytes.NewReader(body))
		if requestErr != nil {
			results <- "request failed"
			return
		}
		defer func() { _ = response.Body.Close() }()
		content, _ := io.ReadAll(response.Body)
		results <- string(content)
	}()
	var pending *Approval
	require.Eventually(t, func() bool {
		pending, err = service.ReadApproval(t.Context(), "org", attempt.ID, "one")
		return err == nil
	}, time.Second, 10*time.Millisecond)
	duplicate, err := client.Post(channel.URL(), "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, duplicate.StatusCode)
	require.NoError(t, duplicate.Body.Close())
	_, err = service.DecideApproval(operatorContext(t), attempt.ID, "one", pending.Version, "allow_once")
	require.NoError(t, err)
	select {
	case result := <-results:
		require.Equal(t, `{"status":"granted"}`, result)
	case <-time.After(3 * time.Second):
		t.Fatal("operator response was not delivered")
	}
	require.Eventually(t, func() bool {
		a, e := service.ReadApproval(t.Context(), "org", attempt.ID, "one")
		return e == nil && a.State == ApprovalSent
	}, time.Second, 10*time.Millisecond)
	replay, err := client.Post(channel.URL(), "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	require.NotEqual(t, http.StatusOK, replay.StatusCode)
	require.NoError(t, replay.Body.Close())
}

func TestSandboxApprovalChannelSupportsUnassignedOrganization(t *testing.T) {
	service, attempt, _ := approvalStoreFixture(t, sqlitePolicyDB(t))
	channels := NewApprovalChannels(service, func(context.Context, *Attempt, []byte) (ApprovalOperation, error) {
		return ApprovalOperation{}, ErrConflict
	}, func(context.Context, *Attempt) error { return nil })
	t.Cleanup(func() { require.NoError(t, channels.Close()) })
	replies := ApprovalReplies{Allow: `{}`, Deny: `{}`, Timeout: `{}`}
	unassigned, err := channels.Open(t.Context(), "", attempt.ID, nil, replies)
	require.NoError(t, err)
	named, err := channels.Open(t.Context(), "org", attempt.ID, nil, replies)
	require.NoError(t, err)
	require.NotEqual(t, unassigned.URL(), named.URL())
	_, err = channels.Open(t.Context(), "", attempt.ID, nil, replies)
	require.ErrorIs(t, err, ErrConflict, "the same authority cannot open duplicate channels")
}

func TestSandboxApprovalChannelRejectsUnboundAndOversizedRequests(t *testing.T) {
	service, attempt, _ := approvalStoreFixture(t, sqlitePolicyDB(t))
	channels := NewApprovalChannels(service, func(context.Context, *Attempt, []byte) (ApprovalOperation, error) {
		return ApprovalOperation{}, ErrConflict
	}, func(context.Context, *Attempt) error { return nil })
	t.Cleanup(func() { require.NoError(t, channels.Close()) })
	channel, err := channels.Open(t.Context(), "org", attempt.ID, nil, ApprovalReplies{Allow: `{}`, Deny: `{}`, Timeout: `{}`})
	require.NoError(t, err)
	client := &http.Client{Timeout: time.Second}
	for _, test := range []struct {
		method, target, body string
		status               int
	}{
		{"GET", channel.URL(), "", http.StatusMethodNotAllowed},
		{"POST", channel.URL() + "?leak=1", "{}", http.StatusNotFound},
		{"POST", channel.URL() + "wrong", "{}", http.StatusNotFound},
		{"POST", channel.URL(), strings.Repeat("x", (64<<10)+1), http.StatusRequestEntityTooLarge},
		{"POST", channel.URL(), "{}", http.StatusForbidden},
	} {
		req, e := http.NewRequest(test.method, test.target, strings.NewReader(test.body))
		require.NoError(t, e)
		response, e := client.Do(req)
		require.NoError(t, e)
		require.Equal(t, test.status, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	require.NoError(t, channel.Close())
	_, err = client.Post(channel.URL(), "application/json", strings.NewReader("{}"))
	require.Error(t, err, "closed callbacks cannot accept decisions")
}

func TestSandboxApprovalChannelCancellationRefusesLateDecision(t *testing.T) {
	service, attempt, file := approvalStoreFixture(t, sqlitePolicyDB(t))
	channels := NewApprovalChannels(service, func(_ context.Context, _ *Attempt, raw []byte) (ApprovalOperation, error) {
		var op ApprovalOperation
		err := json.Unmarshal(raw, &op)
		return op, err
	}, func(context.Context, *Attempt) error { return nil })
	t.Cleanup(func() { require.NoError(t, channels.Close()) })
	channel, err := channels.Open(t.Context(), "org", attempt.ID, nil, ApprovalReplies{Allow: `{"status":"granted"}`, Deny: `{"status":"denied"}`, Timeout: `{"status":"timeout"}`})
	require.NoError(t, err)
	body, err := json.Marshal(ApprovalOperation{RequestID: "cancelled", ProviderSessionID: "provider-session", ChildPID: 101, Access: "read", Target: file})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, channel.URL(), bytes.NewReader(body))
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		response, e := http.DefaultClient.Do(request)
		if e == nil {
			_ = response.Body.Close()
		}
	}()
	var pending *Approval
	require.Eventually(t, func() bool {
		pending, err = service.ReadApproval(t.Context(), "org", attempt.ID, "cancelled")
		return err == nil
	}, time.Second, 10*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback did not stop")
	}
	require.Eventually(t, func() bool {
		a, e := service.ReadApproval(t.Context(), "org", attempt.ID, "cancelled")
		return e == nil && a.State == ApprovalUndelivered
	}, time.Second, 10*time.Millisecond)
	_, err = service.DecideApproval(operatorContext(t), attempt.ID, "cancelled", pending.Version, "allow_once")
	require.ErrorIs(t, err, ErrConflict)
}
