import io
import json
import tempfile
import unittest
import urllib.error
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import nomad_native_e2e_pool_checkout as checkout
from nomad_native_e2e import PreflightError

LOCAL = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
SSH = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
NOMAD_TOKEN = "nomad-runner-token-value"
REAL_KEY = "SHA256:" + "B" * 43
BASE = "1" * 40
TREE = "2" * 40
COMMIT = "3" * 40
OTHER = "4" * 40
REPO = "/fixture/repo"
JOB = "kandev-abc123"
ENV_ID = "11111111-1111-4111-8111-111111111111"
SPREAD_REFUSAL = ('launch failed: pool preflight: placement "spread" needs a spread scheduler for node pool "kandev"; '
                  'set the node pool or cluster scheduler algorithm to spread (`nomad operator scheduler set-config '
                  '-scheduler-algorithm=spread`), or use placement binpack')
LOCAL_REFUSAL = 'launch failed: reconstruct remote workspace repositories: local worktree for "fixture-repo": unsupported option: checkout branch'


class Sim:
    """Offline model of two pool nodes, a scheduler configuration, a fixture repository and the backend's launch outcomes."""

    def __init__(self, tmp, bad=None, **flags):
        self.flags, self.bad = flags, bad or {}
        self.home = Path(tmp) / "home"
        (self.home / "data").mkdir(parents=True)
        self.nodes = {
            LOCAL: self.node(LOCAL, "local", {"kandev.host_identity_sha256": "d" * 64}),
            SSH: self.node(SSH, "ssh", {"kandev.ssh_host": "worker.invalid", "kandev.ssh_host_key_sha256": REAL_KEY}),
        }
        self.launches, self.tasks, self.calls, self.markers, self.jobs = 0, {}, [], set(), set()
        self.archived, self.released, self.leftovers, self.git, self.ctx_seen, self.local_cmds = [], [], [], [], [], []
        self.branch_ref = flags.get("branch_exists", False)
        self.algorithm = flags.get("algorithm", "spread")
        self.pool_override = flags.get("pool_override")
        self.evidence = SimpleNamespace(data={"status": "running", "steps": []}, secrets=[NOMAD_TOKEN],
                                        scrub=lambda text: text.replace(NOMAD_TOKEN, "[redacted]"))
        self.state = {"nomad": {"address": "http://127.0.0.1:4646", "namespace": "kandev", "region": "global", "node_id": SSH},
                      "environments": [], "baseline_jobs": []}

    @staticmethod
    def node(node_id, attach, extra):
        meta = {"kandev.attach": attach, "kandev.state_root": f"/{attach}-root", **extra}
        return {"ID": node_id, "NodePool": "kandev", "Status": "ready", "SchedulingEligibility": "eligible", "Meta": meta}

    # -- Nomad ---------------------------------------------------------------------------------------
    def nomad(self, path):
        if path == "/v1/nodes":
            return [{"ID": node_id} for node_id in self.nodes]
        if path == "/v1/node/pool/kandev":
            return {"Name": "kandev", "SchedulerConfiguration": self.pool_override}
        if path.startswith("/v1/node/"):
            return json.loads(json.dumps(self.nodes[path.rsplit("/", 1)[1]]))
        if path.startswith("/v1/jobs"):
            return [{"ID": job} for job in sorted(self.jobs)]
        if path.startswith("/v1/var/"):
            if any(node_id in path for node_id in self.markers):
                return {"Path": "marker", "Items": {"reason": "x"}}
            raise PreflightError("Nomad GET failed with status 404")
        if path.startswith("/v1/job/") and path.endswith("/allocations"):
            number = int(path.split("/")[3].replace(JOB, ""))
            return [{"ID": "al1", "NodeID": self.tasks[f"task-{number}"]["node"], "ClientStatus": "failed"}]
        raise PreflightError("Nomad GET failed with status 404")

    def set_eligibility(self, address, token, region, node_id, eligible):
        self.calls.append(("eligibility", node_id, eligible))
        if eligible and self.flags.get("enable_fails"):
            raise PreflightError("Nomad eligibility change is unreachable")
        self.nodes[node_id]["SchedulingEligibility"] = "eligible" if eligible else "ineligible"

    # -- backend launch --------------------------------------------------------------------------------
    def eligible(self):
        return {n["Meta"]["kandev.attach"] for n in self.nodes.values() if n["SchedulingEligibility"] == "eligible"}

    def launch(self):
        self.launches += 1
        n = self.launches
        bad = self.bad.get(n)
        info = {"n": n, "bad": bad, "phase": "cleanup_pending", "node": LOCAL, "text": "launched", "job": True, "head": COMMIT}
        if self.algorithm == "binpack":
            info.update(text=SPREAD_REFUSAL, job=bool(self.flags.get("registers")))
        elif self.eligible() == {"ssh"}:
            info.update(node=SSH, phase="ready")
            if self.flags.get("ssh_refused"):
                info.update(phase="cleanup_pending", text=LOCAL_REFUSAL)
        elif self.eligible() == {"local"}:
            if self.flags.get("local_ready"):
                info.update(phase="ready")
            else:
                info.update(text=LOCAL_REFUSAL)
        if bad == "wrong_head":
            info["head"] = OTHER
        if bad == "no_cause":
            info["text"] = "launch failed: something unrelated"
        if bad == "secret":
            info["text"] += f" token={NOMAD_TOKEN}"
        if bad == "ready":
            info["phase"] = "ready"
        if bad == "fallback":
            info["node"] = SSH if info["node"] == LOCAL else LOCAL
        if bad == "wrong_node":
            info["node"] = LOCAL if info["node"] == SSH else SSH
        if info["job"]:
            self.jobs.add(f"{JOB}{n}")
        self.tasks[f"task-{n}"] = info
        return {"id": f"task-{n}", "session_id": f"sess-{n}"}

    def api(self, method, path, body):
        task_id = path.split("/")[2] if path.startswith("/tasks/") else None
        info = self.tasks[task_id] if task_id else next(t for t in self.tasks.values() if t["n"] == int(path.split("sess-")[1].split("/")[0]))
        if path.endswith("/sessions"):
            return {"sessions": [{"id": f"sess-{info['n']}", "state": "FAILED"}]}
        return {"messages": [{"content": info["text"]}]}

    def inventory(self, session_id, task_id):
        info = self.tasks[task_id]
        if not info["job"]:
            raise PreflightError("expected exactly one durable executor inventory row")
        return {"phase": info["phase"], "job_id": f"{JOB}{info['n']}", "allocation_id": "al1", "node_id": info["node"],
                "environment_id": ENV_ID, "generation": 1}

    # -- scheduler and git -------------------------------------------------------------------------------
    def local_exec(self, command):
        self.local_cmds.append(command)
        if "update-ref -d" in command:
            self.branch_ref = False
            self.git.append("delete")
            return (1, "") if self.flags.get("delete_fails") else (0, "")
        if "update-ref refs/heads" in command:
            self.branch_ref = True
            self.git.append("create")
            return 0, ""
        if "--verify --quiet refs/heads/" in command:
            return (0, COMMIT + "\n") if self.branch_ref else (1, "")
        if "commit-tree" in command:
            return 0, COMMIT + "\n"
        if "^{tree}" in command:
            return 0, TREE + "\n"
        raise AssertionError(command)

    def git_head(self, path, ref):
        if ref.startswith("refs/heads/"):
            if self.flags.get("unserved"):
                raise PreflightError("could not resolve the fixture repository base branch commit")
            return COMMIT
        return BASE


