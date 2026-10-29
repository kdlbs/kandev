"""Managed acceptance against one explicitly selected disposable fixture.

This module never finds an instance or credential file automatically. It creates
its own tasks and sessions, uses the operator's explicit test login, and removes
only those sessions. All raw responses stay in the private fixture directory.
"""
import fcntl
import hashlib
import http.cookiejar
import http.server
import ipaddress
import json
import os
from pathlib import Path
import secrets
import socket
import sqlite3
import struct
import subprocess
import termios
import threading
import time
import traceback
import urllib.error
import urllib.parse
import urllib.request
import uuid


class Blocked(Exception):
    pass


class Failed(Exception):
    pass


def require(condition, reason):
    if not condition:
        raise Failed(reason)


def contained(root, value):
    path = Path(value)
    return path.is_absolute() and path.resolve().is_relative_to(root)


def birth(pid):
    return Path(f'/proc/{pid}/stat').read_text().rsplit(') ', 1)[1].split()[19]


def owns_listener(pid, host, port):
    address = socket.inet_aton(host)[::-1].hex().upper() + f':{port:04X}'
    inodes = {row.split()[9] for row in Path('/proc/net/tcp').read_text().splitlines()[1:]
              if row.split()[1] == address and row.split()[3] == '0A'}
    pending, seen = [pid], set()
    while pending and len(seen) < 4096:
        current = pending.pop()
        if current in seen:
            continue
        seen.add(current)
        try:
            proc = Path(f'/proc/{current}')
            if any(link.readlink().as_posix() in {f'socket:[{inode}]' for inode in inodes}
                   for link in (proc / 'fd').iterdir()):
                return True
            pending.extend(int(value) for value in (proc / f'task/{current}/children').read_text().split())
        except (OSError, ValueError):
            continue
    return False


def validate_seed(seed):
    if seed.get('schema_version') != 1 or seed.get('purpose') != 'nono_acceptance' or seed.get('status') != 'ready':
        raise Blocked('fixture_identity_required')
    root = Path(seed['root'])
    url = urllib.parse.urlsplit(seed['url'])
    if (url.scheme != 'http' or url.username or url.password or url.query or url.fragment
            or url.path not in ('', '/') or not url.port or url.port == 9998
            or url.port != seed['port'] or ipaddress.ip_address(url.hostname).version != 4):
        raise Blocked('unsafe_fixture_address')
    if (not root.is_dir() or root.resolve() != root or not root.name.startswith('kw-')
            or root.stat().st_mode & 0o077 or root.stat().st_uid != os.getuid()):
        raise Blocked('unsafe_fixture_root')
    if birth(seed['pid']) != seed['birth'] or not owns_listener(seed['pid'], url.hostname, url.port):
        raise Blocked('fixture_process_identity_changed')
    for field in ('tools_dir', 'repository_path'):
        if not contained(root, seed[field]):
            raise Blocked('foreign_fixture_path')
    return root


