#!/usr/bin/env python3
"""Opt-in Linux Pi/Greywall acceptance. Never uses the developer's runtime state.

Set WRAPPER_ACCEPTANCE_TOOLS to a private installation with bin/{pi,pi-acp,
greywall,greyproxy,bwrap,socat,node,npm,npx,bash,git,fd,rg} and a prepared npm cache/.
Set WRAPPER_ACCEPTANCE_AUTH_FILE to an explicit disposable Pi auth.json and
WRAPPER_ACCEPTANCE_MODEL to an openai-codex model ID. Only chatgpt.com:443 is
allowed through the owned proxy. No credentials or raw output enter the record.
"""
import argparse
from contextlib import AbstractContextManager
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import platform
import re
import secrets
import shlex
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

REPO = Path(__file__).resolve().parent.parent
TOOLS = ("pi", "pi-acp", "greywall", "greyproxy", "bwrap", "socat", "node", "npm", "npx", "bash", "git", "fd", "rg")
SCENARIOS = {"direct", "discovery", "utility", "chat", "passthrough"}


class CheckFailed(Exception):
    """A bounded error code safe to put in an acceptance record."""


class OwnedRun(AbstractContextManager):
    def __init__(self, parent=None, private_logs=None):
        self.private_logs = private_logs
        self.root = Path(tempfile.mkdtemp(prefix="kw-", dir=parent))
        self.root.chmod(0o700)
        self.processes = []
        self.logs = []
        self.nonce = secrets.token_hex(16)
        for directory in ("home", "work", "tmp", "bin", "logs", "cases", "proxy-home"):
            (self.root / directory).mkdir(mode=0o700)
        # A wrapper question must not consume the native agent's initial prompt.
        config = self.root / "home/.config/greywall"
        config.mkdir(parents=True, mode=0o700)
        (config / "preferences.json").write_text(json.dumps({"suppressProfilePrompt": ["pi", "npx"]}))

    def start(self, name, argv, env, cwd=None):
        log = (self.root / "logs" / (name + ".log")).open("wb")
        self.logs.append(log)
        proc = subprocess.Popen(argv, cwd=cwd or self.root, env={**env, "WRAPPER_ACCEPTANCE_OWNER": self.nonce}, stdin=subprocess.DEVNULL,
                                stdout=log, stderr=log, start_new_session=True)
        self.processes.append(proc)
        self._record()
        return proc

    def _record(self):
        (self.root / "ownership.json").write_text(json.dumps({
            "nonce": self.nonce, "pids": [p.pid for p in self.processes]}))

    def stop(self, proc):
        if proc not in self.processes:
            raise ValueError("process is not owned by this run")
        if proc.poll() is None:
            os.killpg(proc.pid, signal.SIGTERM)
            try:
                proc.wait(timeout=20)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, signal.SIGKILL)
                proc.wait(timeout=5)

    def _owned_descendants(self):
        identities = {}
        marker = ("WRAPPER_ACCEPTANCE_OWNER=" + self.nonce).encode()
        for proc in Path("/proc").iterdir():
            if not proc.name.isdigit():
                continue
            try:
                if marker not in (proc / "environ").read_bytes().split(b"\0"):
                    continue
                fields = (proc / "stat").read_text().split(") ", 1)[1].split()
                if fields[0] != "Z":
                    identities[int(proc.name)] = fields[19]
            except (OSError, IndexError):
                continue
        return identities

    def _stop_descendants(self):
        for sig in (signal.SIGTERM, signal.SIGKILL):
            deadline = time.monotonic() + 3
            while True:
                identities = self._owned_descendants()
                (self.root / "ownership.json").write_text(
                    json.dumps({"nonce": self.nonce, "processes": identities}))
                if not identities:
                    return
                current = self._owned_descendants()
                for pid, birth in identities.items():
                    if current.get(pid) == birth:
                        try:
                            os.kill(pid, sig)
                        except ProcessLookupError:
                            pass
                if time.monotonic() >= deadline:
                    break
                time.sleep(0.05)
        if self._owned_descendants():
            raise CheckFailed("owned_process_cleanup_failed")

    def __exit__(self, *args):
        try:
            for proc in reversed(self.processes):
                self.stop(proc)
            self._stop_descendants()
        finally:
            for log in self.logs:
                log.close()
            try:
                if self.private_logs:
                    self.private_logs.mkdir(mode=0o700, parents=True, exist_ok=True)
                    self.private_logs.chmod(0o700)
                    shutil.copytree(self.root / "logs", self.private_logs / self.nonce, dirs_exist_ok=True)
            finally:
                shutil.rmtree(self.root)


