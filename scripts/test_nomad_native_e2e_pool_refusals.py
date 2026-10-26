import hashlib
import io
import json
import os
import stat
import tempfile
import unittest
import urllib.error
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import nomad_native_e2e_pool_refusals as refusals
from nomad_native_e2e import PreflightError

LOCAL = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
SSH = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
TOKEN = "original-identity-token-content\n"
NOMAD_TOKEN = "nomad-runner-token-value"
REAL_KEY = "SHA256:" + "B" * 43
JOB = "kandev-abc123"
ENV_ID = "11111111-1111-4111-8111-111111111111"


class Sim:
    """Offline model of two pool nodes, the backend's launch outcome and the operator calls the scenario makes."""

    def __init__(self, tmp, bad=None, **flags):
        self.flags, self.bad = flags, bad or {}
        self.home = Path(tmp) / "home"
        (self.home / "data").mkdir(parents=True)
        self.token_file = self.home / "data" / "nomad-host-identity"
        self.token_file.write_text(TOKEN)
        self.token_file.chmod(0o600)
        digest = hashlib.sha256(TOKEN.strip().encode()).hexdigest()
        self.nodes = {
            LOCAL: self.node(LOCAL, "local", {"kandev.host_identity_sha256": digest}),
            SSH: self.node(SSH, "ssh", {"kandev.ssh_host": "worker.invalid", "kandev.ssh_host_key_sha256": REAL_KEY}),
        }
        self.launches, self.tasks, self.calls, self.markers = 0, {}, [], set()
        self.archived, self.released, self.leftovers, self.statuses = [], [], [], []
        self.evidence = SimpleNamespace(data={"status": "running", "steps": []}, secrets=[NOMAD_TOKEN],
                                        scrub=lambda text: text.replace(NOMAD_TOKEN, "[redacted]"))
        self.state = {"nomad": {"address": "http://127.0.0.1:4646", "namespace": "default", "region": "global", "node_id": SSH},
                      "environments": []}

    @staticmethod
    def node(node_id, attach, extra):
        meta = {"kandev.attach": attach, "kandev.state_root": f"/{attach}-root", **extra}
        return {"ID": node_id, "NodePool": "kandev", "Status": "ready", "SchedulingEligibility": "eligible", "Meta": meta}

    # -- Nomad ---------------------------------------------------------------------------------------
    def nomad(self, path):
        if path == "/v1/nodes":
            return [{"ID": node_id} for node_id in self.nodes]
        if path.startswith("/v1/node/"):
            return json.loads(json.dumps(self.nodes[path.rsplit("/", 1)[1]]))
        if path.startswith("/v1/var/"):
            if any(node_id in path for node_id in self.markers):
                return {"Path": "marker", "Items": {"reason": getattr(self, "marker_text", "x")}}
            raise PreflightError("Nomad GET failed with status 404")
        if path.startswith("/v1/job/") and path.endswith("/allocations"):
            return [{"ID": "al1", "NodeID": self.tasks_node(), "ClientStatus": "failed"}]
        raise PreflightError("Nomad GET failed with status 404")

    def tasks_node(self):
        return self.current["node"]

    def set_eligibility(self, address, token, region, node_id, eligible):
        self.calls.append(("eligibility", node_id, eligible))
        if eligible and self.flags.get("enable_fails"):
            raise PreflightError("Nomad eligibility change is unreachable")
        self.nodes[node_id]["SchedulingEligibility"] = "eligible" if eligible else "ineligible"

    # -- backend launch --------------------------------------------------------------------------------
    def launch(self):
        self.launches += 1
        n = self.launches
        local_digest = hashlib.sha256(self.token_file.read_text().strip().encode()).hexdigest()
        ssh_key = self.nodes[SSH]["Meta"]["kandev.ssh_host_key_sha256"]
        if ssh_key == refusals.WRONG_HOST_KEY and self.nodes[LOCAL]["SchedulingEligibility"] == "ineligible":
            node, text = SSH, "launch failed: connect to pinned Nomad worker: ssh: handshake failed: host key changed: expected SHA256:A, got SHA256:B"
        elif self.nodes[SSH]["SchedulingEligibility"] == "ineligible" and local_digest != self.nodes[LOCAL]["Meta"]["kandev.host_identity_sha256"]:
            node, text = LOCAL, "launch failed: nomad local node refused: the node host identity does not match this host"
        else:
            node, text = SSH, "launched"
        phase, bad = "cleanup_pending", self.bad.get(n)
        if bad == "ready":
            phase = "ready"
        if bad == "no_cause":
            text = "launch failed: something unrelated"
        if bad == "secret":
            text += f" token={NOMAD_TOKEN}"
        if bad == "secret_identity":
            text += f" identity={self.token_file.read_text().strip()}"
        if bad == "fallback":
            node = SSH if node == LOCAL else LOCAL
        self.current = {"text": text, "phase": phase, "node": node, "n": n, "bad": bad}
        task = {"id": f"task-{n}", "session_id": f"sess-{n}"}
        self.tasks[task["id"]] = {**self.current}
        return task

    def api(self, method, path, body):
        if path == "/executors/nomad/readmit":
            self.calls.append(("readmit", body["node_id"], self.markers_at_readmit(body)))
            return {"readmitted": True}
        task_id = path.split("/")[2] if path.startswith("/tasks/") else None
        info = self.tasks[task_id] if task_id else next(t for t in self.tasks.values() if t["n"] == int(path.split("sess-")[1].split("/")[0]))
        if path.endswith("/sessions"):
            return {"sessions": [{"id": f"sess-{info['n']}", "state": "FAILED"}]}
        return {"messages": [{"content": info["text"]}]}

    def markers_at_readmit(self, body):
        return {"marker_present": body["node_id"] in self.markers, "attestation": body["attestation"]["fence_removed"]}

    def inventory(self, session_id, task_id):
        info = self.tasks[task_id]
        return {"phase": info["phase"], "job_id": JOB, "allocation_id": "al1", "node_id": info["node"], "environment_id": ENV_ID,
                "generation": 1}


