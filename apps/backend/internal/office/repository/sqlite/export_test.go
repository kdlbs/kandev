package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// SetClaimWindowHook installs a yield point between the registration's
// claimable-seat selection and its reassigning update, and returns a function
// restoring the previous value. This file is compiled only under `go test`, so
// the hook has no setter at all in a production build.
func SetClaimWindowHook(fn func(ctx context.Context, tx *sqlx.Tx, seatID string)) func() {
	prev := claimWindowHook
	claimWindowHook = fn
	return func() { claimWindowHook = prev }
}
