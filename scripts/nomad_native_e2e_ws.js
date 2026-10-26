// Minimal fixture-only WebSocket driver for the attached real-session proof.
// Uses Node's built-in WebSocket; it has no dependencies or credentials.
const input = JSON.parse(require('fs').readFileSync(0, 'utf8'));

function request(ws, action, payload, timeoutMs = 10000) {
  return new Promise((resolve, reject) => {
    const id = `wo04-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const timer = setTimeout(() => reject(new Error(`timeout:${action}`)), timeoutMs);
    const onMessage = async (event) => {
      let raw;
      try { raw = typeof event.data === 'string' ? event.data : await event.data.text(); }
      catch { return; }
      let message;
      try { message = JSON.parse(raw); } catch { return; }
      if (message.id !== id) return;
      clearTimeout(timer);
      ws.removeEventListener('message', onMessage);
      if (message.type === 'error') reject(new Error(`rejected:${action}`));
      else resolve(message.payload || {});
    };
    ws.addEventListener('message', onMessage);
    ws.send(JSON.stringify({ id, type: 'request', action, payload }));
  });
}

async function withApiSocket(fn) {
  const ws = new WebSocket(`${input.ws_base}/ws`);
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('ws-connect-timeout')), 5000);
    ws.addEventListener('open', () => { clearTimeout(timer); resolve(); }, { once: true });
    ws.addEventListener('error', () => { clearTimeout(timer); reject(new Error('ws-connect-failed')); }, { once: true });
  });
  try { return await fn(ws); } finally { ws.close(); }
}

async function terminalProbe(terminalId) {
  const url = `${input.ws_base}/terminal/environment/${encodeURIComponent(input.environment_id)}?mode=shell&terminalId=${encodeURIComponent(terminalId)}`;
  const ws = new WebSocket(url);
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('terminal-connect-timeout')), 8000);
    ws.addEventListener('open', () => { clearTimeout(timer); resolve(); }, { once: true });
    ws.addEventListener('error', () => { clearTimeout(timer); reject(new Error('terminal-connect-failed')); }, { once: true });
  });
  const marker = input.terminal_marker;
  try {
    const resize = JSON.stringify({ cols: 80, rows: 24 });
    ws.send(new Uint8Array([1, ...new TextEncoder().encode(resize)]));
    await new Promise(resolve => setTimeout(resolve, 250));
    ws.send(`printf '${marker}\\n'\r`);
    return await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('terminal-marker-timeout')), 10000);
      ws.addEventListener('message', async event => {
        const text = typeof event.data === 'string' ? event.data : await event.data.text();
        if (text.includes(marker)) { clearTimeout(timer); resolve(true); }
      });
      ws.addEventListener('error', () => { clearTimeout(timer); reject(new Error('terminal-stream-failed')); });
    });
  } finally { ws.close(); }
}

async function unknownTerminalRejected() {
  const url = `${input.ws_base}/terminal/environment/${encodeURIComponent(input.environment_id)}?mode=shell&terminalId=shell-00000000-0000-4000-8000-000000000001`;
  const ws = new WebSocket(url);
  return await new Promise((resolve, reject) => {
    const timer = setTimeout(() => { ws.close(); reject(new Error('wrong-environment-timeout')); }, 5000);
    ws.addEventListener('open', () => { clearTimeout(timer); ws.close(); resolve(false); }, { once: true });
    ws.addEventListener('error', () => { clearTimeout(timer); resolve(true); }, { once: true });
  });
}

// Read-only: lists the workspace tree and reads git snapshots. Sends no prompt.
async function workspaceRead() {
  return await withApiSocket(async ws => {
    const tree = await request(ws, 'workspace.tree.get', { session_id: input.session_id, path: '', depth: 4 });
    let repoHead = null;
    try {
      const git = await request(ws, 'session.git.snapshots', { task_id: input.task_id, session_id: input.session_id });
      const head = ((git.snapshots || []).find(item => typeof item.head_commit === 'string' && /^[0-9a-f]{40}$/.test(item.head_commit)) || {}).head_commit;
      if (head) repoHead = head;
    } catch { repoHead = null; }
    const contains = JSON.stringify(tree).includes(input.expected_file);
    const out = { ok: true, contains_expected_file: contains };
    if (repoHead) out.repo_head = repoHead;
    console.log(JSON.stringify(out));
    if (!contains) process.exit(2);
  });
}

// Production stop path: the same session.stop action the UI uses.
async function sessionStop() {
  return await withApiSocket(async ws => {
    await request(ws, 'session.stop', { session_id: input.session_id, reason: 'fixture cleanup' }, 90000);
    console.log(JSON.stringify({ ok: true }));
  });
}

async function main() {
  if (input.mode === 'workspace-read') return workspaceRead();
  if (input.mode === 'session-stop') return sessionStop();
  const result = await withApiSocket(async ws => {
    const prompt = await request(ws, 'message.add', {
      task_id: input.task_id,
      session_id: input.session_id,
      content: `For this inert transport check, output exactly ${input.result_marker}`,
    }, 15000);
    const tree = await request(ws, 'workspace.tree.get', {
      session_id: input.session_id, path: '', depth: 4,
    });
    const file = await request(ws, 'workspace.file.get', {
      session_id: input.session_id, path: input.repo_file, repo: input.repo_name,
    });
    const git = await request(ws, 'session.git.snapshots', {
      task_id: input.task_id, session_id: input.session_id,
    });
    const shell = await request(ws, 'user_shell.create', {
      task_id: input.task_id, task_environment_id: input.environment_id,
      label: 'WO04 disposable terminal probe',
    });
    return { promptAccepted: !!prompt, tree: tree, file: file, git: git,
      terminalId: shell.terminal_id };
  });
  if (!result.terminalId) throw new Error('terminal-create-missing-id');
  result.identityDenied = await unknownTerminalRejected();
  result.terminalMarkerObserved = await terminalProbe(result.terminalId);
  result.terminalDestroyed = await withApiSocket(async ws => {
    const response = await request(ws, 'user_shell.destroy', {
      task_id: input.task_id, task_environment_id: input.environment_id,
      terminal_id: result.terminalId,
    });
    return !!response;
  });
  // Print only pass/fail facts and Git identifiers, never arbitrary API bodies.
  const fileBody = JSON.stringify(result.file);
  const treeBody = JSON.stringify(result.tree);
  const gitSnapshot = (result.git.snapshots || []).find(item => item.branch === 'main');
  const gitHeadBaseMatch = !!gitSnapshot && gitSnapshot.head_commit === input.repo_head && gitSnapshot.base_commit === input.repo_head;
  console.log(JSON.stringify({
    promptAccepted: result.promptAccepted,
    treeContainsRepo: treeBody.includes(input.repo_name),
    fileContainsExpectedText: fileBody.includes(input.repo_file_marker),
    gitHeadBaseMatch,
    unknownRuntimeTerminalRejected: result.identityDenied,
    terminalMarkerObserved: result.terminalMarkerObserved,
    terminalMarker: input.terminal_marker,
    terminalDestroyed: result.terminalDestroyed,
    terminalId: result.terminalId,
  }));
  if (!result.promptAccepted || !treeBody.includes(input.repo_name) ||
      !fileBody.includes(input.repo_file_marker) || !gitHeadBaseMatch ||
      !result.identityDenied || !result.terminalMarkerObserved || !result.terminalDestroyed) process.exit(2);
}

main().catch(error => {
  console.error(`fixture scenario failed: ${String(error.message || error).replace(/[^a-zA-Z0-9:_-]/g, '')}`);
  process.exit(2);
});
