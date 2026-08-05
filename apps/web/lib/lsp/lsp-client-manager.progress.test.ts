import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { activateLocale } from "@/lib/i18n";
import { createLspManagerHarness, FakeWebSocket } from "./lsp-client-manager.test-harness";
import { LSP_IDLE_TIMEOUT } from "./lsp-client-config";
import type { LspProgressToken } from "./lsp-progress";

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

vi.mock("./lsp-providers", () => ({
  registerLspProviders: mocks.registerLspProviders,
}));

import { lspClientManager } from "./lsp-client-manager";

const { createMonacoHarness } = createLspManagerHarness(lspClientManager, mocks);
const SESSION_ID = "progress-session";
const REPLACEMENT_SESSION_ID = "progress-replacement-session";
const ERROR_SESSION_ID = "progress-error-session";
const INSTALL_SESSION_ID = "progress-install-session";
const LANGUAGE = "typescript";
const WORKSPACE_PATH = "/workspace";
const PROGRESS_METHOD = "$/progress";
const EXPECTED_SOCKET_ERROR = "expected an LSP WebSocket";

type InitializeRequest = {
  id: number;
  params: {
    capabilities: { window?: { workDoneProgress?: boolean } };
    workDoneToken: LspProgressToken;
  };
};

function beginInitialization(
  sessionId = SESSION_ID,
  workspacePath = WORKSPACE_PATH,
): { initialize: InitializeRequest; release: () => void; socket: FakeWebSocket } {
  const release = lspClientManager.connect(sessionId, LANGUAGE);
  const socket = FakeWebSocket.instances.at(-1);
  if (!socket) throw new Error(EXPECTED_SOCKET_ERROR);
  socket.open();
  socket.emitMessage(JSON.stringify({ status: "ready", workspacePath }));
  return {
    initialize: JSON.parse(socket.sent[0]) as InitializeRequest,
    release,
    socket,
  };
}

function completeInitialization(socket: FakeWebSocket, id: number): void {
  socket.emitMessage(JSON.stringify({ jsonrpc: "2.0", id, result: { capabilities: {} } }));
}

function emitProgress(socket: FakeWebSocket, token: LspProgressToken, value: unknown): void {
  socket.emitMessage(
    JSON.stringify({
      jsonrpc: "2.0",
      method: PROGRESS_METHOD,
      params: { token, value },
    }),
  );
}

beforeEach(() => {
  lspClientManager.disconnectAll();
  FakeWebSocket.instances = [];
  vi.resetAllMocks();
  vi.stubGlobal("WebSocket", FakeWebSocket);
  const { monaco } = createMonacoHarness([]);
  mocks.waitForMonacoInstance.mockResolvedValue(monaco);
  mocks.registerLspProviders.mockReturnValue([]);
  mocks.registerBuiltinTsSuppression.mockReturnValue({ dispose: vi.fn() });
});

afterEach(() => {
  lspClientManager.disconnectAll();
  vi.unstubAllGlobals();
});

describe("LSP connection failure reporting", () => {
  it("preserves detailed install failures when the socket closes", () => {
    lspClientManager.connect(SESSION_ID, LANGUAGE);
    const socket = FakeWebSocket.instances.at(-1);
    if (!socket) throw new Error(EXPECTED_SOCKET_ERROR);
    socket.open();
    socket.emitMessage(
      JSON.stringify({ status: "install_failed", error: "npm install failed: registry offline" }),
    );

    socket.failClosed(4003, "install failed");

    expect(lspClientManager.getStatus(SESSION_ID, LANGUAGE)).toEqual({
      state: "error",
      reason: "npm install failed: registry offline",
    });
  });

  it("reports a bridge close while initialization is pending", async () => {
    const { socket } = beginInitialization();

    socket.failClosed(1006, "language server crashed during initialize");

    await vi.waitFor(() => {
      expect(lspClientManager.getStatus(SESSION_ID, LANGUAGE)).toEqual({
        state: "error",
        reason: "language server crashed during initialize",
      });
    });
  });

  it("preserves the message from a JSON-RPC initialize error", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    const { initialize, socket } = beginInitialization();

    socket.emitMessage(
      JSON.stringify({
        jsonrpc: "2.0",
        id: initialize.id,
        error: { code: -32603, message: "Gradle project import failed" },
      }),
    );

    await vi.waitFor(() => {
      expect(lspClientManager.getStatus(SESSION_ID, LANGUAGE)).toEqual({
        state: "error",
        reason: "Gradle project import failed",
      });
    });
    expect(socket.readyState).toBe(FakeWebSocket.CLOSED);
    consoleError.mockRestore();
  });

  it("describes an unexpected bridge close when the proxy omits its reason", async () => {
    const { initialize, socket } = beginInitialization();
    completeInitialization(socket, initialize.id);
    await vi.waitFor(() => {
      expect(lspClientManager.getStatus(SESSION_ID, LANGUAGE)).toEqual({ state: "ready" });
    });

    socket.failClosed(1000, "");

    expect(lspClientManager.getStatus(SESSION_ID, LANGUAGE)).toEqual({
      state: "error",
      reason: "language server exited",
    });
  });
});

