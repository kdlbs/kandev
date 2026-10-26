import contextlib
import io
import json
import os
import sqlite3
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace

import nomad_native_e2e as base
import nomad_native_e2e_bootstrap as bootstrap
import nomad_native_e2e_fixture as fixture
from nomad_native_e2e import PreflightError, main

TOKEN = "s3cr3t-nomad-token-0f9e8d7c"
KEY_BODY = "-----PRIVATE-KEY-BODY-7a6b5c-----"
NODE = "44444444-4444-4444-8444-444444444444"
ENV_ID = "11111111-1111-4111-8111-111111111111"
EXEC_ID = "66666666-6666-4666-8666-666666666666"
ALLOC = "55555555-5555-4555-8555-555555555555"
JOB = "kandev-abc123"
FINGERPRINT = "SHA256:" + "A" * 43
EXECUTOR = "77777777-7777-4777-8777-777777777777"


FIXTURE_HEAD = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"


class FakeClock:
    def __init__(self):
        self.now = 0.0

    def clock(self):
        return self.now

    def sleep(self, seconds):
        self.now += seconds


class FakeRegistry:
    def __init__(self, world):
        self.world = world

    def read_records(self):
        return [dict(item) for item in self.world.records]

    def start(self, label, argv, *, cwd, env):
        w = self.world
        if w.reuse_pid and w.last_crashed_pid and label == fixture.PROCESS_LABEL:
            pid = w.last_crashed_pid
        else:
            w.pid += 1
            pid = w.pid
        record = {"label": label, "pid": pid, "started_at": "t", "pgid": pid}
        w.records.append(record)
        w.alive.add(pid)
        port = int(env["KANDEV_SERVER_PORT"]) if "KANDEV_SERVER_PORT" in env else int(next(a for a in argv if a.startswith("--port=")).split("=")[1])
        w.open_ports.add(port)
        w.record_ports[pid] = port
        w.starts.append({"argv": list(argv), "env": dict(env), "cwd": str(cwd)})
        if label != fixture.GIT_DAEMON_LABEL:
            w.on_start(env)
        return dict(record)

    def _retire(self, label, result):
        w = self.world
        record = next(item for item in w.records if item["label"] == label)
        w.events.append(result)
        if not (w.stop_leaves_alive and result == "stopped"):
            w.alive.discard(record["pid"])
            w.open_ports.discard(w.record_ports.pop(record["pid"], None))
            w.records = [item for item in w.records if item["label"] != label]
        return result

    def crash(self, label, timeout=5):
        w = self.world
        record = next(item for item in w.records if item["label"] == label)
        port = w.record_ports.get(record["pid"])
        if w.crash_result != "crashed":
            return w.crash_result
        result = self._retire(label, "crashed")
        w.last_crashed_pid = record["pid"]
        if w.crash_leaves == "alive":
            w.alive.add(record["pid"])
        elif w.crash_leaves == "port":
            w.open_ports.add(port)
        elif w.crash_leaves == "record":
            w.records.append(record)
        return result

    def stop(self, label, timeout=5):
        if label == fixture.PROCESS_LABEL and self.world.backend_stop_error:
            raise RuntimeError(self.world.backend_stop_error)
        if label == fixture.GIT_DAEMON_LABEL and self.world.daemon_group_survives:
            raise RuntimeError("leader stopped but process group still has members")
        if label == fixture.GIT_DAEMON_LABEL and self.world.daemon_stop_error:
            raise RuntimeError(self.world.daemon_stop_error)
        return self._retire(label, "stopped")


