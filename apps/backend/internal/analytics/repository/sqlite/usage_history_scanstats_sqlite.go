//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

/*
#include <stdint.h>

// Public ABI from go-sqlite3 v1.14.33/sqlite3-binding.h. Link to the driver's
// amalgamation, not a second system SQLite with different compile options.
typedef struct sqlite3_stmt sqlite3_stmt;
typedef struct sqlite3 sqlite3;
typedef int64_t sqlite3_int64;
extern int sqlite3_stmt_scanstatus_v2(sqlite3_stmt *statement, int index, int opcode, int flags, void *output);
extern void sqlite3_stmt_scanstatus_reset(sqlite3_stmt*);
extern const char *sqlite3_sql(sqlite3_stmt*);
extern int sqlite3_compileoption_used(const char*);
extern int sqlite3_db_config(sqlite3*, int, ...);

typedef struct {
  sqlite3_int64 loops, visits;
  int select_id, parent_id;
  const char *name, *plan;
  int result;
} usage_bench_scan;

static usage_bench_scan usage_bench_read_scan(uintptr_t handle, int idx) {
  sqlite3_stmt *s = (sqlite3_stmt*)handle;
  usage_bench_scan measured = {0};
  usage_bench_scan *out = &measured;
  // flags=0 enumerates SCAN/SEARCH loops. NVISIT already covers all NLOOPs.
  if (sqlite3_stmt_scanstatus_v2(s, idx, 1, 0, &out->visits)) out->result = 1;
  else if (sqlite3_stmt_scanstatus_v2(s, idx, 0, 0, &out->loops)) out->result = 2;
  else if (sqlite3_stmt_scanstatus_v2(s, idx, 3, 0, &out->name)) out->result = 3;
  else if (sqlite3_stmt_scanstatus_v2(s, idx, 4, 0, &out->plan)) out->result = 4;
  else if (sqlite3_stmt_scanstatus_v2(s, idx, 5, 0, &out->select_id)) out->result = 5;
  else if (sqlite3_stmt_scanstatus_v2(s, idx, 6, 0, &out->parent_id)) out->result = 6;
  return measured;
}
static void usage_bench_reset(uintptr_t h) {
  sqlite3_stmt_scanstatus_reset((sqlite3_stmt*)h);
}
static const char *usage_bench_sql(uintptr_t h) {
  return sqlite3_sql((sqlite3_stmt*)h);
}
static int usage_bench_supported(void) {
  return sqlite3_compileoption_used("ENABLE_STMT_SCANSTATUS");
}
static int usage_bench_collection(uintptr_t h, int enabled) {
  int actual = -1;
  int rc = sqlite3_db_config((sqlite3*)h, 1018, enabled, &actual);
  return rc ? rc : (actual == enabled ? 0 : -1);
}
*/
import "C"

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mattn/go-sqlite3"
)

func attachUsageBenchSQLite(ctx context.Context, database *sql.DB, observer *usageBenchObserver) error {
	if err := setUsageBenchSQLiteCollection(ctx, database, true); err != nil {
		return err
	}
	if C.usage_bench_supported() != 1 {
		return fmt.Errorf("SQLite benchmark requires SQLITE_ENABLE_STMT_SCANSTATUS")
	}
	// The report uses the production writer's single persistent connection.
	// Release the lease before the repository begins its own transaction.
	if database.Stats().MaxOpenConnections != 1 {
		return fmt.Errorf("SQLite benchmark instrumentation requires exactly one writer connection")
	}
	connection, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	return connection.Raw(func(raw any) error {
		return raw.(*sqlite3.SQLiteConn).SetTrace(&sqlite3.TraceConfig{
			EventMask: sqlite3.TraceStmt | sqlite3.TraceProfile,
			Callback: func(info sqlite3.TraceInfo) int {
				if info.EventCode == sqlite3.TraceStmt && !strings.HasPrefix(info.StmtOrTrigger, "--") {
					C.usage_bench_reset(C.uintptr_t(info.StmtHandle))
				}
				if info.EventCode == sqlite3.TraceProfile && observer.active() {
					statement, err := readUsageBenchSQLite(info.StmtHandle)
					statement.DurationNS = info.RunTimeNanosec
					observer.record(statement, err)
				}
				return 0
			},
		})
	})
}

func setUsageBenchSQLiteCollection(ctx context.Context, database *sql.DB, enabled bool) error {
	if C.usage_bench_supported() != 1 || database.Stats().MaxOpenConnections != 1 {
		return fmt.Errorf("benchmark requires scanstatus-capable SQLite with one persistent writer connection")
	}
	connection, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	var configurationError error
	err = connection.Raw(func(raw any) error {
		return raw.(*sqlite3.SQLiteConn).SetTrace(&sqlite3.TraceConfig{
			EventMask: sqlite3.TraceStmt,
			Callback: func(info sqlite3.TraceInfo) int {
				if code := C.usage_bench_collection(C.uintptr_t(info.ConnHandle), C.int(boolInt(enabled))); code != 0 {
					configurationError = fmt.Errorf("SQLite rejected benchmark scanstatus collection configuration")
				}
				return 0
			},
		})
	})
	if err != nil {
		return err
	}
	_, queryError := connection.ExecContext(ctx, `SELECT 1`)
	detachError := connection.Raw(func(raw any) error { return raw.(*sqlite3.SQLiteConn).SetTrace(nil) })
	if configurationError != nil {
		return configurationError
	}
	if queryError != nil {
		return queryError
	}
	return detachError
}

func readUsageBenchSQLite(handle uintptr) (usageBenchStatement, error) {
	statement := usageBenchStatement{SQL: C.GoString(C.usage_bench_sql(C.uintptr_t(handle)))}
	for index := 0; ; index++ {
		measured := C.usage_bench_read_scan(C.uintptr_t(handle), C.int(index))
		if measured.result == 1 {
			return statement, nil
		}
		if measured.result != 0 || measured.visits < 0 || measured.loops < 0 {
			return statement, fmt.Errorf("SQLite scanstatus unavailable for loop %d", index)
		}
		statement.SQLiteLoops = append(statement.SQLiteLoops, usageBenchScanLoop{
			Name: C.GoString(measured.name), Plan: C.GoString(measured.plan),
			SelectID: int(measured.select_id), ParentID: int(measured.parent_id),
			Loops: int64(measured.loops), Visits: int64(measured.visits),
		})
	}
}
