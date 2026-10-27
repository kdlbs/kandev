"""pool-refusals create-fixture scenario: a pool launch is refused on a node that fails the Kandev node contract.

Two refusals run in one invocation against the two-node pool fixture:

* wrong-host-key: the local node is made ineligible and the ssh node's Nomad node metadata
  ``kandev.ssh_host_key_sha256`` is overridden (dynamic node metadata) with a fixture value, so the pinned SSH dial
  fails with a host key mismatch.
* fake-local-node: the ssh node is made ineligible and the run home's ``data/nomad-host-identity`` token is replaced,
  so the local node's declared identity digest no longer matches this host. The backend reads that token on every
  launch, so no backend restart is needed.

Each case asserts the task never reaches ready, the error names the cause, the placement stayed on the expected node
(no fallback), no agentctl process runs on either node, and no credential or key material appears in the error. The
inventory phase, durable state summary and quarantine marker are recorded verbatim. The task then ends through
production archive cleanup when that settles, otherwise through the operator-teardown helpers.

Every fault (eligibility, metadata override, token content) is restored in the same invocation, also when an
assertion fails.
"""

from __future__ import annotations

import hashlib
import json
import re
import secrets
import stat
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any, Callable, Iterator
from urllib.parse import quote

import nomad_native_e2e_bootstrap as bootstrap
import nomad_native_e2e_fixture as fx
import nomad_native_e2e_pool as pool
import nomad_native_e2e_wo06 as wo06
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError

SCENARIO = "pool-refusals"
HOST_KEY_META = "kandev.ssh_host_key_sha256"
IDENTITY_META = "kandev.host_identity_sha256"
ROLLBACK_QUARANTINE_REASON = "runtime_identity_unrecorded"  # a rolled-back launch with no recorded runtime identity quarantines its node
WRONG_HOST_KEY = "SHA256:" + "A" * 43
HOST_KEY_CAUSES = ("host key changed", "host key mismatch")
IDENTITY_CAUSES = ("the node host identity does not match this host",)
WRONG_KEY_CASE = {"name": "wrong-host-key", "label": "R1", "attach": "ssh", "causes": HOST_KEY_CAUSES}
FAKE_LOCAL_CASE = {"name": "fake-local-node", "label": "R2", "attach": "local", "causes": IDENTITY_CAUSES}
_NODE_ID = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
_PRIVATE_KEY = re.compile(r"-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----")
LIMITATIONS = [
    "the host key refusal overrides the Nomad node metadata value; the ssh node's real host key is unchanged",
    "the override uses Nomad dynamic node metadata (Nomad 1.5 or later); a client that does not let it replace the static value fails preflight",
    "the host identity token is read by the backend on every launch, so the token is replaced without a backend restart",
    "the nodes are two clients of one throwaway server; this is not a multi-machine fleet",
]


def check_scenario_config(cfg: dict[str, Any]) -> None:
    pool_cfg = cfg.get("pool")
    if not pool_cfg or not pool_cfg.get("host_identity_token_file"):
        raise PreflightError("pool-refusals requires a pool block with host_identity_token_file in the bootstrap config")
    bootstrap.require_loopback_nomad(cfg, "pool-refusals mutates Nomad")


