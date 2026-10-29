#!/usr/bin/env python3
"""Pinned Linux Nono conformance and opt-in managed product acceptance.

Conformance does not read credentials or Kandev state. Managed acceptance requires
an explicitly selected disposable fixture and test credential file. Both modes
require the published 0.80.0 Linux x86_64 musl artifact.
"""
import argparse
import ctypes
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess

VERSION = '0.80.0'
SOURCE = '8bb041356d48f771a221e58c98d9b1ff442e76fe'
ARCHIVE_SHA256 = 'cae6755f1188b561ea02f62995feb8ad87d457f6d5255956ec54f41bbefca31e'
BINARY_SHA256 = 'a66396777ed756dff196ef61cf704abd4feebf58599d41e2da3363bbe90e22af'


def prerequisites(env):
    if platform.system() != 'Linux' or platform.machine() not in ('x86_64', 'amd64'):
        return None, 'unsupported_host'
    binary = Path(env.get('NONO_ACCEPTANCE_BINARY', '/nonexistent')).resolve()
    if not binary.is_file() or not os.access(binary, os.X_OK):
        return None, 'missing_binary'
    with binary.open('rb') as source:
        digest = hashlib.file_digest(source, 'sha256').hexdigest()
    if digest != BINARY_SHA256:
        return None, 'binary_digest_mismatch'
    return binary, None


# Reuse the existing owned-process harness. Its process identity and cleanup
# tests also protect this runner; no Kandev instance is started here.
import fcntl
import http.server
import importlib.util
import pty
import select
import signal
import socket
import struct
import sys
import threading
import time

_spec = importlib.util.spec_from_file_location('wrapper_acceptance', Path(__file__).with_name('check-generic-wrapper-real.py'))
owned_runner = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(owned_runner)


class Failed(Exception):
    """A closed, credential-free failure code."""


def require(condition, code):
    if not condition:
        raise Failed(code)


class Probe(owned_runner.OwnedRun):
    def __init__(self, binary):
        super().__init__()
        self.binary = binary
        self.env = {'HOME': str(self.root / 'home'), 'XDG_CONFIG_HOME': str(self.root / 'config'),
                    'XDG_STATE_HOME': str(self.root / 'state'), 'TMPDIR': str(self.root / 'work'),
                    'PATH': os.defpath, 'TERM': 'xterm-256color', 'LANG': 'C.UTF-8'}
        self.python = str(Path(sys.executable).resolve())
        self.read_roots = [str(Path(self.python).parent)]
        # Runtime dependencies only. No real home, Kandev state, or temporary root.
        for path in ('/usr', '/lib', '/lib64'):
            if Path(path).is_dir():
                self.read_roots.append(str(Path(path).resolve()))
        if self.python.startswith('/nix/store/'):
            self.read_roots.append('/nix/store')

    def argv(self, command, profile=None):
        args = [str(self.binary), 'run', '--silent', '--block-net', '--allow', str(self.root / 'work')]
        for path in dict.fromkeys(self.read_roots):
            args.extend(['--read', path])
        if profile:
            args.extend(['--profile', str(profile)])
        return args + ['--'] + command

    def spawn(self, command, name, profile=None, terminal=None):
        log = (self.root / 'logs' / (name + '.log')).open('wb')
        self.logs.append(log)
        proc = subprocess.Popen(self.argv(command, profile), cwd=self.root / 'work',
            env={**self.env, 'WRAPPER_ACCEPTANCE_OWNER': self.nonce},
            stdin=subprocess.PIPE if terminal is None else terminal,
            stdout=subprocess.PIPE if terminal is None else terminal,
            stderr=log if terminal is None else terminal, start_new_session=True)
        self.processes.append(proc)
        self._record()
        return proc

    def run(self, code, name, extra=(), profile=None, input_bytes=None):
        proc = self.spawn([self.python, '-c', code, *map(str, extra)], name, profile)
        try:
            output, _ = proc.communicate(input_bytes, timeout=15)
        except subprocess.TimeoutExpired:
            self.stop(proc)
            raise Failed(name + '_timeout') from None
        return proc.returncode, output


def pipe_check(probe):
    status, output = probe.run('import sys; sys.stdout.buffer.write(sys.stdin.buffer.read())',
                              'pipe', input_bytes=b'{"jsonrpc":"2.0","id":1}\n')
    require(status == 0 and output == b'{"jsonrpc":"2.0","id":1}\n', 'pipe_transparency')