describe("LSP connection failure localization", () => {
  it("localizes fallback reasons when the proxy omits its close reason", async () => {
    await activateLocale("pseudo");
    try {
      lspClientManager.connect(INSTALL_SESSION_ID, LANGUAGE);
      const installSocket = FakeWebSocket.instances.at(-1);
      if (!installSocket) throw new Error(EXPECTED_SOCKET_ERROR);
      installSocket.emitMessage(JSON.stringify({ status: "install_failed" }));

      expect(lspClientManager.getStatus(INSTALL_SESSION_ID, LANGUAGE)).toEqual({
        state: "error",
        reason: "Ĩńśţàĺĺ ƒàĩĺēď",
      });

      lspClientManager.connect(SESSION_ID, LANGUAGE);
      const startingSocket = FakeWebSocket.instances.at(-1);
      if (!startingSocket) throw new Error(EXPECTED_SOCKET_ERROR);
      startingSocket.open();
      startingSocket.failClosed(1006, "");

      expect(lspClientManager.getStatus(SESSION_ID, LANGUAGE)).toEqual({
        state: "error",
        reason: "Ćōńńēćţĩōń ćĺōśēď",
      });

      lspClientManager.connect(ERROR_SESSION_ID, LANGUAGE);
      const errorSocket = FakeWebSocket.instances.at(-1);
      if (!errorSocket) throw new Error(EXPECTED_SOCKET_ERROR);
      errorSocket.onerror?.(new Event("error"));

      expect(lspClientManager.getStatus(ERROR_SESSION_ID, LANGUAGE)).toEqual({
        state: "error",
        reason: "ŴēƀŚōćķēţ ēŕŕōŕ",
      });

      const { initialize, socket } = beginInitialization(REPLACEMENT_SESSION_ID);
      completeInitialization(socket, initialize.id);
      await vi.waitFor(() => {
        expect(lspClientManager.getStatus(REPLACEMENT_SESSION_ID, LANGUAGE)).toEqual({
          state: "ready",
        });
      });
      socket.failClosed(1000, "");

      expect(lspClientManager.getStatus(REPLACEMENT_SESSION_ID, LANGUAGE)).toEqual({
        state: "error",
        reason: "ĺàńĝũàĝē śēŕvēŕ ēxĩţēď",
      });
    } finally {
      await activateLocale("en");
    }
  });
});

describe("LSP progress handshake and initialization", () => {
  it("advertises work-done support and tracks initialize until the response", async () => {
    const { initialize, socket } = beginInitialization();

    expect(initialize.params.capabilities.window?.workDoneProgress).toBe(true);
    expect(initialize.params.workDoneToken).toEqual(expect.any(String));
    expect(lspClientManager.getProgress(SESSION_ID, LANGUAGE)).toEqual({
      initializingSince: expect.any(Number),
      active: [],
      completed: null,
      hasReportedProgress: false,
    });

    completeInitialization(socket, initialize.id);
    await Promise.resolve();

    expect(lspClientManager.getProgress(SESSION_ID, LANGUAGE).initializingSince).toBeNull();
  });

  it("accepts initialize-token progress before initialize completes and notifies subscribers", () => {
    const listener = vi.fn();
    const unsubscribe = lspClientManager.onChange(listener);
    const { initialize, socket } = beginInitialization();
    listener.mockClear();

    emitProgress(socket, initialize.params.workDoneToken, {
      kind: "begin",
      title: "Importing Kotlin project",
      message: "Resolving Gradle modules",
      percentage: 35,
    });

    expect(lspClientManager.getProgress(SESSION_ID, LANGUAGE).active).toEqual([
      {
        token: initialize.params.workDoneToken,
        title: "Importing Kotlin project",
        message: "Resolving Gradle modules",
        percentage: 35,
        startedAt: expect.any(Number),
      },
    ]);
    expect(listener).toHaveBeenCalledWith(`${SESSION_ID}:${LANGUAGE}`);
    unsubscribe();
  });
});

