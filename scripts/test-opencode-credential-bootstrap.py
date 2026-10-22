#!/usr/bin/env python3
"""Opt-in, real-controller credential validation against disposable targets.

The private JSON inventory supplies controller {root, binary, cli, path, port,
standalone_port}, versions, and executors keyed docker/ssh/kubernetes. Each
executor supplies config, profile_config, and optionally inspect_prefix (SSH)
or kubectl_prefix (Kubernetes). No credentials belong in this inventory.
The controller root must be dedicated to this test and already have its managed
CLI versions installed. Worker images must contain the rebuilt agentctl.

Only assertions are printed. CLI output and credential comparisons stay in
memory. Every controller invocation is bounded and stopped in finally.
"""

import argparse
import contextlib
import importlib.util
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid


def command(argv, *, env=None, data=None, timeout=180):
    result = subprocess.run(argv, env=env, input=data, capture_output=True,
                            timeout=timeout)
    if result.returncode:
        phase = re.search(rb"FAILED: credential fault assertion at ([a-z ]+); diagnostics withheld",
                          result.stderr)
        detail = " at " + phase[1].decode() if phase else ""
        diagnostic = (result.stdout + result.stderr).lower()
        categories = [word for word in ("unknown", "certificate", "proxy", "model", "refresh",
                      "unauthorized", "econnrefused", "not found", "client", "expired", "403", "401")
                      if word.encode() in diagnostic]
        if categories:
            detail += " [categories: " + ", ".join(categories) + "]"
        raise RuntimeError("External command failed (" + Path(argv[0]).name +
                           ", exit " + str(result.returncode) + ")" + detail +
                           "; diagnostics withheld")
    return result.stdout


