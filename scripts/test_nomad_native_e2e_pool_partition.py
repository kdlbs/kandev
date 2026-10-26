import unittest
import unittest.mock
from pathlib import Path

import nomad_native_e2e_fixture as fx
import nomad_native_e2e_pool_partition as pp
from nomad_native_e2e import PreflightError

CFG = {"nomad": {"address": "http://127.0.0.1:4646"}, "pool": {"node_pool": "kandev"},
       "ssh_client": {"pid": 4242, "config": "/fx/ssh-client.hcl"}}
ENVS = {
    "A": {"label": "A", "attach": "local", "state_root": "/l", "node_id": "n-local", "job_id": "job-l", "allocation_id": "al-l",
          "environment_id": "env-l", "session_id": "s-l", "task_id": "t-l"},
    "B": {"label": "B", "attach": "ssh", "state_root": "/s", "node_id": "n-ssh", "job_id": "job-s", "allocation_id": "al-s",
          "environment_id": "env-s", "session_id": "s-s", "task_id": "t-s"},
}


class World:
    def __init__(self, **flags):
        self.flags, self.partitioned, self.log = flags, False, []
        self.node = {"n-local": "ready", "n-ssh": "ready"}
        self.allocs = {"job-l": [{"ID": "al-l"}], "job-s": [{"ID": "al-s"}]}
        self.status = {"al-l": "running", "al-s": "running"}
        self.jobs = {"job-l", "job-s"}
        self.messages = {"s-l": ["m1"], "s-s": ["m2"]}
        self.tree = {"env-l": [(1, "a")], "env-s": [(2, "b")]}
        self.fence = {"env-l": "f1", "env-s": "f2"}
        self.prompts = []
        self.states = {"s-l": flags.get("state", "WAITING_FOR_INPUT"), "s-s": "WAITING_FOR_INPUT"}

    def on_partition(self):
        self.partitioned = True
        if not self.flags.get("bypass"):
            self.node["n-ssh"] = "disconnected"
            self.status["al-s"] = "unknown"
        if self.flags.get("replaced"):
            self.allocs["job-s"].append({"ID": "al-new"})
        if self.flags.get("new_job"):
            self.jobs.add("kandev-new")
        if self.flags.get("ssh_report"):
            self.messages["s-s"].append("kandev-unavailable-report")
        if self.flags.get("ssh_lost_messages"):
            self.messages["s-s"] = ["other"]
        if self.flags.get("ssh_lost"):
            self.status["al-s"] = "lost"
        if self.flags.get("local_tree"):
            self.tree["env-l"] = [(9, "z")]
        if self.flags.get("local_fence"):
            self.fence["env-l"] = "other"

    def on_heal(self):
        self.partitioned = False
        self.node["n-ssh"] = "ready"
        if not self.flags.get("stay_unknown"):
            self.status["al-s"] = "paused" if self.flags.get("not_running") else "running"
        if self.flags.get("ssh_tree_after"):
            self.tree["env-s"] = [(7, "q")]


class Process:
    """Fake local nomad client agent: ps output plus signal log wired into the world."""

    def __init__(self, world, command="/fx/bin/nomad agent -config=/fx/ssh-client.hcl", fail_cont=False):
        self.world, self.command, self.calls, self.fail_cont = world, command, [], fail_cont
        self.partitioned = False

    def process_command(self, pid):
        return self.command

    def signal(self, pid, sig):
        self.calls.append((pid, sig))
        if sig == "STOP":
            self.partitioned = True
            self.world.on_partition()
        else:
            if self.fail_cont:
                raise OSError("boom")
            self.partitioned = False
            self.world.on_heal()


def make_runner(world, ops):
    runner = type("R", (), {})()
    runner.ops, runner.state, runner.evidence = ops, {"nomad": {"node_id": "n-local"}, "task_id": "t-l", "session_id": "s-l", "port": 1}, \
        type("E", (), {"data": {"steps": [], "identities": {}}, "save": lambda self: None})()
    runner.step = lambda name, fn: fn()
    runner.poll = lambda t, i, probe, failure: fx.Runner.poll(runner, t, i, probe, failure)
    runner.use_node = lambda a, r: None
    runner.start_git_daemon = runner.start_backend = lambda *a: None
    runner.setup_api_objects = lambda cfg: None
    runner.create_task = lambda title: {"id": "t-s", "session_id": "s-s"}
    envs = {"s-l": "env-l", "s-s": "env-s"}
    runner.inventory = lambda sid, tid: {"session": sid, "job_id": "job-l" if sid == "s-l" else "job-s"}
    runner.fence = lambda env_id=None: world.fence[env_id]
    runner.nomad_snapshot = lambda inv: {"inv": inv["session"], "jobs": sorted(world.jobs & {"job-l", "job-s"})}
    runner.nomad = lambda path: {"Status": world.node[path.rsplit("/", 1)[1]], "SchedulingEligibility": "eligible"}
    def api(method, path, body):
        if path.startswith("/tasks/"):
            sid = {"t-l": "s-l", "t-s": "s-s"}[path.split("/")[2]]
            return {"sessions": [{"id": sid, "state": world.states[sid]}]}
        return {"messages": [{"id": m} for m in world.messages[path.split("/")[2]]]}

    runner.api = api
    return runner


