package sshconnections

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/jmoiron/sqlx"
)

type Route struct {
	LatestEnrollment *Enrollment `json:"latest_enrollment,omitempty"`
	Destination      Connection  `json:"destination"`
	Bastion          *Connection `json:"bastion,omitempty"`
	Stamp            string      `json:"stamp"`
}

func routeIn(ctx context.Context, tx *sqlx.Tx, id string) (Route, error) {
	d, err := connectionIn(ctx, tx, id)
	if err != nil {
		return Route{}, err
	}
	route := Route{Destination: d}
	parts := []any{d.ID, d.Revision}
	if d.BastionID != "" {
		b, err := connectionIn(ctx, tx, d.BastionID)
		if err != nil {
			return Route{}, err
		}
		if b.BastionID != "" || b.ID == d.ID {
			return Route{}, ErrInvalidRoute
		}
		route.Bastion = &b
		parts = append(parts, b.ID, b.Revision)
	}
	data, _ := json.Marshal(parts)
	hash := sha256.Sum256(data)
	route.Stamp = hex.EncodeToString(hash[:])
	route.LatestEnrollment, err = latestEnrollmentIn(ctx, tx, id)
	return route, err
}

func latestEnrollmentIn(ctx context.Context, tx *sqlx.Tx, id string) (*Enrollment, error) {
	var rows []string
	if err := tx.SelectContext(ctx, &rows, tx.Rebind(`SELECT data FROM ssh_enrollments WHERE connection_id=?`), id); err != nil {
		return nil, err
	}
	var latest *Enrollment
	for _, data := range rows {
		var result Enrollment
		if err := json.Unmarshal([]byte(data), &result); err != nil {
			return nil, err
		}
		if latest == nil || result.CreatedAt.After(latest.CreatedAt) {
			latest = &result
		}
	}
	return latest, nil
}
