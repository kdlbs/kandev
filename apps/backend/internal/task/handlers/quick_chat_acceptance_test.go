package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type quickChatLaunchCall struct {
	ctx context.Context
	req *orchestrator.LaunchSessionRequest
}

type recordingQuickChatOrchestrator struct {
	calls            chan quickChatLaunchCall
	prepare          func(context.Context, *orchestrator.LaunchSessionRequest)
	prepareErr       error
	startErr         error
	startCompleted   chan error
	startRelease     <-chan struct{}
	startStarted     chan struct{}
	startStartedOnce sync.Once
}

func (o *recordingQuickChatOrchestrator) LaunchSession(
	ctx context.Context,
	req *orchestrator.LaunchSessionRequest,
) (*orchestrator.LaunchSessionResponse, error) {
	requestCopy := *req
	requestCopy.Attachments = append([]v1.MessageAttachment(nil), req.Attachments...)
	o.calls <- quickChatLaunchCall{ctx: ctx, req: &requestCopy}
	if req.Intent == orchestrator.IntentPrepare {
		if o.prepare != nil {
			o.prepare(ctx, req)
		}
		if o.prepareErr != nil {
			return nil, o.prepareErr
		}
		return &orchestrator.LaunchSessionResponse{SessionID: "session-1"}, nil
	}
	if req.Intent == orchestrator.IntentStartCreated {
		return o.launchCreatedSession(req)
	}
	return &orchestrator.LaunchSessionResponse{Success: true, SessionID: "session-1", AgentProfileID: req.AgentProfileID}, nil
}

func (o *recordingQuickChatOrchestrator) launchCreatedSession(
	req *orchestrator.LaunchSessionRequest,
) (*orchestrator.LaunchSessionResponse, error) {
	o.signalCreatedStart()
	if o.startRelease != nil {
		<-o.startRelease
	}
	if o.startErr != nil {
		o.completeCreatedStart(o.startErr)
		return nil, o.startErr
	}
	o.completeCreatedStart(nil)
	return &orchestrator.LaunchSessionResponse{
		Success: true, SessionID: req.SessionID, AgentProfileID: req.AgentProfileID,
	}, nil
}

func (o *recordingQuickChatOrchestrator) signalCreatedStart() {
	if o.startStarted != nil {
		o.startStartedOnce.Do(func() { close(o.startStarted) })
	}
}

func (o *recordingQuickChatOrchestrator) completeCreatedStart(err error) {
	if o.startCompleted != nil {
		o.startCompleted <- err
	}
}

func (*recordingQuickChatOrchestrator) EnsureSession(
	context.Context,
	string,
	...orchestrator.EnsureSessionOptions,
) (*orchestrator.EnsureSessionResponse, error) {
	return nil, nil
}

func newRecordingQuickChatOrchestrator() *recordingQuickChatOrchestrator {
	return &recordingQuickChatOrchestrator{calls: make(chan quickChatLaunchCall, 2)}
}

type gatedQuickChatOrchestrator struct {
	mu           sync.Mutex
	requests     []*orchestrator.LaunchSessionRequest
	startSeen    chan orchestrator.SessionIntent
	startRelease <-chan struct{}
	startOnce    sync.Once
}

func (o *gatedQuickChatOrchestrator) LaunchSession(
	_ context.Context,
	req *orchestrator.LaunchSessionRequest,
) (*orchestrator.LaunchSessionResponse, error) {
	o.mu.Lock()
	o.requests = append(o.requests, req)
	o.mu.Unlock()

	if req.Intent == orchestrator.IntentStart || req.Intent == orchestrator.IntentStartCreated {
		o.startOnce.Do(func() { o.startSeen <- req.Intent })
		<-o.startRelease
	}
	return &orchestrator.LaunchSessionResponse{SessionID: "session-1"}, nil
}

func (*gatedQuickChatOrchestrator) EnsureSession(
	context.Context,
	string,
	...orchestrator.EnsureSessionOptions,
) (*orchestrator.EnsureSessionResponse, error) {
	return nil, nil
}

