"""Pool cleanup create-fixture scenarios: unplaced and refused launches, an unknown register outcome, and a force stop.

* pool-unplaced-cleanup: the ssh node is made ineligible and the profile sets exclude_local, so the pool launch preflight
  (one ready, eligible node with capacity) passes while the registered job has no node it may run on. Archiving the task
  must settle the inventory to ``absent`` with the job stopped, no quarantine variable and a deletable task. A second phase
  repeats the archive, with exclude_local lifted from the profile, for a launch refused on the local node (host identity
  mismatch), which must also end ``absent`` without a quarantine marker.
* pool-register-unknown: a loopback fault proxy sits between Kandev and Nomad and loses the job-register response after
  Nomad committed it (connection dropped, then an HTTP 502). No job may stay running without a durable inventory record and
  each launch must end ``absent`` with its job stopped. The proxy serves plain http, so the profile carries no ca_file; the
  proxy still verifies the upstream with the CA, and only the backend-to-proxy leg is plaintext.
* pool-force-stop: a running session on each node type is force-stopped. Its recorded agent processes must be gone (matched
  by pid and start time) and the session must never be reported stopped while one of them still runs.

Every fault (eligibility, host identity token) is restored in the same invocation, also when an assertion fails. The
management token is used for assertions only; the backend uses whatever token the fixture profile carries.
"""

from __future__ import annotations

import http.client
import json
import socket
import ssl
import subprocess
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any, Callable
from urllib.parse import urlsplit

import nomad_native_e2e_bootstrap as bootstrap
import nomad_native_e2e_fixture as fx
import nomad_native_e2e_pool as pool
import nomad_native_e2e_pool_refusals as refusals
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError

SCENARIO_UNPLACED = "pool-unplaced-cleanup"
SCENARIO_REGISTER = "pool-register-unknown"
SCENARIO_FORCE = "pool-force-stop"
QUARANTINE_LIST = "/v1/vars?prefix=kandev%2Fquarantine%2F"
OUTCOME_PHASES = ("ready", "cleanup_pending", "absent")
# A stopped session is CANCELLED; COMPLETED and WAITING_FOR_INPUT are ordinary states of a session that finished a turn.
STOPPED_STATES = ("CANCELLED",)
REFUSED_LOCAL_CASE = refusals.FAKE_LOCAL_CASE
FORCE_STOP_JS = r"""
const input = JSON.parse(require('fs').readFileSync(0, 'utf8'));
if (typeof input.ws_base !== 'string' || !input.ws_base.startsWith('ws://127.0.0.1:') || !/^[0-9a-zA-Z-]{1,64}$/.test(input.session_id)) {
  console.error('fixture-force-stop-input-invalid');
  process.exit(2);
}
const ws = new WebSocket(`${input.ws_base}/ws`);
const id = `pool-force-stop-${Date.now()}-${Math.random().toString(16).slice(2)}`;
const timer = setTimeout(() => { console.error('force-stop-timeout'); process.exit(2); }, 100000);
ws.addEventListener('open', () => {
  ws.send(JSON.stringify({ id, type: 'request', action: 'session.stop',
    payload: { session_id: input.session_id, reason: 'fixture force stop', force: true }, timestamp: new Date().toISOString() }));
});
ws.addEventListener('message', async event => {
  let raw;
  try { raw = typeof event.data === 'string' ? event.data : await event.data.text(); } catch { return; }
  let message;
  try { message = JSON.parse(raw); } catch { return; }
  if (message.id !== id) return;
  clearTimeout(timer);
  ws.close();
  if (message.type === 'error' || (message.payload || {}).success !== true) { console.error('force-stop-rejected'); process.exit(2); }
  console.log(JSON.stringify({ ok: true }));
  process.exit(0);
});
ws.addEventListener('error', () => { console.error('force-stop-websocket-error'); process.exit(2); });
"""


