package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// Fixture-only crash barrier for the Nomad launch window in which the Nomad
// inventory row is durable at phase ready but task_environments has not yet
// been published creating->ready. It is armed only when BOTH
// KANDEV_FIXTURE_ONLY=1 and KANDEV_FIXTURE_NOMAD_PRE_PUBLISH_BARRIER (an
// absolute marker-file path in a private directory owned by the current user)
// are set. It fires at most once per process for the nomad executor type: it
// writes a 0600 marker holding only ids, fsyncs it, then blocks until the
// context is cancelled and returns the context error.
const (
	fixtureOnlyEnv            = "KANDEV_FIXTURE_ONLY"
	nomadPrePublishBarrierEnv = "KANDEV_FIXTURE_NOMAD_PRE_PUBLISH_BARRIER"
)

type nomadBarrierMarker struct {
	ExecutionID   string `json:"execution_id"`
	EnvironmentID string `json:"environment_id"`
	SessionID     string `json:"session_id"`
}

type nomadPrePublishBarrier struct {
	fired atomic.Bool
}

var processNomadPrePublishBarrier = &nomadPrePublishBarrier{}

// await returns nil immediately unless the barrier is armed and applies.
func (b *nomadPrePublishBarrier) await(ctx context.Context, logger *zap.Logger, executorType string, m nomadBarrierMarker) error {
	if executorType != string(models.ExecutorTypeNomad) || os.Getenv(fixtureOnlyEnv) != "1" {
		return nil
	}
	path := os.Getenv(nomadPrePublishBarrierEnv)
	if path == "" {
		return nil
	}
	if err := validateBarrierPath(path); err != nil {
		logger.Warn("nomad pre-publish barrier refused", zap.Error(err))
		return nil
	}
	if !b.fired.CompareAndSwap(false, true) {
		return nil
	}
	if err := writeBarrierMarker(path, m); err != nil {
		logger.Warn("nomad pre-publish barrier marker not written", zap.Error(err))
		return nil
	}
	<-ctx.Done()
	return fmt.Errorf("nomad pre-publish barrier released: %w", ctx.Err())
}

func validateBarrierPath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("barrier path is not absolute")
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("stat barrier parent: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("barrier parent is not a directory")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("barrier parent is group or world writable")
	}
	if !ownedByCurrentUser(info) {
		return fmt.Errorf("barrier parent is not owned by the current user")
	}
	return nil
}

// writeBarrierMarker publishes the marker atomically: the content is fsynced
// in a temp file and linked into place, so readers never see a partial file.
func writeBarrierMarker(path string, m nomadBarrierMarker) error {
	line, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".barrier-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(line, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmp.Name(), path); err != nil {
		return err
	}
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
