// Fixture-only WebSocket probe for a recovered native Nomad session.
const input = JSON.parse(require('fs').readFileSync(0, 'utf8'));
const ws = new WebSocket(`${input.ws_base}/ws`);
const id = `wo05-recovery-${Date.now()}-${Math.random().toString(16).slice(2)}`;
const timer = setTimeout(() => { ws.close(); process.exit(2); }, 15000);
ws.addEventListener('open', () => {
  ws.send(JSON.stringify({
    id,
    type: 'request',
    action: 'message.add',
    payload: { task_id: input.task_id, session_id: input.session_id, content: input.content },
    timestamp: new Date().toISOString(),
  }));
});
ws.addEventListener('message', async event => {
  let raw;
  try { raw = typeof event.data === 'string' ? event.data : await event.data.text(); }
  catch { return; }
  let message;
  try { message = JSON.parse(raw); }
  catch { return; }
  if (message.id !== id) return;
  clearTimeout(timer);
  ws.close();
  if (message.type === 'error') {
    console.error('recovery-prompt-rejected');
    process.exit(2);
    return;
  }
  const payload = message.payload || {};
  const item = payload.message || payload;
  if (typeof item.id !== 'string' || !item.id) {
    console.error('recovery-prompt-message-id-missing');
    process.exit(2);
    return;
  }
  console.log(JSON.stringify({ accepted: true, request_id: id, message_id: item.id }));
});
ws.addEventListener('error', () => {
  clearTimeout(timer);
  console.error('recovery-prompt-websocket-error');
  process.exit(2);
});
