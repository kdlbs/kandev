import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createLspManagerHarness, FakeWebSocket } from "./lsp-client-manager.test-harness";
import { EMPTY_LSP_PROGRESS } from "./lsp-progress";

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

const TASK_ID = "task-progress";
const SESSION_ID = "session-progress";
const LANGUAGE = "typescript";

function attach(): { release: () => void; socket: FakeWebSocket } {
  const release = lspClientManager.connect(TASK_ID, SESSION_ID, LANGUAGE);
  const socket = FakeWebSocket.instances.at(-1);
  if (!socket) throw new Error("expected an LSP WebSocket");
  socket.open();
  socket.emitMessage(
    JSON.stringify({
      status: "attached",
      language: LANGUAGE,
      generation: 2,
      workspaceUri: "file:///workspace",
      workspaceFolders: [],
      serverCapabilities: {},
    }),
  );
  return { release, socket };
}

beforeEach(() => {
  lspClientManager.disconnectAll();
  FakeWebSocket.instances = [];
  vi.resetAllMocks();
  vi.stubGlobal("WebSocket", FakeWebSocket);
  const { monaco } = createLspManagerHarness(lspClientManager, mocks).createMonacoHarness([]);
  mocks.waitForMonacoInstance.mockResolvedValue(monaco);
  mocks.registerLspProviders.mockReturnValue([]);
  mocks.registerBuiltinTsSuppression.mockReturnValue({ dispose: vi.fn() });
});

afterEach(() => {
  lspClientManager.disconnectAll();
  vi.unstubAllGlobals();
});

describe("task-host-owned LSP progress", () => {
  it("never sends initialize or invents browser-owned progress", async () => {
    const { socket } = attach();
    await vi.waitFor(() =>
      expect(lspClientManager.getStatus(TASK_ID, LANGUAGE)).toEqual({ state: "ready" }),
    );
    expect(socket.sent).toEqual([]);
    expect(lspClientManager.getProgress(TASK_ID, LANGUAGE)).toBe(EMPTY_LSP_PROGRESS);
  });

  it("ignores protocol progress because task snapshots are authoritative", () => {
    const { socket } = attach();
    socket.emitMessage(
      JSON.stringify({
        jsonrpc: "2.0",
        method: "$/progress",
        params: { token: "gradle", value: { kind: "begin", title: "Importing" } },
      }),
    );
    expect(lspClientManager.getProgress(TASK_ID, LANGUAGE)).toBe(EMPTY_LSP_PROGRESS);
  });

  it("detaches immediately after the last editor without a lifecycle frame or timer", async () => {
    const { release, socket } = attach();
    await vi.waitFor(() =>
      expect(lspClientManager.getStatus(TASK_ID, LANGUAGE).state).toBe("ready"),
    );
    release();
    expect(socket.readyState).toBe(FakeWebSocket.CLOSED);
    expect(socket.sent).toEqual([]);
  });

  it("reports transport loss while allowing a new attachment to recover", async () => {
    const first = attach();
    await vi.waitFor(() =>
      expect(lspClientManager.getStatus(TASK_ID, LANGUAGE).state).toBe("ready"),
    );
    first.socket.failClosed(1006, "network interrupted");
    expect(lspClientManager.getStatus(TASK_ID, LANGUAGE)).toEqual({
      state: "error",
      reason: "network interrupted",
    });

    const second = attach();
    expect(second.socket).not.toBe(first.socket);
    await vi.waitFor(() =>
      expect(lspClientManager.getStatus(TASK_ID, LANGUAGE).state).toBe("ready"),
    );
    expect(second.socket.sent).toEqual([]);
  });
});
