package process

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/passthroughconfig"
)

func (o *PassthroughOwner) fileOwnerPrefix() string {
	return o.manager.cfg.InstanceID + ":" + o.manager.cfg.SessionID + ":"
}

func (o *PassthroughOwner) Prepare(ctx context.Context, request types.PassthroughPrepareRequest) error {
	o.manager.startMu.Lock()
	defer o.manager.startMu.Unlock()
	ctx, release, err := o.manager.BeginOwnedOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
	snapshot := o.Snapshot()
	if request.Generation == 0 || request.Generation != snapshot.Generation+1 {
		return ErrPassthroughConflict
	}
	if len(request.Files) > 32 {
		return errors.New("too many MCP configuration files")
	}
	cfg := o.manager.cfg
	if cfg.SessionID == "" || filepath.Base(cfg.SessionID) != cfg.SessionID {
		return ErrPassthroughConflict
	}
	if snapshot.Generation > 0 && !o.Live() {
		if err := o.CleanupConfig(ctx, snapshot.Generation); err != nil {
			return err
		}
	}
	if len(request.Files) == 0 {
		return nil
	}
	privateRoot := filepath.Join(cfg.WorkDir, ".kandev", "sessions", cfg.SessionID, "passthrough")
	return passthroughconfig.Prepare(ctx, cfg.WorkDir, privateRoot, o.fileOwnerPrefix()+strconv.FormatUint(request.Generation, 10), request.Files)
}

func (o *PassthroughOwner) validConfigGeneration(generation uint64) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return generation > 0 && generation <= o.current.Generation+1
}

func (o *PassthroughOwner) CleanupConfig(ctx context.Context, generation uint64) error {
	if !o.validConfigGeneration(generation) {
		return ErrPassthroughConflict
	}
	return passthroughconfig.Cleanup(ctx, o.manager.cfg.WorkDir, o.fileOwnerPrefix()+strconv.FormatUint(generation, 10), false)
}

func (m *Manager) cleanupPassthroughFiles(ctx context.Context) error {
	if m.passthrough == nil {
		return nil
	}
	return passthroughconfig.Cleanup(ctx, m.cfg.WorkDir, m.passthrough.fileOwnerPrefix(), true)
}
