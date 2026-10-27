import hashlib
import http.client
import json
import stat
import tempfile
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import nomad_native_e2e_pool as pool
import nomad_native_e2e_pool_cleanup as cleanup
from nomad_native_e2e import PreflightError

LOCAL = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
SSH = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
TOKEN = "original-identity-token-content\n"
NOMAD_TOKEN = "nomad-runner-token-value"
REAL_KEY = "SHA256:" + "B" * 43
ENV_ID = "11111111-1111-4111-8111-111111111111"
CFG = {"pool": {"node_pool": "kandev", "host_identity_token_file": "/fixture/token"},
       "nomad": {"address": "http://127.0.0.1:4646"}}


def node(node_id, attach, extra):
    meta = {"kandev.attach": attach, "kandev.state_root": f"/{attach}-root", **extra}
    return {"ID": node_id, "NodePool": "kandev", "Status": "ready", "SchedulingEligibility": "eligible", "Meta": meta}


class Base:
    """Offline model of a two-node pool, the Nomad job book and the durable inventory rows."""

    def __init__(self, tmp, bad=None, **flags):
        self.flags, self.bad = flags, bad or {}
        self.home = Path(tmp) / "home"
        (self.home / "data").mkdir(parents=True)
        self.token_file = self.home / "data" / "nomad-host-identity"
        self.token_file.write_text(TOKEN)
        self.token_file.chmod(0o600)
        digest = hashlib.sha256(TOKEN.strip().encode()).hexdigest()
        self.nodes = {
            LOCAL: node(LOCAL, "local", {"kandev.host_identity_sha256": digest}),
            SSH: node(SSH, "ssh", {"kandev.ssh_host": "worker.invalid", "kandev.ssh_host_key_sha256": REAL_KEY}),
        }
        self.jobs, self.rows, self.tasks, self.vars, self.node_markers = {}, {}, {}, [], set()
        self.calls, self.archived, self.deleted, self.launches, self.finished = [], [], set(), 0, False
        self.evidence = SimpleNamespace(data={"status": "running", "steps": []}, secrets=[NOMAD_TOKEN],
                                        scrub=lambda text: text.replace(NOMAD_TOKEN, "[redacted]"))
        self.state = {"nomad": {"address": "http://127.0.0.1:4646", "namespace": "kandev", "region": "global", "node_id": SSH},
                      "environments": [], "baseline_jobs": [], "pool_state_root": "/ssh-root", "port": 1234}
        self.sessions = {}
        self.patches, self.profile_config = [], None

    # -- Nomad ---------------------------------------------------------------------------------------
    def nomad(self, path):
        base = path.split("?")[0]
        if base == "/v1/nodes":
            return [{"ID": node_id} for node_id in self.nodes]
        if base.startswith("/v1/node/"):
            return json.loads(json.dumps(self.nodes[base.rsplit("/", 1)[1]]))
        if base == "/v1/vars":
            return [{"Path": item} for item in self.vars]
        if base.startswith("/v1/var/"):
            if any(node_id in base for node_id in self.node_markers):
                return {"Path": "marker", "Items": {"reason": "x"}}
            raise PreflightError("Nomad GET failed with status 404")
        if base.startswith("/v1/job/"):
            parts = base.split("/")
            job = self.jobs.get(parts[3])
            if job is None:
                raise PreflightError("Nomad GET failed with status 404")
            if len(parts) == 5 and parts[4] == "allocations":
                return job["allocs"]
            if len(parts) == 5 and parts[4] == "evaluations":
                return job["evals"]
            return {"Job": {"ID": parts[3], "Status": job["status"]}}
        if base.startswith("/v1/allocation/"):
            return {"ID": base.rsplit("/", 1)[1], "ClientStatus": "running", "NodeID": SSH}
        raise PreflightError("Nomad GET failed with status 404")

    def set_eligibility(self, address, token, region, node_id, eligible):
        self.calls.append(("eligibility", node_id, eligible))
        self.nodes[node_id]["SchedulingEligibility"] = "eligible" if eligible else "ineligible"

    # -- backend ---------------------------------------------------------------------------------------
    def inventory(self, session_id, task_id):
        row = self.rows.get(session_id)
        if row is None:
            raise PreflightError("expected exactly one durable executor inventory row")
        return {"phase": row["phase"], "job_id": row["job_id"], "allocation_id": row.get("allocation_id"), "node_id": row.get("node_id"),
                "environment_id": row.get("environment_id", ENV_ID), "generation": 1}

    def api(self, method, path, body, headers=None):
        parts = path.split("/")
        if method == "POST" and path == "/tasks/delete-preflight":
            self.previews = getattr(self, "previews", {})
            self.previews["confirm-1"] = list(body["task_ids"])
            return {"confirmation_id": "confirm-1"}
        if method == "POST" and path == "/secrets":
            return {"id": "secret-1"}
        if method == "PATCH" and path.startswith("/executors/") and "/profiles/" in path:
            self.patches.append((path, body))
            return {}
        if method == "DELETE" and path.startswith("/tasks/"):
            if self.flags.get("delete_fails"):
                raise PreflightError("fixture API DELETE /tasks/x failed with status 500")
            confirmation = (headers or {}).get("X-Kandev-Task-Delete-Confirmation")
            if parts[2] not in getattr(self, "previews", {}).get(confirmation, []):
                raise PreflightError("fixture API DELETE /tasks/x failed with status 428")
            self.deleted.add(parts[2])
            return {}
        if method == "GET" and path.startswith("/tasks/") and len(parts) == 3:
            if parts[2] in self.deleted:
                raise PreflightError("fixture API GET /tasks/x failed with status 404")
            return {"id": parts[2]}
        if method == "GET" and path.endswith("/sessions"):
            task = self.tasks[parts[2]]
            return {"sessions": [{"id": task["session"], "state": self.session_state(task["session"])}]}
        raise PreflightError(f"unexpected api {method} {path}")

    def session_state(self, session_id):
        return self.sessions.get(session_id, "WAITING_FOR_INPUT")


