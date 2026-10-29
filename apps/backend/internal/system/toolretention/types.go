package toolretention

import (
	"context"
	"time"
)

type Preparation struct {
	State  string `json:"state"`
	Choice string `json:"choice"`
	Error  string `json:"error,omitempty"`
}

type Operation struct {
	ID                 string           `json:"id"`
	Kind               string           `json:"kind"`
	State              string           `json:"state"`
	Age                Age              `json:"age"`
	Scanned            int64            `json:"scanned"`
	EligibleTasks      int64            `json:"eligible_tasks"`
	EligibleMessages   int64            `json:"eligible_messages"`
	RemovedMessages    int64            `json:"removed_messages"`
	PayloadBytes       int64            `json:"payload_bytes"`
	HistoryTasks       int64            `json:"history_tasks"`
	HistorySnapshots   int64            `json:"history_snapshots"`
	HistorySQLBytes    int64            `json:"history_sql_bytes"`
	HistoryGitBytes    int64            `json:"history_git_bytes"`
	ExpiredSets        int64            `json:"expired_change_sets"`
	DeletedArchiveRefs int64            `json:"deleted_archive_refs"`
	ArchiveFailures    int64            `json:"archive_failures"`
	ReclaimedBytes     int64            `json:"reclaimed_bytes"`
	HistoryTruncated   bool             `json:"history_truncated,omitempty"`
	Skipped            map[string]int64 `json:"skipped"`
	Cutoff             time.Time        `json:"cutoff"`
	StartedAt          time.Time        `json:"started_at"`
	FinishedAt         *time.Time       `json:"finished_at,omitempty"`
	Error              string           `json:"error,omitempty"`
}

// HistoryStats is the bounded result of one history-compaction pass. Byte
// categories stay separate so the UI never presents a misleading disk total.
type HistoryStats struct {
	Tasks              int64
	Snapshots          int64
	LegacySQLBytes     int64
	ArchiveBytes       int64
	ExpiredChangeSets  int64
	DeletedContents    int64
	FreedPayloadBytes  int64
	DeletedArchiveRefs int64
	ArchiveFailures    int64
	ReclaimedBytes     int64
	Remaining          int64
	Complete           bool
}

// HistoryCompactor is the task-owned bounded history operation used by the
// single administrative compaction policy.
type HistoryCompactor interface {
	Analyze(ctx context.Context, cutoff time.Time) (HistoryStats, error)
	HasWork(ctx context.Context, cutoff time.Time) (bool, error)
	CompactOnce(ctx context.Context, cutoff time.Time, limit int) (HistoryStats, error)
}

type Status struct {
	Policy       Policy      `json:"policy"`
	Supported    bool        `json:"supported"`
	Preparation  Preparation `json:"preparation"`
	Operation    *Operation  `json:"operation,omitempty"`
	LastAnalysis *Operation  `json:"last_analysis,omitempty"`
	LastRun      *Operation  `json:"last_run,omitempty"`
	NextDueAt    *time.Time  `json:"next_due_at,omitempty"`
}

type Update struct {
	Enabled      bool   `json:"enabled"`
	Age          Age    `json:"age"`
	Revision     int64  `json:"revision"`
	BackupChoice string `json:"backup_choice,omitempty"`
}

type progress struct {
	SchemaVersion          int    `json:"schema_version"`
	Task                   string `json:"task"`
	TaskAfter              string `json:"task_after"`
	Session                string `json:"session"`
	SessionAfter           string `json:"session_after"`
	Message                int64  `json:"message"`
	UpperTask              string `json:"upper_task"`
	UpperMessage           int64  `json:"upper_message"`
	TaskCounted            bool   `json:"task_counted"`
	HistoryDone            bool   `json:"history_done"`
	HistoryArchiveFailures int64  `json:"history_archive_failures,omitempty"`
	Revision               int64  `json:"revision"`
	Started                bool   `json:"started"`
}

type record struct {
	Status
	Version          int      `json:"version"`
	ApprovedRevision int64    `json:"approved_revision"`
	Receipt          string   `json:"receipt,omitempty"`
	FirstMutation    bool     `json:"first_mutation"`
	Progress         progress `json:"progress"`
	// PreparationDetail keeps the last backup preparation failure cause for
	// diagnosis. It sits outside the embedded Status, so the status endpoint
	// never returns raw internal error text.
	PreparationDetail string `json:"preparation_detail,omitempty"`
}

func defaultRecord() record {
	return record{Status: Status{Policy: DefaultPolicy(), Supported: true, Preparation: Preparation{State: stateNone}}, Version: recordVersion}
}
