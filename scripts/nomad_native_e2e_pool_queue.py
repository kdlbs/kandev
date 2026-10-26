"""pool-queue create-fixture scenario: a third environment queues at pool capacity, then is placed after a release.

The fixture pool has one local and one ssh node. The profile memory is sized per scenario so each node holds exactly
one environment: A and B fill the pool, C is registered but never allocated (blocked evaluation, no fence, no
processes). Archiving A releases its node through the production cleanup path, C takes that node, and B stays
untouched throughout. No operator cleanup is permitted.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any, Callable

import nomad_native_e2e_fixture as fx
import nomad_native_e2e_pool as pool
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError

SCENARIO = "pool-queue"
EXHAUSTED_DIMENSION = "memory"


# -- pure helpers ----------------------------------------------------------------------------------------
def node_free_memory_mb(node: dict[str, Any]) -> int:
    """Schedulable memory of a node: NodeResources minus ReservedResources."""
    try:
        total = int(node["NodeResources"]["Memory"]["MemoryMB"])
    except (KeyError, TypeError, ValueError):
        raise PreflightError(f"node {node.get('ID')} reports no memory capacity") from None
    reserved = (((node.get("ReservedResources") or {}).get("Memory")) or {}).get("MemoryMB") or 0
    free = total - int(reserved)
    if free <= 0:
        raise PreflightError(f"node {node.get('ID')} has no free memory")
    return free


def size_pool_memory(free_by_node: list[int]) -> dict[str, int]:
    """A memory size that fits on every node but never twice on any node, so each node holds exactly one environment."""
    if len(free_by_node) < 2:
        raise PreflightError("memory sizing needs the free memory of at least two nodes")
    lowest, highest = min(free_by_node), max(free_by_node)
    floor, ceiling = highest // 2 + 1, lowest
    if floor > ceiling:
        raise PreflightError(f"no profile memory holds exactly one environment per node (free memory {sorted(free_by_node)} MB)")
    memory_mb = floor + (ceiling - floor) // 2
    return {"memory_mb": memory_mb, "memory_max_mb": memory_mb, "min_free_mb": lowest, "max_free_mb": highest}


def with_memory(cfg: dict[str, Any], sizing: dict[str, int]) -> dict[str, Any]:
    """A copy of the bootstrap config carrying the per-scenario memory; the original is not modified."""
    return {**cfg, "memory_mb": sizing["memory_mb"], "pool": {**cfg["pool"], "memory_max_mb": sizing["memory_max_mb"]}}


def blocked_proof(evals: list[dict[str, Any]], dimension: str = EXHAUSTED_DIMENSION) -> dict[str, Any] | None:
    """The queued proof from a job's evaluations: a blocked evaluation whose failed placements name the dimension."""
    named = {str(dim).lower() for item in evals for group in (item.get("FailedTGAllocs") or {}).values() for dim in (group.get("DimensionExhausted") or {})}
    if any(item.get("Status") == "blocked" for item in evals) and dimension in named:
        return {"blocked_evaluation": True, "dimensions_exhausted": sorted(named)}
    return None


def check_fence_order(a_fence_present: bool, c_fence_present: bool) -> None:
    """C may hold a worker fence only once A's is gone (A's is read after C's, so a present A fence was present throughout)."""
    if c_fence_present and a_fence_present:
        raise PreflightError("C's worker fence appeared while A's fence was still held")


def check_same_node(released_node: str, placed_node: str) -> None:
    if released_node != placed_node:
        raise PreflightError("C was not placed on the node A released")


