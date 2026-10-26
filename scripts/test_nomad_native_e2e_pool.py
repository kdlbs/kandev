import os
import unittest
import os
import unittest.mock

import nomad_native_e2e_fixture as fixture
import nomad_native_e2e_pool as pool
from nomad_native_e2e import PreflightError
from nomad_profile_payload import ProfilePayloadError, build_profile_payload, validate_profile_source
from test_nomad_native_e2e_wo06 import Wo06Base

POOL = {"node_pool": "kandev", "placement": "binpack", "ssh_user": "worker", "memory_max_mb": 2048}
SOURCE = {
    "schema_version": 1, "executor_id": "00000000-0000-0000-0000-000000000002",
    "profile_id": "00000000-0000-0000-0000-000000000003", "profile_name": "pool fixture",
    "nomad_token_ref": "$kandev-secret:00000000-0000-0000-0000-000000000004",
    "config": {"address": "http://127.0.0.1:4646", "namespace": "default", "region": "global", "datacenter": "dc1",
               "platform": "darwin/arm64", "node_pool": "kandev", "placement": "spread", "cpu_mhz": "100"},
}


def with_config(**updates):
    return {**SOURCE, "config": {**SOURCE["config"], **updates}}


class PoolPayloadTest(unittest.TestCase):
    def test_pool_profile_payload_carries_pool_keys_and_token_reference(self):
        payload = build_profile_payload(validate_profile_source(SOURCE))
        self.assertEqual(payload["config"]["node_pool"], "kandev")
        self.assertEqual(payload["config"]["nomad_token"], SOURCE["nomad_token_ref"])
        self.assertNotIn("node_id", payload["config"])

    def test_pool_profile_rejects_pinned_keys_and_missing_required(self):
        for updates in ({"node_id": "x"}, {"ssh_target": "a@b"}, {"workspace_root": "/x"}, {"bogus": "1"}):
            with self.assertRaises(ProfilePayloadError, msg=str(updates)):
                validate_profile_source(with_config(**updates))
        config = dict(SOURCE["config"])
        del config["datacenter"]
        with self.assertRaises(ProfilePayloadError):
            validate_profile_source({**SOURCE, "config": config})


class PoolBootstrapTest(Wo06Base):
    def test_pool_is_optional_and_validated(self):
        self.assertIsNone(self.validate()["pool"])
        self.config["pool"] = dict(POOL)
        self.assertEqual(self.validate()["pool"], POOL)

    def test_host_identity_token_file_must_be_a_private_regular_file(self):
        token = self.parent / "identity-token"
        token.write_text("secret-token\n")
        token.chmod(0o600)
        self.config["pool"] = {**POOL, "host_identity_token_file": str(token)}
        self.assertEqual(self.validate()["pool"]["host_identity_token_file"], str(token))
        token.chmod(0o644)
        with self.assertRaises(PreflightError):
            self.validate()
        self.config["pool"] = {**POOL, "host_identity_token_file": "relative/token"}
        with self.assertRaises(PreflightError):
            self.validate()

    def test_ssh_client_is_optional_and_validated(self):
        self.assertIsNone(self.validate()["ssh_client"])
        conf = self.parent / "ssh-client.hcl"
        conf.write_text("client {}\n")
        conf.chmod(0o600)
        self.config["ssh_client"] = {"pid": 4242, "config": str(conf)}
        self.assertEqual(self.validate()["ssh_client"], {"pid": 4242, "config": str(conf)})
        for value in ({"pid": 4242}, {"pid": 1, "config": str(conf)}, {"pid": True, "config": str(conf)},
                      {"pid": 4242, "config": "rel.hcl"}, {"pid": 4242, "config": str(conf), "x": 1}, "x"):
            self.config["ssh_client"] = value
            with self.assertRaises(PreflightError, msg=str(value)):
                self.validate()

    def test_pool_rejections(self):
        bad = [{}, {"placement": "spread"}, {"node_pool": "bad pool"}, {"node_pool": "kandev", "placement": "random"},
               {"node_pool": "kandev", "node_id": "x"}, {"node_pool": "kandev", "ssh_user": "a@b"},
               {"node_pool": "kandev", "memory_max_mb": 0}, "x"]
        for value in bad:
            self.config["pool"] = value
            with self.assertRaises(PreflightError, msg=str(value)):
                self.validate()

    def test_pool_profile_config_names_no_node_or_worker_paths(self):
        self.config["pool"] = dict(POOL)
        cfg = self.validate()
        config = fixture.Runner.pool_profile_config(cfg)
        self.assertEqual(config["node_pool"], "kandev")
        self.assertEqual(config["placement"], "binpack")
        self.assertEqual(config["memory_max_mb"], "2048")
        for key in ("node_id", "ssh_target", "ssh_port", "ssh_host_fingerprint", "workspace_root", "allocation_root"):
            self.assertNotIn(key, config)

    def test_scenario_requires_pool_block_and_loopback_nomad(self):
        cfg = self.validate()
        with self.assertRaises(PreflightError):
            pool.check_scenario_config(cfg, "pool-placement")
        self.config["pool"] = dict(POOL)
        cfg = self.validate()
        pool.check_scenario_config(cfg, "pool-placement")
        with self.assertRaises(PreflightError):
            pool.check_scenario_config(cfg, "other")