class FakeMeta:
    def __init__(self, sim):
        self.sim = sim

    def apply(self, node_id, value):
        self.sim.calls.append(("meta", node_id, value))
        meta = self.sim.nodes[node_id]["Meta"]
        if value is None or value == REAL_KEY:
            if self.sim.flags.get("unset_fails"):
                raise PreflightError("Nomad dynamic node metadata change is unreachable")
            meta["kandev.ssh_host_key_sha256"] = REAL_KEY
        elif self.sim.flags.get("rejected"):
            raise PreflightError("Nomad rejected the dynamic node metadata change with status 400")
        elif not self.sim.flags.get("invisible"):
            meta["kandev.ssh_host_key_sha256"] = value


def make_runner(sim):
    runner = SimpleNamespace(home=sim.home, token=NOMAD_TOKEN, state=sim.state, evidence=sim.evidence, api=sim.api, nomad=sim.nomad,
                             inventory=sim.inventory)
    ticks = iter(range(10 ** 6))
    runner.ops = SimpleNamespace(nomad_set_eligibility=sim.set_eligibility, retention_settle=0.0, nomad_delete_variable=lambda address, token, namespace, path: sim.markers.discard(path.rsplit('/', 1)[1]) or 'deleted', scenario_timeout=1.0, poll_interval=0.0,
                                 clock=lambda: float(next(ticks)), sleep=lambda s: None)
    runner.save_state = lambda **kw: sim.state.update(kw)
    runner.use_node = lambda attach, root: None
    runner.fence = lambda: None
    runner.start_git_daemon = lambda: None
    runner.start_backend = lambda barrier: {"pid": 1}

    def setup(cfg):
        task = sim.launch()
        sim.state.update(task_id=task["id"], session_id=task["session_id"])

    runner.setup_api_objects = setup
    runner.create_task = lambda title: sim.launch()

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


