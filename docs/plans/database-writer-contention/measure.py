#!/usr/bin/env python3
"""Run a Go test command with disposable transaction observations.

Usage, from any directory:
  python3 docs/plans/database-writer-contention/measure.py -- go test ...

The default observes the fixed production path. The optional
--restore-message-reservation switch restores the historical redundant UPDATE
inside the private overlay for comparison; it never edits repository source.

The script never modifies source files or database configuration. All fixture
databases are created by testing.TB.TempDir. Go removes those after each case.
"""

import json
from pathlib import Path
import subprocess
import sys
import tempfile


ROOT = Path(__file__).resolve().parents[3]
PACKAGE = ROOT / "apps/backend/internal/task/repository/sqlite"


def observe_function(source, name, operation):
    start = source.index("func (r *Repository) " + name + "(")
    end = source.find("\nfunc ", start + 1)
    if end == -1:
        end = len(source)
    body = source[start:end]
    if operation:
        old = "r.db.BeginTxx(ctx, nil)"
        assert body.count(old) == 1, name
        body = body.replace(old, f'writerWorkloadBeginTx(ctx, r.db, "{operation}")')
    body = body.replace("tx.Commit()", "writerWorkloadCommit(ctx, tx)")
    body = body.replace("tx.Rollback()", "writerWorkloadRollback(ctx, tx)")
    return source[:start] + body + source[end:]


def main():
    args = sys.argv[1:]
    if args[:1] == ["--omit-message-reservation"]:
        raise SystemExit("--omit-message-reservation is obsolete; the fixed path is the default; use --restore-message-reservation for the comparison")
    restore_reservation = args[:1] == ["--restore-message-reservation"]
    if restore_reservation:
        args = args[1:]
    if args[:1] == ["--"]:
        args = args[1:]
    if args[:2] != ["go", "test"]:
        raise SystemExit("Expected go test arguments")
    with tempfile.TemporaryDirectory(prefix="kandev-writer-overlay-") as directory:
        temporary = Path(directory)
        replacements = {}
        sites = {
            "agent_delivery.go": [("ReceiveAgentDeliveryEvent", "receive")],
            "agent_delivery_projection.go": [("ProjectCanonicalAgentDeliveryEvents", "project")],
            "agent_delivery_settlement.go": [
                ("SettleAgentDeliveryTerminal", "settle"),
                ("settleAgentDeliveryTerminalTx", None),
            ],
            "message_payload_replay.go": [("updateMessageWithPayloadGuard", "message_update")],
        }
        for filename, functions in sites.items():
            original = PACKAGE / filename
            source = original.read_text()
            for name, operation in functions:
                source = observe_function(source, name, operation)
            if restore_reservation and filename == "message_payload_replay.go":
                start = source.index("func (r *Repository) updateMessageWithPayloadGuardTx(")
                query_start = source.index("\tquery :=", start)
                assert 'SET id = id WHERE id = ?' not in source[start:query_start]
                reservation = '''\tif !dialect.IsPostgres(r.db.DriverName()) {
		result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_messages SET id = id WHERE id = ?`), message.ID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return fmt.Errorf("message not found: %s", message.ID)
		}
	}
'''
                source = source[:query_start] + reservation + source[query_start:]
            target = temporary / filename
            target.write_text(source)
            replacements[str(original)] = str(target)
        original = PACKAGE / "writer_workload_observation_test.go"
        source = original.read_text()
        assert source.count("var writerWorkloadInstrumented = false") == 1
        target = temporary / original.name
        target.write_text(source.replace("var writerWorkloadInstrumented = false", "var writerWorkloadInstrumented = true"))
        replacements[str(original)] = str(target)
        overlay = temporary / "overlay.json"
        overlay.write_text(json.dumps({"Replace": replacements}))
        result = subprocess.run(args[:2] + ["-overlay", str(overlay)] + args[2:], cwd=ROOT / "apps/backend")
        return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
