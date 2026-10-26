#!/usr/bin/env python3
"""Crash one owned Kandev backend at a committed Nomad response-loss barrier.

Fixture-only controller. It must be started before the normal-API task request.
It fails closed when the marker is late, the fresh DB is not single-owner, the
inventory has reached ready, or the recorded backend process identity changed.
"""

from __future__ import annotations

import argparse
import json
import os
import signal
import sqlite3
import stat
import subprocess
import sys
import time
import urllib.request
from pathlib import Path
from typing import Any, Callable


class BarrierError(RuntimeError):
    pass


def _backend_child_identity(pid: int) -> tuple[str, int, int, str]:
    """Read a child start/parent/group/executable tuple; never expose full ps output."""
    try:
        started = subprocess.run(["/bin/ps", "-p", str(pid), "-o", "lstart="], capture_output=True, text=True, timeout=5, check=False)
        ids = subprocess.run(["/bin/ps", "-p", str(pid), "-o", "pgid=,ppid="], capture_output=True, text=True, timeout=5, check=False)
        details = subprocess.run(["/bin/ps", "eww", "-p", str(pid), "-o", "command="], capture_output=True, text=True, timeout=5, check=False)
    except (OSError, subprocess.SubprocessError) as exc:
        raise BarrierError("backend child identity cannot be inspected") from exc
    if any(result.returncode != 0 for result in (started, ids, details)):
        raise BarrierError("backend child identity cannot be inspected")
    fields = ids.stdout.split()
    start_text = started.stdout.strip()
    command = details.stdout.strip()
    if len(fields) != 2 or not fields[0].isdigit() or not fields[1].isdigit() or not start_text or not command:
        raise BarrierError("backend child identity is incomplete")
    executable = command.split(maxsplit=1)[0]
    return (start_text, int(fields[0]), int(fields[1]), executable)


def _verified_backend_child(*, root: Path, launcher_pid: int, port: int, binary: Path, binary_sha256: str, manifest_path: Path) -> tuple[int, tuple[str, int, int, str]]:
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    from fixture_ssh_identity_preflight import listener_pids, read_child_home, sha256_file, verify_launch_manifest

    manifest_path = _private_path(manifest_path, root)
    try:
        binary_info = binary.lstat()
    except OSError as exc:
        raise BarrierError("backend fixture binary is unavailable") from exc
    if stat.S_ISLNK(binary_info.st_mode) or not stat.S_ISREG(binary_info.st_mode) or binary_info.st_uid != os.getuid() or not (binary_info.st_mode & 0o111):
        raise BarrierError("backend fixture binary is not user-owned in a private bundle")
    current_dir = binary.parent
    private_bundle = False
    while current_dir != current_dir.parent:
        try:
            directory_info = current_dir.lstat()
        except OSError as exc:
            raise BarrierError("backend fixture bundle path cannot be inspected") from exc
        if stat.S_ISLNK(directory_info.st_mode) or not stat.S_ISDIR(directory_info.st_mode) or directory_info.st_uid != os.getuid():
            raise BarrierError("backend fixture bundle path is not user-owned")
        if directory_info.st_mode & 0o077 == 0:
            private_bundle = True
            break
        current_dir = current_dir.parent
    if not private_bundle:
        raise BarrierError("backend fixture bundle has no private owner directory")
    listeners = listener_pids(port)
    if len(listeners) != 1:
        raise BarrierError("API listener is missing or ambiguous")
    child_pid = next(iter(listeners))
    identity = _backend_child_identity(child_pid)
    if identity[2] != launcher_pid or identity[3] != str(binary.resolve(strict=True)):
        raise BarrierError("API listener is not the registered launcher's expected backend child")
    if sha256_file(binary) != binary_sha256:
        raise BarrierError("backend child binary differs from the current fixture build")
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        home = manifest["home_dir"]
        database = manifest["env"]["KANDEV_DATABASE_PATH"]
        manifest_port = manifest["port"]
    except (OSError, KeyError, TypeError, json.JSONDecodeError) as exc:
        raise BarrierError("supervisor launch manifest is unavailable or incomplete") from exc
    if manifest_port != port:
        raise BarrierError("supervisor manifest port differs from the listener")
    try:
        child_home, child_kandev_home, child_database = read_child_home(child_pid)
    except Exception as exc:
        raise BarrierError("backend child HOME cannot be verified") from exc
    if child_home != home or child_kandev_home != home or child_database != database:
        raise BarrierError("backend child HOME differs from supervisor manifest")
    try:
        verify_launch_manifest(manifest_path, executable=binary.resolve(strict=True), expected_home=home, expected_database=database, port=port)
    except Exception as exc:
        raise BarrierError("supervisor manifest does not match the current backend child") from exc
    return child_pid, identity