def check_scenario_config(cfg: dict[str, Any], scenario: str) -> None:
    pool_cfg = cfg.get("pool")
    if not pool_cfg:
        raise PreflightError(f"{scenario} requires a pool block in the bootstrap config")
    if scenario == SCENARIO_UNPLACED and not pool_cfg.get("host_identity_token_file"):
        raise PreflightError(f"{scenario} requires a pool block with host_identity_token_file in the bootstrap config")
    bootstrap.require_loopback_nomad(cfg, f"{scenario} mutates Nomad")


class SettleMixin:
    """Production-cleanup assertions shared by the unplaced and register scenarios."""

    marker_baseline: list[str]

    def roots(self) -> list[tuple[str, str]]:
        raise NotImplementedError

    def marker_paths(self) -> list[str]:
        items = self.get_or_none(QUARANTINE_LIST) or []  # type: ignore[attr-defined]
        return sorted(str(item.get("Path")) for item in items if isinstance(item, dict))

    def new_markers(self) -> list[str]:
        return [path for path in self.marker_paths() if path not in self.marker_baseline]

    def cleanup_probe(self, env: dict[str, Any], pending: list[str]) -> dict[str, Any] | None:
        """One poll probe: a quarantine is a definitive failure, work still in progress is listed in pending."""
        state = self.read_state(env)  # type: ignore[attr-defined]
        phase = state.get("phase")
        if phase == "quarantined" or state.get("quarantine_state"):
            raise PreflightError(f"cleanup of {env['label']} quarantined the environment (phase {phase!r}) instead of settling to absent")
        written = self.new_markers()
        if written:
            raise PreflightError(f"cleanup of {env['label']} wrote a quarantine variable: {', '.join(written)}")
        pending.clear()
        if phase != "absent":
            pending.append(f"inventory phase {phase!r}")
        status = self.job_status(env["job_id"])  # type: ignore[attr-defined]
        if status not in (None, "dead"):
            pending.append(f"job status {status!r}")
        if pending:
            return None
        return {"phase": "absent", "job": status or "absent", "quarantine_variables": 0}

    def settle(self, env: dict[str, Any]) -> dict[str, Any]:
        pending: list[str] = []
        failure = f"cleanup of {env['label']} did not settle before timeout"
        return self.settle_note(pending, failure)(lambda: self.cleanup_probe(env, pending))  # type: ignore[attr-defined]

    def assert_nothing_running(self, name: str, env: dict[str, Any], check_fence: bool = True) -> dict[str, Any]:
        """No agentctl and no worker fence for the environment on any pool node."""
        trees: dict[str, Any] = {}
        for attach, root in self.roots():
            self.r.use_node(attach, root)  # type: ignore[attr-defined]
            probe = {**{key: env.get(key) for key in ("job_id", "allocation_id", "environment_id")},
                     "label": f"{env['label']}@{attach}", "attach": attach, "state_root": root}
            tree = self.tree(probe)  # type: ignore[attr-defined]
            trees[attach] = [{"pid": item["pid"], "role": item["role"], "depth": item["depth"]} for item in tree]
            if any(item["role"] == "agentctl" for item in tree):
                raise PreflightError(f"{name}: agentctl is running on the {attach} node")
            if check_fence and env.get("environment_id") and self.r.fence_owner(env["environment_id"]) is not None:  # type: ignore[attr-defined]
                raise PreflightError(f"{name}: environment {env['label']} holds a worker fence on the {attach} node")
        return trees

    def delete_task(self, task: dict[str, str]) -> bool:
        """DELETE through the Kandev API with the deletion-preview confirmation, then wait until the task is no longer served."""
        preview = self.r.api("POST", "/tasks/delete-preflight", {"task_ids": [task["id"]], "cascade": False})  # type: ignore[attr-defined]
        confirmation = preview.get("confirmation_id") if isinstance(preview, dict) else None
        if not confirmation:
            raise PreflightError("the task deletion preview returned no confirmation id")
        self.r.api("DELETE", f"/tasks/{task['id']}", None, headers={"X-Kandev-Task-Delete-Confirmation": confirmation})  # type: ignore[attr-defined]

        def gone() -> bool:
            try:
                self.r.api("GET", f"/tasks/{task['id']}", None)  # type: ignore[attr-defined]
            except PreflightError as exc:
                return "status 404" in str(exc)
            return False

        self.poll(gone, f"task {task['id']} is still served after it was deleted")  # type: ignore[attr-defined]
        return True