def make_runner(sim, creates):
    runner = SimpleNamespace(home=sim.home, token=NOMAD_TOKEN, state=sim.state, evidence=sim.evidence, api=sim.api, nomad=sim.nomad,
                             inventory=sim.inventory)
    ticks = iter(range(10 ** 6))
    runner.ops = SimpleNamespace(nomad_set_eligibility=sim.set_eligibility, retention_settle=0.0, scenario_timeout=1.0, poll_interval=0.0,
                                 clock=lambda: float(next(ticks)), sleep=lambda s: None, prompt=lambda payload: {"accepted": True})
    runner.save_state = lambda **kw: sim.state.update(kw)
    runner.use_node = lambda attach, root: setattr(runner, "active_attach", attach)
    runner.active_attach = "ssh"
    runner.fence = lambda environment_id=None: None
    runner.fence_owner = lambda environment_id=None: "owner" if sim.flags.get("fence") else None
    runner.start_git_daemon = lambda: None
    runner.start_backend = lambda barrier: {"pid": 1}
    runner.database = "db"

    def pool_profile_config(cfg):
        config = {"node_pool": "kandev"}
        if cfg.get("security"):
            config["ca_file"] = cfg["security"]["ca_file"]
        return config

    runner.pool_profile_config = pool_profile_config
    runner._task_ctx = {"executor_id": "exec-1"}
    sim.state["executor_profile_id"] = "prof-1"

    def setup(cfg):
        sim.setup_cfg = cfg
        runner.api("POST", "/secrets", {"name": "nomad-fixture-token", "value": "x"})
        sim.profile_config = runner.pool_profile_config(cfg)
        task = creates()
        sim.state.update(task_id=task["id"], session_id=task["session_id"])

    runner.setup_api_objects = setup
    runner.create_task = lambda title: creates()

    def poll(timeout, interval, probe, failure):
        for _ in range(sim.flags.get("poll_rounds", 4 if not sim.flags.get("script") else 400)):
            value = probe()
            if value:
                return value
            time.sleep(0.005)
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


class DriveBase(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)

    def assert_fails(self, message, **kw):
        with self.assertRaises(PreflightError) as ctx:
            self.drive(**kw)
        self.assertIn(message, str(ctx.exception))
        return ctx.exception


# -- pool-unplaced-cleanup ---------------------------------------------------------------------------------
class UnplacedSim(Base):
    def launch(self):
        self.launches += 1
        n = self.launches
        bad = self.bad.get(n)
        job = f"kandev-job{n}"
        unplaced = n == 1
        allocs = [{"ID": "al1", "NodeID": SSH, "ClientStatus": "pending"}] if unplaced and bad == "placed" else []
        if not unplaced:
            allocs = [{"ID": "al2", "NodeID": LOCAL, "ClientStatus": "failed"}]
        evals = [] if bad == "no_eval" else ([{"Status": "blocked", "FailedTGAllocs": {"g": {"NodesEvaluated": 0}}}] if unplaced else [])
        self.jobs[job] = {"status": "pending", "allocs": allocs, "evals": evals}
        task = {"id": f"task-{n}", "session_id": f"sess-{n}"}
        self.tasks[task["id"]] = {"session": task["session_id"], "n": n, "job": job}
        self.rows[task["session_id"]] = {
            "phase": "ready" if bad == "ready" else ("registered" if unplaced else "cleanup_pending"), "job_id": job,
            "node_id": None if unplaced else LOCAL, "allocation_id": None if unplaced else "al2", "quarantine_state": "", "generation": 1}
        if n == 2 and bad == "refusal_marker":
            self.node_markers.add(LOCAL)
        return task

    def archive(self, task):
        self.archived.append(task["id"])
        info = self.tasks[task["id"]]
        row, bad = self.rows[info["session"]], self.bad.get(info["n"])
        if bad == "stuck":
            row["phase"] = "cleanup_pending"
            return
        if bad == "quarantine":
            self.vars.append("kandev/quarantine/" + (LOCAL if info["n"] == 2 else "x"))
            row.update(phase="quarantined", quarantine_state="marked")
            return
        row["phase"] = "absent"
        if bad == "job_running":
            return
        if bad == "purged":
            del self.jobs[info["job"]]
        else:
            self.jobs[info["job"]]["status"] = "dead"