def enforcement_checks(probe):
    outside = probe.root / 'outside.txt'
    outside.write_text('synthetic-outside-data')
    with socket.socket() as server:
        server.bind(('127.0.0.1', 0))
        server.listen()
        port = server.getsockname()[1]
        # A positive control distinguishes denial from an unavailable endpoint.
        with socket.create_connection(('127.0.0.1', port), timeout=2):
            pass
        code = '''import errno,json,pathlib,socket,sys
outside=pathlib.Path(sys.argv[1]); result={}
pathlib.Path('permitted.txt').write_text('allowed')
result['permitted']=pathlib.Path('permitted.txt').read_text()=='allowed'
for operation in ('read','write'):
 try:
  outside.read_text() if operation=='read' else outside.write_text('forbidden')
  result[operation]=False
 except OSError as error: result[operation]=error.errno in (errno.EACCES,errno.EPERM)
try:
 with socket.create_connection(('127.0.0.1',int(sys.argv[2])),timeout=2):pass
 result['network']=False
except OSError as error: result['network']=error.errno in (errno.EACCES,errno.EPERM)
print(json.dumps(result))
'''
        status, output = probe.run(code, 'enforcement', [outside, port])
    require(status == 0, 'enforcement_child')
    result = json.loads(output)
    require(result == {'permitted': True, 'read': True, 'write': True, 'network': True}, 'enforcement_mismatch')
    require(outside.read_text() == 'synthetic-outside-data', 'outside_modified')


def approval_check(probe, decision):
    outside = probe.root / 'approval.txt'
    outside.write_text('synthetic-approved-marker')
    requests = []
    release = threading.Event()

    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            size = int(self.headers.get('Content-Length', '0'))
            if size < 1 or size > 65536:
                self.send_error(400)
                return
            requests.append(json.loads(self.rfile.read(size)))
            if decision == 'timeout':
                release.wait(4)  # Deliberately exceeds the configured two-second approval deadline.
            payload = json.dumps({'decision': decision}).encode()
            self.send_response(200)
            self.send_header('Content-Length', str(len(payload)))
            self.end_headers()
            try:
                self.wfile.write(payload)
            except (BrokenPipeError, ConnectionResetError):
                pass

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        profile = probe.root / ('approval-' + decision + '.json')
        profile.write_text(json.dumps({'meta': {'name': 'kandev-conformance'}, 'security': {
            'capability_elevation': True, 'approval_backends': {'kandev': {'type': 'webhook',
                'url': f'http://127.0.0.1:{server.server_port}/approve', 'timeout_secs': 2}},
            'approval_defaults': {'backend': 'kandev'}}}))
        profile.chmod(0o600)
        code = 'import pathlib,sys; print(pathlib.Path(sys.argv[1]).read_text())'
        status, output = probe.run(code, 'approval_' + decision, [outside], profile)
        require((status == 0 and b'synthetic-approved-marker' in output) == (decision == 'granted'),
                'approval_' + decision + '_decision')
        require(len(requests) == 1, 'approval_' + decision + '_count')
        request = requests[0].get('request', {})
        require(request.get('capability_type') == 'capability' and request.get('path') == str(outside),
                'approval_request_shape')
        require(bool(request.get('request_id')) and bool(request.get('session_id')) and
                isinstance(request.get('child_pid'), int), 'approval_request_identity')
    finally:
        release.set()
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


def read_terminal(fd, expected, timeout=10):
    deadline = time.monotonic() + timeout
    output = b''
    while time.monotonic() < deadline:
        if select.select([fd], [], [], min(0.2, deadline - time.monotonic()))[0]:
            try:
                part = os.read(fd, 8192)
            except OSError:
                break
            if not part:
                break
            output += part
            if expected in output:
                return
    raise Failed('pty_expected_output_missing')


