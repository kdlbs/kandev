import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { createServer } from "node:http";
import { mkdir, mkdtemp, readFile, writeFile, access } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve, dirname } from "node:path";
import { createInterface } from "node:readline";
import { once } from "node:events";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = resolve(packageRoot, "../../..");
const artifactDir = join(repoRoot, "apps/backend/internal/agentctl/server/opencodeplugin/assets");
const CLI_PACKAGE = "@opencode/cli@2.2.0";
const CLI_VERSION = "2.2.0";

const optIn = process.env.KANDEV_OPENCODE_CONFORMANCE === "1";

test(
  "packaged OpenCode 2.2.0 loads the shipped plugin and delivers a shell outcome",
  { skip: !optIn },
  async (t) => {
    const cli = await resolveCLI();
    await assertPinnedCLI(cli);
    const manifest = JSON.parse(await readFile(join(artifactDir, "artifact.json"), "utf8"));
    const artifact = await readFile(join(artifactDir, "index.mjs"));
    assert.equal(sha256(artifact), manifest.sha256, "the shipped artifact matches its manifest");

    const root = await mkdtemp(join(tmpdir(), "kandev-opencode-conformance-"));
    const home = join(root, "home");
    const workspace = join(root, "workspace");
    const configPath = join(home, ".config", "opencode", "opencode.json");
    const markerPath = join(root, "user-plugin-marker.json");
    const userPluginDir = join(root, "user-plugin");
    await mkdir(workspace, { recursive: true });
    await mkdir(join(home, ".config", "opencode"), { recursive: true });
    await mkdir(userPluginDir, { recursive: true });

    await writeFile(
      join(userPluginDir, "package.json"),
      JSON.stringify({
        name: "kandev-conformance-user-plugin",
        version: "1.0.0",
        type: "module",
        exports: { ".": "./index.mjs" },
      }),
    );
    await writeFile(
      join(userPluginDir, "index.mjs"),
      `
import { appendFileSync } from "node:fs";
export default {
  id: "kandev-conformance-user-plugin",
  setup(ctx) {
    appendFileSync(${JSON.stringify(markerPath)}, JSON.stringify({ strict: ctx.options.strict === true }) + "\\n");
    return () => {};
  },
};
`,
    );

    const model = await startModelStub();
    t.after(() => model.close());
    await writeFile(
      configPath,
      JSON.stringify(
        {
          $schema: "https://opencode.ai/config.json",
          model: "openai/gpt-4o-mini",
          autoupdate: false,
          username: "{env:KANDEV_CONFORMANCE_USERNAME}",
          permissions: [{ action: "shell", resource: "*", effect: "allow" }],
          providers: {
            openai: {
              package: "@opencode/ai/providers/openai/chat",
              settings: { baseURL: `${model.endpoint}/v1` },
              models: {
                "gpt-4o-mini": {
                  name: "Conformance fixture",
                  limit: { context: 200000, output: 32000 },
                },
              },
            },
          },
          plugins: [{ package: userPluginDir, options: { strict: true } }],
        },
        null,
        2,
      ),
    );

    const bridge = await startBridge();
    t.after(() => bridge.close());
    const bootstrapPath = join(root, "bootstrap.json");
    await writeFile(
      bootstrapPath,
      JSON.stringify({
        protocolVersion: 1,
        endpoint: bridge.endpoint,
        credential: bridge.credential,
        leaseId: bridge.leaseId,
        artifactHash: manifest.sha256,
        behaviors: [{ id: "shell-outcomes", version: 1 }],
      }),
      { mode: 0o600 },
    );

    const env = isolatedEnv(home, root, configPath, {
      KANDEV_CONFORMANCE_USERNAME: "alice",
      OPENCODE_CONFIG_CONTENT: JSON.stringify({
        plugins: [{ package: artifactDir, options: { bootstrapFile: bootstrapPath } }],
      }),
    });

    const substituted = await debugConfig(
      cli,
      workspace,
      isolatedEnv(home, root, configPath, {
        KANDEV_CONFORMANCE_USERNAME: "alice",
      }),
    );
    assert.match(
      substituted,
      /"username": "alice"/,
      "environment substitution survives launch composition",
    );
    assert.match(substituted, /user-plugin/, "the file-configured user plugin survives");

    const child = spawn(cli, ["acp"], {
      cwd: workspace,
      env,
      stdio: ["pipe", "pipe", "pipe"],
      detached: process.platform !== "win32",
    });
    t.after(() => stopChild(child, client));
    const stderr = [];
    child.stderr.on("data", (chunk) => stderr.push(chunk.toString()));
    const client = new ACPClient(child, t);
    const sessionID = await client.start(workspace);
    let claimed = false;
    const claimWatcher = setInterval(() => {
      if (claimed || !client.lastShellID) return;
      claimed = true;
      bridge.setClaim({
        shellId: client.lastShellID,
        nativeSessionId: sessionID,
        nativeGeneration: 1,
        workId: client.lastShellID,
        runId: client.lastShellID,
        sourceCallId: "call-conformance-shell",
        title: "sleep 1; exit 3",
        promptGeneration: 1,
        submissionId: "submission-conformance",
      });
    }, 20);
    t.after(() => clearInterval(claimWatcher));
    const prompt = client.prompt(sessionID, "Run the packaged conformance command.");
    await waitUntil(() => bridge.state.hellos.length > 0, "plugin hello");
    const hello = bridge.state.hellos[0];
    assert.equal(hello.artifactHash, manifest.sha256);
    assert.equal(hello.runtime.version, CLI_VERSION);
    assert.deepEqual(hello.behaviors, [{ id: "shell-outcomes", version: 1 }]);
    assert.equal(bridge.state.claims.length, 0, "a foreground shell is never claimed");
    await waitUntil(() => bridge.state.claims.length === 1, "ACP launch claim");
    const claim = bridge.state.claims[0];
    await prompt;
    await waitUntil(() => bridge.state.events.length >= 1, "shell terminal observation");
    const observation = bridge.state.events[0];
    assert.equal(observation.behaviorId, "shell-outcomes");
    assert.equal(observation.payload.kind, "shell.terminal");
    assert.equal(observation.payload.shellId, claim.shellId);
    assert.equal(observation.payload.status, "exited");
    assert.equal(observation.payload.exitCode, 3);
    assert.equal(bridge.state.events.length, 1, "a retried claim must not duplicate the outcome");
    await waitForFile(markerPath);
    const marker = JSON.parse((await readFile(markerPath, "utf8")).trim());
    assert.deepEqual(marker, { strict: true }, "user plugin options retain their meaning");
    assert.equal(
      stderr.join("").includes("integration_unavailable"),
      false,
      "plugin setup succeeded",
    );
  },
);

