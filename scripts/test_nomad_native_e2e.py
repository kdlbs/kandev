import contextlib
import io
import json
import os
import sqlite3
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import nomad_native_e2e as nne
from nomad_native_e2e import (
    PreflightError, REQUIRED_SCENARIOS, _fixture_backend_argv, _pre_ready_contract, _recovery_completed_turn,
    _run_attached_session, check_post_ready_environment, check_pre_ready_environment, check_pre_ready_session,
    main, validate_config,
)


class NomadNativePreflightTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="nomad-e2e-contract-")
        self.root = Path(self.temp.name).resolve()
        self.root.chmod(0o700)
        self.nomad = self._executable("nomad")
        self.ssh = self._executable("ssh")
        self.sshd = self._executable("sshd")
        self.keygen = self._executable("ssh-keygen")
        self.private_key = self._file("id_ed25519", b"fixture-private-key\n", 0o600)
        self.known_hosts = self._file("known_hosts", b"127.0.0.1 ssh-ed25519 fixture\n", 0o600)
        self.kandev_home = self.root / "kandev-home"
        (self.kandev_home / ".ssh").mkdir(parents=True, mode=0o700)
        self.kandev_home.chmod(0o700)
        (self.kandev_home / ".ssh").chmod(0o700)
        (self.kandev_home / ".ssh" / "id_ed25519").write_bytes(self.private_key.read_bytes())
        (self.kandev_home / ".ssh" / "id_ed25519").chmod(0o600)
        (self.kandev_home / "supervisor").mkdir(mode=0o700)
        self.bundle = self.root / "runtime-bundle"
        self.bin_dir = self.bundle / "bin"
        self.bin_dir.mkdir(parents=True, mode=0o700)
        self.kandev_binary = self._executable_at(self.bin_dir / "kandev")
        for helper in ("agentctl", "agentctl-linux-amd64", "agentctl-linux-arm64", "agentctl-darwin-arm64", "agentctl-darwin-amd64"):
            self._executable_at(self.bin_dir / helper)
        self.launch_manifest = self.kandev_home / "supervisor" / "launch.json"
        self.launch_manifest.write_text(json.dumps({
            "home_dir": str(self.kandev_home),
            "backend_executable": str(self.kandev_binary),
            "argv": ["__backend"],
            "port": 43840,
            "env": {
                "HOME": str(self.kandev_home),
                "KANDEV_HOME_DIR": str(self.kandev_home),
                "KANDEV_DATABASE_PATH": str(self.kandev_home / "data" / "kandev.db"),
            },
        }))
        self.launch_manifest.chmod(0o600)
        self.config = {
            "schema_version": 1,
            "fixture_only": True,
            "fixture_root": str(self.root),
            "kandev_home": str(self.kandev_home),
            "backend": {"binary": str(self.kandev_binary), "port": 43840},
            "nomad": {
                "binary": str(self.nomad),
                "version": "2.0.7",
                "server_rpc": "127.0.0.1:26446",
                "client_rpc_proxy": "127.0.0.1:26448",
                "node_name": "ff-fixture-worker",
            },
            "ssh": {
                "client": str(self.ssh),
                "server": str(self.sshd),
                "keygen": str(self.keygen),
                "host": "127.0.0.1",
                "user": "fixture",
                "port": 22422,
                "private_key": str(self.private_key),
                "known_hosts": str(self.known_hosts),
            },
            "partition": {
                "mode": "loopback-tcp-relay",
                "listen": "127.0.0.1:26448",
                "can_drop_active_connections": True,
            },
        }
        self.commands = []

    def tearDown(self):
        self.temp.cleanup()

    def _file(self, name, content, mode):
        path = self.root / name
        path.write_bytes(content)
        path.chmod(mode)
        return path

    def _executable(self, name):
        path = self._file(name, b"#!/bin/sh\nexit 0\n", 0o700)
        return path

    def _executable_at(self, path):
        path.write_bytes(b"#!/bin/sh\nexit 0\n")
        path.chmod(0o700)
        return path

    def _run(self, command, **kwargs):
        self.commands.append(command)
        if command[-1] == "version":
            return SimpleNamespace(returncode=0, stdout="Nomad v2.0.7 (abc)\n", stderr="")
        return SimpleNamespace(returncode=0, stdout="", stderr="OpenSSH_10.3p1\n")

    def test_valid_config_runs_only_local_version_probes(self):
        result = validate_config(self.config, run=self._run)
        self.assertEqual(result["nomad_version"], "2.0.7")
        self.assertEqual(self.commands, [
            ["/usr/bin/codesign", "--verify", "--strict", str(self.bin_dir / "agentctl-darwin-arm64")],
            [str(self.nomad), "version"],
            [str(self.ssh), "-V"],
        ])

    def test_non_loopback_nomad_endpoint_is_rejected_before_probes(self):
        self.config["nomad"]["client_rpc_proxy"] = "192.0.2.20:26448"
        with self.assertRaises(PreflightError):
            validate_config(self.config, run=self._run)
        self.assertEqual(self.commands, [])

    def test_fixture_node_name_is_not_confused_with_runtime_node_uuid(self):
        result = validate_config(self.config, run=self._run)
        self.assertEqual(result["nomad_version"], "2.0.7")
        self.assertNotIn("node_id", self.config["nomad"])

        self.config["nomad"]["node_name"] = "00000000-0000-0000-0000-000000000001"
        with self.assertRaises(PreflightError):
            validate_config(self.config, run=self._run)

    def test_private_key_permissions_are_required(self):
        self.private_key.chmod(0o644)
        with self.assertRaises(PreflightError):
            validate_config(self.config, run=self._run)
        self.assertEqual(self.commands, [])

    def test_backend_home_identity_must_match_pinned_ssh_key(self):
        identity = self.kandev_home / ".ssh" / "id_ed25519"
        identity.write_bytes(b"different-private-key\n")
        identity.chmod(0o600)
        with self.assertRaisesRegex(PreflightError, "differs from the pinned fixture client identity"):
            validate_config(self.config, run=self._run)
        self.assertEqual(self.commands, [])

    def test_backend_home_must_be_private_and_inside_fixture_root(self):
        self.kandev_home.chmod(0o755)
        with self.assertRaisesRegex(PreflightError, "private directory inside fixture_root"):
            validate_config(self.config, run=self._run)
        self.assertEqual(self.commands, [])

    def test_launch_manifest_must_prove_child_home_before_any_probe(self):
        payload = json.loads(self.launch_manifest.read_text())
        payload["env"]["HOME"] = str(self.root)
        self.launch_manifest.write_text(json.dumps(payload))
        self.launch_manifest.chmod(0o600)
        with self.assertRaisesRegex(PreflightError, "launch manifest does not prove fixture HOME"):
            validate_config(self.config, run=self._run)
        self.assertEqual(self.commands, [])

    def test_launch_manifest_is_required_before_any_probe(self):
        self.launch_manifest.unlink()
        with self.assertRaisesRegex(PreflightError, "Kandev launch manifest must exist"):
            validate_config(self.config, run=self._run)
        self.assertEqual(self.commands, [])

    def test_backend_bundle_must_use_adjacent_complete_bin_layout(self):
        self.kandev_binary = self._executable_at(self.root / "kandev")
        self.config["backend"]["binary"] = str(self.kandev_binary)
        with self.assertRaisesRegex(PreflightError, "canonical <bundle>/bin/kandev"):
            from nomad_native_e2e import _verify_runtime_bundle
            _verify_runtime_bundle(self.kandev_binary, self._run)
        self.assertEqual(self.commands, [])

    def test_missing_runtime_helper_fails_before_any_probe(self):
        (self.bin_dir / "agentctl-darwin-arm64").unlink()
        with self.assertRaisesRegex(PreflightError, "missing agentctl-darwin-arm64"):
            validate_config(self.config, run=self._run)
        self.assertEqual(self.commands, [])

    def test_fixture_launcher_exec_argv_contains_binary_once_before_run_args(self):
        argv = _fixture_backend_argv(self.kandev_binary, self.root / "startup.log", 43841)
        self.assertEqual(argv[4:], [str(self.kandev_binary), "run", "--headless", "--port", "43841"])
        self.assertEqual(argv[4:].count(str(self.kandev_binary)), 1)

    def test_non_session_scenario_fails_closed_without_starting_services(self):
        error = io.StringIO()
        with contextlib.redirect_stderr(error):
            result = main(["run", "--scenario", "admission", "--require-real", "--config", "/missing/config.json"])
        self.assertEqual(result, 2)
        self.assertIn("requires --require-real --scenario session", error.getvalue())
        self.assertEqual(self.commands, [])

    def test_recovery_scenario_has_its_own_strict_fixture_gate(self):
        error = io.StringIO()
        with contextlib.redirect_stderr(error):
            result = main(["run", "--scenario", "recovery", "--require-real", "--config", "/missing/config.json"])
        self.assertEqual(result, 2)
        self.assertIn("recovery verification failed", error.getvalue())
        self.assertEqual(self.commands, [])

    def test_attached_real_session_requires_complete_explicit_contract(self):
        with self.assertRaisesRegex(PreflightError, "exact attached-fixture session contract"):
            _run_attached_session({"fixture_root": str(self.root), "session": {}})

    def test_recovery_turn_requires_one_correlated_user_and_agent_message(self):
        turn = {
            "user_message_id": "user-1",
            "agent_delivery_id": "delivery-1",
            "turn_id": "turn-1",
            "prompt_marker": "WO05_MARKER_1",
            "expected_output": "nomad-inert-result",
        }
        messages = [
            {"id": "user-1", "author_type": "user", "type": "message", "turn_id": "turn-1", "content": "output WO05_MARKER_1"},
            {"id": "delivery-1", "author_type": "agent", "type": "message", "turn_id": "turn-1", "content": "nomad-inert-result"},
        ]
        result = _recovery_completed_turn(messages, turn)
        self.assertEqual(result, {"user_message_id": "user-1", "agent_delivery_id": "delivery-1", "turn_id": "turn-1"})

    def test_recovery_turn_allows_prior_user_input_in_same_agent_turn(self):
        turn = {
            "user_message_id": "user-2",
            "agent_delivery_id": "delivery-1",
            "turn_id": "turn-1",
            "prompt_marker": "WO05_MARKER_2",
            "expected_output": "nomad-inert-result",
        }
        messages = [
            {"id": "user-1", "author_type": "user", "type": "message", "turn_id": "turn-1", "content": "initial task input"},
            {"id": "user-2", "author_type": "user", "type": "message", "turn_id": "turn-1", "content": "WO05_MARKER_2"},
            {"id": "delivery-1", "author_type": "agent", "type": "message", "turn_id": "turn-1", "content": "nomad-inert-result"},
        ]
        self.assertEqual(_recovery_completed_turn(messages, turn)["agent_delivery_id"], "delivery-1")

    def test_recovery_turn_rejects_duplicate_or_uncorrelated_delivery(self):
        turn = {
            "user_message_id": "user-1",
            "agent_delivery_id": "delivery-1",
            "turn_id": "turn-1",
            "prompt_marker": "WO05_MARKER_1",
            "expected_output": "nomad-inert-result",
        }
        user = {"id": "user-1", "author_type": "user", "type": "message", "turn_id": "turn-1", "content": "output WO05_MARKER_1"}
        delivery = {"id": "delivery-1", "author_type": "agent", "type": "message", "turn_id": "turn-1", "content": "nomad-inert-result"}
        with self.assertRaisesRegex(PreflightError, "duplicate or unexpected"):
            _recovery_completed_turn([user, delivery, {**delivery, "id": "delivery-2"}], turn)
        with self.assertRaisesRegex(PreflightError, "correlated"):
            _recovery_completed_turn([user, {**delivery, "turn_id": "other-turn"}], turn)


    def _pre_ready_config(self):
        env_id = "11111111-1111-4111-8111-111111111111"
        session = {
            "api_base": "http://127.0.0.1:43840/api/v1", "ws_base": "ws://127.0.0.1:43840", "nomad_http": "http://127.0.0.1:4646",
            "task_id": "22222222-2222-4222-8222-222222222222", "session_id": "33333333-3333-4333-8333-333333333333",
            "environment_id": env_id, "node_id": "44444444-4444-4444-8444-444444444444", "node_name": "ff-fixture-worker",
            "job_id": "job", "allocation_id": "55555555-5555-4555-8555-555555555555", "workspace_path": "/w/g1",
            "environment_generation": 1, "agentctl_pid": 11, "acp_pid": 12, "child_pid": 13, "grandchild_pid": 14,
            "nomad_client_pid": 15, "kandev_backend_pid": 16, "kandev_binary_path": "/bin/kandev",
        }
        expected = {"process_label": "kandev", "operation_id": "op", "server_identity": "srv", "job_create_index": 3,
                    "job_definition_index": 4, "expected_repo_count": 1, "expected_file": "README.md"}
        return {"session": session, "pre_ready_recovery": expected}

    def test_pre_ready_scenario_is_selectable_and_requires_real(self):
        self.assertIn("pre-ready-recovery", REQUIRED_SCENARIOS)
        error = io.StringIO()
        with contextlib.redirect_stderr(error):
            result = main(["run", "--scenario", "pre-ready-recovery", "--config", "/missing/config.json"])
        self.assertEqual(result, 2)
        self.assertIn("requires --require-real", error.getvalue())
        error = io.StringIO()
        with contextlib.redirect_stderr(error):
            result = main(["run", "--scenario", "pre-ready-recovery", "--require-real", "--config", "/missing/config.json"])
        self.assertEqual(result, 2)
        self.assertIn("pre-ready recovery verification failed", error.getvalue())

    def test_config_may_carry_pre_ready_recovery_key(self):
        self.config["pre_ready_recovery"] = self._pre_ready_config()["pre_ready_recovery"]
        self.assertEqual(validate_config(self.config, run=self._run)["nomad_version"], "2.0.7")

    def test_pre_ready_contract_accepts_exact_key_sets(self):
        session, expected = _pre_ready_contract(self._pre_ready_config())
        self.assertEqual(expected["expected_repo_count"], 1)
        self.assertEqual(session["environment_generation"], 1)

    def test_pre_ready_contract_rejects_missing_and_extra_keys(self):
        for section in ("session", "pre_ready_recovery"):
            missing = self._pre_ready_config()
            missing[section].pop(next(iter(missing[section])))
            with self.assertRaises(PreflightError):
                _pre_ready_contract(missing)
            extra = self._pre_ready_config()
            extra[section]["unexpected"] = 1
            with self.assertRaises(PreflightError):
                _pre_ready_contract(extra)
        with self.assertRaises(PreflightError):
            _pre_ready_contract({"session": self._pre_ready_config()["session"]})

    def test_pre_ready_contract_rejects_invalid_values(self):
        for key, value in (("expected_repo_count", -1), ("expected_repo_count", True), ("expected_file", "")):
            config = self._pre_ready_config()
            config["pre_ready_recovery"][key] = value
            with self.assertRaises(PreflightError):
                _pre_ready_contract(config)

    def _environment_db(self, status="creating", workspace_path="", repos=1, generation=1):
        db = sqlite3.connect(":memory:")
        db.execute("CREATE TABLE task_environments (id TEXT PRIMARY KEY, status TEXT, workspace_path TEXT, ownership_generation INTEGER)")
        db.execute("CREATE TABLE task_environment_repos (id TEXT PRIMARY KEY, task_environment_id TEXT)")
        db.execute("INSERT INTO task_environments VALUES ('env', ?, ?, ?)", (status, workspace_path, generation))
        for index in range(repos):
            db.execute("INSERT INTO task_environment_repos VALUES (?, 'env')", (f"r{index}",))
        return db

    def test_pre_ready_environment_requires_creating_with_empty_workspace(self):
        self.assertEqual(check_pre_ready_environment(self._environment_db(), "env"), 1)
        with self.assertRaises(PreflightError):
            check_pre_ready_environment(self._environment_db(status="ready"), "env")
        with self.assertRaises(PreflightError):
            check_pre_ready_environment(self._environment_db(workspace_path="/w/g1"), "env")
        with self.assertRaises(PreflightError):
            check_pre_ready_environment(self._environment_db(), "other")

    def test_post_ready_environment_pending_until_ready_then_exact(self):
        self.assertEqual(check_post_ready_environment(self._environment_db(), "env", "/w/g1", 1, 1), "pending")
        ready = self._environment_db(status="ready", workspace_path="/w/g1")
        self.assertEqual(check_post_ready_environment(ready, "env", "/w/g1", 1, 1), "ready")
        for kwargs, args in (
            ({"status": "ready", "workspace_path": "/w/other"}, ("/w/g1", 1, 1)),
            ({"status": "ready", "workspace_path": "/w/g1", "generation": 2}, ("/w/g1", 1, 1)),
            ({"status": "ready", "workspace_path": "/w/g1", "repos": 0}, ("/w/g1", 1, 1)),
            ({"status": "ready", "workspace_path": "/w/g1", "repos": 2}, ("/w/g1", 1, 1)),
        ):
            with self.assertRaises(PreflightError):
                check_post_ready_environment(self._environment_db(**kwargs), "env", *args)

    def test_pre_ready_session_requires_created_without_acp_or_user_messages(self):
        check_pre_ready_session({"state": "CREATED"}, [{"author_type": "agent", "type": "status"}])
        with self.assertRaises(PreflightError):
            check_pre_ready_session({"state": "WAITING_FOR_INPUT"}, [])
        with self.assertRaises(PreflightError):
            check_pre_ready_session({"state": "CREATED", "downstream_acp_session_id": "acp"}, [])
        with self.assertRaises(PreflightError):
            check_pre_ready_session({"state": "CREATED"}, [{"author_type": "user"}])