class FakeScheduler:
    def __init__(self, sim):
        self.sim = sim

    def read(self):
        return {"SchedulerAlgorithm": self.sim.algorithm, "MemoryOversubscriptionEnabled": False}

    def set_algorithm(self, algorithm):
        self.sim.calls.append(("scheduler", algorithm))
        if algorithm == "binpack" and self.sim.flags.get("put_rejected"):
            raise PreflightError("Nomad rejected the scheduler configuration change with status 403")
        if algorithm == "spread" and self.sim.flags.get("restore_fails"):
            raise PreflightError("Nomad scheduler configuration change is unreachable")
        if not (algorithm == "binpack" and self.sim.flags.get("flip_invisible")):
            self.sim.algorithm = algorithm


def make_runner(sim):
    ops = SimpleNamespace(
        nomad_set_eligibility=sim.set_eligibility, retention_settle=0.0, scenario_timeout=1.0, poll_interval=0.0, clock=lambda: 0.0,
        sleep=lambda s: None, local_exec=sim.local_exec, git_binary=lambda: "/usr/bin/git", git_head=sim.git_head,
        nomad_delete_variable=lambda *a: "deleted")
    runner = SimpleNamespace(home=sim.home, token=NOMAD_TOKEN, state=sim.state, evidence=sim.evidence, api=sim.api, nomad=sim.nomad,
                             inventory=sim.inventory, ops=ops, database="db")
    runner.save_state = lambda **kw: sim.state.update(kw)
    runner.use_node = lambda attach, root: None
    runner.fence = lambda: None
    runner.start_git_daemon = lambda: sim.calls.append(("git-daemon",))
    runner.start_backend = lambda barrier: sim.calls.append(("backend",)) or {"pid": 1}
    runner._task_ctx = {"repositories": [{"repository_id": "r1", "base_branch": "main"}]}

    def create_task(title, description="x"):
        sim.ctx_seen.append([dict(item) for item in runner._task_ctx["repositories"]])
        return sim.launch()

    runner.create_task = create_task

    def setup(cfg):
        task = runner.create_task("fixture")
        sim.state.update(task_id=task["id"], session_id=task["session_id"])

    runner.setup_api_objects = setup

    def poll(timeout, interval, probe, failure):
        for _ in range(3):
            value = probe()
            if value:
                return value
        raise PreflightError(failure)

    runner.poll = poll

    def step(name, fn):
        entry = {"name": name, "status": "running"}
        sim.evidence.data["steps"].append(entry)
        try:
            result = fn()
        except BaseException:
            entry["status"] = "failed"
            raise
        entry["status"] = "passed"
        return result

    runner.step = step
    return runner


