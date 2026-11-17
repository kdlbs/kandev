package launcher

import (
	"encoding/json"
	"io"
	"time"

	"github.com/kandev/kandev/internal/maintenance"
)

const maintenanceJSONExecuteMode = "execute"

type maintenanceJSONGroup struct {
	Rows               int   `json:"rows"`
	EstimatedBytes     int64 `json:"estimated_bytes"`
	SampleLimitReached bool  `json:"sample_limit_reached"`
}

type maintenanceJSONDeliveryCandidates struct {
	ProjectedPayloads  maintenanceJSONGroup `json:"projected_delivery_payloads"`
	SettledPrompts     maintenanceJSONGroup `json:"settled_delivery_prompts"`
	EventReceipts      maintenanceJSONGroup `json:"retired_delivery_event_receipts"`
	SessionEvidence    maintenanceJSONGroup `json:"retired_delivery_session_evidence"`
	OrphanedIdentities maintenanceJSONGroup `json:"orphaned_delivery_identities"`
}

type maintenanceJSONReport struct {
	GeneratedAt       string `json:"generated_at"`
	DatabaseSizeBytes int64  `json:"database_size_bytes"`
	Candidates        struct {
		GitSnapshots    maintenanceJSONGroup `json:"duplicate_git_snapshots"`
		PlanRevisions   maintenanceJSONGroup `json:"obsolete_plan_revisions"`
		MessagePayloads maintenanceJSONGroup `json:"orphaned_message_payloads"`
	} `json:"candidates"`
	DeliveryRetention struct {
		Available         bool                               `json:"available"`
		UnavailableReason string                             `json:"unavailable_reason"`
		Candidates        *maintenanceJSONDeliveryCandidates `json:"candidates,omitempty"`
	} `json:"delivery_retention"`
	TotalCandidateRows  int   `json:"total_candidate_rows"`
	TotalEstimatedBytes int64 `json:"total_estimated_bytes"`
}

type maintenanceJSONExecution struct {
	BackupPath                      string `json:"backup_path"`
	Partial                         bool   `json:"partial"`
	DeletedGitSnapshots             int    `json:"deleted_git_snapshots"`
	DeletedPlanRevisions            int    `json:"deleted_plan_revisions"`
	DeletedMessagePayloads          int    `json:"deleted_message_payloads"`
	DeletedDeliveryEventPayloads    int    `json:"deleted_delivery_event_payloads"`
	DeletedSettledDeliveryPrompts   int    `json:"deleted_settled_delivery_prompts"`
	RetiredDeliveryEventReceipts    int    `json:"retired_delivery_event_receipts"`
	PurgedRetiredDeliveryRows       int    `json:"purged_retired_delivery_rows"`
	DeliveryOrphanJobsScheduled     int    `json:"delivery_orphan_jobs_scheduled"`
	DeliveryRetirementJobsCompleted int    `json:"delivery_retirement_jobs_completed"`
}

type maintenanceJSONCompaction struct {
	SizeBeforeBytes int64  `json:"size_before_bytes"`
	SizeAfterBytes  int64  `json:"size_after_bytes"`
	RollbackPath    string `json:"rollback_path"`
}

type maintenanceJSONOutcome struct {
	Mode       string                     `json:"mode"`
	Report     maintenanceJSONReport      `json:"report"`
	Execution  *maintenanceJSONExecution  `json:"execution,omitempty"`
	Compaction *maintenanceJSONCompaction `json:"compaction,omitempty"`
}

func maintenanceJSONCandidate(group maintenance.CandidateGroup) maintenanceJSONGroup {
	return maintenanceJSONGroup{group.RowCount, group.EstBytes, group.SampleLimitReached}
}

func maintenanceJSONReportFrom(report maintenance.Report) maintenanceJSONReport {
	var result maintenanceJSONReport
	result.GeneratedAt = report.GeneratedAt.UTC().Format(time.RFC3339)
	result.DatabaseSizeBytes = report.DatabaseSizeBytes
	result.Candidates.GitSnapshots = maintenanceJSONCandidate(report.DuplicateGitSnapshots)
	result.Candidates.PlanRevisions = maintenanceJSONCandidate(report.ObsoletePlanRevisions)
	result.Candidates.MessagePayloads = maintenanceJSONCandidate(report.OrphanedMessagePayloads)
	result.DeliveryRetention.Available = report.DeliveryRetentionAvailable
	result.DeliveryRetention.UnavailableReason = report.DeliveryRetentionUnavailableReason
	if report.DeliveryRetentionAvailable {
		result.DeliveryRetention.Candidates = &maintenanceJSONDeliveryCandidates{
			ProjectedPayloads:  maintenanceJSONCandidate(report.ProjectedDeliveryPayloads),
			SettledPrompts:     maintenanceJSONCandidate(report.SettledDeliveryPrompts),
			EventReceipts:      maintenanceJSONCandidate(report.RetiredDeliveryEventReceipts),
			SessionEvidence:    maintenanceJSONCandidate(report.RetiredDeliverySessionEvidence),
			OrphanedIdentities: maintenanceJSONCandidate(report.OrphanedDeliveryIdentities),
		}
	}
	result.TotalCandidateRows = report.TotalCandidateRows()
	result.TotalEstimatedBytes = report.TotalEstBytes()
	return result
}

func writeMaintenanceJSON(w io.Writer, outcome maintenance.Outcome) error {
	result := maintenanceJSONOutcome{Mode: "dry_run", Report: maintenanceJSONReportFrom(outcome.Report)}
	if outcome.Executed {
		e := outcome.Execution
		result.Mode = maintenanceJSONExecuteMode
		result.Execution = &maintenanceJSONExecution{
			BackupPath: outcome.BackupPath, Partial: e.Partial,
			DeletedGitSnapshots: e.DeletedGitSnapshots, DeletedPlanRevisions: e.DeletedPlanRevisions,
			DeletedMessagePayloads: e.DeletedMessagePayloads, DeletedDeliveryEventPayloads: e.DeletedDeliveryEventPayloads,
			DeletedSettledDeliveryPrompts: e.DeletedSettledDeliveryPrompts, RetiredDeliveryEventReceipts: e.RetiredDeliveryEventReceipts,
			PurgedRetiredDeliveryRows: e.PurgedRetiredDeliveryRows, DeliveryOrphanJobsScheduled: e.DeliveryOrphanJobsScheduled,
			DeliveryRetirementJobsCompleted: e.DeliveryRetirementJobsCompleted,
		}
	}
	if c := outcome.Compaction; c != nil {
		result.Compaction = &maintenanceJSONCompaction{c.SizeBeforeBytes, c.SizeAfterBytes, c.RollbackPath}
	}
	return json.NewEncoder(maintenanceJSONWriter{w}).Encode(result)
}

// maintenanceJSONWriter treats incomplete writes as failures even if the writer
// does not supply the error required by io.Writer's contract.
type maintenanceJSONWriter struct{ io.Writer }

func (w maintenanceJSONWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if n != len(p) && err == nil {
		err = io.ErrShortWrite
	}
	return n, err
}