class Controller:
    def __init__(self, settings):
        self.settings = settings
        self.root = Path(settings["root"])
        self.base = "http://127.0.0.1:" + str(settings["port"])
        self.env = dict(os.environ)
        for key in ("OPENCODE_DB", "OPENCODE_CONFIG", "OPENCODE_CONFIG_CONTENT"):
            self.env.pop(key, None)
        for key, directory in (("HOME", "home"), ("XDG_DATA_HOME", "data"),
                               ("XDG_CONFIG_HOME", "config"), ("XDG_STATE_HOME", "state")):
            path = self.root / directory
            path.mkdir(parents=True, exist_ok=True, mode=0o700)
            self.env[key] = str(path)
        self.env.update(KANDEV_HOME_DIR=str(self.root / "kandev"),
                        KANDEV_SERVER_HOST="127.0.0.1",
                        KANDEV_SERVER_PORT=str(settings["port"]),
                        KANDEV_AGENT_STANDALONE_PORT=str(settings["standalone_port"]),
                        KANDEV_FEATURES_AUTH="false", PATH=settings["path"])

    def auth(self, operation, entries=None):
        output = command([self.settings["cli"], "auth", operation, "--standalone"],
                         env=self.env,
                         data=None if entries is None else json.dumps(entries).encode())
        return json.loads(output) if operation == "export" else None

    def api(self, route, body=None, headers=None):
        request = urllib.request.Request(self.base + "/api/v1/" + route,
                  data=None if body is None else json.dumps(body).encode(),
                  headers={"Content-Type": "application/json", **(headers or {})})
        try:
            with urllib.request.urlopen(request, timeout=180) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            raise RuntimeError("Controller HTTP operation failed: " + str(error.code)) from None

    def action(self, action, payload):
        script = """
const [url,request]=process.argv.slice(1); const req=JSON.parse(request);
const socket=new WebSocket(url);
const timer=setTimeout(()=>{socket.close();process.exit(2)},180000);
socket.onopen=()=>socket.send(JSON.stringify(req));
socket.onmessage=e=>{const msg=JSON.parse(e.data);if(msg.id!==req.id)return;
clearTimeout(timer);console.log(JSON.stringify(msg));socket.close();};
socket.onerror=()=>{clearTimeout(timer);process.exit(3)};
"""
        result = json.loads(command(["node", "-e", script,
                    self.base.replace("http:", "ws:") + "/ws",
                    json.dumps({"id": uuid.uuid4().hex, "type": "request",
                                "action": action, "payload": payload})], timeout=190))
        if result.get("type") == "error":
            code = result.get("payload", {}).get("code", "unknown")
            allowed = {"session_busy", "session_not_promptable", "queue_session_unavailable", "validation", "internal_error"}
            category = code if code in allowed else "unclassified"
            raise RuntimeError("Controller session action failed: " + action + " (" + category + ")")

    def wait_session(self, task):
        deadline = time.monotonic() + 300
        while time.monotonic() < deadline:
            state = self.api("task-sessions/" + task["session_id"])["session"]["state"]
            if state == "WAITING_FOR_INPUT":
                return
            if state in ("FAILED", "STOPPED", "COMPLETED"):
                raise RuntimeError("Session did not reach expected synthetic-auth rejection")
            time.sleep(2)
        raise RuntimeError("Session startup deadline exceeded")

    def select_version(self, version):
        token = self.api("app-state?path=/settings/agents")["interimSettingsInterlockToken"]
        job = self.api("agent-update/opencode-acp", {"target_version": version},
                       {"X-Kandev-Interim-Settings-Interlock": token})
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            state = self.api("agent-update/jobs/" + job["job_id"])["status"]
            if state == "succeeded":
                return
            if state == "failed":
                break
            time.sleep(1)
        raise RuntimeError("Managed runtime selection failed")

    @contextlib.contextmanager
    def running(self):
        # Discard logs: live credential material must never enter test artifacts.
        process = subprocess.Popen([self.settings["binary"], "__backend"], env=self.env,
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            deadline = time.monotonic() + 90
            while time.monotonic() < deadline:
                if process.poll() is not None:
                    raise RuntimeError("Isolated controller exited")
                try:
                    self.api("executors")
                    break
                except (OSError, RuntimeError):
                    time.sleep(.2)
            else:
                raise RuntimeError("Controller readiness deadline exceeded")
            yield
        finally:
            process.terminate()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()

    def resume_without_source(self, task):
        self.action("session.stop", {"session_id": task["session_id"]})
        source = self.root / "data" / "opencode"
        held = source.with_name("opencode-validation-held")
        if held.exists() or not source.is_dir():
            raise RuntimeError("Unsafe test source-store layout")
        source.rename(held)
        try:
            if self.auth("export") != []:
                raise RuntimeError("Controller source was not empty")
            self.action("session.launch", {"task_id": task["id"],
                        "session_id": task["session_id"], "intent": "resume"})
            self.wait_session(task)
        finally:
            if source.exists():
                shutil.rmtree(source)
            held.rename(source)


class Target:
    def __init__(self, kind, settings):
        self.kind, self.settings = kind, settings

    def workers(self, task_id=None):
        if self.kind == "docker":
            args = ["docker", "ps", "--format", "{{.Names}}"]
            if task_id:
                args += ["--filter", "label=kandev.task_id=" + task_id]
        elif self.kind == "kubernetes":
            args = self.settings["kubectl_prefix"] + ["get", "pods", "-o", "name"]
            if task_id:
                args += ["-l", "kandev.ai/task-id=" + task_id]
        else:
            return set()
        return set(command(args).decode().splitlines())

    def inspect(self, task_id):
        if self.kind == "ssh":
            return self.settings["inspect_prefix"]
        created = self.workers(task_id)
        if len(created) != 1:
            raise RuntimeError("Expected exactly one new disposable worker")
        name = created.pop()
        if self.kind == "docker":
            return ["docker", "exec", name]
        return self.settings["kubectl_prefix"] + ["exec", name, "--"]

    def export(self, prefix, home, version):
        return json.loads(command(prefix + ["env", "HOME=" + home,
                     "XDG_DATA_HOME=" + home + "/data", "XDG_CONFIG_HOME=" + home + "/config",
                     "XDG_STATE_HOME=" + home + "/state", "npx", "--yes", "--prefer-offline",
                     "@opencode/cli@" + version, "auth", "export", "--standalone"]))

    def smoke(self, controller, version, expected, scenario="smoke"):
        print(json.dumps({"executor": self.kind, "version": version, "phase": "starting"}), flush=True)
        before = self.workers()
        home = "/tmp/kandev-bootstrap-" + uuid.uuid4().hex
        if self.kind == "ssh":
            command(self.settings["inspect_prefix"] + ["sh", "-c",
                    'mkdir -p "$1"; ln -s "$2" "$1/.npm"', "sh", home,
                    self.settings["npm_cache"]])
        executor = controller.api("executors", {"name": "Credential validation " + self.kind,
                   "type": {"docker": "local_docker", "ssh": "ssh", "kubernetes": "k8s"}[self.kind],
                   "config": self.settings["config"]})
        profile = controller.api("executors/" + executor["id"] + "/profiles", {
            "name": "Disposable credential validation",
            "env_vars": [{"key": key, "value": home + suffix} for key, suffix in
                         (("HOME", ""), ("XDG_DATA_HOME", "/data"),
                          ("XDG_CONFIG_HOME", "/config"), ("XDG_STATE_HOME", "/state"))],
            "config": {**self.settings["profile_config"],
                       "remote_credentials": json.dumps(["agent:opencode-acp:files:0"])},
            "prepare_script": 'mkdir -p "$HOME/.npm"\nnpx --yes @opencode/cli@' + version +
                ' --version || true\nfind "$HOME/.npm/_npx" -path "*/@opencode/cli/postinstall.mjs" -exec node {} \\;\n'})
        agent = next(a for a in controller.api("agents")["agents"] if a["name"] == "opencode-acp")
        task = controller.api("tasks", {"title": "Synthetic credential validation",
               "workspace_id": controller.api("workspaces")["workspaces"][0]["id"],
               "workflow_id": controller.api("workflows")["workflows"][0]["id"],
               "agent_profile_id": agent["profiles"][0]["id"], "executor_id": executor["id"],
               "executor_profile_id": profile["id"], "start_agent": True,
               "description": "Synthetic credential validation; no provider success expected."})
        phase = "startup"
        try:
            controller.wait_session(task)
            prefix = self.inspect(task["id"])
            phase = "destination export"
            exported = self.export(prefix, home, version)
            if not any(c["id"] == expected["id"] and c["value"] == expected["value"] for c in exported):
                raise RuntimeError("Destination credential mismatch")
            phase = "restart without source"
            controller.resume_without_source(task)
            phase = "destination comparison"
            if self.export(prefix, home, version) != exported:
                raise RuntimeError("Restart changed destination values or selection")
            if scenario in ("lost-ack", "interrupted-import"):
                controller.action("session.stop", {"session_id": task["session_id"]})
                phase = "worker fault injection"
                interactive = list(prefix)
                interactive.insert(interactive.index("exec") + 1, "-i")
                helper = Path(__file__).with_name("opencode-credential-faults.mjs").read_bytes()
                result = command(interactive + ["env", "HOME=" + home,
                                 "node", "--input-type=module", "-", version, scenario],
                                 data=helper, timeout=300)
                assertion = json.loads(result)
                if assertion.get("passed") is not True:
                    raise RuntimeError("Worker fault assertion failed")
                print(json.dumps({"executor": self.kind, **assertion}), flush=True)
            print(json.dumps({"executor": self.kind, "version": version,
                              "bootstrap": "passed", "resume_without_source": "passed"}), flush=True)
        except RuntimeError as error:
            raise RuntimeError(phase + ": " + str(error)) from None
        finally:
            primary_failure = sys.exc_info()[0] is not None
            try:
                try:
                    controller.action("session.stop", {"session_id": task["session_id"]})
                except RuntimeError:
                    if not primary_failure:
                        raise
                    print("Cleanup could not stop session; preserving original failure", flush=True)
            finally:
                if self.kind != "ssh":
                    # Only remove workers absent from this run's initial snapshot.
                    for name in self.workers(task["id"]) - before:
                        if self.kind == "docker":
                            command(["docker", "rm", "-f", name])
                        else:
                            command(self.settings["kubectl_prefix"] +
                                    ["delete", name, "--wait=false"])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--targets", required=True, type=Path)
    parser.add_argument("--scenario", required=True,
                        choices=("smoke", "interrupted-import", "lost-ack", "oauth-refresh"))
    parser.add_argument("--executors", required=True)
    args = parser.parse_args()
    inventory = json.loads(args.targets.read_text())
    kinds = args.executors.split(",")
    if not kinds or any(k not in ("docker", "ssh", "kubernetes") for k in kinds):
        raise RuntimeError("Unsupported executor")
    controller = Controller(inventory["controller"])
    if args.scenario == "oauth-refresh":
        if kinds != ["kubernetes"]:
            raise RuntimeError("OAuth scenario requires only Kubernetes")
        spec = importlib.util.spec_from_file_location("oauth_validation",
            Path(__file__).with_name("opencode-oauth-validation.py"))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        module.run(inventory, controller,
                   Target("kubernetes", inventory["executors"]["kubernetes"]), command)
        return
    synthetic = {"id": "kandev-smoke-synthetic", "integrationID": "openai",
                 "label": "synthetic validation only", "active": True,
                 "value": {"type": "key", "key": "synthetic-not-a-provider-key"}}
    controller.auth("import", [synthetic])
    with controller.running():
        for version in inventory["versions"]:
            controller.select_version(version)
            for kind in kinds:
                # Separate profiles ensure genuinely independent first/second HOMEs.
                for _ in range(2):
                    Target(kind, inventory["executors"][kind]).smoke(
                        controller, version, synthetic, args.scenario)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, ValueError, KeyError, subprocess.TimeoutExpired) as error:
        # Never include subprocess outputs, exported entries, or arbitrary JSON.
        print("FAILED: " + (str(error) if isinstance(error, RuntimeError) else type(error).__name__))
        raise SystemExit(1) from None