def public_result(result):
    if result.get("scenario") not in SCENARIOS or result.get("status") not in {"passed", "failed"}:
        raise ValueError("invalid public result")
    clean = {key: result[key] for key in ("scenario", "status")}
    for key in ("models", "modes", "receipts", "tool_calls"):
        if key in result:
            if type(result[key]) is not int or result[key] < 0:
                raise ValueError("invalid result count")
            clean[key] = result[key]
    return clean


def prerequisites(env):
    missing = []
    tools = Path(env.get("WRAPPER_ACCEPTANCE_TOOLS", "/nonexistent"))
    if platform.system() != "Linux":
        missing.append("Linux host required")
    for name in TOOLS:
        if not os.access(tools / "bin" / name, os.X_OK):
            missing.append("tool: " + name)
    if not (tools / "cache").is_dir():
        missing.append("prepared private npm cache")
    for name in ("go", "git", "bash"):
        if not shutil.which(name):
            missing.append("build tool: " + name)
    auth = Path(env.get("WRAPPER_ACCEPTANCE_AUTH_FILE", "/nonexistent"))
    try:
        credential = json.loads(auth.read_text()).get("openai-codex", {})
        if not all(credential.get(key) for key in ("access", "refresh", "expires", "accountId")):
            raise ValueError()
        if credential["expires"] <= time.time() * 1000:
            missing.append("fresh OpenAI Codex login")
        if auth.stat().st_mode & 0o077:
            missing.append("private credential file permissions (0600)")
    except (OSError, ValueError, TypeError, AttributeError):
        missing.append("explicit disposable Pi auth.json")
    if not re.fullmatch(r"openai-codex/[a-zA-Z0-9_.-]+", env.get("WRAPPER_ACCEPTANCE_MODEL", "")):
        missing.append("explicit openai-codex/model identity")
    return missing


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def http(base, path, data=None, token="", method=None, interlock=""):
    headers = {"Content-Type": "application/json", "Connection": "close"}
    if token:
        headers["Authorization"] = "Bearer " + token
    if interlock:
        headers["X-Kandev-Interim-Settings-Interlock"] = interlock
    request = urllib.request.Request(base + path, headers=headers, method=method,
        data=None if data is None else json.dumps(data).encode())
    try:
        with urllib.request.urlopen(request, timeout=240) as response:
            raw = response.read()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as error:
        # The route has no credentials. Response bodies remain private.
        if error.code != 503:
            print(f"HTTP {error.code}: {request.get_method()} {path.split('?')[0]}", flush=True)
        raise


def wait_until(predicate, timeout=60):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = predicate()
        if value:
            return value
        time.sleep(0.25)
    raise CheckFailed("timeout")


def require_owned_listener(owned, port):
    listeners = set()
    for table in (Path("/proc/net/tcp"), Path("/proc/net/tcp6")):
        for line in table.read_text().splitlines()[1:]:
            fields = line.split()
            if fields[3] == "0A" and int(fields[1].split(":")[1], 16) == port:
                listeners.add(fields[9])
    if not listeners:
        return False
    for pid in owned._owned_descendants():
        try:
            for fd in Path(f"/proc/{pid}/fd").iterdir():
                try:
                    target = os.readlink(fd)
                    if target.startswith("socket:["):
                        listeners.discard(target[8:-1])
                except OSError:
                    pass
        except OSError:
            pass
    if listeners:
        raise CheckFailed("listener_not_owned")
    return True


def ready_owned(owned, port, path="/health"):
    wait_until(lambda: require_owned_listener(owned, port))
    return ready(f"http://127.0.0.1:{port}", path)


def ready(base, path="/health"):
    def check():
        try:
            return http(base, path)
        except (OSError, ValueError):
            return False
    return wait_until(check)


