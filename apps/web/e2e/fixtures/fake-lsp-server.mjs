#!/usr/bin/env node

import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const logPath = process.env.KANDEV_LSP_E2E_LOG ?? path.join(os.homedir(), "lsp-e2e-events.jsonl");
const crashOnOpenPath = path.join(os.homedir(), "lsp-e2e-crash-on-open");
let input = Buffer.alloc(0);
let nextServerRequestId = 10_000;
const openDocumentUris = new Set();
const definitionTargetFile = "nested/references/Definition Target # query? 100%.kt";

function log(event, details = {}) {
  fs.appendFileSync(
    logPath,
    `${JSON.stringify({ event, pid: process.pid, timestamp: Date.now(), ...details })}\n`,
  );
}

function send(message) {
  const payload = Buffer.from(JSON.stringify({ jsonrpc: "2.0", ...message }), "utf8");
  process.stdout.write(`Content-Length: ${payload.length}\r\n\r\n`);
  process.stdout.write(payload);
}

function diagnostic(uri, message) {
  send({
    method: "textDocument/publishDiagnostics",
    params: {
      uri,
      diagnostics: [
        {
          range: {
            start: { line: 0, character: 0 },
            end: { line: 0, character: 8 },
          },
          severity: 2,
          source: "kandev-e2e",
          code: "FAKE_KOTLIN",
          message,
        },
      ],
    },
  });
}

function location(uri, line = 2) {
  return {
    uri,
    range: {
      start: { line, character: 4 },
      end: { line, character: 13 },
    },
  };
}

function siblingFileUri(uri, fileName) {
  const directoryEnd = uri.lastIndexOf("/") + 1;
  const encodedPath = fileName
    .split("/")
    .map((segment) => encodeURIComponent(segment))
    .join("/");
  return `${uri.slice(0, directoryEnd)}${encodedPath}`;
}

function requestDocumentUri(message) {
  const uri = message.params?.textDocument?.uri;
  if (uri) return uri;
  if (openDocumentUris.size !== 1) return null;
  return openDocumentUris.values().next().value ?? null;
}

function handleRequest(message) {
  const uri = requestDocumentUri(message);
  switch (message.method) {
    case "initialize":
      send({
        id: message.id,
        result: {
          capabilities: {
            textDocumentSync: { openClose: true, change: 1 },
            completionProvider: { triggerCharacters: ["."] },
            hoverProvider: true,
            definitionProvider: true,
            referencesProvider: true,
            signatureHelpProvider: { triggerCharacters: ["(", ","] },
            semanticTokensProvider: {
              legend: { tokenTypes: ["function", "variable"], tokenModifiers: [] },
              full: true,
            },
          },
        },
      });
      return;
    case "textDocument/completion":
      send({
        id: message.id,
        result: {
          isIncomplete: false,
          items: [
            {
              label: "fakeGreeting",
              kind: 3,
              detail: "Kandev E2E completion",
              insertText: "fakeGreeting",
            },
          ],
        },
      });
      return;
    case "textDocument/hover":
      send({
        id: message.id,
        result: {
          contents: { kind: "markdown", value: "**Fake Kotlin hover**" },
          range: location(uri).range,
        },
      });
      return;
    case "textDocument/definition":
      send({
        id: message.id,
        result: uri ? location(siblingFileUri(uri, definitionTargetFile)) : null,
      });
      return;
    case "textDocument/references":
      send({ id: message.id, result: uri ? [location(uri), location(uri, 3)] : [] });
      return;
    case "textDocument/signatureHelp":
      send({
        id: message.id,
        result: {
          signatures: [
            {
              label: "greeting(name: String): String",
              documentation: "Kandev E2E signature",
              parameters: [{ label: "name: String" }],
            },
          ],
          activeSignature: 0,
          activeParameter: 0,
        },
      });
      return;
    case "textDocument/semanticTokens/full":
      send({ id: message.id, result: { resultId: "e2e-1", data: [2, 4, 8, 0, 0] } });
      return;
    case "shutdown":
      send({ id: message.id, result: null });
      return;
    default:
      send({ id: message.id, result: null });
  }
}

function handleDidOpen(message) {
  const uri = message.params?.textDocument?.uri;
  if (uri) {
    openDocumentUris.add(uri);
    diagnostic(uri, "Fake Kotlin diagnostic");
  }
  if (fs.existsSync(crashOnOpenPath)) {
    log("crashing", { reason: "didOpen" });
    process.exit(23);
  }
}

function handleDidChange(message) {
  const uri = message.params?.textDocument?.uri;
  if (uri && openDocumentUris.has(uri)) {
    diagnostic(uri, "Fake Kotlin diagnostic after edit");
  }
}

function handleDidClose(message) {
  const uri = message.params?.textDocument?.uri;
  if (!uri) return;
  send({
    method: "textDocument/publishDiagnostics",
    params: { uri, diagnostics: [] },
  });
  openDocumentUris.delete(uri);
}

function handleNotification(message) {
  switch (message.method) {
    case "initialized": {
      const id = nextServerRequestId++;
      log("workspace/configuration requested", { id });
      send({
        id,
        method: "workspace/configuration",
        params: { items: [{ section: "kotlin" }] },
      });
      return;
    }
    case "textDocument/didOpen":
      handleDidOpen(message);
      return;
    case "textDocument/didChange":
      handleDidChange(message);
      return;
    case "textDocument/didClose":
      handleDidClose(message);
      return;
    case "exit":
      log("exit");
      process.exit(0);
  }
}

function handleMessage(message) {
  if (message.method) {
    log("message", { id: message.id, method: message.method, params: message.params });
    if (message.id !== undefined) handleRequest(message);
    else handleNotification(message);
    return;
  }

  log("response", { id: message.id, result: message.result, error: message.error });
}

function parseInput() {
  while (input.length > 0) {
    const headerEnd = input.indexOf("\r\n\r\n");
    if (headerEnd < 0) return;

    const header = input.subarray(0, headerEnd).toString("ascii");
    const lengthMatch = /^Content-Length:\s*(\d+)$/im.exec(header);
    if (!lengthMatch) {
      log("protocol error", { header });
      process.exit(2);
    }

    const contentLength = Number(lengthMatch[1]);
    const payloadStart = headerEnd + 4;
    const payloadEnd = payloadStart + contentLength;
    if (input.length < payloadEnd) return;

    const payload = input.subarray(payloadStart, payloadEnd).toString("utf8");
    input = input.subarray(payloadEnd);
    handleMessage(JSON.parse(payload));
  }
}

process.stdin.on("data", (chunk) => {
  input = Buffer.concat([input, chunk]);
  parseInput();
});
process.stdin.on("end", () => {
  log("stdin ended");
});
process.on("SIGTERM", () => {
  log("signal", { signal: "SIGTERM" });
  process.exit(0);
});
process.on("SIGINT", () => {
  log("signal", { signal: "SIGINT" });
  process.exit(0);
});
process.on("uncaughtException", (error) => {
  log("uncaught exception", { error: String(error), stack: error.stack });
  process.exit(1);
});

log("started", { argv: process.argv.slice(2), cwd: process.cwd(), home: os.homedir() });