def fake_checkout_class(sim):
    class Fake(checkout.CheckoutScenario):
        def verify_node_identity(self):
            self.r.state["pool_state_root"] = "/ssh-root"
            return {"node_id": SSH}

        def kandev_jobs(self):
            return set(sim.jobs)

        def await_ready(self, label, task):
            return {}

        def checkout_head(self, env):
            info = sim.tasks[env["task_id"]]
            kind = "worktree" if env["attach"] == "local" else "checkout"
            return {"kind": kind, "branch": checkout.BRANCH, "head": info["head"], "git_common_dir": "/common", "workspace": "/ws"}

        def assert_pool_cleaned(self, env, layout):
            if sim.flags.get("dirty"):
                raise PreflightError("left a worktree registration")
            return {"kind": layout["kind"], "branch_removed": layout["kind"] == "worktree", "workspace_removed": True}

        def tree(self, env):
            if sim.flags.get("agentctl_on") == env.get("attach"):
                return [{"pid": 9, "role": "agentctl", "depth": 0, "started": "x"}]
            return []

        def archive(self, task):
            sim.archived.append(task["id"])

        def read_state(self, env):
            return {"phase": "cleanup_pending", "generation": 1, "node_attach": env["attach"], "quarantine_state": ""}

        def production_settled(self, env, pending):
            if sim.flags.get("unsettled"):
                pending.append("inventory phase 'cleanup_pending'")
                return None
            return {"phase": "absent"}

        def release(self, label, kill_first, **kw):
            sim.released.append(label)
            return {"label": label, "release": "operator-teardown"}

        def remove_pool_leftovers(self, env, strict):
            sim.leftovers.append(env["label"])
            return {"removed": True}

        def finish(self, report):
            return {**report, "finished": True}

    return Fake


