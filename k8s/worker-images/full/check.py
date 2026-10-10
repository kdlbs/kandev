#!/usr/bin/env python3
"""Run one bounded check in the task daemon, outside agent memory."""
import argparse
from dataclasses import dataclass
import json
import os
from pathlib import Path
import re
import signal
import shlex
import subprocess
import sys
import time
import uuid

SLOT = 'kandev-validation'
WORKSPACE = Path('/workspace')
RECEIPT = Path('/run/docker/validation-accounting.json')
IMAGE_RE = re.compile(r'(?:[a-z0-9][a-z0-9./:_-]*@)?sha256:[a-f0-9]{64}')
ENV_ALLOWLIST = ('LANG', 'LC_ALL', 'TZ', 'CI', 'KANDEV_E2E_MOCK',
                 'KANDEV_E2E_WS_ASSERT', 'KANDEV_E2E_RUN_INSTRUMENTATION',
                 'KANDEV_E2E_TEST_TIMEOUT', 'E2E_PORT_OFFSET', 'CAPTURE_PR_ASSETS')


@dataclass(frozen=True)
class Policy:
    image: str
    memory: int = 2 * 1024**3
    budget: int = 3 * 1024**3
    reserve: int = 1024**3
    cpus: int = 2
    pids: int = 512
    queue: int = 60
    job: int = 1800

    @classmethod
    def from_env(cls, env):
        image = env.get('FULL_WORKER_CHECK_IMAGE', '')
        if not IMAGE_RE.fullmatch(image):
            raise ValueError('validation image must be an immutable digest or exact image ID')
        fields = {'memory': ('MEMORY_BYTES', 2 * 1024**3, 128 * 1024**3),
                  'budget': ('BUDGET_BYTES', 3 * 1024**3, 128 * 1024**3),
                  'reserve': ('RESERVE_BYTES', 1024**3, 128 * 1024**3),
                  'cpus': ('CPUS', 2, 64), 'pids': ('PIDS', 512, 4096),
                  'queue': ('QUEUE_SECONDS', 60, 300), 'job': ('JOB_SECONDS', 1800, 3600)}
        values = {}
        for field, (key, default, maximum) in fields.items():
            raw = env.get('FULL_WORKER_CHECK_' + key, str(default))
            if not re.fullmatch(r'[1-9][0-9]*', raw) or int(raw) > maximum:
                raise ValueError('invalid validation policy: ' + key)
            values[field] = int(raw)
        if values['memory'] + values['reserve'] > values['budget']:
            raise ValueError('validation memory exceeds companion budget minus daemon reserve')
        return cls(image=image, **values)


def workspace_cwd(cwd, root=WORKSPACE):
    root, cwd = root.resolve(strict=True), cwd.resolve(strict=True)
    if not cwd.is_relative_to(root):
        raise ValueError('validation working directory must be inside the canonical workspace')
    return cwd


def validate_receipt(policy, receipt, info):
    if (receipt.get('version') != 1 or not receipt.get('generation')
            or receipt.get('daemon_id') != info.get('ID')
            or info.get('CgroupVersion') != '2' or info.get('CgroupDriver') != 'cgroupfs'
            or receipt.get('memory_max') != policy.budget
            or receipt.get('probe_memory_max') != 64 * 1024**2
            or receipt.get('probe_memory_current', 0) < 8 * 1024**2
            or receipt.get('parent_delta', 0) < 8 * 1024**2
            or not re.fullmatch(r'/docker/[a-zA-Z0-9_-]+', receipt.get('probe_cgroup', ''))):
        raise ValueError('validation companion accounting is unavailable or mismatched')


def memory_available(policy, containers):
    reserved = policy.memory + policy.reserve
    for container in containers:
        if container.get('State', {}).get('Running'):
            limit = container.get('HostConfig', {}).get('Memory', 0)
            if not isinstance(limit, int) or limit <= 0:
                raise ValueError('another daemon workload has no verifiable memory limit')
            reserved += limit
    return reserved <= policy.budget


