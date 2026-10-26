"""Pool create-fixture scenario: placement onto a node pool, then production cleanup, on a real worker.

pool-placement creates a pool profile (a node pool and sizing, no node id and no SSH target), launches a task and
proves the job named no node, Nomad placed the single allocation on a node carrying the Kandev node contract, the
durable state is pool-placed on that node, and the agent is attached. It then runs the same archive-and-assert
production cleanup as cleanup-positive.
"""

from __future__ import annotations

import json
import shlex
import sys
from pathlib import Path
from typing import Any, Callable
from urllib.parse import urlsplit

import nomad_native_e2e_fixture as fx
import nomad_native_e2e_wo06 as wo06
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError

SCENARIOS = ("pool-placement", "pool-two-node", "pool-node-quarantine", "pool-queue", "pool-refusals")
CONTRACT_KEYS = ("kandev.attach", "kandev.state_root")
NODE_ID_TARGET = "${node.unique.id}"


def contract_problems(meta: dict[str, Any]) -> list[str]:
    """Why a node's metadata is not a usable Kandev node contract; empty when it is."""
    problems = [f"missing {key}" for key in CONTRACT_KEYS if not meta.get(key)]
    attach = meta.get("kandev.attach")
    if attach not in (None, "", "local", "ssh"):
        problems.append(f"kandev.attach {attach!r} is not local or ssh")
    if attach == "ssh":
        problems += [f"missing {key}" for key in ("kandev.ssh_host", "kandev.ssh_host_key_sha256") if not meta.get(key)]
    if attach == "local" and not meta.get("kandev.host_identity_sha256"):
        problems.append("missing kandev.host_identity_sha256")
    return problems


def job_named_node(job: dict[str, Any]) -> bool:
    """True when the registered job pins itself to a node by id, at job, group or task level."""
    scopes = [job, *(job.get("TaskGroups") or [])]
    scopes += [task for group in job.get("TaskGroups") or [] for task in group.get("Tasks") or []]
    return any(c.get("LTarget") == NODE_ID_TARGET for scope in scopes for c in scope.get("Constraints") or [])


class PoolScenario(wo07.Wo07Scenario):
    def verify_node_identity(self) -> dict[str, Any]:
        """Pool paths come from the node's own kandev.state_root; they are recorded so cleanup-fixture uses the same ones."""
        identity = super().verify_node_identity()
        problems = contract_problems(self.node().get("Meta") or {})
        if problems:
            raise PreflightError(f"the fixture node does not carry the Kandev node contract: {', '.join(problems)}")
        self.r.save_state(pool_state_root=self.node()["Meta"]["kandev.state_root"])
        return {**identity, "pool_state_root": self.r.state["pool_state_root"]}

    def production_settled(self, env: dict[str, Any], pending: list[str]) -> dict[str, Any] | None:
        """Cleanup is settled only once the pool workspace and the per-environment fence directory are gone as well."""
        result = super().production_settled(env, pending)
        if result is None:
            return None
        workspace = self.read_state(env).get("workspace_path")
        state_root = self.r.state["pool_state_root"]
        if not workspace or not workspace.startswith(state_root + "/"):
            raise PreflightError("the recorded pool workspace is not under the node state root")
        remaining = []
        for label, path in (("workspace", workspace), ("fence directory", f"{state_root}/environments/{env['environment_id']}"),
                            ("allocation directory", f"{state_root}/pool/allocations/{env['job_id']}")):
            code, _ = self.r.ssh(f"[ -e '{path}' ] || [ -L '{path}' ]")
            if code == 0:
                remaining.append(label)
        if remaining:
            pending.append(f"pool {', '.join(remaining)} remains")
            return None
        return {**result, "workspace_removed": True, "fence_directory_removed": True,
                  "allocation_directory_removed": True}

    def assert_placement(self, env: dict[str, Any]) -> dict[str, Any]:
        pool = self.cfg["pool"]
        job = self.r.nomad(f"/v1/job/{env['job_id']}")
        job = job.get("Job", job)
        if job_named_node(job):
            raise PreflightError("the pool job names a node by id")
        if job.get("NodePool") != pool["node_pool"]:
            raise PreflightError(f"the pool job is in node pool {job.get('NodePool')!r}, expected {pool['node_pool']!r}")
        allocs = self.job_allocs(env["job_id"])
        if len(allocs) != 1:
            raise PreflightError(f"the pool job has {len(allocs)} allocations, expected exactly one")
        node = self.r.nomad(f"/v1/node/{allocs[0]['NodeID']}")
        if node.get("NodePool") != pool["node_pool"]:
            raise PreflightError("the allocation landed on a node outside the configured pool")
        problems = contract_problems(node.get("Meta") or {})
        if problems:
            raise PreflightError(f"the placed node does not carry the Kandev node contract: {', '.join(problems)}")
        state = self.read_state(env)
        if state.get("placement_mode") != "pool" or state.get("node_id") != allocs[0]["NodeID"]:
            raise PreflightError("durable state is not pool-placed on the allocation's node")
        if not state.get("node_contract_sha256"):
            raise PreflightError("durable state has no node contract digest")
        return {"node_id": state["node_id"], "node_pool": node["NodePool"], "attach": node["Meta"]["kandev.attach"],
                "job_names_node": False, "allocations": 1, "node_contract_sha256": state["node_contract_sha256"],
                "placement": state.get("placement")}

    def common_prelude(self, step: Callable[..., Any], runner: fx.Runner, cfg: dict[str, Any]) -> tuple[dict[str, str], dict[str, Any], dict[str, Any]]:
        result = super().common_prelude(step, runner, cfg)
        self.data["placement"] = step("assert-pool-placement", lambda: self.assert_placement(self.envs["A"]))
        return result


