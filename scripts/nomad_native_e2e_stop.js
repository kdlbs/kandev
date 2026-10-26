// Fixture-only normal session-stop driver. Sends no prompts or credentials.
const input = JSON.parse(require('fs').readFileSync(0, 'utf8'));

function request(ws, action, payload, timeoutMs = 10000) {
  return new Promise((resolve, reject) => {
    const id = `wo04-stop-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const timer = setTimeout(() => reject(new Error('stop-request-timeout')), timeoutMs);
    const onMessage = async event => {
      let raw;
      try { raw = typeof event.data === 'string' ? event.data : await event.data.text(); }
      catch { return; }
      let message;
      try { message = JSON.parse(raw); } catch { return; }
      if (message.id !== id) return;
      clearTimeout(timer);
      ws.removeEventListener('message', onMessage);
      if (message.type === 'error') reject(new Error('session-stop-rejected'));
      else resolve(message.payload || {});
    };
    ws.addEventListener('message', onMessage);
    ws.send(JSON.stringify({ id, type: 'request', action, payload }));
  });
}

async function main() {
  if (typeof input.ws_base !== 'string' || !input.ws_base.startsWith('ws://127.0.0.1:') ||
      typeof input.session_id !== 'string' || !/^[0-9a-f-]{36}$/i.test(input.session_id)) {
    throw new Error('fixture-stop-input-invalid');
  }
  const ws = new WebSocket(`${input.ws_base}/ws`);
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('ws-connect-timeout')), 5000);
    ws.addEventListener('open', () => { clearTimeout(timer); resolve(); }, { once: true });
    ws.addEventListener('error', () => { clearTimeout(timer); reject(new Error('ws-connect-failed')); }, { once: true });
  });
  try {
    const result = await request(ws, 'session.stop', { session_id: input.session_id });
    if (result.success !== true) throw new Error('session-stop-not-accepted');
    console.log(JSON.stringify({ normalStopAccepted: true, sessionId: input.session_id }));
  } finally {
    ws.close();
  }
}

main().catch(error => {
  console.error(String(error.message || error).replace(/[^a-zA-Z0-9:_-]/g, ''));
  process.exit(2);
});
