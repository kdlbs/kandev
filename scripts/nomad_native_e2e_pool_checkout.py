"""Pool create-fixture scenarios for fixes 8 and 9: the checkout branch contract and the scheduler preflight.

pool-checkout-branch gives the fixture repository a branch holding a distinct commit and launches a task whose
repository row names that branch, once per node type (the other node is made ineligible for the case). The ssh node
must materialize the branch (HEAD equals the branch commit). The local node either carries the same commit in its
worktree or refuses the launch with the documented ``unsupported option: checkout branch`` error; the outcome that
occurred is recorded. Both cases end through production archive cleanup when it settles.

pool-binpack-refused flips the cluster scheduler algorithm to binpack with the management token, launches a task for
a spread profile, and asserts the launch is refused at preflight with the clear message and that no Nomad job was
registered. The algorithm is restored to its original value (spread) on every path.

Every fault (eligibility, the fixture branch, the scheduler algorithm) is restored in the same invocation, also when an
assertion fails; a restoration that cannot complete is recorded with the operator command.
"""

from __future__ import annotations

import json
import re
import shlex
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any, Callable
from urllib.parse import quote

import nomad_native_e2e_fixture as fx
import nomad_native_e2e_pool_refusals as refusals
import nomad_native_e2e_wo06 as wo06
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError

CHECKOUT_SCENARIO = "pool-checkout-branch"
BINPACK_SCENARIO = "pool-binpack-refused"
BRANCH = "kandev-e2e/checkout-branch"
UNSUPPORTED_CAUSE = "unsupported option: checkout branch"
BINPACK_CAUSE = "needs a spread scheduler for node pool"
SSH_CASE = {"name": "ssh", "label": "C1", "attach": "ssh"}
LOCAL_CASE = {"name": "local", "label": "C2", "attach": "local", "causes": (UNSUPPORTED_CAUSE,)}
ZERO_COMMIT = "0" * 40
_COMMIT = re.compile(r"[0-9a-f]{40}")
SCHEDULER_PATH = "/v1/operator/scheduler/configuration"
ALGORITHMS = ("spread", "binpack")
CHECKOUT_LIMITATIONS = [
    "the fixture branch is a ref in the fixture repository holding one commit on the base tree; it is deleted at the end",
    "the local node documents a refusal rather than a materialized checkout branch; both outcomes are accepted and recorded",
    "each case makes the other node ineligible so Nomad has exactly one node to place on; eligibility is restored in the same run",
]
BINPACK_LIMITATIONS = [
    "the cluster scheduler algorithm is flipped for the duration of the run; a pool-level algorithm override is refused as a precondition",
    "the refusal is asserted from the session error and the Nomad job list; the algorithm is restored in the same invocation",
]


# -- the fixture repository branch ---------------------------------------------------------------------
class FixtureBranch:
    """A branch ref with one distinct commit, created and deleted with git plumbing so HEAD and the worktree are never touched."""

    def __init__(self, ops: Any, repo: str, base_branch: str):
        self.ops, self.repo, self.base_branch = ops, repo, base_branch
        self.ref = f"refs/heads/{BRANCH}"
        self.record: dict[str, Any] | None = None

    def _git(self, *args: str, prefix: str = "") -> tuple[int, str]:
        binary = shlex.quote(self.ops.git_binary())
        return self.ops.local_exec(f"{prefix}{binary} -C {shlex.quote(self.repo)} " + " ".join(shlex.quote(arg) for arg in args))

    @property
    def restore_with(self) -> str:
        return f"git -C {self.repo} update-ref -d {self.ref}"

    def public(self) -> dict[str, Any]:
        """The record with the home directory redacted, for evidence and printed commands."""
        return {key: fx.redact_path(value) if isinstance(value, str) else value for key, value in (self.record or {}).items()}

    def create(self) -> dict[str, Any]:
        if self._git("rev-parse", "--verify", "--quiet", self.ref)[0] == 0:
            raise PreflightError(f"the branch {self.ref} already exists in the fixture repository; refusing to overwrite or delete it")
        base = self.ops.git_head(self.repo, self.base_branch)
        code, tree = self._git("rev-parse", f"{base}^{{tree}}")
        if code != 0 or not _COMMIT.fullmatch(tree.strip()):
            raise PreflightError("could not resolve the base tree of the fixture repository")
        identity = "env GIT_AUTHOR_NAME=fixture GIT_AUTHOR_EMAIL=fixture@invalid GIT_COMMITTER_NAME=fixture GIT_COMMITTER_EMAIL=fixture@invalid "
        code, commit = self._git("commit-tree", tree.strip(), "-p", base, "-m", "Nomad e2e fixture commit", prefix=identity)
        commit = commit.strip()
        if code != 0 or not _COMMIT.fullmatch(commit) or commit == base:
            raise PreflightError("could not create a distinct fixture branch commit")
        if self._git("update-ref", self.ref, commit, ZERO_COMMIT)[0] != 0:
            raise PreflightError("could not create the fixture branch in the fixture repository")
        self.record = {"branch": BRANCH, "base_commit": base, "commit": commit, "repo": self.repo, "deleted": False, "restore_with": self.restore_with}
        return self.record

    def delete(self) -> str:
        if self.record is None or self.record["deleted"]:
            return "not needed"
        if self._git("rev-parse", "--verify", "--quiet", self.ref)[0] != 0:
            self.record["deleted"] = True
            return "already absent"
        if self._git("update-ref", "-d", self.ref, self.record["commit"])[0] != 0:
            raise PreflightError(f"the fixture repository branch could not be deleted; remove it with `{fx.redact_path(self.restore_with)}`")
        self.record["deleted"] = True
        return "deleted"


