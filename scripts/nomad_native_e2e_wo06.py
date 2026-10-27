"""WO06 create-fixture scenarios: capacity admission and control-plane disconnect.

One invocation creates its own fresh fixture (private temp root and its own headless Kandev) through the shared
Runner, induces the work order's condition by polling real Nomad and worker state with deadlines (no fixed sleeps),
asserts, tears down through the explicit operator path and writes evidence.json. The first failed assertion stops the
run, keeps the fixture and prints the `cleanup-fixture` command.

Mutations are limited to (a) ending jobs recorded as fixture-created (purge disabled) and (b) compare-owner removal of the
fixture's own worker fence; processes are signalled only through Ops.kill_exact after pid, start time and command are
verified against a recorded identity. Release is an operator teardown, never production cleanup (WO07 owns that).
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import shlex
import socket
import sys
import threading
from pathlib import Path
from typing import Any, Callable

import nomad_native_e2e as base
import nomad_native_e2e_bootstrap as bootstrap
import nomad_native_e2e_fixture as fx
from nomad_native_e2e import PreflightError

LIMITATIONS = [
    "native reservations are scheduler accounting, not hard OS limits",
    "release is an operator teardown (job end with purge disabled and compare-owner fence removal), not production cleanup",
    "quarantine behaviour is covered by Go tests and WO07, not by this scenario",
    "node ACL scope is not per node; the dedicated-node check is the profile-pinned NodeID, datacenter and platform",
]
ROLES = ("agentctl", "acp", "child", "grandchild")
SCENARIOS = ("admission", "disconnect")
START_FENCE_REJECTION_EXIT_CODE = 73
_SHA256 = re.compile(r"[0-9a-f]{64}")
_NOMAD_HELPERS = ("executor", "logmon", "docklog")
RELAY_INSTRUCTION = (
    "the partition relay lives in the runner process and dies with it; before running cleanup-fixture, repoint or restart "
    "the throwaway Nomad client to the upstream RPC address {upstream}"
)


def _is_nomad_helper(command: str) -> bool:
    """Nomad's own executor, logmon and docklog processes name the allocation but are never fixture runtime processes."""
    tokens = command.split()
    if not tokens:
        return False
    if any(f"nomad {sub}" in command for sub in _NOMAD_HELPERS):
        return True
    return tokens[0].rsplit("/", 1)[-1] == "nomad"


# The runtime installs agentctl content-addressed as agentctl-<sha256>.
_AGENTCTL_NAME = re.compile(r"agentctl(-[0-9a-f]{64})?")


def _is_agentctl(command: str) -> bool:
    tokens = command.split()
    return bool(tokens) and _AGENTCTL_NAME.fullmatch(tokens[0].rsplit("/", 1)[-1]) is not None and not _is_nomad_helper(command)


def quarantine_marker_path(node_id: str, namespace: str) -> str:
    return f"/v1/var/kandev%2Fquarantine%2F{node_id}?namespace={namespace}"


def quarantine_operator_step(node_id: str, namespace: str) -> str:
    return (f"resolve the quarantine cause, then run `nomad var purge -namespace {namespace} kandev/quarantine/{node_id}` "
            f"and `nomad node eligibility -enable {node_id}`")


class TcpRelay:
    """Loopback TCP relay owned by the runner process. partition() severs every live connection and refuses new ones."""

    def __init__(self, listen: tuple[str, int], upstream: tuple[str, int]):
        self._bind, self.upstream = listen, upstream
        self._lock = threading.Lock()
        self._conns: set[socket.socket] = set()
        self._listener: socket.socket | None = None
        self.partitioned = False
        self._stopped = False

    @property
    def address(self) -> tuple[str, int]:
        return self._bind

    def start(self) -> None:
        with self._lock:
            self._open()

    def _open(self) -> None:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.bind(self._bind)
        sock.listen(32)
        sock.settimeout(0.1)
        self._bind = sock.getsockname()
        self._listener = sock
        threading.Thread(target=self._accept, args=(sock,), daemon=True).start()

    def _accept(self, sock: socket.socket) -> None:
        while True:
            try:
                client, _ = sock.accept()
            except socket.timeout:
                if self._listener is not sock:
                    return
                continue
            except OSError:
                return
            try:
                server = socket.create_connection(self.upstream, timeout=5)
            except OSError:
                client.close()
                continue
            with self._lock:
                if self._listener is not sock:
                    client.close()
                    server.close()
                    return
                self._conns.update((client, server))
            for src, dst in ((client, server), (server, client)):
                threading.Thread(target=self._pump, args=(src, dst), daemon=True).start()

    def _pump(self, src: socket.socket, dst: socket.socket) -> None:
        try:
            while True:
                data = src.recv(65536)
                if not data:
                    break
                dst.sendall(data)
        except OSError:
            pass
        finally:
            with self._lock:
                for sock in (src, dst):
                    self._close(sock)
                    self._conns.discard(sock)

    @staticmethod
    def _close(sock: socket.socket) -> None:
        try:
            sock.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass
        sock.close()

    def _sever(self) -> None:
        listener, self._listener = self._listener, None
        if listener is not None:
            listener.close()
        for sock in list(self._conns):
            self._close(sock)
        self._conns.clear()

    def connection_count(self) -> int:
        with self._lock:
            return len(self._conns)

    def partition(self) -> None:
        with self._lock:
            self.partitioned = True
            self._sever()

    def heal(self) -> None:
        with self._lock:
            if self._stopped:
                raise PreflightError("relay is stopped")
            self.partitioned = False
            if self._listener is None:
                self._open()

    def stop(self) -> None:
        with self._lock:
            self._stopped = True
            self._sever()


def _endpoint(text: str) -> tuple[str, int]:
    host, port = text.rsplit(":", 1)
    return ("127.0.0.1" if host == "localhost" else host), int(port)


def _ident(rows: list[dict[str, Any]]) -> list[tuple[int, str]]:
    return sorted((row["pid"], row["started"]) for row in rows)


def _self_and_ancestors(rows: list[dict[str, Any]]) -> set[int]:
    """This process and its ancestors: never part of a fixture tree, so a teardown cannot signal itself or its caller."""
    by_pid = {row["pid"]: row for row in rows}
    out, cursor = set(), os.getpid()
    while cursor > 1 and cursor not in out:
        out.add(cursor)
        cursor = by_pid[cursor]["ppid"] if cursor in by_pid else (os.getppid() if cursor == os.getpid() else 0)
    return out


_SAFE_ID = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]*")


def _needles(env: dict[str, Any]) -> list[str]:
    return [value for value in (env.get("allocation_id"), env.get("job_id"), env.get("environment_id")) if value]


