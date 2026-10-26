import dataclasses
import hashlib
import json
import os
import socket
import time
import unittest
from pathlib import Path
from types import SimpleNamespace

import nomad_native_e2e as base
import nomad_native_e2e_bootstrap as bootstrap
import nomad_native_e2e_fixture as fixture
import nomad_native_e2e_wo06 as wo06
from nomad_native_e2e import PreflightError, main
from test_nomad_native_e2e_fixture import (ALLOC, ENV_ID, FINGERPRINT, JOB, NODE, TOKEN, FixtureBase, World)

JOB_B = "kandev-def456"
ALLOC_B = "55555555-5555-4555-8555-5555555555bb"
ENV_B = "11111111-1111-4111-8111-1111111111bb"
STARTED_A = "Sun Oct 25 10:00:00 2026"
STARTED_B = "Sun Oct 25 10:30:00 2026"
TERMINAL = ("complete", "failed", "lost")


class Sim(World):
    """Offline model of one Nomad node, the worker's processes and fence, and the fixture backend's inventory."""

    def __init__(self, root_parent, **flags):
        super().__init__(root_parent)
        self.flags = flags
        self.ticks, self.pending = 0, []
        self.node = {
            "ID": NODE, "Status": "ready", "SchedulingEligibility": "eligible", "Datacenter": "dc1",
            "Attributes": {"kernel.name": "linux", "cpu.arch": "amd64"},
            "NodeResources": {"Cpu": {"CpuShares": flags.get("total_cpu", 1000)}, "Memory": {"MemoryMB": flags.get("total_mem", 4096)}},
            "ReservedResources": {"Cpu": {"CpuShares": 100}, "Memory": {"MemoryMB": 96}},
        }
        self.jobs, self.allocs, self.procs = {}, {}, []
        self.prompts, self.flapped = [], False
        self.fence_text = None
        self.stops, self.eligibility_calls, self.kills, self.relays, self.titles = [], [], [], [], []
        self.b_created = False
        self.free_ports = iter(range(43111, 43200))

    # -- time ----------------------------------------------------------------------------------------
    def sleep(self, seconds):
        self.clock.sleep(seconds)
        self.ticks += 1
        due = [item for item in self.pending if item[0] <= self.ticks]
        self.pending = [item for item in self.pending if item[0] > self.ticks]
        for _, fn in due:
            fn()
        self.reschedule()

    def later(self, ticks, fn):
        self.pending.append((self.ticks + ticks, fn))

    # -- entities ---------------------------------------------------------------------------------------
    def _alloc(self, alloc_id, job, status, exit_code=None, events=None, restarts=0):
        cpu, mem = self.flags.get("alloc_cpu", 500), self.flags.get("alloc_mem", 512)
        evs = list(events or ([] if exit_code is None else [{"Type": "Terminated", "ExitCode": exit_code, "Details": {"exit_code": str(exit_code)}}]))
        alloc = {
            "ID": alloc_id, "NodeID": NODE, "JobID": job, "ClientStatus": status,
            "AllocatedResources": {"Tasks": {"agentctl": {"Cpu": {"CpuShares": cpu}, "Memory": {"MemoryMB": mem}}}},
            "TaskStates": {"agentctl": {"State": "running" if status == "running" else "dead", "Restarts": restarts, "Events": evs}},
        }
        self.allocs[alloc_id] = alloc
        self.jobs[job]["alloc_ids"].append(alloc_id)
        return alloc

    def _start_a(self):
        self.jobs[JOB] = {"status": "running", "alloc_ids": [], "env": ENV_ID, "alloc": ALLOC}
        self._alloc(ALLOC, JOB, "running")
        self.fence_text = f"owner job={JOB} env={ENV_ID}\n"
        if not self.flags.get("no_executor"):
            self.procs.append({"pid": 190, "ppid": 1, "pgid": 190, "started": STARTED_A, "env": "A",
                               "command": f'/usr/local/bin/nomad executor {{"LogFile":"/alloc/{ALLOC}/agentctl/executor.out"}}'})
        agentctl_ppid = 1 if self.flags.get("no_executor") else 190
        rows = [(200, agentctl_ppid, "agentctl"), (201, 200, "acp"), (202, 201, "child"), (203, 202, "grandchild")]
        for pid, ppid, role in rows:
            command = f"/alloc/{ALLOC}/{role} --job {JOB}" if role in ("agentctl", "acp") else f"/usr/bin/python3 {role}.py"
            if role == "agentctl" and self.flags.get("agentctl_renamed"):
                command = f"/alloc/{ALLOC}/mystery --job {JOB}"
            if self.flags.get("no_children") and role in ("child", "grandchild"):
                continue
            if self.flags.get("lazy_tree") and role != "agentctl":
                continue
            self.procs.append({"pid": pid, "ppid": ppid, "pgid": 200, "started": STARTED_A, "command": command, "env": "A"})
        self.reschedule()

    def _state(self, job, alloc, env, phase):
        return {
            "environment_id": env, "generation": 1, "operation_id": "op-" + job, "execution_id": "ex-" + job, "job_id": job,
            "job_create_index": 5, "job_definition_index": 6, "allocation_id": alloc, "node_id": NODE,
            "workspace_path": "/ws/environments/env/g1", "server_identity": "srv", "phase": phase, "workspace_input_captured": True,
        }

    def _create_b(self, body):
        self.titles.append(body["title"])
        self.b_created = True
        self.jobs[JOB_B] = {"status": "pending", "alloc_ids": [], "env": ENV_B, "alloc": None}
        db = self._db()
        db.execute("INSERT INTO executors_running VALUES ('sess2','task2','nomad','ex-b',?)", (json.dumps({"nomad_environment_state": self._state(JOB_B, None, ENV_B, "creating")}),))
        db.commit()
        db.close()
        if self.flags.get("leader_cmd_swap"):
            next(p for p in self.procs if p["pid"] == 200)["command"] = f'/usr/local/bin/nomad executor {{"alloc":"{ALLOC}"}}'
        if self.flags.get("b_early"):
            self._alloc(ALLOC_B, JOB_B, "running")
        self.reschedule()
        return {"id": "task2", "session_id": "sess2"}

    def reschedule(self):
        if not self.b_created or self.jobs[JOB_B]["alloc_ids"] or self.jobs[JOB_B]["status"] == "dead" or self.flags.get("b_never"):
            return
        if not self.flags.get("ignore_eligibility") and self.node["SchedulingEligibility"] != "eligible":
            return
        if self.node["Status"] != "ready":
            return
        if any(self.allocs[a]["ClientStatus"] not in TERMINAL for a in self.jobs[JOB]["alloc_ids"]):
            return
        self._place_b()

    def _place_b(self):
        if self.fence_text is not None:
            code = self.flags.get("b_exit", 73)
            events = [{"Type": "Terminated", "ExitCode": code, "Details": {"exit_code": str(code)}}]
            if self.flags.get("b_restarting"):
                events.append({"Type": "Restarting"})
            self._alloc(ALLOC_B, JOB_B, "failed", exit_code=code, events=events)
            if self.flags.get("b_dup"):
                self.later(1, lambda: self._alloc("55555555-5555-4555-8555-5555555555cc", JOB_B, "failed", exit_code=73))
            return
        self._alloc(ALLOC_B, JOB_B, "running")
        text = f"owner job={JOB_B} env={ENV_B}\n"
        self.fence_text = f"owner job=other env={ENV_B}\n" if self.flags.get("b_fence_wrong") else text
        for pid, ppid, role in ((300, 1, "agentctl"), (301, 300, "acp")):
            self.procs.append({"pid": pid, "ppid": ppid, "pgid": 300, "started": STARTED_B, "command": f"/alloc/{ALLOC_B}/{role} --job {JOB_B}", "env": "B"})
        if self.flags.get("b_dup"):
            self._alloc("55555555-5555-4555-8555-5555555555cc", JOB_B, "running")

    # -- injected callables ------------------------------------------------------------------------------
    def api_factory(self, port):
        inner = super().api_factory(port)

        def call(method, path, body=None):
            if method == "POST" and path == "/tasks" and self.task_created:
                return self._create_b(body)
            if method == "POST" and path == "/tasks":
                self.titles.append(body["title"])
            result = inner(method, path, body)
            if method == "POST" and path == "/tasks" and not self.jobs:
                self._start_a()
            return result

        return call

    def nomad_factory(self, address, token, namespace, region):
        assert token == TOKEN

        def call(path):
            self.nomad_paths.append(path)
            if path == f"/v1/node/{NODE}":
                if self.flags.get("node_unregistered_polls", 0) > getattr(self, "node_404s", 0):
                    self.node_404s = getattr(self, "node_404s", 0) + 1
                    raise PreflightError(f"Nomad GET {path} failed with status 404")
                return json.loads(json.dumps(self.node))
            if path.startswith("/v1/var/"):
                self.var_reads = getattr(self, "var_reads", []) + [path]
                if self.flags.get("marker") or (self.flags.get("marker_late") and self.node["SchedulingEligibility"] == "ineligible"):
                    return {"Path": path, "Items": {}}
                raise PreflightError("Nomad GET /v1/var/x failed with status 404")
            if path == "/v1/jobs?prefix=kandev-":
                return [{"ID": job} for job in self.jobs]
            parts = path.strip("/").split("/")
            if parts[:2] == ["v1", "allocation"]:
                return json.loads(json.dumps(self.allocs[parts[2]]))
            if parts[:2] == ["v1", "job"]:
                job = parts[2]
                if job not in self.jobs:
                    raise PreflightError(f"Nomad GET /v1/job/{job} failed with status 404")
                entry = self.jobs[job]
                if len(parts) == 3:
                    return {"ID": job, "Status": entry["status"], "CreateIndex": 5, "JobModifyIndex": 6}
                if parts[3] == "allocations":
                    return [{"ID": a, "ClientStatus": self.allocs[a]["ClientStatus"]} for a in entry["alloc_ids"]]
                if parts[3] == "evaluations":
                    if entry["alloc_ids"] or job == JOB:
                        return [{"Status": "complete"}]
                    if self.flags.get("no_blocked_eval"):
                        return [{"Status": "complete"}]
                    return [{"Status": "complete", "FailedTGAllocs": {"g": {"DimensionExhausted": {self.flags.get("eval_dim", "cpu"): 1}}}}, {"Status": "blocked"}]
            raise AssertionError(f"unexpected Nomad read {path}")

        return call

    def ssh_factory(self, ssh):
        self.ssh_cfg = ssh

        def call(command):
            self.ssh_commands.append(command)
            if "rmdir" in command:
                if self.fence_text is None:
                    return 3, ""
                expected = command.split('[ "$h" = ')[1].split(" ]")[0]
                if hashlib.sha256(self.fence_text.encode()).hexdigest() != expected:
                    return 4, ""
                self.fence_text = None
                return 0, ""
            if "active-environment" in command:
                return (3, "") if self.fence_text is None else (0, self.fence_text)
            if command.startswith("kill -KILL "):
                self._kill(int(command.split()[-1]))
                return 0, ""
            if command.startswith("LC_ALL=C ps -p "):
                pid = int(command.split()[3])
                return 0, "".join(self._row(p) for p in self.procs if p["pid"] == pid)
            if command.startswith("LC_ALL=C ps -eww"):
                transient = ""
                if self.flags.get("flap") and not self.flapped and any(p["pid"] == 200 for p in self.procs):
                    self.flapped = True
                    transient = self._row({"pid": 210, "ppid": 200, "pgid": 200, "started": STARTED_A, "command": "/usr/bin/helper --short-lived"})
                return 0, transient + "".join(self._row(p) for p in self.procs) + "    1     0     1 Sun Oct 25 09:00:00 2026 /sbin/init\n"
            return super_head(self, command)

        return call

    @staticmethod
    def _row(p):
        return f"  {p['pid']}  {p['ppid']}  {p['pgid']} {p['started']} {p['command']}\n"

    def _kill(self, pid):
        self.kills.append(pid)
        self.procs = [p for p in self.procs if p["pid"] != pid]
        if pid == 200:
            self.allocs[ALLOC]["ClientStatus"] = "failed"
            self.allocs[ALLOC]["TaskStates"]["agentctl"]["State"] = "dead"
            self.reschedule()

    def prompt(self, payload):
        self.prompts.append(payload)
        for pid, ppid, role in ((201, 200, "acp"), (202, 201, "child"), (203, 202, "grandchild")):
            if self.flags.get("lazy_tree") and not any(p["pid"] == pid for p in self.procs):
                command = f"/alloc/{ALLOC}/acp --job {JOB}" if role == "acp" else f"/usr/bin/python3 {role}.py"
                self.procs.append({"pid": pid, "ppid": ppid, "pgid": 200, "started": STARTED_A, "command": command, "env": "A"})
        return {"accepted": True, "message_id": "m1"}

    def nomad_stop(self, address, token, namespace, job_id):
        self.stops.append((address, namespace, job_id))
        assert token == TOKEN
        if self.flags.get("stop_ignored") and job_id == self.flags["stop_ignored"]:
            return {}
        env = self.jobs[job_id]["env"]
        if not self.flags.get("sticky_status"):
            self.jobs[job_id]["status"] = "dead"
        for alloc_id in self.jobs[job_id]["alloc_ids"]:
            if self.allocs[alloc_id]["ClientStatus"] == "running":
                self.allocs[alloc_id]["ClientStatus"] = "complete"
        if not self.flags.get("linger"):
            self.procs = [p for p in self.procs if p["env"] != ("A" if env == ENV_ID else "B")]
        if self.flags.get("fence_swap_on_stop") and job_id == JOB:
            self.fence_text = "owner job=intruder\n"
        self.reschedule()
        return {}

    def set_eligibility(self, address, token, region, node_id, eligible):
        self.eligibility_calls.append(eligible)
        assert node_id == NODE
        if not (self.flags.get("eligibility_stuck") or (eligible and self.flags.get("restore_stuck"))):
            self.node["SchedulingEligibility"] = "eligible" if eligible else "ineligible"
        self.reschedule()

    def make_relay(self, listen, upstream):
        relay = FakeRelay(self, listen, upstream)
        self.relays.append(relay)
        return relay

    # -- partition model -----------------------------------------------------------------------------------
    def on_partition(self):
        if self.flags.get("no_isolation"):
            return

        def isolate():
            self.node["Status"] = "disconnected"
            self.allocs[ALLOC]["ClientStatus"] = "lost" if self.flags.get("lost_during") else "unknown"
            if self.flags.get("replaced"):
                self._alloc("55555555-5555-4555-8555-5555555555dd", JOB, "pending")
            if self.flags.get("tree_changes"):
                self.procs = [p for p in self.procs if p["pid"] != 201]
            if self.flags.get("new_job"):
                self.jobs["kandev-zzz999"] = {"status": "pending", "alloc_ids": [], "env": "Z", "alloc": None}
            if self.flags.get("new_message"):
                self.extra_message = True
            if self.flags.get("backend_swap"):
                next(r for r in self.records if r["label"] == fixture.PROCESS_LABEL)["pid"] = 9999
                self.alive.add(9999)
            if self.flags.get("inventory_changed"):
                db = self._db()
                row = json.loads(db.execute("SELECT metadata FROM executors_running WHERE session_id='sess1'").fetchone()[0])
                row["nomad_environment_state"]["server_identity"] = "other"
                db.execute("UPDATE executors_running SET metadata=? WHERE session_id='sess1'", (json.dumps(row),))
                db.commit()
                db.close()

        self.later(2, isolate)

    def on_heal(self):
        def reconnect():
            self.node["Status"] = "ready"
            if self.allocs[ALLOC]["ClientStatus"] in ("unknown",):
                self.allocs[ALLOC]["ClientStatus"] = "lost" if self.flags.get("lost_after_heal") else "running"

        self.later(1, reconnect)

    def ops(self):
        return dataclasses.replace(
            super().ops(), sleep=self.sleep, nomad_stop=self.nomad_stop, prompt=self.prompt, nomad_set_eligibility=self.set_eligibility,
            relay_factory=self.make_relay, scenario_timeout=60.0, poll_interval=1.0,
        )


