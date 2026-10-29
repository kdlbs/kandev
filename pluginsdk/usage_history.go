package pluginsdk

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sort"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"google.golang.org/protobuf/proto"
)

const (
	UsageObservationSchemaVersionV1 uint32 = 1
	// UsageObservationSchemaVersionV2 requires Hosts to understand explicit segments.
	// Older Hosts reject schema 2 instead of silently treating segments as duplicates.
	UsageObservationSchemaVersionV2 uint32 = 2
	// MaxUsageObservationBatchSizeV1 caps combined observation and tombstone entries.
	MaxUsageObservationBatchSizeV1 = 200
)

const (
	UsageReportStatusAccepted           = "accepted"
	UsageReportStatusPartial            = "partial"
	UsageReportStatusUnsupportedVersion = "unsupported_version"

	UsageObservationStatusAccepted           = "accepted"
	UsageObservationStatusUnchanged          = "unchanged"
	UsageObservationStatusStale              = "stale"
	UsageObservationStatusConflict           = "conflict"
	UsageObservationStatusRejected           = "rejected"
	UsageObservationStatusUnsupportedVersion = "unsupported_version"
)

type UsageCostBasisV1 string

const (
	UsageCostBasisReported  UsageCostBasisV1 = "reported"
	UsageCostBasisEstimated UsageCostBasisV1 = "estimated"
	UsageCostBasisUnknown   UsageCostBasisV1 = "unknown"
)

// UsageWorkIdentityV1 is a collector's work-identity evidence. Kandev accepts
// only schemes it supports. Proven transcript work is shared by retained
// subjects in the same captured accounting scope; event aliases require host
// evidence. Other identities remain specific to the retained subject.
// An unrecognized or unsupported identity remains unproven.
type UsageWorkIdentityV1 struct {
	Scheme    string
	Authority *string
	ID        string
}

// UsageTokenAmountsV1 keeps each category nullable so unknown differs from a
// measured zero. Token totals follow the supplied token-semantics version.
type UsageTokenAmountsV1 struct {
	InputTokens      *int64
	OutputTokens     *int64
	CacheReadTokens  *int64
	CacheWriteTokens *int64
	ReasoningTokens  *int64
	TotalTokens      *int64
}

// UsageTokenSemanticsV1 describes whether subset categories are already
// included in their parent input/output amounts.
type UsageTokenSemanticsV1 struct {
	Version                   string
	CacheReadIncludedInInput  *bool
	CacheWriteIncludedInInput *bool
	ReasoningIncludedInOutput *bool
}

// UsageCostV1 represents a fixed-point amount. Amount is nullable; when it is
// present, Currency is required. USD uses 10,000 units per dollar.
type UsageCostV1 struct {
	Amount            *int64
	Currency          *string
	Basis             UsageCostBasisV1
	PricingProvenance *string
}

// UsageCoverageV1 keeps point observations and interval coverage distinct.
// Intervals use an inclusive start and exclusive end.
type UsageCoverageV1 struct {
	OccurrenceAt   *string
	CoverageStart  *string
	CoverageEnd    *string
	SourceDate     *string
	SourceTimezone *string
}

// UsageObservationQualityV1 describes source completeness independently for
// tokens and cost. Actor attribution is resolved from the Host-issued binding.
type UsageObservationQualityV1 struct {
	TokenCoverageComplete bool
	CostCoverageComplete  bool
	Stale                 bool
}

// UsageObservationV1 is one retained source observation. ItemID remains
// stable when model or provider dimensions are corrected. Collector namespace,
// tenant scope, and actor attribution are supplied or derived by Kandev.
type UsageObservationV1 struct {
	SubjectBinding    string
	ItemID            string
	SourceRecordID    string
	SourceVersion     string
	Revision          int64
	PayloadDigest     string
	SupersedesItemID  *string
	SourceSetID       *string
	SourceSetRevision *int64
	WorkIdentity      *UsageWorkIdentityV1
	Segment           *UsageSegmentIdentityV1
	ModelID           *string
	ModelProvider     *string
	AgentClient       *string
	ServingPlatform   *string
	RawSourceLabels   map[string]string
	Tokens            *UsageTokenAmountsV1
	TokenSemantics    *UsageTokenSemanticsV1
	Cost              *UsageCostV1
	Coverage          *UsageCoverageV1
	Quality           *UsageObservationQualityV1
}