class TwoNodeScenario(PoolScenario):
    """Two environments, one per node: spread, local worktree versus remote checkout, and proven cleanup of both."""

    def pool_nodes(self) -> list[dict[str, Any]]:
        pool = self.cfg["pool"]["node_pool"]
        nodes = []
        for stub in self.r.nomad("/v1/nodes"):
            node = self.r.nomad(f"/v1/node/{stub['ID']}")
            if node.get("NodePool") == pool and node.get("Status") == "ready" and node.get("SchedulingEligibility") == "eligible":
                nodes.append(node)
        return nodes

    def verify_two_nodes(self) -> dict[str, Any]:
        nodes = self.pool_nodes()
        attaches = sorted((n.get("Meta") or {}).get("kandev.attach", "") for n in nodes)
        if attaches != ["local", "ssh"]:
            raise PreflightError(f"the pool needs exactly one ready local node and one ready ssh node, found attach values {attaches}")
        for node in nodes:
            problems = contract_problems(node.get("Meta") or {})
            if problems:
                raise PreflightError(f"node {node['ID']} does not carry the Kandev node contract: {', '.join(problems)}")
        return {"nodes": {n["ID"]: n["Meta"]["kandev.attach"] for n in nodes}}

    def await_placed(self, label: str, task: dict[str, str]) -> dict[str, Any]:
        """Wait for the environment's node, point worker commands at it, then run the shared readiness checks."""
        def placed() -> dict[str, Any] | None:
            inventory = self.soft(lambda: self.r.inventory(task["session_id"], task["id"]))
            return inventory if inventory and inventory.get("node_id") and inventory.get("allocation_id") else None

        inventory = self.poll(placed, f"environment {label} was not placed on a node before timeout")
        meta = self.r.nomad(f"/v1/node/{inventory['node_id']}").get("Meta") or {}
        self.r.use_node(meta["kandev.attach"], meta["kandev.state_root"])
        # Recorded before readiness so cleanup-fixture finds the node files of a launch that fails after placement.
        self.record_env(label, task, inventory, attach=meta["kandev.attach"], state_root=meta["kandev.state_root"])
        ready = self.await_ready(label, task)
        self.envs[label]["attach"] = meta["kandev.attach"]
        self.envs[label]["state_root"] = meta["kandev.state_root"]
        self.save_envs()
        return ready

    def workspace_layout(self, env: dict[str, Any]) -> dict[str, Any]:
        """The repository directory in the workspace: a git worktree on a local node, a plain checkout on an ssh node."""
        self.r.use_node(env["attach"], env["state_root"])
        workspace = self.read_state(env)["workspace_path"]
        code, out = self.r.ssh(f'for d in {workspace}/*/; do [ -e "${{d}}.git" ] || continue; '
                               f'if [ -f "${{d}}.git" ]; then echo worktree; else echo checkout; fi; '
                               f'git -C "$d" rev-parse --abbrev-ref HEAD; '
                               f'git -C "$d" rev-parse --path-format=absolute --git-common-dir; done')
        lines = out.split()
        if code != 0 or len(lines) != 3:
            raise PreflightError(f"environment {env['label']} workspace has no single repository: {out!r}")
        kind, branch, common = lines
        want = "worktree" if env["attach"] == "local" else "checkout"
        if kind != want:
            raise PreflightError(f"environment {env['label']} on a {env['attach']} node has a {kind}, expected a {want}")
        if kind == "worktree" and branch != f"kandev/pool/{env['environment_id']}":
            raise PreflightError(f"the local worktree is on branch {branch!r}, expected the per-environment branch")
        return {"kind": kind, "branch": branch, "git_common_dir": common, "workspace": workspace}

    def assert_pool_cleaned(self, env: dict[str, Any], layout: dict[str, Any]) -> dict[str, Any]:
        self.r.use_node(env["attach"], env["state_root"])
        if layout["kind"] == "worktree":
            code, _ = self.r.ssh(f"git --git-dir='{layout['git_common_dir']}' show-ref --verify --quiet 'refs/heads/{layout['branch']}'")
            if code == 0:
                raise PreflightError(f"environment {env['label']} left the branch {layout['branch']} in the source repository")
            code, out = self.r.ssh(f"git --git-dir='{layout['git_common_dir']}' worktree list --porcelain")
            if code != 0 or env["environment_id"] in out:
                raise PreflightError(f"environment {env['label']} left a worktree registration in the source repository")
        return {"kind": layout["kind"], "branch_removed": layout["kind"] == "worktree", "workspace_removed": True}

    def remove_allocation_dir(self, env: dict[str, Any]) -> dict[str, Any]:
        """The readmission procedure's pool step: once the runtime is verified absent, delete the retained job directory and assert it is gone."""
        self.focus(env)
        root, job_id = self.pool_node_root(env), env["job_id"]
        if not root or not root.startswith("/") or ".." in root or not wo06._SAFE_ID.fullmatch(job_id):
            raise PreflightError("recorded pool paths are not safe to remove")
        alloc = f"{root}/pool/allocations/{job_id}"
        code, _ = self.r.ssh(f"[ -L {shlex.quote(alloc)} ] && exit 1; rm -rf -- {shlex.quote(alloc)}")
        if code != 0 or self.r.ssh(f"[ -e {shlex.quote(alloc)} ] || [ -L {shlex.quote(alloc)} ]")[0] == 0:
            raise PreflightError(f"the allocation directory of {env['label']} could not be removed after readmission")
        return {"allocation_directory_removed": True}

    def remove_b_leftovers(self, env: dict[str, Any], layout: dict[str, Any]) -> dict[str, Any]:
        """Operator teardown released the environment, so its worktree, branch and node directories are removed here and asserted gone."""
        self.focus(env)
        removed = self.remove_pool_leftovers(env, True)
        return {**removed, **self.assert_pool_cleaned(env, layout)}