# -- pool-unplaced-cleanup -------------------------------------------------------------------------------
class ProfileHooks:
    """Reversible overrides on the fixture runner: extra pool profile keys and the Nomad token secret reference."""

    def __init__(self, runner: fx.Runner):
        self.runner = runner
        self.token_ref: str | None = None
        self._config = runner.pool_profile_config
        self._api = runner.api

    def install(self, add: dict[str, str] | None = None, drop: tuple[str, ...] = ()) -> None:
        base = self._config
        self.runner.pool_profile_config = lambda cfg: {**{k: v for k, v in base(cfg).items() if k not in drop}, **(add or {})}

        def api(method: str, path: str, body: Any, **kwargs: Any) -> Any:
            result = self._api(method, path, body, **kwargs)
            if method == "POST" and path == "/secrets" and isinstance(result, dict) and result.get("id"):
                self.token_ref = f"$kandev-secret:{result['id']}"
            return result

        self.runner.api = api

    def restore(self) -> None:
        self.runner.pool_profile_config = self._config
        self.runner.api = self._api

    def replace_profile_config(self, cfg: dict[str, Any], *, drop: tuple[str, ...]) -> None:
        """PATCH the fixture profile with the base pool config minus ``drop``, keeping the original token reference."""
        if not self.token_ref:
            raise PreflightError("the Nomad token secret reference was not recorded")
        config = {k: v for k, v in self._config(cfg).items() if k not in drop}
        executor, profile = self.runner._task_ctx["executor_id"], self.runner.state["executor_profile_id"]
        self._api("PATCH", f"/executors/{executor}/profiles/{profile}", {"config": {**config, "nomad_token": self.token_ref}})


class UnplacedScenario(SettleMixin, refusals.RefusalScenario):
    def __init__(self, runner: fx.Runner, cfg: dict[str, Any], name: str):
        super().__init__(runner, cfg, name, None)  # type: ignore[arg-type]
        self.marker_baseline = []

    def roots(self) -> list[tuple[str, str]]:
        return [(attach, info["meta"]["kandev.state_root"]) for attach, info in self.nodes.items()]

    def assert_unplaced(self, task: dict[str, str]) -> dict[str, Any]:
        """Job registered, placement attempted and failed, no allocation ever, nothing staged on any node."""
        def registered() -> dict[str, Any] | None:
            inventory = self.soft(lambda: self.r.inventory(task["session_id"], task["id"]))
            job_id = (inventory or {}).get("job_id")
            if isinstance(job_id, str) and fx._JOB_ID.fullmatch(job_id) and self.get_or_none(f"/v1/job/{job_id}") is not None:
                return inventory
            return None

        inventory = self.poll(registered, "the Nomad job of the unplaced task was not registered before timeout")
        env = self.record_env("U1", task, inventory)
        self.known_jobs.add(env["job_id"])
        if inventory.get("phase") == "ready":
            raise PreflightError("the unplaced task reached inventory phase ready")

        def attempted() -> list[Any] | None:
            if self.job_allocs(env["job_id"]):
                raise PreflightError("the unplaced job was allocated although no node is eligible")
            evals = self.get_or_none(f"/v1/job/{env['job_id']}/evaluations") or []
            failed = [item for item in evals if item.get("FailedTGAllocs") or item.get("Status") == "blocked"]
            return evals if failed else None

        evals = self.poll(attempted, "no failed placement evaluation was seen for the unplaced job before timeout")
        if self.job_allocs(env["job_id"]):
            raise PreflightError("the unplaced job was allocated after the placement failure was observed")
        phase = self.r.inventory(task["session_id"], task["id"]).get("phase")
        if phase == "ready":
            raise PreflightError("the unplaced task reached inventory phase ready")
        trees = self.assert_nothing_running("unplaced", env)
        result = {"job_id": env["job_id"], "allocations": 0, "inventory_phase": phase, "placement_failed": True,
                  "evaluation_statuses": sorted({str(item.get("Status")) for item in evals}), "process_trees": trees}
        self.data["identities"]["U1"]["unplaced"] = result
        return result

    def finish_case(self, env: dict[str, Any], task: dict[str, str], step: Callable[..., Any], prefix: str) -> dict[str, Any]:
        """Archive, wait for production cleanup to reach absent, prove nothing runs, then delete the task."""
        self.focus(env)
        step(f"{prefix}-archive-task", lambda: self.archive(task))
        settled = step(f"{prefix}-await-absent", lambda: self.settle(env))
        step(f"{prefix}-assert-nothing-running", lambda: self.assert_nothing_running(prefix, env))
        deleted = step(f"{prefix}-delete-task", lambda: self.delete_task(task))
        return {"cleanup": settled, "task_deleted": deleted}


