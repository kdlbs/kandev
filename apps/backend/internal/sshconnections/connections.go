package sshconnections

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"strings"
)

type SaveConnectionRequest struct {
	RequestID       string     `json:"request_id"`
	Connection      Connection `json:"connection"`
	ExpectedVersion int64      `json:"expected_version"`
}

func (s *Service) SaveConnection(ctx context.Context, req SaveConnectionRequest) (Route, error) {
	actor, err := s.authorize(ctx)
	if err != nil {
		return Route{}, err
	}
	c, err := normalizeConnection(req.Connection)
	if err != nil {
		return Route{}, err
	}
	var out Route
	err = s.store.write(ctx, func(tx *sqlx.Tx) error {
		if req.ExpectedVersion == 0 {
			prior, err := createConnectionRequest(ctx, tx, actor, req.RequestID, &c)
			if err != nil {
				return err
			}
			if prior {
				out, err = routeIn(ctx, tx, c.ID)
				return err
			}
		} else {
			if _, err := uuid.Parse(c.ID); err != nil {
				return ErrInvalidRoute
			}
		}
		if c.IdentityID != "" {
			if _, err := identityIn(ctx, tx, c.IdentityID); err != nil {
				return ErrInvalidIdentity
			}
		}
		if err := saveConnectionIn(ctx, tx, &c, req.ExpectedVersion); err != nil {
			return err
		}
		if err := audit(ctx, tx, actor, c.ID, "connection.save", "saved"); err != nil {
			return err
		}
		var err error
		out, err = routeIn(ctx, tx, c.ID)
		return err
	})
	return out, err
}
func createConnectionRequest(ctx context.Context, tx *sqlx.Tx, actor, requestID string, c *Connection) (bool, error) {
	if _, err := uuid.Parse(requestID); err != nil || c.ID != "" {
		return false, ErrInvalidRoute
	}
	data, _ := json.Marshal(c)
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	var prior []struct {
		Digest string `db:"digest"`
		ID     string `db:"connection_id"`
	}
	if err := tx.SelectContext(ctx, &prior, tx.Rebind(`SELECT digest,connection_id FROM ssh_connection_requests WHERE actor=? AND request_id=?`), actor, requestID); err != nil {
		return false, err
	}
	if len(prior) > 0 {
		if prior[0].Digest != digest {
			return false, ErrConflict
		}
		c.ID = prior[0].ID
		return true, nil
	}
	c.ID = uuid.NewString()
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_connection_requests(actor,request_id,digest,connection_id) VALUES (?,?,?,?)`), actor, requestID, digest, c.ID)
	return false, err
}

func normalizeConnection(c Connection) (Connection, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.Host = strings.TrimSpace(c.Host)
	c.User = strings.TrimSpace(c.User)
	if c.Name == "" || len(c.Name) > 200 || len(c.Host) > 253 || len(c.User) > 256 || c.Port < 0 || c.Port > 65535 || strings.ContainsAny(c.Host+c.User, "\r\n\x00") {
		return Connection{}, ErrInvalidRoute
	}
	return c, nil
}
