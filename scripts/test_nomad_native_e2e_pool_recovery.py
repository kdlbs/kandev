import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import nomad_native_e2e_fixture as fx
import nomad_native_e2e_pool_recovery as rec
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError

LOCAL = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
SSH = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
JOB = "kandev-abc123"
DIGEST = "d" * 64
SPEC = "e" * 64
OPERATOR_OPS = wo07.OPERATOR_OPS


class Sim:
    """Offline model of one retained pool instance, the backend process and the tunnel, with fault flags."""

    def __init__(self, **flags):
        self.flags = flags
        self.backend_up, self.backend_pids, self.stops, self.crashes = False, 0, 0, 0
        self.barriers = []
        self.attach = "local" if flags.get("landed_local") else "ssh"
        self.eligibility = {LOCAL: "eligible", SSH: "eligible"}
        self.has_local = not flags.get("no_local_node")
        self.calls, self.archived, self.finished = [], 0, False
        self.jobs = {JOB}
        self.inst = {"job_id": JOB, "job_create_index": 5, "job_definition_index": 6, "allocation_id": "al1", "execution_id": "exec-1",
                     "agentctl": (100, "t0"), "fence": "fence-1", "input_digest": DIGEST, "spec": SPEC, "alive": True}
        self.tunnel_up, self.tunnel_calls, self.outage_ticks = False, 0, 0
        self.metadata = {"nomad_environment_state": {}}
        if not flags.get("native_no_tunnel"):
            self.metadata["ssh_runtime_api_local_url"] = "http://127.0.0.1:8080"
            if not flags.get("no_tunnel_port"):
                self.metadata["ssh_runtime_api_remote_port"] = "40123"
        self.state = {"port": 9999, "task_id": "t1", "session_id": "s1", "expected_repo_count": 1, "manifest_env": {"A": "b"},
                      "nomad": {"address": "https://127.0.0.1:4646", "namespace": "kandev", "region": "global", "node_id": SSH}}
        self.evidence = SimpleNamespace(data={"status": "running", "steps": []}, secrets=[], scrub=lambda text: text)

    # -- recovery effect of one backend restart -------------------------------------------------------------
    def apply_restart_effects(self):
        f, i = self.flags, self.inst
        if f.get("recreate_job"):
            i.update(job_id="kandev-new999", job_create_index=50, allocation_id="al2")
            self.jobs.add("kandev-new999")
        if f.get("agent_restarted"):
            i["agentctl"] = (200, "t9")
        if f.get("digest_changed"):
            i["input_digest"] = "f" * 64
        if f.get("execution_changed"):
            i["execution_id"] = "exec-2"
        if f.get("fence_changed"):
            i["fence"] = "fence-2"

    # -- backend -------------------------------------------------------------------------------------------
    def start_backend(self, barrier):
        self.backend_up, self.backend_pids = True, self.backend_pids + 1
        self.barriers.append(barrier)
        if self.backend_pids > 1 or barrier is None:
            self.tunnel_up = not self.flags.get("tunnel_stays_down")
            if self.backend_pids > 1:
                self.apply_restart_effects()
        return {"pid": 1000 + self.backend_pids}

    def stop_like(self, kind):
        self.backend_up = False
        if kind == "stop":
            self.stops += 1
        else:
            self.crashes += 1
        if not self.flags.get("tunnel_never_down"):
            self.tunnel_up = False
        return "stopped" if kind == "stop" else "crashed"

    def nomad(self, path):
        if path == "/v1/nodes":
            ids = [SSH] + ([LOCAL] if self.has_local else [])
            return [{"ID": n} for n in ids]
        if path.startswith("/v1/node/"):
            node_id = path.rsplit("/", 1)[1]
            attach = "local" if node_id == LOCAL else "ssh"
            meta = {"kandev.attach": attach, "kandev.state_root": f"/{attach}-root"}
            meta.update({"kandev.host_identity_sha256": "h" * 64} if attach == "local" else {"kandev.ssh_host": "worker.invalid", "kandev.ssh_host_key_sha256": "SHA256:k"})
            return {"ID": node_id, "NodePool": "kandev", "Status": "ready", "SchedulingEligibility": self.eligibility[node_id], "Meta": meta}
        raise PreflightError("Nomad GET failed with status 404")

    def set_eligibility(self, address, token, region, node_id, eligible):
        self.calls.append(("eligibility", node_id, eligible))
        self.eligibility[node_id] = "eligible" if eligible else "ineligible"

    def inventory(self, session_id=None, task_id=None):
        i = self.inst
        return {"environment_id": "env-1", "generation": 1, "operation_id": i["execution_id"], "execution_id": i["execution_id"], "job_id": i["job_id"],
                "job_create_index": i["job_create_index"], "job_definition_index": i["job_definition_index"], "allocation_id": i["allocation_id"],
                "node_id": SSH if self.attach == "ssh" else LOCAL, "workspace_path": "/ws", "server_identity": "srv", "phase": "ready",
                "workspace_input_captured": True}

    def nomad_snapshot(self, inventory):
        i = self.inst
        if self.flags.get("job_dead") and not self.backend_up:
            raise PreflightError("Nomad job is not the running job from the inventory")
        return {"job_id": i["job_id"], "job_create_index": i["job_create_index"], "job_definition_index": i["job_definition_index"],
                "allocation_id": i["allocation_id"], "node_id": inventory["node_id"], "allocation_count": 1}

    def ssh(self, command):
        if "curl" in command:
            self.tunnel_calls += 1
            return (0, "200") if self.tunnel_up and not self.flags.get("tunnel_bad_status") else ((0, "502") if self.tunnel_up else (7, ""))
        if "KANDEV_ACP_IDLE_TIMEOUT" in command:
            value = self.flags.get("idle_env", "0")
            return (0, f"{value}\n") if value is not None else (1, "")
        if "/proc/" in command:
            raise AssertionError("the runner must not read /proc/<pid>/environ on the ssh node for the tunnel")
        return 0, ""

    def tree(self):
        if not self.inst["alive"]:
            return []
        pid, started = self.inst["agentctl"]
        return [{"pid": pid, "started": started, "depth": 0, "role": "agentctl"}, {"pid": pid + 1, "started": "c", "depth": 1, "role": "child"}]

    def read_db(self, database, fn):
        meta = json.dumps(self.metadata)
        db = SimpleNamespace(execute=lambda sql, params=(): SimpleNamespace(fetchall=lambda: [(meta,)]))
        return fn(db)


