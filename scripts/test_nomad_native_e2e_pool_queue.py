import unittest
from pathlib import Path

import nomad_native_e2e_pool_queue as pq
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError

JOB = "kandev-abc123"


def node(free, reserved=0, node_id="n1"):
    return {"ID": node_id, "NodeResources": {"Memory": {"MemoryMB": free + reserved}}, "ReservedResources": {"Memory": {"MemoryMB": reserved}}}


class SizingTest(unittest.TestCase):
    def test_free_memory_subtracts_reserved_and_tolerates_missing_reservation(self):
        self.assertEqual(pq.node_free_memory_mb(node(1000, 200)), 1000)
        self.assertEqual(pq.node_free_memory_mb({"NodeResources": {"Memory": {"MemoryMB": 900}}}), 900)
        self.assertEqual(pq.node_free_memory_mb({"NodeResources": {"Memory": {"MemoryMB": 900}}, "ReservedResources": None}), 900)

    def test_free_memory_refuses_missing_or_exhausted_capacity(self):
        for bad in ({}, {"NodeResources": {}}, {"NodeResources": {"Memory": {"MemoryMB": "x"}}},
                    {"NodeResources": {"Memory": {"MemoryMB": 100}}, "ReservedResources": {"Memory": {"MemoryMB": 100}}}):
            with self.assertRaises(PreflightError):
                pq.node_free_memory_mb(bad)

    def test_sizing_holds_one_per_node_and_never_two(self):
        for frees in ([1000, 1000], [1000, 1900], [4000, 5000], [1001, 1001]):
            size = pq.size_pool_memory(frees)
            self.assertLessEqual(size["memory_mb"], min(frees))
            self.assertGreater(2 * size["memory_mb"], max(frees))
            self.assertGreaterEqual(size["memory_max_mb"], size["memory_mb"])

    def test_sizing_fails_when_no_value_exists_or_one_node(self):
        with self.assertRaises(PreflightError):
            pq.size_pool_memory([1000, 2100])
        with self.assertRaises(PreflightError):
            pq.size_pool_memory([1000])

    def test_with_memory_copies_without_mutating(self):
        cfg = {"memory_mb": 1, "pool": {"node_pool": "p", "memory_max_mb": 2}}
        out = pq.with_memory(cfg, {"memory_mb": 600, "memory_max_mb": 600})
        self.assertEqual((out["memory_mb"], out["pool"]["memory_max_mb"], out["pool"]["node_pool"]), (600, 600, "p"))
        self.assertEqual(cfg, {"memory_mb": 1, "pool": {"node_pool": "p", "memory_max_mb": 2}})


class PureChecksTest(unittest.TestCase):
    def test_blocked_proof(self):
        blocked = {"Status": "blocked", "FailedTGAllocs": {"g": {"DimensionExhausted": {"memory": 1}}}}
        self.assertEqual(pq.blocked_proof([blocked])["dimensions_exhausted"], ["memory"])
        self.assertIsNone(pq.blocked_proof([]))
        self.assertIsNone(pq.blocked_proof([{**blocked, "Status": "complete"}]))
        self.assertIsNone(pq.blocked_proof([{"Status": "blocked", "FailedTGAllocs": {"g": {"DimensionExhausted": {"cpu": 1}}}}]))
        self.assertIsNone(pq.blocked_proof([{"Status": "blocked", "FailedTGAllocs": None}]))

    def test_fence_order_and_same_node(self):
        pq.check_fence_order(True, False)
        pq.check_fence_order(False, True)
        pq.check_fence_order(False, False)
        with self.assertRaises(PreflightError):
            pq.check_fence_order(True, True)
        pq.check_same_node("n1", "n1")
        with self.assertRaises(PreflightError):
            pq.check_same_node("n1", "n2")