class ACPClient {
  constructor(child, t) {
    this.child = child;
    this.t = t;
    this.nextID = 1;
    this.pending = new Map();
    this.sessionID = "";
    this.toolCallIDs = new Set();
    this.lastShellID = "";
    this.readline = createInterface({ input: child.stdout });
    this.readline.on("line", (line) => this.handleLine(line));
  }

  async start(workspace) {
    const init = await this.request("initialize", {
      protocolVersion: 1,
      clientCapabilities: { fs: { readTextFile: false, writeTextFile: false }, terminal: false },
      clientInfo: { name: "kandev-conformance", version: "1.0.0" },
    });
    assert.equal(init.agentInfo?.version, CLI_VERSION);
    const session = await this.request("session/new", { cwd: workspace, mcpServers: [] });
    this.sessionID = session.sessionId;
    return this.sessionID;
  }

  async prompt(sessionID, text) {
    return this.request("session/prompt", {
      sessionId: sessionID,
      prompt: [{ type: "text", text }],
    });
  }

  handleLine(line) {
    let message;
    try {
      message = JSON.parse(line);
    } catch {
      return;
    }
    if (message.id !== undefined && (message.result !== undefined || message.error !== undefined)) {
      const pending = this.pending.get(message.id);
      if (pending) {
        this.pending.delete(message.id);
        message.error
          ? pending.reject(new Error(JSON.stringify(message.error)))
          : pending.resolve(message.result);
      }
      return;
    }
    if (message.id !== undefined && message.method) {
      if (message.method === "session/request_permission") {
        const option =
          message.params.options.find((item) => item.kind === "allow_once") ??
          message.params.options[0];
        this.send({
          jsonrpc: "2.0",
          id: message.id,
          result: { outcome: { outcome: "selected", optionId: option.optionId } },
        });
      } else {
        this.send({
          jsonrpc: "2.0",
          id: message.id,
          error: { code: -32601, message: "unsupported" },
        });
      }
      return;
    }
    if (message.method === "session/update") {
      const update = message.params?.update ?? {};
      if (update.sessionUpdate === "tool_call") this.toolCallIDs.add(update.toolCallId);
      if (update.sessionUpdate === "tool_call_update" && update.toolCallId) {
        const raw = JSON.stringify(update);
        const match = raw.match(/shell ID: (sh_[A-Za-z0-9]+)/) || raw.match(/"shellID":"([^"]+)"/);
        if (match) this.lastShellID = match[1];
      }
    }
  }

  send(message) {
    this.child.stdin.write(`${JSON.stringify(message)}\n`);
  }

  request(method, params) {
    const id = this.nextID++;
    this.send({ jsonrpc: "2.0", id, method, params });
    return new Promise((resolvePromise, rejectPromise) => {
      const timer = setTimeout(
        () => rejectPromise(new Error(`ACP request timed out: ${method}`)),
        60_000,
      );
      this.pending.set(id, {
        resolve: (value) => {
          clearTimeout(timer);
          resolvePromise(value);
        },
        reject: (error) => {
          clearTimeout(timer);
          rejectPromise(error);
        },
      });
    });
  }
}