def make_runner(sim):
    ticks = iter(range(10 ** 7))
    ops = SimpleNamespace(nomad_set_eligibility=sim.set_eligibility, clock=lambda: float(next(ticks)), sleep=lambda s: None, scenario_timeout=60.0,
                          poll_interval=0.0, port_open=lambda port: sim.backend_up, read_db=sim.read_db, recovery_timeout=60.0,
                          prompt=lambda payload: {"accepted": True}, retention_settle=0.0, wall=lambda: "now", **{n: (lambda *a, **k: None) for n in OPERATOR_OPS if n != "nomad_set_eligibility"})
    runner = SimpleNamespace(state=sim.state, evidence=sim.evidence, ops=ops, token="management-token", home=Path("/nonexistent-home"), database="db",
                             root=Path("/nonexistent-root"), nomad=sim.nomad, inventory=sim.inventory, nomad_snapshot=sim.nomad_snapshot, ssh=sim.ssh)
    runner.save_state = lambda **kw: sim.state.update(kw)
    runner.use_node = lambda attach, root: None
    runner.fence = lambda environment_id=None: sim.inst["fence"]
    runner.start_git_daemon = lambda: None
    runner.start_backend = sim.start_backend
    runner.setup_api_objects = lambda cfg: None
    runner.create_task = lambda title: {"id": "t2", "session_id": "s2"}
    runner.verify_gone = lambda pid: (_ for _ in ()).throw(PreflightError("fixture backend process is still alive")) if sim.backend_up else None
    runner.registry = SimpleNamespace(
        read_records=lambda: [{"label": fx.PROCESS_LABEL, "pid": 1000 + sim.backend_pids}] if sim.backend_up else [],
        stop=lambda label, timeout: sim.stop_like("stop") if not sim.flags.get("stop_fails") else "unverified",
        crash=lambda label, timeout: sim.stop_like("crash"))
    runner.await_barrier = lambda marker: sim.inventory()
    runner.check_environment_creating = lambda environment_id: 1
    runner.check_session_created = lambda: [{"id": "m1"}]
    runner.session_view = lambda: ({"id": "s1"}, [{"id": "m1"}])

    def poll(timeout, interval, probe, failure):
        deadline = ops.clock() + timeout
        while True:
            value = probe()
            if value:
                return value
            if ops.clock() >= deadline:
                raise PreflightError(failure)
            ops.sleep(interval)

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


