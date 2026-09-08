// Package org owns organizations: the tenant boundary above users.
//
// Every user belongs to exactly one org. That is a deliberate design choice
// rather than a simplification: it makes the tenant a total function of the
// authenticated identity, so no request payload, header or WebSocket frame can
// move a caller between tenants. There is no org switcher and no "acting as
// org" parameter anywhere in the API.
package org

import (
	"regexp"
	"strings"
	"time"
)

// Status is an organization's lifecycle state.
type Status string

const (
	// StatusActive is normal operation.
	StatusActive Status = "active"
	// StatusSuspended fails every session and token closed and stops running
	// executions, while retaining all rows and files. It is reversible and is
	// the intended lever for a billing lapse or an investigation.
	StatusSuspended Status = "suspended"
)

// NormalizeStatus coerces an unknown stored value to suspended: an org whose
// state cannot be read must not keep serving requests.
func NormalizeStatus(value string) Status {
	if Status(value) == StatusActive {
		return StatusActive
	}
	return StatusSuspended
}

// Org is one tenant.
type Org struct {
	ID   string `json:"id" db:"id"`
	Name string `json:"name" db:"name"`
	// Slug is the human-typeable identifier used to confirm destructive
	// operations. Deleting an org requires typing it exactly.
	Slug      string    `json:"slug" db:"slug"`
	Status    string    `json:"status" db:"status"`
	IsDefault bool      `json:"is_default" db:"is_default"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Active reports whether the org may serve requests.
func (o *Org) Active() bool {
	return o != nil && NormalizeStatus(o.Status) == StatusActive
}

var slugInvalid = regexp.MustCompile(`[^a-z0-9-]+`)
var slugDashes = regexp.MustCompile(`-{2,}`)

// Slugify derives a URL- and confirmation-safe slug from a display name.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugInvalid.ReplaceAllString(s, "-")
	s = slugDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "org"
	}
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}
