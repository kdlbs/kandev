"""Pool partition scenario: the ssh node of a two-node pool is cut off from the Nomad server, the local node is untouched.

pool-partition launches two environments (one per node) and freezes the ssh node's Nomad client agent with SIGSTOP, so it
stops heartbeating while the shared server and the other client keep running. While frozen and after SIGCONT it proves
that the local environment is unchanged and still answers a prompt, while the ssh environment keeps its single original
allocation with no replacement and no new job. It then archives both through production cleanup. The agent's pid and
config path come from the bootstrap `ssh_client` block and its command line is verified before every signal; its raw_exec
allocations run under a separate executor process and keep running while the agent is frozen. A single-node relay cannot
do this on a one-server fixture because the server RPC is shared by every client.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

import nomad_native_e2e_fixture as fx
import nomad_native_e2e_pool as pool
import nomad_native_e2e_wo06 as wo06
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError

SCENARIO = "pool-partition"
PROBE_PROMPT = "For this inert partition check, output exactly WO05_PARTITION_LOCAL_PROBE"
ISOLATION_FAILURE = "the ssh node never left ready while its Nomad client agent was stopped"
PROMPTABLE_STATES = ("WAITING_FOR_INPUT", "COMPLETED", "IDLE")
FROZEN_NOTE = ("the ssh node Nomad client agent (pid {pid}) may still be stopped; run `kill -CONT {pid}` after confirming "
               "`ps -p {pid} -o command=` shows the nomad agent started with {config}")


def check_scenario_config(cfg: dict[str, Any]) -> None:
    pool.check_scenario_config(cfg, "pool-two-node")
    client = cfg.get("ssh_client")
    if not isinstance(client, dict) or not client.get("pid") or not client.get("config"):
        raise PreflightError("pool-partition requires an ssh_client block (pid, config) in the bootstrap config naming the "
                             "ssh node's Nomad client agent process")


class ClientFreezer:
    """STOP and CONT of one recorded Nomad client agent. The process identity is verified before every signal."""

    def __init__(self, ops: fx.Ops, client: dict[str, Any]):
        self.ops, self.pid, self.config, self.partitioned = ops, int(client["pid"]), str(client["config"]), False

    def verify(self) -> None:
        command = self.ops.process_command(self.pid)
        if command is None:
            raise PreflightError(f"ssh node Nomad client agent pid {self.pid} is not running")
        words = command.split()
        if "agent" not in words or not any(Path(w).name == "nomad" for w in words) or self.config not in command:
            raise PreflightError(f"pid {self.pid} is not the recorded Nomad client agent; refusing to signal it")

    def partition(self) -> None:
        self.verify()
        self.partitioned = True
        self.ops.signal_local(self.pid, "STOP")

    def heal(self) -> None:
        self.verify()
        self.ops.signal_local(self.pid, "CONT")
        self.partitioned = False


class PartitionScenario(pool.TwoNodeScenario):
    def node_info(self, node_id: str) -> tuple[Any, Any]:
        node = self.r.nomad(f"/v1/node/{node_id}")
        return node.get("Status"), node.get("SchedulingEligibility")

    def message_ids(self, env: dict[str, Any]) -> list[Any]:
        return [item.get("id") for item in self.r.api("GET", f"/task-sessions/{env['session_id']}/messages", None).get("messages", [])]

    def snapshot_env(self, env: dict[str, Any]) -> dict[str, Any]:
        self.focus(env)
        inventory = self.r.inventory(env["session_id"], env["task_id"])
        return {"inventory": inventory, "nomad": self.r.nomad_snapshot(inventory), "fence": self.r.fence(env["environment_id"]),
                "tree_ident": wo06._ident(self.tree(env)), "messages": self.message_ids(env)}

    def check_new_jobs(self, when: str) -> None:
        new_jobs = self.kandev_jobs() - self.known_jobs
        if new_jobs:
            raise PreflightError(f"{when}: unexpected new kandev job(s) {sorted(new_jobs)}")

    def check_allocation(self, env: dict[str, Any], when: str) -> str | None:
        allocs = self.job_allocs(env["job_id"])
        if len(allocs) != 1:
            raise PreflightError(f"{when}: job of environment {env['label']} has {len(allocs)} allocations, expected exactly one (no replacement)")
        if allocs[0]["ID"] != env["allocation_id"]:
            raise PreflightError(f"{when}: allocation of environment {env['label']} was replaced")
        status = self.alloc(env["allocation_id"]).get("ClientStatus")
        if status in fx.TERMINAL_ALLOCATION_STATES:
            raise PreflightError(f"{when}: allocation of environment {env['label']} became {status!r}; the original allocation must survive")
        self.check_new_jobs(when)
        return status

    def check_unchanged(self, env: dict[str, Any], snap: dict[str, Any], when: str, messages: list[Any], report_allowed: bool = False) -> None:
        """Everything observable about an environment equals its snapshot, except the message list: it must equal `messages`, or when
        `report_allowed` only keep them as a prefix because Kandev may append an unavailability report to a partitioned session."""
        self.focus(env)
        label = env["label"]
        if self.r.inventory(env["session_id"], env["task_id"]) != snap["inventory"]:
            raise PreflightError(f"{when}: durable inventory of environment {label} changed")
        if wo06._ident(self.tree(env)) != snap["tree_ident"]:
            raise PreflightError(f"{when}: remote process tree of environment {label} changed")
        if self.r.fence(env["environment_id"]) != snap["fence"]:
            raise PreflightError(f"{when}: worker fence of environment {label} changed")
        if self.r.nomad_snapshot(snap["inventory"]) != snap["nomad"]:
            raise PreflightError(f"{when}: Nomad job or allocation identity of environment {label} changed")
        current = self.message_ids(env)
        if (current[:len(messages)] if report_allowed else current) != messages:
            raise PreflightError(f"{when}: session messages of environment {label} changed unexpectedly")

    def session_state(self, task: dict[str, str]) -> Any:
        sessions = self.r.api("GET", f"/tasks/{task['id']}/sessions", None).get("sessions", [])
        return next((item.get("state") for item in sessions if item.get("id") == task["session_id"]), None)

    def await_promptable(self, task: dict[str, str], when: str) -> None:
        seen: list[Any] = []

        def promptable() -> bool:
            seen[:] = [self.session_state(task)]
            return seen[0] in PROMPTABLE_STATES

        try:
            self.poll(promptable, "the local session never became promptable")
        except PreflightError as exc:
            raise PreflightError(f"the local session was not ready for input {when} (state {seen[0] if seen else None!r}): {exc}") from None

    def probe_local(self, env: dict[str, Any], task: dict[str, str], before: list[Any]) -> list[Any]:
        """The only prompt sent after the snapshot. The local environment must accept it and answer it: the prompt message and at
        least one reply appear and the session returns to a promptable state. The returned list is the settled message list."""
        self.await_promptable(task, "before the probe prompt")
        ack = self.ops.prompt({"ws_base": f"ws://127.0.0.1:{self.r.state['port']}", "task_id": task["id"], "session_id": task["session_id"], "content": PROBE_PROMPT})
        if ack.get("accepted") is not True:
            raise PreflightError("the local environment did not accept the prompt sent during the partition")

        def answered() -> list[Any] | None:
            ids = self.message_ids(env)
            return ids if len(ids) > len(before) + 1 and self.session_state(task) in PROMPTABLE_STATES else None

        after = self.poll(answered, "the local environment did not answer the prompt sent during the partition")
        if after[:len(before)] != before:
            raise PreflightError("the local session lost earlier messages")
        return after


def _partition_steps(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, scn: PartitionScenario, freezer: ClientFreezer, guard: wo07.OperatorGuard) -> dict[str, Any]:
    step = runner.step
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
    tasks = {"A": task_a, "B": task_b}
    if scn.envs["A"]["node_id"] == scn.envs["B"]["node_id"]:
        raise PreflightError("both environments landed on the same node; spread placement did not use the second node")
    by_attach = {env["attach"]: env for env in scn.envs.values()}
    if sorted(by_attach) != ["local", "ssh"]:
        raise PreflightError("the two environments must run on one local node and one ssh node")
    local, remote = by_attach["local"], by_attach["ssh"]
    local_task, remote_task = tasks[local["label"]], tasks[remote["label"]]
    scn.data["spread"] = {"local": local["label"], "ssh": remote["label"]}
    layouts = {label: step(f"assert-workspace-{label.lower()}", lambda e=scn.envs[label]: scn.workspace_layout(e)) for label in ("A", "B")}
    scn.data["layouts"] = layouts
    for label in ("A", "B"):
        step(f"start-managed-process-tree-{label.lower()}", lambda e=scn.envs[label], t=tasks[label]: scn.start_managed_tree(e, t, {}))
    scn.known_jobs = scn.kandev_jobs()
    snaps = {"local": step("snapshot-local", lambda: scn.snapshot_env(local)), "ssh": step("snapshot-ssh", lambda: scn.snapshot_env(remote))}
    if not snaps["ssh"]["tree_ident"] or not snaps["local"]["tree_ident"]:
        raise PreflightError("an environment has no remote process tree to compare across the partition")
    messages = {"local": snaps["local"]["messages"]}
    seen = scn.data["observed_statuses"] = []

    def observe(phase: str) -> str | None:
        status = scn.check_allocation(remote, f"{phase} partition")
        seen.append({"phase": phase, "node": scn.node_info(remote["node_id"])[0], "ssh_allocation": status})
        return status

    def partition() -> None:
        freezer.partition()

        def isolated() -> bool:
            observe("during")
            if scn.node_info(local["node_id"])[0] != "ready":
                raise PreflightError("the local node left ready while only the ssh node was partitioned")
            return scn.node_info(remote["node_id"])[0] != "ready"

        scn.poll(isolated, ISOLATION_FAILURE)
        scn.check_allocation(local, "during partition")
        scn.check_unchanged(local, snaps["local"], "during partition", messages["local"])
        messages["local"] = scn.probe_local(local, local_task, messages["local"])
        scn.check_unchanged(local, snaps["local"], "after the probe prompt", messages["local"])
        scn.check_allocation(remote, "during partition")

    step("partition-and-assert-isolation", partition)

    def heal() -> dict[str, Any]:
        freezer.heal()
        scn.poll(lambda: scn.node_info(remote["node_id"]) == ("ready", "eligible"), "the ssh node did not return to ready and eligible after the partition healed")
        status = scn.poll(lambda: (lambda s: s if s not in (None, "unknown") else None)(observe("healed")),
                          "allocation of the ssh environment stayed unknown after the partition healed")
        if status != "running":
            raise PreflightError(f"allocation of the ssh environment has ClientStatus {status!r} after the heal, expected running")
        scn.check_unchanged(remote, snaps["ssh"], "after heal", snaps["ssh"]["messages"], report_allowed=True)
        scn.check_allocation(local, "after heal")
        scn.check_unchanged(local, snaps["local"], "after heal", messages["local"])
        return {"ssh_allocation_client_status": status, "prompts_sent_to_ssh": 0, "prompts_sent_to_local": 1}

    scn.data["after_heal"] = step("heal-and-assert-original-allocations", heal)
    for label in ("A", "B"):
        step(f"archive-task-{label.lower()}", lambda t=tasks[label]: scn.archive(t))
    for label in ("A", "B"):
        def settled(e=scn.envs[label]) -> dict[str, Any]:
            scn.r.use_node(e["attach"], e["state_root"])
            pending: list[str] = []
            return scn.settle_note(pending, f"production cleanup of {e['label']} did not settle before timeout")(lambda: scn.production_settled(e, pending))

        scn.data["identities"][label]["cleanup"] = step(f"await-cleanup-{label.lower()}", settled)
        step(f"assert-cleaned-{label.lower()}", lambda e=scn.envs[label], l=layouts[label]: scn.assert_pool_cleaned(e, l))

    def assert_no_operator() -> None:
        guard.assert_clean()
        guard.armed = False

    step("assert-no-operator-cleanup", assert_no_operator)
    return step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))


def drive_pool_partition(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    check_scenario_config(cfg)
    guard = wo07.OperatorGuard(ops)
    scn = PartitionScenario(runner, cfg, SCENARIO, guard)
    freezer = ClientFreezer(ops, cfg["ssh_client"])
    try:
        cleanup = _partition_steps(runner, cfg, ops, scn, freezer, guard)
    except Exception as exc:
        note = FROZEN_NOTE.format(pid=freezer.pid, config=freezer.config)
        if freezer.partitioned:
            try:
                freezer.heal()
                note = "the ssh node Nomad client agent was continued"
            except Exception as heal_exc:
                note += f" (it could not be continued: {heal_exc})"
        scn.data["client_stopped_at_exit"] = freezer.partitioned
        scn.data["client_note"] = note
        raise PreflightError(f"{exc}; {note}") from exc
    finally:
        guard.restore()
    scn.data["client_stopped_at_exit"] = freezer.partitioned
    return {"scenario": scn.name, "pool": True, "partition": True, "production_cleanup": True, "operator_cleanup_used": False,
            "release": "none", "cleanup": cleanup, "observed_statuses": scn.data["observed_statuses"], "environments": wo07._env_report(scn)}


def run_scenario(config: Any, ops: fx.Ops | None = None, **kwargs: Any) -> int:
    ops = ops or fx.Ops()

    def check(cfg: dict[str, Any]) -> None:
        check_scenario_config(cfg)
        wo06.check_no_quarantine_marker(cfg, ops)

    return fx.run_create_fixture(
        config, ops=ops, drive=drive_pool_partition, mode=f"created-real-{SCENARIO}", label=SCENARIO,
        stop_owned_on_failure=True, check_config=check, state_extra=wo06._state_extra(SCENARIO), **kwargs,
    )


def create_fixture_command(config_path: str) -> int:
    try:
        config = json.loads(Path(config_path).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        print(f"real {SCENARIO} bootstrap failed: config is unreadable", file=sys.stderr)
        return 2
    return run_scenario(config)