class FakeScn(pp.PartitionScenario):
    world = None

    def verify_node_identity(self):
        return {"ok": True}

    def node_ready(self):
        return True

    def verify_two_nodes(self):
        return {"nodes": 2}

    def await_placed(self, label, task):
        self.envs[label] = dict(ENVS[label])
        self.data["identities"][label] = {}
        return {}

    def workspace_layout(self, env):
        return {"kind": "worktree" if env["attach"] == "local" else "checkout"}

    def start_managed_tree(self, env, task, snapshot):
        return []

    def kandev_jobs(self):
        return set(self.world.jobs)

    def job_allocs(self, job_id):
        return list(self.world.allocs[job_id])

    def alloc(self, allocation_id):
        return {"ClientStatus": self.world.status[allocation_id]}

    def tree(self, env):
        return [{"pid": p, "started": s} for p, s in self.world.tree[env["environment_id"]]]

    def archive(self, task):
        self.world.log.append(("archive", task["id"]))
        if self.world.flags.get("operator"):
            self.ops.nomad_stop("a", "t", "n", "j")

    def production_settled(self, env, pending):
        return {"phase": "absent"}

    def assert_pool_cleaned(self, env, layout):
        self.world.log.append(("cleaned", env["label"], layout["kind"]))
        return {}

    def finish(self, report):
        return {**report, "finished": True}


class PartitionBase(unittest.TestCase):
    def run_drive(self, cfg=CFG, fail_heal=False, probe_accepted=True, probe_adds=True, reply_only_user=False, command=None, **flags):
        world = World(**flags)
        ticks = iter(range(10000))
        ops = fx.Ops()
        ops.sleep = lambda s: None
        ops.clock = lambda: next(ticks)
        ops.scenario_timeout, ops.poll_interval = 30, 0
        ops.nomad_stop = lambda *a: None
        self.proc = Process(world, fail_cont=fail_heal, **({"command": command} if command else {}))
        ops.process_command, ops.signal_local = self.proc.process_command, self.proc.signal

        def prompt(payload):
            world.prompts.append(payload["session_id"])
            if probe_adds:
                world.messages[payload["session_id"]] += [f"m-new-{len(world.prompts)}", f"m-reply-{len(world.prompts)}"]
            if reply_only_user:
                world.messages[payload["session_id"]].pop()
            return {"accepted": probe_accepted}

        ops.prompt = prompt
        runner = make_runner(world, ops)
        FakeScn.world = world
        self.world, self.runner = world, runner
        with unittest.mock.patch.object(pp, "PartitionScenario", FakeScn):
            try:
                return pp.drive_pool_partition(runner, cfg, ops, Path("/x"))
            finally:
                self.ops = ops

    def fails(self, message, **kw):
        with self.assertRaises(PreflightError) as ctx:
            self.run_drive(**kw)
        self.assertIn(message, str(ctx.exception))
        return str(ctx.exception)


class ConfigTest(unittest.TestCase):
    def test_ssh_client_block_is_required(self):
        pp.check_scenario_config(CFG)
        for cfg in ({**CFG, "ssh_client": None}, {**CFG, "ssh_client": {"pid": 4}},
                    {k: v for k, v in CFG.items() if k != "ssh_client"}, {k: v for k, v in CFG.items() if k != "pool"}):
            with self.assertRaises(PreflightError):
                pp.check_scenario_config(cfg)

    def test_drive_refuses_before_touching_anything_without_ssh_client(self):
        t = PartitionBase()
        with self.assertRaises(PreflightError):
            t.run_drive(cfg={"nomad": CFG["nomad"], "pool": CFG["pool"]})

    def test_scenario_name(self):
        self.assertEqual(pp.SCENARIO, "pool-partition")


