package plugins

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/plugins/store"
)

// UpdateConfig replaces the operator-editable config for id. Incoming
// secret fields carrying the mask placeholder keep their stored value
// (mergeMaskedSecrets), the result is validated against the manifest's
// config_schema (ErrConfigInvalid on mismatch, mapped to 400 by the HTTP
// layer), secret fields are moved into the encrypted vault
// (storeConfigSecrets — the config file persists only a vault reference),
// and a currently-running plugin is restarted so the new config takes
// effect — hostForPlugin rebuilds the Host per spawn, and plugins read
// config at startup via the Host GetConfig RPC.
func (s *Service) UpdateConfig(ctx context.Context, id string, config map[string]any) error {
	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	dispatchLock := s.dispatchLocks.lockFor(id)
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	rec, err := s.Get(id)
	if err != nil {
		return err
	}
	existing, err := s.store.GetConfig(id)
	if err != nil {
		return err
	}
	merged := mergeMaskedSecrets(config, existing, rec.ConfigSchema)
	if err := validateConfigSchema(rec.ID, merged, rec.ConfigSchema); err != nil {
		return err
	}
	stored, removedSecrets, rollbackVault, err := s.storeConfigSecrets(ctx, rec, merged)
	if err != nil {
		return err
	}
	if err := s.store.SetConfig(id, stored); err != nil {
		// The config commit failed, so the still-current config file is
		// unchanged — restore the vault to match it, otherwise a field's
		// unchanged ref would resolve to the new (uncommitted) value and a
		// request reported as failed would have changed effective config.
		if rbErr := rollbackVault(); rbErr != nil {
			return errors.Join(fmt.Errorf("plugins: persist config: %w", err),
				fmt.Errorf("plugins: vault rollback failed, effective config may be inconsistent: %w", rbErr))
		}
		return err
	}
	// Vault entries for removed secret fields are deleted only AFTER the
	// config commit succeeds: a failed SetConfig must never leave the old
	// (still-current) config referencing an already-deleted vault entry. The
	// delete runs on a context detached from the request (like the rollback
	// path), so a client disconnect right after the commit cannot cancel it
	// and orphan the now-unreferenced vault entries.
	s.cleanupRemovedConfigSecrets(context.WithoutCancel(ctx), rec.ID, removedSecrets, existing)
	return s.restartForConfigChange(rec)
}

// errSecretVaultRequired is returned by storeConfigSecrets when a plugin
// declares secret config fields but no vault is wired. It fails closed
// rather than silently persisting the secret in cleartext — production
// always wires the vault (Provide), so this only guards a misconfigured or
// test setup.
var errSecretVaultRequired = errors.New("plugins: a secret vault is required to store secret config fields")

// storeConfigSecrets moves each secret config field's cleartext value into
// the encrypted vault (id pluginConfigSecretID) and replaces it with the
// configVaultRef marker, so <id>.config.yml never persists a cleartext
// secret (validateConfigSchema has already rejected non-string secret
// values, so nothing can slip past the string path here). A field already
// carrying its ref (the mask-merge round trip) is left alone. Secret fields
// absent from merged are returned as removedSecrets for the caller to
// delete from the vault AFTER the config commit — deleting here would leave
// the still-current config pointing at a missing entry if SetConfig then
// failed. When a plugin declares secret fields but no vault is wired, it
// fails closed (errSecretVaultRequired) rather than writing cleartext.
//
// The returned rollback restores every vault entry this call overwrote to
// its prior value (or deletes it if it did not exist before), so the whole
// operation is failure-atomic: a vault.Set failure mid-loop rolls back the
// earlier writes before returning, and the caller runs rollback if the
// subsequent config commit fails — in both cases the vault ends up matching
// the unchanged config file, so a failed request never changes the value a
// still-current ref resolves to. Rollback writes run on a context detached
// from the caller's (context.WithoutCancel), so a request cancelled mid-save
// cannot abort the rollback and leave the vault inconsistent with the
// unchanged config file.
func (s *Service) storeConfigSecrets(
	ctx context.Context, rec *store.Record, merged map[string]any,
) (stored map[string]any, removedSecrets []string, rollback func() error, err error) {
	noRollback := func() error { return nil }
	secretFields := secretPropertyKeys(rec.ConfigSchema)
	if len(secretFields) == 0 {
		return merged, nil, noRollback, nil
	}
	if s.secrets == nil {
		return nil, nil, noRollback, fmt.Errorf("%w (plugin %q)", errSecretVaultRequired, rec.ID)
	}

	out := make(map[string]any, len(merged))
	for k, v := range merged {
		out[k] = v
	}
	rollbackCtx := context.WithoutCancel(ctx)
	var restores []func() error
	runRollback := func() error {
		var errs []error
		for i := len(restores) - 1; i >= 0; i-- {
			if e := restores[i](); e != nil {
				errs = append(errs, e)
			}
		}
		return errors.Join(errs...)
	}
	for field := range secretFields {
		value, present := out[field]
		if !present {
			removedSecrets = append(removedSecrets, field)
			continue
		}
		cleartext, ok := value.(string)
		if !ok || cleartext == "" || isConfigVaultRef(rec.ID, field, value) {
			continue
		}
		vaultID := pluginConfigSecretID(rec.ID, field)
		restore, snapErr := s.vaultRestoreFunc(ctx, rollbackCtx, vaultID)
		if snapErr != nil {
			s.warnIfRollbackFailed(rec.ID, runRollback())
			return nil, nil, noRollback, snapErr
		}
		if err := s.secrets.Set(ctx, vaultID, vaultID, cleartext); err != nil {
			s.warnIfRollbackFailed(rec.ID, runRollback())
			return nil, nil, noRollback, fmt.Errorf("plugins: store secret config field %q: %w", field, err)
		}
		restores = append(restores, restore)
		out[field] = configVaultRef(rec.ID, field)
	}
	return out, removedSecrets, runRollback, nil
}

