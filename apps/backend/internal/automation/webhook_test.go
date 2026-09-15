package automation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newWebhookTestHandler(t *testing.T) (*WebhookHandler, *Service) {
	t.Helper()
	svc := newTestService(t)
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	return NewWebhookHandler(svc, log), svc
}

func createWebhookAutomation(t *testing.T, svc *Service, cfg string) (*Automation, *AutomationTrigger) {
	t.Helper()
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-1", Name: "alert ingest", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	trig := &AutomationTrigger{
		AutomationID: a.ID, Type: TriggerTypeWebhook, Enabled: true,
		Config: json.RawMessage(cfg),
	}
	require.NoError(t, svc.store.CreateTrigger(ctx, trig))
	got, err := svc.store.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	return got, trig
}

func doWebhookPost(h *WebhookHandler, automationID, secret, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/automations/webhook/"+automationID, strings.NewReader(body))
	if secret != "" {
		c.Request.Header.Set("X-Webhook-Secret", secret)
	}
	c.Params = gin.Params{{Key: "id", Value: automationID}}
	h.Handle(c)
	return w
}

// A well-formed POST always answers 200, whether the trigger fired, was
// filtered, or was deduplicated (S7) — a non-2xx would make the sender
// retry, and a varying status would give a secret-holder an outcome oracle.
func TestWebhookHandle_AlwaysReturns200OnFire(t *testing.T) {
	h, svc := newWebhookTestHandler(t)
	a, _ := createWebhookAutomation(t, svc, `{}`)

	w := doWebhookPost(h, a.ID, a.WebhookSecret, `{"event":"deploy"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"status":"triggered"}`, w.Body.String())
}

func TestWebhookHandle_RejectsWrongSecret(t *testing.T) {
	h, svc := newWebhookTestHandler(t)
	a, _ := createWebhookAutomation(t, svc, `{}`)

	w := doWebhookPost(h, a.ID, "wrong-secret", `{}`)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// A filtered POST still returns 200, creates no task-eligible run, and
// records exactly one skipped run naming the rejecting predicate's index,
// with an empty dedup key.
func TestWebhookHandle_FilteredTrigger_Records200AndSkipRun(t *testing.T) {
	h, svc := newWebhookTestHandler(t)
	a, trig := createWebhookAutomation(t, svc, `{"dedup_key":"id","filters":[{"path":"severity","op":"eq","values":["critical"]}]}`)

	w := doWebhookPost(h, a.ID, a.WebhookSecret, `{"id":"x1","severity":"warning"}`)
	require.Equal(t, http.StatusOK, w.Code)

	runs, err := svc.store.ListRuns(context.Background(), a.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, RunStatusSkipped, runs[0].Status)
	require.Equal(t, "filter_rejected: 0", runs[0].ErrorMessage)
	require.Empty(t, runs[0].DedupKey)
	require.Equal(t, trig.ID, runs[0].TriggerID)

	active, err := svc.store.CountActiveRuns(context.Background(), a.ID)
	require.NoError(t, err)
	require.Equal(t, 0, active)
}

// A POST passing all filters fires and admits a run with the resolved,
// namespaced dedup key.
func TestWebhookHandle_PassingFilters_AdmitsRun(t *testing.T) {
	h, svc := newWebhookTestHandler(t)
	a, _ := createWebhookAutomation(t, svc, `{"dedup_key":"id","filters":[{"path":"severity","op":"eq","values":["critical"]}]}`)

	w := doWebhookPost(h, a.ID, a.WebhookSecret, `{"id":"x1","severity":"critical"}`)
	require.Equal(t, http.StatusOK, w.Code)

	runs, err := svc.store.ListRuns(context.Background(), a.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, RunStatusTriggered, runs[0].Status)
	require.Equal(t, "webhook:x1", runs[0].DedupKey)
}

// A trigger with no dedup_key path declared admits with an empty key.
func TestWebhookHandle_NoDedupKeyConfigured_AdmitsWithEmptyKey(t *testing.T) {
	h, svc := newWebhookTestHandler(t)
	a, _ := createWebhookAutomation(t, svc, `{}`)

	w := doWebhookPost(h, a.ID, a.WebhookSecret, `{"id":"x1"}`)
	require.Equal(t, http.StatusOK, w.Code)

	runs, err := svc.store.ListRuns(context.Background(), a.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Empty(t, runs[0].DedupKey)
	require.Equal(t, "dedup_not_configured", runs[0].DedupReason)
}

// A declared dedup_key path that resolves to an empty/whitespace value is
// treated the same as unresolved — the literal key "webhook:   " must never
// be stored.
func TestWebhookHandle_DedupKeyResolvesToBlank_TreatedAsUnresolved(t *testing.T) {
	h, svc := newWebhookTestHandler(t)
	a, _ := createWebhookAutomation(t, svc, `{"dedup_key":"id"}`)

	w := doWebhookPost(h, a.ID, a.WebhookSecret, `{"id":"   "}`)
	require.Equal(t, http.StatusOK, w.Code)

	runs, err := svc.store.ListRuns(context.Background(), a.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Empty(t, runs[0].DedupKey)
	require.Equal(t, "dedup_unresolved", runs[0].DedupReason)
}
