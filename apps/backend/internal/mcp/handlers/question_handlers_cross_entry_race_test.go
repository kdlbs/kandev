package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/clarification"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// noopBroadcaster is a clarification.Broadcaster stub; this race test only
// cares about the claim outcome, not WS fan-out.
type noopBroadcaster struct{}

func (noopBroadcaster) BroadcastToSession(string, *ws.Message) {}

// TestAnswerQuestion_RESTvsMCP_OnlyOneClaims proves R3: the REST
// /api/v1/clarification/:id/respond endpoint and answer_question_kandev share
// Resolver.ResolveBundle as their single claim path (P1), so racing the two
// real entry points against the same pending_id still yields exactly one
// winner and a consistent answer for the loser. REST-vs-REST or MCP-vs-MCP
// would only prove one entry point is safe against itself; it would leave the
// other call site's wiring into the shared resolver entirely unexercised.
//
// Both sides are driven through their real, unexported handler code -- REST
// via a genuine gin router built with clarification.RegisterRoutes, MCP via
// handleAnswerQuestion in this package -- rather than by calling
// Resolver.ResolveBundle directly twice, which is exactly the shared
// plumbing this test needs to exercise from both call sites.
func TestAnswerQuestion_RESTvsMCP_OnlyOneClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	seedBundle(t, ctx, svc, repo, "pending-race-1")

	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)

	engine := gin.New()
	clarification.RegisterRoutes(engine, store, noopBroadcaster{}, &svcMessageUpdater{Service: svc}, repo, stubDetachedResumer{}, resolver, testLogger(t))

	mcpH := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	// restAnswers and mcpAnswers must be distinguishable, not the same
	// payload submitted twice: with identical answers, require.JSONEq below
	// would pass even if a bug handed the loser its own submitted payload
	// back instead of the winner's, since both payloads are byte-identical
	// either way. Distinct answers make "the loser gets the winner's
	// response" an assertion that can actually fail.
	restAnswers := []map[string]interface{}{
		{"question_id": "q1", "selected_options": []string{"opt-a"}},
		{"question_id": "q2", "selected_options": []string{"opt-c"}},
	}
	mcpAnswers := []map[string]interface{}{
		{"question_id": "q1", "selected_options": []string{"opt-b"}},
		{"question_id": "q2", "selected_options": []string{"opt-d"}},
	}

	var wg sync.WaitGroup
	start := make(chan struct{})

	var restStatus int
	var restBody []byte
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		payload, err := json.Marshal(map[string]interface{}{"answers": restAnswers})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/clarification/pending-race-1/respond", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		restStatus = rec.Code
		restBody = rec.Body.Bytes()
	}()

	var mcpResp *ws.Message
	var mcpErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		msg := makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
			"pending_id": "pending-race-1",
			"answers":    mcpAnswers,
		})
		mcpResp, mcpErr = mcpH.handleAnswerQuestion(ctx, msg)
	}()

	close(start)
	wg.Wait()

	require.Equal(t, http.StatusOK, restStatus, "REST response: %s", restBody)
	require.NoError(t, mcpErr)
	require.Equal(t, ws.MessageTypeResponse, mcpResp.Type, "MCP response: %s", mcpResp.Payload)

	var restResult struct {
		Claimed  bool            `json:"claimed"`
		Response json.RawMessage `json:"response"`
	}
	require.NoError(t, json.Unmarshal(restBody, &restResult))

	var mcpResult answerQuestionResponse
	require.NoError(t, json.Unmarshal(mcpResp.Payload, &mcpResult))

	require.NotEqual(t, restResult.Claimed, mcpResult.Claimed,
		"exactly one entry point must claim the bundle; got REST claimed=%v, MCP claimed=%v", restResult.Claimed, mcpResult.Claimed)

	// R2/N4: the loser is handed the winner's normalized response verbatim,
	// not its own -- both entry points must agree on the final answer.
	require.JSONEq(t, string(restResult.Response), string(mcpResult.Response))

	// Because restAnswers and mcpAnswers are distinguishable, the shared
	// response must match whichever side actually claimed, not either
	// side's own submission by coincidence.
	winnerAnswers := mcpAnswers
	if restResult.Claimed {
		winnerAnswers = restAnswers
	}
	var parsed struct {
		Answers []struct {
			QuestionID      string   `json:"question_id"`
			SelectedOptions []string `json:"selected_options"`
		} `json:"answers"`
	}
	require.NoError(t, json.Unmarshal(restResult.Response, &parsed))
	require.Len(t, parsed.Answers, len(winnerAnswers))
	for i, got := range parsed.Answers {
		want := winnerAnswers[i]
		require.Equal(t, want["question_id"], got.QuestionID)
		require.Equal(t, want["selected_options"], got.SelectedOptions)
	}
}