# -- the scheduler configuration -----------------------------------------------------------------------
class SchedulerConfig:
    """Reads and replaces the cluster scheduler algorithm (GET/PUT /v1/operator/scheduler/configuration) with the runner token."""

    def __init__(self, address: str, token: str, region: str, opener: Any = None):
        self.address, self.token, self.region = address, token, region
        self.opener = opener or fx._nomad_opener(address)

    def _call(self, method: str, body: dict[str, Any] | None = None) -> Any:
        request = urllib.request.Request(
            f"{self.address}{SCHEDULER_PATH}?region={quote(self.region)}", data=None if body is None else json.dumps(body).encode(), method=method,
            headers={"X-Nomad-Token": self.token, "Accept": "application/json", "Content-Type": "application/json"})
        try:
            with self.opener.open(request, timeout=15) as response:
                return json.loads(response.read() or b"{}")
        except urllib.error.HTTPError as exc:
            if method == "PUT":
                raise PreflightError(f"Nomad rejected the scheduler configuration change with status {exc.code}; "
                                     "the management token needs operator:write") from None
            raise PreflightError(f"Nomad scheduler configuration read failed with status {exc.code}") from None
        except (OSError, urllib.error.URLError):
            raise PreflightError("Nomad scheduler configuration is unreachable") from None
        except json.JSONDecodeError:
            raise PreflightError("Nomad scheduler configuration response is not JSON") from None

    def read(self) -> dict[str, Any]:
        config = (self._call("GET") or {}).get("SchedulerConfig")
        if not isinstance(config, dict):
            raise PreflightError("Nomad scheduler configuration response has no SchedulerConfig")
        return config

    def set_algorithm(self, algorithm: str) -> None:
        if algorithm not in ALGORITHMS:
            raise PreflightError(f"refusing to set the scheduler algorithm to {algorithm!r}")
        body = {key: value for key, value in self.read().items() if key not in ("CreateIndex", "ModifyIndex")}
        body["SchedulerAlgorithm"] = algorithm
        if not (self._call("PUT", body) or {}).get("Updated"):
            raise PreflightError("Nomad did not apply the scheduler configuration change")


def make_scheduler(runner: fx.Runner) -> SchedulerConfig:
    nomad = runner.state["nomad"]
    return SchedulerConfig(nomad["address"], runner.token, nomad["region"])


# -- shared observation --------------------------------------------------------------------------------
def _strings(value: Any) -> list[str]:
    return list(refusals._strings(value))


