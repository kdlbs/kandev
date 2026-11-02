import { createServer } from "node:http";
import { mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { PROTOCOL_VERSION } from "../../src/protocol.js";

/**
 * Minimal in-process host used by the plugin unit tests. It implements only the
 * three protocol operations and records what the plugin sent.
 */
export async function startFakeBridge(options = {}) {
  const behaviorVersions = options.behaviorVersions ?? { "shell-outcomes": 1 };
  const claims = options.claims ?? [];
  const behaviorConfigs = options.behaviorConfigs ?? { "shell-outcomes": { claims } };
  const state = {
    hellos: [],
    configPolls: [],
    events: [],
    revision: options.revision ?? 1,
    rejectEvents: options.rejectEvents ?? false,
  };
  const server = createServer(async (request, response) => {
    if (
      request.headers.authorization !== `Bearer ${options.credential ?? "test-credential-0001"}`
    ) {
      response.writeHead(401).end();
      return;
    }
    const url = new URL(request.url ?? "/", "http://127.0.0.1");
    if (request.method === "POST" && url.pathname === "/v1/hello") {
      state.hellos.push(await readJSON(request));
      sendJSON(response, 200, {
        protocolVersion: PROTOCOL_VERSION,
        leaseId: options.leaseId ?? "lease-test",
        ready: true,
        behaviors: Object.entries(behaviorVersions).map(([id, version]) => ({ id, version })),
      });
      return;
    }
    if (request.method === "GET" && url.pathname === "/v1/config") {
      const revision = Number(url.searchParams.get("revision") ?? "0");
      state.configPolls.push(revision);
      sendJSON(response, 200, {
        protocolVersion: PROTOCOL_VERSION,
        leaseId: options.leaseId ?? "lease-test",
        revision: state.revision,
        behaviors: Object.fromEntries(
          Object.entries(behaviorVersions).map(([id, version]) => [
            id,
            { version, data: behaviorConfigs[id] },
          ]),
        ),
      });
      return;
    }
    if (request.method === "POST" && url.pathname === "/v1/events") {
      const body = await readJSON(request);
      state.events.push(...(body.events ?? []));
      if (state.rejectEvents) {
        response.writeHead(503).end();
        return;
      }
      sendJSON(response, 200, {
        accepted: (body.events ?? []).map((event) => event.eventId),
        rejected: [],
      });
      return;
    }
    response.writeHead(404).end();
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("fake bridge did not bind");
  const endpoint = `http://127.0.0.1:${address.port}`;

  const root = await mkdtemp(join(tmpdir(), "kandev-plugin-test-"));
  const bootstrapFile = join(root, "bootstrap.json");
  const bootstrap = {
    protocolVersion: PROTOCOL_VERSION,
    endpoint,
    credential: options.credential ?? "test-credential-0001",
    leaseId: options.leaseId ?? "lease-test",
    artifactHash: "test-artifact",
    behaviors: Object.entries(behaviorVersions).map(([id, version]) => ({ id, version })),
  };
  await writeFile(bootstrapFile, JSON.stringify(bootstrap), { mode: 0o600 });

  return {
    endpoint,
    bootstrapFile,
    state,
    async close() {
      server.closeAllConnections();
      await new Promise((resolve) => server.close(resolve));
    },
  };
}

export function makeContext({ bootstrapFile, version = "2.2.0", eventSource }) {
  return {
    app: { version },
    options: { bootstrapFile },
    event: {
      subscribe({ signal }) {
        return eventSource(signal);
      },
    },
  };
}

export function makeEventSource() {
  const listeners = new Set();
  return {
    push(event) {
      for (const listener of [...listeners]) listener(event);
    },
    stream(signal) {
      const queue = [];
      let resolveNext;
      const emit = (event) => {
        if (resolveNext) {
          const resolve = resolveNext;
          resolveNext = undefined;
          resolve({ value: event, done: false });
        } else {
          queue.push(event);
        }
      };
      listeners.add(emit);
      const onAbort = () => {
        listeners.delete(emit);
        if (resolveNext) {
          const resolve = resolveNext;
          resolveNext = undefined;
          resolve({ value: undefined, done: true });
        }
      };
      signal.addEventListener("abort", onAbort, { once: true });
      return {
        [Symbol.asyncIterator]() {
          return {
            next() {
              if (queue.length > 0) return Promise.resolve({ value: queue.shift(), done: false });
              if (signal.aborted) return Promise.resolve({ value: undefined, done: true });
              return new Promise((resolve) => {
                resolveNext = resolve;
              });
            },
          };
        },
      };
    },
  };
}

function sendJSON(response, status, value) {
  response.writeHead(status, { "content-type": "application/json" });
  response.end(JSON.stringify(value));
}

async function readJSON(request) {
  let body = "";
  for await (const chunk of request) body += chunk;
  return JSON.parse(body);
}