class Client:
    def __init__(self, seed, auth_file):
        self.seed, self.root, self.auth_file = seed, validate_seed(seed), auth_file
        self.private = self.root / ('nono-product-' + secrets.token_hex(8))
        self.private.mkdir(mode=0o700)
        jar = self.root / 'operator-cookies.txt'
        require(contained(self.root, jar) and jar.is_file(), 'fixture_operator_login_missing')
        self.cookies = http.cookiejar.MozillaCookieJar(str(jar))
        self.cookies.load(ignore_discard=True)
        self.http = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(self.cookies))
        self.helper = self.private / 'ws-client'
        self.sessions, self.checks, self.counter = [], {}, 0
        self.db = self.root / 'kandev.db'
        require(self.db.is_file(), 'fixture_database_missing')

    def save(self, label, value):
        target = self.private / (label + '.json')
        with os.fdopen(os.open(target, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600), 'w') as out:
            json.dump(value, out)

    def api(self, path, data=None, method=None):
        headers = {'Content-Type': 'application/json', 'Origin': self.seed['url']}
        if data is not None or method in ('PUT', 'DELETE'):
            boot = self.api('/api/v1/app-state?path=%2Fsettings%2Fagents')
            headers['X-Kandev-Interim-Settings-Interlock'] = boot['interimSettingsInterlockToken']
        request = urllib.request.Request(self.seed['url'] + path,
            data=json.dumps(data).encode() if data is not None else None, method=method, headers=headers)
        try:
            with self.http.open(request, timeout=90) as response:
                raw = response.read()
                return json.loads(raw) if raw else None
        except urllib.error.HTTPError as error:
            raise Failed('fixture_http_' + str(error.code)) from None

    def ws_input(self, action, payload):
        return {'kind': 'request', 'url': self.seed['url'].replace('http:', 'ws:') + '/ws',
            'origin': self.seed['url'], 'cookie': '; '.join(item.name + '=' + item.value for item in self.cookies),
            'id': str(uuid.uuid4()), 'action': action, 'payload': payload}

    def ws(self, action, payload):
        result = subprocess.run([str(self.helper)], input=json.dumps(self.ws_input(action, payload)).encode(),
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=80)
        require(result.returncode == 0, 'fixture_websocket_transport_failed')
        response = json.loads(result.stdout)
        self.counter += 1
        self.save(f'ws-{self.counter}', response)
        require(response.get('type') != 'error', 'fixture_websocket_request_refused')
        return response.get('payload')

    def session(self, identity):
        data = self.api('/api/v1/task-sessions/' + identity)
        return data.get('session', data)

    def attempt(self, identity):
        with sqlite3.connect(f'file:{self.db}?mode=ro', uri=True) as database:
            row = database.execute('SELECT attempt_id,state,snapshot_json,owner_json FROM sandbox_attempts WHERE session_id=? ORDER BY updated_at DESC LIMIT 1', (identity,)).fetchone()
        if not row:
            return None
        return {'id': row[0], 'state': row[1], 'snapshot': json.loads(row[2]), 'owner': json.loads(row[3]) if row[3] else None}

    def wait(self, predicate, reason, seconds=45):
        until = time.monotonic() + seconds
        while time.monotonic() < until:
            value = predicate()
            if value:
                return value
            time.sleep(.15)
        raise Failed(reason)

    def queue(self, session, content):
        current = self.session(session['id'])
        return self.ws('message.queue.add', {'task_id': session['task'], 'session_id': session['id'],
            'session_incarnation_id': current['queue_incarnation_id'], 'client_queue_id': uuid.uuid4().hex, 'content': content})

    def plan(self, session):
        marker = 'NONO_PRODUCT_' + secrets.token_hex(16)
        self.ws('task.plan.create', {'task_id': session['task'], 'title': 'Owned acceptance marker', 'content': marker})
        return marker

    def prepare(self, kind):
        profile = self.seed['profiles'][kind]
        task = self.api('/api/v1/tasks', {'workspace_id': self.seed['workspace_id'], 'workflow_id': self.seed['workflow_id'],
            'workflow_step_id': self.seed['step_id'], 'title': 'Owned Nono product acceptance: ' + kind,
            'description': '', 'prepare_session': True, 'agent_profile_id': profile, 'executor_id': 'exec-local',
            'metadata': {'nono_acceptance': self.private.name},
            'repositories': [{'repository_id': self.seed['repository_id'], 'base_branch': 'main'}]})
        session = {'id': task['session_id'], 'task': task['id'], 'kind': kind}
        self.sessions.append(session)
        attempt = self.wait(lambda: self.attempt(session['id']), 'sandbox_attempt_not_captured')
        workspace = self.wait(lambda: self.session(session['id']).get('workspace_path'), 'workspace_prepare_not_committed')
        if kind == 'terminal':
            self.wait(lambda: (self.attempt(session['id']) or {}).get('state') == 'running', 'native_start_not_captured')
            self.stop(session)
        state = attempt['snapshot']['agent_state']
        home = Path(state['home'])
        session['home'] = home
        require(contained(self.root, home) and state['scope']['session_id'] == session['id']
                and state['scope']['agent_profile_id'] == profile, 'foreign_agent_state')
        directory = home / '.pi/agent'
        directory.mkdir(parents=True, exist_ok=True, mode=0o700)
        require(contained(home, directory), 'foreign_credential_destination')
        target = directory / ('.test-auth-' + secrets.token_hex(8))
        with os.fdopen(os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600), 'wb') as out:
            out.write(self.auth_file.read_bytes())
        target.replace(directory / 'auth.json')
        session['work'] = Path(workspace)
        require(contained(self.root, session['work']), 'foreign_task_workspace')
        self.ws('session.launch', {'task_id': session['task'], 'session_id': session['id'],
            'intent': 'resume' if kind == 'terminal' else 'start_created'})
        self.wait(lambda: (self.attempt(session['id']) or {}).get('state') == 'running', 'sandbox_owner_missing')
        return session

    def mark(self, name):
        self.checks[name] = 'passed'
        print('Managed Nono acceptance: ' + name, flush=True)

    def build_helper(self):
        repo = Path(__file__).resolve().parent.parent
        with (self.private / 'helper-build.log').open('wb') as log:
            result = subprocess.run(['go', '-C', str(repo / 'apps/backend'), 'build', '-trimpath', '-o', str(self.helper),
                str(repo / 'scripts/fixtures/nono-managed-ws.go')], stdout=log, stderr=subprocess.STDOUT, timeout=120)
        require(result.returncode == 0, 'acceptance_client_build_failed')

    def stop(self, session):
        previous = self.attempt(session['id'])
        self.ws('session.stop', {'session_id': session['id']})
        if previous:
            self.wait(lambda: (current := self.attempt(session['id']))
                and current['id'] == previous['id'] and current['state'] == 'stopped',
                'captured_attempt_teardown_not_finished')

    def cleanup(self):
        for session in reversed(self.sessions):
            if session.get('deleted'):
                continue
            self.stop(session)
            self.ws('session.delete', {'session_id': session['id']})
            if session.get('home'):
                require(not session['home'].exists(), 'retained_state_not_removed')
            session['deleted'] = True