def validate_browser_args(args):
    banned = {'containers', 'docker', 'kubernetes-compat'}
    for index, arg in enumerate(args):
        if arg == '--docker' or (index > 0 and args[index - 1] == '--project' and arg in banned) or arg.startswith('tests/docker/') or arg.startswith('tests/kubernetes/') or arg.startswith('tests/ssh/'):
            raise ValueError('daemon-dependent browser checks cannot run inside validation isolation')
        if arg.startswith('--project=') and arg.split('=', 1)[1] in banned:
            raise ValueError('daemon-dependent browser project is unsupported')
        if arg in ('--workers', '--shards', '-j'):
            if index + 1 >= len(args) or args[index + 1] != '1':
                raise ValueError('isolated browser checks require one worker and shard')
        elif arg.startswith(('--workers=', '--shards=', '-j=')):
            if arg.split('=', 1)[1] != '1':
                raise ValueError('isolated browser checks require one worker and shard')
        elif arg.startswith('-j') and arg != '-j' and arg != '-j1':
            raise ValueError('isolated browser checks require one worker')


def create_args(policy, cwd, kind, command, owner, env):
    if kind not in ('lint', 'build', 'test', 'browser') or not command:
        raise ValueError('validation kind and command are required')
    command = list(command)
    if Path(command[0]).name in ('make', 'gmake'):
        tokens = shlex.split(env.get('MAKEFLAGS', ''))
        if tokens and tokens[0].isalpha():
            tokens[0] = '-' + tokens[0]
        tokens = [token for token in tokens if token != '--' and not token.startswith('--jobserver-auth=') and not token.startswith('--jobserver-fds=')]
        flags = [token for token in tokens if '=' not in token or token.startswith('-')]
        assignments = [token for token in tokens if '=' in token and not token.startswith('-')]
        command = [command[0], *flags, *command[1:], *assignments]
    if kind == 'browser':
        validate_browser_args(command)
        if env.get('KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM'):
            raise ValueError('unsafe browser parallelism is unsupported in isolation')
    labels = {'version': '1', 'workspace': str(WORKSPACE), 'owner': owner,
              'image': policy.image, 'deadline': str(int(time.time()) + policy.job + 30)}
    args = ['create', '--pull=never', '--name', SLOT, '--init', '--memory', str(policy.memory),
            '--memory-swap', str(policy.memory), '--cpus', str(policy.cpus),
            '--pids-limit', str(policy.pids), '--user', '1000:1000',
            '--cap-drop=ALL', '--security-opt=no-new-privileges',
            '--mount', 'type=bind,src=/workspace,dst=/workspace',
            '--workdir', str(cwd), '--tmpfs', '/tmp:rw,exec,uid=1000,gid=1000,size=128m',
            '--entrypoint', '/bin/bash']
    for key, value in labels.items():
        args += ['--label', 'kandev.check.' + key + '=' + value]
    child_env = {key: env[key] for key in ENV_ALLOWLIST if key in env}
    child_env.update({'HOME': '/tmp/check-home', 'TMPDIR': '/tmp', 'GOMAXPROCS': '2',
                      'GOMEMLIMIT': str(policy.memory * 7 // 10) + 'B', 'GOFLAGS': '-p=1',
                      'FULL_WORKER_CHECK_MODE': 'isolated', 'FULL_WORKER_CHECK_INSIDE': '1',
                      'FULL_WORKER_CHECK_KIND': kind})
    for key, value in child_env.items():
        args += ['--env', key + '=' + value]
    supervisor = 'mkdir -p "$HOME"; exec timeout --signal=TERM --kill-after=10 ' + str(policy.job) + ' "$@"'
    return args + [policy.image, '-ceu', supervisor, '--'] + list(command)


class Docker:
    def call(self, *args):
        result = subprocess.run(['docker', *args], text=True, capture_output=True, timeout=15)
        if result.returncode:
            raise RuntimeError('Docker operation failed: ' + result.stderr.strip()[:400])
        return result.stdout.strip()

    def inspect(self, identifier):
        result = subprocess.run(['docker', 'inspect', '--type', 'container', identifier],
                                text=True, capture_output=True, timeout=15)
        if result.returncode:
            if 'No such object:' in result.stderr or 'No such container:' in result.stderr:
                return None
            raise RuntimeError('cannot verify validation workload: ' + result.stderr.strip()[:400])
        return json.loads(result.stdout)[0]


def owned_slot(slot, workspace):
    labels = slot.get('Config', {}).get('Labels') or {}
    if (labels.get('kandev.check.version') != '1'
            or labels.get('kandev.check.workspace') != str(workspace)
            or not labels.get('kandev.check.owner') or not slot.get('Id')
            or not IMAGE_RE.fullmatch(labels.get('kandev.check.image', ''))
            or not re.fullmatch(r'[1-9][0-9]*', labels.get('kandev.check.deadline', ''))):
        raise ValueError('validation slot belongs to an unknown workload')
    return labels


def reconcile_slot(docker, workspace):
    slot = docker.inspect(SLOT)
    if slot is None:
        return True
    labels = owned_slot(slot, workspace)
    state = slot.get('State', {}).get('Status')
    expired_created = state == 'created' and int(labels['kandev.check.deadline']) < time.time()
    if state not in ('exited', 'dead') and not expired_created:
        return False
    # No force: Docker rejects removal if the reserved container concurrently starts.
    docker.call('rm', slot['Id'])
    return True


def acquire_slot(docker, policy, cwd, kind, command, owner, env):
    deadline = time.monotonic() + policy.queue
    while True:
        if reconcile_slot(docker, WORKSPACE):
            ids = docker.call('ps', '-q').split()
            containers = [docker.inspect(identifier) for identifier in ids]
            if any(container is None for container in containers):
                raise ValueError('daemon workload inventory changed during validation admission')
            if memory_available(policy, containers):
                try:
                    return docker.call(*create_args(policy, cwd, kind, command, owner, env))
                except RuntimeError:
                    if docker.inspect(SLOT) is None:
                        raise
                    # A sibling won Docker's atomic name reservation; reconcile on retry.
        if time.monotonic() >= deadline:
            raise ValueError('validation admission deadline exceeded')
        time.sleep(min(0.25, max(0, deadline - time.monotonic())))


def remove_owned(docker, identifier, owner, stop=False):
    slot = docker.inspect(identifier)
    if slot is None:
        return
    labels = owned_slot(slot, WORKSPACE)
    if slot['Id'] != identifier or labels['kandev.check.owner'] != owner:
        raise ValueError('validation workload ownership changed')
    if stop:
        docker.call('stop', '--time', '5', identifier)
    docker.call('rm', identifier)


def run_job(policy, cwd, kind, command, env):
    docker = Docker()
    info = json.loads(docker.call('info', '--format', '{{json .}}'))
    validate_receipt(policy, json.loads(RECEIPT.read_text()), info)
    docker.call('image', 'inspect', policy.image)
    owner = str(uuid.uuid4())
    identifier = ''
    previous = {}

    def canceled(signum, _frame):
        raise KeyboardInterrupt

    for sig in (signal.SIGTERM, signal.SIGINT):
        previous[sig] = signal.signal(sig, canceled)
    try:
        identifier = acquire_slot(docker, policy, cwd, kind, command, owner, env)
        process = subprocess.Popen(['docker', 'start', '--attach', identifier])
        try:
            process.wait(timeout=policy.job + 20)
        except (KeyboardInterrupt, subprocess.TimeoutExpired) as interruption:
            remove_owned(docker, identifier, owner, stop=True)
            identifier = ''
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            return 124 if isinstance(interruption, subprocess.TimeoutExpired) else 130
        slot = docker.inspect(identifier)
        if slot is None:
            raise RuntimeError('validation result disappeared before verification')
        labels = owned_slot(slot, WORKSPACE)
        if labels['kandev.check.owner'] != owner:
            raise ValueError('validation result ownership changed')
        state = slot['State']
        if state.get('Running') or state.get('Status') not in ('exited', 'dead'):
            raise RuntimeError('Docker attach ended before validation completed')
        code = state['ExitCode']
        if state.get('OOMKilled'):
            print('worker-check: validation exceeded its memory limit', file=sys.stderr)
            code = code or 137
        remove_owned(docker, identifier, owner)
        identifier = ''
        return code
    finally:
        try:
            if identifier:
                remove_owned(docker, identifier, owner, stop=True)
        finally:
            for sig, handler in previous.items():
                signal.signal(sig, handler)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--preflight', action='store_true')
    parser.add_argument('--kind', choices=('lint', 'build', 'test', 'browser'))
    parser.add_argument('command', nargs=argparse.REMAINDER)
    options = parser.parse_args()
    command = options.command[1:] if options.command[:1] == ['--'] else options.command
    try:
        policy = Policy.from_env(os.environ)
        if options.preflight:
            docker = Docker()
            validate_receipt(policy, json.loads(RECEIPT.read_text()), json.loads(docker.call('info', '--format', '{{json .}}')))
            docker.call('image', 'inspect', policy.image)
            return 0
        cwd = workspace_cwd(Path.cwd())
        create_args(policy, cwd, options.kind, command, 'preflight', os.environ)
        return run_job(policy, cwd, options.kind, command, os.environ)
    except KeyboardInterrupt:
        return 130
    except (ValueError, RuntimeError, OSError, subprocess.SubprocessError) as error:
        print('worker-check: ' + str(error), file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())
