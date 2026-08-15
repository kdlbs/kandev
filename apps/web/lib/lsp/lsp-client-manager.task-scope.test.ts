import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createLspManagerHarness,
  FakeWebSocket,
  markerMessages,
  publishDiagnostic,
} from "./lsp-client-manager.test-harness";
import { modelUriForDocument } from "./file-uri";

const mocks = vi.hoisted(() => ({
  getMonacoInstance: vi.fn(),
  waitForMonacoInstance: vi.fn(),
  registerLspProviders: vi.fn(),
  registerBuiltinTsSuppression: vi.fn(),
}));

vi.mock("@/components/editors/monaco/monaco-init", () => ({
  getMonacoInstance: mocks.getMonacoInstance,
  waitForMonacoInstance: mocks.waitForMonacoInstance,
}));
vi.mock("@/components/editors/monaco/builtin-providers", () => ({
  registerBuiltinTsSuppression: mocks.registerBuiltinTsSuppression,
  withLspProviderRegistration: <T>(register: () => T) => register(),
}));
vi.mock("./lsp-providers", () => ({ registerLspProviders: mocks.registerLspProviders }));

import { lspClientManager } from "./lsp-client-manager";

const { createMonacoHarness } = createLspManagerHarness(lspClientManager, mocks);

const TASK_ID = "task/one";
const LANGUAGE = "typescript";
const WORKSPACE_URI = "file:///workspace";

beforeEach(() => {
  lspClientManager.disconnectAll();
  FakeWebSocket.instances = [];
  vi.resetAllMocks();
  vi.stubGlobal("WebSocket", FakeWebSocket);
  const { monaco } = createMonacoHarness([]);
  mocks.getMonacoInstance.mockReturnValue(monaco);
  mocks.waitForMonacoInstance.mockResolvedValue(monaco);
  mocks.registerLspProviders.mockReturnValue([]);
  mocks.registerBuiltinTsSuppression.mockReturnValue({ dispose: vi.fn() });
});

afterEach(() => {
  lspClientManager.disconnectAll();
  vi.unstubAllGlobals();
});