async function resolveCLI() {
  if (process.env.KANDEV_OPENCODE_CONFORMANCE_CLI)
    return process.env.KANDEV_OPENCODE_CONFORMANCE_CLI;
  const root = await mkdtemp(join(tmpdir(), "kandev-opencode-cli-"));
  await execFileAsync(
    "npm",
    ["install", "--prefix", root, "--no-audit", "--no-fund", CLI_PACKAGE],
    {
      timeout: 240_000,
      env: { ...process.env, npm_config_cache: join(root, ".npm-cache") },
    },
  );
  return join(root, "node_modules", ".bin", "opencode");
}

async function assertPinnedCLI(cli) {
  const { stdout, stderr } = await execFileAsync(cli, ["--version"], {
    timeout: 30_000,
    env: { ...process.env, NO_COLOR: "1" },
  });
  const combined = `${stdout}${stderr}`;
  assert.match(
    combined,
    /opencode v2\.2\.0/,
    `conformance requires ${CLI_PACKAGE}, got: ${combined}`,
  );
  assert.doesNotMatch(combined, /opencode v(?!2\.2\.0)\d/);
}

async function debugConfig(cli, workspace, env) {
  const child = spawn(cli, ["debug", "config", "--standalone"], {
    cwd: workspace,
    env,
    stdio: ["ignore", "pipe", "pipe"],
    detached: process.platform !== "win32",
  });
  const chunks = [];
  child.stdout.on("data", (chunk) => chunks.push(chunk));
  child.stderr.on("data", (chunk) => chunks.push(chunk));
  const timer = setTimeout(() => {
    try {
      if (process.platform !== "win32" && child.pid) process.kill(-child.pid, "SIGKILL");
    } catch {
      // already exited
    }
  }, 45_000);
  try {
    await once(child, "exit");
  } finally {
    clearTimeout(timer);
    killGroup(child);
  }
  return Buffer.concat(chunks).toString("utf8");
}

function isolatedEnv(home, root, configPath, extra = {}) {
  return {
    PATH: process.env.PATH,
    HOME: home,
    XDG_CONFIG_HOME: join(home, ".config"),
    XDG_DATA_HOME: join(home, ".local", "share"),
    XDG_CACHE_HOME: join(home, ".cache"),
    XDG_STATE_HOME: join(home, ".local", "state"),
    npm_config_cache: join(root, ".npm-cache"),
    OPENAI_API_KEY: "conformance-local-key",
    OPENCODE_CONFIG: configPath,
    OPENCODE_DB: join(root, "opencode.db"),
    NO_COLOR: "1",
    CI: "1",
    NO_PROXY: "127.0.0.1,localhost,::1",
    no_proxy: "127.0.0.1,localhost,::1",
    ...extra,
  };
}

async function startModelStub() {
  const state = { requests: 0 };
  const server = createServer(async (request, response) => {
    if (request.method === "GET" && request.url.endsWith("/models")) {
      sendJSON(response, {
        object: "list",
        data: [{ id: "gpt-4o-mini", object: "model", created: 1, owned_by: "conformance" }],
      });
      return;
    }
    if (!request.url.endsWith("/chat/completions")) {
      response.writeHead(404).end();
      return;
    }
    const body = await readJSON(request);
    state.requests += 1;
    const hasToolResult = (body.messages ?? []).some((message) => message.role === "tool");
    if (!hasToolResult) {
      const toolCall = {
        id: "call-conformance-shell",
        type: "function",
        function: {
          name: "shell",
          arguments: JSON.stringify({ command: "sleep 1; exit 3", background: true }),
        },
      };
      if (body.stream) {
        response.writeHead(200, { "content-type": "text/event-stream" });
        writeStreamChunk(response, body.model, {
          role: "assistant",
          tool_calls: [{ index: 0, ...toolCall }],
        });
        writeStreamChunk(response, body.model, {}, "tool_calls");
        response.end("data: [DONE]\n\n");
      } else {
        sendJSON(response, {
          id: "chatcmpl-conformance",
          object: "chat.completion",
          created: 1,
          model: body.model,
          choices: [
            {
              index: 0,
              message: { role: "assistant", content: null, tool_calls: [toolCall] },
              finish_reason: "tool_calls",
            },
          ],
          usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
        });
      }
      return;
    }
    const answer = "conformance complete";
    if (body.stream) {
      response.writeHead(200, { "content-type": "text/event-stream" });
      writeStreamChunk(response, body.model, { role: "assistant", content: answer });
      writeStreamChunk(response, body.model, {}, "stop");
      response.end("data: [DONE]\n\n");
    } else {
      sendJSON(response, {
        id: "chatcmpl-conformance",
        object: "chat.completion",
        created: 1,
        model: body.model,
        choices: [
          { index: 0, message: { role: "assistant", content: answer }, finish_reason: "stop" },
        ],
        usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
      });
    }
  });
  await listen(server, "127.0.0.1");
  return {
    endpoint: `http://127.0.0.1:${server.address().port}`,
    state,
    close: () =>
      new Promise((resolveClose) => {
        server.closeAllConnections();
        server.close(resolveClose);
      }),
  };
}