class DriveTest(PartitionBase):
    def test_happy_path(self):
        report = self.run_drive()
        self.assertTrue(report["production_cleanup"] and not report["operator_cleanup_used"])
        self.assertEqual(self.proc.calls, [(4242, "STOP"), (4242, "CONT")])
        self.assertEqual(self.world.prompts, ["s-l"])
        self.assertEqual([x for x in self.world.log if x[0] == "archive"], [("archive", "t-l"), ("archive", "t-s")])
        self.assertEqual(sorted(x[1] for x in self.world.log if x[0] == "cleaned"), ["A", "B"])
        self.assertTrue(report["cleanup"]["finished"])
        phases = {(o["phase"], o["node"]) for o in report["observed_statuses"]}
        self.assertIn(("during", "disconnected"), phases)
        self.assertEqual(self.runner.evidence.data["after_heal"]["ssh_allocation_client_status"], "running")

    def test_client_never_leaving_ready_fails_and_continues_it(self):
        self.fails("never left ready while its Nomad client agent was stopped", bypass=True)
        self.assertEqual(self.proc.calls[-1], (4242, "CONT"))
        self.assertFalse(self.runner.evidence.data["client_stopped_at_exit"])

    def test_identity_mismatch_refuses_to_signal(self):
        for command in ("/usr/bin/sleep 100", "/fx/bin/nomad agent -config=/other.hcl", "/fx/bin/nomad version /fx/ssh-client.hcl"):
            self.fails("not the recorded Nomad client agent", command=command)
            self.assertEqual(self.proc.calls, [])

    def test_vanished_process_refuses(self):
        world = World()
        proc = Process(world)
        proc.process_command = lambda pid: None
        ops = fx.Ops()
        ops.process_command, ops.signal_local = proc.process_command, proc.signal
        with self.assertRaises(PreflightError) as ctx:
            pp.ClientFreezer(ops, CFG["ssh_client"]).partition()
        self.assertIn("is not running", str(ctx.exception))
        self.assertEqual(proc.calls, [])

    def test_local_env_drift_fails(self):
        self.fails("remote process tree of environment A changed", local_tree=True)
        self.fails("worker fence of environment A changed", local_fence=True)

    def test_probe_must_be_accepted_and_add_a_message(self):
        self.fails("did not accept the prompt", probe_accepted=False)
        self.fails("did not answer the prompt", probe_adds=False)
        self.fails("did not answer the prompt", reply_only_user=True)

    def test_probe_waits_for_a_promptable_local_session(self):
        self.fails("not ready for input before the probe prompt (state 'RUNNING')", state="RUNNING")
        self.assertEqual(self.world.prompts, [])

    def test_unavailability_report_on_the_ssh_session_is_allowed_but_not_loss(self):
        report = self.run_drive(ssh_report=True)
        self.assertTrue(report["production_cleanup"])
        self.fails("messages of environment B changed", ssh_lost_messages=True)

    def test_ssh_allocation_replaced(self):
        self.fails("has 2 allocations", replaced=True)

    def test_ssh_allocation_terminal(self):
        self.fails("became 'lost'", ssh_lost=True)

    def test_new_job_registered(self):
        self.fails("unexpected new kandev job(s) ['kandev-new']", new_job=True)

    def test_allocation_unknown_after_heal(self):
        self.fails("stayed unknown after the partition healed", stay_unknown=True)

    def test_allocation_not_running_after_heal(self):
        self.fails("expected running", not_running=True)

    def test_ssh_tree_changed_after_heal(self):
        self.fails("remote process tree of environment B changed", ssh_tree_after=True)

    def test_failure_note_names_the_client_and_continue_command(self):
        message = self.fails("was continued", ssh_lost=True)
        self.assertEqual(self.proc.calls, [(4242, "STOP"), (4242, "CONT")])
        self.assertFalse(self.proc.partitioned)

    def test_unresumable_client_is_reported_with_the_manual_command(self):
        message = self.fails("could not be continued: boom", fail_heal=True, bypass=True)
        self.assertIn("kill -CONT 4242", message)
        self.assertIn("/fx/ssh-client.hcl", message)
        self.assertTrue(self.runner.evidence.data["client_stopped_at_exit"])

    def test_operator_cleanup_fails_the_run_and_restores_ops(self):
        original = fx.Ops.nomad_stop
        self.fails("operator cleanup was used", operator=True)
        self.assertNotEqual(getattr(self.ops.nomad_stop, "__name__", ""), "inner")
        self.assertIs(fx.Ops.nomad_stop, original)

    def test_same_node_placement_fails(self):
        world_envs = {"A": dict(ENVS["A"]), "B": {**ENVS["B"], "node_id": "n-local"}}
        with unittest.mock.patch.dict(ENVS, world_envs):
            self.fails("same node")

    def test_both_local_fails(self):
        with unittest.mock.patch.dict(ENVS, {"B": {**ENVS["B"], "attach": "local", "node_id": "n-other"}}):
            self.fails("one local node and one ssh node")


if __name__ == "__main__":
    unittest.main()
