// Run inside a disposable executor with Node and the rebuilt agentctl installed.
// Synthetic credentials only; all CLI output stays in memory.
import assert from 'node:assert/strict';
import { spawn, execFile } from 'node:child_process';
import { mkdtempSync, mkdirSync, rmSync, symlinkSync } from 'node:fs';
import http from 'node:http';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { promisify } from 'node:util';

const version = process.argv[2];
const scenario = process.argv[3] || 'lost-ack';
assert.ok(['lost-ack', 'interrupted-import'].includes(scenario));
assert.match(version ?? '', /^2\.\d+\.\d+$/);
const root = mkdtempSync(path.join(os.tmpdir(), 'kandev-credential-fault-'));
const home = path.join(root, 'home');
mkdirSync(home);
const env = { ...process.env, HOME: home, XDG_DATA_HOME: home + '/data',
  XDG_CONFIG_HOME: home + '/config', XDG_STATE_HOME: home + '/state',
  KANDEV_HOME_DIR: root + '/kandev', AGENTCTL_LISTEN_HOST: '127.0.0.1' };
delete env.OPENCODE_DB;
delete env.OPENCODE_CONFIG;
delete env.OPENCODE_CONFIG_CONTENT;
// This is a new loopback-only worker, not the executor's authenticated worker.
// Never inherit its bootstrap handshake or forward its control credentials.
delete env.AGENTCTL_BOOTSTRAP_NONCE;
delete env.AGENTCTL_AUTH_TOKEN;
delete env.KANDEV_PARENT_PIPE_FD;
const cli = ['--yes', '--prefer-offline', '@opencode/cli@' + version];
// Reuse the disposable executor's package cache, not its credential HOME.
env.npm_config_cache = process.env.npm_config_cache || path.join(os.homedir(), '.npm');
symlinkSync(env.npm_config_cache, path.join(home, '.npm'));
const reserve = net.createServer();
await new Promise(resolve => reserve.listen(0, '127.0.0.1', resolve));
const port = reserve.address().port;
await new Promise(resolve => reserve.close(resolve));
let worker;
function startWorker() {
  worker = spawn(process.env.BOOTSTRAP_AGENTCTL || '/usr/local/bin/agentctl',
    ['-port', String(port), '-workdir', root], { env, stdio: 'ignore', detached: true });
}
async function stopWorker(signal = 'SIGTERM') {
  if (!worker || worker.exitCode !== null || worker.signalCode !== null) return;
  const exited = new Promise(resolve => worker.once('exit', resolve));
  if (signal === 'SIGKILL') {
    // Managed CLI children have their own process groups. Kill the actual
    // importer as well as the worker; killing only the worker group lets a
    // detached import keep committing while the test claims interruption.
    const { stdout } = await promisify(execFile)('ps', ['-eo', 'pid=,ppid=']);
    const rows = stdout.trim().split('\n').map(line => line.trim().split(/\s+/).map(Number));
    const owned = new Set([worker.pid]);
    for (let size = -1; size !== owned.size;) {
      size = owned.size;
      for (const [pid, parent] of rows) if (owned.has(parent)) owned.add(pid);
    }
    for (const pid of [...owned].reverse()) {
      if (pid === worker.pid) continue;
      try { process.kill(pid, 'SIGKILL'); } catch (error) { if (error.code !== 'ESRCH') throw error; }
    }
  }
  process.kill(-worker.pid, signal);
  await Promise.race([exited, delay(10000)]);
  if (worker.exitCode === null && worker.signalCode === null) {
    process.kill(-worker.pid, 'SIGKILL'); await exited;
  }
}
async function exported() {
  const output = await promisify(execFile)('npx', [...cli, 'auth', 'export', '--standalone'],
    { env, timeout: 120000, maxBuffer: 1 << 20 });
  return JSON.parse(output.stdout);
}
let proxy;
let phase = 'worker startup';
async function request(base, route, body) {
  const response = await fetch(base + route, { method: body === undefined ? 'GET' : 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(120000) });
  assert.equal(response.status, 200, 'Worker operation failed');
  return response.json();
}
async function configure() {
  const control = 'http://127.0.0.1:' + port;
  let ready = false;
  for (let attempt = 0; attempt < 150; attempt++) {
    if (worker.exitCode !== null) throw new Error('Worker exited before readiness');
    try { await request(control, '/identity'); ready = true; break; } catch { await delay(200); }
  }
  assert.ok(ready, 'Worker readiness deadline exceeded');
  const response = await fetch(control + '/api/v1/instances', { method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ workspace_path: root, protocol: 'acp', auto_start: false }),
    signal: AbortSignal.timeout(30000) });
  assert.equal(response.status, 201, 'Could not create isolated worker instance');
  const instance = await response.json();
  const base = 'http://127.0.0.1:' + instance.port;
  await request(base, '/api/v1/agent/configure', { command: 'npx',
    agent_args: ['npx', ...cli, 'acp'], env, replace_env: true,
    opencode_runtime: { family: 'v2', source: 'managed', version } });
  return base;
}
try {
  startWorker();
  let base = await configure();
  phase = 'initial import';
  if (scenario === 'interrupted-import') {
    const entries = Array.from({ length: 4000 }, (_, index) => ({
      id: 'partial-' + index, integrationID: 'openai', active: index === 0,
      label: 'synthetic test ' + index, value: { type: 'key', key: 'synthetic-' + index },
    }));
    assert.ok(Buffer.byteLength(JSON.stringify(entries)) < (1 << 20));
    let completed = false;
    const importing = request(base, '/api/v1/agent/opencode/credentials', entries)
      .then(() => { completed = true; }, () => {});
    let partial;
    phase = 'observe partial import';
    const deadline = Date.now() + 90000;
    while (Date.now() < deadline && !completed) {
      // The independent observer can race initial database creation or a write
      // lock. Retry inspection within the deadline; only a successful public
      // export showing a strictly partial store qualifies as interruption.
      let observed;
      try { observed = await exported(); } catch { await delay(100); continue; }
      if (observed.length > 0 && observed.length < entries.length) {
        partial = observed; await stopWorker('SIGKILL'); break;
      }
      await delay(50);
    }
    assert.ok(partial, 'No actual partial import observed; scenario not passed');
    await importing;
    phase = 'verify interruption';
    const stopped = await exported();
    assert.ok(stopped.length > 0 && stopped.length < entries.length,
      'Import completed before interruption; scenario not passed');
    startWorker();
    phase = 'restart worker';
    base = await configure();
    phase = 'retry pending import';
    assert.equal((await request(base, '/api/v1/agent/opencode/credentials')).state, 'pending');
    phase = 'merge retry request';
    await request(base, '/api/v1/agent/opencode/credentials', entries);
    phase = 'inspect completed retry';
    assert.equal((await request(base, '/api/v1/agent/opencode/credentials')).state, 'present');
    const final = await exported();
    phase = 'compare recovered accounts';
    assert.equal(final.length, entries.length);
    for (const entry of final) assert.deepEqual(entry, entries.find(item => item.id === entry.id));
    for (const entry of stopped) assert.deepEqual(final.find(item => item.id === entry.id), entry);
  } else {
  const entry = { id: 'lost-ack-synthetic', integrationID: 'openai', active: true,
    label: 'synthetic test', value: { type: 'key', key: 'synthetic-only' } };
  let committed = false;
  proxy = http.createServer((incoming, outgoing) => {
    const upstream = http.request(base + '/api/v1/agent/opencode/credentials', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, timeout: 120000,
    }, result => {
      let body = '';
      result.on('data', chunk => { body += chunk; });
      result.on('end', () => {
        committed = result.statusCode === 200 && JSON.parse(body).success === true;
        outgoing.destroy(); // Drop only after the complete success acknowledgement.
      });
    });
    upstream.on('error', () => outgoing.destroy());
    upstream.on('timeout', () => upstream.destroy());
    incoming.pipe(upstream);
  });
  await new Promise(resolve => proxy.listen(0, '127.0.0.1', resolve));
  await assert.rejects(request('http://127.0.0.1:' + proxy.address().port, '/', [entry]));
  assert.ok(committed, 'Import did not commit before acknowledgement loss');
  assert.equal((await request(base, '/api/v1/agent/opencode/credentials')).state, 'present');
  await request(base, '/api/v1/agent/opencode/credentials',
    [{ ...entry, active: false, value: { type: 'key', key: 'stale-retry-value' } }]);
  assert.deepEqual((await exported()).find(item => item.id === entry.id), entry);
  }
  console.log(JSON.stringify({ scenario, version, passed: true,
    scope: 'real worker HTTP and CLI; controller smoke is separate' }));
} catch (error) {
  console.error('FAILED: credential fault assertion at ' + phase + '; diagnostics withheld');
  if (typeof error.actual === 'number' || ['pending', 'present', 'empty'].includes(error.actual)) {
    console.error('ASSERTION_ACTUAL: ' + error.actual);
  }
  process.exitCode = 1;
} finally {
  proxy?.closeAllConnections();
  if (proxy) await new Promise(resolve => proxy.close(resolve));
  await stopWorker();
  rmSync(root, { recursive: true, force: true });
}
