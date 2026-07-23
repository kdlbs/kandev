import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createDeferred,
  createLspManagerHarness,
  FakeWebSocket,
  markerMessages,
  publishDiagnostic,
  type TestModel,
} from "./lsp-client-manager.test-harness";
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
}));

vi.mock("./lsp-providers", () => ({
  registerLspProviders: mocks.registerLspProviders,
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: mocks.getWebSocketClient,
}));

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: mocks.requestFileContent,
}));

import { lspClientManager } from "./lsp-client-manager";

const {
  connectReady,
  connectReadyWithRelease,
  createMonacoHarness,
  createPlaceholder,
  requireModel,
} = createLspManagerHarness(lspClientManager, mocks);

const REPLACEMENT_SESSION_ID = "replacement-session";
const WORKSPACE_PATH = "/workspace";
const WORKSPACE_URI = "file:///workspace";
const PRIMARY_DOCUMENT_URI = `${WORKSPACE_URI}/Main.ts`;
const REFERENCED_DOCUMENT_URI = `${WORKSPACE_URI}/Referenced.ts`;
const FIRST_SESSION_ID = "first-session";
const SECOND_SESSION_ID = "second-session";
const WINDOWS_SESSION_ID = "windows-session";
const EXPECTED_SOCKET_ERROR = "expected an LSP WebSocket";
const SESSION_ID = "session";

function modelUri(documentUri: string, sessionId = SESSION_ID): string {
  return modelUriForDocument(documentUri, sessionId);
}

function beginInitialization(
  sessionId = SESSION_ID,
  ready: Record<string, unknown> = { workspacePath: WORKSPACE_PATH },
) {
  const release = lspClientManager.connect(sessionId, "typescript");
  const socket = FakeWebSocket.instances.at(-1);
  if (!socket) throw new Error(EXPECTED_SOCKET_ERROR);
  socket.open();
  socket.emitMessage(JSON.stringify({ status: "ready", ...ready }));
  const initializeId = (JSON.parse(socket.sent[0]) as { id: number }).id;
  return { initializeId, release, socket };
}