def fake_scenario_class(sim):
    class Fake(refusals.RefusalScenario):
        def verify_node_identity(self):
            self.r.state["pool_state_root"] = "/ssh-root"
            return {"node_id": SSH}

        def kandev_jobs(self):
            return set()

        def tree(self, env):
            if sim.flags.get("agentctl_on") == env.get("attach") and sim.launches == sim.flags.get("agentctl_launch", sim.launches):
                return [{"pid": 9, "role": "agentctl", "depth": 0, "started": "x"}]
            return []

        def archive(self, task):
            sim.archived.append(task["id"])

        def read_state(self, env):
            return {"phase": "cleanup_pending", "generation": 1, "node_attach": env["attach"], "quarantine_state": ""}

        def production_settled(self, env, pending):
            if sim.flags.get("production"):
                return {"phase": "absent"}
            raise PreflightError("inventory phase 'cleanup_pending' retained")

        def release(self, label, kill_first, **kw):
            sim.released.append(label)
            return {"label": label, "release": "operator-teardown"}

        def remove_pool_leftovers(self, env, strict):
            if sim.flags.get("leftovers"):
                raise PreflightError("pool leftovers of R1 could not be removed: /x")
            sim.leftovers.append(env["label"])
            return {"removed": True}

        def finish(self, report):
            return {**report, "finished": True}

    return Fake


class DriveBase(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)

    def drive(self, **kw):
        self.sim = sim = Sim(self._tmp.name, **kw)
        self.runner = make_runner(sim)
        cfg = {"pool": {"node_pool": "kandev", "host_identity_token_file": "/fixture/token"}}
        with mock.patch.object(refusals, "RefusalScenario", fake_scenario_class(sim)), \
                mock.patch.object(refusals, "make_meta", lambda runner: FakeMeta(sim)):
            return refusals.drive_pool_refusals(self.runner, cfg, self.runner.ops, Path(self._tmp.name))

    def assert_restored(self):
        sim = self.sim
        self.assertEqual(sim.nodes[SSH]["Meta"]["kandev.ssh_host_key_sha256"], REAL_KEY)
        self.assertEqual(sim.nodes[LOCAL]["SchedulingEligibility"], "eligible")
        self.assertEqual(sim.nodes[SSH]["SchedulingEligibility"], "eligible")
        self.assertEqual(sim.token_file.read_text(), TOKEN)
        self.assertEqual(stat.S_IMODE(sim.token_file.stat().st_mode), 0o600)

    def assert_fails(self, message, **kw):
        with self.assertRaises(PreflightError) as ctx:
            self.drive(**kw)
        self.assertIn(message, str(ctx.exception))
        self.assert_restored()
        return ctx.exception


class HappyPathTest(DriveBase):
    def test_both_refusals_are_asserted_recorded_and_everything_is_restored(self):
        report = self.drive()
        self.assert_restored()
        self.assertEqual(report["scenario"], "pool-refusals")
        one, two = report["cases"]["wrong-host-key"], report["cases"]["fake-local-node"]
        for case, node, cause in ((one, SSH, "host key changed"), (two, LOCAL, "does not match this host")):
            refusal = case["refusal"]
            self.assertEqual(refusal["inventory_phase"], "cleanup_pending")
            self.assertEqual(refusal["node_id"], node)
            self.assertIn(cause, refusal["refusal"])
            self.assertEqual(refusal["state"]["phase"], "cleanup_pending")
            self.assertIsNone(refusal["quarantine_marker"])
            self.assertEqual((refusal["ready_reached"], refusal["ssh_fallback"], refusal["agentctl_processes"], refusal["secret_in_error"]),
                             (False, False, 0, False))
            self.assertEqual(sorted(refusal["process_trees"]), ["local", "ssh"])
            self.assertEqual(case["cleanup"]["path"], "operator-teardown")
            self.assertEqual(case["cleanup"]["retained_state"]["phase"], "cleanup_pending")
        self.assertEqual(self.sim.archived, ["task-1", "task-2"])
        self.assertEqual(self.sim.released, ["R1", "R2"])
        self.assertEqual(self.sim.leftovers, ["R1", "R2"])
        self.assertFalse(report["production_cleanup"])
        self.assertEqual(report["release"], "operator-teardown")
        self.assertTrue(report["cleanup"]["finished"])
        self.assertNotIn(TOKEN.strip(), json.dumps(self.sim.evidence.data))

    def test_fault_order_wrong_key_launches_before_the_identity_fault_exists(self):
        self.drive()
        calls = self.sim.calls
        self.assertEqual(calls[0], ("eligibility", LOCAL, False))
        self.assertEqual(calls[1], ("meta", SSH, refusals.WRONG_HOST_KEY))
        self.assertIn(("meta", SSH, REAL_KEY), calls)
        self.assertLess(calls.index(("meta", SSH, REAL_KEY)), calls.index(("eligibility", SSH, False)))
        self.assertEqual(calls.count(("eligibility", LOCAL, True)), 1)
        self.assertEqual(calls.count(("eligibility", SSH, True)), 1)

    def test_production_cleanup_path_needs_no_operator_release(self):
        report = self.drive(production=True)
        self.assertEqual(report["cleanup_paths"], {"wrong-host-key": "production-archive-cleanup", "fake-local-node": "production-archive-cleanup"})
        self.assertTrue(report["production_cleanup"])
        self.assertEqual(report["release"], "none")
        self.assertEqual(self.sim.released, [])

    def test_restore_record_is_saved_for_the_operator(self):
        self.drive()
        record = self.sim.state["refusal_restore"]
        self.assertEqual(record["host_key_override"]["original"], REAL_KEY)
        self.assertTrue(record["host_key_override"]["restored"])
        self.assertEqual(record["token"]["restore_from"], "/fixture/token")
        self.assertNotIn(TOKEN.strip(), json.dumps(self.sim.state))