class CheckoutScenario(refusals.RefusalScenario):
    """Two-node pool scenario that launches a task naming a checkout branch on one node type at a time."""

    def __init__(self, runner: fx.Runner, cfg: dict[str, Any], name: str):
        super().__init__(runner, cfg, name, None)

    def restore_and_verify(self) -> dict[str, Any]:
        failures = self.restore_all()
        if failures:
            raise PreflightError(f"restoration failed: {'; '.join(failures)}")
        self.poll(self.nodes_restored, "the pool did not return to one eligible local node and one eligible ssh node")
        return dict(self.restored)

    def assert_no_agentctl(self, name: str, env: dict[str, Any]) -> dict[str, Any]:
        """The rollback of a refused launch stops the runtime asynchronously, so absence is awaited before it is judged."""
        gone = self.soft(lambda: refusals.RefusalScenario.assert_no_agentctl(self, name, env))
        if gone:
            return gone
        try:
            return self.poll(lambda: self.soft(lambda: refusals.RefusalScenario.assert_no_agentctl(self, name, env)), "agentctl remains")
        except PreflightError:
            return refusals.RefusalScenario.assert_no_agentctl(self, name, env)

    def observe_outcome(self, name: str, task: dict[str, str]) -> dict[str, Any]:
        """Wait until the task is ready or reports the documented refusal; anything else times out."""
        def probe() -> dict[str, Any] | None:
            inventory = self.soft(lambda: self.r.inventory(task["session_id"], task["id"]))
            if inventory and inventory.get("phase") == "ready":
                return {"outcome": "ready"}
            view = self.soft(lambda: self.session_view(task))
            excerpts = [] if view is None else [text[:600] for text in _strings(list(view)) if UNSUPPORTED_CAUSE in text.lower()]
            return {"outcome": "refused", "excerpts": excerpts} if excerpts else None

        return self.poll(probe, f"{name}: no refusal naming {UNSUPPORTED_CAUSE!r} and no ready environment was reported before timeout")

    def checkout_head(self, env: dict[str, Any]) -> dict[str, Any]:
        """The repository directory in the workspace: its kind, checked-out branch, HEAD commit and git common dir."""
        self.r.use_node(env["attach"], env["state_root"])
        workspace = self.read_state(env)["workspace_path"]
        if not workspace.startswith("/"):
            raise PreflightError("the recorded workspace path is not absolute")
        code, out = self.r.ssh(f'for d in {shlex.quote(workspace)}/*/; do [ -e "${{d}}.git" ] || continue; '
                               f'if [ -f "${{d}}.git" ]; then echo worktree; else echo checkout; fi; '
                               f'git -C "$d" rev-parse --abbrev-ref HEAD; git -C "$d" rev-parse HEAD; '
                               f'git -C "$d" rev-parse --path-format=absolute --git-common-dir; done')
        lines = out.split()
        if code != 0 or len(lines) != 4:
            raise PreflightError(f"environment {env['label']} workspace has no single repository: {out!r}")
        return {"kind": lines[0], "branch": lines[1], "head": lines[2], "git_common_dir": lines[3], "workspace": workspace}

    def assert_checkout(self, env: dict[str, Any], commit: str) -> dict[str, Any]:
        layout = self.checkout_head(env)
        want = "worktree" if env["attach"] == "local" else "checkout"
        if layout["kind"] != want:
            raise PreflightError(f"environment {env['label']} on a {env['attach']} node has a {layout['kind']}, expected a {want}")
        if layout["head"] != commit:
            raise PreflightError(f"environment {env['label']} on the {env['attach']} node does not carry the fixture branch commit "
                                 f"(HEAD {layout['head']}, expected {commit})")
        if env["attach"] == "ssh" and layout["branch"] != BRANCH:
            raise PreflightError(f"environment {env['label']} is on branch {layout['branch']!r}, expected {BRANCH!r}")
        return layout

    def settle_production(self, env: dict[str, Any]) -> dict[str, Any]:
        self.r.use_node(env["attach"], env["state_root"])
        pending: list[str] = []
        return self.settle_note(pending, f"production cleanup of {env['label']} did not settle before timeout")(lambda: self.production_settled(env, pending))


def route_checkout_branch(runner: fx.Runner, branch: str) -> None:
    """Every task the runner creates from now on names the branch on its repository row (the fixture builds that row)."""
    original = runner.create_task

    def create(*args: Any, **kwargs: Any) -> dict[str, str]:
        runner._task_ctx["repositories"] = [{**row, "checkout_branch": branch} for row in runner._task_ctx["repositories"]]
        return original(*args, **kwargs)

    runner.create_task = create


def _check_checkout_config(cfg: dict[str, Any]) -> None:
    repo = cfg.get("repository") or {}
    if not repo.get("path"):
        raise PreflightError("pool-checkout-branch requires repository.path: a local fixture repository the runner can add a branch to")
    if not cfg.get("pool"):
        raise PreflightError("pool-checkout-branch requires a pool block in the bootstrap config")


