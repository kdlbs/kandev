package sshconnections

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/sshtransport"
	"golang.org/x/crypto/ssh"
	"strings"
	"time"
)

type EnrollmentRequest struct {
	RequestID           string `json:"request_id"`
	Stamp               string `json:"expected_stamp"`
	Fingerprint         string `json:"key_fingerprint"`
	Mode                string `json:"mode"`
	Password            string `json:"password,omitempty"`
	BootstrapIdentityID string `json:"bootstrap_identity_id,omitempty"`
}
type Enrollment struct {
	ErrorCode    string    `json:"error_code,omitempty"`
	Hop          string    `json:"hop,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	RequestID    string    `json:"request_id"`
	ConnectionID string    `json:"connection_id"`
	Stamp        string    `json:"stamp"`
	Fingerprint  string    `json:"key_fingerprint"`
	State        string    `json:"state"`
}

func (s *Service) Enroll(ctx context.Context, id string, req EnrollmentRequest) (Enrollment, error) {
	actor, err := s.authorize(ctx)
	if err != nil {
		return Enrollment{}, err
	}
	if _, err = uuid.Parse(req.RequestID); err != nil {
		return Enrollment{}, ErrInvalidRoute
	}
	result, route, publicKey, started, err := s.startEnrollment(ctx, actor, id, req)
	if err != nil || !started {
		return result, err
	}
	operationCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var failure *ProbeFailure
	result.State, failure = s.runEnrollment(operationCtx, route, publicKey, req)
	if failure != nil {
		result.ErrorCode, result.Hop = failure.Code, failure.Hop
	}
	// Cancellation cannot erase the durable outcome of an operation already sent.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	err = s.store.write(finishCtx, func(tx *sqlx.Tx) error {
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(finishCtx, tx.Rebind(`UPDATE ssh_enrollments SET data=? WHERE actor=? AND request_id=?`), string(data), actor, req.RequestID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(finishCtx, tx.Rebind(`DELETE FROM ssh_connection_refs WHERE owner_type='enrollment' AND owner_id=?`), actor+":"+req.RequestID); err != nil {
			return err
		}
		return audit(finishCtx, tx, actor, id, "connection.enroll", result.State)
	})
	return result, err
}
func enrollmentDigest(id string, req EnrollmentRequest) string {
	data, _ := json.Marshal([]string{id, req.Stamp, req.Fingerprint, req.Mode, req.BootstrapIdentityID})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (s *Service) startEnrollment(ctx context.Context, actor, id string, req EnrollmentRequest) (Enrollment, Route, string, bool, error) {
	var result Enrollment
	var route Route
	var publicKey string
	started := false
	digest := enrollmentDigest(id, req)
	err := s.store.write(ctx, func(tx *sqlx.Tx) error {
		prior, found, err := priorEnrollmentIn(ctx, tx, actor, req.RequestID, digest)
		if err != nil {
			return err
		}
		if found {
			result = prior
			return nil
		}

		route, err = routeIn(ctx, tx, id)
		if err != nil {
			return err
		}
		publicKey, err = enrollmentKeyIn(ctx, tx, route, req)
		if err != nil {
			return err
		}
		if err = validateBootstrap(ctx, tx, req); err != nil {
			return err
		}
		var active int
		if err = tx.GetContext(ctx, &active, tx.Rebind(`SELECT COUNT(*) FROM ssh_connection_refs WHERE owner_type='enrollment' AND connection_id=?`), id); err != nil {
			return err
		}
		if active > 0 {
			return ErrInUse
		}
		result = Enrollment{RequestID: req.RequestID, ConnectionID: id, Stamp: req.Stamp, Fingerprint: req.Fingerprint, State: enrollmentStarted, CreatedAt: time.Now().UTC()}
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_enrollments(actor,request_id,connection_id,digest,data) VALUES (?,?,?,?,?)`), actor, req.RequestID, id, digest, string(data)); err != nil {
			return err
		}
		if err = audit(ctx, tx, actor, id, "connection.enroll", enrollmentStarted); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_connection_refs(owner_type,owner_id,connection_id,route_stamp) VALUES ('enrollment',?,?,?)`), actor+":"+req.RequestID, id, req.Stamp); err != nil {
			return err
		}
		started = true
		return nil
	})
	return result, route, publicKey, started, err
}
func validateBootstrap(ctx context.Context, tx *sqlx.Tx, req EnrollmentRequest) error {
	switch req.Mode {
	case enrollmentPassword:
		if req.Password == "" || len(req.Password) > 65536 || req.BootstrapIdentityID != "" {
			return ErrInvalidIdentity
		}
	case "identity":
		if req.Password != "" {
			return ErrInvalidIdentity
		}
		if _, err := identityIn(ctx, tx, req.BootstrapIdentityID); err != nil {
			return err
		}
	default:
		return ErrInvalidIdentity
	}
	return nil
}
func (s *Service) runEnrollment(ctx context.Context, route Route, publicKey string, req EnrollmentRequest) (string, *ProbeFailure) {
	b, cleanup, err := s.bastionHop(ctx, route)
	if err != nil {
		failure := transportFailure(err)
		failure.Hop = hopBastion
		return outcomeFailed, failure
	}
	defer cleanup()
	c := route.Destination
	d := sshtransport.Hop{Host: c.Host, Port: c.Port, User: c.User, Fingerprint: c.Fingerprint}
	if req.Mode == enrollmentPassword {
		d.Auth = []ssh.AuthMethod{ssh.Password(req.Password)}
	} else {
		c.IdentityID = req.BootstrapIdentityID
		var cleanup func()
		d, cleanup, err = s.hop(ctx, c)
		if err != nil {
			return outcomeFailed, transportFailure(err)
		}
		defer cleanup()
	}
	client, err := sshtransport.DialRoute(ctx, d, b)
	if err != nil {
		return outcomeFailed, transportFailure(err)
	}
	defer func() { _ = client.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stop()
	session, err := client.NewSession()
	if err != nil {
		return outcomeFailed, transportFailure(err)
	}
	defer func() { _ = session.Close() }()
	session.Stdin = strings.NewReader(publicKey)
	// The command is fixed, and the only stdin value is the selected public key.
	if err = session.Run(enrollmentScript); err != nil {
		state, code := enrollmentExit(err)
		return state, &ProbeFailure{Code: code, Hop: hopDestination}
	}
	_ = client.Close()
	if _, err = s.TestConnection(ctx, route.Destination.ID, route.Stamp); err != nil {
		return enrollmentInstalledUnverified, transportFailure(err)
	}
	return outcomeVerified, nil
}

func priorEnrollmentIn(ctx context.Context, tx *sqlx.Tx, actor, requestID, digest string) (Enrollment, bool, error) {
	var rows []struct {
		Digest string `db:"digest"`
		Data   string `db:"data"`
	}
	if err := tx.SelectContext(ctx, &rows, tx.Rebind(`SELECT digest,data FROM ssh_enrollments WHERE actor=? AND request_id=?`), actor, requestID); err != nil {
		return Enrollment{}, false, err
	}
	if len(rows) == 0 {
		return Enrollment{}, false, nil
	}
	if rows[0].Digest != digest {
		return Enrollment{}, false, ErrConflict
	}
	var result Enrollment
	err := json.Unmarshal([]byte(rows[0].Data), &result)
	return result, true, err
}

func enrollmentKeyIn(ctx context.Context, tx *sqlx.Tx, route Route, req EnrollmentRequest) (string, error) {
	if route.Stamp != req.Stamp {
		return "", ErrConflict
	}
	if route.Destination.Fingerprint == "" {
		return "", ErrTrust
	}
	identity, err := identityIn(ctx, tx, route.Destination.IdentityID)
	if err != nil {
		return "", err
	}
	if identity.PublicKey == "" || identity.Fingerprint != req.Fingerprint {
		return "", ErrInvalidIdentity
	}
	return identity.PublicKey, nil
}

func enrollmentExit(err error) (string, string) {
	var exit *ssh.ExitError
	if errors.As(err, &exit) {
		switch exit.ExitStatus() {
		case 10:
			return outcomeFailed, "ssh_identity_invalid"
		case 11:
			return outcomeFailed, "ssh_enrollment_account_unavailable"
		case 12:
			return outcomeFailed, "ssh_enrollment_unsafe_files"
		case 13:
			return outcomeFailed, "ssh_enrollment_locked"
		}
	}
	return outcomeUnknown, "ssh_operation_failed"
}