class World:
    """Offline stand-in for the fixture backend, Nomad and the worker."""

    def __init__(self, root_parent):
        self.root_parent = root_parent
        self.pid, self.records, self.alive, self.open_ports, self.starts, self.events = 1000, [], set(), set(), [], []
        self.marker_mode = "normal"
        self.phase, self.captured = "ready", True
        self.recover_mutation = None
        self.stop_leaves_alive = False
        self.stop_effect = False
        self.fence_survives_stop = True
        self.daemon_group_survives = False
        self.backend_stop_error = None
        self.killed_groups = []
        self.ws_failure = None
        self.workspace_read = {"ok": True, "contains_expected_file": True, "snapshots": []}
        self.checkout_heads = FIXTURE_HEAD + "\n"
        self.alloc_status = "running"
        self.fence = "owner:fence-content\n"
        self.tree_present = True
        self.job_index = 6
        self.record_ports = {}
        self.free_ports = iter(range(43111, 43200))
        self.bare_clones = []
        self.agent_profiles = [{"id": "agent1", "name": "inert"}]
        self.agent_create_profiles = [{"id": "inert-prof", "name": "nomad-fixture-inert"}]
        self.task_created = False
        self.api_calls, self.nomad_paths, self.ws_calls, self.ssh_commands = [], [], [], []
        self.bodies = []
        self.database = None
        self.barrier = None
        self.secret_value = None
        self.clock = FakeClock()
        self.state_overrides, self.exec_column = {}, EXEC_ID
        self.env_status, self.env_workspace, self.env_generation = "creating", "", 1
        self.session_state = "CREATED"
        self.crash_result, self.crash_leaves, self.last_crashed_pid, self.reuse_pid = "crashed", None, None, False
        self.node_status, self.node_eligibility, self.node_id_reported = "ready", "eligible", NODE
        self.job_status, self.allocations, self.alloc_node, self.task_state = "running", [{"ID": ALLOC}], NODE, "running"
        self.extra_listeners, self.listener_hosts_value = [], {"127.0.0.1"}
        self.ps_code, self.fence_code, self.head_code = 0, None, 0
        self.extra_message = False
        self.daemon_stop_error = None

    # backend side effects
    def on_start(self, env):
        self.database = env["KANDEV_DATABASE_PATH"]
        self.barrier = env.get(fixture.BARRIER_ENV)
        if self.task_created and not self.barrier:
            self._recover()

    def _db(self):
        return sqlite3.connect(self.database)

    def _create_task(self):
        self.task_created = True
        state = {
            "environment_id": ENV_ID, "generation": 1, "operation_id": EXEC_ID, "execution_id": EXEC_ID, "job_id": JOB,
            "job_create_index": 5, "job_definition_index": 6, "allocation_id": ALLOC, "node_id": NODE,
            "workspace_path": "/ws/environments/env/g1", "server_identity": "srv", "phase": self.phase, "workspace_input_captured": self.captured,
        }
        state.update(self.state_overrides)
        db = self._db()
        db.execute("CREATE TABLE executors_running (session_id TEXT, task_id TEXT, runtime TEXT, agent_execution_id TEXT, metadata TEXT)")
        db.execute("CREATE TABLE task_environments (id TEXT, status TEXT, workspace_path TEXT, ownership_generation INTEGER)")
        db.execute("CREATE TABLE task_environment_repos (id TEXT, task_environment_id TEXT)")
        db.execute("INSERT INTO executors_running VALUES ('sess1','task1','nomad',?,?)", (self.exec_column, json.dumps({"nomad_environment_state": state})))
        db.execute("INSERT INTO task_environments VALUES (?, ?, ?, ?)", (ENV_ID, self.env_status, self.env_workspace, self.env_generation))
        db.execute("INSERT INTO task_environment_repos VALUES ('r1', ?)", (ENV_ID,))
        db.commit()
        db.close()
        if self.barrier and self.marker_mode != "none":
            payload = {"execution_id": EXEC_ID, "environment_id": ENV_ID, "session_id": "sess1"}
            if self.marker_mode == "badkeys":
                payload["extra"] = 1
            if self.marker_mode == "othersession":
                payload["session_id"] = "other-session"
            Path(self.barrier).write_text(json.dumps(payload) + "\n")

    def _recover(self):
        db = self._db()
        db.execute("UPDATE task_environments SET status='ready', workspace_path='/ws/environments/env/g1'")
        db.commit()
        db.close()
        mutation = self.recover_mutation
        if mutation == "touch":
            self.extra_message = True
        elif mutation == "job":
            self.job_index = 99
        elif mutation == "fence":
            self.fence = "owner:other\n"
        elif mutation == "tree":
            self.tree_present = "changed"
        elif mutation == "server":
            db = self._db()
            row = json.loads(db.execute("SELECT metadata FROM executors_running").fetchone()[0])
            row["nomad_environment_state"]["server_identity"] = "other"
            db.execute("UPDATE executors_running SET metadata=?", (json.dumps(row),))
            db.commit()
            db.close()

    # injected callables
    def api_factory(self, port):
        def call(method, path, body=None):
            self.api_calls.append((method, path))
            if body is not None:
                self.bodies.append(json.dumps(body))
            if method == "GET" and path == "/workspaces":
                return {"workspaces": [{"id": "ws1"}]}
            if path == "/workspaces/ws1/workflows":
                return {"workflows": [{"id": "wf1"}]}
            if path == "/workflows/wf1/workflow/steps":
                return {"steps": [{"id": "step1"}]}
            if path == "/agents":
                return {"agents": [{"profiles": list(self.agent_profiles)}]}
            if method == "PUT" and path == "/github/workspace-settings":
                self.git_settings = body
                return {}
            if method == "PATCH" and path.startswith("/agent-profiles/"):
                self.profile_patch = (path, body)
                return {}
            if path == "/agents/tui":
                self.agent_created = body
                return {"id": "agent-inert", "profiles": list(self.agent_create_profiles)}
            if path == "/executors" and method == "GET":
                return {"executors": []}
            if path == "/executors":
                return {"id": EXECUTOR}
            if path == "/secrets":
                self.secret_value = body["value"]
                return {"id": "11111111-aaaa-4aaa-8aaa-111111111111"}
            if path == f"/executors/{EXECUTOR}/profiles":
                assert body["config"]["nomad_token"] == "$kandev-secret:11111111-aaaa-4aaa-8aaa-111111111111"
                return {"id": "prof1"}
            if path == "/workspaces/ws1/repositories/remote":
                return {"id": "repo1"}
            if path == "/workspaces/ws1/repositories":
                return {"id": "repo1"}
            if path == "/tasks" and method == "POST":
                self._create_task()
                return {"id": "task1", "session_id": "sess1"}
            if path == "/tasks/task1/sessions":
                return {"sessions": [{"id": "sess1", "state": self.session_state}]}
            if path == "/task-sessions/sess1/messages":
                messages = [{"id": "m1", "type": "status", "author_type": "agent", "turn_id": "t"}]
                if self.extra_message:
                    messages.append({"id": "m2", "type": "status", "author_type": "agent", "turn_id": "t2"})
                return {"messages": messages}
            raise AssertionError(f"unexpected API call {method} {path}")
        return call

    def nomad_factory(self, address, token, namespace, region):
        assert token == TOKEN

        def call(path):
            self.nomad_paths.append(path)
            if path == f"/v1/node/{NODE}":
                return {"ID": self.node_id_reported, "Status": self.node_status, "SchedulingEligibility": self.node_eligibility}
            if path == f"/v1/job/{JOB}":
                return {"ID": JOB, "Status": self.job_status, "CreateIndex": 5, "JobModifyIndex": self.job_index}
            if path == f"/v1/job/{JOB}/allocations":
                return list(self.allocations)
            if path == f"/v1/allocation/{ALLOC}":
                return {"ID": ALLOC, "NodeID": self.alloc_node, "ClientStatus": self.alloc_status, "TaskStates": {"agentctl": {"State": self.task_state}}}
            if path == "/v1/jobs?prefix=kandev-":
                return []
            raise AssertionError(f"unexpected Nomad read {path}")
        return call

    scanned = None

    def keyscan(self, host, port):
        self.scans = getattr(self, "scans", []) + [(host, port)]
        return ["worker.example ssh-ed25519 GOOD"] if self.scanned is None else self.scanned

    def key_fingerprint(self, line):
        return FINGERPRINT if line.endswith("GOOD") else "SHA256:" + "Z" * 43

    def ssh_factory(self, ssh):
        self.ssh_cfg = ssh
        assert ssh["identity_file"].endswith("/home/.ssh/id_ed25519")

        def call(command):
            self.ssh_commands.append(command)
            if "active-environment" in command:
                if self.fence_code is not None:
                    return self.fence_code, ""
                return (3, "") if self.fence is None else (0, self.fence)
            if "rev-parse HEAD" in command:
                return self.head_code, self.checkout_heads
            assert command.startswith("LC_ALL=C ps -eww")
            if self.ps_code:
                return self.ps_code, ""
            if not self.tree_present:
                return 0, ""
            started = "Sun Oct 25 10:00:00 2026" if self.tree_present is True else "Sun Oct 25 10:09:09 2026"
            return 0, f"  200 1 200 {started} /alloc/{ALLOC}/agentctl\n  201 200 200 {started} /alloc/{ALLOC}/acp\n  1 0 1 Sun Oct 25 09:00:00 2026 /sbin/init\n"
        return call

    def ws(self, payload):
        self.ws_calls.append(payload["mode"])
        if payload["mode"] == "workspace-read":
            if self.ws_failure:
                raise PreflightError("fixture WebSocket driver workspace-read failed")
            return self.workspace_read
        if self.stop_effect:
            self.alloc_status = "complete"
        if not self.fence_survives_stop:
            self.fence = None
        return {"ok": True}

    def kill_group(self, pgid):
        self.killed_groups.append(pgid)
        self.daemon_group_survives = False
        self.daemon_stop_error = None
        self.backend_stop_error = None

    def ops(self):
        return fixture.Ops(
            free_port=lambda: next(self.free_ports), git_binary=lambda: "/usr/bin/git", retention_settle=0, kill_group=self.kill_group,
            git_bare_clone=lambda src, dest: self.bare_clones.append((src, dest)), git_head=lambda path, ref: FIXTURE_HEAD, port_open=lambda port: port in self.open_ports, pid_alive=lambda pid: pid in self.alive,
            listener_pids=lambda port: [r["pid"] for r in self.records if self.record_ports.get(r["pid"]) == port] + self.extra_listeners,
            listener_hosts=lambda port: set(self.listener_hosts_value),
            registry=lambda root: FakeRegistry(self), api_factory=self.api_factory, nomad_factory=self.nomad_factory,
            keyscan=self.keyscan, key_fingerprint=self.key_fingerprint, ssh_factory=self.ssh_factory, ws=self.ws, clock=self.clock.clock, sleep=self.clock.sleep, wall=lambda: "2026-01-01T00:00:00+00:00",
        )


