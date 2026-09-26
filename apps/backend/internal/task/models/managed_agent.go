package models

import "time"

type ManagedAgentBindingLifecycle string

const (
	ManagedAgentBindingCreating           ManagedAgentBindingLifecycle = "creating"
	ManagedAgentBindingReady              ManagedAgentBindingLifecycle = "ready"
	ManagedAgentBindingArchived           ManagedAgentBindingLifecycle = "archived"
	ManagedAgentBindingTerminationPending ManagedAgentBindingLifecycle = "termination_pending"
)

type ManagedAgentOperationKind string

const (
	ManagedAgentOperationCreate   ManagedAgentOperationKind = "create"
	ManagedAgentOperationFollowup ManagedAgentOperationKind = "followup"
)

type ManagedAgentSubmissionState string

const (
	ManagedAgentSubmissionReserved   ManagedAgentSubmissionState = "reserved"
	ManagedAgentSubmissionSubmitting ManagedAgentSubmissionState = "submitting"
	ManagedAgentSubmissionAccepted   ManagedAgentSubmissionState = "accepted"
	ManagedAgentSubmissionCancelling ManagedAgentSubmissionState = "cancelling"
	ManagedAgentSubmissionUnknown    ManagedAgentSubmissionState = "unknown"
	ManagedAgentSubmissionSucceeded  ManagedAgentSubmissionState = "succeeded"
	ManagedAgentSubmissionFailed     ManagedAgentSubmissionState = "failed"
	ManagedAgentSubmissionCancelled  ManagedAgentSubmissionState = "cancelled"
	ManagedAgentSubmissionRejected   ManagedAgentSubmissionState = "rejected"
	ManagedAgentSubmissionRetryAcked ManagedAgentSubmissionState = "retry_acknowledged"
)

func ManagedAgentOperationActive(state ManagedAgentSubmissionState) bool {
	switch state {
	case ManagedAgentSubmissionReserved,
		ManagedAgentSubmissionSubmitting,
		ManagedAgentSubmissionAccepted,
		ManagedAgentSubmissionCancelling,
		ManagedAgentSubmissionUnknown:
		return true
	default:
		return false
	}
}

func ManagedAgentOperationTerminal(state ManagedAgentSubmissionState) bool {
	switch state {
	case ManagedAgentSubmissionSucceeded, ManagedAgentSubmissionFailed, ManagedAgentSubmissionCancelled:
		return true
	default:
		return false
	}
}

type ManagedAgentLaunchSnapshot struct {
	RepositoryID  string `json:"repositoryId"`
	RepositoryURL string `json:"repositoryUrl"`
	StartingRef   string `json:"startingRef"`
	Model         string `json:"model"`
	CallbackURL   string `json:"callbackUrl"`
	AutoCreatePR  bool   `json:"autoCreatePR"`
}

type ManagedAgentRequestSnapshot struct {
	Prompt        string `json:"prompt"`
	TurnID        string `json:"turnId"`
	MCPMode       string `json:"mcpMode"`
	RepositoryURL string `json:"repositoryUrl"`
	StartingRef   string `json:"startingRef"`
	Model         string `json:"model"`
	CallbackURL   string `json:"callbackUrl"`
	AutoCreatePR  bool   `json:"autoCreatePR"`
}

type ManagedAgentResultSnapshot struct {
	RepositoryID    string `json:"repositoryId"`
	Branch          string `json:"branch,omitempty"`
	PullRequestURL  string `json:"pullRequestUrl,omitempty"`
	AgentURL        string `json:"agentUrl,omitempty"`
	AssistantResult string `json:"assistantResult,omitempty"`
}

type ManagedAgentBinding struct {
	ID                 string
	SessionID          string
	TaskID             string
	WorkspaceID        string
	UserID             string
	ExecutionID        string
	ProviderKind       string
	ExecutorID         string
	ExecutorProfileID  string
	CredentialRef      string
	RemoteAgentID      string
	Lifecycle          ManagedAgentBindingLifecycle
	Launch             ManagedAgentLaunchSnapshot
	Revision           int64
	DispatchGeneration int64
	DispatchOwner      string
	DispatchLeaseUntil *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ManagedAgentOperation struct {
	ID                           string
	BindingID                    string
	PromptTurnID                 string
	Kind                         ManagedAgentOperationKind
	RequestDigest                string
	RequestSnapshot              ManagedAgentRequestSnapshot
	ResultSnapshot               ManagedAgentResultSnapshot
	CompletionPending            bool
	State                        ManagedAgentSubmissionState
	RemoteRunID                  string
	PreSubmitRunID               string
	DispatchGeneration           int64
	Revision                     int64
	CreatedAt                    time.Time
	DispatchStartedAt            *time.Time
	AcceptedAt                   *time.Time
	SettledAt                    *time.Time
	UpdatedAt                    time.Time
	SanitizedError               string
	RetryAcknowledgesOperationID string `json:"-"`
	DuplicationRiskAcknowledged  bool   `json:"-"`
}

type ManagedAgentOperationUpdate struct {
	OperationID             string
	ExpectedRevision        int64
	ExpectedBindingRevision int64
	LeaseOwner              string
	State                   ManagedAgentSubmissionState
	RemoteRunID             string
	PreSubmitRunID          string
	SanitizedError          string
	ResultSnapshot          *ManagedAgentResultSnapshot
	CompletionPending       *bool
}

type ManagedAgentStreamCheckpoint struct {
	BindingID               string
	RemoteRunID             string
	LastEventID             string
	Cursor                  string
	TerminalEventType       string
	HistoryGap              bool
	AssistantMessageStarted bool
	DispatchGeneration      int64
	UpdatedAt               time.Time
}

type ManagedAgentStreamEvent struct {
	BindingID               string
	OperationID             string
	RemoteRunID             string
	EventID                 string
	EventType               string
	Cursor                  string
	DispatchGeneration      int64
	TerminalEventType       string
	HistoryGap              bool
	AppendMessage           bool
	AssistantMessageStarted bool
	Message                 *Message
}

type ManagedAgentToolGrant struct {
	ID          string
	BindingID   string
	OperationID string
	TokenHash   string
	Scope       string
	Generation  int64
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}