def _unplaced_steps(runner: fx.Runner, cfg: dict[str, Any], step: Callable[..., Any], scn: UnplacedScenario, hooks: ProfileHooks) -> dict[str, Any]:
    scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
    step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
    scn.data["nodes"] = step("verify-two-nodes", scn.verify_two_nodes)
    scn.data["roles"] = step("locate-nodes", scn.locate_nodes)
    step("verify-host-identity-token", scn.check_token)
    step("start-git-daemon", runner.start_git_daemon)
    scn.marker_baseline = step("record-quarantine-baseline", scn.marker_paths)
    # The first task launches as soon as the fixture objects exist. The pool preflight needs one eligible node, so the
    # local node stays eligible and the profile (exclude_local) is what makes the registered job unplaceable.
    step("unplaced-disable-ssh-node", lambda: scn.disable_node("ssh"))
    step("start-backend", lambda: runner.start_backend(None))
    step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
    scn.known_jobs = scn.kandev_jobs()
    task = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
    unplaced: dict[str, Any] = {"registered": step("assert-unplaced", lambda: scn.assert_unplaced(task))}
    unplaced.update(scn.finish_case(scn.envs["U1"], task, step, "unplaced"))
    scn.cases["unplaced"] = unplaced
    step("unplaced-restore", scn.restore_and_verify)
    step("local-refusal-allow-local-placement", lambda: hooks.replace_profile_config(cfg, drop=("exclude_local",)))
    step("local-refusal-disable-ssh-node", lambda: scn.disable_node("ssh"))
    step("local-refusal-replace-host-identity-token", scn.swap_token)
    second = step("local-refusal-launch", lambda: runner.create_task("Nomad pool cleanup: refused local launch"))
    refusal = step("local-refusal-assert", lambda: scn.assert_refusal(REFUSED_LOCAL_CASE, second))
    if refusal.get("quarantine_marker") is not None:
        raise PreflightError("a quarantine marker was written for the refused local launch")
    refused: dict[str, Any] = {"refusal": refusal}
    refused.update(scn.finish_case(scn.envs["R2"], second, step, "local-refusal"))
    scn.cases["local-refusal"] = refused
    restored = step("local-refusal-restore", scn.restore_and_verify)
    cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    return {"scenario": scn.name, "pool": True, "unplaced": True, "cases": scn.cases, "production_cleanup": True, "release": "none",
            "restored": restored, "cleanup": cleanup, "environments": wo07._env_report(scn)}


