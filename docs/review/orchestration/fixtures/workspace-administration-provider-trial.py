#!/usr/bin/env python3
"""Synthetic Orchestrator trial. Usage: trial.py BUNDLE/bin/kandev CLAUDE_CONFIG_DIR.

Uses the selected native Claude login; never reads or copies credential contents.
Kandev data and example repositories are disposable. Do not publish raw logs.
"""
import hashlib
import json
import os
from pathlib import Path
import signal
import sys
import socket
import sqlite3
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid

REPO = Path(__file__).resolve().parents[4]
ROOT = Path(tempfile.mkdtemp(prefix='workspace-administration-provider-'))
ROOT.chmod(0o700)
BIN = Path(sys.argv[1]).resolve()
CLAUDE_CONFIG = Path(sys.argv[2]).expanduser().resolve()
for name in ('application', 'workspace'):
    (ROOT / name).mkdir(mode=0o700)
subprocess.run(['git', 'init', str(ROOT / 'workspace')], check=True, capture_output=True)
# Provider authentication stays with the explicitly selected native profile.
if not (CLAUDE_CONFIG / '.credentials.json').is_file():
    raise SystemExit('Native Claude authentication is unavailable; trial not launched.')
with socket.socket() as listener:
    listener.bind(('127.0.0.1', 0))
    port = listener.getsockname()[1]
BASE = f'http://127.0.0.1:{port}'
settings_interlock = ''
environment = {key: os.environ[key] for key in ('HOME', 'USER', 'LOGNAME', 'SHELL', 'LANG', 'PATH') if key in os.environ}
environment.update({
    'PATH': str(BIN.parent) + os.pathsep + environment['PATH'],
    'KANDEV_HOME_DIR': str(ROOT / 'application'),
    'KANDEV_DATABASE_PATH': str(ROOT / 'fixture.db'),
    'KANDEV_SERVER_HOST': '127.0.0.1',
    'KANDEV_SERVER_PORT': str(port),
    'KANDEV_WEB_DIST_DIR': str(REPO / 'apps/web/dist'),
    'KANDEV_FEATURES_ORCHESTRATION': 'true',
    'KANDEV_BUNDLE_DIR': str(BIN.parent.parent),
    'KANDEV_FEATURES_OFFICE': 'false',
    'KANDEV_DOCKER_ENABLED': 'false',
    'KANDEV_E2E_MOCK': 'false',
    'KANDEV_MOCK_AGENT': 'false',
    'KANDEV_LOG_LEVEL': 'warn',
    'KANDEV_WORKTREE_BASEPATH': str(ROOT / 'worktrees'),
    'KANDEV_REPOCLONE_BASEPATH': str(ROOT / 'clones'),
    'AGENTCTL_INSTANCE_PORT_BASE': '61001',
    'AGENTCTL_INSTANCE_PORT_MAX': '61099',
    'AGENTCTL_AUTO_APPROVE_PERMISSIONS': 'false',
    'CLAUDE_CONFIG_DIR': str(CLAUDE_CONFIG),
    'CLAUDE_CODE_DISABLE_AUTO_MEMORY': '1',
    'CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC': '1',
})

def request(method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method,
                                 headers={'Content-Type': 'application/json',
                                          'X-Kandev-Interim-Settings-Interlock': settings_interlock})
    try:
        with urllib.request.urlopen(req, timeout=20) as response:
            raw = response.read()
            return json.loads(raw) if raw else None
    except urllib.error.HTTPError as error:
        # This backend is freshly generated and contains synthetic input only.
        raise RuntimeError(f'{method} {path}: {error.code} {error.read().decode()[:1200]}') from None

def wait_for(predicate, seconds, label):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError('Fixture backend exited')
        result = predicate()
        if result:
            return result
        time.sleep(0.5)  # bounded polling of an external readiness/state boundary
    raise TimeoutError(label)

def health():
    try:
        state = request('GET', '/api/v1/app-state?path=%2Fsettings%2Fagents')
        return bool(state.get('interimSettingsInterlockToken'))
    except (RuntimeError, OSError):
        return False