describe("task-scoped LSP attachment", () => {
  it("shares one attachment across same-task sessions and never sends lifecycle methods", async () => {
    const releaseFirst = lspClientManager.connect(TASK_ID, "session-a", LANGUAGE);
    const releaseSecond = lspClientManager.connect(TASK_ID, "session-b", LANGUAGE);
    expect(FakeWebSocket.instances).toHaveLength(1);
    const socket = FakeWebSocket.instances[0];
    expect(socket.url.endsWith("/lsp/tasks/task%2Fone/typescript/attach")).toBe(true);

    socket.open();
    socket.emitMessage(
      JSON.stringify({
        status: "attached",
        language: LANGUAGE,
        generation: 4,
        workspaceUri: WORKSPACE_URI,
        workspaceFolders: [{ uri: `${WORKSPACE_URI}/backend`, name: "backend" }],
        serverCapabilities: { completionProvider: {}, textDocumentSync: 1 },
      }),
    );
    await vi.waitFor(() =>
      expect(lspClientManager.getStatus(TASK_ID, LANGUAGE)).toEqual({ state: "ready" }),
    );
    expect(mocks.registerLspProviders.mock.calls[0]?.[0]?.serverCapabilities).toEqual({
      completionProvider: {},
      textDocumentSync: 1,
    });
    expect(socket.sent.map((frame) => JSON.parse(frame).method).filter(Boolean)).not.toContain(
      "initialize",
    );

    releaseFirst();
    expect(socket.readyState).toBe(FakeWebSocket.OPEN);
    releaseSecond();
    expect(socket.readyState).toBe(FakeWebSocket.CLOSED);
    expect(socket.sent.map((frame) => JSON.parse(frame).method).filter(Boolean)).not.toEqual(
      expect.arrayContaining(["shutdown", "exit"]),
    );
  });

  it("routes both same-task session models and deduplicates their document stream", async () => {
    const documentUri = "file:///workspace/Main.ts";
    const firstModelUri = modelUriForDocument(documentUri, "session-a");
    const secondModelUri = modelUriForDocument(documentUri, "session-b");
    const { markersByUri, models } = createMonacoHarness([firstModelUri, secondModelUri]);

    lspClientManager.connect(TASK_ID, "session-a", LANGUAGE);
    lspClientManager.connect(TASK_ID, "session-b", LANGUAGE);
    const socket = FakeWebSocket.instances[0];
    socket.open();
    socket.emitMessage(
      JSON.stringify({
        status: "attached",
        language: LANGUAGE,
        generation: 4,
        workspaceUri: WORKSPACE_URI,
        workspaceFolders: [],
        serverCapabilities: { textDocumentSync: 1 },
      }),
    );
    await vi.waitFor(() =>
      expect(lspClientManager.getStatus(TASK_ID, LANGUAGE).state).toBe("ready"),
    );

    const provider = mocks.registerLspProviders.mock.calls[0]?.[0] as {
      getDocumentUri: (model: unknown) => string | null;
    };
    expect(provider.getDocumentUri(models.get(firstModelUri))).toBe(documentUri);
    expect(provider.getDocumentUri(models.get(secondModelUri))).toBe(documentUri);

    publishDiagnostic(socket, documentUri, "shared issue");
    expect(markerMessages(markersByUri, firstModelUri)).toContain("shared issue");
    expect(markerMessages(markersByUri, secondModelUri)).toContain("shared issue");

    const document = { uri: documentUri, languageId: LANGUAGE, text: "export {};" };
    lspClientManager.openDocument("session-a", LANGUAGE, document);
    lspClientManager.openDocument("session-b", LANGUAGE, document);
    const count = (method: string) =>
      socket.sent.filter((frame) => JSON.parse(frame).method === method).length;
    expect(count("textDocument/didOpen")).toBe(1);
    lspClientManager.closeDocument("session-a", LANGUAGE, documentUri);
    expect(count("textDocument/didClose")).toBe(0);
    lspClientManager.closeDocument("session-b", LANGUAGE, documentUri);
    expect(count("textDocument/didClose")).toBe(1);
  });
});

describe("task-scoped LSP lease cleanup", () => {
  it("releases a session's document references before dropping its final lease", async () => {
    const releaseA = lspClientManager.connect(TASK_ID, "session-a", LANGUAGE);
    const releaseB = lspClientManager.connect(TASK_ID, "session-b", LANGUAGE);
    const socket = FakeWebSocket.instances[0];
    socket.open();
    socket.emitMessage(
      JSON.stringify({
        status: "attached",
        language: LANGUAGE,
        generation: 4,
        workspaceUri: WORKSPACE_URI,
        workspaceFolders: [],
        serverCapabilities: { textDocumentSync: 1 },
      }),
    );
    await vi.waitFor(() =>
      expect(lspClientManager.getStatus(TASK_ID, LANGUAGE).state).toBe("ready"),
    );

    const document = {
      uri: "file:///workspace/Shared.ts",
      languageId: LANGUAGE,
      text: "export const shared = true;",
    };
    lspClientManager.openDocument("session-a", LANGUAGE, document);
    lspClientManager.openDocument("session-b", LANGUAGE, document);
    releaseA();
    lspClientManager.closeDocument("session-b", LANGUAGE, document.uri);

    expect(
      socket.sent.filter((frame) => JSON.parse(frame).method === "textDocument/didClose"),
    ).toHaveLength(1);
    releaseB();
  });
});

