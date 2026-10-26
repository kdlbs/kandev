import sys
import signal
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from nomad_response_loss_controller import BarrierError, decide_barrier, guarded_crash


class RegistrySpy:
    def __init__(self, identity):
        self.identity = identity
        self.crash_calls = 0

    def _process_identity(self, _pid):
        return self.identity

    def crash(self, *_args, **_kwargs):
        self.crash_calls += 1
        return "crashed"


class ResponseLossControllerTest(unittest.TestCase):
    def setUp(self):
        self.record = {"pid": 4242, "started_at": "captured-start", "pgid": 4242}
        self.child_identity = ("child-start", 4242, 4242, "/fixture/bin/kandev")
        self.snapshot = {"phase": "registered"}
        self.marker = {"schema": 1, "committed_http_status": 200, "dropped_at_unix_ns": 1_000_000_000}

    def _crash(self, decision, registry, *, child_identity=None, child_reader=None, signals=None):
        return guarded_crash(
            decision, registry, "backend", self.record,
            child_pid=5252,
            child_identity=child_identity or self.child_identity,
            child_identity_reader=child_reader or (lambda _pid: self.child_identity),
            signal_process=((lambda pid, sig: signals.append((pid, sig))) if signals is not None else lambda *_args: None),
        )

    def test_no_marker_never_signals_backend(self):
        registry = RegistrySpy(("captured-start", 4242))
        decision = decide_barrier(None, self.snapshot, now_ns=1_100_000_000, max_age_ns=4_000_000_000)
        self.assertEqual(decision, "wait")
        with self.assertRaises(BarrierError):
            self._crash(decision, registry)
        self.assertEqual(registry.crash_calls, 0)

    def test_backend_identity_mismatch_never_signals(self):
        registry = RegistrySpy(("reused-pid-start", 4242))
        decision = decide_barrier(self.marker, self.snapshot, now_ns=1_100_000_000, max_age_ns=4_000_000_000)
        self.assertEqual(decision, "crash_exact_backend")
        with self.assertRaisesRegex(BarrierError, "identity changed"):
            self._crash(decision, registry)
        self.assertEqual(registry.crash_calls, 0)

    def test_late_marker_never_signals_backend(self):
        registry = RegistrySpy(("captured-start", 4242))
        decision = decide_barrier(self.marker, self.snapshot, now_ns=6_000_000_000, max_age_ns=4_000_000_000)
        self.assertEqual(decision, "late_marker")
        with self.assertRaises(BarrierError):
            self._crash(decision, registry)
        self.assertEqual(registry.crash_calls, 0)

    def test_already_ready_runtime_never_signals_backend(self):
        registry = RegistrySpy(("captured-start", 4242))
        decision = decide_barrier(self.marker, {"phase": "ready"}, now_ns=1_100_000_000, max_age_ns=4_000_000_000)
        self.assertEqual(decision, "already_ready")
        with self.assertRaises(BarrierError):
            self._crash(decision, registry)
        self.assertEqual(registry.crash_calls, 0)

    def test_crash_targets_proven_child_then_exact_registered_launcher(self):
        registry = RegistrySpy(("captured-start", 4242))
        sent = []
        result = self._crash("crash_exact_backend", registry, signals=sent)
        self.assertEqual(result, "crashed")
        self.assertEqual(sent, [(4242, signal.SIGSTOP), (5252, signal.SIGKILL)])
        self.assertEqual(registry.crash_calls, 1)

    def test_child_parent_or_start_identity_mismatch_never_signals(self):
        registry = RegistrySpy(("captured-start", 4242))
        sent = []
        with self.assertRaisesRegex(BarrierError, "child identity changed"):
            self._crash(
                "crash_exact_backend", registry,
                child_reader=lambda _pid: ("reused-child", 4242, 9999, "/fixture/bin/kandev"),
                signals=sent,
            )
        self.assertEqual(sent, [])
        self.assertEqual(registry.crash_calls, 0)

    def test_child_identity_change_after_pause_resumes_launcher_without_killing(self):
        registry = RegistrySpy(("captured-start", 4242))
        sent = []
        reads = 0
        def identity_reader(_pid):
            nonlocal reads
            reads += 1
            if reads == 1:
                return self.child_identity
            return ("changed-after-pause", 4242, 4242, "/fixture/bin/kandev")
        with self.assertRaisesRegex(BarrierError, "changed while launcher was paused"):
            self._crash("crash_exact_backend", registry, child_reader=identity_reader, signals=sent)
        self.assertEqual(sent, [(4242, signal.SIGSTOP), (4242, signal.SIGCONT)])
        self.assertEqual(registry.crash_calls, 0)


if __name__ == "__main__":
    unittest.main()