class FakeRunner:
    def __init__(self, fences=None):
        self.fences = fences if fences is not None else {}  # (attach, env_id) -> owner
        self.active = "local"
        self.inv = {"phase": "creating", "job_id": JOB}
        self.nomad_view = "nomad"
        self.fence_env = None
        self.state = {}

    def use_node(self, attach, root):
        self.active = attach

    def fence_owner(self, env_id=None):
        return self.fences.get((self.active, env_id))

    def fence(self):
        return self.fences.get((self.active, self.fence_env))

    def inventory(self, session_id=None, task_id=None):
        return dict(self.inv)

    def nomad_snapshot(self, inventory):
        return self.nomad_view


class FakeQueue(pq.PoolQueueScenario):
    """A scenario with every outside read replaced by a settable attribute."""

    def __init__(self, runner=None):
        self.r = runner or FakeRunner()
        self.data = {"identities": {"C": {}}}
        self.known_jobs = set()
        self.envs = {}
        self.node_roots = [("local", "/l"), ("ssh", "/s")]
        self.allocs, self.evals, self.procs, self.job_exists = [], [], [], True
        self.tree_value, self.recorded_tree = [], []

    def poll(self, probe, failure, timeout=None):
        value = probe()
        if value is None:
            raise PreflightError(failure)
        return value

    def soft(self, fn):
        return fn()

    def get_or_none(self, path):
        if path.endswith("/evaluations"):
            return self.evals
        return {"Job": {}} if self.job_exists else None

    def job_allocs(self, job_id):
        return self.allocs

    def record_env(self, label, task, inventory, **extra):
        env = {"label": label, "environment_id": "envC", "job_id": inventory["job_id"]}
        self.envs[label] = env
        return env

    def tree(self, env):
        return self.tree_value if self.r.active == "local" else list(self.procs)

    def focus(self, env):
        self.r.fence_env = env.get("environment_id")
        self.r.active = "local"


TASK = {"id": "t", "session_id": "s"}
BLOCKED = [{"Status": "blocked", "FailedTGAllocs": {"g": {"DimensionExhausted": {"memory": 3}}}}]


class QueuedProofTest(unittest.TestCase):
    def scn(self, **kw):
        scn = FakeQueue()
        scn.evals = BLOCKED
        for key, value in kw.items():
            setattr(scn, key, value)
        return scn

    def test_queued_proof_passes(self):
        scn = self.scn()
        result = scn.assert_queued("C", TASK)
        self.assertEqual((result["allocations"], result["fences"], result["remote_processes"]), (0, 0, 0))
        self.assertIn(JOB, scn.known_jobs)

    def test_unregistered_job_fails(self):
        with self.assertRaises(PreflightError):
            self.scn(job_exists=False).assert_queued("C", TASK)

    def test_allocated_fails(self):
        with self.assertRaises(PreflightError):
            self.scn(allocs=[{"ID": "a"}]).assert_queued("C", TASK)

    def test_missing_blocked_evaluation_fails(self):
        with self.assertRaises(PreflightError):
            self.scn(evals=[]).assert_queued("C", TASK)

    def test_ready_phase_fails(self):
        scn = self.scn()
        scn.r.inv["phase"] = "ready"
        with self.assertRaises(PreflightError):
            scn.assert_queued("C", TASK)

    def test_fence_on_either_node_fails(self):
        for attach in ("local", "ssh"):
            scn = self.scn()
            scn.r.fences[(attach, "envC")] = "owner"
            with self.assertRaises(PreflightError, msg=attach):
                scn.assert_queued("C", TASK)

    def test_process_on_other_node_fails(self):
        with self.assertRaises(PreflightError):
            self.scn(procs=[{"pid": 1}]).assert_queued("C", TASK)


class UntouchedTest(unittest.TestCase):
    def setup(self):
        scn = FakeQueue()
        scn.envs["B"] = {"label": "B", "environment_id": "envB"}
        scn.r.inv = {"phase": "ready"}
        scn.r.fences[("local", "envB")] = "f"
        scn.tree_value = [{"pid": 2}]
        return scn, {"inventory": {"phase": "ready"}, "nomad": "nomad", "tree": [{"pid": 2}], "fence": "f"}

    def test_unchanged_passes_and_each_drift_fails(self):
        scn, before = self.setup()
        self.assertTrue(scn.assert_untouched("B", TASK, before)["fence_unchanged"])
        for mutate in (lambda s: s.r.inv.update(phase="cleanup_pending"), lambda s: setattr(s.r, "nomad_view", "other"),
                       lambda s: setattr(s, "tree_value", []), lambda s: s.r.fences.update({("local", "envB"): "g"})):
            scn, before = self.setup()
            mutate(scn)
            with self.assertRaises(PreflightError):
                scn.assert_untouched("B", TASK, before)


