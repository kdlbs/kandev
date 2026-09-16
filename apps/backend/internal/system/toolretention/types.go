package toolretention

import "time"

type Preparation struct {
	State  string `json:"state"`
	Choice string `json:"choice"`
	Error  string `json:"error,omitempty"`
}

type Operation struct {
	ID               string           `json:"id"`
	Kind             string           `json:"kind"`
	State            string           `json:"state"`
	Age              Age              `json:"age"`
	Scanned          int64            `json:"scanned"`
	EligibleTasks    int64            `json:"eligible_tasks"`
	EligibleMessages int64            `json:"eligible_messages"`
	RemovedMessages  int64            `json:"removed_messages"`
	PayloadBytes     int64            `json:"payload_bytes"`
	Skipped          map[string]int64 `json:"skipped"`
	Cutoff           time.Time        `json:"cutoff"`
	StartedAt        time.Time        `json:"started_at"`
	FinishedAt       *time.Time       `json:"finished_at,omitempty"`
	Error            string           `json:"error,omitempty"`
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
	SchemaVersion int    `json:"schema_version"`
	Task          string `json:"task"`
	TaskAfter     string `json:"task_after"`
	Session       string `json:"session"`
	SessionAfter  string `json:"session_after"`
	Message       int64  `json:"message"`
	UpperTask     string `json:"upper_task"`
	UpperMessage  int64  `json:"upper_message"`
	TaskCounted   bool   `json:"task_counted"`
	Revision      int64  `json:"revision"`
	Started       bool   `json:"started"`
}

type record struct {
	Status
	Version          int      `json:"version"`
	ApprovedRevision int64    `json:"approved_revision"`
	Receipt          string   `json:"receipt,omitempty"`
	FirstMutation    bool     `json:"first_mutation"`
	Progress         progress `json:"progress"`
}

func defaultRecord() record {
	return record{Status: Status{Policy: DefaultPolicy(), Supported: true, Preparation: Preparation{State: stateNone}}, Version: 1}
}