def unplaced_scenario_class(sim):
    class Fake(cleanup.UnplacedScenario):
        def verify_node_identity(self):
            self.r.state["pool_state_root"] = "/ssh-root"
            return {"node_id": SSH}

        def kandev_jobs(self):
            return set(sim.jobs)

        def tree(self, env):
            if sim.flags.get("agentctl_on") and sim.flags["agentctl_on"] == getattr(self.r, "active_attach", None):
                return [{"pid": 9, "role": "agentctl", "depth": 0, "started": "x"}]
            return []

        def archive(self, task):
            sim.archive(task)

        def read_state(self, env):
            row = sim.rows[env["session_id"]]
            return {k: row.get(k) for k in ("phase", "generation", "job_id", "node_id", "quarantine_state")}

        def assert_refusal(self, case, task):
            env = self.record_env("R2", task, self.r.inventory(task["session_id"], task["id"]), attach="local", state_root="/local-root")
            return {"task_id": task["id"], "job_id": env["job_id"], "node_attach": "local", "refusal": "the node host identity does not match this host",
                    "quarantine_marker": {"Path": "marker"} if LOCAL in sim.node_markers else None}

        def finish(self, report):
            sim.finished = True
            return {**report, "finished": True}

    return Fake


class UnplacedBase(DriveBase):
    SimClass = UnplacedSim

    def drive(self, **kw):
        sim = self.sim = self.SimClass(self._tmp.name, **kw)
        self.runner = make_runner(sim, sim.launch)
        with mock.patch.object(cleanup, "UnplacedScenario", unplaced_scenario_class(sim)):
            return cleanup.drive_pool_unplaced_cleanup(self.runner, CFG, self.runner.ops, Path(self._tmp.name))

    def assert_restored(self):
        sim = self.sim
        self.assertEqual(sim.nodes[LOCAL]["SchedulingEligibility"], "eligible")
        self.assertEqual(sim.nodes[SSH]["SchedulingEligibility"], "eligible")
        self.assertEqual(sim.token_file.read_text(), TOKEN)
        self.assertEqual(stat.S_IMODE(sim.token_file.stat().st_mode), 0o600)

    def assert_fails(self, message, **kw):
        error = super().assert_fails(message, **kw)
        self.assert_restored()
        return error


class UnplacedHappyTest(UnplacedBase):
    def test_unplaced_job_is_cleaned_to_absent_without_quarantine_and_the_task_deletes(self):
        report = self.drive()
        unplaced = report["cases"]["unplaced"]
        self.assertEqual(unplaced["registered"]["allocations"], 0)
        self.assertEqual(unplaced["registered"]["inventory_phase"], "registered")
        self.assertTrue(unplaced["registered"]["placement_failed"])
        self.assertEqual(unplaced["cleanup"]["phase"], "absent")
        self.assertEqual(unplaced["cleanup"]["job"], "dead")
        self.assertEqual(unplaced["cleanup"]["quarantine_variables"], 0)
        self.assertTrue(unplaced["task_deleted"])
        self.assertEqual(self.sim.vars, [])

    def test_local_refusal_cleanup_completes_without_quarantine(self):
        report = self.drive()
        refused = report["cases"]["local-refusal"]
        self.assertEqual(refused["refusal"]["node_attach"], "local")
        self.assertEqual(refused["cleanup"]["phase"], "absent")
        self.assertIsNone(refused["refusal"]["quarantine_marker"])
        self.assertTrue(refused["task_deleted"])
        self.assertEqual(self.sim.archived, ["task-1", "task-2"])
        self.assertEqual(self.sim.deleted, {"task-1", "task-2"})

    def test_faults_are_applied_in_order_and_restored(self):
        report = self.drive()
        self.assert_restored()
        calls = self.sim.calls
        self.assertEqual(calls[0], ("eligibility", SSH, False))
        self.assertNotIn(("eligibility", LOCAL, False), calls[:1])
        self.assertTrue(report["unplaced"] and report["cleanup"]["finished"])
        self.assertTrue(self.sim.finished)
        self.assertNotIn(TOKEN.strip(), json.dumps(self.sim.evidence.data))

    def test_unplaced_launch_keeps_one_node_eligible_so_pool_preflight_passes(self):
        self.drive()
        self.assertNotIn(("eligibility", LOCAL, False), self.sim.calls)
        self.assertEqual(self.sim.calls[0], ("eligibility", SSH, False))

    def test_unplaced_profile_excludes_the_local_node_and_the_refusal_phase_lifts_it(self):
        self.drive()
        self.assertEqual(self.sim.profile_config.get("exclude_local"), "true")
        self.assertEqual(len(self.sim.patches), 1)
        path, body = self.sim.patches[0]
        self.assertEqual(path, "/executors/exec-1/profiles/prof-1")
        self.assertNotIn("exclude_local", body["config"])
        self.assertEqual(body["config"]["nomad_token"], "$kandev-secret:secret-1")

    def test_profile_hooks_are_restored_after_the_run(self):
        self.drive()
        self.assertNotIn("exclude_local", self.runner.pool_profile_config({}))

    def test_job_purged_by_production_cleanup_counts_as_stopped(self):
        report = self.drive(bad={1: "purged"})
        self.assertEqual(report["cases"]["unplaced"]["cleanup"]["job"], "absent")

    def test_existing_quarantine_variables_are_a_baseline_not_a_failure(self):
        sim_vars = ["kandev/quarantine/other"]

        class Seeded(UnplacedSim):
            def __init__(self, *a, **k):
                super().__init__(*a, **k)
                self.vars = list(sim_vars)

        self.SimClass = Seeded
        try:
            report = self.drive()
        finally:
            del self.SimClass
        self.assertEqual(report["cases"]["unplaced"]["cleanup"]["quarantine_variables"], 0)


