package dataretention

import "time"

type Preparation struct {
	State  string `json:"state"`
	Choice string `json:"choice"`
	Error  string `json:"error,omitempty"`
}

// Target separates the two approved reductions in every counter and in the
// settings surface, so one target's result is never reported as the other's.
type Target struct {
	Rows       int64 `json:"rows"`
	Reduced    int64 `json:"reduced"`
	Bytes      int64 `json:"bytes"`
	Eligible   int64 `json:"eligible"`
	Oversized  int64 `json:"oversized"`
	TotalBytes int64 `json:"total_bytes"`
}

type Operation struct {
	ID              string           `json:"id"`
	Kind            string           `json:"kind"`
	State           string           `json:"state"`
	Messages        Target           `json:"messages"`
	CleanupJobs     Target           `json:"cleanup_jobs"`
	Skipped         map[string]int64 `json:"skipped"`
	ArchivedCutoff  time.Time        `json:"archived_cutoff"`
	CleanupCutoff   time.Time        `json:"cleanup_cutoff"`
	PolicyRevision  int64            `json:"policy_revision"`
	StartedAt       time.Time        `json:"started_at"`
	FinishedAt      *time.Time       `json:"finished_at,omitempty"`
	Error           string           `json:"error,omitempty"`
	AnalysisOnly    bool             `json:"analysis_only,omitempty"`
	Complete        bool             `json:"complete,omitempty"`
	CompletedTarget string           `json:"completed_target,omitempty"`
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
	ArchivedAge  Age    `json:"archived_age"`
	CleanupAge   Age    `json:"cleanup_age"`
	Revision     int64  `json:"revision"`
	BackupChoice string `json:"backup_choice,omitempty"`
}

// progress is the resumable cursor of one pass. Each target keeps its own
// keyset so finishing one never loses the other's position.
type progress struct {
	SchemaVersion int    `json:"schema_version"`
	Task          string `json:"task"`
	TaskAfter     string `json:"task_after"`
	Session       string `json:"session"`
	SessionAfter  string `json:"session_after"`
	Message       int64  `json:"message"`
	UpperTask     string `json:"upper_task"`
	UpperMessage  int64  `json:"upper_message"`
	Job           string `json:"job"`
	JobAfter      string `json:"job_after"`
	UpperJob      string `json:"upper_job"`
	Phase         string `json:"phase"`
	Revision      int64  `json:"revision"`
	Started       bool   `json:"started"`
}

const (
	phaseMessages = "messages"
	phaseJobs     = "jobs"
)

type record struct {
	Status
	Version          int      `json:"version"`
	ApprovedRevision int64    `json:"approved_revision"`
	Receipt          string   `json:"receipt,omitempty"`
	FirstMutation    bool     `json:"first_mutation"`
	Progress         progress `json:"progress"`
	// PreparationDetail keeps the last preparation failure cause for
	// post-mortem diagnosis. It is persisted but kept off Status so raw internal
	// error strings never reach the read endpoint.
	PreparationDetail string `json:"preparation_detail,omitempty"`
}

func defaultRecord() record {
	return record{Status: Status{Policy: DefaultPolicy(), Supported: true, Preparation: Preparation{State: stateNone}}, Version: 1}
}