class NodeContractTest(unittest.TestCase):
    SSH = {"kandev.attach": "ssh", "kandev.state_root": "/srv/kandev", "kandev.ssh_host": "w.example.test",
           "kandev.ssh_host_key_sha256": "a" * 64}

    def test_contract_problems(self):
        self.assertEqual(pool.contract_problems(self.SSH), [])
        self.assertEqual(pool.contract_problems({"kandev.attach": "local", "kandev.state_root": "/s", "kandev.host_identity_sha256": "b" * 64}), [])
        self.assertTrue(pool.contract_problems({}))
        self.assertIn("missing kandev.ssh_host", pool.contract_problems({k: v for k, v in self.SSH.items() if k != "kandev.ssh_host"}))
        self.assertIn("missing kandev.host_identity_sha256", pool.contract_problems({"kandev.attach": "local", "kandev.state_root": "/s"}))
        self.assertTrue(pool.contract_problems({**self.SSH, "kandev.attach": "rsh"}))

    def test_job_named_node_checks_every_scope(self):
        pin = {"LTarget": "${node.unique.id}", "Operand": "=", "RTarget": "n"}
        other = {"LTarget": "${attr.kernel.name}", "Operand": "=", "RTarget": "darwin"}
        self.assertFalse(pool.job_named_node({"Constraints": [other], "TaskGroups": [{"Tasks": [{}]}]}))
        self.assertTrue(pool.job_named_node({"Constraints": [pin]}))
        self.assertTrue(pool.job_named_node({"TaskGroups": [{"Constraints": [pin]}]}))
        self.assertTrue(pool.job_named_node({"TaskGroups": [{"Tasks": [{"Constraints": [pin]}]}]}))


class PoolFencePathTest(unittest.TestCase):
    STATE = {"allocation_root": "/fx/alloc/a", "node_id": "n1"}

    def test_pool_fence_path_uses_node_state_root(self):
        self.assertEqual(fixture._fence_path({**self.STATE, "pool_state_root": "/fx/worker"}), "/fx/worker/environments/unrecorded/active")

    def test_pool_fence_is_per_environment(self):
        state = {**self.STATE, "pool_state_root": "/fx/worker"}
        self.assertEqual(fixture._fence_path(state, "env-1"), "/fx/worker/environments/env-1/active")
        state["fence_environment_id"] = "env-2"
        self.assertEqual(fixture._fence_path(state), "/fx/worker/environments/env-2/active")
        self.assertNotEqual(fixture._fence_path(state, "env-1"), fixture._fence_path(state))

    def test_legacy_fence_path_is_unchanged(self):
        self.assertEqual(fixture._fence_path(self.STATE), "/fx/worker-state/n1/active-environment")


class OwnedFenceTest(unittest.TestCase):
    def sha(self, owner):
        import nomad_native_e2e_wo06 as wo06
        scn = wo06.Scenario.__new__(wo06.Scenario)
        scn.r = type("R", (), {"fence_owner": lambda self: owner})()
        return scn.owned_fence_sha({"job_id": "kandev-job-1"})

    def test_only_a_fence_naming_the_job_is_released(self):
        import hashlib
        self.assertEqual(self.sha("env kandev-job-1\n"), hashlib.sha256(b"env kandev-job-1\n").hexdigest())
        self.assertIsNone(self.sha("env kandev-job-2\n"))
        self.assertIsNone(self.sha(None))


