"""Pool recovery scenarios: a retained Nomad instance must survive and be re-attached, never re-created.

* pool-pre-ready-recovery crashes the headless backend at the pre-publish barrier of a pool launch, restarts it and
  asserts the same execution reaches a ready environment with an unchanged continuation digest.
* pool-backend-restart restarts the backend while a session runs on the ssh node and asserts recovery attached the
  existing instance (same job, allocation, agentctl and fence) and that the Kandev API is reachable again through the
  reverse tunnel the agentctl uses.
* pool-idle-outage configures the backend with a 1m agentctl idle timeout, stops it for longer than that and asserts
  the retained instance stayed alive throughout and is re-attached after the restart. The running agentctl must carry
  KANDEV_ACP_IDLE_TIMEOUT=0 in its process environment (read on the node); when that cannot be observed the evidence
  records idle_timeout_observed false and labels the scenario survival-only.

Every scenario ends through production archive cleanup with no operator mutation. Each one needs no config keys
beyond the pool block. The idle timeout and the outage window are module constants.
"""

from __future__ import annotations

import json
import re
import subprocess
from pathlib import Path
from typing import Any, Callable

import nomad_native_e2e as base
import nomad_native_e2e_fixture as fx
import nomad_native_e2e_pool as pool
import nomad_native_e2e_wo06 as wo06
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError

IDLE_ENV = "KANDEV_ACP_IDLE_TIMEOUT"
IDLE_TIMEOUT = "1m"
IDLE_TIMEOUT_SECONDS = 60
# The janitor tick defaults to 1m, so a reap needs at most idle timeout plus two ticks; the window leaves a margin.
OUTAGE_SECONDS = 180
RUNTIME_API_PORT_KEY = "ssh_runtime_api_remote_port"
_SHA256 = re.compile(r"[0-9a-f]{64}")
_LOOPBACK_URL = re.compile(r"https?://127\.0\.0\.1:([0-9]{1,5})\b")
LIMITATIONS = [
    "the continuation digest is the durable input_digest of the environment state, with native_job_spec_sha256 recorded beside it",
    "the Kandev API call through the tunnel is made from the ssh node to the loopback URL the agentctl is given; the inert agent makes no API call itself",
    "the Go side forces the idle timeout of a Nomad-managed agentctl to 0, so the 1m setting is observable only as the instance surviving",
]
SURVIVAL_ONLY_LIMITATION = "the agentctl idle timeout could not be read from its process environment, so this run proves survival only"
_IDLE_TIMEOUT_ZERO = re.compile(r"0+(ns|us|ms|s|m|h)?")
_IDLE_ASSIGNMENT = re.compile(r"(?:^|[\s\0])" + IDLE_ENV + r"=(\S*)")


def parse_idle_timeout(text: str) -> str | None:
    """The KANDEV_ACP_IDLE_TIMEOUT value in a NUL, newline or space separated environment dump, or None."""
    found = _IDLE_ASSIGNMENT.search(text)
    return found.group(1) if found else None


# -- pure helpers --------------------------------------------------------------------------------------------
def find_key(value: Any, key: str) -> Any:
    """The first value stored under key anywhere in nested dicts and lists, or None."""
    if isinstance(value, dict):
        if key in value:
            return value[key]
        value = list(value.values())
    if isinstance(value, list):
        for item in value:
            found = find_key(item, key)
            if found is not None:
                return found
    return None


def valid_port(value: Any) -> int:
    if not isinstance(value, (str, int)) or isinstance(value, bool) or not re.fullmatch(r"[0-9]{1,5}", str(value)) or not 1 <= int(value) <= 65535:
        raise PreflightError("the reverse tunnel port is not a valid port number")
    return int(value)


def port_from_url(text: str) -> int:
    match = _LOOPBACK_URL.search(text or "")
    if match is None:
        raise PreflightError("the agentctl Kandev API URL is not a loopback URL with a port")
    return valid_port(match.group(1))


def digests_from_state(state: dict[str, Any]) -> dict[str, Any]:
    digest = state.get("input_digest")
    if not isinstance(digest, str) or not _SHA256.fullmatch(digest):
        raise PreflightError("durable state has no continuation input digest")
    return {"input_digest": digest, "native_job_spec_sha256": state.get("native_job_spec_sha256")}


