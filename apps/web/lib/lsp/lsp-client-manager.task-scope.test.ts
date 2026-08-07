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
        workspaceUri: "file:///workspace",
        workspaceFolders: [{ uri: "file:///workspace/backend", name: "backend" }],
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
        workspaceUri: "file:///workspace",
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
