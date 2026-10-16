package automation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeWebhookTriggerDataUsesBoundedJSONPointers(t *testing.T) {
	data, err := safeWebhookTriggerData(
		[]byte(`{"pull_request":{"number":7,"title":"private title"},"token":"secret"}`),
		[]string{"/pull_request/number"}, "webhook-trigger", "delivery-7",
	)
	require.NoError(t, err)
	require.NotContains(t, string(data), "private title")
	require.NotContains(t, string(data), "secret")
	var projection map[string]any
	require.NoError(t, json.Unmarshal(data, &projection))
	require.Equal(t, "delivery-7", projection["delivery_id"])
	require.Equal(t, map[string]any{"/pull_request/number": float64(7)}, projection["payload"])
}

func TestSafeWebhookProjectionInterpolatesNestedPath(t *testing.T) {
	data, err := safeWebhookTriggerData(
		[]byte(`{"pull_request":{"number":7},"token":"secret"}`),
		[]string{"/pull_request/number"}, "webhook-trigger", "delivery-7",
	)
	require.NoError(t, err)
	require.Equal(t, "PR #7", InterpolatePrompt(
		"PR #{{webhook.pull_request.number}}", TriggerTypeWebhook, data,
	))
}

func TestSafeWebhookTriggerDataPreservesLegacyPayloadWithoutPointers(t *testing.T) {
	data, err := safeWebhookTriggerData(
		[]byte(`{"pull_request":{"number":7},"token":"legacy-token"}`),
		nil, "webhook-trigger", "delivery-7",
	)
	require.NoError(t, err)
	require.Equal(t, "PR #7", InterpolatePrompt(
		"PR #{{webhook.pull_request.number}}", TriggerTypeWebhook, data,
	))
	require.Contains(t, string(data), "legacy-token")
}

func TestResolvePayloadPathReadsLegacyAndProjectedWebhookValues(t *testing.T) {
	for _, test := range []struct {
		name     string
		pointers []string
	}{
		{name: "legacy body"},
		{name: "configured pointer", pointers: []string{"/service"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := safeWebhookTriggerData(
				[]byte(`{"service":"acme/unconfigured-webapp"}`),
				test.pointers, "webhook-trigger", "delivery-7",
			)
			require.NoError(t, err)

			value, ok := ResolvePayloadPath(data, "service")
			require.True(t, ok)
			require.Equal(t, "acme/unconfigured-webapp", value)
		})
	}
}

func TestSafeWebhookTriggerDataRejectsUnboundedPointers(t *testing.T) {
	_, err := safeWebhookTriggerData([]byte(`{"value":1}`), []string{"/value", "bad"}, "trigger", "delivery")
	require.Error(t, err)
}

func TestSafeWebhookTriggerDataValidatesPointerSyntaxAndTraversal(t *testing.T) {
	data, err := safeWebhookTriggerData(
		[]byte(`{"object":{"01":"leading","-":"dash"},"items":["first","second"]}`),
		[]string{"/object/01", "/object/-"}, "trigger", "delivery",
	)
	require.NoError(t, err)
	var projection map[string]any
	require.NoError(t, json.Unmarshal(data, &projection))
	require.Equal(t, map[string]any{
		"/object/01": "leading",
		"/object/-":  "dash",
	}, projection["payload"])

	for _, pointer := range []string{"/value~2", "/items/01", "/items/-", "/items/+1", "/items/-0"} {
		t.Run(pointer, func(t *testing.T) {
			_, err := safeWebhookTriggerData(
				[]byte(`{"value":"ok","items":["first","second"]}`),
				[]string{pointer}, "trigger", "delivery",
			)
			require.Error(t, err)
		})
	}
}

func TestSafeRetryTriggerProjectionKeepsRoutingMetadata(t *testing.T) {
	data := SafeRetryTriggerProjection(
		TriggerTypeGitHubPR, "trigger", []byte(`{
			"repo":"acme/api","number":42,"title":"Fix retry safety",
			"html_url":"https://github.com/acme/api/pull/42","author_login":"alice",
			"body":"review me","head_branch":"feature","base_branch":"main",
			"token":"secret"
		}`), "delivery-1")
	var projection map[string]any
	require.NoError(t, json.Unmarshal(data, &projection))
	require.Equal(t, "acme/api", projection["repo"])
	require.Equal(t, float64(42), projection["number"])
	require.Equal(t, "Fix retry safety", projection["title"])
	require.Equal(t, "https://github.com/acme/api/pull/42", projection["html_url"])
	require.Equal(t, "alice", projection["author_login"])
	require.Equal(t, "review me", projection["body"])
	require.Equal(t, "feature", projection["head_branch"])
	require.Equal(t, "main", projection["base_branch"])
	require.NotContains(t, projection, "token")
}