# -- scenario --------------------------------------------------------------------------------------------
class PoolQueueScenario(pool.TwoNodeScenario):
    node_roots: list[tuple[str, str]]

    def verify_two_nodes(self) -> dict[str, Any]:
        result = super().verify_two_nodes()
        self.node_roots = [((n.get("Meta") or {})["kandev.attach"], (n.get("Meta") or {})["kandev.state_root"]) for n in self.pool_nodes()]
        return result

    def size_memory(self) -> dict[str, int]:
        return size_pool_memory([node_free_memory_mb(node) for node in self.pool_nodes()])

    def fence_on_any_node(self, env: dict[str, Any]) -> list[str]:
        """Attach kinds of the nodes holding a worker fence for the environment."""
        held = []
        for attach, root in self.node_roots:
            self.r.use_node(attach, root)
            if self.r.fence_owner(env["environment_id"]) is not None:
                held.append(attach)
        return held

    def processes_on_any_node(self, env: dict[str, Any]) -> int:
        count = 0
        for attach, root in self.node_roots:
            self.r.use_node(attach, root)
            count += len(self.tree(env))
        return count

    def assert_queued(self, label: str, task: dict[str, str]) -> dict[str, Any]:
        """Job registered, no allocation, blocked evaluation naming memory, inventory not ready, no fence and no processes anywhere."""
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
                raise PreflightError(f"environment {label} was allocated although the pool is at capacity")
            return blocked_proof(self.get_or_none(f"/v1/job/{env['job_id']}/evaluations") or [])

        proof = self.poll(queued, f"environment {label} never showed a blocked evaluation naming {EXHAUSTED_DIMENSION}")
        if self.job_allocs(env["job_id"]):
            raise PreflightError(f"environment {label} was allocated after the queued proof")
        phase = self.r.inventory(task["session_id"], task["id"]).get("phase")
        if phase == "ready":
            raise PreflightError(f"queued environment {label} has inventory phase ready")
        fences = self.fence_on_any_node(env)
        if fences:
            raise PreflightError(f"queued environment {label} holds a worker fence on {fences}")
        if self.processes_on_any_node(env):
            raise PreflightError(f"remote processes reference queued environment {label}")
        result = {**proof, "allocations": 0, "inventory_phase": phase, "fences": 0, "remote_processes": 0}
        self.data["identities"][label].update(queued=result)
        return result

    def assert_untouched(self, label: str, task: dict[str, str], before: dict[str, Any]) -> dict[str, Any]:
        """The environment's inventory, Nomad view, process tree and fence equal their snapshot from when it became ready."""
        env = self.envs[label]
        self.focus(env)
        inventory = self.r.inventory(task["session_id"], task["id"])
        if inventory != before["inventory"] or self.r.nomad_snapshot(inventory) != before["nomad"]:
            raise PreflightError(f"environment {label} inventory or allocation drifted")
        if self.tree(env) != before["tree"] or self.r.fence() != before["fence"]:
            raise PreflightError(f"environment {label} process tree or fence changed")
        return {"inventory_unchanged": True, "tree_unchanged": True, "fence_unchanged": True}

    def settle_with_fence_order(self, env: dict[str, Any], waiting: dict[str, Any], label: str) -> dict[str, Any]:
        """Poll the production cleanup of env while asserting the waiting environment holds no fence before env's is gone."""
        pending: list[str] = []

        def probe() -> dict[str, Any] | None:
            c_held = bool(self.fence_on_any_node(waiting))
            self.focus(env)
            a_held = self.r.fence() is not None
            check_fence_order(a_held, c_held)
            return self.production_settled(env, pending)

        return self.settle_note(pending, f"production cleanup of {label} did not settle before timeout")(probe)

    def settle(self, env: dict[str, Any]) -> dict[str, Any]:
        pending: list[str] = []

        def probe() -> dict[str, Any] | None:
            self.focus(env)
            return self.production_settled(env, pending)

        return self.settle_note(pending, f"production cleanup of {env['label']} did not settle before timeout")(probe)


def _await_c_ready(scn: PoolQueueScenario, step: Callable[..., Any], task_c: dict[str, str], env_a: dict[str, Any]) -> dict[str, Any]:
    ready = step("await-c-placed-and-ready", lambda: scn.await_placed("C", task_c))
    env_c = scn.envs["C"]
    step("assert-c-on-released-node", lambda: check_same_node(env_a["node_id"], env_c["node_id"]) or {"node_id": env_c["node_id"]})

    def placement() -> dict[str, Any]:
        if len(scn.job_allocs(env_c["job_id"])) != 1:
            raise PreflightError("environment C does not have exactly one allocation")
        scn.focus(env_a)
        a_held = scn.r.fence() is not None
        scn.focus(env_c)
        c_held = scn.r.fence() is not None
        if not c_held:
            raise PreflightError("environment C has no worker fence after reaching ready")
        check_fence_order(a_held, c_held)
        return {"node_id": env_c["node_id"], "allocations": 1, "fence_owner_sha256": ready["fence"], "a_fence_absent": True}

    step("assert-c-placement-after-a-fence", placement)
    return ready