class DriveBase(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)

    def cfg(self, **repo):
        return {"pool": {"node_pool": "kandev", "host_identity_token_file": "/fixture/token"},
                "repository": repo or {"path": REPO, "base_branch": "main"}}


class CheckoutDriveBase(DriveBase):
    def drive(self, cfg=None, **kw):
        self.sim = sim = Sim(self._tmp.name, **kw)
        self.runner = make_runner(sim)
        with mock.patch.object(checkout, "CheckoutScenario", fake_checkout_class(sim)):
            return checkout.drive_pool_checkout_branch(self.runner, cfg or self.cfg(), self.runner.ops, Path(self._tmp.name))

    def assert_restored(self):
        for node in self.sim.nodes.values():
            self.assertEqual(node["SchedulingEligibility"], "eligible")
        self.assertFalse(self.sim.branch_ref)

    def assert_fails(self, message, **kw):
        with self.assertRaises(PreflightError) as ctx:
            self.drive(**kw)
        self.assertIn(message, str(ctx.exception))
        return ctx.exception


class CheckoutHappyPathTest(CheckoutDriveBase):
    def test_ssh_materializes_and_local_is_refused_with_the_documented_error(self):
        report = self.drive()
        self.assert_restored()
        self.assertEqual(report["scenario"], "pool-checkout-branch")
        ssh, local = report["cases"]["ssh"], report["cases"]["local"]
        self.assertEqual(ssh["outcome"], "materialized")
        self.assertEqual(ssh["head"], COMMIT)
        self.assertEqual(ssh["branch"], checkout.BRANCH)
        self.assertEqual(ssh["node_attach"], "ssh")
        self.assertEqual(ssh["cleanup"]["phase"], "absent")
        self.assertEqual(local["outcome"], "refused")
        self.assertIn("unsupported option: checkout branch", local["refusal"]["refusal"])
        self.assertEqual(local["refusal"]["node_attach"], "local")
        self.assertFalse(local["refusal"]["ready_reached"])
        self.assertEqual(report["outcomes"], {"ssh": "materialized", "local": "refused"})
        self.assertEqual(report["fixture_branch"]["commit"], COMMIT)
        self.assertNotEqual(report["fixture_branch"]["commit"], report["fixture_branch"]["base_commit"])
        self.assertTrue(report["fixture_branch"]["deleted"])
        self.assertTrue(report["cleanup"]["finished"])

    def test_every_task_carries_the_checkout_branch(self):
        self.drive()
        self.assertEqual(len(self.sim.ctx_seen), 2)
        for repos in self.sim.ctx_seen:
            self.assertEqual(repos[0]["checkout_branch"], checkout.BRANCH)
            self.assertEqual(repos[0]["base_branch"], "main")

    def test_fault_order_branch_and_daemon_precede_tasks_and_ssh_case_disables_local_first(self):
        self.drive()
        calls = self.sim.calls
        self.assertEqual(self.sim.git[0], "create")
        self.assertLess(calls.index(("git-daemon",)), calls.index(("eligibility", LOCAL, False)))
        self.assertLess(calls.index(("eligibility", LOCAL, False)), calls.index(("backend",)))
        self.assertLess(calls.index(("git-daemon",)), calls.index(("backend",)))
        self.assertLess(calls.index(("eligibility", LOCAL, True)), calls.index(("eligibility", SSH, False)))
        self.assertEqual(calls.count(("eligibility", SSH, True)), 1)

    def test_ssh_case_ends_through_production_archive_cleanup(self):
        report = self.drive()
        self.assertEqual(self.sim.archived[0], "task-1")
        self.assertEqual(report["cases"]["ssh"]["cleanup"]["phase"], "absent")
        self.assertEqual(self.sim.released.count("C1"), 0)

    def test_local_worktree_that_carries_the_same_commit_is_the_other_documented_outcome(self):
        report = self.drive(local_ready=True)
        self.assert_restored()
        local = report["cases"]["local"]
        self.assertEqual(local["outcome"], "materialized")
        self.assertEqual(local["head"], COMMIT)
        self.assertEqual(report["outcomes"], {"ssh": "materialized", "local": "materialized"})

    def test_evidence_is_scrubbed_of_the_runner_token(self):
        self.drive()
        self.assertNotIn(NOMAD_TOKEN, json.dumps(self.sim.evidence.data))

    def test_restore_record_is_saved_for_the_operator(self):
        self.drive()
        record = self.sim.state["checkout_branch_fixture"]
        self.assertEqual(record["branch"], checkout.BRANCH)
        self.assertTrue(record["deleted"])
        self.assertIn(f"update-ref -d refs/heads/{checkout.BRANCH}", record["restore_with"])