class UnplacedFailureTest(UnplacedBase):
    def test_allocation_appearing_fails(self):
        self.assert_fails("was allocated", bad={1: "placed"})

    def test_no_placement_attempt_fails(self):
        self.assert_fails("no failed placement evaluation", bad={1: "no_eval"})

    def test_ready_inventory_fails(self):
        self.assert_fails("reached inventory phase ready", bad={1: "ready"})

    def test_agentctl_anywhere_fails(self):
        self.assert_fails("agentctl is running", agentctl_on="ssh")

    def test_fence_anywhere_fails(self):
        self.assert_fails("holds a worker fence", fence=True)

    def test_quarantine_variable_fails_the_unplaced_case(self):
        self.assert_fails("quarantin", bad={1: "quarantine"})

    def test_cleanup_that_never_reaches_absent_fails_with_pending_reason(self):
        error = self.assert_fails("did not settle", bad={1: "stuck"})
        self.assertIn("inventory phase 'cleanup_pending'", str(error))

    def test_job_left_running_fails(self):
        error = self.assert_fails("did not settle", bad={1: "job_running"})
        self.assertIn("job status 'pending'", str(error))

    def test_task_delete_rejected_fails(self):
        self.assert_fails("status 500", delete_fails=True)

    def test_local_refusal_with_quarantine_marker_fails_and_restores(self):
        self.assert_fails("quarantine", bad={2: "refusal_marker"})

    def test_local_refusal_cleanup_quarantine_variable_fails_and_restores(self):
        self.assert_fails("quarantin", bad={2: "quarantine"})

    def test_local_refusal_cleanup_stuck_fails_and_restores_the_token(self):
        self.assert_fails("did not settle", bad={2: "stuck"})
        self.assertTrue(self.sim.archived)


class ConfigTest(unittest.TestCase):
    def test_unplaced_needs_the_host_identity_token_file(self):
        with self.assertRaises(PreflightError):
            cleanup.check_scenario_config({"pool": {"node_pool": "kandev"}, "nomad": {"address": "http://127.0.0.1:1"}}, cleanup.SCENARIO_UNPLACED)
        cleanup.check_scenario_config({"pool": {"node_pool": "kandev"}, "nomad": {"address": "http://127.0.0.1:1"}}, cleanup.SCENARIO_FORCE)

    def test_pool_block_and_loopback_are_required(self):
        for scenario in (cleanup.SCENARIO_UNPLACED, cleanup.SCENARIO_REGISTER, cleanup.SCENARIO_FORCE):
            with self.assertRaises(PreflightError):
                cleanup.check_scenario_config({"nomad": {"address": "http://127.0.0.1:1"}}, scenario)
            with self.assertRaises(PreflightError):
                cleanup.check_scenario_config({"pool": {"node_pool": "kandev", "host_identity_token_file": "/t"},
                                               "nomad": {"address": "http://203.0.113.9:4646"}}, scenario)

    def test_registered_with_the_pool_runner(self):
        for name, (module, function) in pool.COVERAGE_DRIVERS.items():
            if module == "nomad_native_e2e_pool_cleanup":
                self.assertTrue(callable(getattr(cleanup, function)), name)
                self.assertIn(name, pool.SCENARIOS)


# -- pool-register-unknown ---------------------------------------------------------------------------------
class UpstreamHandler(BaseHTTPRequestHandler):
    calls = []

    def log_message(self, *_):
        return

    def _reply(self, status, body):
        type(self).calls.append((self.command, self.path, self.headers.get("X-Nomad-Token")))
        length = int(self.headers.get("Content-Length", "0"))
        if length:
            self.rfile.read(length)
        self.send_response(status)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_PUT(self):
        self._reply(200, b'{"EvalID":"e"}')

    def do_GET(self):
        self._reply(200, b'{"ID":"j"}')