class NodeMeta:
    """Overrides and restores the host key entry of one node's dynamic metadata (POST /v1/client/metadata)."""

    def __init__(self, address: str, token: str, region: str, opener: Any = None):
        self.address, self.token, self.region = address, token, region
        self.opener = opener or fx._nomad_opener(address)

    def apply(self, node_id: str, value: str | None) -> None:
        """Set the host key metadata to value, or remove the dynamic entry so the static value shows again when None."""
        if not _NODE_ID.fullmatch(node_id):
            raise PreflightError("refusing to change metadata of a malformed node id")
        body = json.dumps({"NodeID": node_id, "Meta": {HOST_KEY_META: value}}).encode()
        request = urllib.request.Request(
            f"{self.address}/v1/client/metadata?node_id={node_id}&region={quote(self.region)}", data=body, method="POST",
            headers={"X-Nomad-Token": self.token, "Accept": "application/json", "Content-Type": "application/json"})
        try:
            with self.opener.open(request, timeout=15) as response:
                result = json.loads(response.read() or b"{}")
        except urllib.error.HTTPError as exc:
            raise PreflightError(
                f"Nomad rejected the dynamic node metadata change with status {exc.code}; overriding a static meta key needs "
                "Nomad 1.5 or later, node:write on the token and a client that accepts dynamic metadata") from None
        except (OSError, urllib.error.URLError):
            raise PreflightError("Nomad dynamic node metadata change is unreachable") from None
        except json.JSONDecodeError:
            raise PreflightError("Nomad dynamic node metadata response is not JSON") from None
        effective = result.get("Meta") if isinstance(result, dict) else None
        if value is not None and isinstance(effective, dict) and effective.get(HOST_KEY_META) != value:
            raise PreflightError("Nomad accepted the dynamic node metadata change but the static meta key keeps its value; "
                                 "dynamic metadata cannot override it on this client")


def make_meta(runner: fx.Runner) -> NodeMeta:
    nomad = runner.state["nomad"]
    return NodeMeta(nomad["address"], runner.token, nomad["region"])


def _strings(value: Any) -> Iterator[str]:
    if isinstance(value, str):
        yield value
    elif isinstance(value, dict):
        for item in value.values():
            yield from _strings(item)
    elif isinstance(value, list):
        for item in value:
            yield from _strings(item)