class CheckoutFailureTest(CheckoutDriveBase):
    def test_ssh_head_other_than_the_branch_commit_fails_and_restores(self):
        self.assert_fails("does not carry the fixture branch commit", bad={1: "wrong_head"})
        self.assert_restored()

    def test_ssh_refusal_fails_because_ssh_must_materialize(self):
        self.assert_fails("ssh node was expected to materialize", ssh_refused=True)
        self.assert_restored()

    def test_ssh_case_placed_on_the_local_node_fails(self):
        self.assert_fails("expected a ssh node", bad={1: "wrong_node"})
        self.assert_restored()

    def test_local_worktree_with_a_different_commit_fails(self):
        self.assert_fails("does not carry the fixture branch commit", local_ready=True, bad={2: "wrong_head"})
        self.assert_restored()

    def test_local_error_without_the_documented_text_fails(self):
        self.assert_fails("no refusal naming 'unsupported option: checkout branch'", bad={2: "no_cause"})
        self.assert_restored()

    def test_local_credential_in_the_error_fails_without_echoing_it(self):
        error = self.assert_fails("credential or key material appears", bad={2: "secret"})
        self.assertNotIn(NOMAD_TOKEN, str(error))
        self.assert_restored()

    def test_local_agentctl_after_refusal_fails(self):
        self.assert_fails("agentctl is running on the local node", agentctl_on="local")
        self.assert_restored()

    def test_local_fallback_to_the_ssh_node_fails(self):
        self.assert_fails("other than the local node", bad={2: "fallback"})
        self.assert_restored()

    def test_unsettled_production_cleanup_of_the_ssh_case_fails_without_operator_fallback(self):
        self.assert_fails("production cleanup of C1 did not settle", unsettled=True)
        self.assertEqual(self.sim.released, [])
        self.assert_restored()

    def test_leftovers_after_cleanup_fail(self):
        self.assert_fails("left a worktree registration", dirty=True)

    def test_branch_not_served_by_the_git_daemon_fails_before_any_task(self):
        self.assert_fails("could not resolve", unserved=True)
        self.assertEqual(self.sim.launches, 0)
        self.assert_restored()

    def test_existing_branch_is_refused_and_never_deleted(self):
        self.assert_fails("already exists in the fixture repository", branch_exists=True)
        self.assertTrue(self.sim.branch_ref)
        self.assertEqual(self.sim.git, [])
        self.assertEqual(self.sim.launches, 0)

    def test_remote_repository_is_refused_before_anything_changes(self):
        with self.assertRaises(PreflightError) as ctx:
            self.drive(cfg=self.cfg(url="https://example.invalid/r.git", base_branch="main"))
        self.assertIn("repository.path", str(ctx.exception))
        self.assertEqual(self.sim.git, [])
        self.assertEqual(self.sim.calls, [])

    def test_branch_delete_failure_on_the_success_path_fails_the_run(self):
        self.assert_fails("fixture repository branch could not be deleted", delete_fails=True)

    def test_failed_assertion_keeps_its_error_when_restoration_also_fails(self):
        with self.assertRaises(PreflightError) as ctx:
            self.drive(bad={1: "wrong_head"}, enable_fails=True, delete_fails=True)
        self.assertIn("does not carry the fixture branch commit", str(ctx.exception))
        failures = self.sim.evidence.data["restore_failures"]
        self.assertTrue(any("eligibility" in item for item in failures))
        self.assertTrue(any("fixture branch" in item for item in failures))