def clean_env(owned, tools):
    return {"HOME": str(owned.root / "home"), "PATH": str(tools / "bin") + os.pathsep + os.defpath,
            "TMPDIR": str(owned.root / "tmp"), "TERM": "xterm-256color", "LANG": "C.UTF-8",
            "SHELL": str(tools / "bin/bash"), "KANDEV_HOME_DIR": str(owned.root / "control-state"),
            "GREYWALL_NO_GREYPROXY_INSTALL": "1", "PI_OFFLINE": "1", "npm_config_cache": str(owned.root / "cache"),
            "npm_config_offline": "true", "npm_config_ignore_scripts": "true",
            "PI_CODING_AGENT_DIR": str(owned.root / "home/.pi/agent")}


def wrapper_argv(owned, tools, socks, dns, http_port):
    return [str(tools / "bin/greywall"), "--no-network-rules", "--no-credential-protection",
            "--proxy", f"socks5://127.0.0.1:{socks}", "--dns", f"127.0.0.1:{dns}",
            "--http-proxy", f"http://127.0.0.1:{http_port}",
            "--allow-read-path", str(tools), "--allow-path", str(owned.root / "home"),
            "--allow-path", str(owned.root / "cases"), "--allow-path", str(owned.root / "cache"),
            "--allow-path", str(owned.root / "tmp"),
            "--allow-path", "{{kandev.work_dir}}", "--allow-path", "{{kandev.managed_npm_dir}}", "--"]


def run_command(owned, name, argv, env, cwd=None, timeout=240):
    proc = owned.start(name, argv, env, cwd)
    try:
        status = proc.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        owned.stop(proc)
        raise CheckFailed(name + "_timeout") from None
    if status:
        raise CheckFailed(name + "_exit")
    return (owned.root / "logs" / (name + ".log")).read_text(errors="replace")


def build(owned):
    # No inherited Go flags, overlay, or runtime credentials enter the build.
    env = {key: os.environ[key] for key in ("HOME", "PATH", "GOCACHE", "GOPATH") if key in os.environ}
    env.update(GOFLAGS="", GOENV="off")
    for command in ("acpdbg", "agentctl", "kandev"):
        run_command(owned, "build-" + command,
            ["go", "build", "-trimpath", "-o", str(owned.root / "bin" / command), "./cmd/" + command],
            env, REPO / "apps/backend", timeout=300)


def start_proxy(owned, tools, env):
    api, socks, dns, http_port = [free_port() for _ in range(4)]
    services = []
    for name, port, handler, listener in (("http", http_port, "http", "tcp"),
            ("socks", socks, "socks5", "tcp"), ("dns", dns, "dns", "dns")):
        service = {"name": name, "addr": f"127.0.0.1:{port}", "handler": {"type": handler},
                   "listener": {"type": listener}, "resolver": "resolver-0"}
        if handler != "dns":
            service.update(admission="admission-0", bypass="bypass-0")
            service["handler"]["auther"] = "auther-0"
        else:
            service["listener"]["metadata"] = {"mode": "udp"}
        services.append(service)
    config = {"host": "127.0.0.1", "log": {"level": "warn"}, "services": services,
              "greyproxy": {"addr": f"127.0.0.1:{api}", "db": str(owned.root / "proxy.db"),
                  "auther": "auther-0", "admission": "admission-0", "bypass": "bypass-0",
                  "resolver": "resolver-0", "notifications": {"enabled": False},
                  "docker": {"enabled": False}}}
    path = owned.root / "proxy.json"
    path.write_text(json.dumps(config))
    owned.start("proxy", [str(tools / "bin/greyproxy"), "serve", "-C", str(path)],
                {**env, "HOME": str(owned.root / "proxy-home")})
    base = f"http://127.0.0.1:{api}"
    ready_owned(owned, api, "/api/health")
    for rule in http(base, "/api/rules")["items"]:
        http(base, "/api/rules/" + str(rule["id"]), method="DELETE")
    http(base, "/api/rules", {"container_pattern": "*", "destination_pattern": "chatgpt.com",
                             "port_pattern": "443", "action": "allow", "created_by": "acceptance"})
    return wrapper_argv(owned, tools, socks, dns, http_port)


