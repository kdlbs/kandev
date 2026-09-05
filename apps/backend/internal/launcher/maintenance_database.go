package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/maintenance"
)

const maintenanceHelp = `kandev maintenance database — offline SQLite retention and compaction

Usage:
  kandev maintenance database [--execute] [--compact]
      [--keep-plan-revisions <n>] [--candidate-limit <n>] [--home-dir <path>]

Without --execute, this is a read-only dry run: it reports how many
duplicate git-snapshot, obsolete plan-revision, and orphaned message-payload
rows are eligible for removal (and their estimated bytes), and never
acquires a lock, takes a backup, or changes anything.

--execute requires exclusive access to the database (kandev must not be
running against the same home/database) and always takes and verifies a
fresh backup before deleting anything. Backups are written under
<database-dir>/backups/kandev-pre-maintenance-<timestamp>.db.

--compact additionally stages a VACUUM INTO compacted copy after retention
deletes commit, verifies it with an integrity check, and atomically
replaces the live database, preserving the pre-compaction file alongside it
as <database-path>.pre-compact-<timestamp>.bak for manual rollback (stop
kandev if running, then move that file back over the live database path).

Options:
  --execute                    Perform retention deletes (default: dry run).
  --compact                    Also stage and swap in a compacted copy (requires --execute).
  --keep-plan-revisions <n>    Additionally protect the n most recent non-HEAD plan revisions per task (default 0).
  --candidate-limit <n>        Cap candidates reported/deleted per category (default: unlimited).
  --home-dir <path>            Kandev home directory override (default: $KANDEV_HOME_DIR or ~/.kandev).
  --help, -h                   Print this help.
`

type maintenanceDatabaseArgs struct {
	Execute           bool
	Compact           bool
	KeepPlanRevisions int
	CandidateLimit    int
	HomeDir           string
	ShowHelp          bool
}

func runMaintenance(argv []string, _ BuildInfo) int {
	if len(argv) == 0 || argv[0] == flagHelp || argv[0] == "-h" {
		fmt.Print(maintenanceHelp)
		return 0
	}
	if argv[0] != "database" {
		fmt.Fprintf(os.Stderr, "[kandev] unknown maintenance target %q (only \"database\" is supported)\n", argv[0])
		return 2
	}

	args, err := parseMaintenanceDatabaseArgs(argv[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 2
	}
	if args.ShowHelp {
		fmt.Print(maintenanceHelp)
		return 0
	}

	cfg, err := loadBootstrapConfigWithHome(args.HomeDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}

	homeDir := resolveHomeDirForConfig(cfg)
	databasePath := resolveDatabasePathForConfig(cfg)
	opts := maintenance.RunOptions{
		Execute:           args.Execute,
		Compact:           args.Compact,
		KeepPlanRevisions: args.KeepPlanRevisions,
		CandidateLimit:    args.CandidateLimit,
	}

	outcome, err := maintenance.Run(context.Background(), homeDir, cfg.Database.Driver, databasePath, opts, nil)
	if err != nil {
		printMaintenanceFailure(err)
		return 1
	}
	printMaintenanceOutcome(outcome)
	return 0
}

func parseMaintenanceDatabaseArgs(argv []string) (maintenanceDatabaseArgs, error) {
	var out maintenanceDatabaseArgs
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == flagHelp || arg == "-h" {
			out.ShowHelp = true
			return out, nil
		}
		if arg == "--execute" {
			out.Execute = true
			continue
		}
		if arg == "--compact" {
			out.Compact = true
			continue
		}
		next, err := applyMaintenanceValueFlag(&out, argv, i, arg)
		if err != nil {
			return out, err
		}
		i = next
	}
	return out, nil
}

// applyMaintenanceValueFlag handles the value-taking flags
// (--keep-plan-revisions, --candidate-limit, --home-dir), returning the
// argv index to resume parsing from. Split out of
// parseMaintenanceDatabaseArgs to keep both functions under the
// cyclomatic-complexity limit.
func applyMaintenanceValueFlag(out *maintenanceDatabaseArgs, argv []string, i int, arg string) (int, error) {
	switch {
	case arg == "--keep-plan-revisions" || strings.HasPrefix(arg, "--keep-plan-revisions="):
		value, next, err := maintenanceFlagIntValue(argv, i, arg, "--keep-plan-revisions")
		if err != nil {
			return i, err
		}
		out.KeepPlanRevisions = value
		return next, nil
	case arg == "--candidate-limit" || strings.HasPrefix(arg, "--candidate-limit="):
		value, next, err := maintenanceFlagIntValue(argv, i, arg, "--candidate-limit")
		if err != nil {
			return i, err
		}
		out.CandidateLimit = value
		return next, nil
	case arg == "--home-dir" || strings.HasPrefix(arg, "--home-dir="):
		value, next, err := maintenanceFlagStringValue(argv, i, arg, "--home-dir")
		if err != nil {
			return i, err
		}
		out.HomeDir = value
		return next, nil
	default:
		return i, ParseError{Message: fmt.Sprintf("unknown maintenance database flag %q", arg)}
	}
}