def acp_roundtrip(client, session, suffix):
    marker = client.plan(session)
    receipt = session['work'] / (client.private.name + '-' + suffix + '.txt')
    client.queue(session, 'Call get_task_plan_kandev through Kandev MCP for this task. '
        'Use tool_search or codemode if needed. Write only its exact plan content to ' + receipt.name
        + '. Do not guess contents, inspect credentials, change trust or policy, or advance the task.')
    client.wait(receipt.exists, 'actual_acp_mcp_receipt_missing')
    require(receipt.read_text().strip() == marker, 'actual_acp_mcp_receipt_mismatch')
    attempt = client.attempt(session['id'])
    require(attempt['state'] == 'running' and attempt['owner'], 'captured_runtime_owner_missing')
    client.mark(suffix)


def pty_size(client, session):
    attempt = client.attempt(session['id'])
    if not attempt or not attempt['owner']:
        return None
    identity = attempt['owner']
    pid = identity['child_pid']
    token = 'linux:' + Path('/proc/sys/kernel/random/boot_id').read_text().strip() + ':' + birth(pid)
    if token != identity['child_birth']:
        return None
    descriptor = os.open(f'/proc/{pid}/fd/0', os.O_RDONLY | os.O_NONBLOCK | os.O_NOCTTY)
    try:
        rows, columns, _, _ = struct.unpack('HHHH', fcntl.ioctl(descriptor, termios.TIOCGWINSZ, bytes(8)))
        return columns, rows
    finally:
        os.close(descriptor)