class FenceOrderProbeTest(unittest.TestCase):
    def run_probe(self, a_fence, c_fence):
        scn = FakeQueue()
        env_a, env_c = {"label": "A", "environment_id": "envA"}, {"label": "C", "environment_id": "envC"}
        scn.r.fences = {("local", "envA"): "a"} if a_fence else {}
        if c_fence:
            scn.r.fences[("local", "envC")] = "c"
        scn.production_settled = lambda env, pending: {"settled": True}
        return scn.settle_with_fence_order(env_a, env_c, "A")

    def test_c_fence_after_a_gone_is_allowed_and_a_alone_is_fine(self):
        self.assertEqual(self.run_probe(False, True), {"settled": True})
        self.assertEqual(self.run_probe(True, False), {"settled": True})

    def test_c_fence_while_a_held_fails(self):
        with self.assertRaises(PreflightError):
            self.run_probe(True, True)


class FakeDriverScenario(FakeQueue):
    """Scripted scenario for the driver: records the order of calls, with switches for each failure."""
    fail = None
    log = []

    def __init__(self, runner, cfg, name, guard):
        super().__init__()
        self.cfg, self.name, self.guard = cfg, name, guard
        self.data = {"identities": {"A": {}, "B": {}, "C": {}}}
        self.r = runner
        FakeDriverScenario.log = self.log = []

    def verify_node_identity(self):
        return {}

    def node_ready(self):
        return True

    def verify_two_nodes(self):
        return {"nodes": {}}

    def size_memory(self):
        return {"memory_mb": 600, "memory_max_mb": 600}

    def await_placed(self, label, task):
        node = {"A": "n1", "B": "n2", "C": "n2" if self.fail == "other-node" else "n1"}[label]
        if self.fail == "same-node" and label == "B":
            node = "n1"
        self.envs[label] = {"label": label, "environment_id": "env" + label, "job_id": JOB + label, "allocation_id": "al" + label, "node_id": node, "attach": "local"}
        self.log.append(f"placed-{label}")
        return {"inventory": {}, "nomad": "nomad", "tree": [], "fence": "fence" + label}

    def workspace_layout(self, env):
        return {"kind": "worktree", "attach": env["attach"]}

    def kandev_jobs(self):
        return set()

    def assert_queued(self, label, task):
        self.envs["C"] = {"label": "C", "environment_id": "envC", "job_id": JOB + "C", "allocation_id": None}
        self.log.append("queued")
        return {"blocked_evaluation": True}

    def assert_untouched(self, label, task, before):
        if self.fail == "b-drift" and "archive-a" in self.log:
            raise PreflightError("environment B inventory or allocation drifted")
        self.log.append(f"untouched-{label}")
        return {}

    def archive(self, task):
        self.log.append("archive-" + task["id"])
        if self.fail == "operator":
            self.guard.record("nomad_stop")

    def settle_with_fence_order(self, env, waiting, label):
        return {"phase": "absent"}

    def settle(self, env):
        return {"phase": "absent"}

    def assert_pool_cleaned(self, env, layout):
        self.log.append(f"cleaned-{env['label']}")
        return {}

    def job_allocs(self, job_id):
        return [{"ID": "x"}]

    def finish(self, report):
        return {"finished": True}