class RefusalScenario(pool.TwoNodeScenario):
    def __init__(self, runner: fx.Runner, cfg: dict[str, Any], name: str, meta: NodeMeta):
        super().__init__(runner, cfg, name)
        self.meta = meta
        self.nodes: dict[str, dict[str, Any]] = {}
        self.windows: dict[str, dict[str, Any]] = {}
        self.override: dict[str, Any] | None = None
        self.token_swap: dict[str, Any] | None = None
        self.original_token: str | None = None
        self.restored: dict[str, str] = {}
        self.cases: dict[str, Any] = {}
        self.data["cases"] = self.cases

    # -- nodes ---------------------------------------------------------------------------------------
    def locate_nodes(self) -> dict[str, str]:
        found = {(node.get("Meta") or {}).get("kandev.attach"): node for node in self.pool_nodes()}
        if sorted(found) != ["local", "ssh"]:
            raise PreflightError("pool-refusals needs exactly one eligible local node and one eligible ssh node")
        self.nodes = {attach: {"id": node["ID"], "meta": dict(node["Meta"])} for attach, node in found.items()}
        if not self.nodes["ssh"]["meta"].get(HOST_KEY_META) or self.nodes["ssh"]["meta"][HOST_KEY_META] == WRONG_HOST_KEY:
            raise PreflightError("the ssh node has no usable original host key metadata to override and restore")
        return {attach: info["id"] for attach, info in self.nodes.items()}

    def node_record(self, node_id: str) -> dict[str, Any]:
        return self.r.nomad(f"/v1/node/{node_id}")

    def nodes_restored(self) -> bool:
        attaches = sorted((n.get("Meta") or {}).get("kandev.attach", "") for n in self.pool_nodes())
        return attaches == ["local", "ssh"]

    def save_restore_record(self) -> None:
        record = {"eligibility": self.windows, "host_key_override": self.override, "token": self.token_swap}
        self.r.save_state(refusal_restore=record)
        self.data["restore_record"] = record

    # -- eligibility ---------------------------------------------------------------------------------
    def set_eligibility(self, node_id: str, eligible: bool) -> None:
        self.ops.nomad_set_eligibility(self.nomad_cfg["address"], self.r.token, self.nomad_cfg["region"], node_id, eligible)
        want = "eligible" if eligible else "ineligible"
        self.poll(lambda: self.node_record(node_id).get("SchedulingEligibility") == want, f"node {node_id} did not become {want}")

    def disable_node(self, attach: str) -> dict[str, Any]:
        node_id = self.nodes[attach]["id"]
        original = self.node_record(node_id).get("SchedulingEligibility")
        window = {"node_id": node_id, "attach": attach, "original": original, "changed_by_runner": original == "eligible",
                  "restored": False, "restore_with": f"nomad node eligibility -enable {node_id}"}
        self.windows[node_id] = window
        self.save_restore_record()
        if window["changed_by_runner"]:
            self.set_eligibility(node_id, False)
        return window

    def restore_window(self, window: dict[str, Any]) -> str:
        if window["restored"] or not window["changed_by_runner"]:
            return "not needed"
        node_id = window["node_id"]
        marker = self.get_or_none(wo06.quarantine_marker_path(node_id, self.nomad_cfg["namespace"]))
        if marker is not None:
            step_text = wo06.quarantine_operator_step(node_id, self.nomad_cfg["namespace"])
            window["quarantine_marker"] = f"present: kandev/quarantine/{node_id}"
            self.save_restore_record()
            raise PreflightError(f"quarantine marker kandev/quarantine/{node_id} exists; the node is left ineligible; {step_text}")
        self.set_eligibility(node_id, window["original"] == "eligible")
        window["restored"] = True
        self.save_restore_record()
        return "restored"

    # -- ssh host key metadata -----------------------------------------------------------------------
    def meta_value(self, node_id: str) -> Any:
        return (self.node_record(node_id).get("Meta") or {}).get(HOST_KEY_META)

    def override_host_key(self) -> dict[str, Any]:
        node_id, original = self.nodes["ssh"]["id"], self.nodes["ssh"]["meta"][HOST_KEY_META]
        self.override = {"node_id": node_id, "key": HOST_KEY_META, "original": original, "override": WRONG_HOST_KEY, "applied": True,
                         "restored": False, "restore_with": f"nomad node meta apply -node-id {node_id} {HOST_KEY_META}={original}"}
        self.save_restore_record()
        self.meta.apply(node_id, WRONG_HOST_KEY)
        self.poll(lambda: self.meta_value(node_id) == WRONG_HOST_KEY,
                  f"the {HOST_KEY_META} override is not visible in the node metadata before timeout")
        return {"node_id": node_id, "key": HOST_KEY_META, "original": original, "override": WRONG_HOST_KEY}

    def restore_host_key(self) -> str:
        record = self.override
        if record is None or record["restored"]:
            return "not needed"
        self.meta.apply(record["node_id"], record["original"])
        self.poll(lambda: self.meta_value(record["node_id"]) == record["original"],
                  f"the original {HOST_KEY_META} is not visible again in the node metadata before timeout")
        record["restored"] = True
        self.save_restore_record()
        return "restored"

    # -- host identity token -------------------------------------------------------------------------
    def token_path(self) -> Path:
        return self.r.home / "data" / "nomad-host-identity"

    def check_token(self) -> dict[str, Any]:
        path = self.token_path()
        if path.is_symlink() or not path.is_file():
            raise PreflightError("the run home has no host identity token file")
        digest = hashlib.sha256(path.read_text(encoding="utf-8").strip().encode()).hexdigest()
        if digest != self.nodes["local"]["meta"].get(IDENTITY_META):
            raise PreflightError("the run home host identity token does not match the local node's declared identity digest")
        return {"path_is_regular_file": True, "matches_local_node": True}

    def swap_token(self) -> dict[str, Any]:
        path = self.token_path()
        self.check_token()
        original = path.read_text(encoding="utf-8")
        replacement = secrets.token_hex(32) + "\n"
        self.r.evidence.secrets += [original.strip(), replacement.strip()]
        self.original_token = original
        self.token_swap = {"path": str(path), "restored": False, "restore_from": self.cfg["pool"]["host_identity_token_file"]}
        self.save_restore_record()
        fx._write_private(path, replacement)
        path.chmod(0o600)
        digest = hashlib.sha256(replacement.strip().encode()).hexdigest()
        if digest == self.nodes["local"]["meta"].get(IDENTITY_META):
            raise PreflightError("the replacement host identity token still matches the local node")
        return {"replaced": True, "mode": "600"}

    def restore_token(self) -> str:
        record = self.token_swap
        if record is None or record["restored"] or self.original_token is None:
            return "not needed"
        path = self.token_path()
        fx._write_private(path, self.original_token)
        path.chmod(0o600)
        if path.read_text(encoding="utf-8") != self.original_token or stat.S_IMODE(path.stat().st_mode) != 0o600:
            raise PreflightError("the host identity token was not restored with its original content and mode 600")
        record["restored"] = True
        self.save_restore_record()
        return "restored"

    # -- restoration ---------------------------------------------------------------------------------
    def restore_all(self) -> list[str]:
        """Restores every fault this run applied; never raises, returns the failures."""
        failures: list[str] = []
        steps: list[tuple[str, Callable[[], str]]] = [("ssh host key metadata", self.restore_host_key), ("host identity token", self.restore_token)]
        steps += [(f"{w['attach']} node eligibility", lambda w=w: self.restore_window(w)) for w in self.windows.values()]
        for name, fn in steps:
            try:
                note = fn()
                if note != "not needed" or name not in self.restored:
                    self.restored[name] = note
            except Exception as exc:  # noqa: BLE001 - every restoration is attempted
                failures.append(f"{name}: {exc}")
        self.data["restored"] = dict(self.restored)
        return failures

    def restore_and_verify(self) -> dict[str, Any]:
        failures = self.restore_all()
        if failures:
            raise PreflightError(f"restoration failed: {'; '.join(failures)}")
        self.poll(self.nodes_restored, "the pool did not return to one eligible local node and one eligible ssh node")
        return {**self.restored, "token": self.check_token()}

    # -- refusal observation -------------------------------------------------------------------------
    def session_view(self, task: dict[str, str]) -> tuple[list[Any], list[Any]]:
        sessions = self.r.api("GET", f"/tasks/{task['id']}/sessions", None).get("sessions", [])
        messages = self.r.api("GET", f"/task-sessions/{task['session_id']}/messages", None).get("messages", [])
        return sessions, messages

    def secret_values(self) -> list[str]:
        values = [self.r.token, *self.r.evidence.secrets]
        token = self.token_path()
        if token.is_file():
            values.append(token.read_text(errors="replace").strip())
        key = self.r.home / ".ssh" / "id_ed25519"
        if key.is_file():
            values += [line.strip() for line in key.read_text(errors="replace").splitlines() if len(line.strip()) > 20 and "-----" not in line]
        return [value for value in dict.fromkeys(values) if value and len(value) >= 8]

    def refusal_probe(self, task: dict[str, str], causes: tuple[str, ...]) -> dict[str, Any] | None:
        inventory = self.soft(lambda: self.r.inventory(task["session_id"], task["id"]))
        if inventory and inventory.get("phase") == "ready":
            raise PreflightError("the task that must be refused reached inventory phase ready")
        view = self.soft(lambda: self.session_view(task))
        if view is None:
            return None
        excerpts = [text[:600] for text in _strings(list(view)) if any(cause in text.lower() for cause in causes)]
        return {"sessions": view[0], "messages": view[1], "excerpts": excerpts} if excerpts else None

    def assert_refusal(self, case: dict[str, Any], task: dict[str, str]) -> dict[str, Any]:
        name, attach, causes = case["name"], case["attach"], case["causes"]
        seen = self.poll(lambda: self.refusal_probe(task, causes), f"{name}: no refusal naming {causes[0]!r} was reported before timeout")
        text = json.dumps([seen["sessions"], seen["messages"]])
        for secret in self.secret_values():
            if secret in text:
                raise PreflightError(f"{name}: credential or key material appears in the refusal error text")
        if _PRIVATE_KEY.search(text):
            raise PreflightError(f"{name}: private key material appears in the refusal error text")
        inventory = self.r.inventory(task["session_id"], task["id"])
        if inventory.get("phase") == "ready":
            raise PreflightError(f"{name}: the refused task reached inventory phase ready")
        expected = self.nodes[attach]
        if not inventory.get("job_id") or not inventory.get("node_id"):
            raise PreflightError(f"{name}: the refused launch recorded no job or node, so absence of a runtime cannot be proven")
        if inventory["node_id"] != expected["id"]:
            raise PreflightError(f"{name}: the environment was placed on a node other than the {attach} node (fallback)")
        env = self.record_env(case["label"], task, inventory, attach=attach, state_root=expected["meta"]["kandev.state_root"])
        state = self.read_state(env)
        if state.get("node_attach") not in (None, "", attach):
            raise PreflightError(f"{name}: durable state records attach {state.get('node_attach')!r}, expected {attach!r}")
        stray = {alloc["NodeID"] for alloc in self.job_allocs(env["job_id"])} - {expected["id"]}
        if stray:
            raise PreflightError(f"{name}: the job has allocations on node(s) {sorted(stray)} outside the {attach} node (fallback)")
        trees = self.assert_no_agentctl(name, env)
        marker = self.get_or_none(wo06.quarantine_marker_path(expected["id"], self.nomad_cfg["namespace"]))
        return {
            "task_id": task["id"], "session_id": task["session_id"], "environment_id": env["environment_id"], "job_id": env["job_id"],
            "allocation_id": env["allocation_id"], "node_id": env["node_id"], "node_attach": attach,
            "inventory_phase": inventory["phase"], "state": self.state_summary(state), "quarantine_marker": marker,
            "refusal": self.r.evidence.scrub(" | ".join(seen["excerpts"])),
            "session_states": sorted({str(item.get("state")) for item in seen["sessions"] if isinstance(item, dict)}),
            "process_trees": trees, "ready_reached": False, "ssh_fallback": False, "agentctl_processes": 0, "secret_in_error": False,
        }

    def assert_no_agentctl(self, name: str, env: dict[str, Any]) -> dict[str, Any]:
        trees: dict[str, Any] = {}
        for attach, node in self.nodes.items():
            probe = {**{key: env.get(key) for key in ("job_id", "allocation_id", "environment_id")},
                     "label": f"{env['label']}@{attach}", "attach": attach, "state_root": node["meta"]["kandev.state_root"]}
            tree = self.tree(probe)
            trees[attach] = [{"pid": item["pid"], "role": item["role"], "depth": item["depth"]} for item in tree]
            if any(item["role"] == "agentctl" for item in tree):
                raise PreflightError(f"{name}: agentctl is running on the {attach} node although the launch was refused")
        return trees

    # -- ending the task -----------------------------------------------------------------------------
    def try_production_cleanup(self, env: dict[str, Any]) -> tuple[dict[str, Any] | None, list[str]]:
        """One bounded wait for production cleanup; what stopped it is returned verbatim when it does not settle."""
        notes: list[str] = []
        pending: list[str] = []

        def probe() -> dict[str, Any] | None:
            try:
                return self.production_settled(env, pending)
            except PreflightError as exc:
                notes[:] = [str(exc)]
                return None

        try:
            return self.poll(probe, "production cleanup did not settle", self.ops.retention_settle), notes
        except PreflightError:
            return None, notes or [f"pending: {', '.join(pending)}"]

    def readmit_node(self, name: str, env: dict[str, Any], marker_path: str, marker: str) -> dict[str, Any]:
        """The documented readmission after the operator teardown: readmit, delete the marker, enable eligibility."""
        node_id = env["node_id"]
        self.poll(lambda: not self.tree(env), f"{name}: worker processes remain, so the node cannot be readmitted")
        readmit = self.r.api("POST", wo07.READMIT_API_PATH, wo07.readmit_body(env, node_id, self.read_state(env))) or {}
        self.ops.nomad_delete_variable(self.nomad_cfg["address"], self.r.token, self.nomad_cfg["namespace"], marker)
        self.poll(lambda: self.get_or_none(marker_path) is None, f"{name}: the quarantine marker still exists after deletion")
        self.set_eligibility(node_id, True)
        self.poll(lambda: self.node_record(node_id).get("Status") == "ready", f"{name}: node {node_id} is not ready after readmission")
        return {"marker_deleted": True, "readmit": readmit, "eligibility": "enabled"}

    def end_case(self, case: dict[str, Any], task: dict[str, str]) -> dict[str, Any]:
        env = self.envs[case["label"]]
        self.focus(env)
        self.archive(task)
        settled, notes = self.try_production_cleanup(env)
        marker_path = wo06.quarantine_marker_path(env["node_id"], self.nomad_cfg["namespace"])
        marker = self.get_or_none(marker_path)
        quarantine = None
        if marker is not None:
            if ROLLBACK_QUARANTINE_REASON not in json.dumps(marker):
                raise PreflightError(f"{case['name']}: a quarantine marker exists for the node with a reason other than "
                                     f"{ROLLBACK_QUARANTINE_REASON}; {wo06.quarantine_operator_step(env['node_id'], self.nomad_cfg['namespace'])}")
            quarantine = {"reason": ROLLBACK_QUARANTINE_REASON, "marker": f"kandev/quarantine/{env['node_id']}"}
        if settled is not None and quarantine is None:
            result = {"path": "production-archive-cleanup", "production": settled}
        else:
            retained = self.state_summary(self.read_state(env))
            released = self.release(case["label"], kill_first=False)
            leftovers = self.remove_pool_leftovers(env, True)
            result = {"path": "operator-teardown", "retained_state": retained, "production_note": notes, "released": released,
                      "leftovers": leftovers}
            if quarantine is not None:
                result["quarantine"] = {**quarantine, **self.readmit_node(case["name"], env, marker_path, quarantine["marker"])}
        self.focus(env)
        if self.tree(env):
            raise PreflightError(f"{case['name']}: worker processes remain after the task ended")
        if self.r.fence() is not None:
            raise PreflightError(f"{case['name']}: a worker fence remains after the task ended")
        return result