def drive_pool_queue(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    guard = wo07.OperatorGuard(ops)
    scn = PoolQueueScenario(runner, cfg, SCENARIO, guard)
    step = runner.step
    try:
        scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
        step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
        scn.data["nodes"] = step("verify-two-nodes", scn.verify_two_nodes)
        sizing = scn.data["sizing"] = step("size-profile-memory", scn.size_memory)
        sized = with_memory(cfg, sizing)
        scn.cfg = sized
        step("start-git-daemon", runner.start_git_daemon)
        step("start-backend", lambda: runner.start_backend(None))
        step("create-fixture-objects", lambda: runner.setup_api_objects(sized))
        task_a = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
        snap_a = step("await-a-running", lambda: scn.await_placed("A", task_a))
        task_b = runner.create_task("Nomad pool fixture B")
        snap_b = step("await-b-running", lambda: scn.await_placed("B", task_b))
        env_a, env_b = scn.envs["A"], scn.envs["B"]

        def spread() -> dict[str, Any]:
            if env_a["node_id"] == env_b["node_id"]:
                raise PreflightError("A and B landed on the same node; the pool is not full")
            return {"A": env_a["attach"], "B": env_b["attach"]}

        scn.data["spread"] = step("assert-a-and-b-on-different-nodes", spread)
        layouts = {label: step(f"assert-workspace-{label.lower()}", lambda e=scn.envs[label]: scn.workspace_layout(e)) for label in ("A", "B")}
        scn.known_jobs = scn.kandev_jobs()
        task_c = runner.create_task("Nomad pool fixture C")
        scn.data["queued"] = step("assert-c-queued", lambda: scn.assert_queued("C", task_c))
        env_c = scn.envs["C"]
        scn.data["untouched_while_queued"] = {
            "A": step("assert-a-untouched-while-queued", lambda: scn.assert_untouched("A", task_a, snap_a)),
            "B": step("assert-b-untouched-while-queued", lambda: scn.assert_untouched("B", task_b, snap_b)),
        }
        step("archive-task-a", lambda: scn.archive(task_a))
        scn.data["identities"]["A"]["cleanup"] = step("await-cleanup-a", lambda: scn.settle_with_fence_order(env_a, env_c, "A"))
        step("assert-cleaned-a", lambda: scn.assert_pool_cleaned(env_a, layouts["A"]))
        _await_c_ready(scn, step, task_c, env_a)
        scn.data["b_untouched"] = step("assert-b-untouched-after-c-placed", lambda: scn.assert_untouched("B", task_b, snap_b))
        layouts["C"] = step("assert-workspace-c", lambda: scn.workspace_layout(scn.envs["C"]))
        for label, task in (("B", task_b), ("C", task_c)):
            step(f"archive-task-{label.lower()}", lambda t=task: scn.archive(t))
        for label in ("B", "C"):
            scn.data["identities"][label]["cleanup"] = step(f"await-cleanup-{label.lower()}", lambda e=scn.envs[label]: scn.settle(e))
            step(f"assert-cleaned-{label.lower()}", lambda e=scn.envs[label], l=layouts[label]: scn.assert_pool_cleaned(e, l))

        def assert_no_operator() -> None:
            guard.assert_clean()
            guard.armed = False

        step("assert-no-operator-cleanup", assert_no_operator)
        cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    finally:
        guard.restore()
    return {"scenario": scn.name, "pool": True, "pool_queue": True, "production_cleanup": True, "operator_cleanup_used": False,
            "release": "none", "sizing": sizing, "queued": scn.data["queued"], "cleanup": cleanup, "environments": wo07._env_report(scn)}