class FailureTest(DriveBase):
    def test_override_rejected_restores_eligibility(self):
        self.assert_fails("Nomad rejected the dynamic node metadata change", rejected=True)

    def test_override_not_visible_fails_and_restores(self):
        self.assert_fails("override is not visible", invisible=True)

    def test_task_reaching_ready_fails(self):
        self.assert_fails("reached inventory phase ready", bad={1: "ready"})

    def test_error_missing_the_cause_fails(self):
        self.assert_fails("no refusal naming 'host key changed'", bad={1: "no_cause"})

    def test_second_case_missing_the_cause_fails_and_restores_the_token(self):
        self.assert_fails("no refusal naming 'the node host identity does not match this host'", bad={2: "no_cause"})
        self.assertIn(("meta", SSH, REAL_KEY), self.sim.calls)

    def test_agentctl_on_the_node_fails(self):
        self.assert_fails("agentctl is running on the ssh node", agentctl_on="ssh")

    def test_agentctl_in_second_case_restores_token_and_eligibility(self):
        self.assert_fails("agentctl is running on the local node", agentctl_on="local", agentctl_launch=2)

    def test_credential_in_the_error_fails_without_echoing_it(self):
        error = self.assert_fails("credential or key material appears", bad={1: "secret"})
        self.assertNotIn(NOMAD_TOKEN, str(error))

    def test_host_identity_token_in_the_error_fails(self):
        self.assert_fails("credential or key material appears", bad={2: "secret_identity"})

    def test_fallback_to_another_node_fails(self):
        self.assert_fails("other than the ssh node", bad={1: "fallback"})
        self.assert_restored()

    def test_second_case_fallback_fails(self):
        self.assert_fails("other than the local node", bad={2: "fallback"})

    def test_leftovers_that_cannot_be_removed_fail(self):
        self.assert_fails("could not be removed", leftovers=True)

    def refusal_case(self, marker_text):
        sim = Sim(self._tmp.name)
        self.sim = sim
        scn = fake_scenario_class(sim)(make_runner(sim), {"pool": {"host_identity_token_file": "/f"}}, "x", FakeMeta(sim))
        scn.nodes = {"local": {"id": LOCAL, "meta": sim.nodes[LOCAL]["Meta"]}, "ssh": {"id": SSH, "meta": sim.nodes[SSH]["Meta"]}}
        task = sim.launch()
        scn.envs["R1"] = {"label": "R1", "node_id": SSH, "attach": "ssh", "state_root": "/ssh-root", "job_id": JOB, "environment_id": ENV_ID}
        sim.markers.add(SSH)
        sim.marker_text = marker_text
        return scn, task

    def test_a_marker_with_another_reason_fails_and_is_not_deleted(self):
        scn, task = self.refusal_case("drain_failed")
        with self.assertRaises(PreflightError) as ctx:
            scn.end_case(refusals.WRONG_KEY_CASE, task)
        self.assertIn("reason other than", str(ctx.exception))
        self.assertIn(SSH, self.sim.markers)

    def test_the_rollback_quarantine_is_released_and_its_marker_deleted(self):
        scn, task = self.refusal_case(refusals.ROLLBACK_QUARANTINE_REASON)
        result = scn.end_case(refusals.WRONG_KEY_CASE, task)
        self.assertEqual(result["path"], "operator-teardown")
        self.assertTrue(result["quarantine"]["marker_deleted"])
        self.assertNotIn(SSH, self.sim.markers)
        self.assertEqual(result["quarantine"]["eligibility"], "enabled")
        self.assertEqual(self.sim.nodes[SSH]["SchedulingEligibility"], "eligible")
        order = [c[0] for c in self.sim.calls if c[0] in ("readmit",)]
        self.assertEqual(order, ["readmit"])
        self.assertTrue(next(c for c in self.sim.calls if c[0] == "readmit")[2]["marker_present"])

    def test_eligibility_left_when_a_marker_exists_at_restore(self):
        sim = Sim(self._tmp.name)
        self.sim = sim
        scn = fake_scenario_class(sim)(make_runner(sim), {}, "x", FakeMeta(sim))
        scn.nodes = {"local": {"id": LOCAL, "meta": {}}, "ssh": {"id": SSH, "meta": {}}}
        scn.disable_node("local")
        sim.markers.add(LOCAL)
        failures = scn.restore_all()
        self.assertEqual(sim.nodes[LOCAL]["SchedulingEligibility"], "ineligible")
        self.assertEqual(len(failures), 1)
        self.assertIn("quarantine marker", failures[0])
        self.assertIn(f"nomad node eligibility -enable {LOCAL}", failures[0])