class FixtureBase(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="nomad-e2e-fixture-test-")
        self.addCleanup(self.temp.cleanup)
        self.tmp = Path(self.temp.name).resolve()
        self.checkout = self._dir("checkout")
        self.parent = self._dir("fixtures")
        bundle = self._dir("bundle/bin")
        for name in ("kandev", "agentctl", "agentctl-linux-amd64", "agentctl-linux-arm64", "agentctl-darwin-arm64", "agentctl-darwin-amd64"):
            (bundle / name).write_bytes(b"#!/bin/sh\n")
            (bundle / name).chmod(0o700)
        self.binary = bundle / "kandev"
        self.token_file = self._file("nomad.token", TOKEN + "\n")
        self.identity = self._file("id_ed25519", KEY_BODY + "\n")
        self.repo = self._dir("repo-fixture")
        (self.repo / ".git").mkdir()
        (self.repo / "README.md").write_text("hello")
        self.config = {
            "schema_version": 1, "kandev_binary": str(self.binary),
            "nomad": {"address": "http://127.0.0.1:4646", "token_file": str(self.token_file), "namespace": "default", "region": "global", "datacenter": "dc1", "node_id": NODE, "platform": "linux/amd64"},
            "ssh": {"target": "fixture@worker.example", "port": 22, "host_fingerprint": FINGERPRINT, "identity_file": str(self.identity)},
            "workspace_root": "/srv/kandev/workspace", "allocation_root": "/srv/kandev/nomad/alloc", "cpu_mhz": 500, "memory_mb": 512,
            "repository": {"path": str(self.repo), "base_branch": "main"}, "expected_file": "README.md", "temp_root_parent": str(self.parent),
        }
        self.world = World(self.parent)
        self.out, self.err = [], []

    def _dir(self, name):
        path = self.tmp / name
        path.mkdir(parents=True, mode=0o700)
        return path

    def _file(self, name, text):
        path = self.tmp / name
        path.write_text(text)
        path.chmod(0o600)
        return path

    def run_flow(self, config=None):
        return fixture.run_create_fixture(
            self.config if config is None else config, ops=self.world.ops(), out=self.out.append, err=self.err.append,
            run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout,
        )

    def validate(self, config=None):
        return bootstrap.validate_bootstrap(
            self.config if config is None else config, run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout,
        )

    def fixture_root(self):
        roots = [p for p in self.parent.iterdir() if p.name.startswith("nomad-e2e-fixture-")]
        self.assertEqual(len(roots), 1)
        return roots[0]

    def evidence(self):
        return json.loads((self.fixture_root() / "evidence.json").read_text())

    def everything_written(self):
        chunks = ["\n".join(self.out), "\n".join(self.err), "\n".join(self.world.bodies[:0])]
        for path in self.fixture_root().rglob("*"):
            if path.is_file() and path.name != "id_ed25519":
                chunks.append(path.read_text(errors="replace"))
        return "\n".join(chunks)


class BootstrapContractTest(FixtureBase):
    def test_valid_bootstrap_normalizes_without_reading_token_into_output(self):
        result = self.validate()
        self.assertEqual(result["nomad"]["address"], "http://127.0.0.1:4646")
        self.assertNotIn(TOKEN, json.dumps(result))
        self.assertEqual(bootstrap.read_token(Path(result["nomad"]["token_file"])), TOKEN)

    def test_key_set_is_strict_at_every_level(self):
        for path in ((), ("nomad",), ("ssh",), ("repository",)):
            for mutation in ("missing", "extra"):
                config = json.loads(json.dumps(self.config))
                target = config
                for key in path:
                    target = target[key]
                if mutation == "missing":
                    target.pop(next(k for k in target if k != "base_branch"))
                else:
                    target["unexpected"] = 1
                with self.assertRaises(PreflightError, msg=f"{path} {mutation}"):
                    self.validate(config)

    def test_optional_agent_profile_name_is_accepted(self):
        self.config["agent_profile_name"] = "inert"
        self.assertEqual(self.validate()["agent_profile_name"], "inert")
        self.config["agent_profile_name"] = " "
        with self.assertRaises(PreflightError):
            self.validate()

    def test_repository_inside_checkout_is_refused(self):
        inside = self.checkout / "fixtures-repo"
        (inside / ".git").mkdir(parents=True)
        (inside / "README.md").write_text("x")
        self.config["repository"]["path"] = str(inside)
        with self.assertRaisesRegex(PreflightError, "outside the Kandev checkout"):
            self.validate()

    def test_temp_root_parent_inside_checkout_or_git_repo_is_refused(self):
        inside = self.checkout / "tmp"
        inside.mkdir()
        self.config["temp_root_parent"] = str(inside)
        with self.assertRaisesRegex(PreflightError, "outside the Kandev checkout"):
            self.validate()
        other = self._dir("other-repo")
        (other / ".git").mkdir()
        nested = other / "nested"
        nested.mkdir(mode=0o700)
        self.config["temp_root_parent"] = str(nested)
        with self.assertRaisesRegex(PreflightError, "git repository"):
            self.validate()

    def test_temp_root_parent_must_not_be_group_writable(self):
        self.parent.chmod(0o770)
        with self.assertRaises(PreflightError):
            self.validate()

    def test_nomad_address_contract(self):
        for good in ("http://127.0.0.1:4646", "http://localhost:4646", "https://nomad.example.com:4646", "https://10.1.2.3:4646/"):
            bootstrap.nomad_address(good)
        for bad in ("http://nomad.example.com:4646", "http://10.1.2.3:4646", "https://nomad.example.com", "ftp://127.0.0.1:1", "https://" + "u" + ":" + "p" + "@nomad.example.com:4646",
                    "https://nomad.example.com:4646/v1", "https://nomad.example.com:4646?x=1", "nomad.example.com:4646", "https://nomad.example.com:99999", 5):
            with self.assertRaises(PreflightError, msg=str(bad)):
                bootstrap.nomad_address(bad)

    def test_token_and_identity_files_must_be_private(self):
        self.token_file.chmod(0o640)
        with self.assertRaises(PreflightError):
            self.validate()
        self.token_file.chmod(0o600)
        self.identity.chmod(0o644)
        with self.assertRaises(PreflightError):
            self.validate()

    def test_token_file_must_hold_a_single_token(self):
        self.token_file.write_text("two words\n")
        with self.assertRaises(PreflightError):
            bootstrap.read_token(self.token_file)

    def test_other_invalid_values(self):
        cases = (
            (("ssh", "host_fingerprint"), "MD5:aa"), (("ssh", "target"), "-oProxyCommand=x"), (("ssh", "port"), 0),
            (("nomad", "node_id"), "not-a-uuid"), (("nomad", "platform"), "windows/amd64"), (("nomad", "namespace"), "bad ns"),
            (("workspace_root",), "relative/path"), (("workspace_root",), "/a/../b"), (("allocation_root",), "/a"), (("cpu_mhz",), True),
            (("memory_mb",), 0), (("expected_file",), "/etc/passwd"), (("expected_file",), "../x"), (("expected_file",), "missing.txt"),
            (("repository", "base_branch"), "-bad"), (("schema_version",), 2), (("kandev_binary",), "relative/kandev"),
        )
        for path, value in cases:
            config = json.loads(json.dumps(self.config))
            target = config
            for key in path[:-1]:
                target = target[key]
            target[path[-1]] = value
            with self.assertRaises(PreflightError, msg=str(path)):
                self.validate(config)

    def test_repository_url_form(self):
        self.config["repository"] = {"url": "https://example.com/org/repo.git", "base_branch": "main"}
        self.assertIn("url", self.validate()["repository"])
        self.config["repository"] = {"url": "https://" + "user" + ":" + "pw" + "@example.com/org/repo.git", "base_branch": "main"}
        with self.assertRaises(PreflightError):
            self.validate()
        self.config["repository"] = {"path": str(self.repo), "url": "https://example.com/x", "base_branch": "main"}
        with self.assertRaises(PreflightError):
            self.validate()

    def test_runtime_bundle_must_be_complete(self):
        (self.binary.parent / "agentctl-linux-arm64").unlink()
        with self.assertRaises(PreflightError):
            self.validate()


