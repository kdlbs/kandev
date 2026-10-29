package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kdlbs/kandev/pluginsdk"
)

const usageHistoryFixtureAction = "usage-history-fixture"

type fixtureUsageInput struct {
	Operation      string                               `json:"operation"`
	SubjectBinding string                               `json:"subject_binding"`
	ItemID         string                               `json:"item_id"`
	Revision       int64                                `json:"revision"`
	ModelID        *string                              `json:"model_id"`
	Provider       *string                              `json:"provider"`
	AgentClient    *string                              `json:"agent_client"`
	WorkIdentity   *pluginsdk.UsageWorkIdentityV1       `json:"work_identity"`
	Coverage       *pluginsdk.UsageCoverageV1           `json:"coverage"`
	Quality        *pluginsdk.UsageObservationQualityV1 `json:"quality"`
	TokenSemantics *pluginsdk.UsageTokenSemanticsV1     `json:"token_semantics"`
	Tokens         map[string]string                    `json:"tokens"`
	Cost           *fixtureUsageCost                    `json:"cost"`
	Filter         pluginsdk.SessionUsageFilter         `json:"filter"`
	Page           pluginsdk.Page                       `json:"page"`
	SnapshotID     string                               `json:"snapshot_id"`
	Cursor         string                               `json:"cursor"`
}

type fixtureUsageCost struct {
	Amount   *string                    `json:"amount"`
	Currency *string                    `json:"currency"`
	Basis    pluginsdk.UsageCostBasisV1 `json:"basis"`
}

// This packaged collector uses only public SDK calls. The authenticated action
// supplies its workspace; the Host supplies namespace, bindings and actors.
func (p *fixturePlugin) usageHistoryFixture(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	var input fixtureUsageInput
	decoder := json.NewDecoder(bytes.NewReader(req.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, fmt.Errorf("plugin-fixture: decode usage input: %w", err)
	}
	host := p.Host()
	reader, ok := pluginsdk.HostUsageHistory(host)
	if !ok {
		return nil, fmt.Errorf("plugin-fixture: retained usage Host unavailable")
	}
	var result any
	var err error
	switch input.Operation {
	case "targets":
		result, err = reader.ListTargets(ctx, pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: req.Context.WorkspaceID, Limit: 200, SnapshotID: input.SnapshotID, Cursor: input.Cursor})
	case "report":
		result, err = reportFixtureUsage(ctx, reader, input)
	case "tombstone":
		result, err = retireFixtureUsage(ctx, reader, input)
	case "canonical", "source":
		result, err = readFixtureUsage(ctx, host.Usage(), req.Context.WorkspaceID, input)
	default:
		return nil, fmt.Errorf("plugin-fixture: unknown usage operation %q", input.Operation)
	}
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &pluginsdk.PluginActionResponse{Body: body}, nil
}

func reportFixtureUsage(ctx context.Context, reader pluginsdk.UsageHistoryReader, input fixtureUsageInput) (pluginsdk.UsageObservationBatchResultV1, error) {
	item := pluginsdk.UsageObservationV1{
		SubjectBinding: input.SubjectBinding, ItemID: input.ItemID, SourceRecordID: input.ItemID,
		SourceVersion: "e2e-fixture-v1", Revision: input.Revision, ModelID: input.ModelID, ModelProvider: input.Provider,
		AgentClient: input.AgentClient, WorkIdentity: input.WorkIdentity, Coverage: input.Coverage, Quality: input.Quality, TokenSemantics: input.TokenSemantics,
	}
	var err error
	item.Tokens, err = fixtureUsageTokens(input.Tokens)
	if err != nil {
		return pluginsdk.UsageObservationBatchResultV1{}, err
	}
	if input.Cost != nil {
		amount, parseErr := fixtureUsageDecimal(input.Cost.Amount)
		if parseErr != nil {
			return pluginsdk.UsageObservationBatchResultV1{}, parseErr
		}
		item.Cost = &pluginsdk.UsageCostV1{Amount: amount, Currency: input.Cost.Currency, Basis: input.Cost.Basis}
	}
	item.PayloadDigest, err = item.ComputePayloadDigestV1()
	if err != nil {
		return pluginsdk.UsageObservationBatchResultV1{}, err
	}
	return reader.Report(ctx, pluginsdk.UsageObservationBatchV1{SchemaVersion: pluginsdk.UsageObservationSchemaVersionV1, Observations: []pluginsdk.UsageObservationV1{item}})
}

func retireFixtureUsage(ctx context.Context, reader pluginsdk.UsageHistoryReader, input fixtureUsageInput) (pluginsdk.UsageObservationBatchResultV1, error) {
	item := pluginsdk.UsageObservationTombstoneV1{SubjectBinding: input.SubjectBinding, ItemID: input.ItemID, Revision: input.Revision}
	digest, err := item.ComputePayloadDigestV1()
	if err != nil {
		return pluginsdk.UsageObservationBatchResultV1{}, err
	}
	item.PayloadDigest = digest
	return reader.Report(ctx, pluginsdk.UsageObservationBatchV1{SchemaVersion: pluginsdk.UsageObservationSchemaVersionV1, Tombstones: []pluginsdk.UsageObservationTombstoneV1{item}})
}

func fixtureUsageDecimal(value *string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	amount, err := strconv.ParseInt(*value, 10, 64)
	if err != nil || amount < 0 || strconv.FormatInt(amount, 10) != *value {
		return nil, fmt.Errorf("plugin-fixture: invalid nonnegative int64 amount %q", *value)
	}
	return &amount, nil
}

func fixtureUsageTokens(values map[string]string) (*pluginsdk.UsageTokenAmountsV1, error) {
	if values == nil {
		return nil, nil
	}
	tokens := &pluginsdk.UsageTokenAmountsV1{}
	destinations := map[string]**int64{
		"input_tokens": &tokens.InputTokens, "output_tokens": &tokens.OutputTokens,
		"cache_read_tokens": &tokens.CacheReadTokens, "cache_write_tokens": &tokens.CacheWriteTokens,
		"reasoning_tokens": &tokens.ReasoningTokens, "total_tokens": &tokens.TotalTokens,
	}
	for key, value := range values {
		destination, ok := destinations[key]
		if !ok {
			return nil, fmt.Errorf("plugin-fixture: unknown token category %q", key)
		}
		parsed, err := fixtureUsageDecimal(&value)
		if err != nil {
			return nil, err
		}
		*destination = parsed
	}
	return tokens, nil
}

func readFixtureUsage(ctx context.Context, reader pluginsdk.UsageReader, workspace string, input fixtureUsageInput) (any, error) {
	input.Filter.WorkspaceID = workspace
	var rows []pluginsdk.SessionUsageMeasurement
	var info *pluginsdk.PageInfo
	var err error
	if input.Operation == "canonical" {
		rows, info, err = reader.ListCanonical(ctx, input.Filter, input.Page)
	} else {
		rows, info, err = reader.List(ctx, input.Filter, input.Page)
	}
	return struct {
		Rows []pluginsdk.SessionUsageMeasurement `json:"rows"`
		Page *pluginsdk.PageInfo                 `json:"page_info"`
	}{Rows: rows, Page: info}, err
}