class RestoreFailureTest(DriveBase):
    def test_failed_restore_after_a_failed_assertion_keeps_the_original_error_and_records_failures(self):
        with self.assertRaises(PreflightError) as ctx:
            self.drive(bad={1: "ready"}, enable_fails=True)
        self.assertIn("reached inventory phase ready", str(ctx.exception))
        failures = self.sim.evidence.data["restore_failures"]
        self.assertTrue(any("eligibility" in item for item in failures))
        self.assertEqual(self.sim.token_file.read_text(), TOKEN)

    def test_restore_failure_on_the_success_path_fails_the_run(self):
        with self.assertRaises(PreflightError) as ctx:
            self.drive(unset_fails=True)
        self.assertIn("restoration failed", str(ctx.exception))
        self.assertIn("ssh host key metadata", str(ctx.exception))
        self.assertEqual(self.sim.evidence.data["steps"][-1]["status"], "failed")

    def test_restore_is_idempotent_after_a_clean_restore(self):
        self.drive()
        self.assertEqual(self.sim.calls.count(("meta", SSH, REAL_KEY)), 1)


class NodeMetaTest(unittest.TestCase):
    class Opener:
        def __init__(self, response=b"{}", error=None):
            self.requests, self.response, self.error = [], response, error

        def open(self, request, timeout=None):
            self.requests.append(request)
            if self.error:
                raise self.error
            return io.BytesIO(self.response)

    def meta(self, opener):
        return refusals.NodeMeta("http://127.0.0.1:4646", NOMAD_TOKEN, "global", opener)

    def test_override_posts_the_dynamic_metadata_with_the_runner_token(self):
        opener = self.Opener(json.dumps({"Meta": {"kandev.ssh_host_key_sha256": refusals.WRONG_HOST_KEY}}).encode())
        self.meta(opener).apply(SSH, refusals.WRONG_HOST_KEY)
        request = opener.requests[0]
        self.assertEqual(request.get_method(), "POST")
        self.assertEqual(request.full_url, f"http://127.0.0.1:4646/v1/client/metadata?node_id={SSH}&region=global")
        self.assertEqual(request.get_header("X-nomad-token"), NOMAD_TOKEN)
        self.assertEqual(json.loads(request.data), {"NodeID": SSH, "Meta": {"kandev.ssh_host_key_sha256": refusals.WRONG_HOST_KEY}})

    def test_unset_sends_null(self):
        opener = self.Opener()
        self.meta(opener).apply(SSH, None)
        self.assertEqual(json.loads(opener.requests[0].data)["Meta"], {"kandev.ssh_host_key_sha256": None})

    def test_rejection_names_the_status_and_requirements_without_the_token(self):
        error = urllib.error.HTTPError("http://x", 400, "bad", {}, io.BytesIO(b"static meta"))
        with self.assertRaises(PreflightError) as ctx:
            self.meta(self.Opener(error=error)).apply(SSH, refusals.WRONG_HOST_KEY)
        self.assertIn("status 400", str(ctx.exception))
        self.assertIn("Nomad 1.5", str(ctx.exception))
        self.assertNotIn(NOMAD_TOKEN, str(ctx.exception))

    def test_static_value_that_survives_the_override_is_rejected(self):
        body = json.dumps({"Meta": {"kandev.ssh_host_key_sha256": REAL_KEY}}).encode()
        with self.assertRaises(PreflightError) as ctx:
            self.meta(self.Opener(body)).apply(SSH, refusals.WRONG_HOST_KEY)
        self.assertIn("cannot override", str(ctx.exception))

    def test_unreachable_and_malformed_responses_and_node_ids(self):
        with self.assertRaises(PreflightError) as ctx:
            self.meta(self.Opener(error=OSError("down"))).apply(SSH, None)
        self.assertIn("unreachable", str(ctx.exception))
        with self.assertRaises(PreflightError) as ctx:
            self.meta(self.Opener(b"not json")).apply(SSH, None)
        self.assertIn("not JSON", str(ctx.exception))
        with self.assertRaises(PreflightError):
            self.meta(self.Opener()).apply("../bad", None)