def _ready_case(scn: CheckoutScenario, step: Callable[..., Any], case: dict[str, Any], task: dict[str, str], commit: str) -> dict[str, Any]:
    name, label = case["name"], case["label"]
    step(f"{name}-await-placed", lambda: scn.await_placed(label, task))
    env = scn.envs[label]
    if env["attach"] != case["attach"]:
        raise PreflightError(f"environment {label} was placed on a {env['attach']} node, expected a {case['attach']} node")
    layout = step(f"{name}-assert-checkout", lambda: scn.assert_checkout(env, commit))
    scn.known_jobs = scn.kandev_jobs()
    step(f"{name}-archive-task", lambda: scn.archive(task))
    cleanup = step(f"{name}-await-cleanup", lambda: scn.settle_production(env))
    pool_cleanup = step(f"{name}-assert-cleaned", lambda: scn.assert_pool_cleaned(env, layout))
    return {"outcome": "materialized", "node_attach": case["attach"], "branch": layout["branch"], "head": layout["head"], "expected_commit": commit,
            "workspace_kind": layout["kind"], "cleanup": cleanup, "pool_cleanup": pool_cleanup, "path": "production-archive-cleanup"}


def _refused_case(scn: CheckoutScenario, step: Callable[..., Any], case: dict[str, Any], task: dict[str, str]) -> dict[str, Any]:
    refusal = step(f"{case['name']}-assert-refusal", lambda: scn.assert_refusal(case, task))
    cleanup = step(f"{case['name']}-end-task", lambda: scn.end_case(case, task))
    return {"outcome": "refused", "node_attach": case["attach"], "refusal": refusal, "cleanup": cleanup, "path": cleanup["path"]}


def _run_case(scn: CheckoutScenario, step: Callable[..., Any], case: dict[str, Any], task: dict[str, str], commit: str) -> dict[str, Any]:
    seen = step(f"{case['name']}-observe-outcome", lambda: scn.observe_outcome(case["name"], task))
    if seen["outcome"] == "refused" and case["attach"] == "ssh":
        raise PreflightError("the ssh node was expected to materialize the checkout branch but the launch was refused: "
                             + scn.r.evidence.scrub(" | ".join(seen["excerpts"])))
    if seen["outcome"] == "ready":
        return _ready_case(scn, step, case, task, commit)
    return _refused_case(scn, step, case, task)


def _save_branch_record(scn: CheckoutScenario, fixture: FixtureBranch) -> None:
    scn.r.save_state(checkout_branch_fixture=fixture.record)
    scn.data["fixture_branch"] = fixture.public()


def _checkout_steps(runner: fx.Runner, cfg: dict[str, Any], root: Path, scn: CheckoutScenario, fixture: FixtureBranch) -> dict[str, Any]:
    step = runner.step
    scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
    step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
    scn.data["nodes"] = step("verify-two-nodes", scn.verify_two_nodes)
    scn.data["roles"] = step("locate-nodes", scn.locate_nodes)
    record = step("create-fixture-branch", fixture.create)
    _save_branch_record(scn, fixture)
    step("start-git-daemon", runner.start_git_daemon)
    served = root / "gitserve" / f"{fx.FIXTURE_REPO_NAME}.git"
    step("verify-branch-served", lambda: scn_assert_served(runner, served, record["commit"]))
    route_checkout_branch(runner, BRANCH)
    step("ssh-disable-local-node", lambda: scn.disable_node("local"))
    step("start-backend", lambda: runner.start_backend(None))
    step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
    scn.known_jobs = scn.kandev_jobs()
    first = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
    scn.cases["ssh"] = _run_case(scn, step, SSH_CASE, first, record["commit"])
    step("ssh-restore", scn.restore_and_verify)
    step("local-disable-ssh-node", lambda: scn.disable_node("ssh"))
    second = step("local-launch", lambda: runner.create_task("Nomad pool checkout branch: local node"))
    scn.cases["local"] = _run_case(scn, step, LOCAL_CASE, second, record["commit"])
    restored = step("local-restore", scn.restore_and_verify)
    step("delete-fixture-branch", fixture.delete)
    _save_branch_record(scn, fixture)
    cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    outcomes = {name: case["outcome"] for name, case in scn.cases.items()}
    production = all(case["path"] == "production-archive-cleanup" for case in scn.cases.values())
    return {"scenario": scn.name, "pool": True, "checkout_branch": True, "cases": scn.cases, "outcomes": outcomes,
            "fixture_branch": fixture.public(), "production_cleanup": production, "release": "none" if production else "operator-teardown",
            "restored": restored, "limitations": CHECKOUT_LIMITATIONS, "cleanup": cleanup, "environments": wo07._env_report(scn)}


