import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createLspManagerHarness,
  FakeWebSocket,
  markerMessages,
  publishDiagnostic,
} from "./lsp-client-manager.test-harness";
import { LSP_IDLE_TIMEOUT } from "./lsp-client-config";
import { modelUriForDocument } from "./file-uri";

const mocks = vi.hoisted(() => ({
  getMonacoInstance: vi.fn(),
  waitForMonacoInstance: vi.fn(),
  getWebSocketClient: vi.fn(),
  registerLspProviders: vi.fn(),
  requestFileContent: vi.fn(),
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
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: mocks.getWebSocketClient }));
vi.mock("@/lib/ws/workspace-files", () => ({ requestFileContent: mocks.requestFileContent }));

import { lspClientManager } from "./lsp-client-manager";

const { createMonacoHarness } = createLspManagerHarness(lspClientManager, mocks);
const SESSION_ID = "continuity-session";
const WORKSPACE_URI = "file:///workspace";
const DOCUMENT_URI = `${WORKSPACE_URI}/Main.ts`;
const DOCUMENT_MODEL_URI = modelUriForDocument(DOCUMENT_URI, SESSION_ID);

function initializeRequest(socket: FakeWebSocket): { id: number; method: string } {
  const request = socket.sent
    .map((frame) => JSON.parse(frame) as { id?: number; method?: string })
    .find((message) => message.method === "initialize");
  if (request?.id === undefined || request.method === undefined) {
    throw new Error("expected the LSP initialize request");
  }
  return { id: request.id, method: request.method };
}

function latestControl(
  socket: FakeWebSocket,
  action: string,
): { action: string; requestId: string } {
  const control = socket.sent
    .map((frame) => JSON.parse(frame) as { action?: string; requestId?: string })
    .find((message) => message.action === action);
  if (!control?.requestId) throw new Error(`expected LSP ${action} control`);
  return { action, requestId: control.requestId };
}

function sendResumedHandshake(socket: FakeWebSocket): void {
  socket.open();
  socket.emitMessage(
    JSON.stringify({
      status: "ready",
      leaseId: "lease-1",
      resumed: true,
      workspacePath: "/workspace",
      workspaceUri: WORKSPACE_URI,
      repoSubpaths: [],
      capabilities: { definitionProvider: true },
      registrations: [],
      progressTokens: [],
      progress: [],
    }),
  );
}

async function waitForDocumentSync(socket: FakeWebSocket): Promise<void> {
  await vi.waitFor(() => {
    expect(socket.sent.some((frame) => JSON.parse(frame).method === "textDocument/didOpen")).toBe(
      true,
    );
  });
}

function acknowledgeAttachment(socket: FakeWebSocket): void {
  const acknowledgement = latestControl(socket, "attachmentReady");
  socket.emitMessage(
    JSON.stringify({
      kandev: "lsp",
      action: "attachmentReady",
      requestId: acknowledgement.requestId,
    }),
  );
}

async function connectContinuityReady() {
  const disconnect = lspClientManager.connect(SESSION_ID, "typescript", undefined, true);
  const socket = FakeWebSocket.instances.at(-1);
  if (!socket) throw new Error("expected an LSP WebSocket");
  socket.open();
  socket.emitMessage(
    JSON.stringify({
      status: "ready",
      leaseId: "lease-1",
      resumed: false,
      workspacePath: "/workspace",
      workspaceUri: WORKSPACE_URI,
      repoSubpaths: [],
    }),
  );
  const initialize = initializeRequest(socket);
  socket.emitMessage(
    JSON.stringify({
      jsonrpc: "2.0",
      id: initialize.id,
      result: { capabilities: { definitionProvider: true } },
    }),
  );
  await vi.waitFor(() =>
    expect(lspClientManager.getStatus(SESSION_ID, "typescript")).toEqual({ state: "ready" }),
  );
  return { disconnect, socket };
}

beforeEach(() => {
  lspClientManager.disconnectAll();
  FakeWebSocket.instances = [];
  vi.resetAllMocks();
  vi.stubGlobal("WebSocket", FakeWebSocket);
  mocks.waitForMonacoInstance.mockImplementation(async () => mocks.getMonacoInstance());
  mocks.registerBuiltinTsSuppression.mockReturnValue({ dispose: vi.fn() });
  sessionStorage.clear();
});

