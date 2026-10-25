package sandbox

import (
	"context"
	"time"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

const learningScopeWorkspace = "workspace"

type LearningRequest struct {
	ReportDigest string   `json:"report_digest"`
	RecordIDs    []string `json:"record_ids"`
	Name         string   `json:"name"`
	Scope        string   `json:"scope"`
}

type LearningProposal struct {
	ID                  string          `json:"id"`
	SchemaVersion       int             `json:"schema_version"`
	AttemptID           string          `json:"attempt_id"`
	SessionID           string          `json:"session_id"`
	ReportDigest        string          `json:"report_digest"`
	ReportSources       []ReportSource  `json:"report_sources"`
	ReportIntegrity     string          `json:"report_integrity"`
	ReportComplete      bool            `json:"report_complete"`
	ReportVersion       int64           `json:"report_version"`
	BaseRevisionID      string          `json:"base_revision_id"`
	BaseDigest          string          `json:"base_digest"`
	CeilingRevisionID   string          `json:"ceiling_revision_id"`
	CeilingDigest       string          `json:"ceiling_digest"`
	Provider            AttemptProvider `json:"provider"`
	ExecutorID          string          `json:"executor_id"`
	BindingVersion      int64           `json:"binding_version"`
	SelectedRecords     []DenialRecord  `json:"selected_records"`
	AddedGrants         contract.Grants `json:"added_grants"`
	Policy              Policy          `json:"policy"`
	OperatorID          string          `json:"operator_id"`
	CreatedAt           time.Time       `json:"created_at"`
	Digest              string          `json:"digest"`
	PublishedRevisionID string          `json:"published_revision_id,omitempty"`
}

type LearningCompiler func(context.Context, *Attempt, []DenialRecord) (contract.Grants, error)

// SetLearningCompiler binds the installed provider's bounded proposal operation at startup.
func (s *Service) SetLearningCompiler(compile LearningCompiler) { s.compileLearning = compile }

// LearningProvenance remains in the immutable revision when a session is deleted.
type LearningProvenance struct {
	Sources        []ReportSource  `json:"sources"`
	Integrity      string          `json:"integrity"`
	Complete       bool            `json:"complete"`
	ProposalDigest string          `json:"proposal_digest"`
	ReportDigest   string          `json:"report_digest"`
	Base           Source          `json:"base"`
	Ceiling        Source          `json:"ceiling"`
	Provider       AttemptProvider `json:"provider"`
	Records        []DenialRecord  `json:"records"`
}