class ProxyTest(unittest.TestCase):
    def setUp(self):
        UpstreamHandler.calls = []
        self.upstream = ThreadingHTTPServer(("127.0.0.1", 0), UpstreamHandler)
        threading.Thread(target=self.upstream.serve_forever, daemon=True).start()
        self.addCleanup(self.upstream.server_close)
        self.addCleanup(self.upstream.shutdown)
        self.proxy = cleanup.RegisterFaultProxy(f"http://127.0.0.1:{self.upstream.server_address[1]}")
        self.address = self.proxy.start()
        self.addCleanup(self.proxy.stop)
        self.port = int(self.address.rsplit(":", 1)[1])

    def request(self, method, path):
        conn = http.client.HTTPConnection("127.0.0.1", self.port, timeout=5)
        try:
            conn.request(method, path, body=b"{}" if method == "PUT" else None, headers={"X-Nomad-Token": "t"})
            response = conn.getresponse()
            return response.status, response.read()
        finally:
            conn.close()

    def test_listens_on_loopback_and_forwards_unarmed_requests(self):
        self.assertTrue(self.address.startswith("http://127.0.0.1:"))
        self.assertEqual(self.request("PUT", "/v1/jobs")[0], 200)
        self.assertEqual(self.request("GET", "/v1/job/j")[0], 200)
        self.assertEqual(self.proxy.fired, [])
        self.assertEqual(UpstreamHandler.calls[0][2], "t")

    def test_drop_commits_the_register_then_closes_the_connection_once(self):
        self.proxy.arm("drop")
        with self.assertRaises((http.client.RemoteDisconnected, ConnectionError, http.client.HTTPException)):
            self.request("PUT", "/v1/jobs")
        self.assertEqual([c[:2] for c in UpstreamHandler.calls], [("PUT", "/v1/jobs")])
        self.assertEqual(self.proxy.fired, [{"mode": "drop", "committed_http_status": 200}])
        self.assertEqual(self.request("PUT", "/v1/jobs")[0], 200)
        self.assertEqual(len(self.proxy.fired), 1)

    def test_5xx_commits_the_register_then_answers_a_server_error_once(self):
        self.proxy.arm("5xx")
        self.assertEqual(self.request("PUT", "/v1/jobs")[0], 502)
        self.assertEqual(len(UpstreamHandler.calls), 1)
        self.assertEqual(self.proxy.fired, [{"mode": "5xx", "committed_http_status": 200}])
        self.assertEqual(self.request("PUT", "/v1/jobs")[0], 200)

    def test_only_the_job_register_is_faulted(self):
        self.proxy.arm("drop")
        self.assertEqual(self.request("GET", "/v1/jobs?prefix=kandev-")[0], 200)
        self.assertEqual(self.request("PUT", "/v1/job/kandev-x/plan")[0], 200)
        self.assertEqual(self.proxy.fired, [])

    def test_unknown_mode_and_non_loopback_upstream_are_refused(self):
        with self.assertRaises(PreflightError):
            self.proxy.arm("slow")
        with self.assertRaises(PreflightError):
            cleanup.RegisterFaultProxy("http://203.0.113.9:4646")
        with self.assertRaises(PreflightError):
            cleanup.RegisterFaultProxy("ftp://127.0.0.1:4646")

    def test_https_upstream_is_verified_against_the_throwaway_ca(self):
        seen = {}

        class FakeConn:
            def __init__(self, host, port, timeout=None, context=None):
                seen.update(host=host, port=port, context=context)

            def request(self, *a, **k):
                raise OSError("stop here")

            def close(self):
                pass

        sentinel = object()
        with mock.patch.object(cleanup.ssl, "create_default_context", lambda cafile=None: (seen.update(cafile=cafile) or sentinel)), \
                mock.patch.object(cleanup.http.client, "HTTPSConnection", FakeConn):
            proxy = cleanup.RegisterFaultProxy("https://127.0.0.1:4646", ca_file="/ca.pem")
            address = proxy.start()
            try:
                conn = http.client.HTTPConnection("127.0.0.1", int(address.rsplit(":", 1)[1]), timeout=5)
                conn.request("GET", "/v1/jobs")
                try:
                    conn.getresponse()
                except (http.client.HTTPException, ConnectionError, OSError):
                    pass
                conn.close()
            finally:
                proxy.stop()
        self.assertEqual((seen["cafile"], seen["host"], seen["port"], seen["context"]), ("/ca.pem", "127.0.0.1", 4646, sentinel))


class RegisterSim(Base):
    """Register-unknown backend model: each launch commits a job whose register response is lost."""

    def __init__(self, tmp, **flags):
        super().__init__(tmp, **flags)
        self.proxy_events = []

    def launch(self):
        self.launches += 1
        n = self.launches
        bad = self.bad.get(n)
        job = f"kandev-job{n}"
        self.jobs[job] = {"status": "running", "allocs": [], "evals": []}
        task = {"id": f"task-{n}", "session_id": f"sess-{n}"}
        self.tasks[task["id"]] = {"session": task["session_id"], "n": n, "job": job}
        phase = {"orphan": "intent", "ready": "ready"}.get(bad, "cleanup_pending")
        row = {"phase": phase, "job_id": job, "node_id": None, "allocation_id": None, "quarantine_state": "", "generation": 1}
        self.rows[task["session_id"]] = row
        if bad == "orphan":
            del self.rows[task["session_id"]]   # a committed job with no durable record at all
        elif bad != "ready" and bad != "stuck":
            self.jobs[job]["status"] = "dead"
        return task

    def archive(self, task):
        self.archived.append(task["id"])
        info = self.tasks[task["id"]]
        row, bad = self.rows.get(info["session"]), self.bad.get(info["n"])
        if row is None or bad == "stuck":
            return
        row["phase"] = "absent"
        if bad == "quarantine":
            self.vars.append("kandev/quarantine/x")
            row.update(phase="quarantined", quarantine_state="marked")
            return
        self.jobs[info["job"]]["status"] = "running" if bad == "job_running" else "dead"


class FakeProxy:
    instances = []

    def __init__(self, upstream, ca_file=None):
        self.upstream, self.ca_file, self.fired, self.modes, self.stopped = upstream, ca_file, [], [], False
        FakeProxy.instances.append(self)

    def start(self):
        return "http://127.0.0.1:55555"

    def arm(self, mode):
        self.modes.append(mode)
        if FakeProxy.fire:
            self.fired.append({"mode": mode, "committed_http_status": 200})

    def stop(self):
        self.stopped = True