class TwoNodeLayoutTest(unittest.TestCase):
    def scenario(self, output, code=0, executed=None):
        scn = pool.TwoNodeScenario.__new__(pool.TwoNodeScenario)
        calls = executed if executed is not None else []
        scn.read_state = lambda env: {"workspace_path": "/n/workspaces/environments/e1/g1"}
        scn.r = type("R", (), {"use_node": lambda self, a, r: calls.append(("use", a, r)),
                               "ssh": lambda self, cmd: (calls.append(("ssh", cmd)) or (code, output))})()
        return scn

    def env(self, attach):
        return {"label": "A", "attach": attach, "state_root": "/n", "environment_id": "e1"}

    def test_local_node_must_hold_a_worktree_on_the_environment_branch(self):
        layout = self.scenario("worktree kandev/pool/e1 /src/.git").workspace_layout(self.env("local"))
        self.assertEqual(layout["kind"], "worktree")
        with self.assertRaises(PreflightError):
            self.scenario("checkout main /n/ws/.git").workspace_layout(self.env("local"))
        with self.assertRaises(PreflightError):
            self.scenario("worktree main /src/.git").workspace_layout(self.env("local"))

    def test_ssh_node_must_hold_a_plain_checkout(self):
        self.assertEqual(self.scenario("checkout main /n/ws/.git").workspace_layout(self.env("ssh"))["kind"], "checkout")
        with self.assertRaises(PreflightError):
            self.scenario("worktree kandev/pool/e1 /src/.git").workspace_layout(self.env("ssh"))

    def test_empty_or_failed_listing_is_refused(self):
        for output, code in (("", 0), ("worktree kandev/pool/e1 /src/.git", 1)):
            with self.assertRaises(PreflightError):
                self.scenario(output, code).workspace_layout(self.env("local"))

    def test_cleaned_requires_branch_and_registration_gone_for_a_worktree(self):
        layout = {"kind": "worktree", "branch": "kandev/pool/e1", "git_common_dir": "/src/.git"}
        gone = self.scenario("", 1)
        gone.r.ssh = lambda cmd: (1, "") if "show-ref" in cmd else (0, "worktree /src\n")
        self.assertTrue(gone.assert_pool_cleaned(self.env("local"), layout)["branch_removed"])
        branch_left = self.scenario("", 0)
        branch_left.r.ssh = lambda cmd: (0, "")
        with self.assertRaises(PreflightError):
            branch_left.assert_pool_cleaned(self.env("local"), layout)
        registered = self.scenario("", 0)
        registered.r.ssh = lambda cmd: (1, "") if "show-ref" in cmd else (0, "worktree /n/workspaces/environments/e1/g1\n")
        with self.assertRaises(PreflightError):
            registered.assert_pool_cleaned(self.env("local"), layout)


class AwaitPlacedRecordTest(unittest.TestCase):
    def test_placement_is_recorded_before_readiness_so_cleanup_finds_the_node(self):
        scn = pool.TwoNodeScenario.__new__(pool.TwoNodeScenario)
        scn.envs, scn.data, scn.saved = {}, {"identities": {}}, []
        inventory = {"node_id": "n1", "allocation_id": "al1", "job_id": "kandev-abc", "environment_id": "e1"}
        scn.poll = lambda probe, message, *a: probe()
        scn.soft = lambda fn: fn()
        scn.save_envs = lambda: scn.saved.append(dict(scn.envs))

        def not_ready(label, task):
            raise PreflightError("did not reach durable inventory phase ready")

        scn.await_ready = not_ready
        scn.r = type("R", (), {"inventory": lambda self, sid, tid: inventory,
                               "nomad": lambda self, path: {"Meta": {"kandev.attach": "ssh", "kandev.state_root": "/n"}},
                               "use_node": lambda self, a, r: None, "state": {}, "save_state": lambda self, **kw: None})()
        with self.assertRaises(PreflightError):
            scn.await_placed("C", {"id": "t", "session_id": "s"})
        self.assertEqual((scn.envs["C"]["node_id"], scn.envs["C"]["state_root"], scn.envs["C"]["attach"]), ("n1", "/n", "ssh"))


class FocusTest(unittest.TestCase):
    def test_focus_points_commands_and_fence_at_the_environment_node(self):
        import nomad_native_e2e_wo06 as wo06
        calls = []
        scn = wo06.Scenario.__new__(wo06.Scenario)
        scn.r = type("R", (), {"state": {}, "use_node": lambda self, a, r: calls.append((a, r))})()
        scn.focus({"attach": "local", "state_root": "/l", "environment_id": "e9"})
        scn.focus({"label": "legacy"})
        self.assertEqual(calls, [("local", "/l")])
        self.assertEqual(scn.r.state["fence_environment_id"], "e9")

    def test_quarantine_scenario_is_registered(self):
        self.assertIn("pool-node-quarantine", pool.SCENARIOS)
        self.assertIn("pool-node-quarantine", pool.DRIVERS)
        self.assertIn("pool-two-node", pool.DRIVERS)


