package sshconnections

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/ssh"
	"path/filepath"
	"strings"
)

var ErrInvalidIdentity = errors.New("ssh_identity_invalid")

func identityIn(ctx context.Context, tx *sqlx.Tx, id string) (Identity, error) {
	var data string
	if err := tx.GetContext(ctx, &data, tx.Rebind(`SELECT data FROM ssh_identities WHERE id=?`), id); err != nil {
		return Identity{}, ErrNotFound
	}
	var identity Identity
	err := json.Unmarshal([]byte(data), &identity)
	return identity, err
}
func audit(ctx context.Context, tx *sqlx.Tx, actor, entity, operation, result string) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_connection_audit(id,actor,entity_id,operation,result) VALUES (?,?,?,?,?)`), uuid.NewString(), actor, entity, operation, result)
	return err
}

func (s *Service) CreateIdentity(ctx context.Context, req CreateIdentityRequest) (Identity, error) {
	actor, err := s.authorize(ctx)
	if err != nil {
		return Identity{}, err
	}
	if _, err = uuid.Parse(req.RequestID); err != nil || strings.TrimSpace(req.Label) == "" || len(req.Label) > 200 {
		return Identity{}, ErrInvalidIdentity
	}
	input, _ := json.Marshal([]string{req.Label, req.Kind, req.Reference})
	sum := sha256.Sum256(input)
	digest := hex.EncodeToString(sum[:])
	var out Identity
	err = s.store.write(ctx, func(tx *sqlx.Tx) error {
		var prior []struct {
			Digest string `db:"digest"`
			ID     string `db:"identity_id"`
		}
		if err := tx.SelectContext(ctx, &prior, tx.Rebind(`SELECT digest,identity_id FROM ssh_identity_requests WHERE actor=? AND request_id=?`), actor, req.RequestID); err != nil {
			return err
		}
		if len(prior) > 0 {
			if prior[0].Digest != digest {
				return ErrConflict
			}
			var err error
			out, err = identityIn(ctx, tx, prior[0].ID)
			return err
		}
		identity, key, err := makeIdentity(req)
		if err != nil {
			return err
		}
		defer clear(key)
		out = identity
		if len(key) > 0 {
			if err = s.vault.Create(ctx, tx, "ssh-identity:"+out.ID, key); err != nil {
				return err
			}
		}
		data, err := json.Marshal(out)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_identities(id,data) VALUES (?,?)`), out.ID, string(data)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_identity_requests(actor,request_id,digest,identity_id) VALUES (?,?,?,?)`), actor, req.RequestID, digest, out.ID); err != nil {
			return err
		}
		return audit(ctx, tx, actor, out.ID, "identity.create", "created")
	})
	return out, err
}

func makeIdentity(req CreateIdentityRequest) (Identity, []byte, error) {
	out := Identity{ID: uuid.NewString(), Label: strings.TrimSpace(req.Label), Kind: identityManaged, Generation: uuid.NewString(), Version: 1}
	if req.Kind == identityFile || req.Kind == identityAgent {
		if !filepath.IsAbs(req.Reference) || req.PrivateKey != "" || req.Passphrase != "" {
			return Identity{}, nil, ErrInvalidIdentity
		}
		out.Kind = req.Kind
		out.Reference = filepath.Clean(req.Reference)
		return out, nil, nil
	}
	private, err := identityKey(req)
	if err != nil {
		return Identity{}, nil, err
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		return Identity{}, nil, err
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		return Identity{}, nil, err
	}
	out.PublicKey = string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	out.Fingerprint = ssh.FingerprintSHA256(signer.PublicKey())
	return out, pem.EncodeToMemory(block), nil
}

func identityKey(req CreateIdentityRequest) (any, error) {
	switch req.Kind {
	case identityGenerate:
		if req.PrivateKey != "" || req.Passphrase != "" || req.Reference != "" {
			return nil, ErrInvalidIdentity
		}
		_, key, err := ed25519.GenerateKey(rand.Reader)
		return key, err
	case identityImport:
		return importIdentityKey(req)
	}
	return nil, ErrInvalidIdentity
}

func importIdentityKey(req CreateIdentityRequest) (any, error) {
	if len(req.PrivateKey) == 0 || len(req.PrivateKey) > 65536 || len(req.Passphrase) > 65536 || req.Reference != "" {
		return nil, ErrInvalidIdentity
	}
	material := strings.TrimSpace(req.PrivateKey)
	block, rest := pem.Decode([]byte(material))
	if block == nil || strings.TrimSpace(string(rest)) != "" || !strings.HasPrefix(material, "-----BEGIN "+block.Type+"-----") {
		return nil, ErrInvalidIdentity
	}
	var key any
	var err error
	if req.Passphrase != "" {
		key, err = ssh.ParseRawPrivateKeyWithPassphrase([]byte(req.PrivateKey), []byte(req.Passphrase))
	} else {
		key, err = ssh.ParseRawPrivateKey([]byte(req.PrivateKey))
	}
	if err != nil {
		return nil, ErrInvalidIdentity
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, ErrInvalidIdentity
	}
	switch signer.PublicKey().Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoRSA, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521:
		return key, nil
	}
	return nil, ErrInvalidIdentity
}