class FixtureBranchTest(unittest.TestCase):
    def make(self, **flags):
        sim = Sim(tempfile.mkdtemp(), **flags)
        return sim, checkout.FixtureBranch(make_runner(sim).ops, REPO, "main")

    def test_create_builds_a_distinct_commit_without_touching_head_or_worktree(self):
        sim, fixture = self.make()
        record = fixture.create()
        self.assertEqual((record["base_commit"], record["commit"], record["branch"]), (BASE, COMMIT, checkout.BRANCH))
        joined = "\n".join(sim.local_cmds)
        for forbidden in (" checkout ", " reset ", " switch ", " branch ", " worktree "):
            self.assertNotIn(forbidden, joined)
        self.assertIn("commit-tree", joined)
        self.assertTrue(sim.branch_ref)

    def test_delete_is_idempotent_and_noop_when_never_created(self):
        sim, fixture = self.make()
        self.assertEqual(fixture.delete(), "not needed")
        fixture.create()
        self.assertEqual(fixture.delete(), "deleted")
        self.assertEqual(fixture.delete(), "not needed")
        self.assertEqual(sim.git, ["create", "delete"])

    def test_delete_failure_names_the_operator_command(self):
        sim, fixture = self.make(delete_fails=True)
        fixture.create()
        with self.assertRaises(PreflightError) as ctx:
            fixture.delete()
        self.assertIn(f"update-ref -d refs/heads/{checkout.BRANCH}", str(ctx.exception))

    def test_a_commit_equal_to_the_base_is_rejected(self):
        sim, fixture = self.make()
        sim.local_exec = lambda command: (0, BASE + "\n") if "commit-tree" in command else (0, TREE + "\n") if "^{tree}" in command else (1, "")
        fixture.ops.local_exec = sim.local_exec
        with self.assertRaises(PreflightError):
            fixture.create()


# -- pool-binpack-refused ---------------------------------------------------------------------------------
def fake_binpack_class(sim):
    class Fake(checkout.BinpackScenario):
        def verify_node_identity(self):
            self.r.state["pool_state_root"] = "/ssh-root"
            return {"node_id": SSH}

        def kandev_jobs(self):
            return set(sim.jobs)

        def archive(self, task):
            sim.archived.append(task["id"])
            sim.calls.append(("archive", task["id"]))

        def finish(self, report):
            return {**report, "finished": True}

    return Fake


class BinpackDriveBase(DriveBase):
    def drive(self, cfg=None, **kw):
        self.sim = sim = Sim(self._tmp.name, **kw)
        self.runner = make_runner(sim)
        with mock.patch.object(checkout, "BinpackScenario", fake_binpack_class(sim)), \
                mock.patch.object(checkout, "make_scheduler", lambda runner: FakeScheduler(sim)):
            return checkout.drive_pool_binpack_refused(self.runner, cfg or self.cfg(), self.runner.ops, Path(self._tmp.name))

    def assert_fails(self, message, restored=True, **kw):
        with self.assertRaises(PreflightError) as ctx:
            self.drive(**kw)
        self.assertIn(message, str(ctx.exception))
        if restored:
            self.assertEqual(self.sim.algorithm, "spread")
        return ctx.exception


