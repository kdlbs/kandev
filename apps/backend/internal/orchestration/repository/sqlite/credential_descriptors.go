package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func affectedOne(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return models.ErrConflict
	}
	return err
}

func (r *Repository) SaveCredentialDescriptor(ctx context.Context, d *models.CredentialDescriptor, expected int64) error {
	d.Revision = expected + 1
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	query := `INSERT INTO orchestration_credential_descriptors(binding_id,id,revision,content_json) VALUES(?,?,?,?)
	ON CONFLICT(binding_id,id) DO UPDATE SET revision=excluded.revision,content_json=excluded.content_json
	WHERE orchestration_credential_descriptors.revision=? AND orchestration_credential_descriptors.forgotten=0`
	// An update must not recreate a missing or forgotten descriptor.
	if expected > 0 {
		query = `UPDATE orchestration_credential_descriptors SET content_json=?,revision=revision+1
		WHERE binding_id=? AND id=? AND revision=? AND forgotten=0`
		result, err := r.db.ExecContext(ctx, r.db.Rebind(query), string(raw), d.BindingID, d.ID, expected)
		return affectedOne(result, err)
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), d.BindingID, d.ID, d.Revision, string(raw), expected)
	return affectedOne(result, err)
}

func (r *Repository) CredentialDescriptors(ctx context.Context, binding string) ([]models.CredentialDescriptor, error) {
	var raw []string
	if err := r.ro.SelectContext(ctx, &raw, r.ro.Rebind(`SELECT content_json FROM orchestration_credential_descriptors
	WHERE binding_id=? AND forgotten=0 ORDER BY id`), binding); err != nil {
		return nil, err
	}
	rows := make([]models.CredentialDescriptor, 0, len(raw))
	for _, v := range raw {
		var d models.CredentialDescriptor
		if err := json.Unmarshal([]byte(v), &d); err != nil {
			return nil, err
		}
		rows = append(rows, d)
	}
	return rows, nil
}

func (r *Repository) ForgetCredentialDescriptor(ctx context.Context, binding, id string, expected int64) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE orchestration_credential_descriptors
	SET forgotten=1,revision=revision+1,content_json='{}' WHERE binding_id=? AND id=? AND revision=? AND forgotten=0`), binding, id, expected)
	return affectedOne(result, err)
}