describe("task-scoped LSP reconnect", () => {
  it("preserves every session lease across a browser transport generation", async () => {
    vi.useFakeTimers();
    const releaseA = lspClientManager.connect(TASK_ID, "session-a", LANGUAGE);
    const releaseB = lspClientManager.connect(TASK_ID, "session-b", LANGUAGE);
    const first = FakeWebSocket.instances[0];
    first.failClosed(1006, "network interrupted");

    await vi.advanceTimersByTimeAsync(999);
    expect(FakeWebSocket.instances).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(FakeWebSocket.instances).toHaveLength(2);
    const second = FakeWebSocket.instances[1];
    second.open();
    second.emitMessage(
      JSON.stringify({
        status: "attached",
        language: LANGUAGE,
        generation: 4,
        workspaceUri: WORKSPACE_URI,
        workspaceFolders: [],
        serverCapabilities: { textDocumentSync: 1 },
      }),
    );
    await vi.runAllTimersAsync();
    expect(lspClientManager.getStatus(TASK_ID, LANGUAGE)).toEqual({ state: "ready" });

    lspClientManager.openDocument("session-a", LANGUAGE, {
      uri: "file:///workspace/A.ts",
      languageId: LANGUAGE,
      text: "export const a = 1;",
    });
    lspClientManager.openDocument("session-b", LANGUAGE, {
      uri: "file:///workspace/B.ts",
      languageId: LANGUAGE,
      text: "export const b = 1;",
    });
    expect(
      second.sent.filter((frame) => JSON.parse(frame).method === "textDocument/didOpen"),
    ).toHaveLength(2);
    releaseA();
    expect(second.readyState).toBe(FakeWebSocket.OPEN);
    releaseB();
    expect(second.readyState).toBe(FakeWebSocket.CLOSED);
    vi.useRealTimers();
  });

  it("cancels manager reconnect when the final stable lease releases", async () => {
    vi.useFakeTimers();
    const release = lspClientManager.connect(TASK_ID, "session-a", LANGUAGE);
    FakeWebSocket.instances[0].failClosed(1006, "network interrupted");
    release();
    await vi.advanceTimersByTimeAsync(5_000);
    expect(FakeWebSocket.instances).toHaveLength(1);
    vi.useRealTimers();
  });

  it("cancels a transport retry when close proves the attachment unavailable", async () => {
    vi.useFakeTimers();
    lspClientManager.connect(TASK_ID, "session-a", LANGUAGE);
    const first = FakeWebSocket.instances[0];
    first.onerror?.(new Event("error"));
    first.failClosed(4004, "unsupported executor");
    await vi.advanceTimersByTimeAsync(5_000);
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(lspClientManager.getStatus(TASK_ID, LANGUAGE)).toMatchObject({
      state: "unavailable",
      cause: "unsupported_executor",
    });
    vi.useRealTimers();
  });
});

describe("task-scoped LSP activation recovery", () => {
  it("retries a failed attachment activation without losing its stable lease", async () => {
    vi.useFakeTimers();
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    mocks.waitForMonacoInstance.mockRejectedValueOnce(new Error("Monaco unavailable"));
    const release = lspClientManager.connect(TASK_ID, "session-a", LANGUAGE);
    const first = FakeWebSocket.instances[0];
    first.open();
    first.emitMessage(
      JSON.stringify({
        status: "attached",
        language: LANGUAGE,
        generation: 4,
        workspaceUri: WORKSPACE_URI,
        workspaceFolders: [],
        serverCapabilities: { textDocumentSync: 1 },
      }),
    );
    await Promise.resolve();
    await Promise.resolve();
    expect(lspClientManager.getStatus(TASK_ID, LANGUAGE).state).toBe("error");

    await vi.advanceTimersByTimeAsync(1_000);
    expect(FakeWebSocket.instances).toHaveLength(2);
    const second = FakeWebSocket.instances[1];
    second.open();
    second.emitMessage(
      JSON.stringify({
        status: "attached",
        language: LANGUAGE,
        generation: 4,
        workspaceUri: WORKSPACE_URI,
        workspaceFolders: [],
        serverCapabilities: { textDocumentSync: 1 },
      }),
    );
    await vi.runAllTimersAsync();
    expect(lspClientManager.getStatus(TASK_ID, LANGUAGE)).toEqual({ state: "ready" });
    release();
    consoleError.mockRestore();
    vi.useRealTimers();
  });
});