class BinpackHappyPathTest(BinpackDriveBase):
    def test_spread_profile_on_a_binpack_cluster_is_refused_with_no_job_and_the_scheduler_is_restored(self):
        report = self.drive()
        self.assertEqual(self.sim.algorithm, "spread")
        self.assertEqual(report["scenario"], "pool-binpack-refused")
        self.assertEqual(report["scheduler"], {"original": "spread", "flipped": "binpack", "restored": "spread"})
        refusal = report["refusal"]
        self.assertIn('needs a spread scheduler for node pool "kandev"', refusal["refusal"])
        self.assertIn("scheduler-algorithm=spread", refusal["refusal"])
        self.assertFalse(refusal["ready_reached"])
        self.assertEqual(refusal["jobs_registered"], [])
        self.assertFalse(refusal["inventory_job_recorded"])
        self.assertEqual(report["jobs_registered_after_archive"], [])
        self.assertEqual(self.sim.archived, ["task-1"])
        self.assertTrue(report["cleanup"]["finished"])
        self.assertTrue(report["production_cleanup"])
        self.assertEqual(report["release"], "none")

    def test_flip_happens_after_the_backend_starts_and_before_the_first_task(self):
        self.drive()
        calls = self.sim.calls
        self.assertLess(calls.index(("backend",)), calls.index(("scheduler", "binpack")))
        self.assertLess(calls.index(("scheduler", "binpack")), calls.index(("archive", "task-1")))
        self.assertEqual(calls.count(("scheduler", "spread")), 1)
        self.assertEqual(self.sim.launches, 1)

    def test_restore_record_is_saved_before_the_flip(self):
        self.sim_state_at_flip = None
        sim_holder = {}
        original = FakeScheduler.set_algorithm

        def spy(scheduler, algorithm):
            if algorithm == "binpack":
                sim_holder["record"] = json.loads(json.dumps(scheduler.sim.state.get("scheduler_restore")))
            return original(scheduler, algorithm)

        with mock.patch.object(FakeScheduler, "set_algorithm", spy):
            self.drive()
        record = sim_holder["record"]
        self.assertEqual(record["original"], "spread")
        self.assertFalse(record["restored"])
        self.assertIn("-scheduler-algorithm=spread", record["restore_with"])
        self.assertTrue(self.sim.state["scheduler_restore"]["restored"])

    def test_evidence_is_scrubbed(self):
        self.drive()
        self.assertNotIn(NOMAD_TOKEN, json.dumps(self.sim.evidence.data))


class BinpackFailureTest(BinpackDriveBase):
    def test_a_registered_job_fails_and_restores(self):
        self.assert_fails("a Nomad job was registered", registers=True)

    def test_error_without_the_clear_message_fails_and_restores(self):
        self.assert_fails("no refusal naming 'needs a spread scheduler for node pool'", bad={1: "no_cause"})

    def test_task_reaching_ready_fails_and_restores(self):
        self.assert_fails("reached inventory phase ready", bad={1: "ready"}, registers=True)

    def test_credential_in_the_error_fails_without_echoing_it(self):
        error = self.assert_fails("credential or key material appears", bad={1: "secret"})
        self.assertNotIn(NOMAD_TOKEN, str(error))

    def test_rejected_flip_fails_with_nothing_to_restore(self):
        self.assert_fails("Nomad rejected the scheduler configuration change", put_rejected=True)
        self.assertEqual(self.sim.launches, 0)

    def test_flip_that_is_not_visible_fails_and_restores(self):
        self.assert_fails("scheduler algorithm did not become binpack", flip_invisible=True)
        self.assertEqual(self.sim.launches, 0)

    def test_original_algorithm_other_than_spread_is_refused_before_any_change(self):
        self.assert_fails("expected 'spread'", restored=False, algorithm="binpack")
        self.assertNotIn(("scheduler", "binpack"), self.sim.calls)
        self.assertEqual(self.sim.algorithm, "binpack")

    def test_node_pool_scheduler_override_is_refused_before_any_change(self):
        self.assert_fails("overrides the scheduler algorithm", pool_override={"SchedulerAlgorithm": "spread"})
        self.assertEqual([c for c in self.sim.calls if c[0] == "scheduler"], [])

    def test_binpack_profile_is_refused_before_any_change(self):
        cfg = self.cfg()
        cfg["pool"]["placement"] = "binpack"
        with self.assertRaises(PreflightError) as ctx:
            self.drive(cfg=cfg)
        self.assertIn("spread", str(ctx.exception))
        self.assertEqual(self.sim.calls, [])

    def test_restore_failure_on_the_success_path_fails_the_run(self):
        self.assert_fails("restoration failed", restored=False, restore_fails=True)
        self.assertEqual(self.sim.evidence.data["steps"][-1]["status"], "failed")
        self.assertEqual(self.sim.algorithm, "binpack")
        self.assertIn("-scheduler-algorithm=spread", self.sim.state["scheduler_restore"]["restore_with"])

    def test_failed_assertion_keeps_its_error_when_restoration_also_fails(self):
        with self.assertRaises(PreflightError) as ctx:
            self.drive(registers=True, restore_fails=True)
        self.assertIn("a Nomad job was registered", str(ctx.exception))
        self.assertTrue(any("scheduler" in item for item in self.sim.evidence.data["restore_failures"]))


