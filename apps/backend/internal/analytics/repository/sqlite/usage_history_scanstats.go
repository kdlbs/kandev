//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"sync"
)

type usageBenchScanLoop struct {
	Name     string `json:"name"`
	Plan     string `json:"plan"`
	SelectID int    `json:"select_id"`
	ParentID int    `json:"parent_id"`
	Loops    int64  `json:"nloop"`
	Visits   int64  `json:"nvisit"`
}

type usageBenchStatement struct {
	SQL          string               `json:"sql"`
	SQLiteLoops  []usageBenchScanLoop `json:"sqlite_scan_loops,omitempty"`
	PostgresPlan json.RawMessage      `json:"postgres_explain_analyze_buffers,omitempty"`
	Args         []driver.NamedValue  `json:"-"`
}

// This observer is compiled only for the explicit benchmark instrumentation
// build. A callback copies counters while the driver still owns the statement.
type usageBenchObserver struct {
	mu         sync.Mutex
	enabled    bool
	statements []usageBenchStatement
	err        error
}

func (o *usageBenchObserver) start() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.enabled, o.statements, o.err = true, nil, nil
}

func (o *usageBenchObserver) active() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.enabled
}

func (o *usageBenchObserver) record(statement usageBenchStatement, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.enabled {
		o.statements = append(o.statements, statement)
		if err != nil {
			o.err = err
		}
	}
}

func (o *usageBenchObserver) finish() ([]usageBenchStatement, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.enabled = false
	if o.err != nil {
		return nil, o.err
	}
	if len(o.statements) == 0 {
		return nil, errors.New("benchmark instrumentation observed no SQL statements")
	}
	return o.statements, nil
}

func usageBenchReadSQL(query string) bool {
	upper := strings.ToUpper(strings.TrimSpace(query))
	return strings.HasPrefix(upper, "SELECT ") || strings.HasPrefix(upper, "WITH ")
}