def prepare_case(owned, name, auth, model):
    case = owned.root / "cases" / name
    case.mkdir()
    (case / "agent").mkdir()
    (case / "agent/auth.json").write_bytes(auth.read_bytes())
    (case / "agent/auth.json").chmod(0o600)
    (case / "agent/settings.json").write_text(json.dumps({
        "defaultProvider": "openai-codex", "defaultModel": model.split("/", 1)[1],
        "defaultThinkingLevel": "low"}))
    seed = secrets.randbelow(9000) + 1000
    nonce = secrets.token_hex(12)
    (case / "input.json").write_text(json.dumps({"seed": seed, "factor": 7, "offset": 19, "nonce": nonce}))
    # The expected result is never included in the prompt.
    prompt = (f"Acceptance test. Read {case / 'input.json'} with the read tool. "
              "Use the bash tool to run a computation of seed * factor + offset. "
              f"Use the write tool to create {case / 'result.txt'} with the result, a space, "
              "the input nonce, and a newline. Read result.txt with the read tool to verify it. "
              "Do not change other files. Reply DONE when all four tool actions are complete.")
    return case, prompt, f"{seed * 7 + 19} {nonce}\n"


def tool_calls(case):
    names = []
    for path in (case / "agent/sessions").rglob("*.jsonl"):
        for line in path.read_text().splitlines():
            item = json.loads(line)
            message = item.get("message", {})
            content = message.get("content", [])
            if isinstance(content, list):
                names.extend(c.get("name") for c in content if isinstance(c, dict) and c.get("type") == "toolCall")
    return names


def verify_case(case, expected, timeout=240):
    path = case / "result.txt"
    wait_until(lambda: path.is_file(), timeout=timeout)
    if path.read_text() != expected:
        raise CheckFailed("output_mismatch")
    def complete_tools():
        names = tool_calls(case)
        remaining = iter(names)
        return names if all(name in remaining for name in ("read", "bash", "write", "read")) else None
    try:
        return len(wait_until(complete_tools, timeout=timeout))
    except CheckFailed:
        raise CheckFailed("tool_evidence_missing") from None


def recorder(owned, name, prefix):
    receipt = owned.root / "logs" / (name + "-receipts.jsonl")
    script = owned.root / "bin" / (name + "-launcher")
    nonce = secrets.token_hex(16)
    script.write_text("#!/bin/sh\n" + "printf '%s\\n' " + shlex.quote(json.dumps({"nonce": nonce})) +
                      " >> " + shlex.quote(str(receipt)) + "\nexec \"$@\" 2> " + shlex.quote(str(owned.root / "logs" / (name + "-stderr.log"))) + "\n")
    script.chmod(0o700)
    return [str(script)] + prefix, receipt


def receipt_count(path):
    return len(path.read_text().splitlines()) if path.exists() else 0


def direct_check(owned, tools, env, auth, model):
    case, prompt, expected = prepare_case(owned, "direct", auth, model)
    trace = owned.root / "logs/direct.jsonl"
    run_command(owned, "direct", [str(owned.root / "bin/acpdbg"), "prompt", "--exec",
        shlex.join([str(tools / "bin/pi-acp")]), "--prompt", prompt,
        "--workdir", str(case), "--file", str(trace), "--timeout", "240s"],
        {**env, "PI_CODING_AGENT_DIR": str(case / "agent")})
    frames = [json.loads(line) for line in trace.read_text().splitlines()]
    selected = [option.get("currentValue") for frame in frames
                for option in frame.get("frame", {}).get("result", {}).get("configOptions", [])
                if option.get("id") == "model"]
    if model not in selected:
        raise CheckFailed("direct_model_mismatch")
    return public_result({"scenario": "direct", "status": "passed", "tool_calls": verify_case(case, expected)})


