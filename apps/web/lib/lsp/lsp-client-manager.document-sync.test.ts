import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createLspManagerHarness, FakeWebSocket } from "./lsp-client-manager.test-harness";
import { modelUriForDocument } from "./file-uri";

const mocks = vi.hoisted(() => ({
  getMonacoInstance: vi.fn(),
  waitForMonacoInstance: vi.fn(),
  getWebSocketClient: vi.fn(),
  registerLspProviders: vi.fn(),
  requestFileContent: vi.fn(),
  setBuiltinTsSuppressed: vi.fn(),
}));

vi.mock("@/components/editors/monaco/monaco-init", () => ({
  getMonacoInstance: mocks.getMonacoInstance,
  waitForMonacoInstance: mocks.waitForMonacoInstance,
}));
vi.mock("@/components/editors/monaco/builtin-providers", () => ({
  setBuiltinTsSuppressed: mocks.setBuiltinTsSuppressed,
  withLspProviderRegistration: <T>(register: () => T) => register(),
}));
vi.mock("./lsp-providers", () => ({ registerLspProviders: mocks.registerLspProviders }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: mocks.getWebSocketClient,
}));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: mocks.requestFileContent,
}));

import { lspClientManager } from "./lsp-client-manager";

const { connectReady, createMonacoHarness } = createLspManagerHarness(lspClientManager, mocks);
const SESSION_ID = "session";
const WORKSPACE_PATH = "/workspace";
const DOCUMENT_URI = "file:///workspace/backend/src/Main.ts";
const DID_CHANGE = "textDocument/didChange";
const DID_SAVE = "textDocument/didSave";
const PERSISTED_SNAPSHOT = "persisted snapshot";

function documentSynchronization(socket: FakeWebSocket) {
  return socket.sent
    .map((frame) => JSON.parse(frame) as { method?: string; params?: unknown })
    .filter((frame) => frame.method === DID_CHANGE || frame.method === DID_SAVE)
    .map(({ method, params }) => ({ method, params }));
}

beforeEach(() => {
  lspClientManager.disconnectAll();
  FakeWebSocket.instances = [];
  vi.resetAllMocks();
  vi.stubGlobal("WebSocket", FakeWebSocket);
  mocks.waitForMonacoInstance.mockImplementation(async () => mocks.getMonacoInstance());
});

afterEach(() => {
  lspClientManager.disconnectAll();
  vi.unstubAllGlobals();
});

describe("LSP document subscriptions", () => {
  it("shares synchronization for duplicate logical views of one canonical URI", async () => {
    createMonacoHarness([modelUriForDocument(DOCUMENT_URI, SESSION_ID)]);
    mocks.registerLspProviders.mockReturnValue([]);
    const socket = await connectReady(SESSION_ID, WORKSPACE_PATH);
    const document = {
      uri: DOCUMENT_URI,
      languageId: "typescript",
      text: "export const value = 1;",
    };

    lspClientManager.openDocument(SESSION_ID, "typescript", document);
    lspClientManager.openDocument(SESSION_ID, "typescript", { ...document, repo: "backend" });

    const notificationCount = (method: string) =>
      socket.sent.filter((frame) => JSON.parse(frame).method === method).length;
    expect(notificationCount("textDocument/didOpen")).toBe(1);

    lspClientManager.changeDocument(
      SESSION_ID,
      "typescript",
      DOCUMENT_URI,
      "export const value = 2;",
    );
    lspClientManager.changeDocument(
      SESSION_ID,
      "typescript",
      DOCUMENT_URI,
      "export const value = 2;",
    );
    expect(notificationCount(DID_CHANGE)).toBe(1);

    lspClientManager.closeDocument(SESSION_ID, "typescript", DOCUMENT_URI);
    expect(notificationCount("textDocument/didClose")).toBe(0);
    lspClientManager.closeDocument(SESSION_ID, "typescript", DOCUMENT_URI);
    expect(notificationCount("textDocument/didClose")).toBe(1);
  });
});

describe("LSP document saves", () => {
  it("notifies a save-capable server after an open repo-scoped document is persisted", async () => {
    createMonacoHarness([modelUriForDocument(DOCUMENT_URI, SESSION_ID)]);
    mocks.registerLspProviders.mockReturnValue([]);
    const socket = await connectReady(SESSION_ID, WORKSPACE_PATH, {
      textDocumentSync: {
        openClose: true,
        change: 1,
        save: { includeText: true },
      },
    });
    lspClientManager.openDocument(SESSION_ID, "typescript", {
      uri: DOCUMENT_URI,
      languageId: "typescript",
      text: "before save",
      repo: "backend",
    });

    lspClientManager.saveDocument(SESSION_ID, "src/Main.ts", "backend", PERSISTED_SNAPSHOT);

    const synchronization = documentSynchronization(socket);
    expect(synchronization.map((frame) => frame.method)).toEqual([DID_CHANGE, DID_SAVE]);
    expect(synchronization[0]?.params).toEqual({
      textDocument: { uri: DOCUMENT_URI, version: 2 },
      contentChanges: [{ text: PERSISTED_SNAPSHOT }],
    });
    expect(synchronization[1]?.params).toEqual({
      textDocument: { uri: DOCUMENT_URI },
      text: PERSISTED_SNAPSHOT,
    });
  });

  it("does not rewind an open document when the editor advances during persistence", async () => {
    createMonacoHarness([modelUriForDocument(DOCUMENT_URI, SESSION_ID)]);
    mocks.registerLspProviders.mockReturnValue([]);
    const socket = await connectReady(SESSION_ID, WORKSPACE_PATH, {
      textDocumentSync: {
        openClose: true,
        change: 1,
        save: { includeText: true },
      },
    });
    lspClientManager.openDocument(SESSION_ID, "typescript", {
      uri: DOCUMENT_URI,
      languageId: "typescript",
      text: "before save",
      repo: "backend",
    });

    lspClientManager.saveDocument(
      SESSION_ID,
      "src/Main.ts",
      "backend",
      PERSISTED_SNAPSHOT,
      "newer editor snapshot",
    );

    const synchronization = documentSynchronization(socket);
    expect(synchronization).toEqual([
      {
        method: DID_CHANGE,
        params: {
          textDocument: { uri: DOCUMENT_URI, version: 2 },
          contentChanges: [{ text: "newer editor snapshot" }],
        },
      },
      {
        method: DID_SAVE,
        params: { textDocument: { uri: DOCUMENT_URI } },
      },
    ]);
  });

  it("does not notify save-capable servers for documents that are not open", async () => {
    createMonacoHarness([]);
    mocks.registerLspProviders.mockReturnValue([]);
    const socket = await connectReady(SESSION_ID, WORKSPACE_PATH, {
      textDocumentSync: { openClose: true, change: 1, save: true },
    });

    lspClientManager.saveDocument(SESSION_ID, "src/Closed.ts", undefined, "saved text");

    expect(socket.sent.some((frame) => JSON.parse(frame).method === DID_SAVE)).toBe(false);
  });
});