func TestQuickChatAcceptance_PreparesBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		name              string
		body              string
		wantPrompt        string
		wantInitialPrompt bool
		wantInlineCount   int
	}{
		{
			name:              "structured opening is delivered after the accepted response",
			body:              `{"agent_profile_id":"profile-1"}`,
			wantInitialPrompt: true,
		},
		{
			name:            "legacy explicit HTTP prompt remains backend-owned",
			body:            `{"agent_profile_id":"profile-1","prompt":"Review this trace","attachments":[{"type":"resource","data":"dGVzdA==","mime_type":"text/plain","name":"trace.txt","delivery_mode":"path"}]}`,
			wantPrompt:      "Review this trace",
			wantInlineCount: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			h, _, _ := newQuickChatHandlerForTest(t)
			orch := newRecordingQuickChatOrchestrator()
			h.orchestrator = orch
			c, rec := quickChatRequestContext(t, "/workspaces/ws-1/quick-chat", tc.body)

			h.httpStartQuickChat(c)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var prepare quickChatLaunchCall
			select {
			case prepare = <-orch.calls:
			case <-time.After(time.Second):
				t.Fatal("Quick Chat did not prepare its retained session")
			}
			require.Equal(t, orchestrator.IntentPrepare, prepare.req.Intent)
			require.True(t, prepare.req.DeferredStart)
			require.False(t, prepare.req.LaunchWorkspace)
			if tc.wantPrompt == "" {
				require.Nil(t, prepare.req.InitialPromptSubmission)
			} else {
				require.NotNil(t, prepare.req.InitialPromptSubmission)
				require.Equal(t, tc.wantPrompt, prepare.req.InitialPromptSubmission.Content)
				require.Equal(t, tc.wantInlineCount, prepare.req.InitialPromptSubmission.InlineAttachmentCount)
				require.Equal(t, models.InitialPromptSubmissionPending, prepare.req.InitialPromptSubmission.State)
			}

			var start quickChatLaunchCall
			select {
			case start = <-orch.calls:
			case <-time.After(time.Second):
				t.Fatal("Quick Chat did not dispatch its deferred start")
			}
			require.Equal(t, orchestrator.IntentStartCreated, start.req.Intent)
			require.Equal(t, "session-1", start.req.SessionID)
			require.Equal(t, tc.wantPrompt, start.req.Prompt)
			require.Equal(t, tc.wantInitialPrompt, start.req.NoInitialPrompt)
			require.True(t, start.req.QuickChatInitialStart)
			require.Equal(t, prepare.req.Attachments, start.req.Attachments)
		})
	}
}

func TestQuickChatAcceptance_DispatchUsesDetachedBoundedContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	identity := authn.Identity{UserID: "user-1", Role: authn.RoleMember}
	requestCtx, cancel := context.WithCancel(authn.WithIdentity(context.Background(), identity))
	defer cancel()
	h, _, _ := newQuickChatHandlerForTest(t)
	orch := newRecordingQuickChatOrchestrator()
	startRelease := make(chan struct{})
	defer close(startRelease)
	orch.startRelease = startRelease
	orch.prepare = func(_ context.Context, req *orchestrator.LaunchSessionRequest) {
		if req.Intent == orchestrator.IntentPrepare {
			cancel()
		}
	}
	h.orchestrator = orch
	c, rec := quickChatRequestContext(t, "/workspaces/ws-1/quick-chat", `{"agent_profile_id":"profile-1"}`)
	c.Request = c.Request.WithContext(requestCtx)

	h.httpStartQuickChat(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var prepare quickChatLaunchCall
	select {
	case prepare = <-orch.calls:
	case <-time.After(time.Second):
		t.Fatal("Quick Chat did not prepare its retained session")
	}
	require.Equal(t, orchestrator.IntentPrepare, prepare.req.Intent)
	var start quickChatLaunchCall
	select {
	case start = <-orch.calls:
	case <-time.After(time.Second):
		t.Fatal("Quick Chat did not dispatch after request cancellation")
	}
	require.Equal(t, orchestrator.IntentStartCreated, start.req.Intent)
	require.NoError(t, start.ctx.Err())
	deadline, hasDeadline := start.ctx.Deadline()
	require.True(t, hasDeadline, "background startup must have a finite budget")
	require.True(t, time.Until(deadline) > 0)
	require.LessOrEqual(t, time.Until(deadline), constants.AgentLaunchTimeout)
	gotIdentity, ok := authn.IdentityFromContext(start.ctx)
	require.True(t, ok)
	require.Equal(t, identity, gotIdentity)
}