def start_control(owned, env):
    control, instance = free_port(), free_port()
    nonce = secrets.token_hex(24)
    owned.start("agentctl", [str(owned.root / "bin/agentctl"), "--port", str(control),
                            "--workdir", str(owned.root / "work")],
        {**env, "AGENTCTL_BOOTSTRAP_NONCE": nonce, "AGENTCTL_LISTEN_HOST": "127.0.0.1",
         "AGENTCTL_INSTANCE_PORT_BASE": str(instance), "AGENTCTL_INSTANCE_PORT_MAX": str(instance),
         "AGENTCTL_SHELL_ENABLED": "false", "AGENTCTL_LOG_LEVEL": "warn"})
    base = f"http://127.0.0.1:{control}"
    ready_owned(owned, control)
    token = http(base, "/auth/handshake", {"nonce": nonce})["token"]
    created = http(base, "/api/v1/instances", {"workspace_path": str(owned.root / "work"),
                   "agent_type": "pi-acp", "agent_command": "pi-acp", "protocol": "acp"}, token)
    return f'http://127.0.0.1:{created["port"]}', token


def utility_checks(owned, env, prefix, auth, model):
    base, token = start_control(owned, env)
    case, prompt, expected = prepare_case(owned, "utility", auth, model)
    wrapped, receipt = recorder(owned, "utility", prefix)
    config = {"command": ["npx", "--yes", "--offline", "--prefix", "~/.kandev/managed-npm-runtime", "pi-acp@0.0.34"],
              "work_dir": str(case), "env": {"PI_CODING_AGENT_DIR": str(case / "agent")},
              "launch_wrapper": {"version": 1, "source": "profile", "argv": wrapped}}
    result = http(base, "/api/v1/inference/probe", {"agent_id": "pi-acp", "profile_context": True,
                                                    "inference_config": config}, token)
    if not result.get("success") or receipt_count(receipt) != 1:
        raise CheckFailed("discovery_failed")
    yield public_result({"scenario": "discovery", "status": "passed", "models": len(result.get("models", [])),
                         "modes": len(result.get("modes", [])), "receipts": 1})
    result = http(base, "/api/v1/inference/prompt", {"agent_id": "pi-acp", "model": model,
        "prompt": prompt, "inference_config": config, "auto_approve_permissions": True}, token)
    if not result.get("success") or receipt_count(receipt) != 2:
        raise CheckFailed("utility_failed")
    yield public_result({"scenario": "utility", "status": "passed", "receipts": 1,
                         "tool_calls": verify_case(case, expected)})


def ws_request(owned, tools, base, action, payload):
    script = '''const [url, action, payload] = process.argv.slice(1);
const ws = new WebSocket(url); const id = crypto.randomUUID();
const timer = setTimeout(() => { ws.close(); process.exitCode=1; }, 60000);
ws.onopen = () => ws.send(JSON.stringify({id,type:'request',action,payload:JSON.parse(payload),timestamp:new Date().toISOString()}));
ws.onmessage = event => { const msg=JSON.parse(String(event.data)); if(msg.id!==id)return;
 clearTimeout(timer); ws.close(); if(msg.type==='error'){process.exitCode=1;return;} console.log(JSON.stringify(msg.payload)); };
ws.onerror = () => {clearTimeout(timer);process.exitCode=1;};'''
    raw = run_command(owned, "ws-" + secrets.token_hex(4), [str(tools / "bin/node"), "-e", script,
        base.replace("http:", "ws:") + "/ws", action, json.dumps(payload)],
        {"PATH": str(tools / "bin"), "HOME": str(owned.root / "home")}, timeout=70)
    return json.loads(raw)


def start_backend(owned, env):
    port = free_port()
    backend_env = {**env, "PATH": str(owned.root / "bin") + os.pathsep + env["PATH"],
        "KANDEV_SERVER_PORT": str(port), "KANDEV_SERVER_HOST": "127.0.0.1",
        "KANDEV_DATABASE_PATH": str(owned.root / "kandev.db"), "KANDEV_HOME_DIR": str(owned.root / "backend-state"),
        "KANDEV_WEB_DIST_DIR": str(REPO / "apps/web/dist"), "KANDEV_DOCKER_ENABLED": "false",
        "KANDEV_WORKTREE_BASEPATH": str(owned.root / "home/worktrees"),
        "KANDEV_REPOCLONE_BASEPATH": str(owned.root / "home/clones"),
        "KANDEV_MOCK_AGENT": "false", "KANDEV_LOG_LEVEL": "warn",
        "AGENTCTL_PORT": str(free_port()), "AGENTCTL_INSTANCE_PORT_BASE": str(free_port())}
    backend_env["AGENTCTL_INSTANCE_PORT_MAX"] = str(int(backend_env["AGENTCTL_INSTANCE_PORT_BASE"]) + 5)
    proc = owned.start("backend", [str(owned.root / "bin/kandev"), "__backend"], backend_env)
    base = f"http://127.0.0.1:{port}"
    ready_owned(owned, port, "/ready")
    return base, proc


