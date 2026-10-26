import contextlib
import dataclasses
import hashlib
import io
import json
import unittest
from types import SimpleNamespace

import nomad_native_e2e_fixture as fixture
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError, main
from test_nomad_native_e2e_fixture import ALLOC, ENV_ID, JOB, NODE, TOKEN
from test_nomad_native_e2e_wo06 import ALLOC_B, ENV_B, JOB_B, Sim, Wo06Base

RECEIPT = hashlib.sha256(b"drain-receipt").hexdigest()
TERMINAL = ("complete", "failed", "lost")


class Sim7(Sim):
    """Sim plus the WO07 production behaviour: archive-driven cleanup, quarantine marker, readmission and refusal."""

    def __init__(self, root_parent, mode, **flags):
        flags.setdefault("lazy_tree", True)
        super().__init__(root_parent, **flags)
        self.mode, self.log, self.signals, self.readmit_bodies = mode, [], [], []
        self.stopped, self.marker_present, self.readmitted = False, False, False
        self.refused_tasks, self.archived, self.last_ops, self.restarts = {}, [], None, 0

    # -- durable state --------------------------------------------------------------------------------
    def set_state(self, session="sess1", **updates):
        db = self._db()
        row = json.loads(db.execute("SELECT metadata FROM executors_running WHERE session_id=?", (session,)).fetchone()[0])
        row["nomad_environment_state"].update(updates)
        db.execute("UPDATE executors_running SET metadata=? WHERE session_id=?", (json.dumps(row), session))
        db.commit()
        db.close()

    def on_start(self, env):
        super().on_start(env)
        if self.task_created:
            self.restarts += 1
            if self.flags.get("restart_changes"):
                self.set_state(quarantine_state="")

    # -- production behaviour -------------------------------------------------------------------------
    def drain_ok(self):
        if self.flags.get("operator_during"):
            self.last_ops.nomad_stop("http://127.0.0.1:4646", TOKEN, "default", JOB)
        if not self.flags.get("procs_remain"):
            self.procs = [p for p in self.procs if p["env"] != "A"]
        if not self.flags.get("job_purged"):
            self.jobs[JOB]["status"] = "dead" if not self.flags.get("job_running") else "running"
        else:
            self.jobs.pop(JOB)
        for alloc_id in self.jobs.get(JOB, {"alloc_ids": []})["alloc_ids"]:
            self.allocs[alloc_id]["ClientStatus"] = "complete"
        if self.flags.get("extra_alloc"):
            self._alloc("55555555-5555-4555-8555-5555555555ee", JOB, "complete")
        if self.flags.get("other_node"):
            self.allocs[ALLOC]["NodeID"] = "99999999-9999-4999-8999-999999999999"
        if self.flags.get("alloc_running"):
            self.allocs[ALLOC]["ClientStatus"] = "running"
        if self.flags.get("window_open"):
            self.jobs[JOB]["status"] = "running"
            self.allocs[ALLOC]["ClientStatus"] = "running"
            self.later(3, self.finish_stop)
        if not self.flags.get("fence_remains"):
            self.fence_text = None
        if self.flags.get("marker_left"):
            self.marker_present = True
        if self.flags.get("node_ineligible"):
            self.node["SchedulingEligibility"] = "ineligible"
        state = {"phase": "absent", "drain_receipt_digest": "" if self.flags.get("no_receipt") else RECEIPT,
                 "drain_receipt_generation": 0 if self.flags.get("stale_generation") else 1}
        if self.flags.get("bad_digest"):
            state["drain_receipt_digest"] = "not-a-digest"
        if self.flags.get("quarantined"):
            state.update(phase="quarantined", quarantine_state="marked")
        self.set_state(**state)

    def finish_stop(self):
        self.procs = [p for p in self.procs if p["env"] != "A"]
        self.jobs[JOB]["status"] = "dead"
        self.allocs[ALLOC]["ClientStatus"] = "complete"

    def drain_fails(self):
        phase = "cleanup_pending"
        updates = {"phase": phase, "quarantine_state": "" if self.flags.get("no_quarantine") else "marked"}
        if self.flags.get("receipt_written"):
            updates.update(drain_receipt_digest=RECEIPT, drain_receipt_generation=1)
        if self.flags.get("false_absent"):
            updates["phase"] = "absent"
        if self.flags.get("false_operator_released"):
            updates["phase"] = "operator_released"
        if self.flags.get("neg_job_purged"):
            self.jobs.pop(JOB)
        if self.flags.get("neg_fence_released"):
            self.fence_text = None
        self.marker_present = not self.flags.get("no_marker")
        if not self.flags.get("stay_eligible"):
            self.node["SchedulingEligibility"] = "ineligible"
        self.set_state(**updates)

    def on_archive(self):
        self.archived.append(True)
        self.later(2, self.drain_fails if self.mode == "cleanup-negative" and self.stopped else self.drain_ok)

    # -- injected callables -----------------------------------------------------------------------------
    def api_factory(self, port):
        inner = super().api_factory(port)

        def call(method, path, body=None):
            if method == "POST" and path == "/tasks/task1/archive":
                self.on_archive()
                return {}
            if method == "POST" and path == wo07.READMIT_API_PATH:
                self.log.append("readmit")
                self.readmit_bodies.append(body)
                if self.flags.get("readmit_rejects"):
                    raise PreflightError("fixture API POST /executors/nomad/readmit failed with status 409")
                self.readmitted = True
                return {}
            if method == "POST" and path == "/tasks" and self.task_created:
                return self.create_b(body)
            for task_id, session in self.refused_tasks.items():
                if path == f"/tasks/{task_id}/sessions":
                    return {"sessions": [{"id": session, "state": "FAILED"}]}
                if path == f"/task-sessions/{session}/messages":
                    text = "" if self.flags.get("no_refusal") else "launch refused: Nomad node is quarantined"
                    return {"messages": [{"id": "r1", "type": "error", "content": text}]}
            return inner(method, path, body)

        return call

    def create_b(self, body):
        self.titles.append(body["title"])
        quarantined = self.marker_present or (self.mode == "cleanup-negative" and not self.readmitted and not self.flags.get("b_launches_always"))
        if quarantined and not self.readmitted and not self.flags.get("b_launches"):
            self.refused_tasks["task2"] = "sess2"
            return {"id": "task2", "session_id": "sess2"}
        if self.flags.get("b_still_refused") and self.readmitted:
            self.refused_tasks["task3"] = "sess3"
            return {"id": "task3", "session_id": "sess3"}
        task_id, session = ("task2", "sess2") if not self.readmitted else ("task3", "sess3")
        self.b_created = True
        self.jobs[JOB_B] = {"status": "pending", "alloc_ids": [], "env": ENV_B, "alloc": None}
        state = self._state(JOB_B, None, ENV_B, "creating")
        db = self._db()
        db.execute("INSERT INTO executors_running VALUES (?,?,'nomad','ex-b',?)", (session, task_id, json.dumps({"nomad_environment_state": state})))
        db.commit()
        db.close()
        self.b_session = session
        self.reschedule()
        return {"id": task_id, "session_id": session}

    def _place_b(self):
        super()._place_b()
        if self.fence_text is not None and ALLOC_B in self.allocs and self.allocs[ALLOC_B]["ClientStatus"] == "running":
            self.jobs[JOB_B]["status"] = "running"
            self.set_state(session=self.b_session, phase="ready", allocation_id=ALLOC_B)

    def nomad_factory(self, address, token, namespace, region):
        inner = super().nomad_factory(address, token, namespace, region)

        def call(path):
            if path.startswith("/v1/var/"):
                if self.marker_present:
                    return {"Path": path, "Items": {}}
                raise PreflightError("Nomad GET /v1/var/x failed with status 404")
            return inner(path)

        return call

    def signal_pid(self, ssh, pid, sig):
        self.log.append(f"signal-{sig}")
        self.signals.append((pid, sig))
        self.stopped = sig == "STOP"
        return "signalled"

    def delete_variable(self, address, token, namespace, path):
        self.log.append("delete-marker")
        assert path == f"kandev/quarantine/{NODE}"
        self.marker_present = False
        return "removed"

    def nomad_stop(self, address, token, namespace, job_id):
        self.log.append("job-stop")
        return super().nomad_stop(address, token, namespace, job_id)

    def set_eligibility(self, address, token, region, node_id, eligible):
        self.log.append(f"eligibility-{'enable' if eligible else 'disable'}")
        return super().set_eligibility(address, token, region, node_id, eligible)

    def ssh_factory(self, ssh):
        inner = super().ssh_factory(ssh)

        def call(command):
            result = inner(command)
            if "rmdir" in command and result[0] == 0:
                self.log.append("fence-removed")
            if command.startswith("kill -KILL "):
                self.log.append("kill")
            return result

        return call

    def ops(self):
        self.last_ops = dataclasses.replace(super().ops(), signal_pid=self.signal_pid, nomad_delete_variable=self.delete_variable)
        return self.last_ops