def scn_assert_served(runner: fx.Runner, served: Path, commit: str) -> dict[str, str]:
    head = runner.ops.git_head(str(served), f"refs/heads/{BRANCH}")
    if head != commit:
        raise PreflightError("the git daemon repository does not carry the fixture branch commit")
    return {"branch": BRANCH, "commit": head}


def drive_pool_checkout_branch(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    runner.step("check-config", lambda: _check_checkout_config(cfg))
    scn = CheckoutScenario(runner, cfg, CHECKOUT_SCENARIO)
    fixture = FixtureBranch(ops, cfg["repository"]["path"], cfg["repository"]["base_branch"])
    scn.cases = {}
    scn.data["cases"] = scn.cases
    try:
        return _checkout_steps(runner, cfg, root, scn, fixture)
    except BaseException:
        failures = scn.restore_all()
        try:
            fixture.delete()
        except Exception as exc:  # noqa: BLE001 - every restoration is attempted
            failures.append(f"fixture branch: {exc}")
        if failures:
            scn.data["restore_failures"] = failures
        raise


# -- pool-binpack-refused ------------------------------------------------------------------------------
class BinpackScenario(refusals.RefusalScenario):
    """A spread profile against a cluster flipped to binpack: refused at preflight, no job registered."""

    def __init__(self, runner: fx.Runner, cfg: dict[str, Any], name: str):
        super().__init__(runner, cfg, name, None)
        self.scheduler: Any = None
        self.record: dict[str, Any] | None = None

    def check_profile(self) -> dict[str, str]:
        placement = self.cfg["pool"].get("placement", "spread")
        if placement != "spread":
            raise PreflightError(f"pool-binpack-refused requires a spread profile (pool.placement unset or 'spread'), found {placement!r}")
        return {"placement": placement}

    def verify_scheduler(self) -> dict[str, Any]:
        algorithm = self.scheduler.read().get("SchedulerAlgorithm")
        if algorithm != "spread":
            raise PreflightError(f"the Nomad scheduler algorithm is {algorithm!r}, expected 'spread'")
        pool = self.cfg["pool"]["node_pool"]
        record = self.get_or_none(f"/v1/node/pool/{pool}")
        if record is None:
            raise PreflightError(f"the node pool {pool!r} does not exist on the Nomad server")
        override = (record.get("SchedulerConfiguration") or {}).get("SchedulerAlgorithm")
        if override:
            raise PreflightError(f"the node pool {pool!r} overrides the scheduler algorithm ({override!r}), so the cluster setting is not "
                                 "what the preflight reads; remove the pool override first")
        self.record = {"original": algorithm, "flipped_to": "binpack", "applied": False, "restored": False,
                       "restore_with": f"nomad operator scheduler set-config -scheduler-algorithm={algorithm}"}
        self.save_record()
        return {"original": algorithm, "node_pool": pool, "pool_override": None}

    def save_record(self) -> None:
        self.r.save_state(scheduler_restore=self.record)
        self.data["scheduler_restore"] = self.record

    def algorithm(self) -> Any:
        return self.soft(lambda: self.scheduler.read().get("SchedulerAlgorithm"))

    def flip(self) -> dict[str, str]:
        self.record["applied"] = True
        self.save_record()
        self.scheduler.set_algorithm("binpack")
        self.poll(lambda: self.algorithm() == "binpack", "the scheduler algorithm did not become binpack before timeout")
        return {"original": self.record["original"], "flipped": "binpack"}

    def restore_scheduler(self) -> str:
        record = self.record
        if record is None or record["restored"] or not record["applied"]:
            return "not needed"
        self.scheduler.set_algorithm(record["original"])
        self.poll(lambda: self.algorithm() == record["original"], f"the scheduler algorithm did not become {record['original']} again before timeout")
        record["restored"] = True
        self.save_record()
        return "restored"

    def restore_all(self) -> list[str]:
        failures = super().restore_all()
        try:
            note = self.restore_scheduler()
            if note != "not needed":
                self.restored["scheduler algorithm"] = note
        except Exception as exc:  # noqa: BLE001 - every restoration is attempted
            failures.append(f"scheduler algorithm: {exc}; restore with `{self.record['restore_with']}`")
        self.data["restored"] = dict(self.restored)
        return failures

    def restore_and_verify(self) -> dict[str, Any]:
        failures = self.restore_all()
        if failures:
            raise PreflightError(f"restoration failed: {'; '.join(failures)}")
        return {**self.restored, "algorithm": self.algorithm()}

    def registered_jobs(self) -> list[str]:
        return sorted(self.kandev_jobs() - set(self.r.state.get("baseline_jobs") or []))

    def assert_refused(self, task: dict[str, str]) -> dict[str, Any]:
        name, pool = self.name, self.cfg["pool"]["node_pool"]
        seen = self.poll(lambda: self.refusal_probe(task, (BINPACK_CAUSE,)), f"{name}: no refusal naming {BINPACK_CAUSE!r} was reported before timeout")
        text = json.dumps([seen["sessions"], seen["messages"]])
        for secret in self.secret_values():
            if secret in text:
                raise PreflightError(f"{name}: credential or key material appears in the refusal error text")
        excerpt = " | ".join(seen["excerpts"])
        if f'"{pool}"' not in excerpt or "-scheduler-algorithm=spread" not in excerpt:
            raise PreflightError(f"{name}: the refusal does not name the node pool and the spread remedy")
        jobs = self.registered_jobs()
        if jobs:
            raise PreflightError(f"{name}: a Nomad job was registered despite the preflight refusal: {', '.join(jobs)}")
        inventory = self.soft(lambda: self.r.inventory(task["session_id"], task["id"]))
        if inventory and inventory.get("job_id"):
            raise PreflightError(f"{name}: the refused launch recorded job {inventory['job_id']} in the durable inventory")
        return {"task_id": task["id"], "session_id": task["session_id"], "node_pool": pool, "refusal": self.r.evidence.scrub(excerpt),
                "session_states": sorted({str(item.get("state")) for item in seen["sessions"] if isinstance(item, dict)}),
                "ready_reached": False, "jobs_registered": jobs, "inventory_job_recorded": False, "secret_in_error": False}


def _check_binpack_config(cfg: dict[str, Any]) -> None:
    if not cfg.get("pool"):
        raise PreflightError("pool-binpack-refused requires a pool block in the bootstrap config")


def _binpack_steps(runner: fx.Runner, cfg: dict[str, Any], scn: BinpackScenario) -> dict[str, Any]:
    step = runner.step
    scn.data["profile"] = step("check-profile", scn.check_profile)
    scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
    step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
    scn.data["scheduler_before"] = step("verify-scheduler", scn.verify_scheduler)
    step("start-git-daemon", runner.start_git_daemon)
    step("start-backend", lambda: runner.start_backend(None))
    if cfg.get("security"):
        step("provision-scoped-token", lambda: runner.provision_scoped_token(cfg) and None)
    flipped = step("flip-scheduler-to-binpack", scn.flip)
    step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
    task = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
    refusal = step("assert-refusal", lambda: scn.assert_refused(task))
    step("archive-task", lambda: scn.archive(task))
    after = step("assert-no-job-after-archive", scn.registered_jobs)
    if after:
        raise PreflightError(f"a Nomad job appeared after the refused task was archived: {', '.join(after)}")
    restored = step("restore-scheduler", scn.restore_and_verify)
    cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    return {"scenario": scn.name, "pool": True, "binpack_refused": True, "refusal": refusal, "jobs_registered_after_archive": after,
            "scheduler": {"original": flipped["original"], "flipped": flipped["flipped"], "restored": restored["algorithm"]},
            "production_cleanup": True, "release": "none", "limitations": BINPACK_LIMITATIONS, "restored": restored, "cleanup": cleanup,
            "environments": wo07._env_report(scn)}


def drive_pool_binpack_refused(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    runner.step("check-config", lambda: _check_binpack_config(cfg))
    scn = BinpackScenario(runner, cfg, BINPACK_SCENARIO)
    scn.scheduler = make_scheduler(runner)
    try:
        return _binpack_steps(runner, cfg, scn)
    except BaseException:
        failures = scn.restore_all()
        if failures:
            scn.data["restore_failures"] = failures
        raise