def _case(scn: RefusalScenario, step: Callable[..., Any], case: dict[str, Any], task: dict[str, str]) -> dict[str, Any]:
    name = case["name"]
    scn.cases[name] = {"refusal": step(f"{name}-assert-refusal", lambda: scn.assert_refusal(case, task))}
    scn.cases[name]["cleanup"] = step(f"{name}-end-task", lambda: scn.end_case(case, task))
    return scn.cases[name]


def _steps(runner: fx.Runner, cfg: dict[str, Any], step: Callable[..., Any], scn: RefusalScenario) -> dict[str, Any]:
    scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
    step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
    scn.data["nodes"] = step("verify-two-nodes", scn.verify_two_nodes)
    scn.data["roles"] = step("locate-nodes", scn.locate_nodes)
    step("verify-host-identity-token", scn.check_token)
    step("start-git-daemon", runner.start_git_daemon)
    # The first task launches as soon as the fixture objects exist, so the first fault is in place before that.
    step("wrong-host-key-disable-local-node", lambda: scn.disable_node("local"))
    step("wrong-host-key-override-ssh-host-key", scn.override_host_key)
    step("start-backend", lambda: runner.start_backend(None))
    step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
    scn.known_jobs = scn.kandev_jobs()
    first = _case(scn, step, WRONG_KEY_CASE, {"id": runner.state["task_id"], "session_id": runner.state["session_id"]})
    step("wrong-host-key-restore", scn.restore_and_verify)
    step("fake-local-node-disable-ssh-node", lambda: scn.disable_node("ssh"))
    step("fake-local-node-replace-host-identity-token", scn.swap_token)
    second_task = step("fake-local-node-launch", lambda: runner.create_task("Nomad pool refusal: fake local node"))
    second = _case(scn, step, FAKE_LOCAL_CASE, second_task)
    restored = step("fake-local-node-restore", scn.restore_and_verify)
    cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    paths = {name: item["cleanup"]["path"] for name, item in (("wrong-host-key", first), ("fake-local-node", second))}
    return {"scenario": scn.name, "pool": True, "refusals": True, "cases": scn.cases, "cleanup_paths": paths,
            "production_cleanup": all(path == "production-archive-cleanup" for path in paths.values()),
            "release": "none" if all(path == "production-archive-cleanup" for path in paths.values()) else "operator-teardown",
            "restored": restored, "limitations": LIMITATIONS, "cleanup": cleanup, "environments": wo07._env_report(scn)}


def drive_pool_refusals(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    scn = RefusalScenario(runner, cfg, SCENARIO, make_meta(runner))
    try:
        return _steps(runner, cfg, runner.step, scn)
    except BaseException:
        failures = scn.restore_all()
        if failures:
            scn.data["restore_failures"] = failures
        raise