def drive_pool_unplaced_cleanup(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    check_scenario_config(cfg, SCENARIO_UNPLACED)
    scn = UnplacedScenario(runner, cfg, SCENARIO_UNPLACED)
    hooks = ProfileHooks(runner)
    hooks.install(add={"exclude_local": "true"})
    try:
        return _unplaced_steps(runner, cfg, runner.step, scn, hooks)
    except BaseException:
        failures = scn.restore_all()
        if failures:
            scn.data["restore_failures"] = failures
        raise
    finally:
        hooks.restore()


# -- pool-register-unknown -------------------------------------------------------------------------------
class RegisterFaultProxy:
    """Loopback HTTP proxy to Nomad that loses the response of one committed job register per arming.

    ``drop`` closes the client connection without answering, ``5xx`` answers 502. Either way Nomad already committed the
    job, so the caller faces an unknown outcome. Nothing is logged: requests carry the Nomad token.
    """

    MODES = ("drop", "5xx")

    def __init__(self, upstream: str, ca_file: str | None = None):
        parsed = urlsplit(upstream)
        if (parsed.scheme not in ("http", "https") or parsed.hostname not in ("127.0.0.1", "localhost") or not parsed.port
                or parsed.username or parsed.password or parsed.path not in ("", "/") or parsed.query):
            raise PreflightError("the register fault proxy needs a loopback http(s) Nomad origin without credentials or path")
        self.scheme, self.host, self.port = parsed.scheme, parsed.hostname, parsed.port
        self.context = ssl.create_default_context(cafile=ca_file) if self.scheme == "https" else None
        self.fired: list[dict[str, Any]] = []
        self._armed: str | None = None
        self._lock = threading.Lock()
        self._server: ThreadingHTTPServer | None = None
        self._thread: threading.Thread | None = None

    def arm(self, mode: str) -> None:
        if mode not in self.MODES:
            raise PreflightError(f"unknown register fault mode {mode!r}")
        with self._lock:
            self._armed = mode

    def _take(self, status: int) -> str | None:
        with self._lock:
            mode, self._armed = self._armed, None
            if mode is not None:
                self.fired.append({"mode": mode, "committed_http_status": status})
            return mode

    def _connection(self) -> http.client.HTTPConnection:
        if self.scheme == "https":
            return http.client.HTTPSConnection(self.host, self.port, timeout=30, context=self.context)
        return http.client.HTTPConnection(self.host, self.port, timeout=30)

    def start(self) -> str:
        proxy = self
        skipped = {"connection", "transfer-encoding", "host", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailers", "upgrade"}

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *_: object) -> None:
                return

            def _reply(self, status: int, body: bytes, headers: list[tuple[str, str]] | None = None) -> None:
                self.send_response(status)
                for key, value in headers or []:
                    self.send_header(key, value)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                if body:
                    self.wfile.write(body)

            def _forward(self) -> None:
                length = self.headers.get("Content-Length", "0")
                if not length.isdigit() or int(length) > 8 * 1024 * 1024:
                    self._reply(413, b"")
                    return
                body = self.rfile.read(int(length)) if int(length) else None
                headers = {key: value for key, value in self.headers.items() if key.lower() not in skipped}
                connection = proxy._connection()
                try:
                    connection.request(self.command, self.path, body=body, headers=headers)
                    response = connection.getresponse()
                    payload = response.read()
                    status = response.status
                    committed = self.command == "PUT" and self.path.split("?", 1)[0] == "/v1/jobs" and 200 <= status < 300
                    mode = proxy._take(status) if committed else None
                    if mode == "drop":
                        self.close_connection = True
                        self.connection.shutdown(socket.SHUT_RDWR)
                        self.connection.close()
                        return
                    if mode == "5xx":
                        self._reply(502, b'{"error":"fixture register fault"}', [("Content-Type", "application/json")])
                        return
                    self._reply(status, payload, [(k, v) for k, v in response.getheaders() if k.lower() not in skipped | {"content-length"}])
                except OSError:
                    self._reply(502, b"")
                finally:
                    connection.close()

            do_GET = do_POST = do_PUT = do_DELETE = do_PATCH = _forward

        self._server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self._server.daemon_threads = True
        self._thread = threading.Thread(target=self._server.serve_forever, kwargs={"poll_interval": 0.1}, daemon=True)
        self._thread.start()
        return f"http://127.0.0.1:{self._server.server_address[1]}"

    def stop(self) -> None:
        if self._server is not None:
            self._server.shutdown()
            self._server.server_close()
            self._server = None
        if self._thread is not None:
            self._thread.join(timeout=5)
            self._thread = None


class RegisterScenario(SettleMixin, pool.TwoNodeScenario):
    node_roots: list[tuple[str, str]]

    def __init__(self, runner: fx.Runner, cfg: dict[str, Any], name: str):
        super().__init__(runner, cfg, name)
        self.marker_baseline = []
        self.launched: list[dict[str, str]] = []
        self.node_roots = []
        self._previous_orphans: set[str] = set()

    def roots(self) -> list[tuple[str, str]]:
        return self.node_roots

    def verify_two_nodes(self) -> dict[str, Any]:
        result = super().verify_two_nodes()
        self.node_roots = [((n.get("Meta") or {})["kandev.attach"], (n.get("Meta") or {})["kandev.state_root"]) for n in self.pool_nodes()]
        return result

    def orphan_jobs(self) -> list[str]:
        """Kandev jobs created during this run that are not dead and that no durable inventory row names."""
        owned = set()
        for task in self.launched:
            inventory = self.soft(lambda t=task: self.r.inventory(t["session_id"], t["id"]))
            if inventory and inventory.get("job_id"):
                owned.add(inventory["job_id"])
        baseline = set(self.r.state.get("baseline_jobs") or [])
        return [job for job in sorted(self.kandev_jobs() - baseline) if job not in owned and self.job_status(job) not in (None, "dead")]

    def check_orphans(self) -> None:
        """An orphan must be seen on two consecutive probes: a single probe can race the intent record."""
        current = set(self.orphan_jobs())
        persistent, self._previous_orphans = current & self._previous_orphans, current
        if persistent:
            raise PreflightError(f"job {', '.join(sorted(persistent))} is registered without a durable cleanup record")

    def await_outcome(self, task: dict[str, str]) -> dict[str, Any]:
        """The launch left the registration outcome behind: ready, cleanup_pending or already absent."""
        def resolved() -> dict[str, Any] | None:
            self.check_orphans()
            inventory = self.soft(lambda: self.r.inventory(task["session_id"], task["id"]))
            return inventory if inventory and inventory.get("phase") in OUTCOME_PHASES else None

        return self.poll(resolved, "the launch with the lost register response did not reach ready, cleanup_pending or absent before timeout")

    def run_case(self, name: str, label: str, mode: str, task: dict[str, str], proxy: RegisterFaultProxy, index: int,
                 step: Callable[..., Any]) -> dict[str, Any]:
        self.launched.append(task)
        inventory = step(f"{name}-await-outcome", lambda: self.await_outcome(task))
        if len(proxy.fired) <= index:
            raise PreflightError(f"{name}: the register response fault never fired, so the unknown outcome was not exercised")
        env = self.record_env(label, task, inventory)
        self.known_jobs.add(env["job_id"])
        step(f"{name}-archive-task", lambda: self.archive(task))
        settled = step(f"{name}-await-absent", lambda: self.settle(env))
        step(f"{name}-assert-nothing-running", lambda: self.assert_nothing_running(name, env))
        orphans = self.orphan_jobs()
        if orphans:
            raise PreflightError(f"{name}: job {', '.join(orphans)} is registered without a durable cleanup record")
        return {"fault": proxy.fired[index], "launch_outcome": inventory["phase"], "cleanup": settled, "orphan_jobs": 0,
                "quarantine_variables": 0, "job_id": env["job_id"]}


def _register_steps(runner: fx.Runner, cfg: dict[str, Any], step: Callable[..., Any], scn: RegisterScenario, proxy: RegisterFaultProxy) -> dict[str, Any]:
    scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
    step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
    scn.data["nodes"] = step("verify-two-nodes", scn.verify_two_nodes)
    step("start-git-daemon", runner.start_git_daemon)
    scn.marker_baseline = step("record-quarantine-baseline", scn.marker_paths)
    address = step("start-register-fault-proxy", proxy.start)
    proxy.arm("drop")
    proxied = {**cfg, "nomad": {**cfg["nomad"], "address": address}}
    # The proxy is plain http and the backend rejects ca_file with a non-https address.
    step("start-backend", lambda: runner.start_backend(None))
    step("create-fixture-objects", lambda: runner.setup_api_objects(proxied))
    scn.known_jobs = scn.kandev_jobs()
    first = {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}
    scn.cases["drop"] = scn.run_case("drop", "D1", "drop", first, proxy, 0, step)
    proxy.arm("5xx")
    second = step("five-xx-launch", lambda: runner.create_task("Nomad pool register unknown: server error"))
    scn.cases["five-xx"] = scn.run_case("five-xx", "D2", "5xx", second, proxy, 1, step)
    cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    return {"scenario": scn.name, "pool": True, "register_unknown": True, "cases": scn.cases, "faults": proxy.fired,
            "backend_to_proxy_leg": "plaintext-http", "proxy_upstream_verified_with_ca": bool((cfg.get("security") or {}).get("ca_file")),
            "production_cleanup": True, "release": "none", "cleanup": cleanup, "environments": wo07._env_report(scn)}


def drive_pool_register_unknown(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    check_scenario_config(cfg, SCENARIO_REGISTER)
    scn = RegisterScenario(runner, cfg, SCENARIO_REGISTER)
    scn.cases = {}  # type: ignore[attr-defined]
    proxy = RegisterFaultProxy(runner.state["nomad"]["address"], (cfg.get("security") or {}).get("ca_file"))
    hooks = ProfileHooks(runner)
    hooks.install(drop=("ca_file",))
    try:
        return _register_steps(runner, cfg, runner.step, scn, proxy)
    finally:
        proxy.stop()
        hooks.restore()


# -- pool-force-stop -------------------------------------------------------------------------------------
def force_stop_session(ws_base: str, session_id: str, run: Callable[..., Any] = subprocess.run) -> None:
    """The production session.stop action with force=true, sent over the loopback WebSocket API."""
    if not ws_base.startswith("ws://127.0.0.1:"):
        raise PreflightError("the force-stop driver only talks to the loopback fixture API")
    result = run(["node", "-e", FORCE_STOP_JS], input=json.dumps({"ws_base": ws_base, "session_id": session_id}), text=True,
                 capture_output=True, timeout=120, check=False)
    if result.returncode != 0:
        raise PreflightError("session force-stop was rejected or failed")


class ForceStopScenario(pool.TwoNodeScenario):
    def send_force_stop(self, session_id: str) -> None:
        force_stop_session(f"ws://127.0.0.1:{self.r.state['port']}", session_id)

    def session_state(self, task: dict[str, str]) -> Any:
        sessions = self.r.api("GET", f"/tasks/{task['id']}/sessions", None).get("sessions", [])
        return next((item.get("state") for item in sessions if item.get("id") == task["session_id"]), None)

    def force_stop_case(self, env: dict[str, Any], task: dict[str, str], tree: list[dict[str, Any]]) -> dict[str, Any]:
        """Force-stop one running session and watch the agent processes and the reported state until both agree."""
        self.focus(env)
        agents = [item for item in tree if item["depth"] >= 1]
        if not agents:
            raise PreflightError(f"environment {env['label']} has no agent process to force-stop")
        if len(self.alive(agents)) != len(agents):
            raise PreflightError(f"environment {env['label']}: a recorded agent process is not running before the force stop")
        before = self.soft(lambda: self.session_state(task))
        if before in STOPPED_STATES:
            raise PreflightError(f"environment {env['label']}: the session was already reported stopped before the force stop")
        outcome: dict[str, Any] = {}

        def send() -> None:
            try:
                self.send_force_stop(task["session_id"])
            except BaseException as exc:  # noqa: BLE001 - reported to the polling thread
                outcome["error"] = self.r.evidence.scrub(str(exc))

        worker = threading.Thread(target=send, daemon=True)
        samples: list[tuple[Any, int]] = []
        worker.start()

        def settled() -> bool:
            self.focus(env)
            # The stop report (call returned, state) is read before the processes: a process seen alive after the report
            # proves the session was reported stopped too early, while one that died after an earlier read proves nothing.
            done = not worker.is_alive()
            state = self.soft(lambda: self.session_state(task))
            running = self.alive(agents)
            samples.append((state, len(running)))
            if done and outcome.get("error"):
                raise PreflightError(f"environment {env['label']}: the force-stop was not accepted: {outcome['error']}")
            if running and (state in STOPPED_STATES or done):
                raise PreflightError(f"environment {env['label']}: the session was reported stopped while {len(running)} agent process(es) "
                                     f"were still running (state {state!r}, stop call {'returned' if done else 'pending'})")
            return done and not running

        try:
            self.poll(settled, f"environment {env['label']}: the force-stop did not finish before timeout")
        finally:
            worker.join(timeout=130)
        self.focus(env)
        leader = next((item for item in tree if item["depth"] == 0), None)
        allocation = self.soft(lambda: self.alloc(env["allocation_id"]).get("ClientStatus"))
        return {"attach": env["attach"], "agent_processes": len(agents), "agent_processes_after": len(self.alive(agents)),
                "state_before": before, "final_state": self.soft(lambda: self.session_state(task)), "samples": len(samples),
                "leader_alive": bool(leader and self.alive([leader])), "allocation_client_status": allocation,
                "stopped_reported_while_running": False}


def _force_steps(runner: fx.Runner, cfg: dict[str, Any], step: Callable[..., Any], scn: ForceStopScenario) -> dict[str, Any]:
    scn.data["node"] = step("verify-node-identity", scn.verify_node_identity)
    step("poll-node-ready", lambda: scn.poll(scn.node_ready, "Nomad node is not ready and eligible"))
    scn.data["nodes"] = step("verify-two-nodes", scn.verify_two_nodes)
    step("start-git-daemon", runner.start_git_daemon)
    step("start-backend", lambda: runner.start_backend(None))
    step("create-fixture-objects", lambda: runner.setup_api_objects(cfg))
    tasks = {"A": {"id": runner.state["task_id"], "session_id": runner.state["session_id"]}}
    snapshots = {"A": step("await-a-running", lambda: scn.await_placed("A", tasks["A"]))}
    tasks["B"] = runner.create_task("Nomad pool force stop B")
    snapshots["B"] = step("await-b-running", lambda: scn.await_placed("B", tasks["B"]))
    envs = scn.envs
    if envs["A"]["node_id"] == envs["B"]["node_id"]:
        raise PreflightError("both environments landed on the same node; spread placement did not use the second node")
    by_attach = {env["attach"]: env["label"] for env in envs.values()}
    if sorted(by_attach) != ["local", "ssh"]:
        raise PreflightError("the two environments must run on one local node and one ssh node")
    scn.data["spread"] = {label: envs[label]["attach"] for label in ("A", "B")}
    layouts = {label: step(f"assert-workspace-{label.lower()}", lambda e=envs[label]: scn.workspace_layout(e)) for label in ("A", "B")}
    trees = {label: step(f"start-managed-process-tree-{label.lower()}", lambda l=label: scn.start_managed_tree(envs[l], tasks[l], snapshots[l]))
             for label in ("A", "B")}
    scn.known_jobs = scn.kandev_jobs()
    force_stop: dict[str, Any] = {}
    for attach in ("local", "ssh"):
        label = by_attach[attach]
        force_stop[attach] = step(f"force-stop-{attach}", lambda l=label: scn.force_stop_case(envs[l], tasks[l], trees[l]))
    for label in ("A", "B"):
        step(f"archive-task-{label.lower()}", lambda t=tasks[label]: scn.archive(t))
    for label in ("A", "B"):
        def settled(e=envs[label]) -> dict[str, Any]:
            scn.focus(e)
            pending: list[str] = []
            return scn.settle_note(pending, f"production cleanup of {e['label']} did not settle before timeout")(lambda: scn.production_settled(e, pending))

        scn.data["identities"][label]["cleanup"] = step(f"await-cleanup-{label.lower()}", settled)
        step(f"assert-cleaned-{label.lower()}", lambda e=envs[label], l=layouts[label]: scn.assert_pool_cleaned(e, l))
    cleanup = step("teardown", lambda: scn.finish({"released": [], "operator_calls": []}))
    return {"scenario": scn.name, "pool": True, "force_stop": force_stop, "spread": scn.data["spread"], "production_cleanup": True,
            "release": "none", "cleanup": cleanup, "environments": wo07._env_report(scn)}


def drive_pool_force_stop(runner: fx.Runner, cfg: dict[str, Any], ops: fx.Ops, root: Path) -> dict[str, Any]:
    check_scenario_config(cfg, SCENARIO_FORCE)
    return _force_steps(runner, cfg, runner.step, ForceStopScenario(runner, cfg, SCENARIO_FORCE))