func TestQuickChatAcceptance_InterruptedTerminalDispatchKeepsAcceptedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _, _ := newQuickChatHandlerForTest(t)
	orch := newRecordingQuickChatOrchestrator()
	orch.startErr = errors.New("terminal dispatch interrupted")
	orch.startCompleted = make(chan error, 1)
	h.orchestrator = orch
	c, rec := quickChatRequestContext(t, "/workspaces/ws-1/quick-chat", `{"agent_profile_id":"profile-1","prompt":"Review this trace","attachments":[{"type":"resource","data":"dGVzdA==","mime_type":"text/plain","name":"trace.txt","delivery_mode":"path"}]}`)

	h.httpStartQuickChat(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var accepted map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &accepted))
	require.NotEmpty(t, accepted["task_id"])
	require.Equal(t, "session-1", accepted["session_id"])
	var prepare quickChatLaunchCall
	select {
	case prepare = <-orch.calls:
	case <-time.After(time.Second):
		t.Fatal("Quick Chat did not persist its terminal opening before acceptance")
	}
	require.NotNil(t, prepare.req.InitialPromptSubmission)
	require.Equal(t, "Review this trace", prepare.req.InitialPromptSubmission.Content)
	require.Equal(t, 1, prepare.req.InitialPromptSubmission.InlineAttachmentCount)
	var start quickChatLaunchCall
	select {
	case start = <-orch.calls:
	case <-time.After(time.Second):
		t.Fatal("Quick Chat did not dispatch the terminal opening")
	}
	require.Equal(t, orchestrator.IntentStartCreated, start.req.Intent)
	require.Equal(t, "Review this trace", start.req.Prompt)
	require.Equal(t, []v1.MessageAttachment{openingAttachment}, start.req.Attachments)
	require.ErrorIs(t, <-orch.startCompleted, orch.startErr)
}

func TestQuickChatAcceptance_ReturnsBeforePreparation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	startSeen := make(chan orchestrator.SessionIntent, 1)
	startRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseStart := func() { releaseOnce.Do(func() { close(startRelease) }) }
	defer releaseStart()

	h, _, _ := newQuickChatHandlerForTest(t)
	h.orchestrator = &gatedQuickChatOrchestrator{
		startSeen:    startSeen,
		startRelease: startRelease,
	}
	router := gin.New()
	router.POST("/workspaces/:id/quick-chat", h.httpStartQuickChat)

	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 150 * time.Millisecond
	server.Start()
	t.Cleanup(server.Close)

	type responseResult struct {
		status int
		body   []byte
		err    error
	}
	responseCh := make(chan responseResult, 1)
	go func() {
		request, err := http.NewRequest(http.MethodPost,
			server.URL+"/workspaces/ws-1/quick-chat",
			strings.NewReader(`{"agent_profile_id":"profile-1"}`),
		)
		if err != nil {
			responseCh <- responseResult{err: err}
			return
		}
		request.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Do(request)
		if err != nil {
			responseCh <- responseResult{err: err}
			return
		}
		body, err := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		err = errors.Join(err, closeErr)
		responseCh <- responseResult{status: response.StatusCode, body: body, err: err}
	}()

	select {
	case <-startSeen:
	case <-time.After(time.Second):
		t.Fatal("Quick Chat startup did not reach the gated launch")
	}

	var response responseResult
	select {
	case response = <-responseCh:
	case <-time.After(time.Second):
		t.Fatal("Quick Chat response waited for the gated launch to finish")
	}
	require.NoError(t, response.err)
	require.Equal(t, http.StatusOK, response.status, string(response.body))

	var payload map[string]string
	require.NoError(t, json.Unmarshal(response.body, &payload))
	require.NotEmpty(t, payload["task_id"])
	require.Equal(t, "session-1", payload["session_id"])
	select {
	case <-startRelease:
		t.Fatal("startup gate was released before the HTTP response completed")
	default:
	}
}