def register_scenario_class(sim):
    class Fake(cleanup.RegisterScenario):
        def verify_node_identity(self):
            self.r.state["pool_state_root"] = "/ssh-root"
            return {"node_id": SSH}

        def kandev_jobs(self):
            return set(sim.jobs)

        def tree(self, env):
            if sim.flags.get("agentctl_on") and sim.flags["agentctl_on"] == getattr(self.r, "active_attach", None):
                return [{"pid": 9, "role": "agentctl", "depth": 0, "started": "x"}]
            return []

        def archive(self, task):
            sim.archive(task)

        def read_state(self, env):
            row = sim.rows[env["session_id"]]
            return {k: row.get(k) for k in ("phase", "generation", "job_id", "node_id", "quarantine_state")}

        def finish(self, report):
            sim.finished = True
            return {**report, "finished": True}

    return Fake


class RegisterBase(DriveBase):
    def drive(self, fire=True, security=False, **kw):
        sim = self.sim = RegisterSim(self._tmp.name, **kw)
        self.runner = make_runner(sim, sim.launch)
        FakeProxy.instances, FakeProxy.fire = [], fire
        cfg = {**CFG, "security": {"ca_file": "/ca.pem"}} if security else CFG
        with mock.patch.object(cleanup, "RegisterScenario", register_scenario_class(sim)), mock.patch.object(cleanup, "RegisterFaultProxy", FakeProxy):
            return cleanup.drive_pool_register_unknown(self.runner, cfg, self.runner.ops, Path(self._tmp.name))


class RegisterHappyTest(RegisterBase):
    def test_both_faults_end_cleaned_up_with_no_orphan_job(self):
        report = self.drive()
        self.assertEqual(sorted(report["cases"]), ["drop", "five-xx"])
        for name in ("drop", "five-xx"):
            case = report["cases"][name]
            self.assertEqual(case["fault"]["committed_http_status"], 200)
            self.assertEqual(case["cleanup"]["phase"], "absent")
            self.assertEqual(case["orphan_jobs"], 0)
            self.assertEqual(case["quarantine_variables"], 0)
        self.assertEqual(report["cases"]["drop"]["fault"]["mode"], "drop")
        self.assertEqual(report["cases"]["five-xx"]["fault"]["mode"], "5xx")
        self.assertEqual(self.sim.archived, ["task-1", "task-2"])
        self.assertTrue(self.sim.finished)

    def test_profile_points_at_the_proxy_and_the_proxy_is_stopped(self):
        self.drive()
        self.assertEqual(self.sim.setup_cfg["nomad"]["address"], "http://127.0.0.1:55555")
        self.assertEqual(CFG["nomad"]["address"], "http://127.0.0.1:4646")
        proxy = FakeProxy.instances[0]
        self.assertEqual(proxy.upstream, "http://127.0.0.1:4646")
        self.assertEqual(proxy.modes, ["drop", "5xx"])
        self.assertTrue(proxy.stopped)

    def test_security_mode_verifies_the_upstream_with_the_throwaway_ca(self):
        self.drive(security=True)
        self.assertEqual(FakeProxy.instances[0].ca_file, "/ca.pem")

    def test_plaintext_proxy_profile_drops_ca_file_and_evidence_records_it(self):
        report = self.drive(security=True)
        self.assertNotIn("ca_file", self.sim.profile_config)
        self.assertEqual(report["backend_to_proxy_leg"], "plaintext-http")
        self.assertTrue(report["proxy_upstream_verified_with_ca"])

    def test_without_security_no_ca_file_and_leg_is_plaintext(self):
        report = self.drive()
        self.assertNotIn("ca_file", self.sim.profile_config)
        self.assertEqual(report["backend_to_proxy_leg"], "plaintext-http")
        self.assertFalse(report["proxy_upstream_verified_with_ca"])

    def test_a_launch_that_recovers_to_ready_is_archived_and_cleaned(self):
        report = self.drive(bad={1: "ready"})
        self.assertEqual(report["cases"]["drop"]["launch_outcome"], "ready")
        self.assertEqual(report["cases"]["drop"]["cleanup"]["phase"], "absent")
        self.assertEqual(report["cases"]["five-xx"]["launch_outcome"], "cleanup_pending")


class RegisterFailureTest(RegisterBase):
    def test_job_with_no_durable_record_is_an_orphan(self):
        self.assert_fails("registered without a durable cleanup record", bad={1: "orphan"})
        self.assertTrue(FakeProxy.instances[0].stopped)

    def test_second_case_orphan_fails(self):
        self.assert_fails("registered without a durable cleanup record", bad={2: "orphan"})

    def test_fault_that_never_fired_fails(self):
        self.assert_fails("register response fault never fired", fire=False)

    def test_cleanup_stuck_fails_with_pending_reason(self):
        error = self.assert_fails("did not settle", bad={1: "stuck"})
        self.assertIn("inventory phase", str(error))

    def test_job_left_running_fails(self):
        self.assert_fails("did not settle", bad={2: "job_running"})

    def test_quarantine_variable_fails(self):
        self.assert_fails("quarantin", bad={1: "quarantine"})

    def test_agentctl_remaining_fails(self):
        self.assert_fails("agentctl is running", agentctl_on="ssh")