class Scenario:
    """Observation and operator helpers shared by both scenarios and cleanup-fixture."""

    def __init__(self, runner: fx.Runner, cfg: dict[str, Any] | None, name: str):
        self.r, self.cfg, self.name, self.ops, self.data = runner, cfg, name, runner.ops, runner.evidence.data
        self.nomad_cfg = runner.state["nomad"]
        self.envs: dict[str, dict[str, Any]] = {env["label"]: env for env in runner.state.get("environments", [])}
        self.known_jobs: set[str] = set()
        self.observed: list[dict[str, Any]] = []
        self.cleanup_mode = False

    def relay_instruction(self) -> str:
        return RELAY_INSTRUCTION.format(upstream=(self.r.state.get("relay") or {}).get("upstream", "the configured upstream"))

    # -- polling and Nomad reads ---------------------------------------------------------------
    def poll(self, probe: Callable[[], Any], failure: str, timeout: float | None = None) -> Any:
        return self.r.poll(timeout or self.ops.scenario_timeout, self.ops.poll_interval, probe, failure)

    def soft(self, fn: Callable[[], Any]) -> Any:
        try:
            return fn()
        except PreflightError:
            return None

    def get_or_none(self, path: str) -> Any:
        try:
            return self.r.nomad(path)
        except PreflightError as exc:
            if "status 404" in str(exc):
                return None
            raise

    def job_allocs(self, job_id: str) -> list[dict[str, Any]]:
        return self.get_or_none(f"/v1/job/{job_id}/allocations") or []

    def alloc(self, allocation_id: str) -> dict[str, Any]:
        return self.r.nomad(f"/v1/allocation/{allocation_id}")

    def job_status(self, job_id: str) -> str | None:
        job = self.get_or_none(f"/v1/job/{job_id}")
        return None if job is None else job.get("Job", job).get("Status")

    def node(self) -> dict[str, Any]:
        return self.r.nomad(f"/v1/node/{self.nomad_cfg['node_id']}")

    def kandev_jobs(self) -> set[str]:
        return {item["ID"] for item in self.r.nomad("/v1/jobs?prefix=kandev-") if isinstance(item, dict) and "ID" in item}

    def verify_node_identity(self) -> dict[str, Any]:
        """The node must be the configured dedicated worker: same id, datacenter and platform."""
        node = self.poll(lambda: self.get_or_none(f"/v1/node/{self.nomad_cfg['node_id']}"), "the throwaway Nomad client did not register its node before timeout")
        attrs = node.get("Attributes") or {}
        platform = f"{attrs.get('kernel.name')}/{attrs.get('cpu.arch')}"
        if node.get("ID") != self.nomad_cfg["node_id"] or node.get("Datacenter") != self.cfg["nomad"]["datacenter"]:
            raise PreflightError("Nomad node id or datacenter differs from the bootstrap config; refusing to mutate it")
        if platform != self.cfg["nomad"]["platform"]:
            raise PreflightError("Nomad node platform differs from the bootstrap config; refusing to mutate it")
        return {"node_id": node["ID"], "datacenter": node["Datacenter"], "platform": platform, "meta_compared": False,
                "node_pool": self.check_node_pool(node)}

    def check_node_pool(self, node: dict[str, Any]) -> str:
        """The node's pool must agree with the profile and exist on the server; otherwise a launch would stay pending."""
        node_pool = node.get("NodePool") or "default"
        wanted = (self.cfg.get("pool") or {}).get("node_pool")
        if wanted and node_pool != wanted:
            raise PreflightError(f"the fixture node is in node pool {node_pool!r} but pool.node_pool {wanted!r} is configured; "
                                 "a launch would stay pending with no eligible node. Align pool.node_pool or the node's client node_pool")
        if node_pool != "default" and self.get_or_none(f"/v1/node/pool/{node_pool}") is None:
            raise PreflightError(f"the fixture node is in node pool {node_pool!r}, which does not exist on the Nomad server; "
                                 "a launch would stay pending with no eligible node")
        return node_pool

    def quarantine_marker(self) -> Any:
        """The node's quarantine Variable in the config namespace, or None when absent (404)."""
        return self.get_or_none(quarantine_marker_path(self.nomad_cfg["node_id"], self.nomad_cfg["namespace"]))

    def open_eligibility_window(self) -> dict[str, Any]:
        """Make the fixture node ineligible so a released reservation cannot place queued work before the fence is gone."""
        node_id = self.nomad_cfg["node_id"]
        original = self.node().get("SchedulingEligibility")
        window = {"node_id": node_id, "original": original, "changed_by_runner": False, "restored": False,
                  "restore_with": f"nomad node eligibility -enable {node_id}"}
        if original == "eligible":
            window["changed_by_runner"] = True
            self.r.save_state(eligibility_window=window)
            self.data["eligibility_window"] = window
            self.ops.nomad_set_eligibility(self.nomad_cfg["address"], self.r.token, self.nomad_cfg["region"], node_id, False)
            self.poll(lambda: self.node().get("SchedulingEligibility") == "ineligible", "node did not become ineligible")
        self.r.save_state(eligibility_window=window)
        self.data["eligibility_window"] = window
        return window

    def restore_eligibility(self, strict: bool) -> str:
        """Restore the recorded eligibility, only when this runner changed it and the worker fence is absent."""
        window = self.r.state.get("eligibility_window")
        if not window or not window["changed_by_runner"] or window["restored"]:
            return "not needed"
        node_id, namespace = window["node_id"], self.nomad_cfg["namespace"]
        try:
            marker = self.quarantine_marker()
        except PreflightError as exc:
            if strict:
                raise
            return f"left ineligible: quarantine marker could not be read ({exc}); once readable, {quarantine_operator_step(node_id, namespace)}"
        if marker is not None:
            step_text = quarantine_operator_step(node_id, namespace)
            window["quarantine_marker"] = f"present: kandev/quarantine/{node_id}"
            window["operator_step"] = step_text
            self.r.save_state(eligibility_window=window)
            self.data["eligibility_window"] = window
            if strict:
                raise PreflightError(f"quarantine marker kandev/quarantine/{node_id} exists; the node is left ineligible; {step_text}")
            return f"left ineligible: quarantine marker kandev/quarantine/{node_id} exists; {step_text}"
        if self.r.fence() is not None:
            if strict:
                raise PreflightError("worker fence is still present; the node is left ineligible")
            return f"left ineligible: worker fence still present; restore with `{window['restore_with']}` after removing it"
        self.ops.nomad_set_eligibility(self.nomad_cfg["address"], self.r.token, self.nomad_cfg["region"], window["node_id"], window["original"] == "eligible")
        self.poll(lambda: self.node().get("SchedulingEligibility") == window["original"], "node eligibility was not restored")
        window["restored"] = True
        self.r.save_state(eligibility_window=window)
        self.data["eligibility_window"] = window
        return "restored"

    def node_ready(self) -> bool:
        node = self.node()
        return node.get("Status") == "ready" and node.get("SchedulingEligibility") == "eligible"

    # -- capacity -----------------------------------------------------------------------------
    def capacity_precondition(self) -> dict[str, Any]:
        node = self.node()
        try:
            total_cpu, total_mem = node["NodeResources"]["Cpu"]["CpuShares"], node["NodeResources"]["Memory"]["MemoryMB"]
            reserved = node.get("ReservedResources") or {}
            reserved_cpu = (reserved.get("Cpu") or {}).get("CpuShares") or 0
            reserved_mem = (reserved.get("Memory") or {}).get("MemoryMB") or 0
            numbers = [total_cpu, total_mem, reserved_cpu, reserved_mem]
        except (KeyError, TypeError):
            raise PreflightError("Nomad node resources are unreadable") from None
        if any(not isinstance(item, int) or isinstance(item, bool) or item < 0 for item in numbers):
            raise PreflightError("Nomad node resources are unreadable")
        cpu, mem = self.cfg["cpu_mhz"], self.cfg["memory_mb"]
        avail_cpu, avail_mem = total_cpu - reserved_cpu, total_mem - reserved_mem
        result = {
            "request_cpu_mhz": cpu, "request_memory_mb": mem, "total_cpu_mhz": total_cpu, "reserved_cpu_mhz": reserved_cpu,
            "available_cpu_mhz": avail_cpu, "total_memory_mb": total_mem, "reserved_memory_mb": reserved_mem, "available_memory_mb": avail_mem,
        }
        self.data["capacity"] = result
        if cpu > avail_cpu or mem > avail_mem:
            raise PreflightError("capacity precondition failed: one environment does not fit the node's available resources")
        exhausted = [name for name, need, avail in (("cpu", cpu, avail_cpu), ("memory", mem, avail_mem)) if 2 * need > avail]
        if not exhausted:
            raise PreflightError("capacity precondition failed: two environments both fit the node, so admission cannot be exhausted")
        result["exhausted_dimensions"] = exhausted
        return result

    # -- remote processes and fence -----------------------------------------------------------
    def ps_rows(self) -> list[dict[str, Any]]:
        code, out = self.r.ssh("LC_ALL=C ps -eww -o pid=,ppid=,pgid=,lstart=,command=")
        if code != 0:
            raise PreflightError("remote process listing failed")
        rows = []
        for line in out.splitlines():
            match = fx._PS_ROW.match(line)
            if match:
                rows.append({"pid": int(match.group(1)), "ppid": int(match.group(2)), "pgid": int(match.group(3)),
                             "started": " ".join(match.group(4).split()), "command": match.group(5)})
        if not rows and out.strip():
            raise PreflightError("remote process listing was not empty but no row parsed; refusing to continue")
        return rows

    def focus(self, env: dict[str, Any]) -> None:
        """Point worker commands and fence paths at the node a two-node pool environment was placed on."""
        if not env.get("attach") and env.get("node_id") and getattr(self.r, "nomad", None) and self.r.state.get("pool_state_root"):
            meta = self.soft(lambda: self.r.nomad(f"/v1/node/{env['node_id']}").get("Meta")) or {}
            if meta.get("kandev.attach") and meta.get("kandev.state_root"):
                env = {**env, "attach": meta["kandev.attach"], "state_root": meta["kandev.state_root"]}
        if env.get("attach") and env.get("state_root"):
            self.r.use_node(env["attach"], env["state_root"])
            if env.get("environment_id"):
                self.r.state["fence_environment_id"] = env["environment_id"]

    def tree(self, env: dict[str, Any]) -> list[dict[str, Any]]:
        self.focus(env)
        """Fixture runtime processes naming the environment's allocation, job or environment id, plus all their descendants.

        Nomad's own executor, logmon and docklog processes are excluded. Only a root whose argv[0] is agentctl is labelled
        agentctl; any other root is labelled unidentified.
        """
        needles = _needles(env)
        protected = _self_and_ancestors(self.ps_rows())
        rows = [row for row in self.ps_rows() if row["pid"] not in protected]
        if getattr(self.r, "active_attach", "ssh") == "local":
            # The Kandev backend shares this host and runs short-lived git commands whose paths name the environment;
            # only a process started from the allocation's own directory roots the environment's tree.
            roots = [value for value in (env.get("allocation_id"), env.get("job_id")) if value]
            members = {row["pid"]: row for row in rows if row["command"].split(None, 1)[:1] and any(r in row["command"].split(None, 1)[0] for r in roots)}
        else:
            members = {row["pid"]: row for row in rows if any(needle in row["command"] for needle in needles)}
        matched = set(members)
        grew = True
        while grew:
            grew = False
            for row in rows:
                if row["pid"] not in members and row["ppid"] in members:
                    members[row["pid"]] = row
                    grew = True
        members = {pid: row for pid, row in members.items() if not _is_nomad_helper(row["command"])}
        out = []
        for pid, row in members.items():
            depth, cursor = 0, row
            while cursor["ppid"] in members and depth < 32:
                cursor, depth = members[cursor["ppid"]], depth + 1
            if depth == 0:
                role = "agentctl" if _is_agentctl(row["command"]) else "unidentified"
            else:
                role = ROLES[min(depth, 3)]
            out.append({
                "pid": pid, "ppid": row["ppid"], "pgid": row["pgid"], "started": row["started"], "depth": depth, "role": role,
                "needle_matched": pid in matched, "cmd_sha256": hashlib.sha256(row["command"].encode()).hexdigest(),
            })
        return sorted(out, key=lambda item: item["pid"])

    def verify_agentctl(self, item: dict[str, Any]) -> None:
        """Refuse unless the recorded process is, right now, the same agentctl process (pid, start time and argv[0])."""
        if item.get("role") != "agentctl":
            raise PreflightError("the recorded leader is not an identified agentctl process; refusing to signal it")
        row = next((r for r in self.ps_rows() if r["pid"] == item["pid"]), None)
        if row is None:
            return
        if row["started"] != item["started"] or not _is_agentctl(row["command"]):
            raise PreflightError(f"process {item['pid']} is not the recorded agentctl process; refusing to signal it")

    def alive(self, identities: list[dict[str, Any]]) -> list[dict[str, Any]]:
        current = {(row["pid"], row["started"]) for row in self.ps_rows()}
        return [item for item in identities if (item["pid"], item["started"]) in current]

    def remove_fence(self, expected_sha: str) -> str:
        """Compare-owner removal: the remote side hashes the owner file and removes only on an exact match."""
        if not _SHA256.fullmatch(expected_sha):
            raise PreflightError("recorded fence hash is invalid")
        path = shlex.quote(fx._fence_path(self.r.state))
        command = (
            f'p={path}; [ -e "$p" ] || exit 3; h=$(cat -- "$p/owner" | {{ sha256sum 2>/dev/null || shasum -a 256; }} | cut -d" " -f1); '
            f'[ "$h" = {expected_sha} ] || exit 4; rm -f -- "$p/owner" && rmdir -- "$p"'
        )
        code, _ = self.r.ssh(command)
        if code == 3:
            return "absent"
        if code == 4:
            raise PreflightError("fence owner no longer matches the recorded hash; refusing to remove another owner's fence")
        if code != 0:
            raise PreflightError("fence removal failed")
        if self.r.fence() is not None:
            raise PreflightError("fence is still present after compare-owner removal")
        return "removed"

    # -- environments --------------------------------------------------------------------------
    def save_envs(self) -> None:
        self.r.save_state(environments=list(self.envs.values()))

    def record_env(self, label: str, task: dict[str, str], inventory: dict[str, Any], **extra: Any) -> dict[str, Any]:
        env = {"label": label, "task_id": task["id"], "session_id": task["session_id"], "environment_id": inventory.get("environment_id"),
               "job_id": inventory.get("job_id"), "allocation_id": inventory.get("allocation_id"), "node_id": inventory.get("node_id"),
               "fence_sha256": None, "tree": [], **extra}
        self.envs[label] = env
        if self.r.state.get("pool_state_root") and env["environment_id"]:
            self.r.state["fence_environment_id"] = env["environment_id"]
        self.save_envs()
        self.data.setdefault("identities", {})[label] = {key: env[key] for key in ("task_id", "session_id", "environment_id", "job_id", "allocation_id", "node_id")}
        return env

    def stable_tree(self, env: dict[str, Any], minimum_depth: int = 0) -> list[dict[str, Any]]:
        """Two consecutive equal snapshots, so a transient helper that exits during startup is never recorded."""
        previous: list[Any] = []
        current_box: list[list[dict[str, Any]]] = []

        def stable() -> list[dict[str, Any]] | None:
            current = self.tree(env)
            current_box.append(current)
            settled = bool(previous) and _ident(current) == previous[-1] and any(item["depth"] >= minimum_depth for item in current)
            previous[:] = [_ident(current)]
            return current if settled else None

        try:
            return self.poll(stable, f"the remote process tree of environment {env['label']} did not settle before timeout")
        except PreflightError:
            self.data.setdefault("tree_diagnostics", {})[env["label"]] = self.tree_diagnostics(env, previous, current_box)
            raise

    def tree_diagnostics(self, env: dict[str, Any], previous: list[Any], last: list[list[dict[str, Any]]]) -> dict[str, Any]:
        """Evidence for a tree that never settled: the node in use, what was searched for, what the process list held and the last snapshots."""
        diagnostics: dict[str, Any] = {"active_attach": getattr(self.r, "active_attach", None), "state_root": self.r.state.get("pool_state_root"),
                                       "needles": _needles(env), "last_identities": previous[-1] if previous else None,
                                       "last_snapshots": last[-2:]}
        try:
            rows = self.ps_rows()
            protected = _self_and_ancestors(rows)
            named = [row for row in rows if any(needle in row["command"] for needle in _needles(env))]
            diagnostics.update(ps_row_count=len(rows), protected_pids=sorted(protected), named_rows=[
                {"pid": row["pid"], "ppid": row["ppid"], "started": row["started"], "protected": row["pid"] in protected,
                 "nomad_helper": _is_nomad_helper(row["command"]), "agentctl": _is_agentctl(row["command"]), "command_head": row["command"][:200]}
                for row in named[:20]])
        except PreflightError as exc:
            diagnostics["ps_error"] = str(exc)
        return diagnostics

    def await_ready(self, label: str, task: dict[str, str]) -> dict[str, Any]:
        def ready() -> dict[str, Any] | None:
            inventory = self.soft(lambda: self.r.inventory(task["session_id"], task["id"]))
            if inventory and inventory["phase"] == "ready" and inventory.get("allocation_id") and fx._JOB_ID.fullmatch(str(inventory.get("job_id"))):
                return inventory
            return None

        inventory = self.poll(ready, f"environment {label} did not reach durable inventory phase ready before timeout")
        env = self.record_env(label, task, inventory)
        self.poll(lambda: self.soft(lambda: self.alloc(inventory["allocation_id"]).get("ClientStatus") == "running"),
                  f"Nomad allocation of environment {label} did not reach running before timeout")
        allocation = self.alloc(inventory["allocation_id"])
        tasks = (allocation.get("AllocatedResources") or {}).get("Tasks") or {}
        got_cpu = sum(((item.get("Cpu") or {}).get("CpuShares") or 0) for item in tasks.values())
        got_mem = sum(((item.get("Memory") or {}).get("MemoryMB") or 0) for item in tasks.values())
        allocated = {"cpu_mhz": got_cpu, "memory_mb": got_mem}
        self.data["identities"][label]["allocated_resources"] = allocated
        if got_cpu != self.cfg["cpu_mhz"] or got_mem != self.cfg["memory_mb"]:
            raise PreflightError(f"allocation resources {allocated} differ from the request cpu_mhz={self.cfg['cpu_mhz']} memory_mb={self.cfg['memory_mb']}")
        nomad = self.r.nomad_snapshot(inventory)
        tree = self.stable_tree(env)
        fence = self.r.fence()
        if not tree:
            raise PreflightError(f"environment {label} has no remote process tree")
        if fence is None:
            raise PreflightError(f"environment {label} has no worker fence at {fx._fence_path(self.r.state)}")
        if not any(item["role"] == "agentctl" and item["depth"] == 0 for item in tree):
            raise PreflightError(f"environment {label} has no identifiable agentctl root process (argv[0] agentctl); the process tree is not trustworthy")
        env.update(fence_sha256=fence, tree=tree)
        self.save_envs()
        self.data["identities"][label].update(fence_owner_sha256=fence, remote_processes=len(tree), roles=[item["role"] for item in tree])
        return {"inventory": inventory, "nomad": nomad, "tree": tree, "fence": fence}

    def await_queued(self, label: str, task: dict[str, str], exhausted: list[str]) -> dict[str, Any]:
        """Poll until the environment's job exists in Nomad with no allocation and a blocked evaluation naming the exhausted dimension."""
        def registered() -> dict[str, Any] | None:
            inventory = self.soft(lambda: self.r.inventory(task["session_id"], task["id"]))
            job_id = (inventory or {}).get("job_id")
            if isinstance(job_id, str) and fx._JOB_ID.fullmatch(job_id) and self.get_or_none(f"/v1/job/{job_id}") is not None:
                return inventory
            return None

        inventory = self.poll(registered, f"the Nomad job of environment {label} was not registered before timeout")
        env = self.record_env(label, task, inventory)
        self.known_jobs.add(env["job_id"])

        def queued() -> dict[str, Any] | None:
            if self.job_allocs(env["job_id"]):
                raise PreflightError(f"environment {label} was allocated although the node cannot hold a second environment")
            evals = self.get_or_none(f"/v1/job/{env['job_id']}/evaluations") or []
            named = {str(dim).lower() for item in evals for group in (item.get("FailedTGAllocs") or {}).values() for dim in (group.get("DimensionExhausted") or {})}
            blocked = any(item.get("Status") == "blocked" for item in evals)
            if blocked and any(dim in named for dim in exhausted):
                return {"blocked_evaluation": True, "dimensions_exhausted": sorted(named)}
            return None

        proof = self.poll(queued, f"environment {label} never showed a blocked evaluation naming {exhausted}")
        if self.job_allocs(env["job_id"]):
            raise PreflightError(f"environment {label} was allocated after the queued proof")
        if self.tree(env):
            raise PreflightError(f"remote processes reference queued environment {label}")
        self.data["identities"][label].update(queued=proof, allocations=0, remote_processes=0)
        return proof

    # -- operator release ------------------------------------------------------------------------
    def release(self, label: str, kill_first: bool, strict: bool = True, watch: Callable[[], Any] | None = None) -> dict[str, Any]:
        env = self.envs[label]
        self.focus(env)
        job_id = env["job_id"]
        out: dict[str, Any] = {"label": label, "job_id": job_id, "production_cleanup": False, "release": "operator-teardown"}
        merged = {(item["pid"], item["started"]): item for item in (env.get("tree") or [])}
        if env.get("job_id"):
            merged.update({(item["pid"], item["started"]): item for item in self.tree(env)})
        rows = sorted(merged.values(), key=lambda item: -item["depth"])
        out["recorded_processes"] = _ident(rows)
        if kill_first:
            out["kill_exact"] = {str(item["pid"]): self.ops.kill_exact(self.r.ssh, item, _needles(env)) for item in rows}
        if self.job_status(job_id) is None:
            out["job"] = "absent"
        else:
            self.ops.nomad_stop(self.nomad_cfg["address"], self.r.token, self.nomad_cfg["namespace"], job_id)
            out["job"] = "ended purge=false"
        def watched(fn: Callable[[], Any]) -> Callable[[], Any]:
            def probe() -> Any:
                if watch:
                    watch()
                return fn()

            return probe

        def terminal() -> bool:
            if self.cleanup_mode and self.r.state.get("relay") and self.node().get("Status") != "ready":
                raise PreflightError(f"the Nomad client is not ready, so the stop of {label} cannot be acknowledged; {self.relay_instruction()}")
            return all(item.get("ClientStatus") in fx.TERMINAL_ALLOCATION_STATES for item in self.job_allocs(job_id))

        try:
            self.poll(watched(terminal), f"allocations of {label} did not become terminal after the job was ended")
        except PreflightError as exc:
            if self.cleanup_mode and self.r.state.get("relay") and "upstream RPC" not in str(exc):
                raise PreflightError(f"{exc}; {self.relay_instruction()}") from None
            raise
        self.poll(watched(lambda: not self.alive(rows)), f"recorded processes of {label} are still present on the worker")
        out["processes_gone"] = True
        expected = env.get("fence_sha256") or (self.owned_fence_sha(env) if self.cleanup_mode else None)
        if expected:
            try:
                out["fence"] = self.remove_fence(expected)
            except PreflightError as exc:
                if strict:
                    raise
                out["fence"] = f"left in place: {exc}"
        if self.cleanup_mode and env.get("environment_id") and self.pool_node_root(env):
            out["pool_leftovers"] = self.remove_pool_leftovers(env, strict)
        if watch:
            watch()
        env["released"] = True
        self.save_envs()
        return out

    def pool_node_root(self, env: dict[str, Any]) -> str | None:
        """The state root of the node the environment ran on: recorded, else from the node's metadata, else the pinned pool root."""
        if env.get("state_root"):
            return env["state_root"]
        if env.get("node_id") and self.r.state.get("pool_state_root"):
            meta = self.soft(lambda: self.r.nomad(f"/v1/node/{env['node_id']}").get("Meta")) or {}
            return meta.get("kandev.state_root") or self.r.state["pool_state_root"]
        return self.r.state.get("pool_state_root")

    def remove_pool_leftovers(self, env: dict[str, Any], strict: bool) -> dict[str, Any]:
        """Operator teardown of a placed environment's node files once its job is dead: the git worktree and its
        per-generation branch, the workspace, the fence directory and the allocation directory."""
        root, env_id, job_id = self.pool_node_root(env), env["environment_id"], env["job_id"]
        if not root.startswith("/") or not _SAFE_ID.fullmatch(env_id) or not _SAFE_ID.fullmatch(job_id) or ".." in root:
            raise PreflightError("recorded pool paths are not safe to remove")
        ws, fence, alloc = (f"{root}/workspaces/environments/{env_id}", f"{root}/environments/{env_id}", f"{root}/pool/allocations/{job_id}")
        q = shlex.quote
        prefix = f"kandev/pool/{env_id}-g"
        script = (f'for d in {q(ws)}/*/*/; do [ -f "${{d}}.git" ] || continue; '
                  f'common=$(git -C "$d" rev-parse --path-format=absolute --git-common-dir) || continue; '
                  f'b=$(git -C "$d" rev-parse --abbrev-ref HEAD); '
                  f'git -C "$d" worktree remove --force "$d"; '
                  f'case "$b" in {q(prefix)}*) git --git-dir="$common" branch -D "$b";; esac; '
                  f'git --git-dir="$common" worktree prune; done; '
                  f'rm -rf {q(ws)} {q(fence)} {q(alloc)}')
        self.r.ssh(script)
        remaining = [path for path in (ws, fence, alloc) if self.r.ssh(f"[ -e {q(path)} ] || [ -L {q(path)} ]")[0] == 0]
        report = {"removed": not remaining, "branch_prefix": prefix}
        if remaining:
            report["remaining"] = remaining
            if strict:
                raise PreflightError(f"pool leftovers of {env['label']} could not be removed: {', '.join(remaining)}")
        return report

    def owned_fence_sha(self, env: dict[str, Any]) -> str | None:
        """Hash of the current fence when its owner names this environment's job; a run that failed before recording it still releases it."""
        owner = self.r.fence_owner()
        if owner is None or env["job_id"] not in owner:
            return None
        return hashlib.sha256(owner.encode()).hexdigest()

    def assert_jobs_ended(self) -> dict[str, str]:
        statuses: dict[str, str] = {}

        def ended() -> bool:
            for label, env in self.envs.items():
                status = self.job_status(env["job_id"])
                statuses[label] = status or "absent"
                if status not in (None, "dead") or any(item.get("ClientStatus") not in fx.TERMINAL_ALLOCATION_STATES for item in self.job_allocs(env["job_id"])):
                    return False
            return True

        self.poll(ended, "fixture jobs are still running after the operator teardown")
        return statuses

    def stop_processes(self, report: dict[str, Any]) -> None:
        r = self.r
        backend = next((item for item in r.registry.read_records() if item["label"] == fx.PROCESS_LABEL), None)
        if backend is not None:
            if r.registry.stop(fx.PROCESS_LABEL, timeout=15) not in ("stopped", "already_absent"):
                raise PreflightError("fixture backend did not stop through its exact owned process record")
            r.verify_gone(backend["pid"])
            report["backend_pid_stopped"] = backend["pid"]
            report["session_stop"] = "skipped: the fixture jobs are ended by the operator path and the fixture database is discarded"
        else:
            report["session_stop"] = "skipped: the backend is not running"
        daemon = next((item for item in r.registry.read_records() if item["label"] == fx.GIT_DAEMON_LABEL), None)
        if daemon is not None:
            r._stop_daemon(daemon)
            report["git_daemon_pid_stopped"] = daemon["pid"]

    def finish(self, report: dict[str, Any]) -> dict[str, Any]:
        report["jobs"] = self.assert_jobs_ended()
        report["eligibility"] = self.restore_eligibility(strict=False)
        self.stop_processes(report)
        self.r.revoke_scoped_token(report)
        self.r.save_state(torn_down=True)
        (self.r.home / ".ssh" / "id_ed25519").unlink(missing_ok=True)
        report["ssh_identity_removed"] = True
        return report