def compare_instance(before: dict[str, Any], after: dict[str, Any], when: str) -> None:
    """Raises naming the first way the instance after `when` differs from the one before it."""
    if before["jobs"] != after["jobs"]:
        raise PreflightError(f"{when} registered a different set of Nomad jobs; the instance was re-created")
    if before["nomad"] != after["nomad"]:
        raise PreflightError(f"{when} changed the Nomad job or allocation identity; the instance was re-created")
    if before["inventory"] != after["inventory"]:
        raise PreflightError(f"{when} changed the execution or inventory identity")
    if before["digests"] != after["digests"]:
        raise PreflightError(f"{when} changed the continuation digest")
    if before["fence"] != after["fence"]:
        raise PreflightError(f"{when} changed the worker fence")
    if before["tree"] != after["tree"]:
        raise PreflightError(f"{when} did not preserve the agentctl: the process tree changed")


def check_outage_window() -> None:
    if OUTAGE_SECONDS <= IDLE_TIMEOUT_SECONDS:
        raise PreflightError(f"the outage window ({OUTAGE_SECONDS}s) must exceed the idle timeout ({IDLE_TIMEOUT_SECONDS}s)")


# -- scenario ------------------------------------------------------------------------------------------------
class RecoveryScenario(pool.TwoNodeScenario):
    pin: dict[str, Any] | None = None

    # -- ssh node pinning -----------------------------------------------------------------------------
    def set_eligibility(self, node_id: str, eligible: bool) -> None:
        self.ops.nomad_set_eligibility(self.nomad_cfg["address"], self.r.token, self.nomad_cfg["region"], node_id, eligible)
        want = "eligible" if eligible else "ineligible"
        self.poll(lambda: self.r.nomad(f"/v1/node/{node_id}").get("SchedulingEligibility") == want, f"node {node_id} did not become {want}")

    def pin_to_ssh(self) -> dict[str, Any]:
        """Makes the local node ineligible so Nomad can only place on the ssh node; the original state is recorded for restoration."""
        nodes = {(n.get("Meta") or {}).get("kandev.attach"): n for n in self.pool_nodes()}
        if "ssh" not in nodes:
            raise PreflightError("pool-backend-restart needs a ready and eligible ssh node")
        local = nodes.get("local")
        if local is None:
            return {"pinned": False, "reason": "the pool has no eligible local node"}
        self.pin = {"node_id": local["ID"], "original": local["SchedulingEligibility"], "restored": False,
                    "restore_with": f"nomad node eligibility -enable {local['ID']}"}
        self.r.save_state(recovery_restore=self.pin)
        self.set_eligibility(local["ID"], False)
        return {"pinned": True, "node_id": local["ID"]}

    def restore_pin(self) -> str:
        pin = self.pin
        if pin is None or pin["restored"]:
            return "not needed"
        self.set_eligibility(pin["node_id"], pin["original"] == "eligible")
        pin["restored"] = True
        self.r.save_state(recovery_restore=pin)
        return "restored"

    def restore_pin_quietly(self) -> None:
        try:
            self.restore_pin()
        except Exception as exc:  # noqa: BLE001 - the original failure is the one that must surface
            self.data["restore_failure"] = f"local node eligibility: {exc}"

    # -- backend process ------------------------------------------------------------------------------
    def stop_backend(self) -> dict[str, Any]:
        record = next((item for item in self.r.registry.read_records() if item["label"] == fx.PROCESS_LABEL), None)
        if record is None:
            raise PreflightError("the fixture backend is not running")
        if self.r.registry.stop(fx.PROCESS_LABEL, timeout=15) not in ("stopped", "already_absent"):
            raise PreflightError("fixture backend did not stop through its exact owned process record")
        self.r.verify_gone(record["pid"])
        return record

    def apply_idle_override(self) -> dict[str, str]:
        env = {**self.r.state["manifest_env"], IDLE_ENV: IDLE_TIMEOUT}
        self.r.save_state(manifest_env=env, idle_timeout_override=IDLE_TIMEOUT)
        return {IDLE_ENV: IDLE_TIMEOUT}

    def local_environ(self, pid: int) -> str:
        """The process environment of a local-node process: /proc on Linux, ``ps eww`` elsewhere."""
        proc = Path(f"/proc/{pid}/environ")
        if proc.exists():
            return proc.read_bytes().decode("utf-8", "replace")
        return subprocess.run(["ps", "eww", "-p", str(pid)], capture_output=True, text=True, timeout=15, check=False).stdout

    def agentctl_idle_timeout(self, env: dict[str, Any]) -> str | None:
        """The idle timeout the running agentctl process carries, or None when it cannot be observed."""
        self.focus(env)
        leader = next((item for item in self.tree(env) if item["depth"] == 0 and item["role"] == "agentctl"), None)
        if leader is None:
            raise PreflightError(f"environment {env['label']} has no identifiable agentctl root process")
        pid = int(leader["pid"])
        if _is_ssh(env):
            code, out = self.r.ssh(f"tr '\\0' '\\n' < /proc/{pid}/environ | sed -n 's/^{IDLE_ENV}=//p'")
            return out.strip() if code == 0 and out.strip() else None
        try:
            return parse_idle_timeout(self.local_environ(pid))
        except (OSError, subprocess.SubprocessError):
            return None

    def assert_idle_timeout_disabled(self, env: dict[str, Any]) -> dict[str, Any]:
        value = self.agentctl_idle_timeout(env)
        if value is not None and not _IDLE_TIMEOUT_ZERO.fullmatch(value):
            raise PreflightError(f"environment {env['label']}: the agentctl idle timeout is {value!r}, not 0, so it can reap itself while the backend is down")
        return {"idle_timeout_observed": value is not None, "agentctl_idle_timeout": value}

    # -- instance observation -------------------------------------------------------------------------
    def digests(self, env: dict[str, Any]) -> dict[str, Any]:
        return digests_from_state(self.read_state(env))

    def snapshot(self, env: dict[str, Any], task: dict[str, str]) -> dict[str, Any]:
        self.focus(env)
        inventory = self.r.inventory(task["session_id"], task["id"])
        tree = self.tree(env)
        if not any(item["depth"] == 0 and item["role"] == "agentctl" for item in tree):
            raise PreflightError(f"environment {env['label']} has no identifiable agentctl root process")
        fence = self.r.fence()
        if fence is None:
            raise PreflightError(f"environment {env['label']} has no worker fence")
        return {"inventory": inventory, "nomad": self.r.nomad_snapshot(inventory), "tree": wo06._ident(tree), "fence": fence,
                "digests": self.digests(env), "jobs": sorted(self.kandev_jobs())}

    def survival_problems(self, env: dict[str, Any], before: dict[str, Any]) -> list[str]:
        """What changed on the node while the backend was down; reads only Nomad, the node and the durable database."""
        self.focus(env)
        problems = []
        try:
            if self.r.nomad_snapshot(before["inventory"]) != before["nomad"]:
                problems.append("the Nomad job or allocation identity changed")
        except PreflightError as exc:
            problems.append(str(exc))
        if wo06._ident(self.tree(env)) != before["tree"]:
            problems.append("the agentctl process tree changed")
        if self.r.fence() != before["fence"]:
            problems.append("the worker fence changed")
        return problems

    # -- reverse tunnel -------------------------------------------------------------------------------
    def tunnel_port(self, env: dict[str, Any]) -> int:
        def read(db: Any) -> list[Any]:
            return db.execute("SELECT metadata FROM executors_running WHERE session_id = ?", (env["session_id"],)).fetchall()

        rows = self.ops.read_db(self.r.database, read)
        try:
            recorded = find_key(json.loads(rows[0][0]), RUNTIME_API_PORT_KEY) if len(rows) == 1 else None
        except (TypeError, ValueError, IndexError):
            recorded = None
        if recorded is not None:
            return valid_port(recorded)
        leader = next((item for item in self.tree(env) if item["depth"] == 0 and item["role"] == "agentctl"), None)
        if leader is not None:
            code, out = self.r.ssh(f"tr '\\0' '\\n' < /proc/{int(leader['pid'])}/environ | sed -n 's/^KANDEV_API_URL=//p'")
            if code == 0 and out.strip():
                return port_from_url(out)
        raise PreflightError("the reverse tunnel port could not be determined from the durable state or the agentctl environment")

    def tunnel_status(self, env: dict[str, Any], port: int) -> int | None:
        """HTTP status of a Kandev API read made from the node over the tunnel, or None when nothing answers."""
        self.focus(env)
        code, out = self.r.ssh(f"curl -sS --max-time 10 -o /dev/null -w '%{{http_code}}' http://127.0.0.1:{port}/api/v1/workspaces")
        text = out.strip()
        return int(text) if code == 0 and text.isdigit() else None

    def assert_tunnel(self, env: dict[str, Any], port: int) -> dict[str, Any]:
        status = self.tunnel_status(env, port)
        if status != 200:
            raise PreflightError(f"the Kandev API call through the reverse tunnel on port {port} returned status {status}")
        return {"port": port, "http_status": status}

    def await_tunnel_down(self, env: dict[str, Any], port: int) -> bool:
        self.poll(lambda: self.tunnel_status(env, port) is None, f"the reverse tunnel on port {port} did not go down with the backend; a later success would prove nothing")
        return True

    def await_tunnel(self, env: dict[str, Any]) -> dict[str, Any]:
        def answering() -> dict[str, Any] | None:
            port = self.soft(lambda: self.tunnel_port(env))
            return {"port": port, "http_status": 200} if port and self.tunnel_status(env, port) == 200 else None

        return self.poll(answering, "the reverse tunnel did not carry a Kandev API call after recovery")