def super_head(sim, command):
    assert "rev-parse HEAD" in command, command
    return sim.head_code, sim.checkout_heads


class FakeRelay:
    def __init__(self, sim, listen, upstream):
        self.sim, self.listen, self.upstream, self.partitioned, self.calls = sim, listen, upstream, False, []

    def start(self):
        self.calls.append("start")

    def partition(self):
        self.partitioned = True
        self.calls.append("partition")
        self.sim.on_partition()

    def heal(self):
        self.partitioned = False
        self.calls.append("heal")
        self.sim.on_heal()

    def stop(self):
        self.calls.append("stop")


class Wo06Base(FixtureBase):
    PARTITION = {"listen": "127.0.0.1:45001", "upstream": "127.0.0.1:45002"}

    def make(self, **flags):
        self.world = Sim(self.parent, **flags)
        return self.world

    def run_scn(self, scenario, partition=True, **flags):
        sim = self.make(**flags)
        if partition:
            self.config["partition"] = dict(self.PARTITION)
        return wo06.run_scenario(
            self.config, scenario, ops=sim.ops(), out=self.out.append, err=self.err.append,
            run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout,
        )

    def assert_failed(self, code, message, step=None):
        self.assertEqual(code, 2, "\n".join(self.err))
        evidence = self.evidence()
        self.assertEqual(evidence["status"], "failed")
        self.assertIn(message, evidence["error"])
        if step:
            self.assertEqual(evidence["failed_step"], step)
        # first failure stops: nothing runs after the failed step, and the fixture is kept with its cleanup command
        self.assertEqual(evidence["steps"][-1]["status"], "failed")
        self.assertEqual(sum(item["status"] == "failed" for item in evidence["steps"]), 1)
        self.assertTrue(any(line.startswith("operator cleanup: python3 scripts/nomad_native_e2e.py cleanup-fixture --fixture-root ") for line in self.out))
        self.assertTrue((self.fixture_root() / "fixture.json").exists())
        return evidence

    def cleanup(self, ops=None):
        out, err = [], []
        code = fixture.run_cleanup_fixture(str(self.fixture_root()), ops=ops or self.world.ops(), out=out.append, err=err.append)
        return code, out, err