def session_checks(owned, tools, env, prefix, auth, model):
    base, _ = start_backend(owned, env)
    boot = ready(base, "/api/v1/app-state?path=%2Fsettings%2Fagents")
    interlock = boot["interimSettingsInterlockToken"]
    def api(path, data=None):
        return http(base, "/api/v1/" + path, data, interlock=interlock)
    workspace = api("workspaces", {"name": "Wrapper acceptance"})
    workflow = api("workflows", {"workspace_id": workspace["id"], "name": "Acceptance", "workflow_template_id": "simple"})
    steps = api("workflows/" + workflow["id"] + "/workflow/steps")["steps"]
    step = next((s for s in steps if s.get("is_start_step")), sorted(steps, key=lambda s: s["position"])[0])
    def find_agent():
        return next((a for a in api("agents")["agents"] if a["name"] == "pi-acp"), None)
    agent = wait_until(find_agent)
    for scenario in ("chat", "passthrough"):
        case, prompt, expected = prepare_case(owned, scenario, auth, model)
        repo = owned.root / "home" / (scenario + "-repo")
        repo.mkdir()
        # Trust only this run's empty repository, before Kandev writes its Pi configuration.
        (case / "agent/trust.json").write_text(json.dumps({str(repo.resolve()): True}))
        for command in (["git", "init", "-b", "main"], ["git", "-c", "user.name=Acceptance", "-c",
                "user.email=acceptance@example.invalid", "commit", "--allow-empty", "-m", "test fixture"]):
            run_command(owned, "git-" + secrets.token_hex(4), command, env, repo)
        repository = api("workspaces/" + workspace["id"] + "/repositories", {"name": scenario,
            "source_type": "local", "local_path": str(repo), "default_branch": "main", "pull_before_worktree": False})
        wrapped, receipt = recorder(owned, scenario, prefix)
        profile = api("agents/" + agent["id"] + "/profiles", {"name": "Wrapped " + scenario,
            "model": model, "auto_approve": True, "cli_passthrough": scenario == "passthrough",
            "command_prefix": shlex.join(wrapped),
            "env_vars": [{"key": "PI_CODING_AGENT_DIR", "value": str(case / "agent")},
                         {"key": "npm_config_cache", "value": str(owned.root / "cache")},
                         {"key": "npm_config_offline", "value": "true"},
                         {"key": "PI_OFFLINE", "value": "1"},
                         {"key": "npm_config_ignore_scripts", "value": "true"}]})
        profile = profile.get("profile", profile)
        task = api("tasks", {"workspace_id": workspace["id"], "title": "Wrapper " + scenario,
            "description": prompt, "start_agent": True, "agent_profile_id": profile["id"],
            "workflow_id": workflow["id"], "workflow_step_id": step["id"],
            "repositories": [{"repository_id": repository["id"]}]})
        try:
            count = verify_case(case, expected)
            def finished(task=task, scenario=scenario):
                sessions = api("tasks/" + task["id"] + "/sessions")["sessions"]
                current = next((item for item in sessions if item["id"] == task.get("session_id")), None)
                if current and current["state"] in {"FAILED", "CANCELLED"}:
                    raise CheckFailed(scenario + "_session_failed")
                return current and current["state"] in {"WAITING_FOR_INPUT", "COMPLETED"}
            wait_until(finished, timeout=90)
            if receipt_count(receipt) != 1:
                raise CheckFailed(scenario + "_receipt_count")
            yield public_result({"scenario": scenario, "status": "passed", "receipts": 1, "tool_calls": count})
        finally:
            if task.get("session_id"):
                ws_request(owned, tools, base, "session.stop", {"session_id": task["session_id"], "force": True})


