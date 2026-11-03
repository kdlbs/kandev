package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/maintenance"
	"github.com/kandev/kandev/internal/task/models"
)

const maintenanceHelp = `kandev maintenance database — offline SQLite retention and compaction

Usage:
  kandev maintenance database [--execute] [--compact] [--json]
      [--keep-plan-revisions <n>] [--candidate-limit <n>] [--home-dir <path>]

Without --execute, this is a read-only dry run: it reports legacy retention
categories and a bounded sample of eligible delivery payloads, receipts,
retired-session evidence, and attributable orphan identities. It never
acquires a lock, takes a backup, migrates a schema, or changes anything.

--execute requires exclusive access to the database (kandev must not be
running against the same home/database) and always takes and verifies a
fresh backup before deleting anything. Backups are written under
<database-dir>/backups/kandev-pre-maintenance-<timestamp>.db. Delivery work
uses bounded transactions and may be partial; rerun the command to continue.

--compact additionally stages a VACUUM INTO compacted copy after retention
deletes commit, verifies it with an integrity check, and atomically
replaces the live database, preserving the pre-compaction file alongside it
as <database-path>.pre-compact-<timestamp>.bak for manual rollback (stop
kandev if running, then move that file back over the live database path).

Options:
  --json                       Print a machine-readable JSON outcome.
  --execute                    Perform retention deletes (default: dry run).
  --compact                    Also stage and swap in a compacted copy (requires --execute).
  --keep-plan-revisions <n>    Additionally protect the n most recent non-HEAD plan revisions per task (default 0).
  --candidate-limit <n>        Cap candidates reported/deleted per category. A capped delivery run may need another invocation.
  --home-dir <path>            Kandev home directory override (default: $KANDEV_HOME_DIR or ~/.kandev).
  --help, -h                   Print this help.
`

