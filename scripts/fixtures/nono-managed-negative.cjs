// This program runs inside the actual agent sandbox. It emits no endpoint data.
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const net = require('node:net');
const permission = (error) => ['EACCES', 'EPERM'].includes(error.code);
async function denied(operation) {
  try { await operation(); return false; } catch (error) { return permission(error); }
}
async function directDenied(address) {
  const url = new URL(address);
  return new Promise((resolve) => {
    const connection = net.connect({ host: url.hostname, port: url.port });
    connection.setTimeout(1500);
    connection.once('connect', () => { connection.destroy(); resolve(false); });
    connection.once('error', (error) => resolve(permission(error)));
    connection.once('timeout', () => { connection.destroy(); resolve(false); });
  });
}
(async () => {
  const fixture = JSON.parse(await fs.readFile(process.argv[2], 'utf8'));
  const raw = process.env.PI_CODING_AGENT_DIR || path.join(os.homedir(), '.pi/agent');
  const directory = raw.startsWith('~/') ? path.join(os.homedir(), raw.slice(2)) : raw;
  const endpoint = JSON.parse(await fs.readFile(path.join(directory, 'mcp.json'), 'utf8')).mcpServers.kandev.url;
  const results = {};
  await fs.writeFile(fixture.control, 'permitted');
  results.permitted_rw = await fs.readFile(fixture.control, 'utf8') === 'permitted';
  results.outside_read_denied = await denied(() => fs.readFile(fixture.outside));
  results.outside_write_denied = await denied(() => fs.writeFile(fixture.outside, 'FORBIDDEN'));
  const initialized = await fetch(endpoint, { method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json, text/event-stream' },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: {
      protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'owned-control', version: '1' },
    } }), signal: AbortSignal.timeout(10000) });
  results.mcp_post_ok = initialized.status === 200 && !!(await initialized.json()).result;
  const wrong = new URL(endpoint); wrong.pathname = '/api/v1/settings';
  const response = await fetch(wrong, { signal: AbortSignal.timeout(10000) });
  results.mcp_api_denied = [403, 404, 405].includes(response.status);
  await response.arrayBuffer();
  results.direct_destination_denied = await directDenied(fixture.destination);
  results.direct_relay_denied = await directDenied(endpoint);
  try {
    const response = await fetch(fixture.destination + '/forbidden', { signal: AbortSignal.timeout(10000) });
    results.unapproved_proxy_denied = [403, 502].includes(response.status);
    await response.arrayBuffer();
  } catch (error) { results.unapproved_proxy_denied = error.name === 'TypeError'; }
  await fs.writeFile(fixture.receipt, JSON.stringify(results));
})().catch(() => { process.exitCode = 1; });