class StrayJobTest(unittest.TestCase):
    def scenario(self, jobs, baseline=(), known=(), remaining=()):
        import nomad_native_e2e_wo06 as wo06
        stops = []
        scn = wo06.Scenario.__new__(wo06.Scenario)
        scn.nomad_cfg = {"address": "http://127.0.0.1:4646", "namespace": "default"}
        scn.envs = {"A": {"job_id": j} for j in known}
        status = {j: s for j, s in jobs.items()}
        scn.kandev_jobs = lambda: set(jobs)
        scn.job_status = lambda j: status.get(j)
        scn.job_allocs = lambda j: [{"ID": "al-" + j, "NodeID": "n1", "ClientStatus": "complete" if j in [s[2] for s in stops] else "running"}]
        scn.poll = lambda probe, failure, timeout=None: probe() or (_ for _ in ()).throw(PreflightError(failure))
        scn.tree = lambda probe: list(remaining)
        scn.ops = type("O", (), {"nomad_stop": lambda self, a, t, n, j: stops.append((a, n, j))})()
        scn.r = type("R", (), {"token": "t", "state": {"baseline_jobs": list(baseline)},
                               "nomad": lambda self, path: {"Meta": {"kandev.attach": "ssh", "kandev.state_root": "/n"}}})()
        return scn, stops

    def test_stops_running_unrecorded_job_with_purge_off_and_skips_known_dead_and_baseline(self):
        import nomad_native_e2e_wo06 as wo06
        scn, stops = self.scenario({"kandev-old": "running", "kandev-known": "running", "kandev-dead": "dead", "kandev-stray": "running"},
                                   baseline=["kandev-old"], known=["kandev-known"])
        stopped = wo06.stop_stray_jobs(scn)
        self.assertEqual([s[2] for s in stops], ["kandev-stray"])
        self.assertEqual(stopped, [{"job_id": "kandev-stray", "previous_status": "running", "allocations": 1, "purged": False}])

    def test_remaining_worker_process_fails_cleanup(self):
        import nomad_native_e2e_wo06 as wo06
        scn, _ = self.scenario({"kandev-stray": "running"}, remaining=[{"pid": 1}])
        with self.assertRaises(PreflightError):
            wo06.stop_stray_jobs(scn)


class ActiveNodeRoutingTest(unittest.TestCase):
    def test_commands_follow_the_node_in_use(self):
        runner = fixture.Runner.__new__(fixture.Runner)
        runner.ops = fixture.Ops()
        sent = []
        runner.ops.local_exec = lambda cmd: sent.append(("local", cmd)) or (0, "")
        runner.ssh_remote = lambda cmd: sent.append(("ssh", cmd)) or (0, "")
        runner.active_attach = "ssh"
        runner.save_state = lambda **kw: runner.__dict__.setdefault("saved", []).append(kw)
        runner._exec_on_active_node("a")
        runner.use_node("local", "/n")
        runner._exec_on_active_node("b")
        self.assertEqual(sent, [("ssh", "a"), ("local", "b")])
        self.assertEqual(runner.saved, [{"pool_state_root": "/n"}])
        with self.assertRaises(PreflightError):
            runner.use_node("bogus", "/n")


class PoolSettledTest(unittest.TestCase):
    def scenario(self, existing):
        import nomad_native_e2e_wo07 as wo07
        scn = pool.PoolScenario.__new__(pool.PoolScenario)
        scn.guard = None
        scn.read_state = lambda env: {"workspace_path": "/fx/worker/workspaces/environments/e1/g1"}
        scn.r = type("R", (), {"state": {"pool_state_root": "/fx/worker"},
                               "ssh": lambda self, cmd: (0 if any(p in cmd for p in existing) else 1, "")})()
        base = lambda self, env, pending: {"phase": "absent"}
        self.patch = unittest.mock.patch.object(wo07.Wo07Scenario, "production_settled", base, create=True)
        self.patch.start()
        self.addCleanup(self.patch.stop)
        return scn

    def test_settled_only_when_workspace_fence_and_allocation_directories_are_gone(self):
        env = {"environment_id": "e1", "job_id": "kandev-j1"}
        done = self.scenario([]).production_settled(env, [])
        self.assertTrue(done["workspace_removed"] and done["fence_directory_removed"] and done["allocation_directory_removed"])
        pending = []
        self.assertIsNone(self.scenario(["pool/allocations/kandev-j1'"]).production_settled(env, pending))
        self.assertIn("allocation directory", pending[0])
        pending = []
        self.assertIsNone(self.scenario(["workspaces/environments"]).production_settled(env, pending))
        self.assertIn("workspace", pending[0])
        pending = []
        self.assertIsNone(self.scenario(["/environments/e1'"]).production_settled(env, pending))
        self.assertIn("fence directory", pending[0])


