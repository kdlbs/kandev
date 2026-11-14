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
	DurationNS   int64                `json:"duration_ns,omitempty"`
	Error        string               `json:"error,omitempty"`
	SQLiteLoops  []usageBenchScanLoop `json:"sqlite_scan_loops,omitempty"`
	PostgresPlan json.RawMessage      `json:"postgres_explain_analyze_buffers,omitempty"`
	Args         []driver.NamedValue  `json:"-"`
}

// This observer is compiled only for the explicit benchmark instrumentation
// build. A callback copies counters while the driver still owns the statement.
type usageBenchObserver struct {
	mu            sync.Mutex
	enabled       bool
	statements    []usageBenchStatement
	maxStatements int
	discardArgs   bool
	err           error
}

func (o *usageBenchObserver) start() {
	o.startWithLimit(0, false)
}

func (o *usageBenchObserver) startWithLimit(maxStatements int, discardArgs bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.enabled, o.statements, o.maxStatements, o.discardArgs, o.err = true, nil, maxStatements, discardArgs, nil
}

func (o *usageBenchObserver) active() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.enabled
}

func (o *usageBenchObserver) limitedTrace() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.enabled && o.maxStatements > 0
}

func (o *usageBenchObserver) record(statement usageBenchStatement, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.enabled {
		if o.discardArgs {
			statement.Args = nil
		}
		if o.maxStatements > 0 && len(o.statements) == o.maxStatements {
			copy(o.statements, o.statements[1:])
			o.statements[len(o.statements)-1] = statement
		} else {
			o.statements = append(o.statements, statement)
		}
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

func (o *usageBenchObserver) snapshot() []usageBenchStatement {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]usageBenchStatement(nil), o.statements...)
}

func usageBenchReadSQL(query string) bool {
	upper := strings.ToUpper(strings.TrimSpace(query))
	return strings.HasPrefix(upper, "SELECT ") || strings.HasPrefix(upper, "WITH ")
}
