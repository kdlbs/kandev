package inventoryrepair

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type row map[string]any

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type inspection struct {
	report *Report
	rows   map[string]string
	git    map[string]GitState
	slots  map[string]row
	envs   map[string]row
	tasks  map[string]row
	jobs   map[string]row
	db     queryer
	guards []observation
}

func openDatabase(p Plan, write bool) (*sql.DB, error) {
	mode := "ro"
	if write {
		mode = "rw"
	}
	u := url.URL{Scheme: "file", Path: p.Database}
	q := url.Values{"mode": {mode}, "_foreign_keys": {"on"}, "_busy_timeout": {"5000"}}
	if write {
		q.Set("_txlock", "immediate")
		q.Set("_synchronous", "FULL")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func queryRows(ctx context.Context, db queryer, query string, args ...any) ([]row, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := []row{}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		r := row{}
		for i, k := range columns {
			switch v := values[i].(type) {
			case []byte:
				r[k] = string(v)
			case time.Time:
				r[k] = v.UTC().Format(time.RFC3339Nano)
			default:
				r[k] = v
			}
		}
		result = append(result, r)
		if len(result) > 1000 {
			return nil, errors.New("repair inventory exceeds bounded row count")
		}
	}
	return result, rows.Err()
}

func digest(v any) string                  { b, _ := json.Marshal(v); return fmt.Sprintf("%x", sha256.Sum256(b)) }
func stringValue(r row, key string) string { v, _ := r[key].(string); return v }

func (in *inspection) read(ctx context.Context, key, query string, args ...any) ([]row, error) {
	rows, err := queryRows(ctx, in.db, query, args...)
	if err != nil {
		return nil, err
	}
	in.rows[key] = digest(rows)
	in.guards = append(in.guards, observation{Query: query, Args: args, Before: digest(rows)})
	return rows, nil
}

func (in *inspection) one(ctx context.Context, key, query string, args ...any) (row, error) {
	rows, err := in.read(ctx, key, query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("%s: expected one exact row, found %d", key, len(rows))
	}
	return rows[0], nil
}