class DriverRunner:
    def __init__(self, fence_a_present=False):
        self.state = {"task_id": "a", "session_id": "sa"}
        self.fence_a_present = fence_a_present
        self.steps = []
        self.created = 0
        self.fence_env = None

    def step(self, name, fn):
        self.steps.append(name)
        return fn()

    def start_git_daemon(self): pass
    def start_backend(self, x): pass

    def setup_api_objects(self, cfg):
        self.cfg = cfg

    def create_task(self, title):
        self.created += 1
        return {"id": "b" if self.created == 1 else "c", "session_id": "s" + str(self.created)}

    def use_node(self, attach, root): pass

    def fence(self):
        return "fenceC" if self.fence_env == "envC" else None


class DriveTest(unittest.TestCase):
    def setUp(self):
        self._orig = pq.PoolQueueScenario
        pq.PoolQueueScenario = FakeDriverScenario
        FakeDriverScenario.fail = None
        self.ops = type("Ops", (), {name: staticmethod(lambda *a, **k: None) for name in wo07.OPERATOR_OPS})()
        self.cfg = {"memory_mb": 1, "pool": {"node_pool": "p"}}

    def tearDown(self):
        pq.PoolQueueScenario = self._orig

    def drive(self, fail=None):
        FakeDriverScenario.fail = fail
        runner = DriverRunner()
        return runner, pq.drive_pool_queue(runner, self.cfg, self.ops, Path("."))

    def test_happy_path_report_and_order(self):
        runner, report = self.drive()
        self.assertTrue(report["production_cleanup"])
        self.assertFalse(report["operator_cleanup_used"])
        self.assertEqual(report["scenario"], "pool-queue")
        self.assertEqual(report["sizing"]["memory_mb"], 600)
        self.assertEqual(runner.cfg["memory_mb"], 600)
        self.assertEqual(runner.cfg["pool"]["memory_max_mb"], 600)
        self.assertEqual(self.cfg, {"memory_mb": 1, "pool": {"node_pool": "p"}})
        log = FakeDriverScenario.log
        self.assertLess(log.index("queued"), log.index("archive-a"))
        self.assertLess(log.index("archive-a"), log.index("placed-C"))
        self.assertLess(log.index("cleaned-A"), log.index("placed-C"))
        self.assertEqual({"A", "B", "C"}, set(report["environments"]))

    def test_c_on_the_other_node_fails(self):
        with self.assertRaises(PreflightError):
            self.drive("other-node")

    def test_a_and_b_on_one_node_fails(self):
        with self.assertRaises(PreflightError):
            self.drive("same-node")

    def test_b_drift_fails(self):
        with self.assertRaises(PreflightError):
            self.drive("b-drift")

    def test_operator_cleanup_trips_the_guard(self):
        with self.assertRaises(PreflightError):
            self.drive("operator")

    def test_guard_is_restored_after_failure(self):
        original = self.ops.nomad_stop
        with self.assertRaises(PreflightError):
            self.drive("operator")
        self.assertIs(self.ops.nomad_stop, original)


class PlacementOrderingTest(unittest.TestCase):
    """_await_c_ready fence ordering and allocation count against a scripted scenario."""

    def build(self, a_fence, c_fence, allocs=1):
        scn = FakeDriverScenario(FakeRunner(), {}, "pool-queue", None)
        scn.envs["C"] = {"label": "C", "environment_id": "envC", "job_id": "j", "node_id": "n1"}
        env_a = {"label": "A", "environment_id": "envA", "node_id": "n1"}
        scn.r.fences = {}
        if a_fence:
            scn.r.fences[("local", "envA")] = "a"
        if c_fence:
            scn.r.fences[("local", "envC")] = "c"
        scn.job_allocs = lambda job: [{"ID": "x"}] * allocs
        scn.await_placed = lambda label, task: {"fence": "c"}
        return scn, env_a

    def test_ordering_and_allocation_count(self):
        scn, env_a = self.build(False, True)
        pq._await_c_ready(scn, lambda name, fn: fn(), TASK, env_a)
        for a, c, allocs in ((True, True, 1), (False, False, 1), (False, True, 2)):
            scn, env_a = self.build(a, c, allocs)
            with self.assertRaises(PreflightError, msg=str((a, c, allocs))):
                pq._await_c_ready(scn, lambda name, fn: fn(), TASK, env_a)


if __name__ == "__main__":
    unittest.main()
