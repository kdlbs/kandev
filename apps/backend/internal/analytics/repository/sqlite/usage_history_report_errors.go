package sqlite

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kandev/kandev/internal/analytics"
	"github.com/mattn/go-sqlite3"
)

func normalizeUsageReportError(err error) error {
	if err == nil {
		return nil
	}
	var pgError *pgconn.PgError
	var sqliteError sqlite3.Error
	if errors.As(err, &pgError) && pgError.Code == "22003" || errors.As(err, &sqliteError) && sqliteError.Error() == "integer overflow" {
		return fmt.Errorf("%w: %v", analytics.ErrUsageAmountOverflow, err)
	}
	return err
}