class Wo07Base(Wo06Base):
    def make(self, **flags):
        self.world = Sim7(self.parent, self.mode, **flags)
        return self.world

    def run7(self, **flags):
        sim = self.make(**flags)
        return wo07.run_scenario(
            self.config, self.mode, ops=sim.ops(), out=self.out.append, err=self.err.append,
            run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout,
        )

    def assert_sanitized(self):
        text = (self.fixture_root() / "evidence.json").read_text() + "\n".join(self.out + self.err)
        for forbidden in (TOKEN, "worker.example", "fixture@", "/srv/kandev", "BEGIN", "KEY"):
            self.assertNotIn(forbidden, text)
        self.assertNotIn(TOKEN, self.everything_written())

    def assert_fails(self, message, step=None, **flags):
        return self.assert_failed(self.run7(**flags), message, step)


class CleanupPositiveTest(Wo07Base):
    mode = "cleanup-positive"

    def test_success_path_and_evidence(self):
        self.assertEqual(self.run7(), 0, "\n".join(self.err))
        evidence = self.evidence()
        self.assertEqual(evidence["status"], "passed")
        self.assertTrue(evidence["production_cleanup"])
        self.assertFalse(evidence["operator_cleanup_used"])
        self.assertEqual(evidence["release"], "none")
        cleanup = evidence["identities"]["A"]["cleanup"]
        self.assertEqual((cleanup["phase"], cleanup["drain_receipt_digest"], cleanup["drain_receipt_generation"]), ("absent", RECEIPT, 1))
        self.assertEqual((cleanup["job_status"], cleanup["job_purged"], cleanup["allocations"], cleanup["same_node"]), ("dead", False, 1, True))
        self.assertEqual((cleanup["processes"], cleanup["fence"], cleanup["quarantine_marker"]), (0, None, False))
        names = [item["name"] for item in evidence["steps"]]
        self.assertLess(names.index("archive-task"), names.index("await-production-cleanup"))
        self.assertLess(names.index("await-production-cleanup"), names.index("assert-no-operator-cleanup"))
        self.assertEqual((cleanup["window_missed"], cleanup["tree_gone_before_job_stop"]), (True, False))
        self.assertEqual(self.world.archived, [True])
        self.assertEqual(self.world.stops, [])
        self.assertEqual(self.world.kills, [])
        self.assertEqual(self.world.signals, [])
        self.assertFalse(any("rmdir" in c for c in self.world.ssh_commands))
        self.assertEqual(self.world.records, [])

    def test_evidence_and_output_are_sanitized(self):
        self.assertEqual(self.run7(), 0)
        self.assert_sanitized()

    def test_operator_cleanup_during_the_wait_fails_the_run(self):
        self.assert_fails("operator cleanup was used", "await-production-cleanup", operator_during=True)

    def test_failed_run_stops_the_runners_own_backend_and_keeps_the_fixture(self):
        self.assert_fails("operator cleanup was used", "await-production-cleanup", operator_during=True)
        evidence = json.loads((self.fixture_root() / "evidence.json").read_text())
        self.assertEqual(evidence["stopped_on_failure"]["kandev-fixture"], "stopped")
        self.assertTrue((self.fixture_root() / "fixture.json").exists())
        self.assertIn("stopped", self.world.events)

    def test_operator_guard_records_each_operator_op_and_restores(self):
        ops = fixture.Ops()
        originals = {name: getattr(ops, name) for name in wo07.OPERATOR_OPS}
        guard = wo07.OperatorGuard(ops)
        self.assertTrue(all(getattr(ops, name) is not originals[name] for name in wo07.OPERATOR_OPS))
        guard.assert_clean()
        guard.restore()
        self.assertTrue(all(getattr(ops, name) is originals[name] for name in wo07.OPERATOR_OPS))
        ops = fixture.Ops(kill_exact=lambda *a: "killed")
        guard = wo07.OperatorGuard(ops)
        ops.kill_exact(None, {}, [])
        guard.record("remove_fence")
        with self.assertRaisesRegex(PreflightError, "kill_exact.*remove_fence"):
            guard.assert_clean()
        guard.armed = False
        ops.kill_exact(None, {}, [])
        self.assertEqual(guard.calls, ["kill_exact", "remove_fence"])

    def test_missing_drain_receipt(self):
        self.assert_fails("without a drain receipt", "await-production-cleanup", no_receipt=True)

    def test_stale_receipt_generation(self):
        self.assert_fails("is not the current generation", "await-production-cleanup", stale_generation=True)

    def test_invalid_receipt_digest(self):
        self.assert_fails("not a sha256", "await-production-cleanup", bad_digest=True)

    def test_job_purged_instead_of_stopped(self):
        self.assert_fails("purged instead of stopped", "await-production-cleanup", job_purged=True)

    def test_job_not_dead(self):
        self.assert_fails("job status 'running'", "await-production-cleanup", job_running=True)

    def test_second_allocation(self):
        self.assert_fails("2 allocations", "await-production-cleanup", extra_alloc=True)

    def test_allocation_on_another_node(self):
        self.assert_fails("not on the original node", "await-production-cleanup", other_node=True)

    def test_allocation_not_terminal(self):
        self.assert_fails("allocation 'running'", "await-production-cleanup", alloc_running=True)

    def test_tree_already_gone_at_receipt_passes(self):
        self.assertEqual(self.run7(window_open=True), 0, "\n".join(self.err))
        cleanup = self.evidence()["identities"]["A"]["cleanup"]
        self.assertEqual((cleanup["tree_gone_before_job_stop"], cleanup["window_missed"]), (True, False))
        self.assertEqual(cleanup["drain_receipt_digest"], RECEIPT)

    def test_tree_present_at_receipt_with_running_allocation_fails(self):
        self.assert_fails("the drain stopped nothing", "await-production-cleanup", window_open=True, procs_remain=True)

    def test_window_missed_is_recorded(self):
        self.assertEqual(self.run7(), 0)
        cleanup = self.evidence()["identities"]["A"]["cleanup"]
        self.assertTrue(cleanup["window_missed"])
        self.assertFalse(cleanup["tree_gone_before_job_stop"])
        self.assertIn(cleanup["drain_receipt_digest"], cleanup["window_missed_evidence"] + RECEIPT)

    def test_processes_remaining(self):
        self.assert_fails("managed processes remain", "await-production-cleanup", procs_remain=True)

    def test_fence_remaining(self):
        self.assert_fails("worker fence remains", "await-production-cleanup", fence_remains=True)

    def test_quarantine_marker_left_behind(self):
        self.assert_fails("left a quarantine marker", "await-production-cleanup", marker_left=True)

    def test_node_left_ineligible(self):
        self.assert_fails("ineligible node", "await-production-cleanup", node_ineligible=True)

    def test_production_quarantine_is_not_a_pass(self):
        self.assert_fails("quarantined the environment", "await-production-cleanup", quarantined=True)

    def test_cleanup_never_happens(self):
        sim = self.make()
        sim.on_archive = lambda: None
        code = wo07.run_scenario(self.config, self.mode, ops=sim.ops(), out=self.out.append, err=self.err.append,
                                 run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assert_failed(code, "production cleanup did not settle before timeout; pending: inventory phase 'ready'", "await-production-cleanup")


class CleanupNegativeTest(Wo07Base):
    mode = "cleanup-negative"

    def test_success_path_and_evidence(self):
        self.assertEqual(self.run7(), 0, "\n".join(self.err))
        sim = self.world
        evidence = self.evidence()
        self.assertEqual(evidence["status"], "passed")
        self.assertFalse(evidence["production_cleanup"])
        self.assertTrue(evidence["production_quarantine"])
        quarantined = evidence["identities"]["A"]["quarantined"]
        self.assertEqual((quarantined["phase"], quarantined["quarantine_state"], quarantined["drain_receipt_digest"]), ("cleanup_pending", "marked", None))
        self.assertEqual((quarantined["quarantine_marker"], quarantined["node_eligibility"], quarantined["fence_retained"], quarantined["job_purged"]),
                         (True, "ineligible", True, False))
        self.assertEqual(evidence["refused_launch"]["jobs_created"], 0)
        self.assertEqual(evidence["refused_launch"]["allocations"], 0)
        self.assertTrue(evidence["restart"]["state_unchanged"])
        self.assertEqual(sim.restarts, 1)
        self.assertEqual(sim.signals[0], (200, "STOP"))
        self.assertEqual(evidence["readmission"]["endpoint"], wo07.READMIT_PATH)
        self.assertEqual(evidence["identities"]["B"]["allocation_id"], ALLOC_B)
        self.assertEqual(sim.node["SchedulingEligibility"], "eligible")
        self.assertFalse(sim.marker_present)
        self.assertEqual(evidence["cleanup"]["jobs"], {"A": "dead", "B": "dead"})

    def test_readmission_steps_run_in_order(self):
        self.assertEqual(self.run7(), 0, "\n".join(self.err))
        names = [item["name"] for item in self.evidence()["steps"]]
        steps = [name for name in names if name.startswith("readmit-")]
        self.assertEqual(steps, [f"readmit-{name}" for name in wo07.READMIT_STEPS])
        self.assertLess(names.index("restart-backend-and-assert-state-unchanged"), names.index("readmit-resume-leader"))
        self.assertLess(names.index("readmit-enable-eligibility"), names.index("assert-b-admitted-after-readmission"))
        log = self.world.log
        order = ["signal-STOP", "signal-CONT", "kill", "job-stop", "fence-removed", "readmit", "delete-marker", "eligibility-enable"]
        positions = [log.index(item) for item in order]
        self.assertEqual(positions, sorted(positions))
        self.assertEqual(log.count("signal-STOP"), 1)

    def test_readmit_payload_and_path(self):
        self.assertEqual(wo07.READMIT_PATH, "/api/v1/executors/nomad/readmit")
        self.assertEqual(self.run7(), 0, "\n".join(self.err))
        body = self.world.readmit_bodies[0]
        self.assertEqual(sorted(body), ["attestation", "environment_id", "generation", "job_id", "node_id"])
        self.assertEqual((body["node_id"], body["job_id"], body["generation"]), (NODE, JOB, 1))
        self.assertTrue(body["attestation"]["process_tree_absent"])
        self.assertNotIn("attestation", self.evidence()["readmission"].get("readmit", {}))

    def test_evidence_and_output_are_sanitized(self):
        self.assertEqual(self.run7(), 0)
        self.assert_sanitized()

    def test_receipt_persisted_despite_failed_drain(self):
        self.assert_fails("drain receipt was persisted", "assert-a-retained-and-quarantined", receipt_written=True)

    def test_false_absence(self):
        self.assert_fails("false absence", "assert-a-retained-and-quarantined", false_absent=True)

    def test_false_operator_released(self):
        self.assert_fails("'operator_released' without a drain receipt", "assert-a-retained-and-quarantined", false_operator_released=True)

    def test_job_purged(self):
        self.assert_fails("purged although cleanup is retained", "assert-a-retained-and-quarantined", neg_job_purged=True)

    def test_fence_released(self):
        self.assert_fails("fence was released", "assert-a-retained-and-quarantined", neg_fence_released=True)

    def test_quarantine_not_marked(self):
        self.assert_fails("pending: quarantine_state ''", "assert-a-retained-and-quarantined", no_quarantine=True)

    def test_marker_missing(self):
        self.assert_fails("pending: quarantine marker absent", "assert-a-retained-and-quarantined", no_marker=True)

    def test_node_still_eligible(self):
        self.assert_fails("node still eligible", "assert-a-retained-and-quarantined", stay_eligible=True)

    def test_new_launch_not_refused(self):
        self.assert_fails("not refused with a quarantine error", "launch-b-refused", no_refusal=True)

    def test_new_launch_created_a_job(self):
        self.assert_fails("although the node is quarantined", "launch-b-refused", b_launches=True)

    def test_state_changed_across_restart(self):
        self.assert_fails("state of A changed across the backend restart", "restart-backend-and-assert-state-unchanged", restart_changes=True)

    def test_readmit_rejection_stops_before_marker_and_eligibility(self):
        self.assert_fails("status 409", "readmit-readmit", readmit_rejects=True)
        log = self.world.log
        self.assertNotIn("delete-marker", log)
        self.assertNotIn("eligibility-enable", log)
        self.assertIn("fence-removed", log)
        self.assertTrue(self.world.marker_present)
        self.assertEqual(self.world.node["SchedulingEligibility"], "ineligible")

    def test_b_not_admitted_after_readmission(self):
        self.assert_fails("did not reach durable inventory phase ready", "assert-b-admitted-after-readmission", b_still_refused=True)

    def test_cleanup_fixture_recovers_a_failed_run(self):
        self.assert_fails("status 409", "readmit-readmit", readmit_rejects=True)
        code, out, err = self.cleanup()
        self.assertEqual(code, 0, err)
        self.assertEqual(self.world.records, [])


class Wo07OpsTest(unittest.TestCase):
    def test_signal_pid_sends_only_stop_or_cont(self):
        sent = []
        ssh = lambda command: sent.append(command) or (0, "")
        self.assertEqual(fixture._default_signal_pid(ssh, 200, "STOP"), "signalled")
        fixture._default_signal_pid(ssh, 200, "CONT")
        self.assertEqual(sent, ["kill -STOP 200", "kill -CONT 200"])
        for pid, sig in ((200, "KILL"), (1, "STOP"), (True, "STOP"), (0, "CONT")):
            with self.assertRaises(PreflightError):
                fixture._default_signal_pid(ssh, pid, sig)
        with self.assertRaises(PreflightError):
            fixture._default_signal_pid(lambda command: (1, ""), 200, "STOP")

    def test_signal_pid_is_an_ops_default(self):
        self.assertIs(fixture.Ops().signal_pid, fixture._default_signal_pid)
        self.assertIs(fixture.Ops().nomad_delete_variable, fixture._default_nomad_delete_variable)

    def test_delete_variable_refuses_anything_but_a_node_marker(self):
        for path in ("kandev/quarantine/not-a-uuid", "kandev/other/" + NODE, "kandev/quarantine/" + NODE + "/x", "../" + NODE):
            with self.assertRaises(PreflightError):
                fixture._default_nomad_delete_variable("http://127.0.0.1:4646", TOKEN, "default", path)


class DispatchTest(unittest.TestCase):
    def test_both_scenarios_dispatch_to_wo07(self):
        calls = []
        original = wo07.create_fixture_command
        wo07.create_fixture_command = lambda path, scenario: calls.append((path, scenario)) or 0
        try:
            self.assertEqual(main(["run", "--scenario", "cleanup-positive", "--require-real", "--create-fixture", "--config", "p.json"]), 0)
            self.assertEqual(main(["run", "--scenario", "cleanup-negative", "--require-real", "--create-fixture", "--config", "n.json"]), 0)
        finally:
            wo07.create_fixture_command = original
        self.assertEqual(calls, [("p.json", "cleanup-positive"), ("n.json", "cleanup-negative")])

    def test_require_real_is_needed(self):
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            self.assertEqual(main(["run", "--scenario", "cleanup-positive", "--create-fixture", "--config", "x"]), 2)
        self.assertIn("--create-fixture requires", err.getvalue())

    def test_unreadable_config(self):
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            self.assertEqual(wo07.create_fixture_command("/nonexistent.json", "cleanup-negative"), 2)
        self.assertIn("config is unreadable", err.getvalue())

    def test_non_loopback_nomad_creates_nothing(self):
        with self.assertRaisesRegex(PreflightError, "loopback http"):
            wo07.check_scenario_config({"nomad": {"address": "https://nomad.example.com:4646"}}, "cleanup-positive")
        with self.assertRaises(PreflightError):
            wo07.check_scenario_config({"nomad": {"address": "http://127.0.0.1:4646"}}, "admission")


if __name__ == "__main__":
    unittest.main()