// warnIfRollbackFailed logs a mid-loop vault rollback failure. A double
// fault (a vault write succeeded, then its rollback also failed) can leave
// earlier fields' vault entries at their new values while the config file is
// unchanged — making a failed request silently change effective config for
// those fields. It is very unlikely (needs a transient vault failure on both
// the write and the compensating write) and uninstall's namespace purge is a
// backstop, but surfacing it makes the inconsistency observable rather than
// silent.
func (s *Service) warnIfRollbackFailed(pluginID string, err error) {
	if err != nil {
		s.log.Warn("plugins: vault rollback failed after a store error; config may be inconsistent",
			zap.String("plugin_id", pluginID), zap.Error(err))
	}
}

// vaultRestoreFunc snapshots vaultID's current value (read on readCtx) and
// returns a closure that restores it (writes on restoreCtx): reset to the
// prior cleartext if the entry existed, or delete it if it did not. Used to
// undo a config-secret write when the config commit that would reference it
// fails. A not-found snapshot means "absent" (rollback deletes what we
// create); any other Reveal error is a genuine backend fault where the prior
// value cannot be determined — it returns an error so the caller aborts
// before writing rather than risk a rollback that deletes a real secret.
// restoreCtx is detached from the request so a cancelled save cannot abort
// the rollback.
func (s *Service) vaultRestoreFunc(readCtx, restoreCtx context.Context, vaultID string) (func() error, error) {
	prior, err := s.secrets.Reveal(readCtx, vaultID)
	switch {
	case err == nil:
		return func() error { return s.secrets.Set(restoreCtx, vaultID, vaultID, prior) }, nil
	case isSecretNotFound(err):
		return func() error {
			if delErr := s.secrets.Delete(restoreCtx, vaultID); delErr != nil && !isSecretNotFound(delErr) {
				return delErr
			}
			return nil
		}, nil
	default:
		return nil, fmt.Errorf("plugins: cannot snapshot secret config field %q for rollback: %w", vaultID, err)
	}
}

// cleanupRemovedConfigSecrets best-effort deletes the vault entries backing
// secret config fields that the just-committed config no longer contains,
// when the previous config actually pointed at them. Runs only after a
// successful SetConfig (see UpdateConfig); a deletion failure leaves an
// orphaned vault entry, which uninstall's namespace purge also sweeps.
func (s *Service) cleanupRemovedConfigSecrets(
	ctx context.Context, pluginID string, removed []string, existing map[string]any,
) {
	for _, field := range removed {
		if !isConfigVaultRef(pluginID, field, existing[field]) {
			continue
		}
		if err := s.secrets.Delete(ctx, pluginConfigSecretID(pluginID, field)); err != nil {
			s.log.Warn("plugins: failed to delete removed secret config field from vault",
				zap.String("plugin_id", pluginID), zap.String("field", field), zap.Error(err))
		}
	}
}

// GetMaskedConfig returns id's stored config with secret values (per the
// manifest's config_schema) replaced by the mask placeholder — the shape
// the operator settings UI is allowed to see.
func (s *Service) GetMaskedConfig(id string) (map[string]any, error) {
	rec, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	config, err := s.store.GetConfig(id)
	if err != nil {
		return nil, err
	}
	return maskSecrets(config, rec.ConfigSchema), nil
}

// restartForConfigChange bounces id's process after a config write so the
// plugin re-reads its config on the fresh spawn. A plugin that is not
// running (disabled, errored, or no runtime wired) is left alone — it will
// pick the config up on its next spawn anyway. The config is already
// persisted by the time this runs; a restart failure transitions the plugin
// to StatusError and is returned so the operator sees that the save
// succeeded but the plugin did not come back up.
func (s *Service) restartForConfigChange(rec *store.Record) error {
	if s.runtime == nil || !s.runtime.Running(rec.ID) {
		return nil
	}
	s.runtime.Stop(rec.ID)
	ctx, cancel := context.WithTimeout(context.Background(), activateStartTimeout)
	defer cancel()
	if err := s.runtime.Start(ctx, rec, s.hostForPlugin); err != nil {
		if setErr := s.setStatusAndDiagnostic(rec.ID, StatusError, err, true); setErr != nil {
			// The restart error stays the returned error (it is the primary
			// signal), but a failed status write means the registry may show
			// StatusActive with no process running — don't lose that.
			s.log.Warn("plugins: could not transition to error status after restart failure",
				zap.String("plugin_id", rec.ID), zap.Error(setErr))
		}
		s.notifyDeliverer()
		return fmt.Errorf("plugins: config saved but restart of %q failed: %w", rec.ID, err)
	}
	return nil
}