def parse_version(raw):
    match = re.search(r"(?<![0-9])\d+\.\d+\.\d+(?:\.\d+)?\b", raw)
    if not match:
        raise CheckFailed("version_unavailable")
    return match.group()


def versions(owned, tools, env):
    result = {}
    for name in ("pi", "greywall", "greyproxy", "bwrap", "node", "npm", "fd", "rg"):
        raw = run_command(owned, "version-" + name, [str(tools / "bin" / name), "--version"], env, timeout=30)
        result[name] = parse_version(raw)
    adapter = json.loads((tools / "node_modules/pi-acp/package.json").read_text())["version"]
    if adapter != "0.0.34":
        raise CheckFailed("adapter_version_mismatch")
    result["pi-acp"] = adapter
    result["socat"] = parse_version(run_command(owned, "version-socat", [str(tools / "bin/socat"), "-V"], env))
    result["go"] = parse_version(run_command(owned, "version-go", [shutil.which("go"), "version"], env))
    return result


def execute(env, results):
    tools = Path(env["WRAPPER_ACCEPTANCE_TOOLS"]).resolve()
    auth = Path(env["WRAPPER_ACCEPTANCE_AUTH_FILE"]).resolve()
    model = env["WRAPPER_ACCEPTANCE_MODEL"]
    # Installations remain outside /tmp because Greywall replaces /tmp.
    # The short operation root bounds Unix socket paths; only its exact children are granted.
    with OwnedRun(private_logs=Path(env["WRAPPER_ACCEPTANCE_PRIVATE_LOG_DIR"]) if env.get("WRAPPER_ACCEPTANCE_PRIVATE_LOG_DIR") else None) as owned:
        child_env = clean_env(owned, tools)
        # Preserve standard tools on Nix without inheriting account environment variables.
        child_env["PATH"] += os.pathsep + os.pathsep.join(dict.fromkeys(
            str(Path(shutil.which(name)).parent) for name in ("bash", "git")))
        shutil.copytree(tools / "cache", owned.root / "cache")
        build(owned)
        info = versions(owned, tools, child_env)
        print("Current-source binaries built. Starting isolated checks.", flush=True)
        prefix = start_proxy(owned, tools, child_env)
        checks = [direct_check(owned, tools, child_env, auth, model)]
        for result in checks:
            results.append(result)
            print(json.dumps(result), flush=True)
        for result in utility_checks(owned, child_env, prefix, auth, model):
            results.append(result)
            print(json.dumps(result), flush=True)
        for result in session_checks(owned, tools, child_env, prefix, auth, model):
            results.append(result)
            print(json.dumps(result), flush=True)
        return info


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--check-prerequisites", action="store_true")
    mode.add_argument("--run", action="store_true")
    parser.add_argument("--record", type=Path)
    args = parser.parse_args(argv)
    missing = prerequisites(os.environ)
    if missing:
        print(json.dumps({"status": "blocked", "missing": missing}))
        return 2
    if args.check_prerequisites:
        print(json.dumps({"status": "ready", "provider": "openai-codex", "network": ["chatgpt.com:443"]}))
        return 0
    results = []
    def interrupt(signum, frame):
        raise KeyboardInterrupt()
    previous = signal.signal(signal.SIGTERM, interrupt)
    try:
        info = execute(os.environ, results)
    except (CheckFailed, OSError, ValueError, KeyError, KeyboardInterrupt, subprocess.SubprocessError) as exc:
        # Never serialize exception text: providers and subprocesses can include credentials.
        print(json.dumps({"status": "incomplete", "completed": results, "failure_type": type(exc).__name__, "failure_code": str(exc) if isinstance(exc, CheckFailed) else "runtime_error"}))
        return 1
    finally:
        signal.signal(signal.SIGTERM, previous)
    report = {"date": datetime.now(timezone.utc).date().isoformat(), "status": "passed",
              "platform": "linux/" + platform.machine(), "model": os.environ["WRAPPER_ACCEPTANCE_MODEL"],
              "versions": info, "checks": results}
    if args.record:
        with args.record.open("a") as record:
            record.write("\n## Opt-in real-provider runner result\n\n```json\n" + json.dumps(report, indent=2) + "\n```\n")
    print(json.dumps(report))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