def _is_ssh(env: dict[str, Any]) -> bool:
    return env.get("attach") == "ssh"


def _finish_cleanup(scn: RecoveryScenario, step: Callable[..., Any], guard: wo07.OperatorGuard, env: dict[str, Any], task: dict[str, str],
                    layout: dict[str, Any]) -> None:
    guard.calls.clear()
    guard.armed = True
    step("archive-task", lambda: scn.archive(task))

    def settled() -> dict[str, Any]:
        scn.focus(env)
        pending: list[str] = []
        return scn.settle_note(pending, "production cleanup did not settle before timeout")(lambda: scn.production_settled(env, pending))

    scn.data["identities"][env["label"]]["cleanup"] = step("await-production-cleanup", settled)
    step("assert-cleaned", lambda: scn.assert_pool_cleaned(env, layout))

    def assert_no_operator() -> None:
        guard.assert_clean()
        guard.armed = False

    step("assert-no-operator-cleanup", assert_no_operator)


def _report(scn: RecoveryScenario, cleanup: dict[str, Any], **extra: Any) -> dict[str, Any]:
    return {"scenario": scn.name, "pool": True, "production_cleanup": True, "operator_cleanup_used": False, "release": "none",
            "limitations": LIMITATIONS, **extra, "cleanup": cleanup, "environments": wo07._env_report(scn)}