# -- pool-force-stop ----------------------------------------------------------------------------------------
class ForceSim(Base):
    """Two running environments (local, ssh) with a recorded agent process tree; scripted samples drive the stop outcome."""

    def __init__(self, tmp, **flags):
        super().__init__(tmp, **flags)
        self.alive_agents = {}
        self.sent = []
        self.script = {}
        self.release = {}
        self.stop_returned = threading.Event()

    def launch(self):
        self.launches += 1
        n = self.launches
        job = f"kandev-job{n}"
        node_id = SSH if n == 1 else LOCAL
        self.jobs[job] = {"status": "running", "allocs": [{"ID": f"al{n}", "NodeID": node_id, "ClientStatus": "running"}], "evals": []}
        task = {"id": f"task-{n}", "session_id": f"sess-{n}"}
        self.tasks[task["id"]] = {"session": task["session_id"], "n": n, "job": job, "node": node_id}
        self.rows[task["session_id"]] = {"phase": "ready", "job_id": job, "node_id": node_id, "allocation_id": f"al{n}", "quarantine_state": "",
                                         "generation": 1, "environment_id": f"env-{n}"}
        return task

    def archive(self, task):
        self.archived.append(task["id"])
        self.rows[self.tasks[task["id"]]["session"]]["phase"] = "absent"
        self.jobs[self.tasks[task["id"]]["job"]]["status"] = "dead"


def agent_tree(n):
    return [
        {"pid": n * 100, "ppid": 1, "pgid": n * 100, "started": f"s{n}0", "depth": 0, "role": "agentctl"},
        {"pid": n * 100 + 1, "ppid": n * 100, "pgid": n * 100, "started": f"s{n}1", "depth": 1, "role": "acp"},
        {"pid": n * 100 + 2, "ppid": n * 100 + 1, "pgid": n * 100, "started": f"s{n}2", "depth": 2, "role": "child"},
    ]


def force_scenario_class(sim):
    class Fake(cleanup.ForceStopScenario):
        def verify_node_identity(self):
            self.r.state["pool_state_root"] = "/ssh-root"
            return {"node_id": SSH}

        def kandev_jobs(self):
            return set(sim.jobs)

        def await_placed(self, label, task):
            info = sim.tasks[task["id"]]
            meta = sim.nodes[info["node"]]["Meta"]
            inventory = self.r.inventory(task["session_id"], task["id"])
            env = self.record_env(label, task, inventory, attach=meta["kandev.attach"], state_root=meta["kandev.state_root"])
            return {"inventory": inventory, "tree": agent_tree(info["n"]), "fence": "f", "nomad": {}}

        def start_managed_tree(self, env, task, snapshot):
            n = sim.tasks[task["id"]]["n"]
            sim.alive_agents[task["id"]] = {(item["pid"], item["started"]) for item in agent_tree(n)}
            env["tree"] = agent_tree(n)
            return agent_tree(n)

        def workspace_layout(self, env):
            return {"kind": "worktree" if env["attach"] == "local" else "checkout", "branch": "b", "git_common_dir": "/g", "workspace": "/w"}

        def archive(self, task):
            sim.archived.append(task["id"])

        def alive(self, identities):
            task_id = f"task-{identities[0]['pid'] // 100}"
            if sim.tasks[task_id]["session"] not in sim.sent:
                return [item for item in identities if (item["pid"], item["started"]) in sim.alive_agents[task_id]]
            script = sim.script.setdefault(task_id, list(sim.flags.get("script", [])))
            if script and script[0].get("late"):
                # The processes are observed alive, then die and the stop call returns before the caller reads anything else.
                script.pop(0)
                seen = [item for item in identities if (item["pid"], item["started"]) in sim.alive_agents[task_id]]
                sim.alive_agents[task_id] = {key for key in sim.alive_agents[task_id] if key[0] % 100 == 0}
                sim.release[task_id].set()
                sim.stop_returned.wait(5)
                time.sleep(0.2)
                return seen
            if script:
                step = script.pop(0)
                if step["kill"]:
                    sim.alive_agents[task_id] = {key for key in sim.alive_agents[task_id] if key[0] % 100 == 0}
                if step.get("state"):
                    sim.sessions[sim.tasks[task_id]["session"]] = step["state"]
                if not script:
                    sim.release[task_id].set()
            return [item for item in identities if (item["pid"], item["started"]) in sim.alive_agents[task_id]]

        def send_force_stop(self, session_id):
            sim.sent.append(session_id)
            task_id = next(t for t, info in sim.tasks.items() if info["session"] == session_id)
            sim.release[task_id] = threading.Event()
            if not sim.flags.get("script"):
                sim.release[task_id].set()
            sim.release[task_id].wait(5)
            sim.stop_returned.set()
            if sim.flags.get("rejected"):
                raise PreflightError("session force-stop was rejected or failed")

        def production_settled(self, env, pending):
            return {"phase": "absent"}

        def assert_pool_cleaned(self, env, layout):
            return {"kind": layout["kind"]}

        def finish(self, report):
            sim.finished = True
            return {**report, "finished": True}

    return Fake


class ForceBase(DriveBase):
    def drive(self, **kw):
        sim = self.sim = ForceSim(self._tmp.name, **kw)
        self.runner = make_runner(sim, sim.launch)
        with mock.patch.object(cleanup, "ForceStopScenario", force_scenario_class(sim)):
            return cleanup.drive_pool_force_stop(self.runner, CFG, self.runner.ops, Path(self._tmp.name))


GONE = [{"kill": False}, {"kill": True}]