function completeInitialization(socket: FakeWebSocket, initializeId: number): void {
  socket.emitMessage(
    JSON.stringify({ jsonrpc: "2.0", id: initializeId, result: { capabilities: {} } }),
  );
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

describe("LSP editor readiness", () => {
  it("does not report ready or register providers before Monaco is available", async () => {
    const monacoReady = createDeferred<unknown>();
    const primaryUri = PRIMARY_DOCUMENT_URI;
    const primaryModelUri = modelUri(primaryUri);
    const { markersByUri, monaco } = createMonacoHarness([primaryModelUri]);
    mocks.waitForMonacoInstance.mockReturnValue(monacoReady.promise);
    mocks.registerLspProviders.mockReturnValue([]);

    const { socket, initializeId } = beginInitialization();
    completeInitialization(socket, initializeId);
    await Promise.resolve();

    expect(mocks.registerLspProviders).not.toHaveBeenCalled();
    expect(lspClientManager.getStatus("session", "typescript")).toEqual({ state: "starting" });
    lspClientManager.openDocument("session", "typescript", {
      uri: primaryUri,
      languageId: "typescript",
      text: "export {};",
    });
    expect(socket.sent.some((message) => message.includes("textDocument/didOpen"))).toBe(false);

    monacoReady.resolve(monaco);
    await vi.waitFor(() => {
      expect(lspClientManager.getStatus("session", "typescript")).toEqual({ state: "ready" });
    });
    expect(mocks.registerLspProviders).toHaveBeenCalledOnce();

    lspClientManager.openDocument("session", "typescript", {
      uri: primaryUri,
      languageId: "typescript",
      text: "export {};",
    });
    expect(socket.sent.some((message) => message.includes("textDocument/didOpen"))).toBe(true);
    publishDiagnostic(socket, primaryUri, "ready issue");
    expect(markerMessages(markersByUri, primaryModelUri)).toContain("ready issue");
  });

  it("does not finish initialization after the connection is stopped while Monaco loads", async () => {
    const monacoReady = createDeferred<unknown>();
    const { monaco } = createMonacoHarness([]);
    mocks.waitForMonacoInstance.mockReturnValue(monacoReady.promise);
    mocks.registerLspProviders.mockReturnValue([]);

    const { socket, initializeId } = beginInitialization();
    completeInitialization(socket, initializeId);
    await Promise.resolve();

    lspClientManager.stop("session", "typescript");
    monacoReady.resolve(monaco);
    await Promise.resolve();
    await Promise.resolve();

    expect(mocks.registerLspProviders).not.toHaveBeenCalled();
    expect(lspClientManager.getStatus("session", "typescript")).toEqual({ state: "disabled" });
  });
});

describe("LSP diagnostic readiness", () => {
  it("replays diagnostics when their matching Monaco model is created", async () => {
    const primaryUri = "file:///workspace/Late.ts";
    const { markersByUri, monaco } = createMonacoHarness([]);
    mocks.registerLspProviders.mockReturnValue([]);
    const socket = await connectReady("session", WORKSPACE_PATH);

    publishDiagnostic(socket, primaryUri, "late model issue");
    const primaryModelUri = modelUri(primaryUri);
    expect(markerMessages(markersByUri, primaryModelUri)).toEqual([]);

    monaco.editor.createModel("", "typescript", monaco.Uri.parse(primaryModelUri));

    expect(markerMessages(markersByUri, primaryModelUri)).toContain("late model issue");
  });

  it("replays diagnostics cached while Monaco loads onto an existing model", async () => {
    const monacoReady = createDeferred<unknown>();
    const primaryUri = "file:///workspace/Existing.ts";
    const primaryModelUri = modelUri(primaryUri);
    const { markersByUri, monaco } = createMonacoHarness([primaryModelUri]);
    mocks.getMonacoInstance.mockReturnValue(null);
    mocks.waitForMonacoInstance.mockReturnValue(monacoReady.promise);
    mocks.registerLspProviders.mockReturnValue([]);

    const { socket, initializeId } = beginInitialization();
    completeInitialization(socket, initializeId);
    await Promise.resolve();

    publishDiagnostic(socket, primaryUri, "cached while loading");
    expect(markerMessages(markersByUri, primaryModelUri)).toEqual([]);

    mocks.getMonacoInstance.mockReturnValue(monaco);
    monacoReady.resolve(monaco);
    await vi.waitFor(() => {
      expect(lspClientManager.getStatus("session", "typescript")).toEqual({ state: "ready" });
    });
    expect(markerMessages(markersByUri, primaryModelUri)).toContain("cached while loading");
  });
});

describe("LSP readiness failure cleanup", () => {
  it("cleans up a failed readiness attempt so retry creates a fresh connection", async () => {
    const { monaco } = createMonacoHarness([]);
    mocks.waitForMonacoInstance
      .mockRejectedValueOnce(new Error("Monaco chunk failed"))
      .mockResolvedValue(monaco);
    mocks.registerLspProviders.mockReturnValue([]);
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    const { socket: firstSocket, initializeId, release: firstRelease } = beginInitialization();
    completeInitialization(firstSocket, initializeId);

    await vi.waitFor(() => {
      expect(lspClientManager.getStatus("session", "typescript")).toEqual({
        state: "error",
        reason: "Error: Monaco chunk failed",
      });
    });
    expect(firstSocket.readyState).toBe(FakeWebSocket.CLOSED);

    await connectReady("session", WORKSPACE_PATH);
    expect(FakeWebSocket.instances).toHaveLength(2);
    expect(lspClientManager.getStatus("session", "typescript")).toEqual({ state: "ready" });
    firstRelease();
    consoleError.mockRestore();
  });

  it("disposes the model listener and RPC when provider registration fails", async () => {
    const primaryUri = PRIMARY_DOCUMENT_URI;
    const primaryModelUri = modelUri(primaryUri);
    const { markersByUri, monaco } = createMonacoHarness([primaryModelUri]);
    const listenerDispose = vi.fn();
    monaco.editor.onDidCreateModel.mockReturnValue({ dispose: listenerDispose });
    mocks.registerLspProviders.mockImplementation(() => {
      throw new Error("provider registration failed");
    });
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    const { socket, initializeId } = beginInitialization();
    completeInitialization(socket, initializeId);

    await vi.waitFor(() => {
      expect(lspClientManager.getStatus("session", "typescript").state).toBe("error");
    });
    expect(listenerDispose).toHaveBeenCalledOnce();
    expect(socket.readyState).toBe(FakeWebSocket.CLOSED);
    publishDiagnostic(socket, primaryUri, "late stale issue");
    expect(markerMessages(markersByUri, primaryModelUri)).not.toContain("late stale issue");
    consoleError.mockRestore();
  });
});

describe("LSP client connection cleanup", () => {
  it("fully restores editor state when a ready TypeScript server closes unexpectedly", async () => {
    const providerDispose = vi.fn();
    const primaryUri = PRIMARY_DOCUMENT_URI;
    const placeholderUri = REFERENCED_DOCUMENT_URI;
    const primaryModelUri = modelUri(primaryUri);
    const placeholderModelUri = modelUri(placeholderUri);
    const { models, monaco } = createMonacoHarness([primaryModelUri]);
    mocks.registerLspProviders.mockReturnValue([{ dispose: providerDispose }]);

    const socket = await connectReady("session", WORKSPACE_PATH);
    createPlaceholder(0, placeholderUri);
    const placeholder = requireModel(models, placeholderModelUri);

    socket.failClosed(1006, "language server crashed");

    expect(providerDispose).toHaveBeenCalledOnce();
    expect(mocks.setBuiltinTsSuppressed).toHaveBeenLastCalledWith(false);
    expect(placeholder.dispose).toHaveBeenCalledOnce();
    expect(monaco.editor.setModelMarkers).toHaveBeenCalledWith(
      requireModel(models, primaryModelUri),
      expect.stringMatching(/^lsp:session:typescript:\d+$/),
      [],
    );
  });

  it("isolates editor state for sessions that share task-host paths", async () => {
    let builtinTsSuppressed = false;
    mocks.setBuiltinTsSuppressed.mockImplementation((suppressed: boolean) => {
      builtinTsSuppressed = suppressed;
    });

    const documentUri = PRIMARY_DOCUMENT_URI;
    const firstModelUri = modelUri(documentUri, FIRST_SESSION_ID);
    const secondModelUri = modelUri(documentUri, SECOND_SESSION_ID);
    const { markersByUri, models } = createMonacoHarness([firstModelUri, secondModelUri]);
    mocks.registerLspProviders.mockReturnValue([]);

    const firstSocket = await connectReady(FIRST_SESSION_ID, WORKSPACE_PATH);
    const secondSocket = await connectReady(SECOND_SESSION_ID, WORKSPACE_PATH);
    expect(mocks.setBuiltinTsSuppressed.mock.calls).toEqual([[true]]);

    const firstProvider = mocks.registerLspProviders.mock.calls[0][0] as {
      getDocumentUri: (model: TestModel) => string | null;
    };
    const secondProvider = mocks.registerLspProviders.mock.calls[1][0] as {
      getDocumentUri: (model: TestModel) => string | null;
    };
    expect(firstProvider.getDocumentUri(requireModel(models, firstModelUri))).toBe(documentUri);
    expect(firstProvider.getDocumentUri(requireModel(models, secondModelUri))).toBeNull();
    expect(secondProvider.getDocumentUri(requireModel(models, firstModelUri))).toBeNull();
    expect(secondProvider.getDocumentUri(requireModel(models, secondModelUri))).toBe(documentUri);

    const placeholderUri = REFERENCED_DOCUMENT_URI;
    const firstPlaceholderModelUri = modelUri(placeholderUri, FIRST_SESSION_ID);
    const secondPlaceholderModelUri = modelUri(placeholderUri, SECOND_SESSION_ID);
    createPlaceholder(0, placeholderUri);
    createPlaceholder(1, placeholderUri);
    const firstPlaceholder = requireModel(models, firstPlaceholderModelUri);
    const secondPlaceholder = requireModel(models, secondPlaceholderModelUri);
    publishDiagnostic(firstSocket, documentUri, "first issue");
    publishDiagnostic(secondSocket, documentUri, "second issue");

    firstSocket.failClosed(1006, "first language server crashed");

    expect(firstPlaceholder.dispose).toHaveBeenCalledOnce();
    expect(secondPlaceholder.dispose).not.toHaveBeenCalled();
    expect(models.has(secondPlaceholderModelUri)).toBe(true);
    expect(markerMessages(markersByUri, firstModelUri)).not.toContain("first issue");
    expect(markerMessages(markersByUri, secondModelUri)).toContain("second issue");
    expect(builtinTsSuppressed).toBe(true);
    expect(mocks.setBuiltinTsSuppressed).not.toHaveBeenCalledWith(false);

    secondSocket.failClosed(1006, "second language server crashed");

    expect(secondPlaceholder.dispose).toHaveBeenCalledOnce();
    expect(builtinTsSuppressed).toBe(false);
  });
});

describe("LSP workspace handshake", () => {
  it("uses the task-host URI and repository list instead of host path metadata", async () => {
    createMonacoHarness([]);
    mocks.registerLspProviders.mockReturnValue([]);
    lspClientManager.connect("session", "typescript");
    const socket = FakeWebSocket.instances.at(-1);
    if (!socket) throw new Error(EXPECTED_SOCKET_ERROR);
    socket.open();
    socket.emitMessage(
      JSON.stringify({
        status: "ready",
        workspacePath: "/host/tmp/worktree",
        workspaceUri: WORKSPACE_URI,
        repoSubpaths: ["backend"],
      }),
    );

    const initialize = JSON.parse(socket.sent[0]) as {
      id: number;
      params: { rootUri: string; workspaceFolders: Array<{ uri: string }> };
    };
    expect(initialize.params.rootUri).toBe(WORKSPACE_URI);
    expect(initialize.params.workspaceFolders).toEqual([{ uri: WORKSPACE_URI, name: "worktree" }]);
    socket.emitMessage(
      JSON.stringify({ jsonrpc: "2.0", id: initialize.id, result: { capabilities: {} } }),
    );
    await vi.waitFor(() => {
      expect(lspClientManager.getStatus("session", "typescript")).toEqual({ state: "ready" });
    });
    expect(lspClientManager.getWorkspaceUriForSession("session")).toBe(WORKSPACE_URI);
    expect(lspClientManager.getRepositorySubpaths("session")).toEqual(["backend"]);

    lspClientManager.stop("session", "typescript");
    expect(lspClientManager.getWorkspaceUriForSession("session")).toBe(WORKSPACE_URI);
    expect(lspClientManager.getRepositorySubpaths("session")).toEqual(["backend"]);
  });

  it("falls back to the task-host path when its URI field is malformed", () => {
    createMonacoHarness([]);
    mocks.registerLspProviders.mockReturnValue([]);
    lspClientManager.connect("session", "typescript");
    const socket = FakeWebSocket.instances.at(-1);
    if (!socket) throw new Error(EXPECTED_SOCKET_ERROR);
    socket.open();
    socket.emitMessage(
      JSON.stringify({
        status: "ready",
        workspacePath: WORKSPACE_PATH,
        workspaceUri: "https://invalid.example/workspace",
      }),
    );

    const initialize = JSON.parse(socket.sent[0]) as { params: { rootUri: string } };
    expect(initialize.params.rootUri).toBe(WORKSPACE_URI);
  });
});

describe("LSP document subscriptions", () => {
  it("shares synchronization for duplicate logical views of one canonical URI", async () => {
    const documentUri = "file:///workspace/backend/src/Main.ts";
    createMonacoHarness([modelUri(documentUri)]);
    mocks.registerLspProviders.mockReturnValue([]);
    const socket = await connectReady("session", WORKSPACE_PATH);
    const document = {
      uri: documentUri,
      languageId: "typescript",
      text: "export const value = 1;",
    };

    lspClientManager.openDocument("session", "typescript", document);
    lspClientManager.openDocument("session", "typescript", { ...document, repo: "backend" });

    const notificationCount = (method: string) =>
      socket.sent.filter((frame) => JSON.parse(frame).method === method).length;
    expect(notificationCount("textDocument/didOpen")).toBe(1);

    lspClientManager.changeDocument(
      "session",
      "typescript",
      documentUri,
      "export const value = 2;",
    );
    lspClientManager.changeDocument(
      "session",
      "typescript",
      documentUri,
      "export const value = 2;",
    );
    expect(notificationCount("textDocument/didChange")).toBe(1);

    lspClientManager.closeDocument("session", "typescript", documentUri);
    expect(notificationCount("textDocument/didClose")).toBe(0);
    lspClientManager.closeDocument("session", "typescript", documentUri);
    expect(notificationCount("textDocument/didClose")).toBe(1);
  });
});

describe("LSP diagnostic URI identity", () => {
  it("matches Windows URI casing while keeping POSIX paths case-sensitive", async () => {
    const windowsDocumentUri = "file:///C:/TaskRoot/src/Main.ts";
    const posixDocumentUri = PRIMARY_DOCUMENT_URI;
    const windowsModelUri = modelUri(windowsDocumentUri, WINDOWS_SESSION_ID);
    const posixModelUri = modelUri(posixDocumentUri, "posix-session");
    const { markersByUri } = createMonacoHarness([windowsModelUri, posixModelUri]);
    mocks.registerLspProviders.mockReturnValue([]);

    const windowsSocket = await connectReady(WINDOWS_SESSION_ID, "C:\\TaskRoot");
    publishDiagnostic(windowsSocket, "file:///c:/taskroot/SRC/main.ts", "windows issue");
    expect(markerMessages(markersByUri, windowsModelUri)).toContain("windows issue");

    const posixSocket = await connectReady("posix-session", WORKSPACE_PATH);
    publishDiagnostic(posixSocket, `${WORKSPACE_URI}/main.ts`, "wrong-case POSIX issue");
    expect(markerMessages(markersByUri, posixModelUri)).not.toContain("wrong-case POSIX issue");
  });

  it.each([
    {
      sessionId: WINDOWS_SESSION_ID,
      workspacePath: "C:\\TaskRoot",
      documentUri: "file:///C:/TaskRoot/src/Main.ts",
      serverUri: "file:///c:/taskroot/SRC/main.ts",
      targetDocumentUri: "file:///C:/TaskRoot/src/Definition.ts",
      targetServerUri: "file:///c:/taskroot/SRC/definition.ts",
    },
    {
      sessionId: "unc-session",
      workspacePath: "\\\\build-server\\Work",
      documentUri: "file://build-server/Work/src/Main.ts",
      serverUri: "file://BUILD-SERVER/work/SRC/main.ts",
      targetDocumentUri: "file://build-server/Work/src/Definition.ts",
      targetServerUri: "file://BUILD-SERVER/work/SRC/definition.ts",
    },
  ])("reuses an existing $sessionId model for a server case variant", async (testCase) => {
    const existingModelUri = modelUri(testCase.documentUri, testCase.sessionId);
    const { models, monaco } = createMonacoHarness([existingModelUri]);
    mocks.registerLspProviders.mockReturnValue([]);

    await connectReady(testCase.sessionId, testCase.workspacePath);
    const provider = mocks.registerLspProviders.mock.calls[0][0] as {
      ensureModelsExist: (uris: string[]) => void;
      getModelUri: (uri: string) => string | null;
    };
    provider.ensureModelsExist([testCase.serverUri]);

    expect(models.size).toBe(1);
    expect(provider.getModelUri(testCase.serverUri)).toBe(existingModelUri);

    provider.ensureModelsExist([testCase.targetServerUri]);
    const placeholderUri = modelUri(testCase.targetServerUri, testCase.sessionId);
    const placeholder = requireModel(models, placeholderUri);
    const realModelUri = modelUri(testCase.targetDocumentUri, testCase.sessionId);
    const realModel = monaco.editor.createModel("", "typescript", monaco.Uri.parse(realModelUri));

    lspClientManager.promoteDocumentModel(
      testCase.sessionId,
      testCase.targetDocumentUri,
      "authoritative content",
    );

    expect(placeholder.dispose).toHaveBeenCalledOnce();
    expect(models.has(placeholderUri)).toBe(false);
    expect(realModel.setValue).toHaveBeenCalledWith("authoritative content");
    expect(provider.getModelUri(testCase.targetServerUri)).toBe(realModelUri);
  });
});

describe("LSP replacement connection cleanup", () => {
  it("ignores a delayed release from a replaced connection", async () => {
    createMonacoHarness([]);
    const currentProviderDispose = vi.fn();
    mocks.registerLspProviders
      .mockReturnValueOnce([])
      .mockReturnValueOnce([{ dispose: currentProviderDispose }]);

    const { socket: oldSocket, release: releaseOldConnection } = await connectReadyWithRelease(
      REPLACEMENT_SESSION_ID,
      "/old",
    );
    oldSocket.readyState = FakeWebSocket.CLOSING;
    const currentSocket = await connectReady(REPLACEMENT_SESSION_ID, "/replacement");

    vi.useFakeTimers();
    try {
      releaseOldConnection();
      await vi.advanceTimersByTimeAsync(2 * 60 * 1000 + 1);

      expect(currentSocket.readyState).toBe(FakeWebSocket.OPEN);
      expect(currentProviderDispose).not.toHaveBeenCalled();
      expect(lspClientManager.getStatus(REPLACEMENT_SESSION_ID, "typescript")).toEqual({
        state: "ready",
      });
    } finally {
      oldSocket.failClosed(1000, "old connection released");
      vi.useRealTimers();
    }
  });

  it("ignores a delayed close from a replaced TypeScript connection", async () => {
    let builtinTsSuppressed = false;
    mocks.setBuiltinTsSuppressed.mockImplementation((suppressed: boolean) => {
      builtinTsSuppressed = suppressed;
    });
    const oldDocumentUri = "file:///old/Main.ts";
    const currentDocumentUri = "file:///replacement/Main.ts";
    const oldModelUri = modelUri(oldDocumentUri, REPLACEMENT_SESSION_ID);
    const currentModelUri = modelUri(currentDocumentUri, REPLACEMENT_SESSION_ID);
    const currentPlaceholderUri = "file:///replacement/Referenced.ts";
    const currentPlaceholderModelUri = modelUri(currentPlaceholderUri, REPLACEMENT_SESSION_ID);
    const { markersByUri, models } = createMonacoHarness([oldModelUri, currentModelUri]);
    const oldProviderDispose = vi.fn();
    const currentProviderDispose = vi.fn();
    mocks.registerLspProviders
      .mockReturnValueOnce([{ dispose: oldProviderDispose }])
      .mockReturnValueOnce([{ dispose: currentProviderDispose }]);

    const oldSocket = await connectReady(REPLACEMENT_SESSION_ID, "/old");
    publishDiagnostic(oldSocket, oldDocumentUri, "old issue");
    expect(markerMessages(markersByUri, oldModelUri)).toContain("old issue");
    oldSocket.readyState = FakeWebSocket.CLOSING;
    const currentSocket = await connectReady(REPLACEMENT_SESSION_ID, "/replacement");

    expect(oldProviderDispose).toHaveBeenCalledOnce();
    expect(markerMessages(markersByUri, oldModelUri)).not.toContain("old issue");
    createPlaceholder(1, currentPlaceholderUri);
    const currentPlaceholder = requireModel(models, currentPlaceholderModelUri);
    publishDiagnostic(currentSocket, currentDocumentUri, "current issue");

    oldSocket.failClosed(1006, "old language server finished closing");

    expect(oldProviderDispose).toHaveBeenCalledOnce();
    expect(currentProviderDispose).not.toHaveBeenCalled();
    expect(currentPlaceholder.dispose).not.toHaveBeenCalled();
    expect(markerMessages(markersByUri, currentModelUri)).toContain("current issue");
    expect(lspClientManager.getStatus(REPLACEMENT_SESSION_ID, "typescript")).toEqual({
      state: "ready",
    });
    expect(builtinTsSuppressed).toBe(true);
  });

  it("ignores delayed initialization from a replaced connection", async () => {
    createMonacoHarness([]);
    mocks.registerLspProviders.mockReturnValue([]);

    const { socket: oldSocket, initializeId: oldInitializeId } = beginInitialization(
      REPLACEMENT_SESSION_ID,
      { workspacePath: "/old" },
    );
    oldSocket.readyState = FakeWebSocket.CLOSING;

    await connectReady(REPLACEMENT_SESSION_ID, "/replacement");
    oldSocket.emitMessage(
      JSON.stringify({
        jsonrpc: "2.0",
        id: oldInitializeId,
        result: { capabilities: {} },
      }),
    );
    await Promise.resolve();

    expect(mocks.registerLspProviders).toHaveBeenCalledOnce();
    expect(mocks.setBuiltinTsSuppressed.mock.calls).toEqual([[true]]);
    expect(lspClientManager.getStatus(REPLACEMENT_SESSION_ID, "typescript")).toEqual({
      state: "ready",
    });
  });
});

describe("LSP shared placeholder cleanup", () => {
  it("loads encoded placeholders through the correct repository scope", async () => {
    const currentUri = "file:///workspace/backend/src/Main.ts";
    const definitionUri = "file:///workspace/backend/src/My%20Definition%23.ts";
    mocks.getWebSocketClient.mockReturnValue({});
    mocks.requestFileContent.mockResolvedValue({ content: "export const target = true;" });
    mocks.registerLspProviders.mockReturnValue([]);
    createMonacoHarness([modelUri(currentUri)]);

    await connectReady("session", WORKSPACE_PATH);
    lspClientManager.openDocument("session", "typescript", {
      uri: currentUri,
      languageId: "typescript",
      text: "export {};",
      repo: "backend",
    });
    createPlaceholder(0, definitionUri);

    await vi.waitFor(() => {
      expect(mocks.requestFileContent).toHaveBeenCalledWith(
        expect.anything(),
        "session",
        "src/My Definition#.ts",
        "backend",
      );
    });
  });

  it("does not create or fetch placeholders outside the workspace boundary", async () => {
    const outsideUri = "file:///workspace-other/Secret.ts";
    mocks.getWebSocketClient.mockReturnValue({});
    mocks.registerLspProviders.mockReturnValue([]);
    const { models } = createMonacoHarness([]);

    await connectReady("session", WORKSPACE_PATH);
    createPlaceholder(0, outsideUri);

    expect(models.has(outsideUri)).toBe(false);
    expect(mocks.requestFileContent).not.toHaveBeenCalled();
  });

  it("promotes a placeholder for a real editor without a matching LSP", async () => {
    const documentUri = `${WORKSPACE_URI}/NormalOpen.java`;
    const staleContent = createDeferred<{ content: string }>();
    mocks.getWebSocketClient.mockReturnValue({});
    mocks.requestFileContent.mockReturnValue(staleContent.promise);
    mocks.registerLspProviders.mockReturnValue([]);
    const { models } = createMonacoHarness([]);

    const socket = await connectReady(SESSION_ID, WORKSPACE_PATH);
    createPlaceholder(0, documentUri);
    await vi.waitFor(() => expect(mocks.requestFileContent).toHaveBeenCalledOnce());
    const placeholder = requireModel(models, modelUri(documentUri));

    lspClientManager.promoteDocumentModel(SESSION_ID, documentUri, "authoritative editor content");
    staleContent.resolve({ content: "stale placeholder content" });
    await Promise.resolve();
    await Promise.resolve();

    expect(placeholder.setValue).toHaveBeenCalledWith("authoritative editor content");
    expect(placeholder.setValue).not.toHaveBeenCalledWith("stale placeholder content");
    socket.failClosed(1006, "language server crashed");
    expect(placeholder.dispose).not.toHaveBeenCalled();
  });

  it("isolates same-path placeholder models and content between task sessions", async () => {
    const sharedUri = "file:///workspace/Shared.ts";
    const firstContent = createDeferred<{ content: string }>();
    const secondContent = createDeferred<{ content: string }>();
    mocks.getWebSocketClient.mockReturnValue({});
    mocks.requestFileContent
      .mockReturnValueOnce(firstContent.promise)
      .mockReturnValueOnce(secondContent.promise);
    mocks.registerLspProviders.mockReturnValue([]);
    const { models } = createMonacoHarness([]);

    const firstSocket = await connectReady(FIRST_SESSION_ID, WORKSPACE_PATH);
    await connectReady(SECOND_SESSION_ID, WORKSPACE_PATH);
    createPlaceholder(0, sharedUri);
    await vi.waitFor(() => expect(mocks.requestFileContent).toHaveBeenCalledOnce());
    createPlaceholder(1, sharedUri);
    await vi.waitFor(() => expect(mocks.requestFileContent).toHaveBeenCalledTimes(2));
    const firstModel = requireModel(models, modelUri(sharedUri, FIRST_SESSION_ID));
    const secondModel = requireModel(models, modelUri(sharedUri, SECOND_SESSION_ID));
    expect(firstModel).not.toBe(secondModel);

    firstSocket.failClosed(1006, "first language server crashed");
    firstContent.resolve({ content: "stale first-session content" });
    secondContent.resolve({ content: "surviving second-session content" });

    await vi.waitFor(() => {
      expect(secondModel.setValue).toHaveBeenCalledWith("surviving second-session content");
    });
    expect(firstModel.setValue).not.toHaveBeenCalledWith("stale first-session content");
    expect(firstModel.dispose).toHaveBeenCalledOnce();
    expect(secondModel.setValue).not.toHaveBeenCalledWith("stale first-session content");
    expect(secondModel.dispose).not.toHaveBeenCalled();
  });
});
