"""WO07 create-fixture scenarios: production cleanup and quarantine on a real worker.

cleanup-positive archives a task through the backend and proves the production path alone reached cleanup: a drain
receipt at the current generation, the job stopped (purge disabled), exactly one terminal allocation, no managed
processes, no fence and no quarantine. Any operator action before those assertions fails the run.

cleanup-negative makes the drain fail deterministically (SIGSTOP of the agentctl leader), archives the task and proves
the environment is retained and the node quarantined, a restart changes nothing, and a later launch is refused. It then
performs the explicit operator readmission steps in order and proves a new launch is admitted.

Mutations are limited to fixture-recorded jobs (purge disabled), compare-owner removal of the fixture's own fence,
identity-checked process signals and the fixture node's quarantine Variable and eligibility.
"""

from __future__ import annotations

import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any, Callable
from urllib.parse import urlsplit

import nomad_native_e2e_fixture as fx
import nomad_native_e2e_wo06 as wo06
from nomad_native_e2e import PreflightError

SCENARIOS = ("cleanup-positive", "cleanup-negative")
ARCHIVE_API_PATH = "/tasks/{task_id}/archive"
# The runner API client prefixes /api/v1; READMIT_PATH is the full endpoint path.
READMIT_API_PATH = "/executors/nomad/readmit"
READMIT_PATH = "/api/v1" + READMIT_API_PATH
QUARANTINE_REFUSAL = "quarantin"
READMIT_STEPS = ("resume-leader", "kill-exact-tree", "stop-job", "remove-fence", "readmit", "delete-marker", "enable-eligibility")
INERT_PROMPT = "For this inert cleanup check, output exactly WO07_CLEANUP_TREE"
OPERATOR_OPS = ("nomad_stop", "kill_exact", "kill_group", "signal_pid", "nomad_delete_variable", "nomad_set_eligibility")
LIMITATIONS = [
    "success is scoped to trusted managed process groups; a daemonized escape is quarantined, not detected here",
    "the drain failure is induced by stopping the agentctl leader; other failure modes are covered by Go tests",
    "readmission is an explicit operator procedure; the runner performs it only after verifying the exact tree is absent",
    "node ACL scope is not per node; the dedicated-node check is the profile-pinned NodeID, datacenter and platform",
]
_SHA256 = re.compile(r"[0-9a-f]{64}")


class OperatorGuard:
    """Records operator mutations made while armed, by wrapping the Ops callables in place."""

    def __init__(self, ops: fx.Ops):
        self.ops, self.calls, self.armed = ops, [], True
        self._originals = {name: getattr(ops, name) for name in OPERATOR_OPS}
        for name, fn in self._originals.items():
            setattr(ops, name, self._wrap(name, fn))

    def _wrap(self, name: str, fn: Callable[..., Any]) -> Callable[..., Any]:
        def inner(*args: Any, **kwargs: Any) -> Any:
            if self.armed:
                self.calls.append(name)
            return fn(*args, **kwargs)

        return inner

    def record(self, name: str) -> None:
        if self.armed:
            self.calls.append(name)

    def assert_clean(self) -> None:
        if self.calls:
            raise PreflightError(f"operator cleanup was used before production cleanup was asserted: {sorted(set(self.calls))}")

    def restore(self) -> None:
        for name, fn in self._originals.items():
            setattr(self.ops, name, fn)


