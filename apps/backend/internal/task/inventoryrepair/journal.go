package inventoryrepair

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	"github.com/kandev/kandev/internal/persistence"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

const phaseComplete = "complete"

// ErrRepairPending distinguishes an unresolved repair from a running backend.
var ErrRepairPending = errors.New("inventory repair is unresolved")

type journal struct {
	Plan         Plan          `json:"plan"`
	Phase        string        `json:"phase"`
	BackupSHA256 string        `json:"backup_sha256"`
	Changes      []mutation    `json:"changes"`
	Observations []observation `json:"observations"`
}

func journalPath(p Plan) string {
	return filepath.Join(p.Home, "inventory-repairs", p.OperationID, "journal.json")
}

func fencePaths(home, database string) []string {
	return []string{filepath.Join(home, ".kandev-inventory-repair.json"), database + ".inventory-repair.json"}
}

// CheckPending runs under backend ownership locks, before migrations or recovery.
func CheckPending(home, driver, database string) error {
	targets, err := ownershiplock.Targets(home, driver, database)
	if err != nil {
		return err
	}
	return CheckPendingTargets(targets)
}

// CheckPendingTargets checks the canonical resources already locked by startup.
// The home fence covers its in-home database, just as the home ownership lock does.
func CheckPendingTargets(targets []ownershiplock.Target) error {
	for _, target := range targets {
		var path string
		switch target.Kind {
		case ownershiplock.TargetHome:
			path = filepath.Join(target.ResourcePath, ".kandev-inventory-repair.json")
		case ownershiplock.TargetDatabase:
			path = target.ResourcePath + ".inventory-repair.json"
		default:
			return fmt.Errorf("unknown repair ownership target: %q", target.Kind)
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w at %s; use the repair plan with --apply or --rollback before starting the backend", ErrRepairPending, path)
		}
	}
	return nil
}

func privateDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	h, err := storageworkspaces.OpenDirectoryNoFollow(filepath.Dir(path), path)
	if err != nil {
		return err
	}
	defer func() { _ = h.Close() }()
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0700 {
		return fmt.Errorf("repair directory must have mode 0700: %s", path)
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}

func writePrivate(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".repair-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func (j *journal) save() error {
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(journalPath(j.Plan), data)
}

func loadJournal(p Plan) (*journal, error) {
	path := journalPath(p)
	if _, err := os.Lstat(path); err != nil {
		return nil, err
	}
	h, err := storageworkspaces.OpenDirectoryNoFollow(p.Home, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = h.Close() }()
	data, err := h.ReadFile("journal.json")
	if err != nil {
		return nil, err
	}
	if len(data) > 16<<20 {
		return nil, errors.New("repair journal exceeds size limit")
	}
	var j journal
	if err = json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	if digest(j.Plan) != digest(p) {
		return nil, errors.New("operation journal belongs to a different repair plan")
	}
	return &j, nil
}

func (j *journal) fence() error {
	for _, path := range fencePaths(j.Plan.Home, j.Plan.Database) {
		data, err := os.ReadFile(path)
		if err == nil {
			if string(data) != journalPath(j.Plan) {
				return errors.New("another inventory repair is unresolved")
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err = writePrivate(path, []byte(journalPath(j.Plan))); err != nil {
			return err
		}
	}
	return nil
}

func (j *journal) finish(phase string) error {
	j.Phase = phase
	if err := j.save(); err != nil {
		return err
	}
	for _, path := range fencePaths(j.Plan.Home, j.Plan.Database) {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if string(data) != journalPath(j.Plan) {
			return errors.New("refusing to remove another repair's startup fence")
		}
		if err = os.Remove(path); err != nil {
			return err
		}
		if err = syncDirectory(filepath.Dir(path)); err != nil {
			return err
		}
	}
	return nil
}

func prepareJournal(ctx context.Context, in *inspection, db *sql.DB) (*journal, error) {
	p := in.report.Plan
	if err := privateDirectory(filepath.Join(p.Home, "inventory-repairs")); err != nil {
		return nil, err
	}
	dir := filepath.Dir(journalPath(p))
	if err := privateDirectory(dir); err != nil {
		return nil, err
	}
	changes, err := in.mutations(ctx, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	backup := filepath.Join(dir, "before.db")
	if _, err = os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("backup destination already exists or cannot be inspected; use a new operation ID")
	}
	if _, err = persistence.SnapshotSQLiteContext(ctx, sqlx.NewDb(db, "sqlite3"), backup); err != nil {
		return nil, err
	}
	if err = os.Chmod(backup, 0600); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(backup, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	err = f.Sync()
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	if err = verifyBackup(ctx, p, backup); err != nil {
		return nil, err
	}
	hash, err := fileDigest(backup)
	if err != nil {
		return nil, err
	}
	j := &journal{Plan: p, Phase: "prepared", BackupSHA256: hash, Changes: changes, Observations: in.guards}
	if err = j.save(); err != nil {
		return nil, err
	}
	return j, nil
}

func verifyBackup(ctx context.Context, p Plan, path string) error {
	p.Database = path
	db, err := openDatabase(p, false)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	var result string
	if err = db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("repair backup failed SQLite integrity check")
	}
	return nil
}