def stop_stray_jobs(scn: "Scenario") -> list[dict[str, Any]]:
    """Stops every Kandev job created since the fixture started that no recorded environment owns and that is not dead.

    A launch that failed before its runtime identity was recorded leaves a running job the runner never saw. The job
    is stopped with purge off, its allocations are awaited, and a remaining worker process is a failure.
    """
    runner = scn.r
    baseline = set(runner.state.get("baseline_jobs") or [])
    known = {env["job_id"] for env in scn.envs.values() if env.get("job_id")}
    stopped: list[dict[str, Any]] = []
    for job_id in sorted(scn.kandev_jobs() - baseline - known):
        status = scn.job_status(job_id)
        if status in (None, "dead"):
            continue
        allocs = scn.job_allocs(job_id)
        scn.ops.nomad_stop(scn.nomad_cfg["address"], runner.token, scn.nomad_cfg["namespace"], job_id)
        scn.poll(lambda j=job_id: all(item.get("ClientStatus") in fx.TERMINAL_ALLOCATION_STATES for item in scn.job_allocs(j)),
                 f"allocations of stray job {job_id} did not become terminal after it was stopped")
        remaining: list[dict[str, Any]] = []
        for alloc in allocs:
            meta = scn.r.nomad(f"/v1/node/{alloc['NodeID']}").get("Meta") or {}
            probe = {"label": f"stray-{job_id}", "job_id": job_id, "allocation_id": alloc["ID"]}
            if meta.get("kandev.attach") and meta.get("kandev.state_root"):
                probe.update(attach=meta["kandev.attach"], state_root=meta["kandev.state_root"])
            remaining += scn.tree(probe)
        if remaining:
            raise PreflightError(f"stray job {job_id} still has worker processes after it was stopped")
        stopped.append({"job_id": job_id, "previous_status": status, "allocations": len(allocs), "purged": False})
    return stopped