function writeStreamChunk(response, model, delta, finish = null) {
  response.write(
    `data: ${JSON.stringify({
      id: "chatcmpl-conformance",
      object: "chat.completion.chunk",
      created: 1,
      model,
      choices: [{ index: 0, delta, finish_reason: finish }],
    })}\n\n`,
  );
}

async function startBridge() {
  const credential = "conformance-credential-0001";
  const leaseId = "conformance-lease";
  const state = { hellos: [], claims: [], events: [], revision: 0 };
  const waiters = new Set();
  const notify = () => {
    for (const waiter of [...waiters]) waiter();
  };
  const server = createServer(async (request, response) => {
    if (request.headers.authorization !== `Bearer ${credential}`) {
      response.writeHead(401).end();
      return;
    }
    const url = new URL(request.url, "http://127.0.0.1");
    if (request.method === "POST" && url.pathname === "/v1/hello") {
      state.hellos.push(await readJSON(request));
      sendJSON(response, {
        protocolVersion: 1,
        leaseId,
        ready: true,
        behaviors: [{ id: "shell-outcomes", version: 1 }],
      });
      return;
    }
    if (request.method === "GET" && url.pathname === "/v1/config") {
      const requested = Number(url.searchParams.get("revision") ?? "0");
      while (state.revision <= requested) {
        await new Promise((resolveWait) => {
          const timer = setTimeout(() => {
            waiters.delete(resolveWait);
            resolveWait();
          }, 10_000);
          waiters.add(() => {
            clearTimeout(timer);
            waiters.delete(resolveWait);
            resolveWait();
          });
        });
      }
      sendJSON(response, {
        protocolVersion: 1,
        leaseId,
        revision: state.revision,
        behaviors: { "shell-outcomes": { version: 1, data: { claims: state.claims } } },
      });
      return;
    }
    if (request.method === "POST" && url.pathname === "/v1/events") {
      const body = await readJSON(request);
      state.events.push(...(body.events ?? []));
      sendJSON(response, {
        accepted: (body.events ?? []).map((event) => event.eventId),
        rejected: [],
      });
      return;
    }
    response.writeHead(404).end();
  });
  await listen(server, "127.0.0.1");
  const endpoint = `http://127.0.0.1:${server.address().port}`;
  return {
    endpoint,
    credential,
    leaseId,
    state,
    setClaim(claim) {
      state.claims = [claim];
      state.revision += 1;
      notify();
    },
    close: () =>
      new Promise((resolveClose) => {
        server.closeAllConnections();
        server.close(resolveClose);
      }),
  };
}

async function listen(server, host) {
  await new Promise((resolveListen) => server.listen(0, host, resolveListen));
}

function sendJSON(response, value) {
  response.writeHead(200, { "content-type": "application/json" });
  response.end(JSON.stringify(value));
}

async function readJSON(request) {
  let body = "";
  for await (const chunk of request) body += chunk;
  return JSON.parse(body);
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

async function waitUntil(predicate, label, timeoutMs = 20_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise((resolveWait) => setTimeout(resolveWait, 20));
  }
  throw new Error(`timed out waiting for ${label}`);
}

async function waitForFile(path, timeoutMs = 10_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      await access(path);
      return;
    } catch {
      await new Promise((resolveWait) => setTimeout(resolveWait, 20));
    }
  }
  throw new Error("timed out waiting for the user plugin marker");
}

async function stopChild(child, client) {
  client?.readline.close();
  if (!child || child.exitCode !== null) return;
  try {
    if (process.platform !== "win32" && child.pid) process.kill(-child.pid, "SIGKILL");
    else child.kill("SIGKILL");
  } catch {
    return;
  }
  await Promise.race([
    once(child, "exit"),
    new Promise((resolveWait) => setTimeout(resolveWait, 2000)),
  ]);
}

function killGroup(child) {
  if (!child || child.exitCode !== null) return;
  try {
    if (process.platform !== "win32" && child.pid) process.kill(-child.pid, "SIGKILL");
    else child.kill("SIGKILL");
  } catch {
    // The process already exited.
  }
}
