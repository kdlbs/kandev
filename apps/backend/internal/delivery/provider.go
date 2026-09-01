package delivery

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
)

// Provide constructs the delivery ledger repository, wires the ancestry
// checker to the supplied checkout resolver, and starts the sweep
// immediately (spec "Boundary values": first sweep runs at boot, no
// initial delay). writer and reader must already have the tasks and
// repositories tables present — Provide must therefore be called after
// task/repository.Provide in the boot sequence, since the ledger's
// foreign keys require those tables to exist at CREATE TABLE time on
// PostgreSQL. checkout may be nil (e.g. in a deployment with no task
// service wired), in which case the ancestry check is never attempted
// and every pair's default-branch observation is limited to the
// provider/direct-commit bases.
//
// Returns the Sweep (exposed for tests/diagnostics) and a cleanup
// function that stops it.
func Provide(writer, reader *sqlx.DB, checkout CheckoutResolver, log *logger.Logger) (*Sweep, func() error, error) {
	repo, err := NewWithDB(writer, reader, log)
	if err != nil {
		return nil, nil, err
	}
	var ancestry *AncestryChecker
	if checkout != nil {
		ancestry = &AncestryChecker{Checkout: checkout}
	}
	sweep := NewSweep(repo, ancestry, log)
	sweep.Start(context.Background())
	cleanup := func() error {
		sweep.Stop()
		return nil
	}
	return sweep, cleanup, nil
}
