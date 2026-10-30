package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type fixtureHistoryHost struct {
	pluginsdk.Host
	history *fixtureHistoryReader
	usage   *fixtureCanonicalReader
}

func (h fixtureHistoryHost) UsageHistory() pluginsdk.UsageHistoryReader { return h.history }
func (h fixtureHistoryHost) Usage() pluginsdk.UsageReader               { return h.usage }

type fixtureHistoryReader struct {
	query        pluginsdk.UsageCollectionTargetQueryV1
	batch        pluginsdk.UsageObservationBatchV1
	targets      []pluginsdk.UsageCollectionTargetV1
	reportCalled bool
}

func (r *fixtureHistoryReader) ListTargets(_ context.Context, query pluginsdk.UsageCollectionTargetQueryV1) (pluginsdk.UsageCollectionTargetPageV1, error) {
	r.query = query
	for _, target := range r.targets {
		if query.CurrentTarget != nil && query.CurrentTarget.SubjectBinding == target.SubjectBinding {
			return pluginsdk.UsageCollectionTargetPageV1{CurrentTarget: true, Targets: []pluginsdk.UsageCollectionTargetV1{target}}, nil
		}
	}
	return pluginsdk.UsageCollectionTargetPageV1{CurrentTarget: query.CurrentTarget != nil}, nil
}

func (r *fixtureHistoryReader) Report(_ context.Context, batch pluginsdk.UsageObservationBatchV1) (pluginsdk.UsageObservationBatchResultV1, error) {
	r.reportCalled = true
	r.batch = batch
	return pluginsdk.UsageObservationBatchResultV1{}, nil
}

type fixtureCanonicalReader struct {
	pluginsdk.UsageReader
	filter pluginsdk.SessionUsageFilter
}

func (r *fixtureCanonicalReader) ListCanonical(_ context.Context, filter pluginsdk.SessionUsageFilter, _ pluginsdk.Page) ([]pluginsdk.SessionUsageMeasurement, *pluginsdk.PageInfo, error) {
	r.filter = filter
	return nil, &pluginsdk.PageInfo{ProjectionStatus: "preparing", ProjectionPending: true}, nil
}

func TestUsageHistoryFixtureUsesAuthorizedWorkspaceAndExactAmounts(t *testing.T) {
	history, usage := &fixtureHistoryReader{targets: []pluginsdk.UsageCollectionTargetV1{{SubjectBinding: "host-binding"}}}, &fixtureCanonicalReader{}
	plug := &fixturePlugin{}
	plug.SetHost(fixtureHistoryHost{history: history, usage: usage})
	invoke := func(body string) *pluginsdk.PluginActionResponse {
		t.Helper()
		response, err := plug.HandleAction(t.Context(), &pluginsdk.PluginActionRequest{
			ActionKey: usageHistoryFixtureAction, Body: []byte(body), Context: pluginsdk.VerifiedActionContext{WorkspaceID: "authorized"},
		})
		require.NoError(t, err)
		return response
	}
	invoke(`{"operation":"targets"}`)
	require.Equal(t, "authorized", history.query.WorkspaceID)
	response := invoke(`{"operation":"canonical","filter":{"WorkspaceID":"forged","IncludeUndated":true}}`)
	require.Equal(t, "authorized", usage.filter.WorkspaceID)
	require.True(t, usage.filter.IncludeUndated)
	var body struct {
		Page pluginsdk.PageInfo `json:"page_info"`
	}
	require.NoError(t, json.Unmarshal(response.Body, &body))
	require.Equal(t, "preparing", body.Page.ProjectionStatus)
	require.True(t, body.Page.ProjectionPending)
	invoke(`{"operation":"report","subject_binding":"host-binding","item_id":"exact","revision":1,"tokens":{"total_tokens":"9007199254740993","input_tokens":"9007199254740993"},"cost":{"amount":"9223372036854775807","currency":"USD","basis":"reported"}}`)
	require.Len(t, history.batch.Observations, 1)
	item := history.batch.Observations[0]
	require.EqualValues(t, 9007199254740993, *item.Tokens.TotalTokens)
	require.EqualValues(t, 9223372036854775807, *item.Cost.Amount)
	require.Nil(t, item.Tokens.OutputTokens, "the fixture cannot invent measured zeroes")
	digest, err := item.ComputePayloadDigestV1()
	require.NoError(t, err)
	require.Equal(t, digest, item.PayloadDigest)
}

func TestUsageHistoryFixtureRejectsBindingsOutsideAuthorizedWorkspace(t *testing.T) {
	history := &fixtureHistoryReader{targets: []pluginsdk.UsageCollectionTargetV1{{SubjectBinding: "authorized-binding"}}}
	plug := &fixturePlugin{}
	plug.SetHost(fixtureHistoryHost{history: history, usage: &fixtureCanonicalReader{}})

	response, err := plug.HandleAction(t.Context(), &pluginsdk.PluginActionRequest{
		ActionKey: usageHistoryFixtureAction,
		Body:      []byte(`{"operation":"report","subject_binding":"foreign-binding","item_id":"cross-scope","revision":1,"tokens":{"total_tokens":"9999"}}`),
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "authorized"},
	})
	require.NoError(t, err)
	require.Equal(t, "authorized", history.query.WorkspaceID)
	require.NotNil(t, history.query.CurrentTarget)
	require.Equal(t, "foreign-binding", history.query.CurrentTarget.SubjectBinding)
	require.False(t, history.reportCalled, "the fixture must not forward a binding outside the verified workspace")
	var result pluginsdk.UsageObservationBatchResultV1
	require.NoError(t, json.Unmarshal(response.Body, &result))
	require.Len(t, result.Results, 1)
	require.Equal(t, "rejected", result.Results[0].Status)
	require.Equal(t, "cross-scope", result.Results[0].ItemID)

	response, err = plug.HandleAction(t.Context(), &pluginsdk.PluginActionRequest{
		ActionKey: usageHistoryFixtureAction,
		Body:      []byte(`{"operation":"tombstone","subject_binding":"foreign-binding","item_id":"cross-scope-retire","revision":1}`),
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "authorized"},
	})
	require.NoError(t, err)
	require.Equal(t, "authorized", history.query.WorkspaceID)
	require.NotNil(t, history.query.CurrentTarget)
	require.Equal(t, "foreign-binding", history.query.CurrentTarget.SubjectBinding)
	require.False(t, history.reportCalled, "the fixture must not forward a tombstone outside the verified workspace")
	require.NoError(t, json.Unmarshal(response.Body, &result))
	require.Len(t, result.Results, 1)
	require.Equal(t, "rejected", result.Results[0].Status)
	require.Equal(t, "cross-scope-retire", result.Results[0].ItemID)
}

func TestUsageHistoryFixtureRejectsRoundedOrInvalidAmounts(t *testing.T) {
	for _, amount := range []string{"9007199254740993.0", "9223372036854775808", "-1", "01", "1e3"} {
		t.Run(amount, func(t *testing.T) {
			_, err := fixtureUsageTokens(map[string]string{"total_tokens": amount})
			require.Error(t, err)
		})
	}
}
