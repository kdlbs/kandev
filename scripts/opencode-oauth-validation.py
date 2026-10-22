"""Opt-in provider-backed validation; imported by the bootstrap harness.

The inventory's oauth section contains source_export (argv), model, version,
proxy_bind and proxy_reachable. It contains no account values. TLS inspection
requires the user's explicit permission before this scenario is run.
With explicit user waiver, skip_natural_expiry=true checks provider responses
without routing clients through inspection; refresh evidence is reported waived.
"""

import concurrent.futures
import importlib.util
import json
from pathlib import Path
import shutil
import tempfile
import time
import uuid


def run(inventory, controller, target, command):
    settings = inventory["oauth"]
    require_refresh = not settings.get("skip_natural_expiry", False)
    entries = json.loads(command(settings["source_export"]))
    active = [entry for entry in entries if entry.get("active") and
              entry.get("value", {}).get("type") == "oauth"]
    if len(active) != 1:
        raise RuntimeError("Expected one active OAuth account")
    if require_refresh and active[0]["value"].get("expires", float("inf")) >= time.time() * 1000:
        raise RuntimeError("Expected one naturally expired active OAuth account")
    source = controller.root / "data" / "opencode"
    held = source.with_name("opencode-oauth-validation-held")
    if held.exists():
        raise RuntimeError("Previous OAuth source-store cleanup needs attention")
    spec = importlib.util.spec_from_file_location(
        "refresh_proxy", Path(__file__).with_name("opencode-refresh-proxy.py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    tasks = []
    preserve_for_recovery = False
    version = settings["version"]
    nonce = "VALIDATED_" + uuid.uuid4().hex
    prompt = "Reply with exactly " + nonce + ". Do not use tools."

    def wait_reply(task, proxy, expected=nonce):
        deadline = time.monotonic() + 300
        while time.monotonic() < deadline:
            if proxy.evidence()["aborted"]:
                raise RuntimeError("OAuth refresh failed; stop and request recovery decision")
            messages = controller.api("task-sessions/" + task["session_id"] + "/messages")["messages"]
            if any(message.get("author_type") == "agent" and
                   message.get("content", "").strip() == expected for message in messages):
                return
            state = controller.api("task-sessions/" + task["session_id"])["session"]["state"]
            if state in ("FAILED", "STOPPED"):
                raise RuntimeError("Provider-backed session failed")
            time.sleep(1)
        raise RuntimeError("Provider-backed response deadline exceeded")

    def create(agent_profile, proxy, certificate):
        home = "/tmp/kandev-oauth-" + uuid.uuid4().hex
        executor = controller.api("executors", {"name": "Disposable OAuth validation",
                   "type": "k8s", "config": target.settings["config"]})
        environment = {key: home + suffix for key, suffix in (
            ("HOME", ""), ("XDG_DATA_HOME", "/data"),
            ("XDG_CONFIG_HOME", "/config"), ("XDG_STATE_HOME", "/state"))}
        if require_refresh:
            environment.update(HTTPS_PROXY=proxy.url(settings["proxy_reachable"]),
                           HTTP_PROXY=proxy.url(settings["proxy_reachable"]),
                           NO_PROXY="127.0.0.1,localhost", NODE_EXTRA_CA_CERTS=home + "/test-ca.pem")
        # This is a public certificate, not a credential or private key.
        prepare = ('mkdir -p "$HOME/.npm"\ncat > "$HOME/test-ca.pem" <<\'KANDEV_TEST_CA\'\n' +
                   certificate + '\nKANDEV_TEST_CA\nnpx --yes @opencode/cli@' + version +
                   ' --version || true\nfind "$HOME/.npm/_npx" -path "*/@opencode/cli/postinstall.mjs" -exec node {} \\;\n')
        profile = controller.api("executors/" + executor["id"] + "/profiles", {
            "name": "Disposable OAuth validation", "prepare_script": prepare,
            "env_vars": [{"key": key, "value": value} for key, value in environment.items()],
            "config": {**target.settings["profile_config"],
                       "remote_credentials": json.dumps(["agent:opencode-acp:files:0"])}})
        task = controller.api("tasks", {"title": "OAuth refresh validation",
            "workspace_id": controller.api("workspaces")["workspaces"][0]["id"],
            "workflow_id": controller.api("workflows")["workflows"][0]["id"],
            "agent_profile_id": agent_profile, "executor_id": executor["id"],
            "executor_profile_id": profile["id"], "start_agent": True, "description": prompt})
        tasks.append(task)
        return task

    existed = source.exists()
    if existed:
        source.rename(held)
    try:
        controller.auth("import", active)
        with tempfile.TemporaryDirectory(prefix="kandev-refresh-", dir="/tmp/opencode") as directory:
            proxy = module.RefreshProxy(directory, host=settings["proxy_bind"])
            proxy.start()
            try:
                certificate = proxy.cert.read_text()
                with controller.running():
                    controller.select_version(version)
                    agent = next(a for a in controller.api("agents")["agents"] if a["name"] == "opencode-acp")
                    token = controller.api("app-state?path=/settings/agents")["interimSettingsInterlockToken"]
                    profile = controller.api("agents/" + agent["id"] + "/profiles", {
                        "name": "OAuth refresh validation", "model": settings["model"],
                        "require_exact_model": True}, {"X-Kandev-Interim-Settings-Interlock": token})
                    try:
                        a = create(profile["id"], proxy, certificate)
                        b = create(profile["id"], proxy, certificate)
                        env = dict(controller.env, HTTPS_PROXY=proxy.url(settings["proxy_reachable"]),
                                   HTTP_PROXY=proxy.url(settings["proxy_reachable"]),
                                    NODE_EXTRA_CA_CERTS=str(proxy.cert))
                        if not require_refresh:
                            env = dict(controller.env)
                        with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
                            futures = [pool.submit(wait_reply, task, proxy) for task in (a, b)]
                            host = pool.submit(command, [controller.settings["cli"], "run", "--standalone", "--model",
                                               settings["model"], prompt], env=env, timeout=300)
                            if nonce.encode() not in host.result():
                                raise RuntimeError("Controller provider response missing")
                            for future in futures:
                                future.result()
                        evidence = proxy.evidence()
                        if require_refresh and (evidence["aborted"] or not evidence["overlap"] or evidence["statuses"] != [200, 200, 200]):
                            raise RuntimeError("Actual concurrent refresh was not proven")
                        c = create(profile["id"], proxy, certificate)
                        wait_reply(c, proxy)
                        resumed = "RESUMED_" + uuid.uuid4().hex
                        for task in (a, b, c):
                            print("Checking provider response after session resume", flush=True)
                            controller.action("session.stop", {"session_id": task["session_id"]})
                            controller.action("session.launch", {"task_id": task["id"],
                                "session_id": task["session_id"], "intent": "resume"})
                            controller.wait_session(task)
                            session = controller.api("task-sessions/" + task["session_id"])["session"]
                            identity = {"task_id": task["id"], "session_id": task["session_id"],
                                        "session_incarnation_id": session["queue_incarnation_id"]}
                            # Own dispatch explicitly: automatic queue processing can
                            # otherwise start the prompt before the drain request.
                            controller.action("message.queue.auto_run.set", {**identity, "enabled": False})
                            controller.action("message.queue.add", {**identity,
                                "content": "Reply with exactly " + resumed + ". Do not use tools."})
                            controller.action("message.queue.drain", identity)
                            wait_reply(task, proxy, resumed)
                        output = command([controller.settings["cli"], "run", "--standalone", "--model",
                            settings["model"], "Reply with exactly " + resumed + ". Do not use tools."],
                            env=env, timeout=300)
                        if resumed.encode() not in output:
                            raise RuntimeError("Controller subsequent provider response missing")
                        print(json.dumps({"oauth_refresh": "passed" if require_refresh else "waived", "fresh_session": "passed",
                                          "post_resume_responses": "passed",
                                          "refresh_statuses": evidence["statuses"], "overlap": evidence["overlap"]}), flush=True)
                    finally:
                        for task in tasks:
                            try:
                                controller.action("session.stop", {"session_id": task["session_id"]})
                            except RuntimeError:
                                print("Cleanup: session stop failed; removing owned worker below", flush=True)
                            finally:
                                if not proxy.evidence()["aborted"]:
                                    for name in target.workers(task["id"]):
                                        command(target.settings["kubectl_prefix"] + ["delete", name, "--wait=false"])
            finally:
                evidence = proxy.evidence()
                preserve_for_recovery = evidence["aborted"]
                print(json.dumps({"refresh_observation": evidence}), flush=True)
                proxy.close()
    finally:
        if source.exists():
            if preserve_for_recovery:
                source.rename(source.with_name("opencode-oauth-recovery-" + uuid.uuid4().hex))
            else:
                shutil.rmtree(source)
        if existed:
            held.rename(source)