if __name__ == "__main__":
    unittest.main()


class LocalNodeTreeAndLeftoversTest(unittest.TestCase):
    def scenario(self, rows, attach="local"):
        import nomad_native_e2e_wo06 as wo06
        scn = wo06.Scenario.__new__(wo06.Scenario)
        scn.r = type("R", (), {"state": {}, "active_attach": attach, "use_node": lambda self, a, r: None})()
        scn.ps_rows = lambda: rows
        return scn

    def row(self, pid, ppid, command):
        return {"pid": pid, "ppid": ppid, "pgid": pid, "started": "Mon Oct 26 15:17:24 2026", "command": command}

    def test_local_tree_roots_at_the_allocation_binary_and_ignores_backend_git_commands(self):
        env = {"job_id": "kandev-job1", "allocation_id": "al1", "environment_id": "env1"}
        rows = [self.row(10, 1, "/s/pool/allocations/kandev-job1/agentctl-" + "a" * 64), self.row(11, 10, "/bin/zsh -l"),
                self.row(20, 1, "git -C /s/workspaces/environments/env1/g1/repo-main status"),
                self.row(21, 1, "/usr/bin/nomad executor {\"alloc\":\"al1\"}")]
        tree = self.scenario(rows).tree(env)
        self.assertEqual([item["pid"] for item in tree], [10, 11])
        self.assertEqual(tree[0]["role"], "agentctl")

    def test_ssh_tree_still_matches_the_environment_id(self):
        rows = [self.row(20, 1, "git -C /s/workspaces/environments/env1/g1 status")]
        tree = self.scenario(rows, "ssh").tree({"job_id": "kandev-job1", "environment_id": "env1"})
        self.assertEqual([item["pid"] for item in tree], [20])

    def test_pool_leftovers_remove_worktree_branch_and_directories(self):
        import subprocess, tempfile, nomad_native_e2e_wo06 as wo06
        with tempfile.TemporaryDirectory() as tmp:
            tmp = os.path.realpath(tmp)
            src, root = f"{tmp}/src", f"{tmp}/state"
            def sh(cmd):
                return subprocess.run(["/bin/sh", "-c", cmd], capture_output=True, text=True, check=False)
            ws = f"{root}/workspaces/environments/env1/g1"
            for cmd in (f"git init -q -b main {src} && git -C {src} -c user.email=a@b -c user.name=a commit -q --allow-empty -m i",
                        f"mkdir -p {ws} {root}/environments/env1 {root}/pool/allocations/kandev-job1",
                        f"git -C {src} worktree add -q -b kandev/pool/env1 {ws}/repo-main main"):
                self.assertEqual(sh(cmd).returncode, 0, cmd)
            scn = wo06.Scenario.__new__(wo06.Scenario)
            scn.r = type("R", (), {"state": {}, "ssh": lambda self, c: (lambda p: (p.returncode, p.stdout))(sh(c))})()
            report = scn.remove_pool_leftovers({"label": "A", "state_root": root, "environment_id": "env1", "job_id": "kandev-job1"}, True)
            self.assertTrue(report["removed"])
            self.assertNotEqual(sh(f"git -C {src} show-ref --verify --quiet refs/heads/kandev/pool/env1").returncode, 0)
            self.assertNotIn("env1", sh(f"git -C {src} worktree list --porcelain").stdout)
            self.assertFalse(os.path.exists(f"{root}/environments/env1") or os.path.exists(ws))

    def test_remove_b_leftovers_removes_then_asserts_the_checks(self):
        scn = pool.TwoNodeScenario.__new__(pool.TwoNodeScenario)
        calls = []
        scn.focus = lambda env: calls.append("focus")
        scn.remove_pool_leftovers = lambda env, strict: calls.append("remove") or {"removed": True, "branch": "b"}
        scn.assert_pool_cleaned = lambda env, layout: calls.append("assert") or {"kind": layout["kind"]}
        report = scn.remove_b_leftovers({"label": "B"}, {"kind": "worktree"})
        self.assertEqual(calls, ["focus", "remove", "assert"])
        self.assertTrue(report["removed"] and report["kind"] == "worktree")

    def test_readmission_removes_the_allocation_directory_and_asserts_it_gone(self):
        import subprocess, tempfile
        with tempfile.TemporaryDirectory() as tmp:
            tmp = os.path.realpath(tmp)
            alloc = f"{tmp}/pool/allocations/kandev-job1"
            os.makedirs(alloc)
            def sh(cmd):
                p = subprocess.run(["/bin/sh", "-c", cmd], capture_output=True, text=True, check=False)
                return p.returncode, p.stdout
            scn = pool.TwoNodeScenario.__new__(pool.TwoNodeScenario)
            scn.focus = lambda env: None
            scn.pool_node_root = lambda env: tmp
            scn.r = type("R", (), {"ssh": lambda self, c: sh(c)})()
            self.assertTrue(scn.remove_allocation_dir({"label": "A", "job_id": "kandev-job1"})["allocation_directory_removed"])
            self.assertFalse(os.path.exists(alloc))
            with self.assertRaises(PreflightError):
                scn.remove_allocation_dir({"label": "A", "job_id": "../x"})

    def test_pool_leftovers_refuse_unsafe_paths(self):
        import nomad_native_e2e_wo06 as wo06
        scn = wo06.Scenario.__new__(wo06.Scenario)
        scn.r = type("R", (), {"state": {}})()
        with self.assertRaises(PreflightError):
            scn.remove_pool_leftovers({"label": "A", "state_root": "/s", "environment_id": "../x", "job_id": "j"}, True)