def _prelude(runner: fx.Runner, step: Callable[..., Any], scn: RecoveryScenario) -> None:
    scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
    step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))


def _running_instance(runner: fx.Runner, cfg: dict[str, Any], step: Callable[..., Any], scn: RecoveryScenario) -> tuple[dict[str, str], dict[str, Any], dict[str, Any]]:
    """A placed pool environment with a running managed agent tree, its pre-restart snapshot and its workspace layout."""
    step("start-git-daemon", runner.start_git_daemon)
    step("start-backend", lambda: runner.start_backend(None))
    step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
    task = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
    placed = step("await-running", lambda: scn.await_placed("A", task))
    env = scn.envs["A"]
    step("start-managed-process-tree", lambda: scn.start_managed_tree(env, task, placed))
    layout = step("assert-workspace", lambda: scn.workspace_layout(env))
    scn.known_jobs = scn.kandev_jobs()
    return task, env, layout


# -- pool-pre-ready-recovery -----------------------------------------------------------------------------------
def drive_pool_pre_ready_recovery(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    guard = wo07.OperatorGuard(ops)
    guard.armed = False
    scn = RecoveryScenario(runner, cfg, "pool-pre-ready-recovery", guard)
    step, state = runner.step, runner.state
    marker = root / "barrier-marker.json"
    try:
        _prelude(runner, step, scn)
        step("start-git-daemon", runner.start_git_daemon)
        first = step("start-backend-with-barrier", lambda: runner.start_backend(str(marker)))
        step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
        task = {"id": state["task_id"], "session_id": state["session_id"]}
        barrier = step("await-pre-publish-barrier", lambda: runner.await_barrier(marker))

        def place() -> dict[str, Any]:
            meta = runner.nomad(f"/v1/node/{barrier['node_id']}").get("Meta") or {}
            problems = pool.contract_problems(meta)
            if problems:
                raise PreflightError(f"the placed node does not carry the Kandev node contract: {', '.join(problems)}")
            runner.use_node(meta["kandev.attach"], meta["kandev.state_root"])
            return scn.record_env("A", task, barrier, attach=meta["kandev.attach"], state_root=meta["kandev.state_root"])

        env = step("record-placed-node", place)
        scn.known_jobs = scn.kandev_jobs()

        def pre_crash() -> dict[str, Any]:
            generation = runner.check_environment_creating(barrier["environment_id"])
            messages = runner.check_session_created()
            if generation != barrier["generation"]:
                raise PreflightError("environment ownership generation differs from the inventory generation")
            return {"generation": generation, "messages": messages, "instance": scn.snapshot(env, task)}

        before = step("snapshot-before-crash", pre_crash)

        def crash() -> None:
            if runner.registry.crash(fx.PROCESS_LABEL, timeout=10) != "crashed":
                raise PreflightError("fixture backend was not crashed through its exact owned process record")
            runner.verify_gone(first["pid"])

        step("crash-backend", crash)
        second = step("restart-backend-without-barrier", lambda: runner.start_backend(None))
        if second["pid"] == first["pid"]:
            raise PreflightError("restarted backend reused the crashed process identity")

        def recovered() -> dict[str, Any]:
            base._await_recovered_environment(
                lambda fn: ops.read_db(runner.database, fn), runner.inventory, environment_id=barrier["environment_id"],
                generation_before=before["generation"], expected_repo_count=state["expected_repo_count"],
                timeout=ops.recovery_timeout, clock=ops.clock, sleep=ops.sleep)
            row, messages = runner.session_view()
            base._check_session_untouched(row, messages, before["messages"])
            return scn.snapshot(env, task)

        after = step("await-recovery", recovered)
        step("assert-identity-preserved", lambda: compare_instance(before["instance"], after, "pre-ready recovery"))
        layout = step("assert-workspace", lambda: scn.workspace_layout(env))
        _finish_cleanup(scn, step, guard, env, task, layout)
        cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    finally:
        guard.restore()
    digest = before["instance"]["digests"]["input_digest"]
    return _report(scn, cleanup, pre_ready_recovery=True, execution_id=after["inventory"]["execution_id"],
                   continuation_digest={"before": digest, "after": after["digests"]["input_digest"], "equal": True},
                   environment_status={"before": "creating", "after": "ready"}, backend_pids={"before_crash": first["pid"], "after_restart": second["pid"]},
                   node_attach=env["attach"])


# -- pool-backend-restart --------------------------------------------------------------------------------------
def drive_pool_backend_restart(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    guard = wo07.OperatorGuard(ops)
    guard.armed = False
    scn = RecoveryScenario(runner, cfg, "pool-backend-restart", guard)
    step = runner.step
    try:
        _prelude(runner, step, scn)
        pinned = step("pin-to-ssh-node", scn.pin_to_ssh)
        task, env, layout = _running_instance(runner, cfg, step, scn)
        if not _is_ssh(env):
            raise PreflightError(f"the environment was placed on a {env.get('attach')} node; this scenario needs the ssh node")
        port = step("determine-tunnel-port", lambda: scn.tunnel_port(env))
        tunnel_before = step("assert-tunnel-before-restart", lambda: scn.assert_tunnel(env, port))
        before = step("snapshot-before-restart", lambda: scn.snapshot(env, task))
        old = step("stop-backend", scn.stop_backend)
        down = step("assert-tunnel-went-down", lambda: scn.await_tunnel_down(env, port))
        new = step("restart-backend", lambda: runner.start_backend(None))
        if new["pid"] == old["pid"]:
            raise PreflightError("restarted backend reused the stopped process identity")
        tunnel_after = step("await-tunnel-after-recovery", lambda: scn.await_tunnel(env))
        after = step("snapshot-after-recovery", lambda: scn.snapshot(env, task))
        step("assert-instance-attached", lambda: compare_instance(before, after, "backend restart recovery"))
        _finish_cleanup(scn, step, guard, env, task, layout)
        step("restore-local-node-eligibility", scn.restore_pin)
        cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    except BaseException:
        scn.restore_pin_quietly()
        raise
    finally:
        guard.restore()
    return _report(
        scn, cleanup, backend_restart=True, node_attach=env["attach"], pinned_to_ssh=pinned["pinned"],
        backend_pids={"before": old["pid"], "after": new["pid"]},
        attached_existing_instance={"job_id": after["nomad"]["job_id"], "job_recreated": False, "allocation_id": after["nomad"]["allocation_id"],
                                    "agentctl_unchanged": True, "fence_unchanged": True, "new_jobs": []},
        tunnel={"before": tunnel_before, "after": tunnel_after, "went_down_with_backend": down})


# -- pool-idle-outage ------------------------------------------------------------------------------------------
def drive_pool_idle_outage(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    check_outage_window()
    guard = wo07.OperatorGuard(ops)
    guard.armed = False
    scn = RecoveryScenario(runner, cfg, "pool-idle-outage", guard)
    step = runner.step
    try:
        _prelude(runner, step, scn)
        step("configure-agentctl-idle-timeout", scn.apply_idle_override)
        task, env, layout = _running_instance(runner, cfg, step, scn)
        port = step("determine-tunnel-port", lambda: scn.tunnel_port(env)) if _is_ssh(env) else None
        before = step("snapshot-before-outage", lambda: scn.snapshot(env, task))
        idle = step("assert-agentctl-idle-timeout-disabled", lambda: scn.assert_idle_timeout_disabled(env))
        old = step("stop-backend", scn.stop_backend)
        outage = step("hold-outage-beyond-idle-timeout", lambda: _hold_outage(scn, env, before))
        new = step("restart-backend", lambda: runner.start_backend(None))
        if new["pid"] == old["pid"]:
            raise PreflightError("restarted backend reused the stopped process identity")
        tunnel = step("await-tunnel-after-recovery", lambda: scn.await_tunnel(env)) if port is not None else None
        after = step("snapshot-after-recovery", lambda: scn.snapshot(env, task))
        step("assert-instance-recovered", lambda: compare_instance(before, after, "idle outage recovery"))
        _finish_cleanup(scn, step, guard, env, task, layout)
        cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    finally:
        guard.restore()
    report = _report(scn, cleanup, idle_outage=True, idle_timeout=IDLE_TIMEOUT, outage=outage, instance_survived=True, tunnel=tunnel,
                     node_attach=env["attach"], backend_pids={"before": old["pid"], "after": new["pid"]}, **idle,
                     scenario_kind="idle-timeout-disabled-and-survival" if idle["idle_timeout_observed"] else "survival-only")
    if not idle["idle_timeout_observed"]:
        report["limitations"] = [*LIMITATIONS, SURVIVAL_ONLY_LIMITATION]
    return report


def _hold_outage(scn: RecoveryScenario, env: dict[str, Any], before: dict[str, Any]) -> dict[str, Any]:
    """One poll against a monotonic deadline: every sample asserts the backend is down and the instance unchanged."""
    ops, port = scn.ops, scn.r.state["port"]
    started, samples = ops.clock(), [0]

    def elapsed() -> dict[str, Any] | None:
        if ops.port_open(port):
            raise PreflightError("the backend came back during the outage")
        problems = scn.survival_problems(env, before)
        if problems:
            raise PreflightError(f"the retained instance did not survive during the outage: {'; '.join(problems)}")
        samples[0] += 1
        seconds = ops.clock() - started
        return {"observed_seconds": round(seconds, 3), "required_seconds": OUTAGE_SECONDS, "idle_timeout_seconds": IDLE_TIMEOUT_SECONDS,
                "samples": samples[0], "backend_down_throughout": True} if seconds >= OUTAGE_SECONDS else None

    return scn.poll(elapsed, "the outage window did not elapse before timeout", timeout=OUTAGE_SECONDS * 2 + ops.scenario_timeout)
