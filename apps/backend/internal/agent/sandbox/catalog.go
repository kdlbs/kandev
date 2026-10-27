package sandbox

import (
	"context"
	"strings"
	"unicode"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

const policyCatalogPageSize = 64

type PolicyCatalog struct {
	Policies   []*Revision `json:"policies"`
	NextCursor string      `json:"next_cursor,omitempty"`
}

// ListPolicies returns current revisions in the operator's requested workspace scope.
func (s *Service) ListPolicies(ctx context.Context, workspaceID, after string) (*PolicyCatalog, error) {
	identity, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	if len(workspaceID) > 128 || len(after) > 128 || strings.ContainsFunc(after, unicode.IsControl) {
		return nil, contract.ErrInvalidPolicy
	}
	if err = s.workspaceAccess(ctx, workspaceID); err != nil {
		return nil, ErrNotFound
	}
	ids, err := s.catalogRevisionIDs(ctx, identity.OrgID, workspaceID, after)
	if err != nil {
		return nil, err
	}
	more := len(ids) > policyCatalogPageSize
	if more {
		ids = ids[:policyCatalogPageSize]
	}
	result := &PolicyCatalog{Policies: make([]*Revision, 0, len(ids))}
	for _, id := range ids {
		revision, err := s.GetRevision(ctx, id)
		if err != nil {
			return nil, err
		}
		result.Policies = append(result.Policies, revision)
	}
	if more {
		result.NextCursor = result.Policies[len(result.Policies)-1].Policy.ID
	}
	return result, nil
}

func (s *Service) catalogRevisionIDs(ctx context.Context, orgID, workspaceID, after string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(`SELECT r.id FROM sandbox_policy_heads h
 JOIN sandbox_policy_revisions r ON r.org_id=h.org_id AND r.policy_id=h.policy_id AND r.revision=h.revision
 WHERE h.org_id=? AND (h.workspace_id='' OR h.workspace_id=?) AND h.policy_id>?
 ORDER BY h.policy_id LIMIT ?`), orgID, workspaceID, after, policyCatalogPageSize+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