class TreeDiagnosticsTest(unittest.TestCase):
    PS = """\
    1     0     1 Mon Oct 26 09:00:00 2026 /sbin/launchd
  500     1   500 Mon Oct 26 09:01:00 2026 /usr/local/bin/nomad agent -config=/fx/local-client.hcl
  600   500   600 Mon Oct 26 15:17:30 2026 /usr/local/bin/nomad executor {"LogFile":"/fx/alloc/al-1/logs"}
  601   600   600 Mon Oct 26 15:17:31 2026 /fx/local-state/pool/allocations/kandev-job1/agentctl-{sha} --workdir /fx/local-state/workspaces/environments/env1/g1
"""

    def scenario(self, rows_text, attach="local"):
        import nomad_native_e2e_wo06 as wo06
        scn = wo06.Scenario.__new__(wo06.Scenario)
        ops = fixture.Ops()
        ops.sleep = lambda s: None
        ops.scenario_timeout, ops.poll_interval = 0, 0
        ticks = iter(range(0, 1000, 1))
        ops.clock = lambda: next(ticks)
        scn.ops, scn.data, scn.cfg = ops, {}, None
        runner = type("R", (), {"state": {"pool_state_root": "/fx/local-state"}, "active_attach": attach, "use_node": lambda s, a, r: None})()
        runner.ssh = lambda cmd: (0, rows_text)
        runner.poll = lambda t, i, probe, failure: fixture.Runner.poll(runner, t, i, probe, failure)
        runner.ops = ops
        scn.r = runner
        return scn

    ENV = {"label": "A", "job_id": "kandev-job1", "allocation_id": "al-1", "environment_id": "env1"}

    def test_captured_local_listing_yields_one_agentctl_root_and_ignores_nomad_helpers(self):
        text = self.PS.replace("{sha}", "a" * 64)
        tree = self.scenario(text).tree(self.ENV)
        self.assertEqual([(item["pid"], item["role"]) for item in tree], [(601, "agentctl")])

    def test_unsettled_tree_records_diagnostics_in_evidence(self):
        scn = self.scenario("  1     0     1 Mon Oct 26 09:00:00 2026 /sbin/launchd\n")
        with self.assertRaises(PreflightError):
            scn.stable_tree(self.ENV)
        diag = scn.data["tree_diagnostics"]["A"]
        self.assertEqual(diag["active_attach"], "local")
        self.assertEqual(diag["ps_row_count"], 1)
        self.assertEqual(diag["needles"], ["al-1", "kandev-job1", "env1"])
        self.assertEqual(diag["named_rows"], [])
