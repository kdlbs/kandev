// Package orgunit owns the tree an organization is arranged into.
//
// A unit holds child units, workspaces, and members. A department and a team
// are both units and differ only in depth, which is why there is one type here
// rather than one per level: an organization that grows a layer moves rows
// instead of migrating a schema.
//
// Membership on a unit reaches every workspace beneath it. Resolving that
// reach belongs to internal/authz; this package owns the tree and its
// invariants. See docs/specs/workspaces/requirements/org-units.md.
package orgunit

import (
	"errors"
	"time"
)

// Kind separates the units that carry structure from the two that the system
// creates and protects.
type Kind string

const (
	// KindRoot is the single top unit of an organization. Belonging to the
	// organization is membership of this unit, so a workspace parked here is
	// reachable by everyone in the organization.
	KindRoot Kind = "root"
	// KindStandard is a department, a team, or any other level a person makes.
	KindStandard Kind = "standard"
	// KindPersonal belongs to exactly one user and takes no members. It is
	// where work that is nobody else's business lives, and it replaces the
	// private visibility flag rather than sitting beside it.
	KindPersonal Kind = "personal"
)

// Unit is one node of an organization's tree.
type Unit struct {
	ID          string `json:"id" db:"id"`
	OrgID       string `json:"org_id" db:"org_id"`
	ParentID    string `json:"parent_id" db:"parent_id"`
	Kind        Kind   `json:"kind" db:"kind"`
	OwnerUserID string `json:"owner_user_id,omitempty" db:"owner_user_id"`
	Name        string `json:"name" db:"name"`
	// Path is the materialized ancestor chain, "/<id>/<id>/", ending with this
	// unit. It is a denormalization whose only writer is this package, kept
	// because a prefix match works identically on SQLite and Postgres while a
	// recursive query does not.
	Path      string    `json:"path" db:"path"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// IsProtected reports whether the unit is one the system owns. A protected
// unit cannot be moved or deleted while the thing it belongs to exists.
func (u *Unit) IsProtected() bool { return u.Kind == KindRoot || u.Kind == KindPersonal }

// Member is a person's standing on one unit, which they hold on every
// workspace beneath it.
type Member struct {
	UnitID    string    `json:"unit_id" db:"unit_id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Role      string    `json:"role" db:"role"`
	AddedBy   string    `json:"added_by" db:"added_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Tree errors. Each names its blocking condition so a caller can say what is
// in the way rather than that something went wrong.
var (
	ErrUnitNotFound     = errors.New("unit not found")
	ErrNameRequired     = errors.New("a unit needs a name")
	ErrParentRequired   = errors.New("a unit needs a parent")
	ErrCrossOrgParent   = errors.New("a unit's parent must be in the same organization")
	ErrCycle            = errors.New("a unit cannot be moved beneath itself")
	ErrNotEmpty         = errors.New("this unit still holds child units or workspaces")
	ErrProtectedUnit    = errors.New("the root and personal units cannot be moved or deleted")
	ErrPersonalNoMember = errors.New("a personal unit takes no members")
	ErrRootExists       = errors.New("this organization already has a root unit")
)