afterEach(() => {
  lspClientManager.disconnectAll();
  sessionStorage.clear();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("LSP browser continuity", () => {
  it("reattaches without initializing again and waits for current document synchronization", async () => {
    const { markersByUri } = createMonacoHarness([DOCUMENT_MODEL_URI]);
    mocks.registerLspProviders.mockReturnValue([]);
    const { socket: firstSocket } = await connectContinuityReady();
    lspClientManager.openDocument(SESSION_ID, "typescript", {
      uri: DOCUMENT_URI,
      languageId: "typescript",
      text: "export const current = true;",
    });

    firstSocket.failClosed(4009, "browser transport lost");

    expect(lspClientManager.getStatus(SESSION_ID, "typescript")).toEqual({ state: "reconnecting" });
    expect(markerMessages(markersByUri, DOCUMENT_MODEL_URI)).toEqual([]);
    await new Promise((resolve) => setTimeout(resolve, 275));
    const resumedSocket = FakeWebSocket.instances.at(-1);
    if (!resumedSocket || resumedSocket === firstSocket)
      throw new Error("expected a resumed socket");
    sendResumedHandshake(resumedSocket);

    await waitForDocumentSync(resumedSocket);
    expect(resumedSocket.sent.some((frame) => JSON.parse(frame).method === "initialize")).toBe(
      false,
    );
    expect(mocks.registerLspProviders).toHaveBeenCalledTimes(2);
    expect(lspClientManager.getStatus(SESSION_ID, "typescript")).toEqual({ state: "reconnecting" });

    publishDiagnostic(resumedSocket, DOCUMENT_URI, "too early");
    expect(markerMessages(markersByUri, DOCUMENT_MODEL_URI)).toEqual([]);
    acknowledgeAttachment(resumedSocket);
    await vi.waitFor(() =>
      expect(lspClientManager.getStatus(SESSION_ID, "typescript")).toEqual({ state: "ready" }),
    );

    publishDiagnostic(resumedSocket, DOCUMENT_URI, "fresh diagnostic");
    expect(markerMessages(markersByUri, DOCUMENT_MODEL_URI)).toContain("fresh diagnostic");
  });
});

describe("LSP initialization continuation", () => {
  it("finishes the one-time initialized notification when resuming mid-handshake", async () => {
    createMonacoHarness([]);
    mocks.registerLspProviders.mockReturnValue([]);
    const { socket: firstSocket } = await connectContinuityReady();
    firstSocket.failClosed(4009, "browser transport lost");
    await new Promise((resolve) => setTimeout(resolve, 275));
    const resumedSocket = FakeWebSocket.instances.at(-1);
    if (!resumedSocket || resumedSocket === firstSocket)
      throw new Error("expected a resumed socket");
    resumedSocket.open();
    resumedSocket.emitMessage(
      JSON.stringify({
        status: "ready",
        leaseId: "lease-1",
        resumed: true,
        initialized: false,
        workspacePath: "/workspace",
        workspaceUri: WORKSPACE_URI,
        repoSubpaths: [],
        capabilities: { definitionProvider: true },
        registrations: [],
        progressTokens: [],
        progress: [],
      }),
    );
    await vi.waitFor(() =>
      expect(resumedSocket.sent.some((frame) => JSON.parse(frame).method === "initialized")).toBe(
        true,
      ),
    );
    expect(
      resumedSocket.sent.findIndex((frame) => JSON.parse(frame).method === "initialized"),
    ).toBeLessThan(
      resumedSocket.sent.findIndex((frame) => JSON.parse(frame).action === "attachmentReady"),
    );
    expect(resumedSocket.sent.some((frame) => JSON.parse(frame).method === "initialize")).toBe(
      false,
    );
  });
});

describe("LSP browser continuity controls", () => {
  it("treats confirmed process exit as actionable and does not retry", async () => {
    createMonacoHarness([]);
    mocks.registerLspProviders.mockReturnValue([]);
    const { socket } = await connectContinuityReady();

    socket.failClosed(4006, "language server exited");

    expect(lspClientManager.getStatus(SESSION_ID, "typescript")).toEqual({
      state: "error",
      reason: "language server exited",
    });
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(sessionStorage.getItem(`kandev-lsp-lease:${SESSION_ID}:typescript`)).toBeNull();
  });

  it("releases its window lease only after explicit Stop is acknowledged", async () => {
    createMonacoHarness([]);
    mocks.registerLspProviders.mockReturnValue([]);
    const { socket } = await connectContinuityReady();

    lspClientManager.stop(SESSION_ID, "typescript");

    expect(lspClientManager.getStatus(SESSION_ID, "typescript")).toEqual({ state: "stopping" });
    const release = latestControl(socket, "release");
    socket.emitMessage(
      JSON.stringify({ kandev: "lsp", action: "released", requestId: release.requestId }),
    );
    await vi.waitFor(() =>
      expect(lspClientManager.getStatus(SESSION_ID, "typescript")).toEqual({ state: "disabled" }),
    );
    expect(sessionStorage.getItem(`kandev-lsp-lease:${SESSION_ID}:typescript`)).toBeNull();
  });

  it("releases after the last editor idle timeout", async () => {
    vi.useFakeTimers();
    createMonacoHarness([]);
    mocks.registerLspProviders.mockReturnValue([]);
    const { socket, disconnect } = await connectContinuityReady();

    disconnect();
    await vi.advanceTimersByTimeAsync(LSP_IDLE_TIMEOUT);

    const release = latestControl(socket, "release");
    expect(
      JSON.parse(socket.sent.find((frame) => JSON.parse(frame).action === "release") ?? "null"),
    ).toMatchObject({
      reason: "editor_idle",
    });
    socket.emitMessage(
      JSON.stringify({ kandev: "lsp", action: "released", requestId: release.requestId }),
    );
    await Promise.resolve();
    expect(sessionStorage.getItem(`kandev-lsp-lease:${SESSION_ID}:typescript`)).toBeNull();
  });
});
