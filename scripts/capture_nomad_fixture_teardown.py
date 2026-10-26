#!/usr/bin/env python3
"""Capture a private, read-only identity snapshot for fixture teardown.

This emits identifiers, revisions and a digest of the exact SQLite metadata
bytes. It never copies secret values or performs runtime cleanup.
"""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
import sqlite3
import stat
import subprocess
from pathlib import Path


def _owned_private(path: Path, root: Path, *, directory: bool = False, private_file: bool = True) -> None:
    info = path.lstat()
    if stat.S_ISLNK(info.st_mode) or (directory and not stat.S_ISDIR(info.st_mode)):
        raise ValueError("fixture path type is unsafe")
    if not directory and not stat.S_ISREG(info.st_mode):
        raise ValueError("fixture file type is unsafe")
    if info.st_uid != os.getuid() or (private_file and not directory and info.st_mode & 0o077):
        raise ValueError("fixture path must be user-owned and private")
    resolved = path.resolve(strict=True)
    if resolved != root and root not in resolved.parents:
        raise ValueError("fixture path must remain under fixture_root")


def capture(config_path: Path, database_path: Path, output_path: Path) -> dict:
    config_path = config_path.resolve(strict=True)
    database_path = database_path.resolve(strict=True)
    root_text = json.loads(config_path.read_text(encoding="utf-8"))["fixture_root"]
    root = Path(root_text).resolve(strict=True)
    _owned_private(root, root, directory=True)
    _owned_private(config_path, root)
    # SQLite may create a 0644 main file, but the 0700 fixture ancestors keep
    # it private. Config and generated manifest files remain mode 0600.
    _owned_private(database_path, root, private_file=False)
    if output_path.exists() or output_path.is_symlink():
        raise ValueError("output path must not already exist")
    output_parent = output_path.parent.resolve(strict=True)
    if output_parent != root and root not in output_parent.parents:
        raise ValueError("output path must remain under fixture_root")
    _owned_private(output_parent, root, directory=True)

    config = json.loads(config_path.read_text(encoding="utf-8"))
    session = config.get("session")
    if config.get("fixture_only") is not True or not isinstance(session, dict):
        raise ValueError("fixture config is not an attached session manifest")
    task_id = session.get("task_id")
    session_id = session.get("session_id")
    if not isinstance(task_id, str) or not task_id or not isinstance(session_id, str) or not session_id:
        raise ValueError("fixture task/session identity is incomplete")

    uri = f"file:{database_path}?mode=ro"
    connection = sqlite3.connect(uri, uri=True)
    connection.row_factory = sqlite3.Row
    try:
        row = connection.execute(
            """SELECT id, session_id, task_id, agent_execution_id, executor_id,
                      runtime, status, metadata, CAST(updated_at AS TEXT) AS updated_at
                 FROM executors_running WHERE task_id=? AND session_id=? AND runtime='nomad'""",
            (task_id, session_id),
        ).fetchone()
        if row is None:
            raise ValueError("exact Nomad execution row is unavailable")
        state = connection.execute(
            "SELECT state FROM task_sessions WHERE id=? AND task_id=?", (session_id, task_id)
        ).fetchone()
        if state is None:
            raise ValueError("exact task session is unavailable")
    finally:
        connection.close()

    metadata = json.loads(row["metadata"])
    state_key = "nomad_environment_state"
    nomad_state = metadata.get(state_key)
    if not isinstance(nomad_state, dict):
        raise ValueError("Nomad execution state is unavailable")
    pid = session.get("agentctl_pid")
    runtime_path = session.get("agentctl_runtime_path")
    if not isinstance(pid, int) or pid <= 1 or not isinstance(runtime_path, str) or not runtime_path:
        raise ValueError("agentctl process identity is incomplete")
    ps = subprocess.run(
        ["/bin/ps", "-p", str(pid), "-o", "lstart=", "-o", "command="],
        capture_output=True, text=True, timeout=5, check=False,
        env={"PATH": "/usr/bin:/bin"},
    )
    line = ps.stdout.strip()
    if ps.returncode != 0 or not line or runtime_path not in line:
        raise ValueError("agentctl PID does not match the captured runtime path")
    process_start = " ".join(line.split()[:5])

    manifest = {
        "schema_version": 1,
        "fixture_root": str(root),
        "database_path": str(database_path),
        "session_config_path": str(config_path),
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "expected_session_state": state["state"],
        "expected_row": {
            "id": row["id"],
            "task_id": row["task_id"],
            "session_id": row["session_id"],
            "execution_id": row["agent_execution_id"],
            "executor_id": row["executor_id"],
            "runtime": row["runtime"],
            "status": row["status"],
            "updated_at": row["updated_at"],
            "metadata_sha256": hashlib.sha256(row["metadata"].encode()).hexdigest(),
        },
        "expected_nomad": {
            key: nomad_state.get(key)
            for key in (
                "phase", "revision", "environment_id", "generation", "operation_id",
                "job_id", "allocation_id", "allocation_create_index", "node_id",
                "workspace_path", "server_identity",
            )
        },
        "expected_process": {
            "agentctl_pid": pid,
            "agentctl_start": process_start,
            "agentctl_runtime_path": runtime_path,
        },
    }
    required_nomad = ("phase", "revision", "environment_id", "generation", "operation_id", "job_id", "allocation_id", "allocation_create_index", "node_id", "workspace_path", "server_identity")
    if any(manifest["expected_nomad"].get(key) in (None, "", 0) for key in required_nomad):
        raise ValueError("immutable Nomad identity/revision is incomplete")

    payload = (json.dumps(manifest, sort_keys=True, separators=(",", ":")) + "\n").encode()
    fd = os.open(output_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(payload)
            stream.flush()
            os.fsync(stream.fileno())
    except Exception:
        output_path.unlink(missing_ok=True)
        raise
    return manifest


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True, type=Path)
    parser.add_argument("--database", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    try:
        manifest = capture(args.config, args.database, args.out)
    except Exception as exc:
        print("capture failed:", type(exc).__name__)
        return 2
    print(json.dumps({
        "captured": True,
        "task_id": manifest["expected_row"]["task_id"],
        "session_id": manifest["expected_row"]["session_id"],
        "row_updated_at": manifest["expected_row"]["updated_at"],
        "metadata_sha256": manifest["expected_row"]["metadata_sha256"],
        "nomad_revision": manifest["expected_nomad"]["revision"],
        "phase": manifest["expected_nomad"]["phase"],
        "output": str(args.out),
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