class ForceStopHappyTest(ForceBase):
    def scripted(self):
        # Two samples per environment: processes still alive and state not stopped, then processes gone and the state stopped.
        return [{"kill": False}, {"kill": True, "state": "CANCELLED"}]

    def test_each_node_type_is_force_stopped_and_agents_are_gone_before_stopped_is_reported(self):
        report = self.drive(script=self.scripted())
        self.assertEqual(sorted(report["force_stop"]), ["local", "ssh"])
        for attach in ("local", "ssh"):
            case = report["force_stop"][attach]
            self.assertEqual(case["agent_processes"], 2)
            self.assertEqual(case["agent_processes_after"], 0)
            self.assertEqual(case["final_state"], "CANCELLED")
            self.assertGreaterEqual(case["samples"], 2)
            self.assertFalse(case["stopped_reported_while_running"])
        self.assertEqual(sorted(self.sim.sent), ["sess-1", "sess-2"])

    def test_both_environments_are_archived_and_cleaned_through_production(self):
        report = self.drive(script=self.scripted())
        self.assertEqual(self.sim.archived, ["task-1", "task-2"])
        self.assertTrue(report["production_cleanup"])
        self.assertEqual(report["release"], "none")
        self.assertTrue(self.sim.finished)

    def test_spread_must_put_one_environment_on_each_node_type(self):
        self.assertEqual(sorted(self.drive(script=self.scripted())["spread"].values()), ["local", "ssh"])


class ForceStopFailureTest(ForceBase):
    def test_stopped_state_while_an_agent_process_runs_fails(self):
        error = self.assert_fails("reported stopped while", script=[{"kill": False, "state": "CANCELLED"}, {"kill": False}, {"kill": True}])
        self.assertNotIn(NOMAD_TOKEN, str(error))

    def test_force_stop_accepted_while_an_agent_process_runs_fails(self):
        # The call returns (script exhausted) with the agent still alive: the accepted stop is a stopped report.
        self.assert_fails("reported stopped while", script=[{"kill": False}])

    def test_agents_dying_and_the_stop_returning_between_reads_is_not_a_false_failure(self):
        report = self.drive(script=[{"late": True}, {"kill": True, "state": "CANCELLED"}])
        self.assertEqual(report["force_stop"]["ssh"]["agent_processes_after"], 0)
        self.assertEqual(report["force_stop"]["local"]["agent_processes_after"], 0)

    def test_rejected_force_stop_fails_even_when_the_agent_is_gone(self):
        self.assert_fails("force-stop was not accepted", script=[{"kill": True}], rejected=True)

    def test_agent_processes_missing_before_the_stop_fail_the_precondition(self):
        class Empty(ForceSim):
            def launch(self):
                task = super().launch()
                return task

        self.SimClass = Empty
        sim = self.sim = Empty(self._tmp.name)
        runner = make_runner(sim, sim.launch)
        fake = force_scenario_class(sim)

        class NoAgents(fake):
            def start_managed_tree(self, env, task, snapshot):
                return agent_tree(1)[:1]

        with mock.patch.object(cleanup, "ForceStopScenario", NoAgents):
            with self.assertRaises(PreflightError) as ctx:
                cleanup.drive_pool_force_stop(runner, CFG, runner.ops, Path(self._tmp.name))
        self.assertIn("no agent process", str(ctx.exception))

    def test_both_environments_on_one_node_type_fail_the_spread_check(self):
        class SameNode(ForceSim):
            def launch(self):
                task = super().launch()
                self.tasks[task["id"]]["node"] = SSH
                self.rows[task["session_id"]]["node_id"] = SSH
                return task

        sim = self.sim = SameNode(self._tmp.name, script=GONE)
        runner = make_runner(sim, sim.launch)
        with mock.patch.object(cleanup, "ForceStopScenario", force_scenario_class(sim)):
            with self.assertRaises(PreflightError) as ctx:
                cleanup.drive_pool_force_stop(runner, CFG, runner.ops, Path(self._tmp.name))
        self.assertIn("same node", str(ctx.exception))


class ForceStopHelperTest(unittest.TestCase):
    def test_force_stop_helper_sends_force_true_through_node(self):
        seen = {}

        def run(argv, input=None, **kw):
            seen.update(argv=argv, payload=json.loads(input), kw=kw)
            return SimpleNamespace(returncode=0, stdout='{"ok":true}\n', stderr="")

        cleanup.force_stop_session("ws://127.0.0.1:1", "sess-1", run=run)
        self.assertEqual(seen["argv"][0], "node")
        self.assertEqual(seen["payload"], {"ws_base": "ws://127.0.0.1:1", "session_id": "sess-1"})
        self.assertIn("force: true", cleanup.FORCE_STOP_JS)
        self.assertIn("session.stop", cleanup.FORCE_STOP_JS)

    def test_helper_refuses_a_non_loopback_base_and_surfaces_a_rejection(self):
        with self.assertRaises(PreflightError):
            cleanup.force_stop_session("ws://203.0.113.9:1", "sess-1", run=lambda *a, **k: None)
        with self.assertRaises(PreflightError) as ctx:
            cleanup.force_stop_session("ws://127.0.0.1:1", "sess-1", run=lambda *a, **k: SimpleNamespace(returncode=2, stdout="", stderr="secret"))
        self.assertNotIn("secret", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