def drive_pool_two_node(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    guard = wo07.OperatorGuard(ops)
    scn = TwoNodeScenario(runner, cfg, "pool-two-node", guard)
    step = runner.step
    try:
        scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
        step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
        scn.data["nodes"] = step("verify-two-nodes", scn.verify_two_nodes)
        step("start-git-daemon", runner.start_git_daemon)
        step("start-backend", lambda: runner.start_backend(None))
        step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
        task_a = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
        step("await-a-running", lambda: scn.await_placed("A", task_a))
        task_b = runner.create_task("Nomad pool fixture B")
        step("await-b-running", lambda: scn.await_placed("B", task_b))
        env_a, env_b = scn.envs["A"], scn.envs["B"]

        def spread() -> dict[str, Any]:
            if env_a["node_id"] == env_b["node_id"]:
                raise PreflightError("both environments landed on the same node; spread placement did not use the second node")
            return {"A": env_a["attach"], "B": env_b["attach"]}

        scn.data["spread"] = step("assert-spread", spread)
        layouts = {label: step(f"assert-workspace-{label.lower()}", lambda e=scn.envs[label]: scn.workspace_layout(e)) for label in ("A", "B")}
        scn.data["layouts"] = layouts
        scn.known_jobs = scn.kandev_jobs()
        for label, task in (("A", task_a), ("B", task_b)):
            step(f"archive-task-{label.lower()}", lambda t=task: scn.archive(t))
        for label in ("A", "B"):
            def settled(e=scn.envs[label]) -> dict[str, Any]:
                scn.r.use_node(e["attach"], e["state_root"])
                pending: list[str] = []
                return scn.settle_note(pending, f"production cleanup of {e['label']} did not settle before timeout")(lambda: scn.production_settled(e, pending))

            result = step(f"await-cleanup-{label.lower()}", settled)
            scn.data["identities"][label]["cleanup"] = result
            step(f"assert-cleaned-{label.lower()}", lambda e=scn.envs[label], l=layouts[label]: scn.assert_pool_cleaned(e, l))

        def assert_no_operator() -> None:
            guard.assert_clean()
            guard.armed = False

        step("assert-no-operator-cleanup", assert_no_operator)
        cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    finally:
        guard.restore()
    return {"scenario": scn.name, "pool": True, "two_node": True, "production_cleanup": True, "operator_cleanup_used": False,
            "release": "none", "cleanup": cleanup, "environments": wo07._env_report(scn)}


def drive_pool_node_quarantine(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    """A drain failure on one node quarantines that node only: the next environment is placed on the other node."""
    guard = wo07.OperatorGuard(ops)
    scn = TwoNodeScenario(runner, cfg, "pool-node-quarantine", guard)
    step = runner.step
    try:
        scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
        step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
        scn.data["nodes"] = step("verify-two-nodes", scn.verify_two_nodes)
        step("start-git-daemon", runner.start_git_daemon)
        step("start-backend", lambda: runner.start_backend(None))
        step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
        task_a = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
        snapshot = step("await-a-running", lambda: scn.await_placed("A", task_a))
        env_a = scn.envs["A"]
        node_x = env_a["node_id"]
        scn.nomad_cfg = {**scn.nomad_cfg, "node_id": node_x}
        step("start-managed-process-tree", lambda: scn.start_managed_tree(env_a, task_a, snapshot))
        leader = next((item for item in snapshot["tree"] if item["depth"] == 0 and item["role"] == "agentctl"), None)
        if leader is None:
            raise PreflightError("environment A has no agentctl leader")
        scn.known_jobs = scn.kandev_jobs()
        window = scn.record_eligibility_original()

        def stop_leader() -> None:
            scn.focus(env_a)
            scn.verify_agentctl(leader)
            ops.signal_pid(runner.ssh, leader["pid"], "STOP")

        step("induce-drain-failure-sigstop-leader", stop_leader)
        step("archive-task-a", lambda: scn.archive(task_a))

        def await_quarantine() -> dict[str, Any]:
            scn.focus(env_a)
            pending: list[str] = []
            return scn.settle_note(pending, "environment A did not reach cleanup_pending with a quarantine marker before timeout")(
                lambda: wo07._quarantine_assertions(scn, env_a, snapshot, pending))

        scn.data["quarantined"] = step("assert-a-retained-and-quarantined", await_quarantine)

        def b_placed_elsewhere() -> dict[str, Any]:
            task_b = runner.create_task("Nomad pool fixture B (other node)")
            scn.await_placed("B", task_b)
            env_b = scn.envs["B"]
            if env_b["node_id"] == node_x:
                raise PreflightError("B was placed on the quarantined node")
            marker_b = scn.get_or_none(wo06.quarantine_marker_path(env_b["node_id"], scn.nomad_cfg["namespace"]))
            if marker_b is not None:
                raise PreflightError("the other node carries a quarantine marker")
            return {"node_a": node_x, "node_b": env_b["node_id"], "b_attach": env_b["attach"], "other_node_marker": False}

        scn.data["continued_placement"] = step("assert-b-placed-on-the-other-node", b_placed_elsewhere)
        layout_b = step("assert-workspace-b", lambda: scn.workspace_layout(scn.envs["B"]))
        scn.focus(env_a)
        readmitted = wo07._readmit(runner, ops, step, scn, env_a, leader, snapshot, node_x, window)
        readmitted["allocation_directory"] = step("readmit-remove-allocation-directory", lambda: scn.remove_allocation_dir(env_a))
        released = [scn.release("B", kill_first=False)]
        env_a["released"] = True
        scn.save_envs()
        scn.data["b_leftovers"] = step("remove-and-assert-b-leftovers", lambda: scn.remove_b_leftovers(scn.envs["B"], layout_b))
        cleanup = step("teardown", lambda: scn.finish({"released": released}))
    finally:
        guard.restore()
    return {"scenario": scn.name, "pool": True, "per_node_quarantine": True, "production_cleanup": False, "release": "operator-teardown",
            "readmission": readmitted, "cleanup": cleanup, "environments": wo07._env_report(scn)}


def drive_pool_placement(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    guard = wo07.OperatorGuard(ops)
    scn = PoolScenario(runner, cfg, "pool-placement", guard)
    try:
        report = wo07._positive_steps(runner, cfg, runner.step, scn, guard)
    finally:
        guard.restore()
    return {**report, "pool": True, "placement": scn.data.get("placement")}


def check_scenario_config(cfg: dict[str, Any], scenario: str) -> None:
    if scenario not in SCENARIOS:
        raise PreflightError("unknown pool scenario")
    if not cfg.get("pool"):
        raise PreflightError("the pool scenarios require a pool block in the bootstrap config")
    parsed = urlsplit(cfg["nomad"]["address"])
    if parsed.scheme != "http" or parsed.hostname not in ("127.0.0.1", "localhost", "::1"):
        raise PreflightError("the pool scenarios mutate Nomad: nomad.address must be a loopback http address")


def _lazy_driver(module: str, name: str) -> Callable[..., dict[str, Any]]:
    """The queue and refusals drivers import this module, so they are resolved on first use."""
    def drive(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
        return getattr(__import__(module), name)(runner, cfg, ops, root)

    return drive


DRIVERS = {"pool-placement": drive_pool_placement, "pool-two-node": drive_pool_two_node, "pool-node-quarantine": drive_pool_node_quarantine,
           "pool-queue": _lazy_driver("nomad_native_e2e_pool_queue", "drive_pool_queue"),
           "pool-refusals": _lazy_driver("nomad_native_e2e_pool_refusals", "drive_pool_refusals")}


def run_scenario(config: Any, scenario: str, ops: fx.Ops | None = None, **kwargs: Any) -> int:
    ops = ops or fx.Ops()

    def check(cfg: dict[str, Any]) -> None:
        check_scenario_config(cfg, scenario)
        wo06.check_no_quarantine_marker(cfg, ops)

    return fx.run_create_fixture(
        config, ops=ops, drive=DRIVERS[scenario], mode=f"created-real-{scenario}", label=scenario,
        stop_owned_on_failure=True, check_config=check, state_extra=wo06._state_extra(scenario), **kwargs,
    )


def create_fixture_command(config_path: str, scenario: str) -> int:
    try:
        config = json.loads(Path(config_path).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        print(f"real {scenario} bootstrap failed: config is unreadable", file=sys.stderr)
        return 2
    return run_scenario(config, scenario)