class SchedulerConfigTest(unittest.TestCase):
    class Opener:
        def __init__(self, get=None, put=b'{"Updated": true, "Index": 9}', error=None):
            self.requests, self.get, self.put, self.error = [], get, put, error

        def open(self, request, timeout=None):
            self.requests.append(request)
            if self.error:
                raise self.error
            return io.BytesIO(self.get if request.get_method() == "GET" else self.put)

    CURRENT = json.dumps({"SchedulerConfig": {"SchedulerAlgorithm": "spread", "MemoryOversubscriptionEnabled": False, "CreateIndex": 5, "ModifyIndex": 7,
                                              "PreemptionConfig": {"BatchSchedulerEnabled": False}, "RejectJobRegistration": False,
                                              "PauseEvalBroker": False}, "Index": 7}).encode()

    def make(self, opener):
        return checkout.SchedulerConfig("https://127.0.0.1:4646", NOMAD_TOKEN, "global", opener)

    def test_read_returns_the_scheduler_config_with_the_runner_token(self):
        opener = self.Opener(get=self.CURRENT)
        self.assertEqual(self.make(opener).read()["SchedulerAlgorithm"], "spread")
        self.assertEqual(opener.requests[0].get_header("X-nomad-token"), NOMAD_TOKEN)
        self.assertIn("/v1/operator/scheduler/configuration", opener.requests[0].full_url)

    def test_set_algorithm_puts_the_whole_config_with_only_the_algorithm_changed(self):
        opener = self.Opener(get=self.CURRENT)
        self.make(opener).set_algorithm("binpack")
        put = opener.requests[1]
        self.assertEqual(put.get_method(), "PUT")
        body = json.loads(put.data)
        self.assertEqual(body["SchedulerAlgorithm"], "binpack")
        self.assertEqual(body["PreemptionConfig"], {"BatchSchedulerEnabled": False})
        self.assertIs(body["MemoryOversubscriptionEnabled"], False)
        self.assertNotIn("CreateIndex", body)
        self.assertNotIn("ModifyIndex", body)
        self.assertEqual(put.get_header("X-nomad-token"), NOMAD_TOKEN)

    def test_unknown_algorithm_is_refused_without_a_request(self):
        opener = self.Opener(get=self.CURRENT)
        with self.assertRaises(PreflightError):
            self.make(opener).set_algorithm("random")
        self.assertEqual(opener.requests, [])

    def test_rejection_names_the_status_and_never_the_token(self):
        error = urllib.error.HTTPError("u", 403, "forbidden", {}, io.BytesIO(NOMAD_TOKEN.encode()))
        with self.assertRaises(PreflightError) as ctx:
            self.make(self.Opener(error=error)).read()
        self.assertIn("403", str(ctx.exception))
        self.assertNotIn(NOMAD_TOKEN, str(ctx.exception))

    def test_unreachable_malformed_and_not_updated_responses_fail(self):
        with self.assertRaises(PreflightError):
            self.make(self.Opener(error=urllib.error.URLError("down"))).read()
        with self.assertRaises(PreflightError):
            self.make(self.Opener(get=b"not json")).read()
        with self.assertRaises(PreflightError):
            self.make(self.Opener(get=b"{}")).read()
        with self.assertRaises(PreflightError):
            self.make(self.Opener(get=self.CURRENT, put=b'{"Updated": false}')).set_algorithm("binpack")


if __name__ == "__main__":
    unittest.main()