def snapshot(conversation):
    with sqlite3.connect(ROOT / 'fixture.db') as db:
        rows = {}
        for table in ('tasks', 'repositories', 'workflows', 'workflow_steps'):
            query = f'SELECT * FROM "{table}"'
            args = ()
            if table == 'tasks':
                query += ' WHERE id<>?'
                args = (conversation,)
            content = sorted(db.execute(query, args).fetchall(), key=repr)
            rows[table] = {'count': len(content), 'sha256': hashlib.sha256(repr(content).encode()).hexdigest()}
        return rows

receipt = {'outcome': 'in_progress', 'synthetic_only': True,
           'binary_sha256': hashlib.sha256(BIN.read_bytes()).hexdigest(), 'turns': []}
log = (ROOT / 'backend.log').open('wb')
process = subprocess.Popen([str(BIN), '__backend'], cwd=ROOT / 'workspace', env=environment,
                           stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
(ROOT / 'process.json').write_text(json.dumps({'pid': process.pid, 'port': port}))
print(json.dumps({'fixture': str(ROOT), 'pid': process.pid, 'status': 'starting'}), flush=True)
try:
    wait_for(health, 40, 'Backend readiness')
    settings_interlock = request('GET', '/api/v1/app-state?path=%2Fsettings%2Fagents')['interimSettingsInterlockToken']
    workspace = request('POST', '/api/v1/workspaces', {'name': 'Example provider trial'})
    def claude_provider():
        rows = request('GET', '/api/v1/agents')['agents']
        return next((row for row in rows if row['name'] == 'claude-acp'), None)
    provider = wait_for(claude_provider, 30, 'Claude provider discovery')
    profile = request('POST', f'/api/v1/agents/{provider["id"]}/profiles', {
        'name': 'Example workspace Orchestrator', 'model': 'sonnet', 'mode': 'default',
        'auto_approve': False, 'auto_fallback': False, 'cli_passthrough': False,
        'cli_flags': [], 'config_options': {}, 'env_vars': [],
    })
    profile = profile.get('profile', profile)
    executors = request('GET', '/api/v1/executors')['executors']
    local = next(row for row in executors if row['type'] == 'local')
    preset = local['profiles'][0]
    base = '/api/v1/orchestration'
    role = request('POST', base + '/roles', {
        'name': 'Example assistant',
        'instructions': 'Help organize generic example tasks. Report observed results accurately.',
    })
    coordinator = request('POST', f'{base}/workspaces/{workspace["id"]}/orchestrators', {
        'role_id': role['id'], 'profile_id': profile['id'],
        'executor_preference': json.dumps({'executor_profile_id': preset['id']}),
        'context': 'This disposable workspace contains generic examples only.',
    })
    conversation = request('POST', f'{base}/workspaces/{workspace["id"]}/orchestrators/{coordinator["id"]}/conversation', {})
    task_id = conversation['task_id']
    workflow = request('POST', '/api/v1/workflows', {'workspace_id':workspace['id'], 'name':'Example delivery'})
    steps = []
    for i, name in enumerate(['Backlog', 'In progress', 'Done']):
        steps.append(request('POST', '/api/v1/workflow/steps', {'workflow_id':workflow['id'], 'name':name, 'position':i, 'color':'#666666', 'allow_manual_move':True, 'is_start_step':i==0}))
    comments_path = f'{base}/tasks/{task_id}/comments'
    prompts = [
        f'Use workspace and your management tools to update this workspace description to Generic administration trial. Create a workflow named Example managed delivery with description Generic workflow. Add two manually movable columns named Ideas and Doing, with Ideas as the start column and Doing at position 1. Rename Doing to In progress and set its wip_limit to 2. Register the existing local Git checkout at {ROOT / "workspace"} as a repository named Example registered repository with default_branch main. Do not create a worker or start execution. Read workspace to verify the saved settings. These changes to this disposable synthetic workspace are authorized; do not ask for confirmation.',
        'In Example managed delivery, reorder the two columns so In progress comes before Ideas, and read workspace to verify that order. Then delete both empty columns, delete Example managed delivery and remove the Example registered repository registration. Set this workspace description to Generic administration complete. Verify the final state. These cleanup actions on the disposable synthetic fixture are authorized; do not ask for confirmation.',
    ]
    delivery_id = None
    for index, prompt in enumerate(prompts):
        before = request('GET', comments_path).get('comments', [])
        ids = {row['id'] for row in before}
        accepted = request('POST', comments_path, {'body':prompt, 'client_message_id':str(uuid.uuid4())})
        def reply():
            rows = request('GET', comments_path).get('comments', [])
            failures = [row.get('run_error') for row in rows if row['id']==accepted['id'] and row.get('run_status')=='failed']
            if failures: raise RuntimeError('Provider run failed: '+str(failures[0]))
            return [row for row in rows if row['id'] not in ids and row.get('source')=='session']
        replies = wait_for(reply, 360, 'Provider workspace administration reply')
        receipt['turns'].append({'number':index+1, 'reply_received':bool(replies)})
        with sqlite3.connect(ROOT / 'fixture.db') as db:
            if index == 0:
                row = db.execute("SELECT id,description FROM workflows WHERE name='Example managed delivery' AND workspace_id=?", (workspace['id'],)).fetchone()
                assert row and row[1] == 'Generic workflow', 'Workflow creation not confirmed'
                workflow_id = row[0]
                columns = db.execute('SELECT name,position,wip_limit,is_start_step FROM workflow_steps WHERE workflow_id=? ORDER BY position', (workflow_id,)).fetchall()
                assert len(columns) == 2 and columns[0][0] == 'Ideas' and columns[0][3], 'Start column not confirmed'
                assert columns[1][0] == 'In progress' and columns[1][2] == 2, 'Column edit not confirmed'
                repo = db.execute("SELECT id,local_path,default_branch FROM repositories WHERE workspace_id=? AND name='Example registered repository'", (workspace['id'],)).fetchone()
                assert repo and repo[1:] == (str(ROOT/'workspace'), 'main'), 'Repository registration not confirmed'
                repository_id = repo[0]
                assert db.execute('SELECT description FROM workspaces WHERE id=?', (workspace['id'],)).fetchone()[0] == 'Generic administration trial'
                receipt['turns'][-1]['verified'] = ['workspace_update', 'workflow_create', 'step_create', 'step_update', 'repository_register']
            else:
                assert db.execute('SELECT count(*) FROM workflows WHERE id=?', (workflow_id,)).fetchone()[0] == 0
                assert db.execute('SELECT count(*) FROM workflow_steps WHERE workflow_id=?', (workflow_id,)).fetchone()[0] == 0
                assert db.execute('SELECT count(*) FROM repositories WHERE id=? AND deleted_at IS NULL', (repository_id,)).fetchone()[0] == 0
                assert db.execute('SELECT description FROM workspaces WHERE id=?', (workspace['id'],)).fetchone()[0] == 'Generic administration complete'
                receipt['turns'][-1]['verified'] = ['workflow_delete', 'step_delete', 'repository_remove', 'workspace_update']
        print(json.dumps({'status':'native_mutation_verified','turn':index+1}),flush=True)
    receipt['outcome']='passed'
except Exception as error:
    receipt['outcome']='failed'
    receipt['error']=str(error)
    print(json.dumps({'status':'failed','error':str(error)}),flush=True)
finally:
    if process.poll() is None:
        os.killpg(process.pid,signal.SIGINT)
        try: process.wait(timeout=20)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid,signal.SIGKILL)
            process.wait(timeout=5)
    log.close()
    receipt['teardown']={'backend_exit':process.returncode,'provider_credentials_copied':False}
    (ROOT/'receipt.json').write_text(json.dumps(receipt,indent=2))
    print(json.dumps({'outcome':receipt['outcome'],'receipt':str(ROOT/'receipt.json')}),flush=True)
raise SystemExit(0 if receipt['outcome']=='passed' else 1)
