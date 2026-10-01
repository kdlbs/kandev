package replay

import (
	"context"
	"errors"
	"time"

	commoncosts "github.com/kandev/kandev/internal/common/costs"
)

var (
	// ErrNotFound is returned by Cases when a row or text does not exist; every
	// other error is a failed read.
	ErrNotFound = errors.New("replay: not found")
	// ErrNoTarget is returned by Instructions.Render when the candidate names
	// nothing to apply to, or is of a kind that is never replayed.
	ErrNoTarget = errors.New("replay: candidate has no target")
	// ErrReplayRunning is returned by Run when the dream item already has a
	// running row.
	ErrReplayRunning = errors.New("replay: already running")
	// ErrResultNotStored is returned by Run when the final write failed or
	// matched no row; the Result is still valid but must not be stored as a
	// verdict.
	ErrResultNotStored = errors.New("replay: result not stored")
)

// Outcome is the decided state of one proposal of a turn.
type Outcome struct {
	ProposalID   string
	Decision     string
	Automatic    bool
	EditedFields []string
}

// Turn is one selected ledger turn with its outcome rows.
type Turn struct {
	ID           string
	Trigger      string
	WakeKinds    []string
	SnapshotHash string
	Outcomes     []Outcome
}

// Proposal is a proposal as the coordinator made it.
type Proposal struct {
	Kind         string
	TargetTaskID string
	WorkflowID   string
	Title        string
}

// Cases reads the recorded past. A case id is the ledger turn id.
type Cases interface {
	// SelectTurns returns the turns to replay in running order: the newest
	// decided turns, then the override turns not already selected.
	SelectTurns(ctx context.Context, coordinatorID string, now time.Time) ([]Turn, error)
	Proposal(ctx context.Context, proposalID string) (Proposal, error)
	Snapshot(ctx context.Context, hash string) (string, error)
	// TriggerText is the text of a message turn's trigger.
	TriggerText(ctx context.Context, turn Turn) (string, error)
	TaskTitle(ctx context.Context, taskID string) (string, error)
}

// Profile is the coordinator's agent profile as the replay may use it.
type Profile struct {
	ID           string
	Model        string
	AutoApprove  bool
	Prefix       string
	EnabledFlags []string
}

// Profiles resolves the coordinator's own agent profile.
type Profiles interface {
	Resolve(ctx context.Context, coordinatorID string) (Profile, error)
}

// Reply is one model answer with the tokens the executor reported, if any.
type Reply struct {
	Text           string
	PromptTokens   int64
	ResponseTokens int64
}

// Prompts runs one sessionless prompt.
type Prompts interface {
	Run(ctx context.Context, profileID, prompt string) (Reply, error)
}

// Prices looks up a model's price.
type Prices interface {
	Lookup(ctx context.Context, model string) (commoncosts.ModelPricing, bool, error)
}

// SpendReading is the coordinator's 24 hour spend. CeilingSubcents is nil when
// no ceiling is set.
type SpendReading struct {
	WindowSubcents  int64
	Measurable      bool
	CeilingSubcents *int64
}

// Spend reads the coordinator's spend, replay rows included.
type Spend interface {
	Reading(ctx context.Context, coordinatorID string, now time.Time) (SpendReading, error)
}

// Override is a candidate as the renderer applies it.
type Override struct {
	Kind     string
	Text     string
	TargetID string
}

// Renders are the baseline and candidate instruction texts, read from one
// snapshot of the coordinator's context and orders.
type Renders struct {
	BaselineText  string
	BaselineHash  string
	CandidateText string
	CandidateHash string
}

// Instructions renders the instruction text of both sides.
type Instructions interface {
	Render(ctx context.Context, coordinatorID string, o Override) (Renders, error)
}

// Clock is the harness's time source.
type Clock interface{ Now() time.Time }

// Stored is a result row as the store returns it.
type Stored struct {
	ID     string
	Status string
	Result Result
}

// NewRow is what is known when a replay starts.
type NewRow struct {
	CoordinatorID string
	DreamID       string
	ItemID        string
	PromptVersion string
	CreatedAt     time.Time
}

// Results is the result-row store.
type Results interface {
	// Insert writes a running row. When the unique (dream, item) key is already
	// held it returns the held row and inserted false.
	Insert(ctx context.Context, n NewRow) (Stored, bool, error)
	// AddCost adds subcents to the row's cost in one statement.
	AddCost(ctx context.Context, rowID string, subcents int64) error
	// Finish writes the rest of the row and marks it done, only while it is
	// running; it reports whether a row matched.
	Finish(ctx context.Context, rowID string, r Result, finishedAt time.Time) (bool, error)
	// BaselineAttempts returns, per case id, the successful baseline attempts
	// of the newest finished row of the coordinator that holds that case under
	// the key, ignoring rows settled interrupted or cancelled.
	BaselineAttempts(ctx context.Context, coordinatorID, baselineHash, model, promptVersion string) (map[string][]Attempt, error)
}

// Deps are the harness's dependencies; none may be nil.
type Deps struct {
	Cases        Cases
	Profiles     Profiles
	Prompts      Prompts
	Prices       Prices
	Spend        Spend
	Instructions Instructions
	Results      Results
	Clock        Clock
}

// Candidate is one change to the coordinator's instructions.
type Candidate struct {
	Kind         string
	Text         string
	TargetID     string
	CitedTurnIDs []string
}

// Request is one replay.
type Request struct {
	CoordinatorID string
	Candidate     Candidate
	DreamID       string
	ItemID        string
}

// Attempt is one model call over one case on one side.
type Attempt struct {
	Index int      `json:"index"`
	OK    bool     `json:"ok"`
	Keys  []string `json:"keys,omitempty"`
}

// CaseRecord is the per-case data of a replay.
type CaseRecord struct {
	TurnID    string    `json:"turn_id"`
	Skip      string    `json:"skip,omitempty"`
	Notes     []string  `json:"notes,omitempty"`
	Candidate []Attempt `json:"candidate,omitempty"`
	Baseline  []Attempt `json:"baseline,omitempty"`
}

// Flip is an expected reproduced proposal the baseline reproduces and the
// candidate does not.
type Flip struct {
	TurnID     string `json:"turn_id"`
	ProposalID string `json:"proposal_id"`
}

// Skip is a turn left out of every score.
type Skip struct {
	TurnID string `json:"turn_id"`
	Reason string `json:"reason"`
}

// Result is what a replay found. Scores are in thousandths and nil when the
// replay could not measure them.
type Result struct {
	RowID                 string
	Guard                 string
	Verdict               string
	Reason                string
	CandidateHash         string
	BaselineHash          string
	Model                 string
	CandidateScore        *int64
	BaselineScore         *int64
	HeldOutCandidateScore *int64
	HeldOutBaselineScore  *int64
	Flips                 []Flip
	CasesRanCandidate     int
	CasesRanBaseline      int
	CasesCompared         int
	HeldOutCompared       int
	CitedTurnIDs          []string
	Cases                 []CaseRecord
	Skipped               []Skip
	UnmatchedCandidate    int
	UnmatchedBaseline     int
	CostSubcents          int64
	// Notes are the conditions the caller logs and counts.
	Notes []string
}