class TokenAndConfigTest(DriveBase):
    def scenario(self, content=TOKEN):
        sim = Sim(self._tmp.name)
        sim.token_file.write_text(content)
        self.sim = sim
        scn = fake_scenario_class(sim)(make_runner(sim), {"pool": {"host_identity_token_file": "/f"}}, "x", FakeMeta(sim))
        scn.nodes = {"local": {"id": LOCAL, "meta": sim.nodes[LOCAL]["Meta"]}, "ssh": {"id": SSH, "meta": sim.nodes[SSH]["Meta"]}}
        return scn

    def test_token_that_does_not_hash_to_the_local_digest_is_refused_before_any_change(self):
        scn = self.scenario("some other token\n")
        with self.assertRaises(PreflightError):
            scn.swap_token()
        self.assertEqual(self.sim.token_file.read_text(), "some other token\n")

    def test_missing_or_symlinked_token_is_refused(self):
        scn = self.scenario()
        self.sim.token_file.unlink()
        with self.assertRaises(PreflightError):
            scn.check_token()
        target = Path(self._tmp.name) / "elsewhere"
        target.write_text(TOKEN)
        os.symlink(target, self.sim.token_file)
        with self.assertRaises(PreflightError):
            scn.check_token()

    def test_swap_replaces_with_private_random_content_and_restore_is_exact(self):
        scn = self.scenario()
        scn.swap_token()
        replaced = self.sim.token_file.read_text()
        self.assertNotEqual(replaced, TOKEN)
        self.assertEqual(stat.S_IMODE(self.sim.token_file.stat().st_mode), 0o600)
        self.assertIn(replaced.strip(), self.sim.evidence.secrets)
        self.assertEqual(scn.restore_token(), "restored")
        self.assertEqual(self.sim.token_file.read_text(), TOKEN)
        self.assertEqual(scn.restore_token(), "not needed")

    def test_locate_nodes_requires_a_restorable_host_key(self):
        sim = Sim(self._tmp.name)
        sim.nodes[SSH]["Meta"]["kandev.ssh_host_key_sha256"] = refusals.WRONG_HOST_KEY
        scn = fake_scenario_class(sim)(make_runner(sim), {"pool": {"node_pool": "kandev"}}, "x", FakeMeta(sim))
        with self.assertRaises(PreflightError):
            scn.locate_nodes()

    def test_scenario_config_requires_token_file_and_loopback_nomad(self):
        good = {"pool": {"host_identity_token_file": "/t"}, "nomad": {"address": "http://127.0.0.1:4646"}}
        refusals.check_scenario_config(good)
        with self.assertRaises(PreflightError):
            refusals.check_scenario_config({**good, "pool": {}})
        with self.assertRaises(PreflightError):
            refusals.check_scenario_config({**good, "nomad": {"address": "http://example.invalid:4646"}})

    def test_wrong_host_key_fixture_value_is_a_valid_unpadded_fingerprint(self):
        self.assertRegex(refusals.WRONG_HOST_KEY, r"^SHA256:[A-Za-z0-9+/]{43}$")


if __name__ == "__main__":
    unittest.main()