def _run_ssh_preflight(*, config_path: Path, root: Path, process_label: str, launcher_pid: int,
                       port: int, binary: Path, binary_sha256: str, launch_manifest: Path) -> dict[str, Any]:
    config_path = _private_path(config_path, root)
    try:
        config = json.loads(config_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise BarrierError("SSH preflight manifest is unreadable") from exc
    required = {"expected_home", "expected_database", "expected_key", "profile_source", "known_hosts", "host", "ssh_port", "host_fingerprint"}
    if not isinstance(config, dict) or config.keys() != required:
        raise BarrierError("SSH preflight manifest has missing or unsupported fields")
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    from fixture_ssh_identity_preflight import run as run_ssh_preflight
    args = argparse.Namespace(
        fixture_root=str(root), backend_label=process_label, backend_pid=launcher_pid,
        port=port, expected_home=config["expected_home"], expected_database=config["expected_database"],
        expected_key=config["expected_key"], profile_source=config["profile_source"],
        known_hosts=config["known_hosts"], host=config["host"], ssh_port=config["ssh_port"],
        host_fingerprint=config["host_fingerprint"], backend_binary=str(binary),
        backend_sha256=binary_sha256, launch_manifest=str(launch_manifest),
    )
    try:
        return run_ssh_preflight(args)
    except Exception as exc:
        raise BarrierError(f"SSH identity preflight failed: {type(exc).__name__}") from exc


def _private_root(path: Path) -> Path:
    try:
        info = path.lstat()
    except OSError as exc:
        raise BarrierError("fixture root is unavailable") from exc
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
        raise BarrierError("fixture root must be a real directory")
    if info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise BarrierError("fixture root must be user-owned and private")
    return path.resolve(strict=True)


def _private_path(path: Path, root: Path, *, absent_ok: bool = False) -> Path:
    try:
        info = path.lstat()
    except FileNotFoundError:
        if absent_ok:
            return path
        raise BarrierError("fixture evidence file is unavailable") from None
    except OSError as exc:
        raise BarrierError("fixture evidence path cannot be inspected") from exc
    if absent_ok:
        raise BarrierError("fixture evidence output already exists")
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
        raise BarrierError("fixture evidence file must be regular and non-symlink")
    if info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise BarrierError("fixture evidence file must be user-owned and private")
    resolved = path.resolve(strict=True)
    if root not in resolved.parents:
        raise BarrierError("fixture evidence file must remain under fixture root")
    return resolved


def _read_marker(path: Path, root: Path) -> dict[str, Any] | None:
    if not path.exists() and not path.is_symlink():
        return None
    path = _private_path(path, root)
    try:
        marker = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise BarrierError("response-loss marker is unreadable") from exc
    if (
        not isinstance(marker, dict)
        or marker.get("schema") != 1
        or marker.get("committed_http_status") != 200
        or not isinstance(marker.get("dropped_at_unix_ns"), int)
    ):
        raise BarrierError("response-loss marker does not prove a committed 200")
    return marker


def _inventory_snapshot(database: Path, root: Path) -> dict[str, Any]:
    database = _private_path(database, root)
    try:
        connection = sqlite3.connect(f"file:{database}?mode=ro", uri=True, timeout=2)
        connection.row_factory = sqlite3.Row
        rows = connection.execute(
            """SELECT id, task_id, session_id, agent_execution_id, executor_id,
                      runtime, status, metadata, CAST(created_at AS TEXT) AS created_at,
                      CAST(updated_at AS TEXT) AS updated_at
                 FROM executors_running WHERE runtime='nomad'"""
        ).fetchall()
    except (sqlite3.Error, OSError) as exc:
        raise BarrierError("fresh runtime inventory cannot be read") from exc
    finally:
        try:
            connection.close()
        except (UnboundLocalError, sqlite3.Error):
            pass
    if len(rows) != 1:
        raise BarrierError("response barrier requires exactly one fresh Nomad inventory row")
    row = dict(rows[0])
    try:
        metadata = json.loads(row["metadata"])
        state = metadata.get("nomad_environment_state")
    except (TypeError, json.JSONDecodeError) as exc:
        raise BarrierError("Nomad inventory envelope is invalid") from exc
    if not isinstance(state, dict):
        raise BarrierError("Nomad inventory envelope is missing")
    safe = {
        "inventory_id": row["id"],
        "task_id": row["task_id"],
        "session_id": row["session_id"],
        "execution_id": row["agent_execution_id"],
        "executor_id": row["executor_id"],
        "status": row["status"],
        "created_at": row["created_at"],
        "updated_at": row["updated_at"],
        "phase": state.get("phase"),
        "operation_id": state.get("operation_id"),
        "job_id": state.get("job_id"),
        "job_create_index": state.get("job_create_index"),
        "node_id": state.get("node_id"),
        "environment_id": state.get("environment_id"),
        "generation": state.get("generation"),
        "workspace_path": state.get("workspace_path"),
        "allocation_id": state.get("allocation_id"),
    }
    required = ("inventory_id", "task_id", "session_id", "execution_id", "executor_id", "phase", "operation_id", "job_id", "node_id", "environment_id", "generation", "workspace_path")
    if any(not safe.get(key) for key in required):
        raise BarrierError("Nomad inventory lacks exact task/operation identity")
    if safe["phase"] not in {"intent", "registered", "allocated"}:
        raise BarrierError("inventory is not at an eligible pre-ready phase")
    return safe


def _verify_committed_job(base_url: str, snapshot: dict[str, Any]) -> dict[str, Any]:
    request = urllib.request.Request(base_url.rstrip("/") + "/v1/job/" + snapshot["job_id"])
    try:
        with urllib.request.urlopen(request, timeout=3) as response:
            if response.status != 200:
                raise BarrierError("committed Nomad job cannot be read")
            job = json.loads(response.read())
    except BarrierError:
        raise
    except Exception as exc:
        raise BarrierError("committed Nomad job identity cannot be verified") from exc
    meta = job.get("Meta") if isinstance(job.get("Meta"), dict) else {}
    if (
        job.get("ID") != snapshot["job_id"]
        or job.get("Namespace") != "default"
        or meta.get("kandev.operation_id") != snapshot["operation_id"]
        or meta.get("kandev.environment_id") != snapshot["environment_id"]
        or meta.get("kandev.node_id") != snapshot["node_id"]
        or job.get("CreateIndex", 0) <= 0
    ):
        raise BarrierError("committed Nomad job does not match the exact fresh inventory owner")
    return {
        "job_id": job["ID"],
        "job_status": job.get("Status"),
        "job_create_index": job["CreateIndex"],
        "job_modify_index": job.get("JobModifyIndex"),
    }


def decide_barrier(marker: dict[str, Any] | None, snapshot: dict[str, Any] | None, *, now_ns: int, max_age_ns: int) -> str:
    """Pure fail-closed gate used by the live controller and its unit tests."""
    if marker is None:
        return "wait"
    age = now_ns - marker["dropped_at_unix_ns"]
    if age < 0 or age > max_age_ns:
        return "late_marker"
    if snapshot is None:
        return "missing_inventory"
    if snapshot.get("phase") == "ready":
        return "already_ready"
    if snapshot.get("phase") not in {"intent", "registered", "allocated"}:
        return "unsupported_phase"
    return "crash_exact_backend"


def guarded_crash(
    decision: str,
    registry: Any,
    process_label: str,
    record: dict[str, Any],
    *,
    child_pid: int,
    child_identity: tuple[str, int, int, str],
    child_identity_reader: Callable[[int], tuple[str, int, int, str]] = _backend_child_identity,
    signal_process: Callable[[int, int], None] = os.kill,
) -> str:
    """Crash the proved API child, then its exact owned launcher, without restart."""
    if decision != "crash_exact_backend":
        raise BarrierError("crash decision is not authorized by the barrier evidence")
    current = registry._process_identity(record["pid"])
    if current != (record["started_at"], record["pgid"]):
        raise BarrierError("launcher identity changed before guarded crash")
    child_now = child_identity_reader(child_pid)
    if child_now != child_identity or child_now[2] != record["pid"]:
        raise BarrierError("backend child identity changed before guarded crash")
    signal_process(record["pid"], signal.SIGSTOP)
    child_signal_sent = False
    try:
        if registry._process_identity(record["pid"]) != (record["started_at"], record["pgid"]):
            raise BarrierError("launcher identity changed while paused")
        if child_identity_reader(child_pid) != child_identity:
            raise BarrierError("backend child identity changed while launcher was paused")
        signal_process(child_pid, signal.SIGKILL)
        child_signal_sent = True
        return registry.crash(process_label, timeout=5)
    except Exception:
        # If no child signal was sent, restore the launcher. After the child is
        # killed, leave an uncertain fixture stopped for explicit reconciliation.
        if not child_signal_sent:
            if registry._process_identity(record["pid"]) == (record["started_at"], record["pgid"]):
                signal_process(record["pid"], signal.SIGCONT)
        raise


def _write_result(path: Path, root: Path, payload: dict[str, Any]) -> None:
    path = _private_path(path, root, absent_ok=True)
    data = (json.dumps(payload, sort_keys=True, separators=(",", ":")) + "\n").encode()
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
    except Exception:
        path.unlink(missing_ok=True)
        raise


def run_controller(
    *,
    fixture_root: Path,
    database: Path,
    marker_path: Path,
    ready_path: Path,
    result_path: Path,
    process_label: str,
    nomad_http: str,
    expected_node_id: str,
    backend_port: int,
    backend_binary: Path,
    backend_sha256: str,
    launch_manifest: Path,
    ssh_preflight_config: Path,
    timeout_seconds: float = 90,
    max_marker_age_seconds: float = 4,
    poll_seconds: float = 0.005,
    registry_factory: Callable[[Path], Any],
    now_ns: Callable[[], int] = time.time_ns,
    monotonic: Callable[[], float] = time.monotonic,
    sleep: Callable[[float], None] = time.sleep,
) -> dict[str, Any]:
    root = _private_root(fixture_root)
    if not nomad_http.startswith("http://127.0.0.1:") or not expected_node_id:
        raise BarrierError("controller must use the explicit fixture loopback Nomad HTTP API")
    if marker_path.parent.resolve(strict=True) != root or ready_path.parent.resolve(strict=True) != root or result_path.parent.resolve(strict=True) != root:
        raise BarrierError("controller markers must be direct children of fixture root")
    if _read_marker(marker_path, root) is not None:
        raise BarrierError("response-loss marker must be absent before the task request")
    _private_path(ready_path, root, absent_ok=True)
    _private_path(result_path, root, absent_ok=True)
    database = _private_path(database, root)
    connection = sqlite3.connect(f"file:{database}?mode=ro", uri=True, timeout=2)
    try:
        baseline = connection.execute("SELECT count(*) FROM executors_running WHERE runtime='nomad'").fetchone()[0]
    finally:
        connection.close()
    if baseline != 0:
        raise BarrierError("early crash barrier requires a genuinely empty Nomad inventory")

    registry = registry_factory(root)
    record = next((item for item in registry.read_records() if item["label"] == process_label), None)
    if record is None:
        raise BarrierError("exact backend process is not registered")
    identity = registry._process_identity(record["pid"])
    if identity != (record["started_at"], record["pgid"]):
        raise BarrierError("backend process identity does not match its ownership record")
    preflight = _run_ssh_preflight(
        config_path=ssh_preflight_config, root=root, process_label=process_label,
        launcher_pid=record["pid"], port=backend_port, binary=backend_binary,
        binary_sha256=backend_sha256, launch_manifest=launch_manifest,
    )
    child_pid, child_identity = _verified_backend_child(
        root=root, launcher_pid=record["pid"], port=backend_port, binary=backend_binary,
        binary_sha256=backend_sha256, manifest_path=launch_manifest,
    )

    ready_payload = {"schema_version": 1, "controller_ready": True, "backend_label": process_label, "backend_pid": record["pid"], "backend_child_pid": child_pid, "ssh_identity_preflight": True, "backend_sha256": preflight["backend_sha256"]}
    _write_result(ready_path, root, ready_payload)
    deadline = monotonic() + timeout_seconds
    max_age_ns = int(max_marker_age_seconds * 1_000_000_000)
    while monotonic() < deadline:
        marker = _read_marker(marker_path, root)
        if marker is None:
            current = registry._process_identity(record["pid"])
            if current != identity:
                raise BarrierError("backend exited or changed identity before the commit marker")
            sleep(poll_seconds)
            continue
        snapshot = _inventory_snapshot(database, root)
        if snapshot["node_id"] != expected_node_id:
            raise BarrierError("fresh inventory selected a different fixture worker")
        decision = decide_barrier(marker, snapshot, now_ns=now_ns(), max_age_ns=max_age_ns)
        if decision != "crash_exact_backend":
            raise BarrierError("commit marker arrived too late or inventory was no longer pre-ready")
        job = _verify_committed_job(nomad_http, snapshot)
        child_pid, child_identity = _verified_backend_child(
            root=root, launcher_pid=record["pid"], port=backend_port, binary=backend_binary,
            binary_sha256=backend_sha256, manifest_path=launch_manifest,
        )
        crash_result = guarded_crash(
            decision, registry, process_label, record,
            child_pid=child_pid, child_identity=child_identity,
        )
        result = {
            "schema_version": 1,
            "fixture_only": True,
            "barrier_passed": True,
            "marker_age_ms": round((now_ns() - marker["dropped_at_unix_ns"]) / 1_000_000),
            "committed_http_status": marker["committed_http_status"],
            "backend_label": process_label,
            "backend_pid": record["pid"],
            "backend_child_pid": child_pid,
            "backend_start_identity": record["started_at"],
            "crash_result": crash_result,
            "inventory": snapshot,
            "nomad_job": job,
        }
        _write_result(result_path, root, result)
        return result
    raise BarrierError("no committed response-loss marker arrived before the bounded deadline")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fixture-root", required=True, type=Path)
    parser.add_argument("--database", required=True, type=Path)
    parser.add_argument("--marker", required=True, type=Path)
    parser.add_argument("--ready", required=True, type=Path)
    parser.add_argument("--result", required=True, type=Path)
    parser.add_argument("--backend-label", required=True)
    parser.add_argument("--nomad-http", required=True)
    parser.add_argument("--node-id", required=True)
    parser.add_argument("--backend-port", required=True, type=int)
    parser.add_argument("--backend-binary", required=True, type=Path)
    parser.add_argument("--backend-sha256", required=True)
    parser.add_argument("--launch-manifest", required=True, type=Path)
    parser.add_argument("--ssh-preflight-config", required=True, type=Path)
    args = parser.parse_args()
    sys.path.insert(0, str(Path(__file__).parent))
    from owned_processes import OwnedProcessRegistry
    try:
        result = run_controller(
            fixture_root=args.fixture_root,
            database=args.database,
            marker_path=args.marker,
            ready_path=args.ready,
            result_path=args.result,
            process_label=args.backend_label,
            nomad_http=args.nomad_http,
            expected_node_id=args.node_id,
            backend_port=args.backend_port,
            backend_binary=args.backend_binary,
            backend_sha256=args.backend_sha256,
            launch_manifest=args.launch_manifest,
            ssh_preflight_config=args.ssh_preflight_config,
            registry_factory=OwnedProcessRegistry,
        )
    except Exception as exc:
        print(json.dumps({"barrier_passed": False, "error_category": type(exc).__name__, "reason": str(exc)}, sort_keys=True), flush=True)
        return 2
    print(json.dumps({"barrier_passed": True, "phase": result["inventory"]["phase"], "task_id": result["inventory"]["task_id"], "job_id": result["inventory"]["job_id"], "backend_pid": result["backend_pid"], "marker_age_ms": result["marker_age_ms"]}, sort_keys=True), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