def native_roundtrip(client, session):
    marker = client.plan(session)
    receipt = session['work'] / (client.private.name + '-native.txt')
    request = client.ws_input('', {})
    request.update(kind='terminal', url=client.seed['url'].replace('http:', 'ws:')
        + '/terminal/session/' + session['id'] + '?mode=agent', output=str(client.private / 'terminal.bin'),
        input='Call get_task_plan_kandev through Kandev MCP for this task. Use codemode or tool_search. '
        'Write its exact plan content alone to ' + receipt.name
        + '. Do not guess contents, inspect credentials, change trust or policy, or advance the task.')
    process = subprocess.Popen([str(client.helper)], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    process.stdin.write(json.dumps(request).encode())
    process.stdin.close()
    sizes = set()
    try:
        def received():
            try:
                size = pty_size(client, session)
                if size:
                    sizes.add(size)
            except OSError:
                pass
            return receipt.exists() and {(120, 40), (88, 28)} <= sizes
        client.wait(received, 'native_input_or_resize_receipt_missing')
        require(receipt.read_text().strip() == marker, 'actual_native_mcp_receipt_mismatch')
    finally:
        if process.poll() is None:
            process.terminate()
        process.wait(timeout=10)
    require(client.attempt(session['id'])['state'] == 'running', 'terminal_disconnect_stopped_supervisor')
    client.save('terminal-sizes', sorted(sizes))
    client.mark('native_input_resize_mcp_and_disconnect')


def negative_controls(client, session):
    counts = {'positive': 0, 'forbidden': 0}
    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            counts['positive' if self.path == '/positive' else 'forbidden'] += 1
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'owned control')
        def log_message(self, *_args):
            pass
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    destination = 'http://127.0.0.1:' + str(server.server_port)
    fixture = session['work'] / (client.private.name + '-negative.json')
    script = session['work'] / (client.private.name + '-negative.cjs')
    receipt = session['work'] / (client.private.name + '-negative-receipt.json')
    outside = client.private / 'outside.txt'
    outside.write_text('owned sentinel')
    body = {'outside': str(outside), 'control': client.private.name + '-control.txt',
            'destination': destination, 'receipt': receipt.name}
    fixture.write_text(json.dumps(body))
    script.write_bytes((Path(__file__).parent / 'fixtures/nono-managed-negative.cjs').read_bytes())
    try:
        with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(destination + '/positive') as response:
            require(response.status == 200, 'network_positive_control_failed')
        client.queue(session, 'Run node ' + script.name + ' ' + fixture.name
            + ' using bash. This is an operator-owned confinement check. Do not edit the program or policy or print diagnostics.')
        client.wait(receipt.exists, 'negative_receipt_missing')
        result = json.loads(receipt.read_text())
        require(len(result) == 8 and all(value is True for value in result.values()), 'negative_control_failed')
        require(counts == {'positive': 1, 'forbidden': 0}, 'forbidden_destination_reached')
        require(outside.read_text() == 'owned sentinel', 'outside_file_changed')
        client.save('negative-controls', {'checks': result, 'destination_requests': counts})
    finally:
        server.shutdown()
        server.server_close()
    client.mark('permitted_work_and_filesystem_network_denials')


def cancellation_and_restart(client, session, sibling):
    prefix = client.private.name + '-cancel'
    script = session['work'] / (prefix + '.cjs')
    script.write_bytes((Path(__file__).parent / 'fixtures/nono-managed-cancel.cjs').read_bytes())
    started = session['work'] / (prefix + '-started.json')
    client.queue(session, 'Run node ' + script.name + ' ' + prefix
        + ' using bash with a timeout of 600 seconds. Wait for it. The operator will cancel it immediately. Do not edit it or start other commands.')
    client.wait(started.exists, 'active_cancel_command_missing')
    identities = {kind: (pid, birth(pid)) for kind, pid in json.loads(started.read_text()).items()}
    previous = client.attempt(session['id'])
    client.ws('agent.cancel', {'session_id': session['id']})
    client.stop(session)
    client.save('cancellation', {'processes': identities, 'before': previous,
        'after': client.attempt(session['id'])})
    for pid, token in identities.values():
        try:
            fields = Path(f'/proc/{pid}/stat').read_text().rsplit(') ', 1)[1].split()
            require(fields[19] != token or fields[0] == 'Z', 'owned_descendant_survived_stop')
        except FileNotFoundError:
            pass
    require(client.attempt(sibling['id'])['state'] == 'running', 'sibling_interrupted')
    require(session['home'].is_dir(), 'restart_state_removed_early')
    client.ws('session.launch', {'task_id': session['task'], 'session_id': session['id'], 'intent': 'resume'})
    def restarted():
        current = client.attempt(session['id'])
        return current if current and current['id'] != previous['id'] and current['state'] == 'running' else None
    current = client.wait(restarted, 'same_session_restart_failed')
    require(current['snapshot']['resolved'] == previous['snapshot']['resolved'], 'captured_selection_changed')
    require(current['snapshot']['agent_state'] == previous['snapshot']['agent_state'], 'restart_state_changed')
    require(current['snapshot']['launch']['mcp_url'] != previous['snapshot']['launch']['mcp_url'], 'endpoint_not_rotated')
    acp_roundtrip(client, session, 'same_session_restart_and_rotated_mcp')
    client.mark('active_cancellation_descendant_cleanup_and_sibling_isolation')