class Wo07Scenario(wo06.Scenario):
    def __init__(self, runner: fx.Runner, cfg: dict[str, Any] | None, name: str, guard: OperatorGuard | None = None):
        super().__init__(runner, cfg, name)
        self.guard = guard

    def remove_fence(self, expected_sha: str) -> str:
        if self.guard:
            self.guard.record("remove_fence")
        return super().remove_fence(expected_sha)

    # -- observation -------------------------------------------------------------------------------
    def read_state(self, env: dict[str, Any]) -> dict[str, Any]:
        """The raw durable Nomad environment state of one session, including drain receipt and quarantine fields."""
        def read(db: Any) -> list[Any]:
            return db.execute("SELECT metadata FROM executors_running WHERE session_id = ?", (env["session_id"],)).fetchall()

        rows = self.ops.read_db(self.r.database, read)
        if len(rows) != 1:
            raise PreflightError(f"expected exactly one durable inventory row for environment {env['label']}; a missing row is not cleanup evidence")
        try:
            return json.loads(rows[0][0])["nomad_environment_state"]
        except (TypeError, KeyError, json.JSONDecodeError):
            raise PreflightError("Nomad inventory state is unreadable") from None

    @staticmethod
    def state_summary(state: dict[str, Any]) -> dict[str, Any]:
        return {key: state.get(key) for key in ("phase", "generation", "job_id", "allocation_id", "node_id", "quarantine_state",
                                                "drain_receipt_digest", "drain_receipt_generation")}

    def start_managed_tree(self, env: dict[str, Any], task: dict[str, str], snapshot: dict[str, Any]) -> list[dict[str, Any]]:
        """An idle prepared session runs no ACP agent; one inert prompt through the normal API starts child and grandchild."""
        ack = self.ops.prompt({"ws_base": f"ws://127.0.0.1:{self.r.state['port']}", "task_id": task["id"], "session_id": task["session_id"], "content": INERT_PROMPT})
        if ack.get("accepted") is not True:
            raise PreflightError("the inert fixture prompt was not accepted")
        tree = self.stable_tree(env, minimum_depth=2)
        env["tree"] = snapshot["tree"] = tree
        self.save_envs()
        return tree

    def archive(self, task: dict[str, str]) -> None:
        self.r.api("POST", ARCHIVE_API_PATH.format(task_id=task["id"]), None)

    def node_state(self) -> tuple[Any, Any]:
        node = self.node()
        return node.get("Status"), node.get("SchedulingEligibility")

    def record_eligibility_original(self) -> dict[str, Any]:
        """Remember the node's eligibility so cleanup-fixture can restore it once no marker or fence remains."""
        node_id = self.nomad_cfg["node_id"]
        window = {"node_id": node_id, "original": self.node().get("SchedulingEligibility"), "changed_by_runner": True, "restored": False,
                  "restore_with": f"nomad node eligibility -enable {node_id}"}
        self.r.save_state(eligibility_window=window)
        self.data["eligibility_window"] = window
        return window

    def settle_note(self, pending: list[str], failure: str) -> Callable[[Callable[[], Any]], Any]:
        def run(probe: Callable[[], Any]) -> Any:
            try:
                return self.poll(probe, failure)
            except PreflightError as exc:
                if str(exc) == failure and pending:
                    raise PreflightError(f"{failure}; pending: {', '.join(pending)}") from None
                raise

        return run

    def common_prelude(self, step: Callable[..., Any], runner: fx.Runner, cfg: dict[str, Any]) -> tuple[dict[str, str], dict[str, Any], dict[str, Any]]:
        self.data["node"] = step("verify-node-identity", self.verify_node_identity)
        step("poll-node-ready", lambda: self.poll(self.node_ready, "Nomad node is not ready and eligible"))
        step("start-git-daemon", runner.start_git_daemon)
        step("start-backend", lambda: runner.start_backend(None))
        step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
        task = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
        snapshot = step("await-a-running", lambda: self.await_ready("A", task))
        env = self.envs["A"]
        step("start-managed-process-tree", lambda: self.start_managed_tree(env, task, snapshot))
        leader = next((item for item in snapshot["tree"] if item["depth"] == 0 and item["role"] == "agentctl"), None)
        if leader is None or not any(item["depth"] >= 2 for item in snapshot["tree"]):
            raise PreflightError("environment A has no agentctl leader with a managed child and grandchild")
        self.known_jobs = self.kandev_jobs()
        self.data["identities"]["A"].update(state=self.state_summary(self.read_state(env)), processes=[
            {"role": item["role"], "pid": item["pid"], "started": item["started"]} for item in snapshot["tree"]])
        return task, snapshot, leader


def _env_report(scn: Wo07Scenario) -> dict[str, Any]:
    return {label: {"job_id": env["job_id"], "allocation_id": env["allocation_id"]} for label, env in scn.envs.items()}