// ComputePayloadDigestV1 returns the digest of deterministic protobuf bytes
// with PayloadDigest cleared. The Host verifies this value before admission.
func (v UsageObservationV1) ComputePayloadDigestV1() (string, error) {
	message := v.toProto()
	message.PayloadDigest = ""
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

// UsageObservationTombstoneV1 explicitly retires one item in this
// authenticated collector namespace. A missing item in a partial report has
// no deletion meaning.
type UsageObservationTombstoneV1 struct {
	SubjectBinding string
	ItemID         string
	Revision       int64
	PayloadDigest  string
}

// ComputePayloadDigestV1 returns the digest of deterministic protobuf bytes
// with PayloadDigest cleared. The Host verifies this value before admission.
func (v UsageObservationTombstoneV1) ComputePayloadDigestV1() (string, error) {
	message := v.toProto()
	message.PayloadDigest = ""
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

// UsageCompleteSetRevisionV1 marks one bounded report as the complete current
// membership for a source set. The Host applies retirement atomically only
// after it validates every member and the set digest.
type UsageCompleteSetRevisionV1 struct {
	SubjectBinding string
	SetID          string
	Revision       int64
	PayloadDigest  string
}

// ComputePayloadDigestV1 hashes the complete set's identity and sorted item
// IDs. Each string uses an eight-byte big-endian length prefix. Duplicate or
// empty item IDs are invalid.
func (v UsageCompleteSetRevisionV1) ComputePayloadDigestV1(itemIDs []string) (string, error) {
	if v.SubjectBinding == "" || v.SetID == "" {
		return "", errors.New("complete-set binding and set id are required")
	}
	if len(itemIDs) > MaxUsageObservationBatchSizeV1 {
		return "", errors.New("complete-set item count exceeds the batch limit")
	}
	ids := append([]string(nil), itemIDs...)
	sort.Strings(ids)
	for i, id := range ids {
		if id == "" {
			return "", errors.New("complete-set item ids must not be empty")
		}
		if i > 0 && id == ids[i-1] {
			return "", errors.New("complete-set item ids must be unique")
		}
	}

	hasher := sha256.New()
	writeString := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(value))
	}
	_, _ = hasher.Write([]byte("kandev-usage-complete-set-v1\x00"))
	writeString(v.SubjectBinding)
	writeString(v.SetID)
	var revision [8]byte
	binary.BigEndian.PutUint64(revision[:], uint64(v.Revision))
	_, _ = hasher.Write(revision[:])
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(ids)))
	_, _ = hasher.Write(count[:])
	for _, id := range ids {
		writeString(id)
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

// UsageObservationBatchV1 is a versioned, bounded write. CompleteSetRevision
// is explicit; ordinary partial reports never retire omitted observations.
type UsageObservationBatchV1 struct {
	SchemaVersion       uint32
	Observations        []UsageObservationV1
	Tombstones          []UsageObservationTombstoneV1
	CompleteSetRevision *UsageCompleteSetRevisionV1
}

// UsageCollectionTargetV1 contains only target identity and supported source
// context. SubjectBinding is durable after task deletion and is required for
// every observation or correction.
type UsageCollectionTargetV1 struct {
	// SourceScopeStatus is eligible, mixed, or unknown. Only eligible source
	// context authorizes aggregate collection; a binding alone does not.
	SourceScopeStatus      string
	SessionStartedAt       *string
	SessionEndedAt         *string
	CoverageStart          *string
	CoverageEnd            *string
	SubjectBinding         string
	TaskID                 string
	SessionID              *string
	ExternalSessionID      *string
	SubjectKind            string
	ActorAttributionStatus string
}

// UsageCollectionTargetPageV1 is bound to one immutable snapshot and keyset
// cursor. SnapshotUpperBound allows a collector to checkpoint a bounded scan.
type UsageCollectionTargetPageV1 struct {
	// CurrentTarget acknowledges point-query support, including an empty result.
	CurrentTarget bool
	// Preparing commits bounded discovery progress. No snapshot or targets
	// exist yet; repeat the initial query to advance durable preparation.
	Preparing          bool
	Targets            []UsageCollectionTargetV1
	SnapshotID         string
	SnapshotUpperBound int64
	NextCursor         string
	HasMore            bool
}

type UsageCollectionTargetQueryV1 struct {
	WorkspaceID   string
	Limit         int32
	SnapshotID    string
	Cursor        string
	CurrentTarget *UsageCollectionCurrentTargetQueryV1
}

type UsageObservationSourceV1 struct {
	CollectorNamespace     string
	ItemID                 string
	ActorAttributionStatus string
}

// UsageSelectionProvenanceV1 reports the selected token and cost source
// independently. Either source may be absent when the evidence is incomplete
// or ambiguous.
type UsageSelectionProvenanceV1 struct {
	TokenSource *UsageObservationSourceV1
	CostSource  *UsageObservationSourceV1
}

type UsageObservationWriteResultV1 struct {
	Operation              string
	ItemID                 string
	Status                 string
	Error                  string
	CurrentRevision        int64
	CurrentDigest          string
	CurrentObservation     *UsageObservationV1
	AcceptedAt             string
	ActorAttributionStatus string
	WorkIdentityStatus     string
	Selection              *UsageSelectionProvenanceV1
}

type UsageObservationBatchResultV1 struct {
	Status                 string
	SupportedSchemaVersion uint32
	CollectorNamespace     string
	Results                []UsageObservationWriteResultV1
}

// UsageHistoryReader is an optional Host extension for retained-history
// collection. Existing Host implementations remain source-compatible.
type UsageHistoryReader interface {
	ListTargets(ctx context.Context, query UsageCollectionTargetQueryV1) (UsageCollectionTargetPageV1, error)
	Report(ctx context.Context, batch UsageObservationBatchV1) (UsageObservationBatchResultV1, error)
}

// UsageHistoryHost is an optional Host extension implemented by the SDK's
// gRPC client and by Host implementations that provide retained usage APIs.
type UsageHistoryHost interface {
	UsageHistory() UsageHistoryReader
}

// HostUsageHistory returns retained usage access when the Host supports the
// versioned collection API.
func HostUsageHistory(host Host) (UsageHistoryReader, bool) {
	provider, ok := host.(UsageHistoryHost)
	if !ok {
		return nil, false
	}
	reader := provider.UsageHistory()
	return reader, reader != nil
}

func (v UsageWorkIdentityV1) toProto() *pluginv1.UsageWorkIdentityV1 {
	return &pluginv1.UsageWorkIdentityV1{Scheme: v.Scheme, Authority: v.Authority, Id: v.ID}
}

func usageWorkIdentityV1FromProto(v *pluginv1.UsageWorkIdentityV1) *UsageWorkIdentityV1 {
	if v == nil {
		return nil
	}
	return &UsageWorkIdentityV1{Scheme: v.GetScheme(), Authority: v.Authority, ID: v.GetId()}
}

func (v UsageTokenAmountsV1) toProto() *pluginv1.UsageTokenAmountsV1 {
	return &pluginv1.UsageTokenAmountsV1{
		InputTokens: v.InputTokens, OutputTokens: v.OutputTokens, CacheReadTokens: v.CacheReadTokens,
		CacheWriteTokens: v.CacheWriteTokens, ReasoningTokens: v.ReasoningTokens, TotalTokens: v.TotalTokens,
	}
}

func usageTokenAmountsV1FromProto(v *pluginv1.UsageTokenAmountsV1) *UsageTokenAmountsV1 {
	if v == nil {
		return nil
	}
	return &UsageTokenAmountsV1{
		InputTokens: v.InputTokens, OutputTokens: v.OutputTokens, CacheReadTokens: v.CacheReadTokens,
		CacheWriteTokens: v.CacheWriteTokens, ReasoningTokens: v.ReasoningTokens, TotalTokens: v.TotalTokens,
	}
}

func (v UsageTokenSemanticsV1) toProto() *pluginv1.UsageTokenSemanticsV1 {
	return &pluginv1.UsageTokenSemanticsV1{
		Version: v.Version, CacheReadIncludedInInput: v.CacheReadIncludedInInput,
		CacheWriteIncludedInInput: v.CacheWriteIncludedInInput, ReasoningIncludedInOutput: v.ReasoningIncludedInOutput,
	}
}

func usageTokenSemanticsV1FromProto(v *pluginv1.UsageTokenSemanticsV1) *UsageTokenSemanticsV1 {
	if v == nil {
		return nil
	}
	return &UsageTokenSemanticsV1{
		Version: v.GetVersion(), CacheReadIncludedInInput: v.CacheReadIncludedInInput,
		CacheWriteIncludedInInput: v.CacheWriteIncludedInInput, ReasoningIncludedInOutput: v.ReasoningIncludedInOutput,
	}
}

func (v UsageCostV1) toProto() *pluginv1.UsageCostV1 {
	return &pluginv1.UsageCostV1{Amount: v.Amount, Currency: v.Currency, Basis: string(v.Basis), PricingProvenance: v.PricingProvenance}
}

func usageCostV1FromProto(v *pluginv1.UsageCostV1) *UsageCostV1 {
	if v == nil {
		return nil
	}
	return &UsageCostV1{Amount: v.Amount, Currency: v.Currency, Basis: UsageCostBasisV1(v.GetBasis()), PricingProvenance: v.PricingProvenance}
}

func (v UsageCoverageV1) toProto() *pluginv1.UsageCoverageV1 {
	return &pluginv1.UsageCoverageV1{
		OccurrenceAt: v.OccurrenceAt, CoverageStart: v.CoverageStart, CoverageEnd: v.CoverageEnd,
		SourceDate: v.SourceDate, SourceTimezone: v.SourceTimezone,
	}
}

func usageCoverageV1FromProto(v *pluginv1.UsageCoverageV1) *UsageCoverageV1 {
	if v == nil {
		return nil
	}
	return &UsageCoverageV1{
		OccurrenceAt: v.OccurrenceAt, CoverageStart: v.CoverageStart, CoverageEnd: v.CoverageEnd,
		SourceDate: v.SourceDate, SourceTimezone: v.SourceTimezone,
	}
}

func (v UsageObservationQualityV1) toProto() *pluginv1.UsageObservationQualityV1 {
	return &pluginv1.UsageObservationQualityV1{
		TokenCoverageComplete: v.TokenCoverageComplete, CostCoverageComplete: v.CostCoverageComplete, Stale: v.Stale,
	}
}

func usageObservationQualityV1FromProto(v *pluginv1.UsageObservationQualityV1) *UsageObservationQualityV1 {
	if v == nil {
		return nil
	}
	return &UsageObservationQualityV1{
		TokenCoverageComplete: v.GetTokenCoverageComplete(), CostCoverageComplete: v.GetCostCoverageComplete(), Stale: v.GetStale(),
	}
}

func (v UsageObservationV1) toProto() *pluginv1.UsageObservationV1 {
	var workIdentity *pluginv1.UsageWorkIdentityV1
	if v.WorkIdentity != nil {
		workIdentity = v.WorkIdentity.toProto()
	}
	var tokens *pluginv1.UsageTokenAmountsV1
	if v.Tokens != nil {
		tokens = v.Tokens.toProto()
	}
	var semantics *pluginv1.UsageTokenSemanticsV1
	if v.TokenSemantics != nil {
		semantics = v.TokenSemantics.toProto()
	}
	var cost *pluginv1.UsageCostV1
	if v.Cost != nil {
		cost = v.Cost.toProto()
	}
	var coverage *pluginv1.UsageCoverageV1
	if v.Coverage != nil {
		coverage = v.Coverage.toProto()
	}
	var quality *pluginv1.UsageObservationQualityV1
	if v.Quality != nil {
		quality = v.Quality.toProto()
	}
	return &pluginv1.UsageObservationV1{
		SubjectBinding: v.SubjectBinding, ItemId: v.ItemID, SourceRecordId: v.SourceRecordID,
		SourceVersion: v.SourceVersion, Revision: v.Revision, PayloadDigest: v.PayloadDigest,
		SupersedesItemId: v.SupersedesItemID, SourceSetId: v.SourceSetID, SourceSetRevision: v.SourceSetRevision,
		WorkIdentity: workIdentity, Segment: v.Segment.toProto(), ModelId: v.ModelID, ModelProvider: v.ModelProvider,
		AgentClient: v.AgentClient, ServingPlatform: v.ServingPlatform,
		RawSourceLabels: cloneStringMap(v.RawSourceLabels), Tokens: tokens, TokenSemantics: semantics,
		Cost: cost, Coverage: coverage, Quality: quality,
	}
}

func usageObservationV1FromProto(v *pluginv1.UsageObservationV1) UsageObservationV1 {
	if v == nil {
		return UsageObservationV1{}
	}
	return UsageObservationV1{
		SubjectBinding: v.GetSubjectBinding(), ItemID: v.GetItemId(), SourceRecordID: v.GetSourceRecordId(),
		SourceVersion: v.GetSourceVersion(), Revision: v.GetRevision(), PayloadDigest: v.GetPayloadDigest(),
		SupersedesItemID: v.SupersedesItemId, SourceSetID: v.SourceSetId, SourceSetRevision: v.SourceSetRevision,
		WorkIdentity: usageWorkIdentityV1FromProto(v.GetWorkIdentity()), Segment: usageSegmentIdentityV1FromProto(v.GetSegment()), ModelID: v.ModelId,
		ModelProvider: v.ModelProvider, AgentClient: v.AgentClient, ServingPlatform: v.ServingPlatform,
		RawSourceLabels: cloneStringMap(v.GetRawSourceLabels()), Tokens: usageTokenAmountsV1FromProto(v.GetTokens()),
		TokenSemantics: usageTokenSemanticsV1FromProto(v.GetTokenSemantics()), Cost: usageCostV1FromProto(v.GetCost()),
		Coverage: usageCoverageV1FromProto(v.GetCoverage()), Quality: usageObservationQualityV1FromProto(v.GetQuality()),
	}
}

func (v UsageObservationTombstoneV1) toProto() *pluginv1.UsageObservationTombstoneV1 {
	return &pluginv1.UsageObservationTombstoneV1{SubjectBinding: v.SubjectBinding, ItemId: v.ItemID, Revision: v.Revision, PayloadDigest: v.PayloadDigest}
}

func usageObservationTombstoneV1FromProto(v *pluginv1.UsageObservationTombstoneV1) UsageObservationTombstoneV1 {
	if v == nil {
		return UsageObservationTombstoneV1{}
	}
	return UsageObservationTombstoneV1{SubjectBinding: v.GetSubjectBinding(), ItemID: v.GetItemId(), Revision: v.GetRevision(), PayloadDigest: v.GetPayloadDigest()}
}

func (v UsageCompleteSetRevisionV1) toProto() *pluginv1.UsageCompleteSetRevisionV1 {
	return &pluginv1.UsageCompleteSetRevisionV1{SubjectBinding: v.SubjectBinding, SetId: v.SetID, Revision: v.Revision, PayloadDigest: v.PayloadDigest}
}

func usageCompleteSetRevisionV1FromProto(v *pluginv1.UsageCompleteSetRevisionV1) *UsageCompleteSetRevisionV1 {
	if v == nil {
		return nil
	}
	return &UsageCompleteSetRevisionV1{SubjectBinding: v.GetSubjectBinding(), SetID: v.GetSetId(), Revision: v.GetRevision(), PayloadDigest: v.GetPayloadDigest()}
}

func (v UsageObservationBatchV1) toProto() *pluginv1.ReportUsageObservationsV1Request {
	out := &pluginv1.ReportUsageObservationsV1Request{SchemaVersion: v.SchemaVersion}
	if len(v.Observations) > 0 {
		out.Observations = make([]*pluginv1.UsageObservationV1, len(v.Observations))
		for i := range v.Observations {
			out.Observations[i] = v.Observations[i].toProto()
		}
	}
	if len(v.Tombstones) > 0 {
		out.Tombstones = make([]*pluginv1.UsageObservationTombstoneV1, len(v.Tombstones))
		for i := range v.Tombstones {
			out.Tombstones[i] = v.Tombstones[i].toProto()
		}
	}
	if v.CompleteSetRevision != nil {
		out.CompleteSetRevision = v.CompleteSetRevision.toProto()
	}
	return out
}

func usageObservationBatchV1FromProto(v *pluginv1.ReportUsageObservationsV1Request) UsageObservationBatchV1 {
	if v == nil {
		return UsageObservationBatchV1{}
	}
	out := UsageObservationBatchV1{SchemaVersion: v.GetSchemaVersion(), CompleteSetRevision: usageCompleteSetRevisionV1FromProto(v.GetCompleteSetRevision())}
	if len(v.GetObservations()) > 0 {
		out.Observations = make([]UsageObservationV1, len(v.GetObservations()))
		for i := range v.GetObservations() {
			out.Observations[i] = usageObservationV1FromProto(v.GetObservations()[i])
		}
	}
	if len(v.GetTombstones()) > 0 {
		out.Tombstones = make([]UsageObservationTombstoneV1, len(v.GetTombstones()))
		for i := range v.GetTombstones() {
			out.Tombstones[i] = usageObservationTombstoneV1FromProto(v.GetTombstones()[i])
		}
	}
	return out
}

func (v UsageCollectionTargetQueryV1) toProto() *pluginv1.ListUsageCollectionTargetsV1Request {
	return &pluginv1.ListUsageCollectionTargetsV1Request{WorkspaceId: v.WorkspaceID, Limit: v.Limit, SnapshotId: v.SnapshotID, Cursor: v.Cursor, CurrentTarget: v.CurrentTarget.toProto()}
}

func usageCollectionTargetQueryV1FromProto(v *pluginv1.ListUsageCollectionTargetsV1Request) UsageCollectionTargetQueryV1 {
	if v == nil {
		return UsageCollectionTargetQueryV1{}
	}
	return UsageCollectionTargetQueryV1{WorkspaceID: v.GetWorkspaceId(), Limit: v.GetLimit(), SnapshotID: v.GetSnapshotId(), Cursor: v.GetCursor(), CurrentTarget: usageCollectionCurrentTargetQueryV1FromProto(v.GetCurrentTarget())}
}

func (v UsageCollectionTargetV1) toProto() *pluginv1.UsageCollectionTargetV1 {
	return &pluginv1.UsageCollectionTargetV1{
		SubjectBinding: v.SubjectBinding, TaskId: v.TaskID, SessionId: v.SessionID, SessionStartedAt: v.SessionStartedAt, SessionEndedAt: v.SessionEndedAt, CoverageStart: v.CoverageStart, CoverageEnd: v.CoverageEnd,
		ExternalSessionId: v.ExternalSessionID, SubjectKind: v.SubjectKind, ActorAttributionStatus: v.ActorAttributionStatus,
		SourceScopeStatus: v.SourceScopeStatus,
	}
}

func usageCollectionTargetV1FromProto(v *pluginv1.UsageCollectionTargetV1) UsageCollectionTargetV1 {
	if v == nil {
		return UsageCollectionTargetV1{}
	}
	return UsageCollectionTargetV1{
		SubjectBinding: v.GetSubjectBinding(), TaskID: v.GetTaskId(), SessionID: v.SessionId, SessionStartedAt: v.SessionStartedAt, SessionEndedAt: v.SessionEndedAt, CoverageStart: v.CoverageStart, CoverageEnd: v.CoverageEnd,
		ExternalSessionID: v.ExternalSessionId, SubjectKind: v.GetSubjectKind(), ActorAttributionStatus: v.GetActorAttributionStatus(),
		SourceScopeStatus: v.GetSourceScopeStatus(),
	}
}

func (v UsageCollectionTargetPageV1) toProto() *pluginv1.ListUsageCollectionTargetsV1Response {
	out := &pluginv1.ListUsageCollectionTargetsV1Response{
		SnapshotId: v.SnapshotID, SnapshotUpperBound: v.SnapshotUpperBound, NextCursor: v.NextCursor, HasMore: v.HasMore,
		Preparing: v.Preparing, CurrentTarget: v.CurrentTarget,
	}
	if len(v.Targets) > 0 {
		out.Items = make([]*pluginv1.UsageCollectionTargetV1, len(v.Targets))
		for i := range v.Targets {
			out.Items[i] = v.Targets[i].toProto()
		}
	}
	return out
}

func usageCollectionTargetPageV1FromProto(v *pluginv1.ListUsageCollectionTargetsV1Response) UsageCollectionTargetPageV1 {
	if v == nil {
		return UsageCollectionTargetPageV1{}
	}
	out := UsageCollectionTargetPageV1{
		SnapshotID: v.GetSnapshotId(), SnapshotUpperBound: v.GetSnapshotUpperBound(), NextCursor: v.GetNextCursor(), HasMore: v.GetHasMore(),
		Preparing: v.GetPreparing(), CurrentTarget: v.GetCurrentTarget(),
	}
	if len(v.GetItems()) > 0 {
		out.Targets = make([]UsageCollectionTargetV1, len(v.GetItems()))
		for i := range v.GetItems() {
			out.Targets[i] = usageCollectionTargetV1FromProto(v.GetItems()[i])
		}
	}
	return out
}

func (v UsageObservationSourceV1) toProto() *pluginv1.UsageObservationSourceV1 {
	return &pluginv1.UsageObservationSourceV1{
		CollectorNamespace: v.CollectorNamespace, ItemId: v.ItemID,
		ActorAttributionStatus: v.ActorAttributionStatus,
	}
}

func usageObservationSourceV1FromProto(v *pluginv1.UsageObservationSourceV1) *UsageObservationSourceV1 {
	if v == nil {
		return nil
	}
	return &UsageObservationSourceV1{
		CollectorNamespace: v.GetCollectorNamespace(), ItemID: v.GetItemId(),
		ActorAttributionStatus: v.GetActorAttributionStatus(),
	}
}

func (v UsageSelectionProvenanceV1) toProto() *pluginv1.UsageSelectionProvenanceV1 {
	out := &pluginv1.UsageSelectionProvenanceV1{}
	if v.TokenSource != nil {
		out.TokenSource = v.TokenSource.toProto()
	}
	if v.CostSource != nil {
		out.CostSource = v.CostSource.toProto()
	}
	return out
}

func usageSelectionProvenanceV1FromProto(v *pluginv1.UsageSelectionProvenanceV1) *UsageSelectionProvenanceV1 {
	if v == nil {
		return nil
	}
	return &UsageSelectionProvenanceV1{TokenSource: usageObservationSourceV1FromProto(v.GetTokenSource()), CostSource: usageObservationSourceV1FromProto(v.GetCostSource())}
}

func (v UsageObservationWriteResultV1) toProto() *pluginv1.UsageObservationWriteResultV1 {
	out := &pluginv1.UsageObservationWriteResultV1{
		Operation: v.Operation, ItemId: v.ItemID, Status: v.Status, Error: v.Error,
		CurrentRevision: v.CurrentRevision, CurrentDigest: v.CurrentDigest, AcceptedAt: v.AcceptedAt,
		ActorAttributionStatus: v.ActorAttributionStatus, WorkIdentityStatus: v.WorkIdentityStatus,
	}
	if v.CurrentObservation != nil {
		out.CurrentObservation = v.CurrentObservation.toProto()
	}
	if v.Selection != nil {
		out.Selection = v.Selection.toProto()
	}
	return out
}

func usageObservationWriteResultV1FromProto(v *pluginv1.UsageObservationWriteResultV1) UsageObservationWriteResultV1 {
	if v == nil {
		return UsageObservationWriteResultV1{}
	}
	var current *UsageObservationV1
	if v.GetCurrentObservation() != nil {
		value := usageObservationV1FromProto(v.GetCurrentObservation())
		current = &value
	}
	return UsageObservationWriteResultV1{
		Operation: v.GetOperation(), ItemID: v.GetItemId(), Status: v.GetStatus(), Error: v.GetError(),
		CurrentRevision: v.GetCurrentRevision(), CurrentDigest: v.GetCurrentDigest(), CurrentObservation: current,
		AcceptedAt: v.GetAcceptedAt(), ActorAttributionStatus: v.GetActorAttributionStatus(),
		WorkIdentityStatus: v.GetWorkIdentityStatus(), Selection: usageSelectionProvenanceV1FromProto(v.GetSelection()),
	}
}

func (v UsageObservationBatchResultV1) toProto() *pluginv1.ReportUsageObservationsV1Response {
	out := &pluginv1.ReportUsageObservationsV1Response{
		Status: v.Status, SupportedSchemaVersion: v.SupportedSchemaVersion, CollectorNamespace: v.CollectorNamespace,
	}
	if len(v.Results) > 0 {
		out.Results = make([]*pluginv1.UsageObservationWriteResultV1, len(v.Results))
		for i := range v.Results {
			out.Results[i] = v.Results[i].toProto()
		}
	}
	return out
}

func usageObservationBatchResultV1FromProto(v *pluginv1.ReportUsageObservationsV1Response) UsageObservationBatchResultV1 {
	if v == nil {
		return UsageObservationBatchResultV1{}
	}
	out := UsageObservationBatchResultV1{
		Status: v.GetStatus(), SupportedSchemaVersion: v.GetSupportedSchemaVersion(), CollectorNamespace: v.GetCollectorNamespace(),
	}
	if len(v.GetResults()) > 0 {
		out.Results = make([]UsageObservationWriteResultV1, len(v.GetResults()))
		for i := range v.GetResults() {
			out.Results[i] = usageObservationWriteResultV1FromProto(v.GetResults()[i])
		}
	}
	return out
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
