package sshconnections

import (
	"context"
	"encoding/json"
	"github.com/jmoiron/sqlx"
)

// RecoverEnrollments settles interrupted attempts without repeating remote writes.
// Backend composition calls it once at startup, before admitting HTTP requests.
func (s *Service) RecoverEnrollments(ctx context.Context) error {
	if !s.enabled {
		return ErrDisabled
	}
	return s.store.write(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM ssh_connection_refs WHERE owner_type IN ('probe','runtime_pending')`); err != nil {
			return err
		}
		var rows []struct {
			Actor     string `db:"actor"`
			RequestID string `db:"request_id"`
			Data      string `db:"data"`
		}
		if err := tx.SelectContext(ctx, &rows, `SELECT actor,request_id,data FROM ssh_enrollments`); err != nil {
			return err
		}
		for _, row := range rows {
			var result Enrollment
			if err := json.Unmarshal([]byte(row.Data), &result); err != nil {
				return err
			}
			if result.State != enrollmentStarted {
				continue
			}
			result.State = outcomeUnknown
			data, err := json.Marshal(result)
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE ssh_enrollments SET data=? WHERE actor=? AND request_id=?`), string(data), row.Actor, row.RequestID); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM ssh_connection_refs WHERE owner_type='enrollment' AND owner_id=?`), row.Actor+":"+row.RequestID); err != nil {
				return err
			}
			if err = audit(ctx, tx, row.Actor, result.ConnectionID, "connection.enroll", outcomeUnknown); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Service) GetEnrollment(ctx context.Context, id, requestID string) (Enrollment, error) {
	actor, err := s.authorize(ctx)
	if err != nil {
		return Enrollment{}, err
	}
	var data string
	if err = s.store.db.GetContext(ctx, &data, s.store.db.Rebind(`SELECT data FROM ssh_enrollments WHERE actor=? AND request_id=? AND connection_id=?`), actor, requestID, id); err != nil {
		return Enrollment{}, ErrNotFound
	}
	var result Enrollment
	err = json.Unmarshal([]byte(data), &result)
	return result, err
}
