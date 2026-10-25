// Package sandbox owns sandbox policy authority, revisions, and launch admission.
package sandbox

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

var (
	ErrDisabled   = errors.New("sandbox policies are disabled")
	ErrAuthority  = errors.New("sandbox policy requires an authenticated human operator")
	ErrConflict   = errors.New("sandbox policy revision changed")
	ErrNotFound   = errors.New("sandbox policy was not found")
	ErrIneligible = errors.New("sandbox policy is not eligible for this executor")
)

type Source struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Digest string `json:"digest"`
}

type Policy struct {
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspace_id"`
	Name           string          `json:"name"`
	InstallationID string          `json:"installation_id"`
	ProviderKey    string          `json:"provider_key"`
	Grants         contract.Grants `json:"grants"`
	Sources        []Source        `json:"sources"`
}

type Revision struct {
	ID                 string              `json:"id"`
	Number             int64               `json:"number"`
	SchemaVersion      int                 `json:"schema_version"`
	Digest             string              `json:"digest"`
	Policy             Policy              `json:"policy"`
	OperatorID         string              `json:"operator_id"`
	OrgID              string              `json:"org_id"`
	CreatedAt          time.Time           `json:"created_at"`
	Revoked            bool                `json:"revoked"`
	LearningProposalID string              `json:"learning_proposal_id,omitempty"`
	Learning           *LearningProvenance `json:"learning,omitempty"`
}

type Service struct {
	attemptChanges     attemptChangeNotifier
	approvalChanges    approvalChangeNotifier
	availabilityCheck  func(context.Context, *Resolved) error
	providerCatalog    func(context.Context) ([]ProviderView, error)
	approvalGuard      func(context.Context, *Attempt, *Approval) error
	compileLearning    LearningCompiler
	refreshReport      func(context.Context, string, string) (*Report, error)
	db                 *sqlx.DB
	enabled            bool
	protectedRoots     []string
	authorizeWorkspace func(context.Context, string) error
	authorizeSession   func(context.Context, string) error
	resolveTarget      func(context.Context, string, string) (TargetScope, error)
}

func NewService(database *sqlx.DB, enabled bool, protectedRoots []string) (*Service, error) {
	s := &Service{db: database, enabled: enabled, protectedRoots: append([]string(nil), protectedRoots...)}
	if err := s.initSchema(); err != nil {
		return nil, err
	}
	return s, nil
}