describe("LSP progress token and generation ownership", () => {
  it("registers numeric server-created tokens and records their completion", () => {
    const { socket } = beginInitialization();

    socket.emitMessage(
      JSON.stringify({
        jsonrpc: "2.0",
        id: 91,
        method: "window/workDoneProgress/create",
        params: { token: 7 },
      }),
    );
    emitProgress(socket, 7, { kind: "begin", title: "Analyzing dependencies" });
    emitProgress(socket, 7, {
      kind: "end",
      message: "Dependency analysis finished",
    });

    expect(
      socket.sent.map((message) => JSON.parse(message) as Record<string, unknown>),
    ).toContainEqual({ jsonrpc: "2.0", id: 91, result: null });
    expect(lspClientManager.getProgress(SESSION_ID, LANGUAGE)).toEqual({
      initializingSince: expect.any(Number),
      active: [],
      completed: {
        token: 7,
        title: "Analyzing dependencies",
        message: "Dependency analysis finished",
        startedAt: expect.any(Number),
        completedAt: expect.any(Number),
      },
      hasReportedProgress: true,
    });
  });

  it("ignores late progress from a replaced connection generation", () => {
    const { socket: oldSocket, initialize: oldInitialize } = beginInitialization(
      REPLACEMENT_SESSION_ID,
      "/old",
    );
    oldSocket.readyState = FakeWebSocket.CLOSING;

    const { socket: currentSocket, initialize: currentInitialize } = beginInitialization(
      REPLACEMENT_SESSION_ID,
      "/replacement",
    );
    emitProgress(currentSocket, currentInitialize.params.workDoneToken, {
      kind: "begin",
      title: "Current generation",
    });
    const currentProgress = lspClientManager.getProgress(REPLACEMENT_SESSION_ID, LANGUAGE);

    emitProgress(oldSocket, oldInitialize.params.workDoneToken, {
      kind: "begin",
      title: "Stale generation",
    });

    expect(lspClientManager.getProgress(REPLACEMENT_SESSION_ID, LANGUAGE)).toBe(currentProgress);
    expect(currentProgress.active.map((item) => item.title)).toEqual(["Current generation"]);
  });

  it("clears connection-owned progress when the server stops", () => {
    const { socket, initialize } = beginInitialization();
    emitProgress(socket, initialize.params.workDoneToken, {
      kind: "begin",
      title: "Temporary work",
    });

    lspClientManager.stop(SESSION_ID, LANGUAGE);

    const progress = lspClientManager.getProgress(SESSION_ID, LANGUAGE);
    expect(progress).toEqual({
      initializingSince: null,
      active: [],
      completed: null,
      hasReportedProgress: false,
    });
    expect(progress).toBe(lspClientManager.getProgress(SESSION_ID, LANGUAGE));
  });
});

describe("LSP progress-aware idle cleanup", () => {
  it("keeps a released connection alive until initialization finishes", async () => {
    vi.useFakeTimers();
    try {
      const { initialize, release, socket } = beginInitialization();
      release();

      await vi.advanceTimersByTimeAsync(LSP_IDLE_TIMEOUT + 1);

      expect(socket.readyState).toBe(FakeWebSocket.OPEN);
      expect(lspClientManager.getStatus(SESSION_ID, LANGUAGE)).toEqual({ state: "starting" });

      completeInitialization(socket, initialize.id);
      await vi.advanceTimersByTimeAsync(0);
      expect(lspClientManager.getStatus(SESSION_ID, LANGUAGE)).toEqual({ state: "ready" });

      await vi.advanceTimersByTimeAsync(LSP_IDLE_TIMEOUT + 1);
      expect(socket.readyState).toBe(FakeWebSocket.CLOSED);
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps a released ready connection alive until reported work finishes", async () => {
    const { initialize, release, socket } = beginInitialization();
    completeInitialization(socket, initialize.id);
    await vi.waitFor(() => {
      expect(lspClientManager.getStatus(SESSION_ID, LANGUAGE)).toEqual({ state: "ready" });
    });
    emitProgress(socket, initialize.params.workDoneToken, {
      kind: "begin",
      title: "Importing project",
    });

    vi.useFakeTimers();
    try {
      release();
      await vi.advanceTimersByTimeAsync(LSP_IDLE_TIMEOUT + 1);
      expect(socket.readyState).toBe(FakeWebSocket.OPEN);

      emitProgress(socket, initialize.params.workDoneToken, { kind: "end" });
      await vi.advanceTimersByTimeAsync(LSP_IDLE_TIMEOUT + 1);
      expect(socket.readyState).toBe(FakeWebSocket.CLOSED);
    } finally {
      vi.useRealTimers();
    }
  });
});