class CreateFlowTest(FixtureBase):
    def test_live_port_is_refused_before_anything_is_created(self):
        ops = self.world.ops()
        ops.free_port = lambda: 8817
        code = fixture.run_create_fixture(self.config, ops=ops, out=self.out.append, err=self.err.append, run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assertEqual(code, 2)
        self.assertIn("live Kandev port", "\n".join(self.err))
        self.assertEqual(list(self.parent.iterdir()), [])
        self.assertEqual(self.world.starts, [])

    def test_invalid_bootstrap_creates_nothing_and_prints_no_cleanup_command(self):
        self.config["unexpected"] = 1
        self.assertEqual(self.run_flow(), 2)
        self.assertEqual(list(self.parent.iterdir()), [])
        self.assertNotIn("cleanup-fixture", "\n".join(self.out + self.err))

    def test_successful_flow_writes_evidence_and_never_leaks_secrets(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        root = self.fixture_root()
        self.assertEqual(root.stat().st_mode & 0o777, 0o700)
        evidence = self.evidence()
        self.assertEqual(evidence["status"], "passed")
        self.assertEqual([s["status"] for s in evidence["steps"]], ["passed"] * len(evidence["steps"]))
        self.assertEqual([s["name"] for s in evidence["steps"]], [
            "start-git-daemon", "start-backend-with-barrier", "create-fixture-objects", "await-pre-publish-barrier", "snapshot-before-crash", "crash-backend",
            "restart-backend-without-barrier", "await-recovery", "assert-identity-preserved", "workspace-read-before-any-prompt", "session-stop-and-teardown",
        ])
        self.assertEqual((evidence["job_id"], evidence["allocation_id"], evidence["node_id"], evidence["execution_id"]), (JOB, ALLOC, NODE, EXEC_ID))
        self.assertEqual(evidence["environment_status"], {"before": "creating", "after": "ready"})
        self.assertEqual(evidence["inventory_phase"], {"before": "ready", "after": "ready"})
        cleanup = evidence["cleanup"]
        self.assertIs(cleanup["production_cleanup"], False)
        self.assertEqual(cleanup["allocation_state_after_stop"], "running")
        self.assertEqual(cleanup["job_status_after_stop"], "running")
        self.assertTrue(cleanup["fence_retained"])
        self.assertEqual(cleanup["fence_sha256_after_stop"], evidence["fence_sha256"])
        self.assertEqual(cleanup["remote_processes_retained"], 2)
        teardown = cleanup["operator_teardown_required"]
        self.assertEqual((teardown["job_id"], teardown["allocation_id"]), (JOB, ALLOC))
        self.assertTrue(teardown["fence_path"].endswith("/active-environment"))
        before, after = evidence["identities"]["before"], evidence["identities"]["after"]
        self.assertEqual(before["environment_status"], "creating")
        self.assertEqual(after["environment_status"], "ready")
        for key in ("execution_id", "job_id", "job_create_index", "job_modify_index", "allocation_id", "node_id", "fence_owner_sha256", "remote_process_count"):
            self.assertEqual(before[key], after[key], key)
            self.assertIsNotNone(before[key], key)
        self.assertEqual((before["job_create_index"], before["job_modify_index"]), (5, 6))
        self.assertEqual(evidence["workspace_read"], {"ok": True, "contains_expected_file": True, "repo_head": FIXTURE_HEAD})
        self.assertTrue(all("/owner" in c for c in self.world.ssh_commands if "active-environment" in c))
        self.assertEqual(evidence["prompts_sent"], 0)
        self.assertTrue(any(line == f"evidence: {root / 'evidence.json'}" for line in self.out))
        text = self.everything_written()
        self.assertNotIn(TOKEN, text)
        self.assertNotIn(KEY_BODY, text)
        self.assertNotIn(str(self.identity), text)
        self.assertNotIn(str(self.token_file), evidence and json.dumps(evidence))
        self.assertEqual(self.world.secret_value, TOKEN)
        self.assertFalse(any(TOKEN in body for body in self.world.bodies if "nomad_token" in body))

    def test_process_and_port_contract(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        first, second = [item for item in self.world.starts if item["argv"][1:2] == ["__backend"]]
        self.assertIn(fixture.BARRIER_ENV, first["env"])
        self.assertTrue(first["env"][fixture.BARRIER_ENV].startswith(str(self.fixture_root())))
        self.assertNotIn(fixture.BARRIER_ENV, second["env"])
        for start in (first, second):
            self.assertEqual(start["env"][fixture.FIXTURE_ONLY_ENV], "1")
            self.assertEqual(start["env"]["KANDEV_SERVER_PORT"], "43111")
            self.assertNotEqual(start["env"]["KANDEV_SERVER_PORT"], "8817")
            self.assertEqual(start["argv"], [str(self.binary), "__backend"])
            self.assertEqual(start["cwd"], str(self.fixture_root() / "home"))
        self.assertEqual(self.world.events, ["crashed", "stopped", "stopped"])
        self.assertEqual(self.world.records, [])
        self.assertEqual(self.world.open_ports, set())

    def test_launch_manifest_is_consistent_with_the_attached_verifier(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        root = self.fixture_root()
        base._verify_backend_launch_manifest(root, root / "home", {"binary": str(self.binary), "port": 43111})

    def test_materialized_identity_copy_is_private_and_exact(self):
        root = self._dir("materialize-root")
        fixture._materialize(self.validate(), root, 43111)
        key = root / "home" / ".ssh" / "id_ed25519"
        self.assertEqual(key.stat().st_mode & 0o777, 0o600)
        self.assertEqual(key.read_text(), KEY_BODY + "\n")

    def test_runner_only_reads_nomad_and_never_stops_jobs(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertTrue(self.world.nomad_paths)
        self.assertTrue(all(path.startswith("/v1/") and not path.startswith("/v1/job/" + JOB + "/") or path.endswith("/allocations") for path in self.world.nomad_paths))
        source = (Path(fixture.__file__).read_text() + Path(base.__file__).read_text()).lower()
        self.assertNotIn("job stop", source)
        self.assertNotIn("deregister", source)

    def test_workspace_read_happens_before_the_production_stop(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertEqual(self.world.ws_calls, ["workspace-read", "session-stop"])

    def test_profile_is_created_through_the_payload_builder_with_a_secret_reference(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertIn(("POST", "/secrets"), self.world.api_calls)
        profile = json.loads(next(b for b in self.world.bodies if "ssh_host_fingerprint" in b))
        self.assertEqual(profile["config"]["cpu_mhz"], "500")
        self.assertTrue(profile["config"]["nomad_token"].startswith("$kandev-secret:"))
        task = json.loads(next(b for b in self.world.bodies if "prepare_session" in b))
        self.assertTrue(task["prepare_session"])
        self.assertNotIn("start_agent", task)


class AgentProfileSelectionTest(FixtureBase):
    def task_body(self):
        return json.loads(next(b for b in self.world.bodies if "prepare_session" in b))

    def test_zero_profiles_creates_the_inert_acp_agent_and_uses_its_profile(self):
        self.world.agent_profiles = []
        self.assertEqual(self.run_flow(), 0, self.err)
        created = self.world.agent_created
        self.assertEqual(created["protocol"], "acp")
        self.assertEqual(created["command"], "python3")
        self.assertEqual(created["command_args"][0], str(fixture.INERT_ACP_SCRIPT))
        self.assertEqual(created["command_args"][1], "--identity-dir")
        self.assertTrue(created["command_args"][2].endswith("inert-identity"))
        self.assertTrue(fixture.INERT_ACP_SCRIPT.is_file())
        self.assertEqual(created["display_name"], fixture.INERT_AGENT_NAME)
        self.assertEqual(self.task_body()["agent_profile_id"], "inert-prof")

    def test_zero_matching_profiles_creates_the_agent_under_the_requested_name(self):
        self.config["agent_profile_name"] = "other"
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertEqual(self.world.agent_created["display_name"], "other")

    def test_exactly_one_profile_is_used_without_creating_an_agent(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertNotIn(("POST", "/agents/tui"), self.world.api_calls)
        self.assertEqual(self.task_body()["agent_profile_id"], "agent1")

    def test_many_profiles_without_a_name_fail_without_creating_anything(self):
        self.world.agent_profiles = [{"id": "a", "name": "x"}, {"id": "b", "name": "y"}]
        self.assertEqual(self.run_flow(), 2)
        self.assertNotIn(("POST", "/agents/tui"), self.world.api_calls)
        self.assertNotIn(("POST", "/tasks"), self.world.api_calls)

    def test_a_name_selects_one_of_many_profiles(self):
        self.world.agent_profiles = [{"id": "a", "name": "x"}, {"id": "b", "name": "y"}]
        self.config["agent_profile_name"] = "y"
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertEqual(self.task_body()["agent_profile_id"], "b")

    def test_inert_agent_creation_without_exactly_one_profile_fails_closed(self):
        self.world.agent_profiles = []
        self.world.agent_create_profiles = []
        self.assertNotEqual(self.run_flow(), 0)
        self.assertNotIn(("POST", "/tasks"), self.world.api_calls)


class GitProviderRedirectTest(FixtureBase):
    def test_local_repository_is_registered_with_a_provider_origin_served_by_an_owned_daemon(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        root = self.fixture_root()
        self.assertEqual(self.world.bare_clones, [(str(self.repo), str(root / "gitserve" / "fixture-repo.git"))])
        daemon = next(item for item in self.world.starts if item["argv"][1:2] == ["daemon"])
        self.assertEqual(daemon["argv"][0], "/usr/bin/git")
        self.assertIn("--listen=127.0.0.1", daemon["argv"])
        self.assertIn("--export-all", daemon["argv"])
        port = int(next(a for a in daemon["argv"] if a.startswith("--port=")).split("=")[1])
        repo = json.loads(next(b for b in self.world.bodies if '"local_path"' in b))
        self.assertEqual((repo["provider"], repo["provider_owner"], repo["provider_name"]), ("github", "fixture-owner", "fixture-repo"))
        self.assertEqual(self.world.git_settings["task_git_credentials_mode"], "executor")
        path, body = self.world.profile_patch
        self.assertEqual(path, "/agent-profiles/agent1")
        env = {item["key"]: item["value"] for item in body["env_vars"]}
        self.assertEqual(env, {
            "GIT_CONFIG_COUNT": "1",
            "GIT_CONFIG_KEY_0": f"url.git://127.0.0.1:{port}/fixture-repo.git.insteadOf",
            "GIT_CONFIG_VALUE_0": "https://github.com/fixture-owner/fixture-repo.git",
        })

    def test_backend_and_manifest_env_isolate_git_config(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        root = self.fixture_root()
        backend = next(item for item in self.world.starts if item["argv"][1:2] == ["__backend"])
        self.assertEqual(backend["env"]["GIT_CONFIG_NOSYSTEM"], "1")
        self.assertEqual(backend["env"]["GIT_CONFIG_GLOBAL"], str(root / "gitconfig"))
        self.assertEqual((root / "gitconfig").read_text(), "")
        manifest = json.loads((root / "home" / "supervisor" / "launch.json").read_text())
        self.assertEqual(manifest["env"]["GIT_CONFIG_GLOBAL"], str(root / "gitconfig"))

    def test_daemon_is_stopped_with_the_backend_on_teardown(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertEqual(self.world.records, [])
        self.assertEqual(self.world.open_ports, set())
        self.assertEqual(self.world.events.count("stopped"), 2)

    def test_forked_daemon_children_are_killed_through_the_owned_process_group(self):
        self.world.daemon_group_survives = True
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertEqual(len(self.world.killed_groups), 1)
        daemon_pid = self.evidence()["cleanup"]["git_daemon_pid_stopped"]
        self.assertEqual(self.world.killed_groups, [daemon_pid])
        self.assertEqual(self.world.records, [])

    def test_daemon_group_is_not_killed_when_the_owned_stop_succeeds(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertEqual(self.world.killed_groups, [])

    def test_daemon_that_survives_teardown_fails_cleanup(self):
        self.world.stop_leaves_alive = True
        self.assertNotEqual(self.run_flow(), 0)

    def test_url_repository_needs_no_daemon_or_redirect(self):
        self.config["repository"] = {"url": "https://example.invalid/o/r.git", "base_branch": "main"}
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertEqual(self.world.bare_clones, [])
        self.assertFalse(any(item["argv"][1:2] == ["daemon"] for item in self.world.starts))
        self.assertFalse(hasattr(self.world, "profile_patch"))


class FailureFlowTest(FixtureBase):
    def assertFailedLeavingFixture(self, step, message=None):
        self.assertEqual(self.run_flow(), 2)
        evidence = self.evidence()
        self.assertEqual(evidence["status"], "failed")
        self.assertEqual(evidence["failed_step"], step)
        if message:
            self.assertIn(message, evidence["error"])
        root = self.fixture_root()
        self.assertIn(f"operator cleanup: python3 scripts/nomad_native_e2e.py cleanup-fixture --fixture-root {root}", self.out)
        self.assertNotIn(TOKEN, self.everything_written())
        return evidence

    def test_marker_never_appears_is_a_bounded_failure_without_cleanup(self):
        self.world.marker_mode = "none"
        self.assertFailedLeavingFixture("await-pre-publish-barrier", "did not appear")
        self.assertLessEqual(self.world.clock.now, fixture.MARKER_TIMEOUT + 1)
        self.assertEqual(self.world.ws_calls, [])
        self.assertEqual(self.world.events, [])
        self.assertEqual(sorted(r["label"] for r in self.world.records), [fixture.GIT_DAEMON_LABEL, fixture.PROCESS_LABEL])

    def test_malformed_marker_is_a_failure(self):
        self.world.marker_mode = "badkeys"
        self.assertFailedLeavingFixture("await-pre-publish-barrier", "exactly execution_id")

    def test_inventory_phase_wrong_fails_before_the_crash(self):
        self.world.phase = "preparing"
        self.assertFailedLeavingFixture("await-pre-publish-barrier", "phase ready")
        self.assertEqual(self.world.events, [])

    def test_inventory_input_not_captured_fails_before_the_crash(self):
        self.world.captured = False
        self.assertFailedLeavingFixture("await-pre-publish-barrier", "workspace input captured")
        self.assertEqual(self.world.events, [])

    def test_identity_changes_after_restart_are_detected_and_nothing_is_cleaned(self):
        for mutation, message in (("job", "Nomad job or allocation"), ("fence", "worker fence"), ("tree", "process tree"), ("server", "execution, job, allocation, or server")):
            with self.subTest(mutation):
                self.setUp()
                self.world.recover_mutation = mutation
                self.assertFailedLeavingFixture("assert-identity-preserved", message)
                self.assertEqual(self.world.ws_calls, [])
                self.assertEqual(self.world.events, ["crashed"])
                self.assertEqual(sorted(r["label"] for r in self.world.records), [fixture.GIT_DAEMON_LABEL, fixture.PROCESS_LABEL])
                self.temp.cleanup()

    def test_workspace_read_failure_leaves_the_fixture(self):
        self.world.ws_failure = True
        self.assertFailedLeavingFixture("workspace-read-before-any-prompt", "workspace-read failed")
        self.assertEqual(self.world.ws_calls, ["workspace-read"])
        self.assertEqual(self.world.events, ["crashed"])

    def test_workspace_read_without_repo_head_fails(self):
        self.world.checkout_heads = ""
        self.assertFailedLeavingFixture("workspace-read-before-any-prompt", "exactly one repository checkout HEAD")

    def test_workspace_head_asserted_when_git_snapshots_are_empty(self):
        self.assertEqual(self.world.workspace_read["snapshots"], [])
        self.world.checkout_heads = "b" * 40 + "\n"
        self.assertFailedLeavingFixture("workspace-read-before-any-prompt", "does not match")

    def test_workspace_head_with_two_checkouts_fails(self):
        self.world.checkout_heads = FIXTURE_HEAD + "\n" + FIXTURE_HEAD + "\n"
        self.assertFailedLeavingFixture("workspace-read-before-any-prompt", "exactly one repository checkout HEAD")

    def test_workspace_read_with_wrong_repo_head_fails(self):
        self.world.checkout_heads = "b" * 40 + "\n"
        self.assertFailedLeavingFixture("workspace-read-before-any-prompt", "does not match")

    def test_workspace_read_missing_expected_file_fails(self):
        self.world.workspace_read = {"ok": True, "contains_expected_file": False}
        self.assertFailedLeavingFixture("workspace-read-before-any-prompt", "expected file")

    def test_recovery_that_never_becomes_ready_times_out(self):
        self.world.recover_mutation = None
        original = self.world._recover
        self.world._recover = lambda: None
        self.assertFailedLeavingFixture("await-recovery", "did not become ready")
        self.assertLessEqual(self.world.clock.now, 130)
        self.world._recover = original

    def test_cleanup_failures_fail_loudly(self):
        cases = (
            ("allocation ended by session stop", {"stop_effect": True}, "retention contract"),
            ("fence released by session stop", {"fence_survives_stop": False}, "retention contract"),
            ("process still alive", {"stop_leaves_alive": True}, "still alive"),
        )
        for name, attrs, message in cases:
            with self.subTest(name):
                self.setUp()
                for key, value in attrs.items():
                    setattr(self.world, key, value)
                self.assertFailedLeavingFixture("session-stop-and-teardown", message)
                self.assertEqual(self.world.ws_calls.count("session-stop"), 1)
                self.temp.cleanup()

    # -- runner branches: every failure stops the run without retrying or prompting ------------------------------
    def assertLeft(self, step, message, *, events=(), labels=None):
        self.assertFailedLeavingFixture(step, message)
        self.assertEqual(self.world.ws_calls, [])
        self.assertEqual(self.world.events, list(events))
        if labels is None:
            labels = [fixture.GIT_DAEMON_LABEL, fixture.PROCESS_LABEL]
        self.assertEqual(sorted(r["label"] for r in self.world.records), sorted(labels))

    def test_barrier_marker_for_a_different_session_fails(self):
        self.world.marker_mode = "othersession"
        self.assertLeft("await-pre-publish-barrier", "different session")

    def test_inventory_identity_mismatching_the_marker_fails(self):
        cases = (
            ("execution_id", {"exec_column": "other-exec"}),
            ("operation_id", {"state_overrides": {"operation_id": "other-op"}}),
            ("environment_id", {"state_overrides": {"environment_id": "other-env"}}),
        )
        for name, attrs in cases:
            with self.subTest(name):
                self.setUp()
                for key, value in attrs.items():
                    setattr(self.world, key, value)
                self.assertLeft("await-pre-publish-barrier", "does not match the barrier marker")
                self.temp.cleanup()

    def test_environment_not_creating_or_with_a_workspace_fails_before_the_crash(self):
        for attrs in ({"env_status": "ready"}, {"env_workspace": "/ws/already"}):
            with self.subTest(attrs):
                self.setUp()
                for key, value in attrs.items():
                    setattr(self.world, key, value)
                self.assertLeft("snapshot-before-crash", "creating state with an empty workspace path")
                self.temp.cleanup()

    def test_session_not_created_before_the_crash_fails(self):
        self.world.session_state = "RUNNING"
        self.assertLeft("snapshot-before-crash", "not in the CREATED state")

    def test_empty_remote_tree_or_missing_fence_before_the_crash_fails(self):
        for attrs in ({"tree_present": False}, {"fence": None}):
            with self.subTest(attrs):
                self.setUp()
                for key, value in attrs.items():
                    setattr(self.world, key, value)
                self.assertLeft("snapshot-before-crash", "tree or worker fence is missing")
                self.temp.cleanup()

    def test_ownership_generation_differing_from_the_inventory_fails(self):
        self.world.env_generation = 2
        self.assertLeft("snapshot-before-crash", "generation differs")

    def test_nomad_snapshot_negatives_fail_before_the_crash(self):
        cases = (
            ("job id pattern", {"state_overrides": {"job_id": "not-a-kandev-job"}}, "kandev-<hash> pattern"),
            ("node not ready", {"node_status": "down"}, "ready and eligible"),
            ("node ineligible", {"node_eligibility": "ineligible"}, "ready and eligible"),
            ("node identity", {"node_id_reported": "99999999-9999-4999-8999-999999999999"}, "ready and eligible"),
            ("job not running", {"job_status": "dead"}, "not the running job"),
            ("extra allocation", {"allocations": [{"ID": ALLOC}, {"ID": "other-alloc"}]}, "exactly the inventory allocation"),
            ("wrong allocation", {"allocations": [{"ID": "other-alloc"}]}, "exactly the inventory allocation"),
            ("allocation not running", {"alloc_status": "pending"}, "not running on the inventory node"),
            ("allocation on another node", {"alloc_node": "other-node"}, "not running on the inventory node"),
            ("agentctl not running", {"task_state": "dead"}, "agentctl task is not running"),
        )
        for name, attrs, message in cases:
            with self.subTest(name):
                self.setUp()
                for key, value in attrs.items():
                    setattr(self.world, key, value)
                self.assertLeft("snapshot-before-crash", message)
                self.temp.cleanup()

    def test_crash_not_reported_as_crashed_fails(self):
        self.world.crash_result = "already_absent"
        self.assertLeft("crash-backend", "was not crashed through its exact owned process record")

    def test_backend_remnants_after_the_crash_fail_verify_gone(self):
        cases = (("alive", "still alive"), ("port", "port is still open"), ("record", "was not retired"))
        for leaves, message in cases:
            with self.subTest(leaves):
                self.setUp()
                self.world.crash_leaves = leaves
                self.assertFailedLeavingFixture("crash-backend", message)
                self.assertEqual(self.world.ws_calls, [])
                self.temp.cleanup()

    def test_restarted_backend_reusing_the_crashed_pid_fails(self):
        self.world.reuse_pid = True
        self.assertLeft("restart-backend-without-barrier", "reused the crashed process identity", events=["crashed"], labels=[fixture.GIT_DAEMON_LABEL, fixture.PROCESS_LABEL])

    def test_start_guard_port_already_in_use(self):
        self.world.open_ports.add(43111)
        self.assertLeft("start-backend-with-barrier", "already in use", labels=[fixture.GIT_DAEMON_LABEL])

    def test_start_guard_listener_that_is_not_exactly_the_registered_child(self):
        self.world.extra_listeners = [9999]
        self.assertLeft("start-backend-with-barrier", "not exactly the registered Kandev child")

    def test_start_guard_refuses_the_live_port_without_starting_anything(self):
        runner = fixture.Runner.__new__(fixture.Runner)
        runner.state, runner.ops = {"port": bootstrap.LIVE_KANDEV_PORT}, self.world.ops()
        with self.assertRaisesRegex(PreflightError, "live Kandev port"):
            runner.start_backend(None)
        self.assertEqual(self.world.starts, [])

    def test_session_touched_during_recovery_fails(self):
        self.world.recover_mutation = "touch"
        self.assertLeft("await-recovery", "changed the durable message identities", events=["crashed"], labels=[fixture.GIT_DAEMON_LABEL, fixture.PROCESS_LABEL])

    def test_workspace_head_read_with_a_failing_ssh_exit_code_fails(self):
        self.world.head_code = 1
        self.assertFailedLeavingFixture("workspace-read-before-any-prompt", "workspace repository HEAD read failed")
        self.assertEqual(self.world.ws_calls, ["workspace-read"])

    def test_remote_tree_read_error_fails_before_the_crash(self):
        self.world.ps_code = 255
        self.assertLeft("snapshot-before-crash", "remote process listing failed")

    def test_remote_fence_read_error_fails_before_the_crash(self):
        self.world.fence_code = 255
        self.assertLeft("snapshot-before-crash", "remote fence read failed")

    def test_secrets_in_error_text_are_scrubbed(self):
        evidence = fixture.Evidence(self.parent, [TOKEN])
        self.assertNotIn(TOKEN, evidence.scrub(f"boom {TOKEN} boom"))


class HostKeyPinningTest(FixtureBase):
    def test_matching_key_is_written_alone_and_used_strictly(self):
        self.world.scanned = ["worker.example ssh-ed25519 OTHER", "worker.example ssh-ed25519 GOOD"]
        self.assertEqual(self.run_flow(), 0, self.err)
        known = (self.fixture_root() / "known_hosts")
        self.assertEqual(known.read_text(), "worker.example ssh-ed25519 GOOD\n")
        self.assertEqual(known.stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.world.ssh_cfg["known_hosts"], str(known))
        self.assertEqual(self.world.scans[0], ("worker.example", 22))

    def test_default_ssh_command_is_strict(self):
        calls = []
        original = fixture.subprocess.run
        fixture.subprocess.run = lambda cmd, **kw: calls.append(cmd) or SimpleNamespace(returncode=0, stdout="", stderr="")
        try:
            fixture._default_ssh({"identity_file": "/k", "known_hosts": "/kh", "port": 22, "target": "u@h"})("true")
        finally:
            fixture.subprocess.run = original
        cmd = calls[0]
        self.assertIn("StrictHostKeyChecking=yes", cmd)
        self.assertIn("UserKnownHostsFile=/kh", cmd)
        self.assertIn("BatchMode=yes", cmd)
        self.assertFalse(any("DEBUG" in part or "=no" in part for part in cmd))

    def test_mismatch_or_empty_scan_fails_before_any_remote_command(self):
        for scanned in (["worker.example ssh-ed25519 OTHER"], []):
            self.setUp()
            self.world.scanned = scanned
            self.assertEqual(self.run_flow(), 2)
            self.assertEqual(self.world.ssh_commands, [])
            self.assertIn("pinned fingerprint", "\n".join(self.err))
            self.temp.cleanup()


class CleanupFixtureTest(FixtureBase):
    def cleanup(self, root=None):
        return fixture.run_cleanup_fixture(str(root or self.fixture_root()), ops=self.world.ops(), out=self.out.append, err=self.err.append)

    def test_cleanup_after_an_assertion_failure_uses_the_production_stop_path(self):
        self.world.recover_mutation = "fence"
        self.assertEqual(self.run_flow(), 2)
        self.world.fence = "owner:fence-content\n"
        self.out.clear()
        self.assertEqual(self.cleanup(), 0, self.err)
        evidence = self.evidence()
        self.assertEqual(evidence["status"], "failed")
        self.assertEqual(evidence["cleanup"]["status"], "passed")
        self.assertEqual(self.world.ws_calls, ["session-stop"])
        self.assertEqual(self.world.records, [])
        self.assertEqual(self.world.alloc_status, "running")
        self.assertIs(evidence["cleanup"]["production_cleanup"], False)
        self.assertNotIn(TOKEN, self.everything_written())

    def test_cleanup_starts_the_backend_first_when_it_is_down(self):
        self.world.marker_mode = "none"
        self.assertEqual(self.run_flow(), 2)
        FakeRegistry(self.world).crash(fixture.PROCESS_LABEL)
        self.world.open_ports.clear()
        starts_before = len(self.world.starts)
        self.assertEqual(self.cleanup(), 0, self.err)
        self.assertEqual(len(self.world.starts), starts_before + 1)
        self.assertNotIn(fixture.BARRIER_ENV, self.world.starts[-1]["env"])

    def test_cleanup_failure_fails_loudly_once_and_reprints_the_command(self):
        self.world.recover_mutation = "tree"
        self.assertEqual(self.run_flow(), 2)
        self.world.stop_leaves_alive = True
        self.out.clear()
        self.assertEqual(self.cleanup(), 2)
        self.assertEqual(self.world.ws_calls.count("session-stop"), 1)
        self.assertTrue(any(line.startswith("operator cleanup: python3 scripts/nomad_native_e2e.py cleanup-fixture") for line in self.out))
        self.assertEqual(self.evidence()["cleanup"]["status"], "failed")

    def test_cleanup_fixture_records_but_does_not_fail_on_a_released_allocation(self):
        self.world.recover_mutation = "tree"
        self.assertEqual(self.run_flow(), 2)
        self.world.stop_effect, self.world.fence_survives_stop = True, False
        self.assertEqual(self.cleanup(), 0, self.err)
        cleanup = self.evidence()["cleanup"]
        self.assertEqual(cleanup["allocation_state_after_stop"], "complete")
        self.assertFalse(cleanup["fence_retained"])

    def test_cleanup_without_a_fixture_state_fails_before_acting(self):
        empty = self._dir("empty-root")
        self.assertEqual(self.cleanup(empty), 2)
        self.assertEqual(self.world.ws_calls, [])
        self.assertEqual(self.world.starts, [])

    def test_cli_wiring(self):
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            self.assertEqual(main(["cleanup-fixture", "--fixture-root", str(self.tmp / "missing")]), 2)
            self.assertEqual(main(["run", "--scenario", "session", "--require-real", "--create-fixture", "--config", "x"]), 2)
            self.assertEqual(main(["run", "--scenario", "pre-ready-recovery", "--create-fixture", "--config", "x"]), 2)
            self.assertEqual(main(["run", "--scenario", "pre-ready-recovery", "--require-real", "--create-fixture", "--config", str(self.tmp / "nope.json")]), 2)
        self.assertIn("cleanup-fixture failed before any action", err.getvalue())
        self.assertIn("--create-fixture requires", err.getvalue())
        self.assertIn("config is unreadable", err.getvalue())


class LoopbackBindTest(FixtureBase):
    def test_backend_and_manifest_env_bind_loopback_only(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        backends = [item for item in self.world.starts if item["argv"][1:2] == ["__backend"]]
        self.assertEqual(len(backends), 2)
        for start in backends:
            self.assertEqual(start["env"]["KANDEV_SERVER_HOST"], "127.0.0.1")
        root = self.fixture_root()
        manifest = json.loads((root / "home" / "supervisor" / "launch.json").read_text())
        self.assertEqual(manifest["env"]["KANDEV_SERVER_HOST"], "127.0.0.1")
        base._verify_backend_launch_manifest(root, root / "home", {"binary": str(self.binary), "port": 43111})

    def test_launch_env_overrides_any_other_host(self):
        runner = SimpleNamespace(state={"manifest_env": {"KANDEV_SERVER_HOST": "0.0.0.0"}, "bundle": "/b"})
        self.assertEqual(fixture.Runner.launch_env(runner, None)["KANDEV_SERVER_HOST"], "127.0.0.1")

    def test_listener_not_bound_to_loopback_only_fails_the_step(self):
        for hosts in ({"0.0.0.0"}, {"127.0.0.1", "0.0.0.0"}, {"*"}, set()):
            with self.subTest(hosts=hosts):
                self.setUp()
                self.world.listener_hosts_value = hosts
                self.assertEqual(self.run_flow(), 2)
                self.assertEqual(self.evidence()["failed_step"], "start-backend-with-barrier")
                self.assertIn("loopback", self.evidence()["error"])
                self.assertEqual(self.world.ws_calls, [])
                self.temp.cleanup()

    def test_default_listener_hosts_parses_lsof_machine_output(self):
        original = fixture.subprocess.run
        output = "p123\nn127.0.0.1:43111\nn[::1]:43111\np123\nn*:43111\n"
        fixture.subprocess.run = lambda cmd, **kw: SimpleNamespace(returncode=0, stdout=output, stderr="")
        try:
            self.assertEqual(fixture._default_listener_hosts(43111), {"127.0.0.1", "::1", "*"})
        finally:
            fixture.subprocess.run = original


class DaemonStopOwnershipTest(FixtureBase):
    def test_identity_change_refusal_never_signals_the_group(self):
        self.world.daemon_stop_error = "PID start identity or process group changed; refusing to signal"
        self.assertEqual(self.run_flow(), 2)
        self.assertEqual(self.world.killed_groups, [])
        self.assertEqual(self.evidence()["failed_step"], "session-stop-and-teardown")
        self.assertIn("refusing to signal", self.evidence()["error"])

    def test_wait_time_identity_change_never_signals_the_group(self):
        self.world.daemon_stop_error = "PID identity changed while waiting; retaining ownership record"
        self.assertEqual(self.run_flow(), 2)
        self.assertEqual(self.world.killed_groups, [])

    def test_verified_leader_with_surviving_group_or_timeout_kills_the_group_once(self):
        for message in ("recorded leader is absent but its process group still has members", "owned process did not stop before timeout; retaining ownership record"):
            with self.subTest(message):
                self.setUp()
                self.world.daemon_stop_error = message
                self.assertEqual(self.run_flow(), 0, self.err)
                self.assertEqual(len(self.world.killed_groups), 1)
                self.temp.cleanup()


class OwnedProcessStopTest(FixtureBase):
    def stop_backend(self):
        registry = FakeRegistry(self.world)
        self.world.pid += 1
        self.world.records.append({"label": fixture.PROCESS_LABEL, "pid": self.world.pid, "started_at": "t", "pgid": self.world.pid})
        self.world.alive.add(self.world.pid)
        self.world.record_ports[self.world.pid] = 43111
        return fixture.stop_owned_process(registry, self.world.ops(), fixture.PROCESS_LABEL)

    def test_a_backend_that_ignores_sigterm_is_killed_through_its_recorded_group(self):
        self.world.backend_stop_error = "owned process did not stop before timeout; retaining ownership record"
        self.assertEqual(self.stop_backend(), "stopped")
        self.assertEqual(self.world.killed_groups, [self.world.pid])
        self.assertEqual(self.world.records, [])

    def test_a_stop_that_succeeds_never_signals_the_group(self):
        self.assertEqual(self.stop_backend(), "stopped")
        self.assertEqual(self.world.killed_groups, [])

    def test_an_identity_refusal_is_unverified_and_never_signals_the_group(self):
        self.world.backend_stop_error = "PID start identity or process group changed; refusing to signal"
        self.assertEqual(self.stop_backend(), "unverified")
        self.assertEqual(self.world.killed_groups, [])


class EarlyFailureStateTest(FixtureBase):
    def test_host_key_mismatch_during_runner_init_leaves_actionable_state(self):
        self.world.scanned = ["worker.example ssh-ed25519 OTHER"]
        self.assertEqual(self.run_flow(), 2)
        root = self.fixture_root()
        self.assertEqual((root / "fixture.json").stat().st_mode & 0o777, 0o600)
        self.assertIn(f"operator cleanup: {fixture.cleanup_command(root)}", self.out)
        self.world.scanned = None
        self.out.clear()
        self.assertEqual(fixture.run_cleanup_fixture(str(root), ops=self.world.ops(), out=self.out.append, err=self.err.append), 0, self.err)
        self.assertEqual(self.world.records, [])
        self.assertFalse((root / "home" / ".ssh" / "id_ed25519").exists())


class IdentityCopyLifetimeTest(FixtureBase):
    def key(self):
        return self.fixture_root() / "home" / ".ssh" / "id_ed25519"

    def test_successful_teardown_deletes_the_copied_identity(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertFalse(self.key().exists())
        self.assertTrue(self.evidence()["cleanup"]["ssh_identity_removed"])

    def test_failed_run_keeps_the_identity_until_cleanup_fixture_succeeds(self):
        self.world.recover_mutation = "fence"
        self.assertEqual(self.run_flow(), 2)
        self.assertTrue(self.key().exists())
        self.world.fence = "owner:fence-content\n"
        self.assertEqual(fixture.run_cleanup_fixture(str(self.fixture_root()), ops=self.world.ops(), out=self.out.append, err=self.err.append), 0, self.err)
        self.assertFalse(self.key().exists())

    def test_failed_cleanup_keeps_the_identity_for_the_retry(self):
        self.world.recover_mutation = "tree"
        self.assertEqual(self.run_flow(), 2)
        self.world.stop_leaves_alive = True
        self.assertEqual(fixture.run_cleanup_fixture(str(self.fixture_root()), ops=self.world.ops(), out=self.out.append, err=self.err.append), 2)
        self.assertTrue(self.key().exists())


class NomadRedirectTest(unittest.TestCase):
    def test_token_is_never_forwarded_on_a_redirect(self):
        import http.server
        import threading

        seen = []

        class Target(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                seen.append(self.headers.get("X-Nomad-Token"))
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b"{}")

            def log_message(self, *args):
                pass

        target = http.server.HTTPServer(("127.0.0.1", 0), Target)

        class Redirector(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                self.send_response(302)
                self.send_header("Location", f"http://127.0.0.1:{target.server_port}/v1/stolen")
                self.end_headers()

            def log_message(self, *args):
                pass

        redirector = http.server.HTTPServer(("127.0.0.1", 0), Redirector)
        threads = [threading.Thread(target=server.serve_forever, daemon=True) for server in (target, redirector)]
        for thread in threads:
            thread.start()
        self.addCleanup(lambda: [(server.shutdown(), server.server_close()) for server in (target, redirector)])
        get = fixture._default_nomad_get(f"http://127.0.0.1:{redirector.server_port}", TOKEN, "default", "global")
        with self.assertRaisesRegex(PreflightError, "status 302"):
            get("/v1/job/x")
        self.assertEqual(seen, [])

    def test_redirect_handler_refuses_every_redirect(self):
        request = fixture.urllib.request.Request("http://127.0.0.1:1/x", headers={"X-Nomad-Token": TOKEN})
        handler = fixture._NoRedirect()
        self.assertIsNone(handler.redirect_request(request, None, 307, "Temporary Redirect", {}, "http://evil.example/"))


class SharedHelpersTest(unittest.TestCase):
    def test_await_recovered_environment_times_out_on_a_fake_clock(self):
        clock = FakeClock()
        db = sqlite3.connect(":memory:")
        db.execute("CREATE TABLE task_environments (id TEXT, status TEXT, workspace_path TEXT, ownership_generation INTEGER)")
        db.execute("CREATE TABLE task_environment_repos (id TEXT, task_environment_id TEXT)")
        db.execute("INSERT INTO task_environments VALUES ('env','creating','',1)")
        with self.assertRaisesRegex(PreflightError, "did not become ready"):
            base._await_recovered_environment(lambda fn: fn(db), lambda: {"workspace_path": "/w"}, environment_id="env", generation_before=1,
                                              expected_repo_count=0, timeout=2, clock=clock.clock, sleep=clock.sleep)
        db.execute("UPDATE task_environments SET status='ready', workspace_path='/w'")
        self.assertEqual(base._await_recovered_environment(lambda fn: fn(db), lambda: {"workspace_path": "/w"}, environment_id="env", generation_before=1,
                                                           expected_repo_count=0, timeout=2, clock=clock.clock, sleep=clock.sleep), {"workspace_path": "/w"})

    def test_session_untouched_and_workspace_read_checks(self):
        messages = [{"id": "a", "type": "status", "turn_id": "t"}]
        base._check_session_untouched({"state": "CREATED"}, messages, messages)
        with self.assertRaises(PreflightError):
            base._check_session_untouched({"state": "CREATED"}, messages + [{"id": "b"}], messages)
        base._check_workspace_read({"ok": True, "contains_expected_file": True})
        with self.assertRaises(PreflightError):
            base._check_workspace_read({"ok": True})
        self.assertEqual(base._parse_checkout_heads(FIXTURE_HEAD + "\n"), FIXTURE_HEAD)
        base._check_workspace_head(FIXTURE_HEAD, FIXTURE_HEAD)
        base._check_workspace_head(FIXTURE_HEAD, None)
        with self.assertRaises(PreflightError):
            base._check_workspace_head("b" * 40, FIXTURE_HEAD)


if __name__ == "__main__":
    unittest.main()