def run_transport(client):
    client.build_helper()
    acp = client.prepare('chat')
    acp_roundtrip(client, acp, 'actual_acp_tools_and_mcp')
    native = client.prepare('terminal')
    native_roundtrip(client, native)
    negative_controls(client, acp)
    cancellation_and_restart(client, acp, native)
    client.stop(native)
    client.ws('session.delete', {'session_id': native['id']})
    native['deleted'] = True
    require(not native['home'].exists() and acp['home'].exists(), 'session_state_scope_cleanup_failed')
    client.cleanup()
    require(Path(client.seed['repository_path']).is_dir(), 'workspace_removed')
    client.mark('scoped_session_state_cleanup_and_workspace_retention')


def main(binary, record):
    fixture = os.environ.get('NONO_ACCEPTANCE_SEED_MANIFEST')
    if not fixture:
        print(json.dumps({'status': 'blocked', 'reason': 'missing_product_fixture'}))
        return 2
    auth = Path(os.environ.get('NONO_ACCEPTANCE_PI_AUTH', '/nonexistent'))
    if not auth.is_file() or auth.stat().st_size > 1 << 20:
        print(json.dumps({'status': 'blocked', 'reason': 'explicit_test_login_required'}))
        return 2
    client = None
    try:
        seed = json.loads(Path(fixture).read_text())
        root = validate_seed(seed)
        for package, version in [('pi-acp', '0.0.34'), ('@earendil-works/pi-coding-agent', '1.3.0')]:
            info = json.loads((Path(seed['tools_dir']) / 'node_modules' / package / 'package.json').read_text())
            require(info['version'] == version, 'unsupported_agent_runtime')
        with (root / 'nono-acceptance.lock').open('a+') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            client = Client(seed, auth)
            try:
                run_transport(client)
            finally:
                client.cleanup()
        report = {'scope': 'managed_transport_acceptance', 'status': 'passed', 'checks': client.checks,
            'nono_binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'pi_acp_version': '0.0.34',
            'pi_version': '1.3.0', 'host_base_commit': seed.get('source'),
            'host_binary_sha256': hashlib.sha256((root / 'bin/kandev').read_bytes()).hexdigest(),
            'agentctl_binary_sha256': hashlib.sha256((root / 'bin/agentctl').read_bytes()).hexdigest(),
            'active_status_evidence': 'unsupported',
            'limitations': ['This mode uses one explicitly selected disposable authenticated fixture.',
                'Operator approval, learning, and policy selection use separate integration and managed evidence.',
                'No accepted current policy attestation exists in the pinned Nono registry. Status remains unknown.']}
        client.save('summary', report)
        if record:
            with record.open('a') as out:
                out.write('\n## Managed transport acceptance\n\n```json\n' + json.dumps(report, indent=2) + '\n```\n')
        print(json.dumps(report))
        return 0
    except Blocked as error:
        print(json.dumps({'status': 'blocked', 'reason': str(error)}))
        return 2
    except (Failed, OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
        reason = str(error) if isinstance(error, Failed) else 'private_fixture_failure'
        if client:
            client.save('failure', {'class': type(error).__name__, 'frames': [
                {'file': Path(frame.filename).name, 'line': frame.lineno, 'function': frame.name}
                for frame in traceback.extract_tb(error.__traceback__)]})
        print(json.dumps({'status': 'failed', 'scope': 'managed_transport_acceptance', 'reason': reason}))
        return 1