type maintenanceDatabaseArgs struct {
	JSON              bool
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
	if args.JSON {
		ignoreBrokenPipeSignal()
	}

	cfg, err := loadBootstrapConfigWithHome(args.HomeDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	applyMaintenanceHomeDirOverride(cfg, args.HomeDir)

	homeDir := resolveHomeDirForConfig(cfg)
	databasePath := resolveDatabasePathForConfig(cfg)
	opts := maintenance.RunOptions{
		Execute:           args.Execute,
		Compact:           args.Compact,
		KeepPlanRevisions: args.KeepPlanRevisions,
		CandidateLimit:    args.CandidateLimit,
		DeliveryRetention: models.AgentDeliveryRetentionBatchOptions{
			MaxCandidates:   cfg.Tasks.DeliveryRetention.BatchRows,
			MaxPayloadBytes: int64(cfg.Tasks.DeliveryRetention.BatchBytes),
			PayloadGrace:    cfg.Tasks.DeliveryRetention.PayloadGrace,
			ReplayAge:       cfg.Tasks.DeliveryRetention.ReplayAge,
			ReplayEvents:    cfg.Tasks.DeliveryRetention.ReplayEvents,
		},
	}

	outcome, err := maintenance.Run(context.Background(), homeDir, cfg.Database.Driver, databasePath, opts, nil)
	return finishMaintenanceOutcome(outcome, err, args.JSON, os.Stdout, os.Stderr)
}

func finishMaintenanceOutcome(outcome maintenance.Outcome, runErr error, jsonOutput bool, stdout, stderr io.Writer) int {
	var outputErr error
	if runErr == nil || outcome.Executed {
		if jsonOutput {
			outputErr = writeMaintenanceJSON(stdout, outcome)
		} else {
			printMaintenanceOutcome(outcome)
		}
	}
	if outputErr != nil {
		_, _ = fmt.Fprintf(stderr, "[kandev] failed to write maintenance output: %v\n", outputErr)
	}
	if runErr != nil {
		printMaintenanceFailure(stderr, runErr)
	}
	if runErr != nil || outputErr != nil {
		return 1
	}
	return 0
}

func applyMaintenanceHomeDirOverride(cfg *config.Config, homeDir string) {
	if cfg != nil && strings.TrimSpace(homeDir) != "" {
		cfg.HomeDir = homeDir
	}
}

func parseMaintenanceDatabaseArgs(argv []string) (maintenanceDatabaseArgs, error) {
	var out maintenanceDatabaseArgs
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == flagHelp || arg == "-h" {
			out.ShowHelp = true
			return out, nil
		}
		if arg == flagJSON {
			out.JSON = true
			continue
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

func printMaintenanceFailure(stderr io.Writer, err error) {
	switch {
	case errors.Is(err, maintenance.ErrUnsupportedDriver):
		_, _ = fmt.Fprintln(stderr, "[kandev] "+err.Error())
	case errors.Is(err, maintenance.ErrOwnershipConflict):
		_, _ = fmt.Fprintln(stderr, "[kandev] "+err.Error())
	case errors.Is(err, maintenance.ErrBackupUnverified):
		_, _ = fmt.Fprintln(stderr, "[kandev] refusing to execute: "+err.Error())
	case errors.Is(err, maintenance.ErrInsufficientDiskSpace):
		_, _ = fmt.Fprintln(stderr, "[kandev] refusing to compact: "+err.Error())
	case errors.Is(err, maintenance.ErrIntegrityCheckFailed):
		_, _ = fmt.Fprintln(stderr, "[kandev] refusing to compact: "+err.Error())
	default:
		_, _ = fmt.Fprintln(stderr, "[kandev] maintenance failed: "+err.Error())
	}
}

func printMaintenanceOutcome(outcome maintenance.Outcome) {
	report := outcome.Report
	fmt.Printf("kandev maintenance database report (generated %s)\n", report.GeneratedAt.Format("2006-01-02T15:04:05Z"))
	fmt.Printf("  database size:                %d bytes\n", report.DatabaseSizeBytes)
	fmt.Printf("  duplicate git snapshots:      %d rows, ~%d bytes\n", report.DuplicateGitSnapshots.RowCount, report.DuplicateGitSnapshots.EstBytes)
	fmt.Printf("  obsolete plan revisions:      %d rows, ~%d bytes\n", report.ObsoletePlanRevisions.RowCount, report.ObsoletePlanRevisions.EstBytes)
	fmt.Printf("  orphaned message payloads:    %d rows, ~%d bytes\n", report.OrphanedMessagePayloads.RowCount, report.OrphanedMessagePayloads.EstBytes)
	if !report.DeliveryRetentionAvailable {
		fmt.Printf("  durable delivery retention:   unavailable (%s)\n", report.DeliveryRetentionUnavailableReason)
	} else {
		printMaintenanceGroup("projected delivery payloads", report.ProjectedDeliveryPayloads)
		printMaintenanceGroup("settled prompt payloads", report.SettledDeliveryPrompts)
		printMaintenanceGroup("retired event receipts", report.RetiredDeliveryEventReceipts)
		printMaintenanceGroup("retired session evidence", report.RetiredDeliverySessionEvidence)
		printMaintenanceGroup("orphaned delivery identities", report.OrphanedDeliveryIdentities)
	}
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
	fmt.Printf("  deleted delivery payloads:    %d\n", outcome.Execution.DeletedDeliveryEventPayloads)
	fmt.Printf("  deleted settled prompts:      %d\n", outcome.Execution.DeletedSettledDeliveryPrompts)
	fmt.Printf("  retired event receipts:       %d\n", outcome.Execution.RetiredDeliveryEventReceipts)
	fmt.Printf("  purged session evidence rows: %d\n", outcome.Execution.PurgedRetiredDeliveryRows)
	fmt.Printf("  orphan jobs scheduled:        %d\n", outcome.Execution.DeliveryOrphanJobsScheduled)
	fmt.Printf("  retirement jobs completed:    %d\n", outcome.Execution.DeliveryRetirementJobsCompleted)
	if outcome.Execution.Partial {
		fmt.Println("  delivery execution:           partial; rerun to continue bounded work")
	}

	if outcome.Compaction == nil {
		return
	}
	fmt.Println("compaction: applied")
	fmt.Printf("  size before:                  %d bytes\n", outcome.Compaction.SizeBeforeBytes)
	fmt.Printf("  size after:                   %d bytes\n", outcome.Compaction.SizeAfterBytes)
	fmt.Printf("  rollback artifact:            %s\n", outcome.Compaction.RollbackPath)
	fmt.Println("  to roll back: stop kandev (if running), then move the rollback artifact back over the live database path.")
}

func printMaintenanceGroup(name string, group maintenance.CandidateGroup) {
	limit := ""
	if group.SampleLimitReached {
		limit = ", sample limit reached"
	}
	fmt.Printf("  %-30s %d rows, ~%d bytes%s\n", name+":", group.RowCount, group.EstBytes, limit)
}