def fake_scenario_class(sim):
    class Fake(rec.RecoveryScenario):
        def verify_node_identity(self):
            self.r.state["pool_state_root"] = "/ssh-root"
            return {"node_id": SSH}

        def node_ready(self):
            return True

        def kandev_jobs(self):
            return set(sim.jobs)

        def tree(self, env):
            return sim.tree()

        def read_state(self, env):
            i = sim.inst
            return {"input_digest": None if sim.flags.get("no_digest") else i["input_digest"], "native_job_spec_sha256": i["spec"], "phase": "ready"}

        def await_placed(self, label, task):
            inv = sim.inventory()
            self.record_env(label, task, inv, attach=sim.attach, state_root=f"/{sim.attach}-root")
            return {"inventory": inv}

        def start_managed_tree(self, env, task, snapshot):
            snapshot["tree"] = sim.tree()
            return snapshot["tree"]

        def workspace_layout(self, env):
            return {"kind": "checkout", "branch": "main", "git_common_dir": "/x", "workspace": "/ws"}

        def assert_pool_cleaned(self, env, layout):
            return {"kind": layout["kind"], "workspace_removed": True}

        def archive(self, task):
            sim.archived += 1

        def production_settled(self, env, pending):
            if self.guard:
                self.guard.assert_clean()
            if sim.flags.get("cleanup_stuck"):
                pending.append("inventory phase 'ready'")
                return None
            return {"phase": "absent"}

        def finish(self, report):
            sim.finished = True
            return {**report, "finished": True}

    return Fake