func maintenanceFlagStringValue(argv []string, i int, arg, flagName string) (string, int, error) {
	if value, ok := strings.CutPrefix(arg, flagName+"="); ok {
		return value, i, nil
	}
	if i+1 >= len(argv) {
		return "", i, ParseError{Message: flagName + " requires a value"}
	}
	return argv[i+1], i + 1, nil
}

func maintenanceFlagIntValue(argv []string, i int, arg, flagName string) (int, int, error) {
	raw, next, err := maintenanceFlagStringValue(argv, i, arg, flagName)
	if err != nil {
		return 0, i, err
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, i, ParseError{Message: flagName + " requires a non-negative integer"}
	}
	return value, next, nil
}

func printMaintenanceFailure(err error) {
	switch {
	case errors.Is(err, maintenance.ErrUnsupportedDriver):
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
	case errors.Is(err, maintenance.ErrOwnershipConflict):
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
	case errors.Is(err, maintenance.ErrBackupUnverified):
		fmt.Fprintln(os.Stderr, "[kandev] refusing to execute: "+err.Error())
	case errors.Is(err, maintenance.ErrInsufficientDiskSpace):
		fmt.Fprintln(os.Stderr, "[kandev] refusing to compact: "+err.Error())
	case errors.Is(err, maintenance.ErrIntegrityCheckFailed):
		fmt.Fprintln(os.Stderr, "[kandev] refusing to compact: "+err.Error())
	default:
		fmt.Fprintln(os.Stderr, "[kandev] maintenance failed: "+err.Error())
	}
}

func printMaintenanceOutcome(outcome maintenance.Outcome) {
	report := outcome.Report
	fmt.Printf("kandev maintenance database report (generated %s)\n", report.GeneratedAt.Format("2006-01-02T15:04:05Z"))
	fmt.Printf("  database size:                %d bytes\n", report.DatabaseSizeBytes)
	fmt.Printf("  duplicate git snapshots:      %d rows, ~%d bytes\n", report.DuplicateGitSnapshots.RowCount, report.DuplicateGitSnapshots.EstBytes)
	fmt.Printf("  obsolete plan revisions:      %d rows, ~%d bytes\n", report.ObsoletePlanRevisions.RowCount, report.ObsoletePlanRevisions.EstBytes)
	fmt.Printf("  orphaned message payloads:    %d rows, ~%d bytes\n", report.OrphanedMessagePayloads.RowCount, report.OrphanedMessagePayloads.EstBytes)
	fmt.Printf("  total candidates:             %d rows, ~%d bytes\n", report.TotalCandidateRows(), report.TotalEstBytes())

	if !outcome.Executed {
		fmt.Println("mode: dry run (no lock acquired, nothing changed). Re-run with --execute to apply retention.")
		return
	}

	fmt.Println("mode: execute")
	fmt.Printf("  backup:                       %s\n", outcome.BackupPath)
	fmt.Printf("  deleted git snapshots:        %d\n", outcome.Execution.DeletedGitSnapshots)
	fmt.Printf("  deleted plan revisions:       %d\n", outcome.Execution.DeletedPlanRevisions)
	fmt.Printf("  deleted message payloads:     %d\n", outcome.Execution.DeletedMessagePayloads)

	if outcome.Compaction == nil {
		return
	}
	fmt.Println("compaction: applied")
	fmt.Printf("  size before:                  %d bytes\n", outcome.Compaction.SizeBeforeBytes)
	fmt.Printf("  size after:                   %d bytes\n", outcome.Compaction.SizeAfterBytes)
	fmt.Printf("  rollback artifact:            %s\n", outcome.Compaction.RollbackPath)
	fmt.Println("  to roll back: stop kandev (if running), then move the rollback artifact back over the live database path.")
}