class BootstrapPartitionTest(Wo06Base):
    def test_partition_is_optional_and_validated(self):
        self.assertIsNone(self.validate()["partition"])
        self.config["partition"] = dict(self.PARTITION)
        self.assertEqual(self.validate()["partition"], self.PARTITION)

    def test_partition_rejections(self):
        bad = [
            {"listen": "127.0.0.1:45001"}, {"listen": "127.0.0.1:45001", "upstream": "127.0.0.1:45002", "x": 1},
            {"listen": "10.0.0.1:45001", "upstream": "127.0.0.1:45002"}, {"listen": "127.0.0.1:45001", "upstream": "example.com:4647"},
            {"listen": "127.0.0.1:45001", "upstream": "127.0.0.1:45001"}, {"listen": "127.0.0.1:8817", "upstream": "127.0.0.1:45002"},
            {"listen": "127.0.0.1:45001", "upstream": "127.0.0.1:8817"}, {"listen": "127.0.0.1:4646", "upstream": "127.0.0.1:45002"},
            {"listen": "127.0.0.1:0", "upstream": "127.0.0.1:45002"}, {"listen": "127.0.0.1:99999", "upstream": "127.0.0.1:45002"}, "x",
        ]
        for value in bad:
            self.config["partition"] = value
            with self.assertRaises(PreflightError, msg=str(value)):
                self.validate()

    def test_disconnect_requires_partition_and_creates_nothing(self):
        code = self.run_scn("disconnect", partition=False)
        self.assertEqual(code, 2)
        self.assertIn("requires a partition", "\n".join(self.err))
        self.assertEqual(list(self.parent.iterdir()), [])

    def test_mutating_scenarios_require_loopback_http_nomad(self):
        for address in ("https://nomad.example.com:4646", "https://127.0.0.1:4646"):
            for scenario in wo06.SCENARIOS:
                self.setUp()
                self.config["nomad"]["address"] = address
                code = self.run_scn(scenario)
                self.assertEqual(code, 2)
                self.assertIn("loopback http", "\n".join(self.err))
                self.assertEqual(list(self.parent.iterdir()), [])

    def test_live_port_is_refused(self):
        sim = self.make()
        sim.free_ports = iter([bootstrap.LIVE_KANDEV_PORT])
        self.config["partition"] = dict(self.PARTITION)
        code = wo06.run_scenario(self.config, "admission", ops=sim.ops(), out=self.out.append, err=self.err.append,
                                 run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assertEqual(code, 2)
        self.assertIn("live Kandev port", "\n".join(self.err))


class AdmissionTest(Wo06Base):
    def test_success_path_and_evidence(self):
        self.assertEqual(self.run_scn("admission"), 0, "\n".join(self.err))
        evidence = self.evidence()
        self.assertEqual(evidence["status"], "passed")
        self.assertFalse(evidence["production_cleanup"])
        self.assertEqual(evidence["release"], "operator-teardown")
        self.assertTrue(any("not hard OS limits" in item for item in evidence["limitations"]))
        self.assertTrue(any("quarantine" in item for item in evidence["limitations"]))
        self.assertTrue(any("ACL" in item for item in evidence["limitations"]))
        self.assertEqual(evidence["capacity"]["exhausted_dimensions"], ["cpu"])
        self.assertEqual(evidence["identities"]["A"]["allocated_resources"], {"cpu_mhz": 500, "memory_mb": 512})
        self.assertEqual(evidence["identities"]["A"]["allocation_id"], ALLOC)
        self.assertEqual(evidence["identities"]["B"]["allocation_id"], ALLOC_B)
        self.assertNotEqual(evidence["identities"]["A"]["fence_owner_sha256"], evidence["identities"]["B"]["fence_owner_sha256"])
        self.assertEqual(evidence["eligibility_window"]["b_allocations_during_window"], 0)
        self.assertEqual(evidence["eligibility_window"]["restored"], True)
        self.assertEqual(self.world.eligibility_calls, [False, True])
        self.assertEqual(self.world.node["SchedulingEligibility"], "eligible")
        self.assertEqual(evidence["cleanup"]["jobs"], {"A": "dead", "B": "dead"})
        self.assertEqual(self.world.titles, ["Nomad pre-ready recovery fixture", "Nomad admission fixture B"])
        names = [item["name"] for item in evidence["steps"]]
        self.assertEqual(names[names.index("release-a-operator-teardown")], "release-a-operator-teardown")
        self.assertTrue(all(item["status"] == "passed" and "duration_s" in item for item in evidence["steps"]))
        self.assertFalse((self.fixture_root() / "home" / ".ssh" / "id_ed25519").exists())
        self.assertEqual(self.world.records, [])

    def test_nomad_is_only_stopped_with_purge_false_for_recorded_jobs(self):
        self.assertEqual(self.run_scn("admission"), 0, "\n".join(self.err))
        self.assertEqual([item[2] for item in self.world.stops], [JOB, JOB_B])
        self.assertTrue(all(item[0] == "http://127.0.0.1:4646" for item in self.world.stops))

    def test_secrets_are_scrubbed_everywhere(self):
        self.assertEqual(self.run_scn("admission"), 0)
        self.assertNotIn(TOKEN, self.everything_written())

    def test_no_fixed_sleeps_and_each_fence_is_removed_by_compare_owner(self):
        import inspect
        self.assertNotIn("time.sleep", inspect.getsource(wo06))
        self.assertEqual(self.run_scn("admission"), 0)
        self.assertEqual(sum("rmdir" in c for c in self.world.ssh_commands), 2)

    def test_capacity_one_does_not_fit(self):
        self.assert_failed(self.run_scn("admission", total_cpu=550), "does not fit", "capacity-precondition")
        self.assertNotIn(("POST", "/tasks"), self.world.api_calls)

    def test_capacity_two_fit(self):
        self.assert_failed(self.run_scn("admission", total_cpu=4000, total_mem=8192), "both fit", "capacity-precondition")

    def test_capacity_memory_dimension_alone_exhausts(self):
        self.assertEqual(self.run_scn("admission", total_cpu=4000, total_mem=1000, alloc_mem=512, eval_dim="memory"), 0, "\n".join(self.err))
        self.assertEqual(self.evidence()["capacity"]["exhausted_dimensions"], ["memory"])

    def test_node_datacenter_mismatch_is_refused_before_anything_else(self):
        sim = self.make()
        sim.node["Datacenter"] = "dc2"
        code = wo06.run_scenario(self.config, "admission", ops=sim.ops(), out=self.out.append, err=self.err.append,
                                 run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assert_failed(code, "datacenter differs", "verify-node-identity")
        self.assertEqual(sim.eligibility_calls, [])

    def test_node_platform_mismatch_is_refused(self):
        sim = self.make()
        sim.node["Attributes"]["cpu.arch"] = "arm64"
        code = wo06.run_scenario(self.config, "admission", ops=sim.ops(), out=self.out.append, err=self.err.append,
                                 run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assert_failed(code, "platform differs", "verify-node-identity")

    def test_node_not_ready_fails_first(self):
        sim = self.make()
        sim.node["SchedulingEligibility"] = "ineligible"
        code = wo06.run_scenario(self.config, "admission", ops=sim.ops(), out=self.out.append, err=self.err.append,
                                 run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assert_failed(code, "not ready and eligible", "poll-node-ready")

    def test_allocation_resources_must_equal_the_request(self):
        self.assert_failed(self.run_scn("admission", alloc_cpu=400), "differ from the request", "await-a-running")

    def test_b_unexpectedly_allocated(self):
        self.assert_failed(self.run_scn("admission", b_early=True), "was allocated although", "queue-b-and-assert-a-unchanged")

    def test_b_never_blocked(self):
        self.assert_failed(self.run_scn("admission", no_blocked_eval=True), "never showed a blocked evaluation", "queue-b-and-assert-a-unchanged")

    def test_blocked_eval_naming_another_dimension_is_not_proof(self):
        self.assert_failed(self.run_scn("admission", eval_dim="disk"), "never showed a blocked evaluation", "queue-b-and-assert-a-unchanged")

    def test_a_drift_fence_changed(self):
        sim = self.make()
        original = sim._create_b

        def drift(body):
            sim.fence_text = "owner job=someone-else\n"
            return original(body)

        sim._create_b = drift
        code = wo06.run_scenario(self.config, "admission", ops=sim.ops(), out=self.out.append, err=self.err.append,
                                 run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assert_failed(code, "fence changed", "queue-b-and-assert-a-unchanged")

    def test_a_drift_allocation_not_running(self):
        sim = self.make()
        original = sim._create_b

        def drift(body):
            sim.allocs[ALLOC]["TaskStates"]["agentctl"]["State"] = "dead"
            return original(body)

        sim._create_b = drift
        code = wo06.run_scenario(self.config, "admission", ops=sim.ops(), out=self.out.append, err=self.err.append,
                                 run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assert_failed(code, "agentctl task is not running", "queue-b-and-assert-a-unchanged")

    def test_a_drift_process_tree_changed(self):
        sim = self.make()
        original = sim._create_b

        def drift(body):
            sim.procs = [p for p in sim.procs if p["pid"] != 201]
            return original(body)

        sim._create_b = drift
        code = wo06.run_scenario(self.config, "admission", ops=sim.ops(), out=self.out.append, err=self.err.append,
                                 run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assert_failed(code, "tree or fence changed", "queue-b-and-assert-a-unchanged")

    def test_compare_owner_mismatch_refuses_removal_and_leaves_node_ineligible(self):
        evidence = self.assert_failed(self.run_scn("admission", fence_swap_on_stop=True), "no longer matches", "release-a-operator-teardown")
        self.assertEqual(self.world.fence_text, "owner job=intruder\n")
        self.assertEqual(self.world.eligibility_calls, [False])
        self.assertEqual(self.world.node["SchedulingEligibility"], "ineligible")
        self.assertIn("nomad node eligibility -enable", evidence["eligibility_window"]["restore_with"])
        self.assertFalse(evidence["eligibility_window"]["restored"])

    def test_processes_not_gone_after_release(self):
        self.assert_failed(self.run_scn("admission", linger=True), "still present on the worker", "release-a-operator-teardown")

    def test_b_allocated_during_the_ineligible_window(self):
        self.assert_failed(self.run_scn("admission", ignore_eligibility=True), "while the node was ineligible", "release-a-operator-teardown")

    def test_b_never_placed(self):
        self.assert_failed(self.run_scn("admission", b_never=True), "was not placed and running", "observe-b-placement")

    def test_b_fence_not_owned_by_b(self):
        self.assert_failed(self.run_scn("admission", b_fence_wrong=True), "not owned by B", "observe-b-placement")

    def test_duplicate_allocation_for_b(self):
        self.assert_failed(self.run_scn("admission", b_dup=True), "more than one allocation", "observe-b-placement")

    def test_jobs_left_running_fail_the_final_assertion(self):
        self.assert_failed(self.run_scn("admission", sticky_status=True), "still running after the operator teardown", "teardown")

    def test_eligibility_not_restored(self):
        self.assert_failed(self.run_scn("admission", restore_stuck=True), "eligibility was not restored", "release-a-operator-teardown")


class CleanupFixtureTest(Wo06Base):
    def test_cleanup_after_failed_admission_run_is_complete_and_idempotent(self):
        self.assert_failed(self.run_scn("admission", linger=True), "still present", "release-a-operator-teardown")
        self.world.flags["linger"] = False
        code, out, err = self.cleanup()
        self.assertEqual(code, 0, err)
        evidence = self.evidence()
        self.assertEqual(evidence["cleanup"]["status"], "passed")
        self.assertFalse(evidence["cleanup"]["production_cleanup"])
        self.assertEqual(self.world.node["SchedulingEligibility"], "eligible")
        self.assertIsNone(self.world.fence_text)
        self.assertEqual(self.world.records, [])
        self.assertTrue(all(item[2] in (JOB, JOB_B) for item in self.world.stops))
        stops = len(self.world.stops)
        code, out, err = self.cleanup()
        self.assertEqual(code, 0, err)
        self.assertEqual(len(self.world.stops), stops)
        self.assertTrue(self.evidence()["cleanup"]["already_torn_down"])
        self.assertNotIn(TOKEN, self.everything_written())

    def test_cleanup_leaves_node_ineligible_when_the_fence_is_not_ours(self):
        self.assert_failed(self.run_scn("admission", fence_swap_on_stop=True), "no longer matches", "release-a-operator-teardown")
        code, out, err = self.cleanup()
        self.assertEqual(code, 0, err)
        report = self.evidence()["cleanup"]
        self.assertIn("left ineligible", report["eligibility"])
        self.assertIn("nomad node eligibility -enable", report["eligibility"])
        self.assertEqual(self.world.node["SchedulingEligibility"], "ineligible")
        self.assertEqual(self.world.fence_text, "owner job=intruder\n")

    def test_cleanup_after_failed_disconnect_kills_exact_survivors_and_reports_relay(self):
        self.assert_failed(self.run_scn("disconnect", lost_after_heal=True), "ClientStatus 'lost' after the heal", "heal-and-assert-original-allocation-survives")
        code, out, err = self.cleanup()
        self.assertEqual(code, 0, err)
        report = self.evidence()["cleanup"]
        self.assertIn("gone", report["relay"])
        self.assertEqual({1, 2} & set(self.world.kills), set())
        self.assertEqual(sorted(set(self.world.kills)), [200, 201, 202, 203])
        self.assertEqual(self.world.procs, [])
        self.assertIsNone(self.world.fence_text)


class QuarantineMarkerTest(Wo06Base):
    def test_preexisting_marker_aborts_preflight_before_anything_is_created(self):
        for scenario in wo06.SCENARIOS:
            self.setUp()
            code = self.run_scn(scenario, marker=True)
            self.assertEqual(code, 2)
            text = "\n".join(self.err)
            self.assertIn("quarantine marker kandev/quarantine/" + NODE, text)
            self.assertIn("nomad var purge", text)
            self.assertEqual(list(self.parent.iterdir()), [])
            self.assertEqual(self.world.eligibility_calls, [])
            self.assertEqual(self.world.api_calls, [])
            self.assertTrue(any(path.startswith("/v1/var/kandev%2Fquarantine%2F" + NODE) for path in self.world.var_reads))

    def test_marker_appearing_mid_run_refuses_strict_restore_and_leaves_node_ineligible(self):
        code = self.run_scn("admission", marker_late=True)
        self.assertEqual(code, 2)
        self.assertIn("quarantine marker", self.evidence()["error"])
        self.assertIn("nomad var purge", self.evidence()["error"])
        self.assertEqual(self.world.node["SchedulingEligibility"], "ineligible")
        self.assertEqual(self.world.eligibility_calls, [False])

    def test_cleanup_fixture_does_not_restore_while_marker_exists(self):
        self.assert_failed(self.run_scn("admission", linger=True), "still present", "release-a-operator-teardown")
        self.world.flags.update(linger=False, marker=True)
        code, out, err = self.cleanup()
        self.assertEqual(code, 0, err)
        report = self.evidence()["cleanup"]
        self.assertIn("left ineligible: quarantine marker", report["eligibility"])
        self.assertIn(f"nomad var purge -namespace", report["eligibility"])
        self.assertEqual(self.world.node["SchedulingEligibility"], "ineligible")
        self.assertEqual(self.world.eligibility_calls, [False])
        self.world.flags["marker"] = False
        self.assertEqual(self.world.eligibility_calls, [False])


class ProcessIdentityTest(Wo06Base):
    def test_executor_is_never_killed_or_labelled_agentctl(self):
        self.assertEqual(self.run_scn("disconnect"), 0, "\n".join(self.err))
        processes = self.evidence()["identities"]["A"]["processes"]
        self.assertEqual([item["role"] for item in processes], ["agentctl", "acp", "child", "grandchild"])
        self.assertNotIn(190, [item["pid"] for item in processes])
        self.assertNotIn(190, self.world.kills)
        self.assertEqual(sorted(set(self.world.kills)), [200, 201, 202, 203])

    def test_executor_only_tree_fails_the_snapshot(self):
        evidence = self.assert_failed(self.run_scn("disconnect", agentctl_renamed=True), "no identifiable agentctl", "await-a-running")
        self.assertEqual(self.world.kills, [])
        self.assertIn(190, [p["pid"] for p in self.world.procs])

    def test_kill_leader_refuses_a_process_that_is_no_longer_agentctl(self):
        self.assert_failed(self.run_scn("disconnect", leader_cmd_swap=True), "not the recorded agentctl", "kill-a-agentctl-leader")
        self.assertEqual(self.world.kills, [])

    def test_ps_rows_refuses_a_listing_it_cannot_parse(self):
        en_gb = "  200  1  200 Mon 26 Oct 15:54:21 2026 /alloc/x/agentctl\n"
        scn = wo06.Scenario.__new__(wo06.Scenario)
        scn.r = SimpleNamespace(ssh=lambda command: (0, en_gb))
        with self.assertRaisesRegex(PreflightError, "no row parsed"):
            scn.ps_rows()
        commands = []
        scn.r = SimpleNamespace(ssh=lambda command: (commands.append(command) or 0, ""))
        self.assertEqual(scn.ps_rows(), [])
        self.assertTrue(commands[0].startswith("LC_ALL=C ps "))

    def test_helper_classification(self):
        self.assertTrue(wo06._is_nomad_helper('/usr/local/bin/nomad executor {"x":1}'))
        self.assertTrue(wo06._is_nomad_helper("/usr/local/bin/nomad logmon"))
        self.assertTrue(wo06._is_nomad_helper("nomad agent -config x"))
        self.assertFalse(wo06._is_nomad_helper("/alloc/x/agentctl --job j"))
        self.assertTrue(wo06._is_agentctl("/alloc/x/agentctl --job j"))
        self.assertFalse(wo06._is_agentctl('/usr/local/bin/nomad executor {"a":"/alloc/x/agentctl"}'))
        self.assertFalse(wo06._is_agentctl("/usr/bin/python3 agentctl.py"))


class LiveRegressionTest(Wo06Base):
    def test_node_identity_waits_for_the_client_to_register(self):
        self.assertEqual(self.run_scn("admission", node_unregistered_polls=3), 0, "\n".join(self.err))
        self.assertEqual(self.world.node_404s, 3)

    def test_node_that_never_registers_fails_identity_with_a_clear_message(self):
        self.assert_failed(self.run_scn("admission", node_unregistered_polls=10**6), "did not register its node", "verify-node-identity")

    def test_content_addressed_agentctl_name_is_identified(self):
        digest = "ab" * 32
        self.assertTrue(wo06._is_agentctl(f"/alloc/x/local/agentctl-{digest} --job j"))
        self.assertTrue(wo06._is_agentctl("agentctl"))
        self.assertFalse(wo06._is_agentctl(f"/alloc/x/agentctl-{digest[:-1]} --job j"))
        self.assertFalse(wo06._is_agentctl(f"/alloc/x/agentctl-{digest.upper()} --job j"))
        self.assertFalse(wo06._is_agentctl("/alloc/x/agentctl-evil --job j"))

    def test_a_transient_startup_helper_is_never_recorded_for_admission(self):
        self.assertEqual(self.run_scn("admission", lazy_tree=True, flap=True), 0, "\n".join(self.err))
        self.assertEqual(self.evidence()["identities"]["A"]["roles"], ["agentctl"])
        self.assertNotIn(210, self.world.kills)

    def test_disconnect_prompts_once_to_start_the_managed_tree(self):
        self.assertEqual(self.run_scn("disconnect", lazy_tree=True), 0, "\n".join(self.err))
        self.assertEqual(len(self.world.prompts), 1)
        self.assertEqual([item["role"] for item in self.evidence()["identities"]["A"]["processes"]], ["agentctl", "acp", "child", "grandchild"])

    def test_tree_excludes_this_process_and_its_ancestors(self):
        rows = [{"pid": os.getpid(), "ppid": os.getppid()}, {"pid": os.getppid(), "ppid": 1}, {"pid": 5, "ppid": 1}]
        self.assertEqual(wo06._self_and_ancestors(rows), {os.getpid(), os.getppid()})

    def test_kill_group_refuses_the_runners_own_group(self):
        with self.assertRaises(wo06.PreflightError):
            fixture._default_kill_group(os.getpgrp())


class RelayFailureTest(Wo06Base):
    def test_failure_heals_and_keeps_the_relay_with_an_operator_note(self):
        evidence = self.assert_failed(self.run_scn("disconnect", lost_after_heal=True), "ClientStatus 'lost' after the heal", "heal-and-assert-original-allocation-survives")
        relay = self.world.relays[0]
        self.assertNotIn("stop", relay.calls)
        self.assertEqual(relay.calls[-1], "heal")
        self.assertFalse(relay.partitioned)
        self.assertIn("dies with it", evidence["relay_note"])
        self.assertIn("127.0.0.1:45002", evidence["relay_note"])
        self.assertIn("127.0.0.1:45002", "\n".join(self.err))

    def test_failure_during_partition_heals_the_relay(self):
        evidence = self.assert_failed(self.run_scn("disconnect", no_isolation=True), "never left ready", "partition-and-assert-nothing-changes")
        relay = self.world.relays[0]
        self.assertEqual(relay.calls[-1], "heal")
        self.assertNotIn("stop", relay.calls)
        self.assertFalse(evidence["relay_partitioned_at_exit"])

    def test_cleanup_fails_fast_with_the_relay_instruction_when_the_client_is_unreachable(self):
        self.assert_failed(self.run_scn("disconnect", lost_after_heal=True), "ClientStatus 'lost' after the heal", "heal-and-assert-original-allocation-survives")
        self.world.node["Status"] = "disconnected"
        self.world.allocs[ALLOC]["ClientStatus"] = "unknown"
        code, out, err = self.cleanup()
        self.assertNotEqual(code, 0)
        text = "\n".join(err)
        self.assertIn("upstream RPC address 127.0.0.1:45002", text)
        self.assertIn("cannot be acknowledged", text)


class DisconnectTest(Wo06Base):
    def test_success_path_and_evidence(self):
        self.assertEqual(self.run_scn("disconnect"), 0, "\n".join(self.err))
        evidence = self.evidence()
        self.assertEqual(evidence["status"], "passed")
        self.assertFalse(evidence["production_cleanup"])
        self.assertTrue(evidence["limitations"])
        relay = self.world.relays[0]
        self.assertEqual(relay.calls[:3], ["start", "partition", "heal"])
        self.assertEqual(relay.listen, ("127.0.0.1", 45001))
        self.assertEqual(relay.upstream, ("127.0.0.1", 45002))
        statuses = evidence["observed_statuses"]
        self.assertTrue(any(item["node"] == "disconnected" and item["allocation"] == "unknown" for item in statuses))
        self.assertEqual(evidence["identities"]["A"]["after_heal"], {"allocation_client_status": "running", "prompts_sent": 0})
        self.assertEqual([item["role"] for item in evidence["identities"]["A"]["processes"]], ["agentctl", "acp", "child", "grandchild"])
        self.assertEqual(evidence["identities"]["B"]["exit_code"], 73)
        self.assertEqual(evidence["identities"]["B"]["allocations"], 1)
        self.assertEqual(evidence["identities"]["B"]["reserved_ports_listener"], "not asserted")
        self.assertIn("backend is not running", evidence["cleanup"]["session_stop"])
        self.assertEqual(evidence["cleanup"]["relay"], "stopped")
        self.assertEqual(evidence["cleanup"]["jobs"], {"A": "dead", "B": "dead"})
        self.assertEqual(sorted(set(self.world.kills)), [200, 201, 202, 203])
        self.assertEqual([item[2] for item in self.world.stops], [JOB, JOB_B])
        self.assertEqual(self.world.eligibility_calls, [])
        self.assertNotIn(TOKEN, self.everything_written())

    def test_node_never_leaves_ready_means_bypass(self):
        self.assert_failed(self.run_scn("disconnect", no_isolation=True), "RPC bypass", "partition-and-assert-nothing-changes")

    def test_allocation_lost_during_partition(self):
        self.assert_failed(self.run_scn("disconnect", lost_during=True), "became 'lost' while the node was 'disconnected'", "partition-and-assert-nothing-changes")

    def test_allocation_replaced_during_partition(self):
        self.assert_failed(self.run_scn("disconnect", replaced=True), "2 allocations", "partition-and-assert-nothing-changes")

    def test_process_tree_changed_during_partition(self):
        self.assert_failed(self.run_scn("disconnect", tree_changes=True), "remote process tree of A changed", "partition-and-assert-nothing-changes")

    def test_new_kandev_job_during_partition(self):
        self.assert_failed(self.run_scn("disconnect", new_job=True), "unexpected new kandev job", "partition-and-assert-nothing-changes")

    def test_new_session_message_during_partition(self):
        self.assert_failed(self.run_scn("disconnect", new_message=True), "session messages changed", "partition-and-assert-nothing-changes")

    def test_backend_pid_changed_during_partition(self):
        self.assert_failed(self.run_scn("disconnect", backend_swap=True), "backend process changed", "partition-and-assert-nothing-changes")

    def test_inventory_changed_during_partition(self):
        self.assert_failed(self.run_scn("disconnect", inventory_changed=True), "durable inventory of A changed", "partition-and-assert-nothing-changes")

    def test_allocation_lost_after_heal(self):
        self.assert_failed(self.run_scn("disconnect", lost_after_heal=True), "ClientStatus 'lost' after the heal, expected running", "heal-and-assert-original-allocation-survives")

    def test_environment_without_children_cannot_prove_survival(self):
        self.assert_failed(self.run_scn("disconnect", no_children=True), "did not settle", "start-managed-process-tree")

    def test_b_exit_code_not_73(self):
        self.assert_failed(self.run_scn("disconnect", b_exit=1), "expected failed with exit code 73", "assert-b-rejected-and-a-survivors-intact")

    def test_b_restarted(self):
        self.assert_failed(self.run_scn("disconnect", b_restarting=True), "restarted or rescheduled", "assert-b-rejected-and-a-survivors-intact")

    def test_duplicate_allocation_for_b(self):
        self.assert_failed(self.run_scn("disconnect", b_dup=True), "more than one allocation", "await-b-start-fence-rejection")

    def test_a_survivor_dies(self):
        sim = self.make()
        original = sim._kill

        def kill(pid):
            original(pid)
            if pid == 200:
                sim.procs = [p for p in sim.procs if p["pid"] != 203]

        sim._kill = kill
        self.config["partition"] = dict(self.PARTITION)
        code = wo06.run_scenario(self.config, "disconnect", ops=sim.ops(), out=self.out.append, err=self.err.append,
                                 run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assert_failed(code, "child/grandchild processes of A died", "assert-b-rejected-and-a-survivors-intact")

    def test_jobs_left_running(self):
        self.assert_failed(self.run_scn("disconnect", sticky_status=True), "still running after the operator teardown", "teardown")

    def test_backend_must_be_gone_after_crash(self):
        sim = self.make()
        sim.crash_leaves = "alive"
        self.config["partition"] = dict(self.PARTITION)
        code = wo06.run_scenario(self.config, "disconnect", ops=sim.ops(), out=self.out.append, err=self.err.append,
                                 run=lambda *a, **k: SimpleNamespace(returncode=0, stdout="", stderr=""), checkout_root=self.checkout)
        self.assert_failed(code, "still alive", "crash-backend")


class MainDispatchTest(Wo06Base):
    def test_create_fixture_accepts_the_three_scenarios_and_rejects_others(self):
        err = []
        import contextlib, io
        for scenario in ("session", "recovery"):
            buf = io.StringIO()
            with contextlib.redirect_stderr(buf):
                self.assertEqual(main(["run", "--scenario", scenario, "--require-real", "--create-fixture", "--config", "/nonexistent"]), 2)
            self.assertIn("--create-fixture requires", buf.getvalue())
            self.assertIn("pre-ready-recovery|admission|disconnect", buf.getvalue())

    def test_admission_and_disconnect_dispatch_to_wo06(self):
        calls = []
        original = wo06.create_fixture_command
        wo06.create_fixture_command = lambda path, scenario: calls.append((path, scenario)) or 0
        try:
            self.assertEqual(main(["run", "--scenario", "admission", "--require-real", "--create-fixture", "--config", "c.json"]), 0)
            self.assertEqual(main(["run", "--scenario", "disconnect", "--require-real", "--create-fixture", "--config", "d.json"]), 0)
        finally:
            wo06.create_fixture_command = original
        self.assertEqual(calls, [("c.json", "admission"), ("d.json", "disconnect")])

    def test_attached_mode_is_not_offered(self):
        import contextlib, io
        for scenario in ("admission", "disconnect"):
            buf = io.StringIO()
            with contextlib.redirect_stderr(buf):
                self.assertEqual(main(["run", "--scenario", scenario, "--require-real", "--config", "/nonexistent"]), 2)
            self.assertIn("not implemented", buf.getvalue())

    def test_unreadable_config(self):
        import contextlib, io
        buf = io.StringIO()
        with contextlib.redirect_stderr(buf):
            self.assertEqual(wo06.create_fixture_command("/nonexistent.json", "admission"), 2)
        self.assertIn("config is unreadable", buf.getvalue())


class OpsDefaultsTest(unittest.TestCase):
    class Opener:
        def __init__(self):
            self.requests = []

        def open(self, request, timeout=0):
            self.requests.append(request)
            return SimpleNamespace(read=lambda: b"{}", __enter__=lambda s: s, __exit__=lambda *a: False)

    def patched(self):
        opener = self.Opener()

        class Response:
            def __init__(self, inner): pass
            def __enter__(self): return self
            def __exit__(self, *a): return False
            def read(self): return b"{}"

        class Wrapper:
            def open(self, request, timeout=0):
                opener.requests.append(request)
                return Response(request)

        original, fixture._NOMAD_OPENER = fixture._NOMAD_OPENER, Wrapper()
        self.addCleanup(setattr, fixture, "_NOMAD_OPENER", original)
        return opener

    def test_nomad_stop_is_a_delete_with_purge_false_never_true(self):
        opener = self.patched()
        fixture._default_nomad_stop("http://127.0.0.1:4646", "tok", "default", "kandev-abc123")
        request = opener.requests[0]
        self.assertEqual(request.get_method(), "DELETE")
        self.assertIn("purge=false", request.full_url)
        self.assertNotIn("purge=true", request.full_url)
        self.assertTrue(request.full_url.startswith("http://127.0.0.1:4646/v1/job/kandev-abc123?"))
        import inspect
        self.assertNotIn("purge=true", inspect.getsource(fixture))
        self.assertNotIn("purge=true", inspect.getsource(wo06))

    def test_nomad_stop_refuses_foreign_job_ids(self):
        self.patched()
        for bad in ("web-api", "kandev-", "kandev-../x", "kandev-abc/allocations"):
            with self.assertRaises(PreflightError):
                fixture._default_nomad_stop("http://127.0.0.1:4646", "tok", "default", bad)

    def test_eligibility_toggle_body_and_node_id_validation(self):
        opener = self.patched()
        fixture._default_nomad_set_eligibility("http://127.0.0.1:4646", "tok", "global", NODE, False)
        request = opener.requests[0]
        self.assertEqual(request.get_method(), "POST")
        self.assertEqual(json.loads(request.data), {"NodeID": NODE, "Eligibility": "ineligible"})
        with self.assertRaises(PreflightError):
            fixture._default_nomad_set_eligibility("http://127.0.0.1:4646", "tok", "global", "../x", True)

    def ssh(self, rows):
        calls = []

        def call(command):
            calls.append(command)
            if command.startswith("LC_ALL=C ps -p"):
                return 0, rows
            return 0, ""

        return call, calls

    def test_kill_exact_verifies_pid_start_and_needle_before_signalling(self):
        row = f"  200  1  200 {STARTED_A} /alloc/{ALLOC}/agentctl\n"
        ssh, calls = self.ssh(row)
        self.assertEqual(fixture._default_kill_exact(ssh, {"pid": 200, "started": STARTED_A}, [ALLOC]), "killed")
        self.assertEqual(calls[-1], "kill -KILL 200")
        ssh, calls = self.ssh(row)
        with self.assertRaises(PreflightError):
            fixture._default_kill_exact(ssh, {"pid": 200, "started": "Mon Oct 26 10:00:00 2026"}, [ALLOC])
        with self.assertRaises(PreflightError):
            fixture._default_kill_exact(ssh, {"pid": 200, "started": STARTED_A}, ["kandev-other"])
        self.assertFalse(any(c.startswith("kill") for c in calls))

    def test_kill_exact_accepts_recorded_command_hash_for_needleless_children(self):
        row = f"  202  201  200 {STARTED_A} /usr/bin/python3 child.py\n"
        digest = hashlib.sha256(b"/usr/bin/python3 child.py").hexdigest()
        ssh, calls = self.ssh(row)
        self.assertEqual(fixture._default_kill_exact(ssh, {"pid": 202, "started": STARTED_A, "cmd_sha256": digest}, [ALLOC]), "killed")
        ssh, calls = self.ssh(row)
        with self.assertRaises(PreflightError):
            fixture._default_kill_exact(ssh, {"pid": 202, "started": STARTED_A, "cmd_sha256": "0" * 64}, [ALLOC])

    def test_kill_exact_gone_and_invalid_pids(self):
        ssh, calls = self.ssh("")
        self.assertEqual(fixture._default_kill_exact(ssh, {"pid": 200, "started": STARTED_A}, [ALLOC]), "already_gone")
        for pid in (0, 1, -5, "1", True, None):
            with self.assertRaises(PreflightError):
                fixture._default_kill_exact(ssh, {"pid": pid, "started": STARTED_A}, [ALLOC])

    def failing_kill(self, rows_after):
        """ps shows the recorded identity first; `kill` fails; the recheck then sees rows_after."""
        row = f"  200  1  200 {STARTED_A} /alloc/{ALLOC}/agentctl\n"
        seen = []

        def call(command):
            if command.startswith("LC_ALL=C ps -p"):
                seen.append(command)
                return 0, row if len(seen) == 1 else rows_after
            return 1, ""

        return call

    def test_kill_exact_that_loses_the_race_to_a_process_exit_is_already_gone(self):
        ssh = self.failing_kill("")
        self.assertEqual(fixture._default_kill_exact(ssh, {"pid": 200, "started": STARTED_A}, [ALLOC]), "already_gone")

    def test_kill_exact_failing_on_a_still_present_process_raises(self):
        ssh = self.failing_kill(f"  200  1  200 {STARTED_A} /alloc/{ALLOC}/agentctl\n")
        with self.assertRaisesRegex(PreflightError, "signalling process 200 failed"):
            fixture._default_kill_exact(ssh, {"pid": 200, "started": STARTED_A}, [ALLOC])

    def test_kill_exact_failing_then_seeing_a_reused_pid_raises_a_mismatch(self):
        ssh = self.failing_kill("  200  1  200 Mon Oct 26 10:00:00 2026 /usr/bin/other\n")
        with self.assertRaisesRegex(PreflightError, "no longer matches"):
            fixture._default_kill_exact(ssh, {"pid": 200, "started": STARTED_A}, [ALLOC])


class TcpRelayTest(unittest.TestCase):
    def setUp(self):
        self.upstream = socket.socket()
        self.upstream.bind(("127.0.0.1", 0))
        self.upstream.listen(8)
        self.upstream.settimeout(5)
        self.addCleanup(self.upstream.close)
        self.relay = wo06.TcpRelay(("127.0.0.1", 0), self.upstream.getsockname())
        self.relay.start()
        self.addCleanup(self.relay.stop)

    def echo_once(self):
        client = socket.create_connection(self.relay.address, timeout=5)
        server, _ = self.upstream.accept()
        client.sendall(b"ping")
        self.assertEqual(server.recv(4), b"ping")
        server.sendall(b"pong")
        self.assertEqual(client.recv(4), b"pong")
        return client, server

    def wait_closed(self, sock):
        sock.settimeout(5)
        try:
            return sock.recv(1) == b""
        except OSError:
            return True

    def test_forwards_both_directions(self):
        client, server = self.echo_once()
        client.close()
        server.close()

    def test_partition_closes_live_connections_and_refuses_new_ones_then_heal_resumes(self):
        client, server = self.echo_once()
        self.relay.partition()
        self.assertTrue(self.wait_closed(client))
        self.assertTrue(self.wait_closed(server))
        self.assertTrue(self.relay.partitioned)
        with self.assertRaises(OSError):
            socket.create_connection(self.relay.address, timeout=2)
        self.relay.heal()
        self.assertFalse(self.relay.partitioned)
        again, again_server = self.echo_once()
        again.close()
        again_server.close()
        client.close()
        server.close()

    def test_stop_closes_everything_and_cannot_be_healed(self):
        client, server = self.echo_once()
        self.relay.stop()
        self.assertTrue(self.wait_closed(client))
        with self.assertRaises(OSError):
            socket.create_connection(self.relay.address, timeout=2)
        with self.assertRaises(PreflightError):
            self.relay.heal()
        client.close()
        server.close()

    def test_unreachable_upstream_drops_the_client(self):
        dead = socket.socket()
        dead.bind(("127.0.0.1", 0))
        address = dead.getsockname()
        dead.close()
        relay = wo06.TcpRelay(("127.0.0.1", 0), address)
        relay.start()
        self.addCleanup(relay.stop)
        client = socket.create_connection(relay.address, timeout=5)
        self.assertTrue(self.wait_closed(client))
        client.close()

    def test_endpoint_parsing(self):
        self.assertEqual(wo06._endpoint("localhost:4647"), ("127.0.0.1", 4647))
        self.assertEqual(wo06._endpoint("127.0.0.1:4647"), ("127.0.0.1", 4647))


if __name__ == "__main__":
    unittest.main()