class AttachedPreReadyRecoveryTest(unittest.TestCase):
    """Drives _run_attached_pre_ready_recovery with every external collaborator replaced by a fake."""

    OLD_PID, NEW_PID = 16, 99
    SNAPSHOT = {"job_id": "job", "allocation_id": "alloc", "node_id": "node", "fence": 7}
    INVENTORY = {"workspace_path": "/w/g1", "server_identity": "srv"}
    ROWS = {11: {"p": 1}, 12: {"p": 2}, 13: {"p": 3}, 14: {"p": 4}}

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="nomad-e2e-recovery-")
        self.addCleanup(self.temp.cleanup)
        self.home = Path(self.temp.name)
        (self.home / "supervisor").mkdir()
        (self.home / "supervisor" / "launch.json").write_text(
            json.dumps({"backend_executable": "/bin/kandev", "port": 43840, "argv": ["serve"], "env": {}}), encoding="utf-8")
        self.config = NomadNativePreflightTest._pre_ready_config(self)
        self.config["session"]["kandev_binary_path"] = "/bin/kandev"
        self.config["session"]["kandev_backend_pid"] = self.OLD_PID
        self.config["nomad"] = {"binary": "/bin/nomad"}
        self.config["fixture_root"] = str(self.home)
        self.config["kandev_home"] = str(self.home)
        self.record = {"label": "kandev", "pid": self.OLD_PID}
        self.registry = mock.Mock()
        self.registry.read_records.return_value = [self.record]
        self.registry.stop.return_value = "stopped"
        self.registry.start.return_value = {"pid": self.NEW_PID}
        self.connect_ex = 111
        self.listener_pids = [self.OLD_PID, self.NEW_PID]
        self.http = mock.Mock(return_value={})
        self.local_pid = lambda path, pid: {"pid": pid}
        self.rows = [dict(self.ROWS), dict(self.ROWS)]
        self.inventories = [dict(self.INVENTORY), dict(self.INVENTORY)]
        self.snapshots = [dict(self.SNAPSHOT), dict(self.SNAPSHOT)]

    def _run(self):
        registry_module = SimpleNamespace(OwnedProcessRegistry=lambda root: self.registry)
        fake_spec = SimpleNamespace(loader=SimpleNamespace(exec_module=lambda module: None))
        probe = mock.MagicMock()
        probe.__enter__.return_value = probe
        probe.connect_ex.return_value = self.connect_ex
        listeners = list(self.listener_pids)
        inventory_iter = iter(self.inventories)
        snapshot_iter = iter(self.snapshots)
        rows_iter = iter(self.rows)
        self.workspace_read = mock.Mock(return_value={"ok": True, "contains_expected_file": True})
        patches = [
            mock.patch.object(nne, "validate_config"),
            mock.patch.object(nne, "_owned_private_root", return_value=self.home),
            mock.patch.object(nne, "_owned_private_directory", return_value=self.home),
            mock.patch.object(nne, "_fixture_database", return_value=str(self.home / "db")),
            mock.patch.object(nne.sqlite3, "connect", return_value=mock.Mock()),
            mock.patch.object(nne, "_pre_ready_messages", return_value=[]),
            mock.patch.object(nne, "_pre_ready_session_row", return_value={"state": "CREATED"}),
            mock.patch.object(nne, "check_pre_ready_environment", return_value=1),
            mock.patch.object(nne, "_nomad_inventory_snapshot", side_effect=lambda *a, **k: next(inventory_iter)),
            mock.patch.object(nne, "_nomad_recovery_snapshot", side_effect=lambda *a, **k: next(snapshot_iter)),
            mock.patch.object(nne.importlib.util, "spec_from_file_location", return_value=fake_spec),
            mock.patch.object(nne.importlib.util, "module_from_spec", return_value=registry_module),
            mock.patch.object(nne, "_executable", return_value=Path("/bin/kandev")),
            mock.patch.object(nne, "_local_process", side_effect=lambda path, pid: self.local_pid(path, pid)),
            mock.patch.object(nne, "_listener_pid", side_effect=lambda port: listeners.pop(0)),
            mock.patch.object(nne, "_remote_process_rows", side_effect=lambda *a: next(rows_iter)),
            mock.patch.object(nne.socket, "socket", return_value=probe),
            mock.patch.object(nne, "_http_json", self.http),
            mock.patch.object(nne.time, "sleep"),
            mock.patch.object(nne, "_await_recovered_environment", side_effect=lambda *a, **k: next(inventory_iter)),
            mock.patch.object(nne, "_workspace_read", self.workspace_read),
        ]
        with contextlib.ExitStack() as stack:
            for patch in patches:
                stack.enter_context(patch)
            return nne._run_attached_pre_ready_recovery(self.config)

    def _assert_rejected(self, message):
        with self.assertRaisesRegex(PreflightError, message):
            self._run()
        self.workspace_read.assert_not_called()

    def test_happy_path_reports_recovery_without_prompts(self):
        report = self._run()
        self.assertEqual(report["prompts_sent"], 0)
        self.assertEqual((report["backend_old_pid"], report["backend_new_pid"]), (self.OLD_PID, self.NEW_PID))
        self.assertEqual(report["allocation_id"], "alloc")
        self.registry.stop.assert_called_once_with("kandev", timeout=10)
        self.workspace_read.assert_called_once()

    def test_rejects_backend_record_that_is_not_the_listener_owner(self):
        self.record["pid"] = 5
        self._assert_rejected("not the exact fixture listener owner")
        self.registry.read_records.return_value = []
        self._assert_rejected("not the exact fixture listener owner")

    def test_rejects_listener_that_is_not_the_registered_child(self):
        self.listener_pids = [4242, self.NEW_PID]
        self._assert_rejected("not the registered current Kandev child")

    def test_rejects_incomplete_agentctl_pids(self):
        self.rows = [{11: {}, 12: {}, 13: {}}, {}]
        self._assert_rejected("agentctl process identities are incomplete")

    def test_rejects_absent_nomad_client_before_recovery(self):
        self.local_pid = lambda path, pid: {"pid": pid} if path != "/bin/nomad" else {"pid": 0}
        self._assert_rejected("Nomad client is absent before recovery")
        self.registry.stop.assert_not_called()

    def test_rejects_backend_that_did_not_stop(self):
        self.registry.stop.return_value = "timeout"
        self._assert_rejected("did not stop through its exact owned process record")
        self.registry.start.assert_not_called()

    def test_rejects_api_port_left_open(self):
        self.connect_ex = 0
        self._assert_rejected("API port remained open")
        self.registry.start.assert_not_called()

    def test_rejects_api_that_never_recovers(self):
        self.http.side_effect = OSError("down")
        clock = iter([0, 0, 30, 61, 62, 63])
        with mock.patch.object(nne.time, "monotonic", side_effect=lambda: next(clock)):
            self._assert_rejected("did not recover its API before timeout")

    def test_rejects_restarted_listener_that_is_not_a_new_process(self):
        self.registry.start.return_value = {"pid": self.OLD_PID}
        self._assert_rejected("does not belong to a new owned backend process")
        self.registry.start.return_value = {"pid": self.NEW_PID}
        self.listener_pids = [self.OLD_PID, 4242]
        self._assert_rejected("does not belong to a new owned backend process")

    def test_rejects_changed_nomad_inventory(self):
        self.inventories[1] = {**self.INVENTORY, "server_identity": "other"}
        self._assert_rejected("changed the retained Nomad operation, allocation, or fence")

    def test_rejects_changed_nomad_allocation_or_fence(self):
        self.snapshots[1] = {**self.SNAPSHOT, "allocation_id": "alloc-2"}
        self._assert_rejected("changed the retained Nomad operation, allocation, or fence")
        self.snapshots[1] = {**self.SNAPSHOT, "fence": 8}
        self._assert_rejected("changed the retained Nomad operation, allocation, or fence")

    def test_rejects_changed_remote_process_tree(self):
        self.rows[1] = {**self.ROWS, 14: {"p": "restarted"}}
        self._assert_rejected("did not preserve the exact remote runtime process tree")

    def test_rejects_changed_nomad_client_identity(self):
        calls = []

        def local_pid(path, pid):
            if path == "/bin/nomad":
                calls.append(pid)
                return {"pid": pid if len(calls) == 1 else pid + 1}
            return {"pid": pid}

        self.local_pid = local_pid
        self._assert_rejected("Nomad client identity changed during Kandev backend recovery")


if __name__ == "__main__":
    unittest.main()