class DriveBase(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        patcher = mock.patch.object(rec, "OUTAGE_SECONDS", 6)
        patcher.start()
        self.addCleanup(patcher.stop)
        patcher = mock.patch.object(rec, "IDLE_TIMEOUT_SECONDS", 2)
        patcher.start()
        self.addCleanup(patcher.stop)

    def drive(self, fn, **flags):
        self.sim = sim = Sim(**flags)
        self.runner = make_runner(sim)
        cfg = {"pool": {"node_pool": "kandev"}, "cpu_mhz": 100, "memory_mb": 100}
        await_recovered = flags.get("await_recovered") or (lambda *a, **k: sim.inventory())
        with mock.patch.object(rec, "RecoveryScenario", fake_scenario_class(sim)), \
                mock.patch.object(rec.base, "_await_recovered_environment", await_recovered), \
                mock.patch.object(rec.base, "_check_session_untouched", lambda *a, **k: None):
            return fn(self.runner, cfg, self.runner.ops, Path(self._tmp.name))

    def assert_fails(self, fn, message, **flags):
        with self.assertRaises(PreflightError) as ctx:
            self.drive(fn, **flags)
        self.assertIn(message, str(ctx.exception))
        self.assertFalse(self.sim.finished, "a failed run must keep the fixture, not tear it down")
        return ctx.exception

    def steps(self):
        return [s["name"] for s in self.sim.evidence.data["steps"]]


# -- pure helpers --------------------------------------------------------------------------------------------
class HelperTest(unittest.TestCase):
    def test_find_key_searches_nested_objects_and_lists(self):
        self.assertEqual(rec.find_key({"a": [{"b": {"k": "7"}}]}, "k"), "7")
        self.assertIsNone(rec.find_key({"a": 1}, "k"))

    def test_valid_port(self):
        self.assertEqual(rec.valid_port("40123"), 40123)
        for bad in (None, "", "0", "70000", "x", "1.5"):
            with self.assertRaises(PreflightError):
                rec.valid_port(bad)

    def test_compare_names_what_changed(self):
        base = {"inventory": {"a": 1}, "nomad": {"b": 1}, "tree": [(1, "t")], "fence": "f", "digests": {"input_digest": DIGEST}, "jobs": ["j"]}
        rec.compare_instance(base, json.loads(json.dumps(base)) | {"tree": [(1, "t")]}, "x")
        cases = {"jobs": (["j", "k"], "re-created"), "nomad": ({"b": 2}, "re-created"), "tree": ([(2, "t")], "process tree"),
                 "fence": ("g", "fence"), "digests": ({"input_digest": "0"}, "continuation digest"), "inventory": ({"a": 2}, "execution")}
        for key, (value, text) in cases.items():
            with self.assertRaises(PreflightError) as ctx:
                rec.compare_instance(base, {**base, key: value}, "recovery")
            self.assertIn(text, str(ctx.exception))

    def test_digest_state_requires_the_input_digest(self):
        self.assertEqual(rec.digests_from_state({"input_digest": DIGEST, "native_job_spec_sha256": SPEC, "x": 1}),
                         {"input_digest": DIGEST, "native_job_spec_sha256": SPEC})
        for bad in ({}, {"input_digest": "short"}, {"input_digest": None}):
            with self.assertRaises(PreflightError):
                rec.digests_from_state(bad)

    def test_outage_window_must_exceed_the_idle_timeout(self):
        with mock.patch.object(rec, "OUTAGE_SECONDS", 60), mock.patch.object(rec, "IDLE_TIMEOUT_SECONDS", 60):
            with self.assertRaises(PreflightError):
                rec.check_outage_window()
        with mock.patch.object(rec, "OUTAGE_SECONDS", 61), mock.patch.object(rec, "IDLE_TIMEOUT_SECONDS", 60):
            rec.check_outage_window()

    def test_registered_scenarios_resolve_to_these_functions(self):
        import nomad_native_e2e_pool as pool
        for name, fn in (("pool-pre-ready-recovery", "drive_pool_pre_ready_recovery"), ("pool-backend-restart", "drive_pool_backend_restart"),
                         ("pool-idle-outage", "drive_pool_idle_outage")):
            self.assertEqual(pool.COVERAGE_DRIVERS[name], ("nomad_native_e2e_pool_recovery", fn))
            self.assertTrue(callable(getattr(rec, fn)))


# -- pool-pre-ready-recovery ---------------------------------------------------------------------------------
class PreReadyTest(DriveBase):
    def run_it(self, **flags):
        return self.drive(rec.drive_pool_pre_ready_recovery, **flags)

    def test_happy_path_asserts_ready_same_execution_and_digest_equality(self):
        report = self.run_it()
        self.assertEqual(report["scenario"], "pool-pre-ready-recovery")
        self.assertEqual(report["execution_id"], "exec-1")
        self.assertEqual(report["continuation_digest"], {"before": DIGEST, "after": DIGEST, "equal": True})
        self.assertEqual(report["environment_status"], {"before": "creating", "after": "ready"})
        self.assertEqual(report["backend_pids"], {"before_crash": 1001, "after_restart": 1002})
        self.assertEqual((self.sim.crashes, self.sim.stops), (1, 0))
        self.assertTrue(report["production_cleanup"])
        self.assertFalse(report["operator_cleanup_used"])
        self.assertEqual(self.sim.archived, 1)
        self.assertTrue(self.sim.finished)
        self.assertEqual([b is not None for b in self.sim.barriers], [True, False])

    def test_first_backend_runs_with_the_barrier_and_the_second_without(self):
        self.run_it()
        self.assertIn("crash-backend", self.steps())
        self.assertLess(self.steps().index("crash-backend"), self.steps().index("restart-backend-without-barrier"))

    def test_recreated_job_fails(self):
        self.assert_fails(rec.drive_pool_pre_ready_recovery, "re-created", recreate_job=True)

    def test_changed_execution_fails(self):
        self.assert_fails(rec.drive_pool_pre_ready_recovery, "execution", execution_changed=True)

    def test_changed_digest_fails(self):
        self.assert_fails(rec.drive_pool_pre_ready_recovery, "continuation digest", digest_changed=True)

    def test_missing_digest_refuses_before_the_crash(self):
        self.assert_fails(rec.drive_pool_pre_ready_recovery, "continuation input digest", no_digest=True)
        self.assertEqual(self.sim.crashes, 0)

    def test_changed_fence_fails(self):
        self.assert_fails(rec.drive_pool_pre_ready_recovery, "fence", fence_changed=True)

    def test_restarted_agentctl_fails(self):
        self.assert_fails(rec.drive_pool_pre_ready_recovery, "process tree", agent_restarted=True)

    def test_empty_process_tree_refuses_before_the_crash(self):
        self.sim = sim = Sim()
        sim.inst["alive"] = False
        self.runner = make_runner(sim)
        with mock.patch.object(rec, "RecoveryScenario", fake_scenario_class(sim)):
            with self.assertRaises(PreflightError) as ctx:
                rec.drive_pool_pre_ready_recovery(self.runner, {"pool": {"node_pool": "kandev"}}, self.runner.ops, Path(self._tmp.name))
        self.assertIn("agentctl", str(ctx.exception))
        self.assertEqual(sim.crashes, 0)

    def test_recovery_that_never_becomes_ready_fails_and_keeps_the_fixture(self):
        def never(*a, **k):
            raise PreflightError("recovered environment did not become ready before timeout")

        self.assert_fails(rec.drive_pool_pre_ready_recovery, "did not become ready", await_recovered=never)

    def test_stuck_cleanup_fails_naming_the_pending_state(self):
        self.assert_fails(rec.drive_pool_pre_ready_recovery, "production cleanup did not settle", cleanup_stuck=True)

    def test_operator_cleanup_during_production_cleanup_fails(self):
        sim = Sim()
        runner = make_runner(sim)
        base_class = fake_scenario_class(sim)

        class Armed(base_class):
            def archive(self, task):
                super().archive(task)
                runner.ops.nomad_stop("a", "t", "n", "job")

        with mock.patch.object(rec, "RecoveryScenario", Armed), mock.patch.object(rec.base, "_await_recovered_environment", lambda *a, **k: sim.inventory()), \
                mock.patch.object(rec.base, "_check_session_untouched", lambda *a, **k: None):
            with self.assertRaises(PreflightError) as ctx:
                rec.drive_pool_pre_ready_recovery(runner, {"pool": {"node_pool": "kandev"}}, runner.ops, Path(self._tmp.name))
        self.assertIn("operator cleanup was used", str(ctx.exception))
        self.assertIn("nomad_stop", str(ctx.exception))

    def test_ops_are_unwrapped_after_the_run(self):
        sim = Sim()
        runner = make_runner(sim)
        original = runner.ops.nomad_stop
        with mock.patch.object(rec, "RecoveryScenario", fake_scenario_class(sim)), mock.patch.object(rec.base, "_await_recovered_environment", lambda *a, **k: sim.inventory()), \
                mock.patch.object(rec.base, "_check_session_untouched", lambda *a, **k: None):
            rec.drive_pool_pre_ready_recovery(runner, {"pool": {"node_pool": "kandev"}}, runner.ops, Path(self._tmp.name))
        self.assertIs(runner.ops.nomad_stop, original)


# -- pool-backend-restart ------------------------------------------------------------------------------------
class BackendRestartTest(DriveBase):
    def run_it(self, **flags):
        return self.drive(rec.drive_pool_backend_restart, **flags)

    def test_happy_path_attaches_the_existing_instance_and_calls_through_the_tunnel(self):
        report = self.run_it()
        self.assertEqual(report["scenario"], "pool-backend-restart")
        self.assertEqual(self.sim.stops, 1)
        self.assertEqual(report["attached_existing_instance"], {"job_id": JOB, "job_recreated": False, "allocation_id": "al1", "agentctl_unchanged": True,
                                                                "fence_unchanged": True, "new_jobs": []})
        self.assertEqual(report["tunnel"]["before"], {"port": 40123, "http_status": 200})
        self.assertEqual(report["tunnel"]["after"], {"port": 40123, "http_status": 200})
        self.assertTrue(report["tunnel"]["went_down_with_backend"])
        self.assertEqual(report["node_attach"], "ssh")
        self.assertTrue(report["production_cleanup"])
        self.assertEqual(report["release"], "none")
        self.assertTrue(self.sim.finished)

    def test_local_node_is_made_ineligible_before_launch_and_restored_after_cleanup(self):
        self.run_it()
        calls = [c for c in self.sim.calls if c[0] == "eligibility"]
        self.assertEqual(calls, [("eligibility", LOCAL, False), ("eligibility", LOCAL, True)])
        self.assertLess(self.steps().index("pin-to-ssh-node"), self.steps().index("start-backend"))
        self.assertEqual(self.sim.eligibility[LOCAL], "eligible")
        self.assertTrue(self.sim.state["recovery_restore"]["restored"])

    def test_eligibility_is_restored_even_when_an_assertion_fails(self):
        self.assert_fails(rec.drive_pool_backend_restart, "re-created", recreate_job=True)
        self.assertEqual(self.sim.eligibility[LOCAL], "eligible")

    def test_pool_without_a_local_node_needs_no_pin(self):
        report = self.run_it(no_local_node=True)
        self.assertEqual(self.sim.calls, [])
        self.assertFalse(report["pinned_to_ssh"])

    def test_environment_landing_on_the_local_node_is_refused(self):
        self.assert_fails(rec.drive_pool_backend_restart, "ssh node", landed_local=True)

    def test_recreated_job_fails(self):
        self.assert_fails(rec.drive_pool_backend_restart, "re-created", recreate_job=True)

    def test_restarted_agentctl_fails(self):
        self.assert_fails(rec.drive_pool_backend_restart, "process tree", agent_restarted=True)

    def test_changed_digest_fails(self):
        self.assert_fails(rec.drive_pool_backend_restart, "continuation digest", digest_changed=True)

    def test_tunnel_that_does_not_come_back_fails(self):
        self.assert_fails(rec.drive_pool_backend_restart, "tunnel", tunnel_stays_down=True)

    def test_tunnel_that_never_went_down_cannot_prove_the_new_tunnel(self):
        self.assert_fails(rec.drive_pool_backend_restart, "did not go down", tunnel_never_down=True)

    def test_bad_tunnel_status_fails_before_any_restart(self):
        self.assert_fails(rec.drive_pool_backend_restart, "status 502", tunnel_bad_status=True)
        self.assertEqual(self.sim.stops, 0)

    def test_unknown_tunnel_port_is_refused_before_any_restart(self):
        self.assert_fails(rec.drive_pool_backend_restart, "tunnel port", no_tunnel_port=True)
        self.assertEqual(self.sim.stops, 0)

    def test_native_launch_without_a_reverse_tunnel_skips_the_tunnel_steps(self):
        report = self.run_it(native_no_tunnel=True)
        self.assertIsNone(report["tunnel"])
        self.assertEqual(self.sim.tunnel_calls, 0)
        self.assertEqual(report["attached_existing_instance"]["job_recreated"], False)
        self.assertEqual(report["attached_existing_instance"]["agentctl_unchanged"], True)
        self.assertNotIn("determine-tunnel-port", self.steps())
        self.assertIn("assert-instance-attached", self.steps())

    def test_backend_that_will_not_stop_fails(self):
        self.assert_fails(rec.drive_pool_backend_restart, "did not stop", stop_fails=True)

    def test_stuck_cleanup_fails_and_keeps_the_fixture(self):
        self.assert_fails(rec.drive_pool_backend_restart, "production cleanup did not settle", cleanup_stuck=True)
        self.assertEqual(self.sim.eligibility[LOCAL], "eligible")


# -- pool-idle-outage ----------------------------------------------------------------------------------------
class IdleOutageTest(DriveBase):
    def run_it(self, **flags):
        return self.drive(rec.drive_pool_idle_outage, **flags)

    def test_happy_path_keeps_the_instance_alive_through_the_outage(self):
        report = self.run_it()
        self.assertEqual(report["scenario"], "pool-idle-outage")
        self.assertEqual(report["idle_timeout"], "1m")
        self.assertGreaterEqual(report["outage"]["observed_seconds"], 6)
        self.assertGreaterEqual(report["outage"]["samples"], 1)
        self.assertTrue(report["outage"]["backend_down_throughout"])
        self.assertTrue(report["instance_survived"])
        self.assertEqual(self.sim.stops, 1)
        self.assertEqual(self.sim.state["manifest_env"]["KANDEV_ACP_IDLE_TIMEOUT"], "1m")
        self.assertEqual(self.sim.state["manifest_env"]["A"], "b")
        self.assertTrue(report["production_cleanup"])

    def test_agentctl_idle_timeout_zero_is_observed_on_the_ssh_node(self):
        report = self.run_it()
        self.assertTrue(report["idle_timeout_observed"])
        self.assertEqual(report["agentctl_idle_timeout"], "0")
        self.assertEqual(report["scenario_kind"], "idle-timeout-disabled-and-survival")

    def test_agentctl_with_a_live_idle_timeout_fails(self):
        self.assert_fails(rec.drive_pool_idle_outage, "idle timeout", idle_env="1m")

    def test_unobservable_idle_timeout_is_recorded_and_the_scenario_is_survival_only(self):
        report = self.run_it(idle_env=None)
        self.assertFalse(report["idle_timeout_observed"])
        self.assertIsNone(report["agentctl_idle_timeout"])
        self.assertEqual(report["scenario_kind"], "survival-only")
        self.assertTrue(any("survival only" in item for item in report["limitations"]))

    def test_local_node_idle_timeout_is_read_from_the_process_environment(self):
        for text, want in (("PATH=/x KANDEV_ACP_IDLE_TIMEOUT=0 HOME=/h", "0"), ("A=b\nKANDEV_ACP_IDLE_TIMEOUT=1m\n", "1m"), ("A=b", None)):
            self.assertEqual(rec.parse_idle_timeout(text), want)
        with mock.patch.object(rec.RecoveryScenario, "local_environ", lambda self, pid: "KANDEV_ACP_IDLE_TIMEOUT=0", create=True):
            report = self.run_it(landed_local=True)
        self.assertTrue(report["idle_timeout_observed"])

    def test_idle_override_is_in_place_before_the_backend_starts(self):
        seen = {}
        sim_start = Sim.start_backend

        def spy(sim, barrier):
            seen.setdefault("env", dict(sim.state["manifest_env"]))
            return sim_start(sim, barrier)

        with mock.patch.object(Sim, "start_backend", spy):
            self.run_it()
        self.assertEqual(seen["env"]["KANDEV_ACP_IDLE_TIMEOUT"], "1m")

    def test_outage_shorter_than_the_idle_timeout_is_refused_before_anything_starts(self):
        with mock.patch.object(rec, "OUTAGE_SECONDS", 2):
            self.assert_fails(rec.drive_pool_idle_outage, "outage window")
        self.assertEqual(self.sim.backend_pids, 0)

    def test_instance_that_dies_during_the_outage_fails(self):
        original = Sim.nomad_snapshot

        def dying(sim, inventory):
            if not sim.backend_up:
                sim.outage_ticks += 1
                if sim.outage_ticks >= 2:
                    sim.inst["alive"] = False
            return original(sim, inventory)

        with mock.patch.object(Sim, "nomad_snapshot", dying):
            self.assert_fails(rec.drive_pool_idle_outage, "during the outage")

    def test_job_that_stops_during_the_outage_fails(self):
        self.assert_fails(rec.drive_pool_idle_outage, "during the outage", job_dead=True)

    def test_backend_that_will_not_stop_fails(self):
        self.assert_fails(rec.drive_pool_idle_outage, "did not stop", stop_fails=True)

    def test_recreated_job_after_recovery_fails(self):
        self.assert_fails(rec.drive_pool_idle_outage, "re-created", recreate_job=True)

    def test_restarted_agentctl_after_recovery_fails(self):
        self.assert_fails(rec.drive_pool_idle_outage, "process tree", agent_restarted=True)

    def test_ssh_node_tunnel_must_return_after_recovery(self):
        self.assert_fails(rec.drive_pool_idle_outage, "tunnel", tunnel_stays_down=True)

    def test_ssh_native_launch_without_a_reverse_tunnel_skips_the_tunnel(self):
        report = self.run_it(native_no_tunnel=True)
        self.assertIsNone(report["tunnel"])
        self.assertEqual(self.sim.tunnel_calls, 0)

    def test_local_node_has_no_tunnel_to_check(self):
        report = self.run_it(landed_local=True)
        self.assertIsNone(report["tunnel"])
        self.assertEqual(self.sim.tunnel_calls, 0)

    def test_stuck_cleanup_fails(self):
        self.assert_fails(rec.drive_pool_idle_outage, "production cleanup did not settle", cleanup_stuck=True)


if __name__ == "__main__":
    unittest.main()