def pty_checks(probe):
    master, slave = pty.openpty()
    try:
        fcntl.ioctl(slave, __import__('termios').TIOCSWINSZ, struct.pack('HHHH', 20, 80, 0, 0))
        code = '''import os,sys
print('probe-ready',flush=True)
for line in sys.stdin:
 if line.strip()=='size':
  size=os.get_terminal_size(0); print(f'size:{size.columns}x{size.lines}',flush=True)
 else:print('reply:'+line.strip(),flush=True)
'''
        proc = probe.spawn([probe.python, '-u', '-c', code], 'pty', terminal=slave)
        os.close(slave)
        slave = None
        read_terminal(master, b'probe-ready')
        os.write(master, b'hello\n')
        read_terminal(master, b'reply:hello')
        fcntl.ioctl(master, __import__('termios').TIOCSWINSZ, struct.pack('HHHH', 40, 100, 0, 0))
        os.killpg(proc.pid, signal.SIGWINCH)
        os.write(master, b'size\n')
        read_terminal(master, b'size:100x40')
        # The registry proves process identity only, not the installed policy.
        records = list((probe.root / 'state/nono/sessions').glob('*.json'))
        matching = [json.loads(path.read_text()) for path in records]
        matching = [record for record in matching if record.get('supervisor_pid') == proc.pid]
        require(len(matching) == 1 and matching[0].get('child_pid', 0) > 0, 'runtime_identity_missing')
        require('policy_digest' not in matching[0], 'runtime_evidence_shape_changed')
        probe.stop(proc)
        # Detached descendants must be removed without touching foreign listeners.
        probe._stop_descendants()
        require(not probe._owned_descendants(), 'owned_cleanup_failed')
    finally:
        os.close(master)
        if slave is not None:
            os.close(slave)


def conformance(binary):
    checks = {}
    with Probe(binary) as probe:
        version = subprocess.run([str(binary), '--version'], env=probe.env, cwd=probe.root,
                                 capture_output=True, timeout=5, check=True)
        require(version.stdout.strip() == ('nono ' + VERSION).encode(), 'runtime_version_mismatch')
        pipe_check(probe)
        checks['pipe'] = 'passed'
        enforcement_checks(probe)
        checks.update(filesystem='passed', network='passed')
        for decision in ('denied', 'granted', 'invalid', 'timeout'):
            approval_check(probe, decision)
            checks['approval_' + decision] = 'passed'
        pty_checks(probe)
        checks.update(pty_input_resize='passed', cancellation_cleanup='passed')
        pipe_check(probe)
        checks['fresh_restart'] = 'passed'
    return {'scope': 'runtime_conformance', 'status': 'passed', 'version': VERSION,
            'source_commit': SOURCE, 'archive_sha256': ARCHIVE_SHA256, 'binary_sha256': BINARY_SHA256,
            'platform': 'linux/x86_64', 'checks': checks, 'active_status_evidence': 'unsupported',
            'limitations': ['No current policy digest in the session registry',
                            'Fresh runtime restart is not Kandev same-session recovery',
                            'No ACP agent, provider login, MCP, callback isolation, or plugin acceptance'],
            'recorded_at': datetime.now(timezone.utc).isoformat()}


def landlock_abi():
    # Linux x86_64 only, guarded by prerequisites. This queries, not installs, a ruleset.
    return max(0, ctypes.CDLL(None, use_errno=True).syscall(444, None, 0, 1))


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--check-prerequisites', action='store_true')
    mode.add_argument('--conformance', action='store_true')
    mode.add_argument('--run', action='store_true')
    parser.add_argument('--record', type=Path)
    args = parser.parse_args(argv)
    binary, error = prerequisites(os.environ)
    if error:
        print(json.dumps({'status': 'blocked', 'reason': error}))
        return 2
    if landlock_abi() < 1:
        print(json.dumps({'status': 'blocked', 'reason': 'landlock_unavailable'}))
        return 2
    if args.check_prerequisites:
        print(json.dumps({'status': 'ready', 'version': VERSION, 'binary_sha256': BINARY_SHA256,
                          'scope': 'binary_and_host_prerequisites', 'landlock_abi': landlock_abi()}))
        return 0
    if args.run:
        spec = importlib.util.spec_from_file_location('nono_managed', Path(__file__).with_name('nono_managed.py'))
        managed = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(managed)
        return managed.main(binary, args.record)
    def interrupted(signum, frame):
        raise KeyboardInterrupt()
    previous = signal.signal(signal.SIGTERM, interrupted)
    try:
        report = conformance(binary)
    except (Failed, owned_runner.CheckFailed, OSError, ValueError, KeyError, subprocess.SubprocessError, KeyboardInterrupt) as exc:
        code = str(exc) if isinstance(exc, Failed) else 'runtime_error'
        print(json.dumps({'status': 'failed', 'scope': 'runtime_conformance', 'reason': code}))
        return 1
    finally:
        signal.signal(signal.SIGTERM, previous)
    if args.record:
        with args.record.open('a') as record:
            record.write('\n## Pinned runtime conformance\n\n```json\n' + json.dumps(report, indent=2) + '\n```\n')
    print(json.dumps(report))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
