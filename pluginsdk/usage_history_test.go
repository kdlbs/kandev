package pluginsdk

import (
	"context"
	"math"
	"testing"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestUsageHistoryV1ObservationProtoRoundTripPreservesOptionalityAndInt64(t *testing.T) {
	zero := int64(0)
	max := int64(math.MaxInt64)
	trueValue := true
	falseValue := false
	empty := ""
	modelProvider := "openai"
	currency := "USD"
	start := "2026-10-01T00:00:00Z"
	end := "2026-11-01T00:00:00Z"

	input := UsageObservationV1{
		SubjectBinding:    "binding-issued-by-core",
		ItemID:            "immutable-observation-id",
		SourceRecordID:    "mutable-provider-record",
		SourceVersion:     "collector-4.2.0",
		Revision:          17,
		PayloadDigest:     "sha256:payload",
		SupersedesItemID:  &empty,
		SourceSetID:       stringPointer("monthly-import"),
		SourceSetRevision: int64Pointer(9),
		WorkIdentity: &UsageWorkIdentityV1{
			Scheme:    "provider_account_thread",
			Authority: stringPointer("provider-account-7"),
			ID:        "thread-123",
		},
		ModelID:         stringPointer("model-exact-id"),
		ModelProvider:   &modelProvider,
		AgentClient:     nil,
		ServingPlatform: stringPointer("regional-edge"),
		RawSourceLabels: map[string]string{"source_kind": "conversation"},
		Tokens: &UsageTokenAmountsV1{
			InputTokens:      &zero,
			OutputTokens:     &max,
			CacheReadTokens:  nil,
			CacheWriteTokens: int64Pointer(12),
			ReasoningTokens:  int64Pointer(4),
			TotalTokens:      &max,
		},
		TokenSemantics: &UsageTokenSemanticsV1{
			Version:                   "provider-v2",
			CacheReadIncludedInInput:  &trueValue,
			CacheWriteIncludedInInput: &falseValue,
			ReasoningIncludedInOutput: nil,
		},
		Cost: &UsageCostV1{
			Amount:            &max,
			Currency:          &currency,
			Basis:             UsageCostBasisReported,
			PricingProvenance: stringPointer("provider invoice estimate"),
		},
		Coverage: &UsageCoverageV1{
			OccurrenceAt:   nil,
			CoverageStart:  &start,
			CoverageEnd:    &end,
			SourceDate:     stringPointer("2026-10-31"),
			SourceTimezone: stringPointer("Europe/Lisbon"),
		},
		Quality: &UsageObservationQualityV1{
			TokenCoverageComplete: true,
			CostCoverageComplete:  false,
			Stale:                 true,
		},
	}

	wire := input.toProto()
	encoded, err := proto.Marshal(wire)
	require.NoError(t, err)
	decoded := &pluginv1.UsageObservationV1{}
	require.NoError(t, proto.Unmarshal(encoded, decoded))
	require.Equal(t, input, usageObservationV1FromProto(decoded))

	jsonBytes, err := protojson.Marshal(wire)
	require.NoError(t, err)
	require.Contains(t, string(jsonBytes), `"outputTokens":"9223372036854775807"`)
	require.Contains(t, string(jsonBytes), `"inputTokens":"0"`)
	require.Nil(t, decoded.Tokens.CacheReadTokens)
	require.NotNil(t, decoded.Tokens.InputTokens)
	require.EqualValues(t, 0, decoded.Tokens.GetInputTokens())
}

func TestUsageHistoryV1BatchPreservesExplicitRetirementOnly(t *testing.T) {
	batch := UsageObservationBatchV1{
		SchemaVersion: UsageObservationSchemaVersionV1,
		Observations: []UsageObservationV1{{
			SubjectBinding:    "binding-1",
			ItemID:            "item-new",
			SourceSetID:       stringPointer("set-1"),
			SourceSetRevision: int64Pointer(4),
			Revision:          3,
			PayloadDigest:     "sha256:new",
		}},
		Tombstones: []UsageObservationTombstoneV1{{
			SubjectBinding: "binding-1",
			ItemID:         "item-retired",
			Revision:       8,
			PayloadDigest:  "sha256:tombstone",
		}},
		CompleteSetRevision: &UsageCompleteSetRevisionV1{
			SubjectBinding: "binding-1",
			SetID:          "set-1",
			Revision:       4,
			PayloadDigest:  "sha256:complete-set",
		},
	}

	require.Equal(t, batch, usageObservationBatchV1FromProto(batch.toProto()))
	partial := UsageObservationBatchV1{SchemaVersion: UsageObservationSchemaVersionV1, Observations: batch.Observations}
	require.Nil(t, partial.CompleteSetRevision)
	require.Empty(t, partial.Tombstones)
}

func TestUsageHistoryV1DigestsAreDeterministicAndCoverThePayload(t *testing.T) {
	first := UsageObservationV1{
		SubjectBinding: "binding-1", ItemID: "item-1", Revision: 3,
		ModelID:         stringPointer("model-a"),
		RawSourceLabels: map[string]string{"z": "last", "a": "first"},
		Tokens:          &UsageTokenAmountsV1{InputTokens: int64Pointer(12)},
	}
	second := UsageObservationV1{
		SubjectBinding: "binding-1", ItemID: "item-1", Revision: 3,
		ModelID:         stringPointer("model-a"),
		RawSourceLabels: map[string]string{"a": "first", "z": "last"},
		Tokens:          &UsageTokenAmountsV1{InputTokens: int64Pointer(12)},
	}

	firstDigest, err := first.ComputePayloadDigestV1()
	require.NoError(t, err)
	secondDigest, err := second.ComputePayloadDigestV1()
	require.NoError(t, err)
	require.Equal(t, firstDigest, secondDigest)
	require.Regexp(t, `^sha256:[0-9a-f]{64}$`, firstDigest)

	first.PayloadDigest = firstDigest
	replayedDigest, err := first.ComputePayloadDigestV1()
	require.NoError(t, err)
	require.Equal(t, firstDigest, replayedDigest)

	changed := second
	changed.ModelID = stringPointer("model-b")
	changedDigest, err := changed.ComputePayloadDigestV1()
	require.NoError(t, err)
	require.NotEqual(t, firstDigest, changedDigest)
	changed.Revision++
	changedRevisionDigest, err := changed.ComputePayloadDigestV1()
	require.NoError(t, err)
	require.NotEqual(t, changedDigest, changedRevisionDigest)

	tombstone := UsageObservationTombstoneV1{SubjectBinding: "binding-1", ItemID: "item-1", Revision: 4}
	tombstoneDigest, err := tombstone.ComputePayloadDigestV1()
	require.NoError(t, err)
	tombstone.PayloadDigest = tombstoneDigest
	replayedTombstoneDigest, err := tombstone.ComputePayloadDigestV1()
	require.NoError(t, err)
	require.Equal(t, tombstoneDigest, replayedTombstoneDigest)

	set := UsageCompleteSetRevisionV1{SubjectBinding: "binding-1", SetID: "set-1", Revision: 5}
	setDigest, err := set.ComputePayloadDigestV1([]string{"item-b", "item-a"})
	require.NoError(t, err)
	reorderedSetDigest, err := set.ComputePayloadDigestV1([]string{"item-a", "item-b"})
	require.NoError(t, err)
	require.Equal(t, setDigest, reorderedSetDigest)
	changedSetDigest, err := set.ComputePayloadDigestV1([]string{"item-a"})
	require.NoError(t, err)
	require.NotEqual(t, setDigest, changedSetDigest)
	_, err = set.ComputePayloadDigestV1([]string{"item-a", "item-a"})
	require.Error(t, err)
}

func TestUsageHistoryV1CollectionTargetUsesImmutableSnapshotKeyset(t *testing.T) {
	query := UsageCollectionTargetQueryV1{
		WorkspaceID: "workspace-1",
		Limit:       37,
		SnapshotID:  "snapshot-opaque",
		Cursor:      "keyset-opaque",
	}
	page := UsageCollectionTargetPageV1{
		Targets: []UsageCollectionTargetV1{{
			SubjectBinding:         "binding-issued-by-core",
			TaskID:                 "task-1",
			SessionID:              stringPointer("session-1"),
			ExternalSessionID:      stringPointer("agent-run-1"),
			ActorAttributionStatus: "unknown",
			SourceScopeStatus:      "eligible",
		}},
		SnapshotID:         "snapshot-opaque",
		SnapshotUpperBound: 9223372036854775000,
		NextCursor:         "next-keyset",
		HasMore:            true,
	}

	require.Equal(t, query, usageCollectionTargetQueryV1FromProto(query.toProto()))
	require.Equal(t, page, usageCollectionTargetPageV1FromProto(page.toProto()))
	preparing := UsageCollectionTargetPageV1{Preparing: true}
	require.Equal(t, preparing, usageCollectionTargetPageV1FromProto(preparing.toProto()))
}

func TestUsageHistoryV1DiscoveryPreparationAndSourceSafetyWire(t *testing.T) {
	target := (&pluginv1.UsageCollectionTargetV1{}).ProtoReflect().Descriptor()
	require.NotNil(t, target.Fields().ByName("source_scope_status"), "source context must expose explicit collection eligibility")
	page := (&pluginv1.ListUsageCollectionTargetsV1Response{}).ProtoReflect().Descriptor()
	require.NotNil(t, page.Fields().ByName("preparing"), "bounded preparation is not a complete empty snapshot")
}

func TestUsageHistoryV1CurrentTargetPointQueryWire(t *testing.T) {
	request := (&pluginv1.ListUsageCollectionTargetsV1Request{}).ProtoReflect().Descriptor()
	point := request.Fields().ByName("current_target")
	require.NotNil(t, point, "current eligibility must support a bounded point query")
	require.Equal(t, protoreflect.FieldNumber(5), point.Number())
	require.Equal(t, protoreflect.MessageKind, point.Kind())
	require.Equal(t, protoreflect.FullName("kandev.plugin.v1.UsageCollectionCurrentTargetQueryV1"), point.Message().FullName())
	for name, number := range map[protoreflect.Name]protoreflect.FieldNumber{
		"task_id": 1, "session_id": 2, "external_session_id": 3, "subject_binding": 4,
	} {
		field := point.Message().Fields().ByName(name)
		require.NotNil(t, field, "current-target constraints must remain available")
		require.Equal(t, number, field.Number())
		require.Equal(t, protoreflect.StringKind, field.Kind())
	}
	response := (&pluginv1.ListUsageCollectionTargetsV1Response{}).ProtoReflect().Descriptor()
	acknowledgement := response.Fields().ByName("current_target")
	require.NotNil(t, acknowledgement, "even an empty current-target result must acknowledge support")
	require.Equal(t, protoreflect.FieldNumber(7), acknowledgement.Number())
	require.Equal(t, protoreflect.BoolKind, acknowledgement.Kind())
	require.False(t, (&pluginv1.ListUsageCollectionTargetsV1Response{}).GetCurrentTarget(), "an older Host's default response cannot acknowledge point-query support")
}

func TestSessionUsageLegacyWireNumbersAndMapperRemainCompatible(t *testing.T) {
	descriptor := (&pluginv1.SessionUsageMeasurement{}).ProtoReflect().Descriptor()
	for name, want := range map[protoreflect.Name]protoreflect.FieldNumber{
		"id":                 1,
		"workspace_id":       2,
		"task_id":            3,
		"session_id":         4,
		"transcript_id":      5,
		"source_record_id":   6,
		"usage_identity":     7,
		"model":              8,
		"provider":           9,
		"source_version":     10,
		"revision":           11,
		"payload_digest":     12,
		"observed_at":        13,
		"collected_at":       14,
		"input_tokens":       15,
		"output_tokens":      16,
		"cache_read_tokens":  17,
		"cache_write_tokens": 18,
		"reasoning_tokens":   19,
		"total_tokens":       20,
		"cost_subcents":      21,
		"currency":           22,
		"cost_basis":         23,
		"coverage":           24,
		"source_timezone":    25,
		"attribution_status": 26,
		"coverage_start":     27,
		"coverage_end":       28,
		"source_date":        29,
		"coverage_key":       30,
		"estimated":          31,
		"stale":              32,
		"turns":              33,
		"cost_coverage":      34,
	} {
		field := descriptor.Fields().ByName(name)
		require.NotNil(t, field, "field %s remains present", name)
		require.Equal(t, want, field.Number(), "field %s retains its wire number", name)
	}
	observationDescriptor := (&pluginv1.UsageObservationV1{}).ProtoReflect().Descriptor()
	for _, name := range []protoreflect.Name{"workspace_id", "organization_id", "actor_id", "collector_namespace"} {
		require.Nil(t, observationDescriptor.Fields().ByName(name), "collectors cannot claim %s", name)
	}

	max := int64(math.MaxInt64)
	legacy := SessionUsageMeasurement{
		ID: "legacy-id", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		SourceRecordID: "legacy-record", UsageIdentity: "legacy-identity", InputTokens: &max,
		CostSubcents: &max, Currency: "USD", CostBasis: "reported",
	}
	require.Equal(t, legacy, sessionUsageMeasurementFromProto(legacy.toProto()))
}

type usageHistoryRecordingHost struct {
	*recordingHost
	reader UsageHistoryReader
}

func (h *usageHistoryRecordingHost) UsageHistory() UsageHistoryReader { return h.reader }

type recordingUsageHistoryReader struct {
	query  UsageCollectionTargetQueryV1
	batch  UsageObservationBatchV1
	page   UsageCollectionTargetPageV1
	result UsageObservationBatchResultV1
}

func (r *recordingUsageHistoryReader) ListTargets(_ context.Context, query UsageCollectionTargetQueryV1) (UsageCollectionTargetPageV1, error) {
	r.query = query
	return r.page, nil
}

func (r *recordingUsageHistoryReader) Report(_ context.Context, batch UsageObservationBatchV1) (UsageObservationBatchResultV1, error) {
	r.batch = batch
	return r.result, nil
}

func TestUsageHistoryV1GRPCRoundTrip(t *testing.T) {
	reader := &recordingUsageHistoryReader{
		page: UsageCollectionTargetPageV1{
			Targets:    []UsageCollectionTargetV1{{SubjectBinding: "binding-1", TaskID: "task-1"}},
			SnapshotID: "snapshot-1", SnapshotUpperBound: 42, HasMore: true, NextCursor: "cursor-2",
		},
		result: UsageObservationBatchResultV1{
			Status: "accepted", SupportedSchemaVersion: UsageObservationSchemaVersionV1,
			CollectorNamespace: "host-assigned-collector",
			Results: []UsageObservationWriteResultV1{{
				ItemID: "item-1", Operation: "observation", Status: "accepted", AcceptedAt: "2026-10-28T12:00:00Z",
				ActorAttributionStatus: "attributed", WorkIdentityStatus: "proven",
				CurrentObservation: &UsageObservationV1{SubjectBinding: "binding-1", ItemID: "item-1", Revision: 1},
				Selection: &UsageSelectionProvenanceV1{
					TokenSource: &UsageObservationSourceV1{CollectorNamespace: "native", ItemID: "native-9", ActorAttributionStatus: "attributed"},
					CostSource:  &UsageObservationSourceV1{CollectorNamespace: "collector:session-cost", ItemID: "item-1", ActorAttributionStatus: "mixed"},
				},
			}},
		},
	}
	host := dialHostOverBufconn(t, &usageHistoryRecordingHost{recordingHost: &recordingHost{}, reader: reader})
	history, ok := HostUsageHistory(host)
	require.True(t, ok)

	query := UsageCollectionTargetQueryV1{WorkspaceID: "workspace-1", Limit: 10}
	page, err := history.ListTargets(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, query, reader.query)
	require.Equal(t, reader.page, page)

	batch := UsageObservationBatchV1{SchemaVersion: UsageObservationSchemaVersionV1, Observations: []UsageObservationV1{{SubjectBinding: "binding-1", ItemID: "item-1", Revision: 1}}}
	result, err := history.Report(context.Background(), batch)
	require.NoError(t, err)
	require.Equal(t, batch, reader.batch)
	require.Equal(t, reader.result, result)
}

func TestUsageHistoryV1OptionalExtension(t *testing.T) {
	_, ok := HostUsageHistory(&recordingHost{})
	require.False(t, ok)
}

func TestUsageHistoryV1RetainedDatesAndProjectionStateAreAdditive(t *testing.T) {
	target := (&pluginv1.UsageCollectionTargetV1{}).ProtoReflect().Descriptor()
	for _, field := range []protoreflect.Name{"session_started_at", "session_ended_at", "coverage_start", "coverage_end"} {
		require.NotNil(t, target.Fields().ByName(field), "trusted retained metadata %s", field)
	}
	page := (&pluginv1.PageInfo{}).ProtoReflect().Descriptor()
	for _, field := range []protoreflect.Name{"projection_status", "projection_generation", "projection_pending"} {
		require.NotNil(t, page.Fields().ByName(field), "canonical read state %s", field)
	}
}

func TestUsageHistoryV1ReportsUnimplementedFromOlderHost(t *testing.T) {
	host := dialHostOverBufconn(t, &recordingHost{})
	history, ok := HostUsageHistory(host)
	require.True(t, ok)
	_, err := history.ListTargets(context.Background(), UsageCollectionTargetQueryV1{WorkspaceID: "workspace-1"})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

func stringPointer(value string) *string { return &value }

func int64Pointer(value int64) *int64 { return &value }