# -- cleanup-positive ------------------------------------------------------------------------------------
def drive_cleanup_positive(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    step = runner.step
    guard = OperatorGuard(ops)
    scn = Wo07Scenario(runner, cfg, "cleanup-positive", guard)
    try:
        return _positive_steps(runner, cfg, step, scn, guard)
    finally:
        guard.restore()


def _positive_steps(runner: fx.Runner, cfg: dict[str, Any], step: Callable[..., Any], scn: Wo07Scenario, guard: OperatorGuard) -> dict[str, Any]:
    task, snapshot, _ = scn.common_prelude(step, runner, cfg)
    env = scn.envs["A"]
    before = {"inventory": snapshot["inventory"], "fence": snapshot["fence"], "tree_ident": wo06._ident(snapshot["tree"])}
    step("archive-task", lambda: scn.archive(task))

    def await_production_cleanup() -> dict[str, Any]:
        pending: list[str] = []
        result = scn.settle_note(pending, "production cleanup did not settle before timeout")(lambda: scn.production_settled(env, pending))
        marker = scn.quarantine_marker()
        status, eligibility = scn.node_state()
        if marker is not None or eligibility != "eligible" or status != "ready":
            raise PreflightError(f"production cleanup left a quarantine marker or an ineligible node (marker={marker is not None}, node={status}/{eligibility})")
        return result

    result = step("await-production-cleanup", await_production_cleanup)

    def assert_no_operator() -> None:
        guard.assert_clean()
        guard.armed = False

    step("assert-no-operator-cleanup", assert_no_operator)
    scn.data["identities"]["A"]["cleanup"] = {**result, "before": {"fence_owner_sha256": before["fence"], "processes": len(before["tree_ident"])}}
    cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    return {"scenario": scn.name, "production_cleanup": True, "operator_cleanup_used": False, "release": "none", "limitations": LIMITATIONS,
            "cleanup": cleanup, "environments": _env_report(scn)}


def _production_settled(self: Wo07Scenario, env: dict[str, Any], pending: list[str]) -> dict[str, Any] | None:
    """One poll probe: definitive violations raise, work still in progress is listed in pending and returns None."""
    if self.guard:
        self.guard.assert_clean()
    state = self.read_state(env)
    phase, digest = state.get("phase"), state.get("drain_receipt_digest") or ""
    if phase == "quarantined" or state.get("quarantine_state") == "marked" or phase == "cleanup_pending" and state.get("quarantine_state"):
        raise PreflightError("production cleanup quarantined the environment instead of draining it")
    if phase == "absent" and not digest:
        raise PreflightError("inventory is absent without a drain receipt (false absence)")
    if digest and not _SHA256.fullmatch(digest):
        raise PreflightError("drain receipt digest is not a sha256")
    if digest and state.get("drain_receipt_generation") != state.get("generation"):
        raise PreflightError(f"drain receipt generation {state.get('drain_receipt_generation')!r} is not the current generation {state.get('generation')!r}")
    pending.clear()
    if phase != "absent":
        pending.append(f"inventory phase {phase!r}")
    status = self.job_status(env["job_id"])
    if status is None:
        raise PreflightError("the Nomad job is gone: it was purged instead of stopped")
    allocs = self.job_allocs(env["job_id"])
    if len(allocs) != 1 or allocs[0]["ID"] != env["allocation_id"]:
        raise PreflightError(f"job has {len(allocs)} allocations, expected exactly the original one")
    alloc = self.alloc(env["allocation_id"])
    if alloc.get("NodeID") != env["node_id"]:
        raise PreflightError("the allocation is not on the original node")
    if alloc.get("ClientStatus") not in fx.TERMINAL_ALLOCATION_STATES:
        pending.append(f"allocation {alloc.get('ClientStatus')!r}")
    if status != "dead":
        pending.append(f"job status {status!r}")
    live = self.tree(env)
    if digest and getattr(self, "receipt_sample", None) is None:
        self.receipt_sample = _sample_at_receipt(live, alloc, status)
    if live:
        pending.append("managed processes remain")
    if self.r.fence() is not None:
        pending.append("worker fence remains")
    if pending:
        return None
    return {"phase": phase, "drain_receipt_digest": digest, "drain_receipt_generation": state["drain_receipt_generation"], "job_status": status,
            "job_purged": False, "allocations": 1, "allocation_client_status": alloc["ClientStatus"], "same_node": True,
            "processes": 0, "fence": None, "quarantine_marker": False, **self.receipt_sample}


def _sample_at_receipt(live: list[dict[str, Any]], alloc: dict[str, Any], job_status: str) -> dict[str, Any]:
    """The process tree at the first poll that sees the drain receipt: the drain itself, not the job stop, must have emptied it.

    While the allocation is non-terminal or the job is not yet dead, the job stop has not confirmed, so no managed
    child or grandchild (depth >= 1) may remain. Once both are terminal the job stop may have killed the process group,
    so the window was missed and the receipt digest in the inventory row is the only drain evidence.
    """
    if alloc.get("ClientStatus") in fx.TERMINAL_ALLOCATION_STATES and job_status == "dead":
        return {"tree_gone_before_job_stop": False, "window_missed": True,
                "window_missed_evidence": "terminal allocation and dead job at first sight of the drain receipt; receipt digest in the inventory row"}
    if any(item["depth"] >= 1 for item in live):
        raise PreflightError("the drain receipt is present but managed child processes remain while the allocation is not terminal: the drain stopped nothing")
    return {"tree_gone_before_job_stop": True, "window_missed": False}


Wo07Scenario.production_settled = _production_settled  # type: ignore[attr-defined]


# -- cleanup-negative ------------------------------------------------------------------------------------
def readmit_body(env: dict[str, Any], node_id: str, state: dict[str, Any]) -> dict[str, Any]:
    return {
        "node_id": node_id, "environment_id": env["environment_id"], "generation": state["generation"], "job_id": env["job_id"],
        "attestation": {"process_tree_absent": True, "job_stopped_purge_false": True, "fence_removed": True, "operator": "fixture-runner"},
    }


def drive_cleanup_negative(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    scn = Wo07Scenario(runner, cfg, "cleanup-negative")
    return _negative_steps(runner, cfg, ops, runner.step, scn)


def _quarantine_assertions(scn: Wo07Scenario, env: dict[str, Any], snapshot: dict[str, Any], pending: list[str]) -> dict[str, Any] | None:
    state = scn.read_state(env)
    phase, digest = state.get("phase"), state.get("drain_receipt_digest") or ""
    if digest:
        raise PreflightError("a drain receipt was persisted although the drain could not complete")
    if phase in ("absent", "operator_released"):
        raise PreflightError(f"inventory became {phase!r} without a drain receipt (false absence)")
    pending.clear()
    if phase != "cleanup_pending":
        pending.append(f"inventory phase {phase!r}")
    if state.get("quarantine_state") != "marked":
        pending.append(f"quarantine_state {state.get('quarantine_state')!r}")
    if scn.job_status(env["job_id"]) is None:
        raise PreflightError("the Nomad job is gone: it was purged although cleanup is retained")
    if len(scn.job_allocs(env["job_id"])) != 1:
        raise PreflightError("job does not have exactly its original allocation")
    if scn.r.fence() != snapshot["fence"]:
        raise PreflightError("worker fence was released although cleanup is retained")
    if scn.quarantine_marker() is None:
        pending.append("quarantine marker absent")
    if scn.node().get("SchedulingEligibility") != "ineligible":
        pending.append("node still eligible")
    if pending:
        return None
    return {**scn.state_summary(state), "quarantine_marker": True, "node_eligibility": "ineligible", "fence_retained": True, "job_purged": False}


def _negative_steps(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, step: Callable[..., Any], scn: Wo07Scenario) -> dict[str, Any]:
    task_a, snapshot, leader = scn.common_prelude(step, runner, cfg)
    env_a, node_id = scn.envs["A"], scn.nomad_cfg["node_id"]
    window = scn.record_eligibility_original()
    stopped = {"done": False}

    def stop_leader() -> None:
        scn.verify_agentctl(leader)
        ops.signal_pid(runner.ssh, leader["pid"], "STOP")
        stopped["done"] = True

    step("induce-drain-failure-sigstop-leader", stop_leader)
    step("archive-task-a", lambda: scn.archive(task_a))

    def await_quarantine() -> dict[str, Any]:
        pending: list[str] = []
        return scn.settle_note(pending, "environment A did not reach cleanup_pending with a quarantine marker before timeout")(
            lambda: _quarantine_assertions(scn, env_a, snapshot, pending))

    retained = step("assert-a-retained-and-quarantined", await_quarantine)
    scn.data["identities"]["A"]["quarantined"] = retained

    def launch_b_refused() -> dict[str, Any]:
        task_b = runner.create_task("Nomad cleanup fixture B (refused)")
        def refused() -> dict[str, Any] | None:
            extra = scn.kandev_jobs() - scn.known_jobs
            if extra:
                raise PreflightError(f"B created Nomad job(s) {sorted(extra)} although the node is quarantined")
            return scn.refusal_seen(task_b)

        refusal = scn.poll(refused, "launch of B was not refused with a quarantine error before timeout")
        inventory = scn.soft(lambda: runner.inventory(task_b["session_id"], task_b["id"]))
        if inventory and inventory.get("allocation_id"):
            raise PreflightError("B has an allocation although the node is quarantined")
        return {"task_id": task_b["id"], "session_id": task_b["session_id"], "refusal": refusal, "jobs_created": 0, "allocations": 0}

    scn.data["refused_launch"] = step("launch-b-refused", launch_b_refused)

    def restart_backend() -> dict[str, Any]:
        record = next((item for item in runner.registry.read_records() if item["label"] == fx.PROCESS_LABEL), None)
        if record is None or runner.registry.stop(fx.PROCESS_LABEL, timeout=15) not in ("stopped", "already_absent"):
            raise PreflightError("fixture backend did not stop through its exact owned process record")
        runner.verify_gone(record["pid"])
        started = runner.start_backend(None)
        pending: list[str] = []
        after = scn.poll(lambda: _quarantine_assertions(scn, env_a, snapshot, pending), "state of A changed across the backend restart")
        if after != retained:
            raise PreflightError("durable state of A changed across the backend restart")
        return {"backend_pid_before": record["pid"], "backend_pid_after": started["pid"], "state_unchanged": True}

    scn.data["restart"] = step("restart-backend-and-assert-state-unchanged", restart_backend)
    readmitted = _readmit(runner, ops, step, scn, env_a, leader, snapshot, node_id, window)

    def b_admitted() -> dict[str, Any]:
        task = runner.create_task("Nomad cleanup fixture B (readmitted)")
        scn.await_ready("B", task)
        env_b = scn.envs["B"]
        if env_b["allocation_id"] == env_a["allocation_id"] or env_b["node_id"] != env_a["node_id"]:
            raise PreflightError("B did not receive a new allocation on the same node")
        owner = runner.fence_owner()
        if owner is None or env_b["job_id"] not in owner or hashlib.sha256(owner.encode()).hexdigest() == env_a["fence_sha256"]:
            raise PreflightError("worker fence is not owned by B")
        return {"allocation_id": env_b["allocation_id"], "fence_owner_sha256": env_b["fence_sha256"]}

    admitted = step("assert-b-admitted-after-readmission", b_admitted)

    def teardown() -> dict[str, Any]:
        released = [scn.release("B", kill_first=False)]
        env_a["released"] = True
        scn.save_envs()
        return scn.finish({"released": released})

    cleanup = step("teardown", teardown)
    return {"scenario": scn.name, "production_cleanup": False, "production_quarantine": True, "release": "operator-teardown", "limitations": LIMITATIONS,
            "readmission": readmitted, "b_admitted": admitted, "cleanup": cleanup, "environments": _env_report(scn)}


def _refusal_seen(self: Wo07Scenario, task: dict[str, str]) -> dict[str, Any] | None:
    sessions = self.r.api("GET", f"/tasks/{task['id']}/sessions", None).get("sessions", [])
    messages = self.r.api("GET", f"/task-sessions/{task['session_id']}/messages", None).get("messages", [])
    text = json.dumps([sessions, messages]).lower()
    if QUARANTINE_REFUSAL in text:
        return {"matched": QUARANTINE_REFUSAL, "session_states": sorted({str(item.get("state")) for item in sessions})}
    return None


Wo07Scenario.refusal_seen = _refusal_seen  # type: ignore[attr-defined]


def _readmit(runner: fx.Runner, ops: fx.Ops, step: Callable[..., Any], scn: Wo07Scenario, env: dict[str, Any], leader: dict[str, Any],
             snapshot: dict[str, Any], node_id: str, window: dict[str, Any]) -> dict[str, Any]:
    """The operator readmission procedure, one runner step each, in the documented order."""
    cfg = scn.nomad_cfg
    marker_path = f"kandev/quarantine/{node_id}"
    out: dict[str, Any] = {"order": list(READMIT_STEPS), "endpoint": READMIT_PATH}

    def resume() -> None:
        scn.verify_agentctl(leader)
        ops.signal_pid(runner.ssh, leader["pid"], "CONT")

    def kill_tree() -> dict[str, Any]:
        merged = {(item["pid"], item["started"]): item for item in snapshot["tree"]}
        merged.update({(item["pid"], item["started"]): item for item in scn.tree(env)})
        rows = sorted(merged.values(), key=lambda item: -item["depth"])
        killed = {str(item["pid"]): ops.kill_exact(runner.ssh, item, wo06._needles(env)) for item in rows}
        scn.poll(lambda: not scn.tree(env) and not scn.alive(rows), "the exact process tree of A is still present after the kill")
        return {"killed": killed, "tree_absent": True}

    def stop_job() -> str:
        ops.nomad_stop(cfg["address"], runner.token, cfg["namespace"], env["job_id"])
        scn.poll(lambda: all(item.get("ClientStatus") in fx.TERMINAL_ALLOCATION_STATES for item in scn.job_allocs(env["job_id"])),
                 "allocations of A did not become terminal after the job was stopped")
        return "stopped purge=false"

    def remove_fence() -> str:
        return scn.remove_fence(env["fence_sha256"])

    def readmit() -> Any:
        state = scn.read_state(env)
        return runner.api("POST", READMIT_API_PATH, readmit_body(env, node_id, state)) or {}

    def delete_marker() -> str:
        result = ops.nomad_delete_variable(cfg["address"], runner.token, cfg["namespace"], marker_path)
        scn.poll(lambda: scn.quarantine_marker() is None, "quarantine marker still exists after deletion")
        return result

    def enable() -> str:
        ops.nomad_set_eligibility(cfg["address"], runner.token, cfg["region"], node_id, True)
        scn.poll(scn.node_ready, "node did not become ready and eligible after readmission")
        window["restored"] = True
        runner.save_state(eligibility_window=window)
        return "enabled"

    actions = dict(zip(READMIT_STEPS, (resume, kill_tree, stop_job, remove_fence, readmit, delete_marker, enable)))
    for name in READMIT_STEPS:
        result = step(f"readmit-{name}", actions[name])
        if isinstance(result, (str, dict)):
            out[name] = result if isinstance(result, str) else {key: value for key, value in result.items() if key != "attestation"}
    return out


# -- entry points ----------------------------------------------------------------------------------------
DRIVES = {"cleanup-positive": drive_cleanup_positive, "cleanup-negative": drive_cleanup_negative}


def check_scenario_config(cfg: dict[str, Any], scenario: str) -> None:
    if scenario not in DRIVES:
        raise PreflightError("unknown WO07 scenario")
    parsed = urlsplit(cfg["nomad"]["address"])
    if parsed.scheme != "http" or parsed.hostname not in ("127.0.0.1", "localhost", "::1"):
        raise PreflightError("the cleanup scenarios mutate Nomad: nomad.address must be a loopback http address")


def run_scenario(config: Any, scenario: str, ops: fx.Ops | None = None, **kwargs: Any) -> int:
    ops = ops or fx.Ops()

    def check(cfg: dict[str, Any]) -> None:
        check_scenario_config(cfg, scenario)
        wo06.check_no_quarantine_marker(cfg, ops)

    return fx.run_create_fixture(
        config, ops=ops, drive=DRIVES[scenario], mode=f"created-real-{scenario}", label=scenario,
        stop_owned_on_failure=True, check_config=check, state_extra=wo06._state_extra(scenario), **kwargs,
    )


def create_fixture_command(config_path: str, scenario: str) -> int:
    try:
        config = json.loads(Path(config_path).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        print(f"real {scenario} bootstrap failed: config is unreadable", file=sys.stderr)
        return 2
    return run_scenario(config, scenario)
