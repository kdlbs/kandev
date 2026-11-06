package usagehistory

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	BindingKindExecution = "execution"
	BindingKindTurn      = "turn"

	ActorStatusAttributed = "attributed"
	ActorStatusUnknown    = "unknown"
	ActorStatusMixed      = "mixed"

	ActorKindUser      = "user"
	ActorKindSynthetic = "synthetic"
	ActorKindService   = "service"
	ActorKindUnknown   = "unknown"
)

// Binding is trusted admission evidence. Workspace and actor fields are
// resolved by core before this contract is called.
type Binding struct {
	ID                   string
	Kind                 string
	WorkspaceID          string
	OrganizationID       string
	TaskID               string
	TaskIncarnationID    string
	SessionID            string
	SessionIncarnationID string
	TurnID               string
	ExecutionID          string
	ParentBindingID      string
	ActorID              string
	ActorStatus          string
	ActorKind            string
	ActorSource          string
	CapturedAt           time.Time
}

// NativeUsage is the normalized accounting copy of one accepted native
// task_usage_events row. It contains no conversation or task display data.
type NativeUsage struct {
	BindingID                string
	UsageEventID             string
	TaskID                   string
	SessionID                string
	TurnID                   string
	AgentClient              string
	Model                    string
	Provider                 string
	ProviderThreadID         string
	ProviderTurnID           string
	ProviderResponseID       string
	NativeScope              string
	MeasurementSource        string
	UsageCompleteness        string
	UsageSchemaVersion       int
	ContractVersion          int
	InputTokens              int64
	ThoughtTokens            *int64
	CacheReadTokens          *int64
	CacheWriteTokens         *int64
	OutputTokens             *int64
	ReasoningOutputTokens    *int64
	ReportedCacheWriteTokens *int64
	ReportedTotalTokens      *int64
	TotalTokens              int64
	CostSubcents             int64
	CostPresent              bool
	CostBasis                string
	CostSource               string
	Estimated                bool
	PricingCatalogVersion    string
	TokenSemanticsVersion    int
	ReasoningInsideOutput    bool
	CacheReadInsideInput     bool
	OccurredAt               time.Time
	CollectedAt              time.Time
}

// Retainer shares the caller's transaction so admission snapshots and native
// observations commit with the task records that authorize them.
type Retainer interface {
	AdmitBindingTx(context.Context, *sqlx.Tx, Binding) error
	RetainNativeUsageTx(context.Context, *sqlx.Tx, NativeUsage) error
}

// CollectionSessionWriter captures source ownership with the session's creation.
type CollectionSessionWriter interface {
	CaptureUsageCollectionSessionTx(context.Context, *sqlx.Tx, string, string) error
}

// Lifecycle serializes retained writes with destructive task/session cleanup.
// Preservation methods commit bounded batches while the durable fence rejects
// competing writes; completion and detachment share the caller's delete Tx.
type Lifecycle interface {
	AdmitUsageWriteTx(context.Context, *sqlx.Tx, string, string, string) error
	PreserveTaskUsage(context.Context, string) error
	CompleteTaskUsageTx(context.Context, *sqlx.Tx, string) error
	ReleaseTaskUsage(context.Context, string) error
	PreserveSessionUsage(context.Context, string, string, string) error
	CompleteSessionUsageTx(context.Context, *sqlx.Tx, string, string, string) error
	ReleaseSessionUsage(context.Context, string, string, string) error
	EraseWorkspaceUsageTx(context.Context, *sqlx.Tx, string) error
	EraseOrganizationUsage(context.Context, string) error
}

// Migrator runs one bounded, durable keyset page. The returned flag is true
// when all sources through their captured high-water marks have been handled.
type Migrator interface {
	BackfillUsageHistory(context.Context, int) (bool, error)
}

// E2EResetter removes retained usage for a reused test workspace after normal
// task deletion has exercised its preservation path.
type E2EResetter interface {
	ResetWorkspaceUsageHistory(context.Context, string) error
}