def cleanup_fixture(runner: fx.Runner) -> dict[str, Any]:
    """Idempotent operator teardown of every recorded environment of an admission or disconnect fixture."""
    scn = Scenario(runner, None, runner.state["scenario"])
    scn.cleanup_mode = True
    report: dict[str, Any] = {"production_cleanup": False, "release": "operator-teardown", "released": []}
    if runner.state.get("torn_down"):
        report["already_torn_down"] = True
        return report
    for label in sorted(scn.envs, reverse=True):
        if scn.envs[label].get("job_id"):
            report["released"].append(scn.release(label, kill_first=True, strict=False))
    report["stray_jobs"] = stop_stray_jobs(scn)
    if runner.state.get("relay"):
        report["relay"] = "gone: the relay lived in the runner process that has exited"
    return scn.finish(report)


# -- admission -----------------------------------------------------------------------------------------
def _base_report(scn: Scenario) -> dict[str, Any]:
    return {"scenario": scn.name, "production_cleanup": False, "release": "operator-teardown", "limitations": LIMITATIONS}


def drive_admission(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    step, scn = runner.step, Scenario(runner, cfg, "admission")
    scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
    step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
    capacity = step("capacity-precondition", scn.capacity_precondition)
    step("start-git-daemon", runner.start_git_daemon)
    step("start-backend", lambda: runner.start_backend(None))
    step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
    task_a = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
    snap_a = step("await-a-running", lambda: scn.await_ready("A", task_a))

    def create_b() -> None:
        task_b = runner.create_task("Nomad admission fixture B")
        scn.await_queued("B", task_b, capacity["exhausted_dimensions"])
        env_a = scn.envs["A"]
        if runner.inventory(task_a["session_id"], task_a["id"]) != snap_a["inventory"] or runner.nomad_snapshot(snap_a["inventory"]) != snap_a["nomad"]:
            raise PreflightError("environment A drifted while B was queued")
        if scn.tree(env_a) != snap_a["tree"] or runner.fence() != snap_a["fence"]:
            raise PreflightError("environment A process tree or fence changed while B was queued")

    step("queue-b-and-assert-a-unchanged", create_b)
    env_a, env_b = scn.envs["A"], scn.envs["B"]

    def release_a() -> dict[str, Any]:
        def b_unallocated() -> None:
            if scn.job_allocs(env_b["job_id"]):
                raise PreflightError("B was allocated while the node was ineligible and A's fence was still held")

        window = scn.open_eligibility_window()
        b_unallocated()
        released = scn.release("A", kill_first=False, watch=b_unallocated)
        if runner.fence() is not None:
            raise PreflightError("worker fence is still present after releasing A")
        b_unallocated()
        window["b_allocations_during_window"] = 0
        released["eligibility"] = scn.restore_eligibility(strict=True)
        return released

    scn.data["release_a"] = step("release-a-operator-teardown", release_a)

    def placed() -> dict[str, Any]:
        def one_running() -> dict[str, Any] | None:
            items = scn.job_allocs(env_b["job_id"])
            if len(items) > 1:
                raise PreflightError("environment B has more than one allocation")
            if len(items) == 1 and scn.alloc(items[0]["ID"]).get("ClientStatus") == "running":
                return items[0]
            return None

        item = scn.poll(one_running, "environment B was not placed and running after A was released")
        allocation = scn.alloc(item["ID"])
        if item["ID"] == env_a["allocation_id"] or allocation.get("NodeID") != env_a["node_id"]:
            raise PreflightError("B did not receive a new allocation on the same node")
        env_b.update(allocation_id=item["ID"], node_id=allocation["NodeID"])
        owner = scn.poll(lambda: scn.soft(runner.fence_owner), "worker fence of B did not appear before timeout")
        digest = hashlib.sha256(owner.encode()).hexdigest()
        if digest == env_a["fence_sha256"] or env_b["job_id"] not in owner:
            raise PreflightError("worker fence is not owned by B")
        if len(scn.job_allocs(env_a["job_id"])) != 1 or len(scn.job_allocs(env_b["job_id"])) != 1:
            raise PreflightError("a job does not have exactly one allocation")
        env_b.update(fence_sha256=digest, tree=scn.tree(env_b))
        scn.save_envs()
        if not env_b["tree"]:
            raise PreflightError("B has no remote process tree after placement")
        scn.data["identities"]["B"].update(allocation_id=item["ID"], fence_owner_sha256=digest, remote_processes=len(env_b["tree"]), allocations=1)
        return {"allocation_id": item["ID"], "fence_owner_sha256": digest}

    step("observe-b-placement", placed)
    cleanup = step("teardown", lambda: scn.finish({"released": [scn.release("B", kill_first=False)]}))
    return {**_base_report(scn), "capacity": capacity, "environments": {k: {"job_id": v["job_id"], "allocation_id": v["allocation_id"]} for k, v in scn.envs.items()}, "cleanup": cleanup}


# -- disconnect ------------------------------------------------------------------------------------------
def _make_relay(ops: fx.Ops, cfg: dict[str, Any]) -> Any:
    listen, upstream = _endpoint(cfg["partition"]["listen"]), _endpoint(cfg["partition"]["upstream"])
    relay = ops.relay_factory(listen, upstream) if ops.relay_factory else TcpRelay(listen, upstream)
    relay.start()
    return relay


def _state_extra(scenario: str) -> Callable[[dict[str, Any]], dict[str, Any]]:
    def extra(cfg: dict[str, Any]) -> dict[str, Any]:
        out: dict[str, Any] = {"scenario": scenario, "environments": []}
        if cfg.get("partition"):
            out["relay"] = {**cfg["partition"], "owner": "runner-process"}
        return out

    return extra


def drive_disconnect(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    step, scn = runner.step, Scenario(runner, cfg, "disconnect")
    relay = _make_relay(ops, cfg)
    try:
        report = _disconnect_steps(runner, cfg, ops, step, scn, relay)
    except Exception as exc:
        # The relay is not stopped: it is healed and keeps serving for as long as this process lives.
        note = RELAY_INSTRUCTION.format(upstream=cfg["partition"]["upstream"])
        try:
            relay.heal()
        except Exception as heal_exc:
            note += f" (the relay could not be healed: {heal_exc})"
        scn.data["relay_note"] = note
        scn.data["relay_partitioned_at_exit"] = bool(getattr(relay, "partitioned", False))
        raise PreflightError(f"{exc}; {note}") from exc
    scn.data["relay_partitioned_at_exit"] = bool(getattr(relay, "partitioned", False))
    relay.stop()
    return report


def _disconnect_steps(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, step: Callable[..., Any], scn: Scenario, relay: Any) -> dict[str, Any]:
    scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
    step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
    capacity = step("capacity-precondition", scn.capacity_precondition)
    step("start-git-daemon", runner.start_git_daemon)
    first = step("start-backend", lambda: runner.start_backend(None))
    step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
    task_a = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
    snapshot = step("await-a-running", lambda: scn.await_ready("A", task_a))
    env_a = scn.envs["A"]

    def start_managed_tree() -> None:
        """An idle prepared session runs no ACP agent; one inert prompt through the normal API starts the child and grandchild."""
        ack = ops.prompt({"ws_base": f"ws://127.0.0.1:{runner.state['port']}", "task_id": task_a["id"], "session_id": task_a["session_id"],
                          "content": "For this inert transport check, output exactly WO06_DISCONNECT_TREE"})
        if ack.get("accepted") is not True:
            raise PreflightError("the inert fixture prompt was not accepted")
        tree = scn.stable_tree(env_a, minimum_depth=2)
        env_a["tree"] = snapshot["tree"] = tree
        scn.save_envs()

    step("start-managed-process-tree", start_managed_tree)

    def snapshot_a() -> dict[str, Any]:
        survivors = [item for item in snapshot["tree"] if item["depth"] >= 2]
        if not survivors:
            raise PreflightError("environment A has no managed child or grandchild process to prove survival with")
        scn.known_jobs = scn.kandev_jobs()
        _, messages = runner.session_view()
        return {**snapshot, "tree_ident": _ident(snapshot["tree"]), "message_ids": [item.get("id") for item in messages], "backend_pid": first["pid"],
                "survivors": survivors, "leader": next(item for item in snapshot["tree"] if item["depth"] == 0 and item["role"] == "agentctl")}

    snap = step("snapshot-a", snapshot_a)
    scn.data["identities"]["A"].update(backend_pid=first["pid"], server_identity=snap["inventory"]["server_identity"], messages=len(snap["message_ids"]),
                                          processes=[{"role": item["role"], "pid": item["pid"], "started": item["started"]} for item in snap["tree"]])

    def check_stable(when: str) -> None:
        allocs = scn.job_allocs(env_a["job_id"])
        if len(allocs) != 1:
            raise PreflightError(f"{when}: job A has {len(allocs)} allocations, expected exactly one")
        if allocs[0]["ID"] != env_a["allocation_id"]:
            raise PreflightError(f"{when}: allocation of A was replaced")
        new_jobs = scn.kandev_jobs() - scn.known_jobs
        if new_jobs:
            raise PreflightError(f"{when}: unexpected new kandev job(s) {sorted(new_jobs)}")
        if _ident(scn.tree(env_a)) != snap["tree_ident"]:
            raise PreflightError(f"{when}: remote process tree of A changed")
        if runner.inventory(task_a["session_id"], task_a["id"]) != snap["inventory"]:
            raise PreflightError(f"{when}: durable inventory of A changed")
        if [item.get("id") for item in runner.session_view()[1]] != snap["message_ids"]:
            raise PreflightError(f"{when}: session messages changed (a prompt was sent)")
        records = [item["pid"] for item in runner.registry.read_records() if item["label"] == fx.PROCESS_LABEL]
        if records != [snap["backend_pid"]] or not ops.pid_alive(snap["backend_pid"]):
            raise PreflightError(f"{when}: backend process changed")

    def note(phase: str) -> dict[str, Any]:
        status = scn.node().get("Status")
        client = scn.alloc(env_a["allocation_id"]).get("ClientStatus")
        if phase == "partitioned" and client in fx.TERMINAL_ALLOCATION_STATES:
            raise PreflightError(f"allocation of A became {client!r} while the node was {status!r}; the original allocation must survive the partition")
        if not scn.observed or (scn.observed[-1]["node"], scn.observed[-1]["allocation"]) != (status, client):
            scn.observed.append({"phase": phase, "node": status, "allocation": client})
        scn.data["observed_statuses"] = scn.observed
        return {"node": status, "allocation": client}

    def partition() -> None:
        relay.partition()
        seen = {"left_ready": False}

        def drained() -> bool:
            check_stable("during partition")
            current = note("partitioned")
            if current["node"] != "ready":
                if seen["left_ready"]:
                    return True
                seen["left_ready"] = True
            return False

        scn.poll(drained, "the node never left ready during the partition: the client was not isolated (RPC bypass)")

    step("partition-and-assert-nothing-changes", partition)

    def heal() -> None:
        relay.heal()
        scn.poll(scn.node_ready, "node did not return to ready and eligible after the partition healed")
        status = scn.poll(lambda: (note("healed")["allocation"] != "unknown" and scn.alloc(env_a["allocation_id"]).get("ClientStatus")),
                          "allocation of A stayed unknown after the partition healed")
        if status != "running":
            raise PreflightError(f"allocation of A has ClientStatus {status!r} after the heal, expected running")
        check_stable("after heal")
        if runner.nomad_snapshot(snap["inventory"]) != snap["nomad"]:
            raise PreflightError("Nomad job or allocation identity of A changed across the partition")
        if runner.fence() != snap["fence"]:
            raise PreflightError("worker fence changed across the partition")
        scn.data["identities"]["A"]["after_heal"] = {"allocation_client_status": status, "prompts_sent": 0}

    step("heal-and-assert-original-allocation-survives", heal)
    task_b = {}

    def queue_b() -> None:
        task_b.update(runner.create_task("Nomad disconnect fixture B"))
        scn.await_queued("B", task_b, capacity["exhausted_dimensions"])

    step("queue-b", queue_b)
    env_b = scn.envs["B"]

    def crash_backend() -> None:
        if runner.registry.crash(fx.PROCESS_LABEL, timeout=10) != "crashed":
            raise PreflightError("fixture backend was not crashed through its exact owned process record")
        runner.verify_gone(first["pid"])

    step("crash-backend", crash_backend)

    def kill_leader() -> None:
        scn.verify_agentctl(snap["leader"])
        if ops.kill_exact(runner.ssh, snap["leader"], _needles(env_a)) != "killed":
            raise PreflightError("the agentctl leader of A was not running")

    step("kill-a-agentctl-leader", kill_leader)

    def settle() -> dict[str, Any]:
        previous: dict[str, Any] = {}

        def stable() -> dict[str, Any] | None:
            a_allocs, b_allocs = scn.job_allocs(env_a["job_id"]), scn.job_allocs(env_b["job_id"])
            if len(a_allocs) > 1:
                raise PreflightError("job A has a replacement allocation")
            if len(b_allocs) > 1:
                raise PreflightError("job B has more than one allocation (restart, reschedule or duplicate)")
            now = {"a": [(item["ID"], item.get("ClientStatus")) for item in a_allocs], "b": [(item["ID"], item.get("ClientStatus")) for item in b_allocs]}
            terminal = len(a_allocs) == 1 and len(b_allocs) == 1 and all(item.get("ClientStatus") in fx.TERMINAL_ALLOCATION_STATES for item in a_allocs + b_allocs)
            same = previous.get("now") == now
            previous["now"] = now
            return now if terminal and same else None

        return scn.poll(stable, "A and B allocations did not settle to terminal with a stable allocation count")

    settled = step("await-b-start-fence-rejection", settle)

    def assert_outcome() -> None:
        alloc_b = scn.alloc(settled["b"][0][0])
        env_b.update(allocation_id=alloc_b["ID"])
        scn.save_envs()
        state = (alloc_b.get("TaskStates") or {}).get("agentctl") or {}
        events = state.get("Events") or []
        codes = {event.get("ExitCode") for event in events} | {str((event.get("Details") or {}).get("exit_code")) for event in events}
        if alloc_b.get("ClientStatus") != "failed" or (START_FENCE_REJECTION_EXIT_CODE not in codes and str(START_FENCE_REJECTION_EXIT_CODE) not in codes):
            raise PreflightError(f"B allocation ended {alloc_b.get('ClientStatus')!r} with exit codes {sorted(str(c) for c in codes)}, expected failed with exit code 73")
        if state.get("Restarts") or any(event.get("Type") in ("Restarting", "Restart Signaled") for event in events) or alloc_b.get("NextAllocation") or alloc_b.get("RescheduleTracker"):
            raise PreflightError("B was restarted or rescheduled")
        if scn.tree(env_b):
            raise PreflightError("remote processes reference B although its start was rejected")
        still = scn.alive(snap["survivors"])
        gone = [item["pid"] for item in snap["survivors"] if item not in still]
        if gone:
            raise PreflightError(f"managed child/grandchild processes of A died: {gone}")
        if runner.fence() != snap["fence"]:
            raise PreflightError("worker fence is no longer A's")
        if len(scn.job_allocs(env_a["job_id"])) != 1 or len(scn.job_allocs(env_b["job_id"])) != 1:
            raise PreflightError("a job does not have exactly one allocation")
        scn.data["identities"]["B"].update(allocation_id=alloc_b["ID"], client_status="failed", exit_code=START_FENCE_REJECTION_EXIT_CODE, allocations=1,
                                          reserved_ports_listener="not asserted", a_fence_owner_unchanged=True)

    step("assert-b-rejected-and-a-survivors-intact", assert_outcome)

    def teardown() -> dict[str, Any]:
        released = [scn.release("A", kill_first=True), scn.release("B", kill_first=False)]
        relay.stop()
        return scn.finish({"released": released, "relay": "stopped"})

    cleanup = step("teardown", teardown)
    return {**_base_report(scn), "capacity": capacity, "backend_pids": {"before_crash": first["pid"]}, "cleanup": cleanup,
            "environments": {k: {"job_id": v["job_id"], "allocation_id": v["allocation_id"]} for k, v in scn.envs.items()}}


DRIVES = {"admission": drive_admission, "disconnect": drive_disconnect}


def check_scenario_config(cfg: dict[str, Any], scenario: str) -> None:
    if scenario not in DRIVES:
        raise PreflightError("unknown WO06 scenario")
    bootstrap.require_loopback_nomad(cfg, "admission and disconnect mutate Nomad")
    if scenario == "disconnect" and not cfg.get("partition"):
        raise PreflightError("the disconnect scenario requires a partition configuration with loopback listen and upstream")


def check_no_quarantine_marker(cfg: dict[str, Any], ops: fx.Ops) -> None:
    """Abort before anything is created when the dedicated node already carries a quarantine marker."""
    nomad = cfg["nomad"]
    token = fx.bootstrap.read_token(Path(nomad["token_file"]))
    get = ops.nomad_factory(nomad["address"], token, nomad["namespace"], nomad["region"])
    try:
        get(quarantine_marker_path(nomad["node_id"], nomad["namespace"]))
    except PreflightError as exc:
        if "status 404" in str(exc):
            return
        raise
    raise PreflightError(f"quarantine marker kandev/quarantine/{nomad['node_id']} already exists; refusing to run; "
                         f"{quarantine_operator_step(nomad['node_id'], nomad['namespace'])}")


def run_scenario(config: Any, scenario: str, ops: fx.Ops | None = None, **kwargs: Any) -> int:
    ops = ops or fx.Ops()

    def check(cfg: dict[str, Any]) -> None:
        check_scenario_config(cfg, scenario)
        check_no_quarantine_marker(cfg, ops)

    return fx.run_create_fixture(
        config, ops=ops, drive=DRIVES[scenario], mode=f"created-real-{scenario}", label=scenario,
        stop_owned_on_failure=True, check_config=check, state_extra=_state_extra(scenario), **kwargs,
    )


def create_fixture_command(config_path: str, scenario: str) -> int:
    try:
        config = json.loads(Path(config_path).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        print(f"real {scenario} bootstrap failed: config is unreadable", file=sys.stderr)
        return 2
    return run_scenario(config, scenario)
