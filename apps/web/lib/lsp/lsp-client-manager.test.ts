import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  getMonacoInstance: vi.fn(),
  registerLspProviders: vi.fn(),
  setBuiltinTsSuppressed: vi.fn(),
}));

vi.mock("@/components/editors/monaco/monaco-init", () => ({
  getMonacoInstance: mocks.getMonacoInstance,
}));

vi.mock("@/components/editors/monaco/builtin-providers", () => ({
  setBuiltinTsSuppressed: mocks.setBuiltinTsSuppressed,
}));

vi.mock("./lsp-providers", () => ({
  registerLspProviders: mocks.registerLspProviders,
}));

import { lspClientManager } from "./lsp-client-manager";

class FakeWebSocket extends EventTarget {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: FakeWebSocket[] = [];

  readyState = FakeWebSocket.CONNECTING;
  onopen: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readonly sent: string[] = [];

  constructor(readonly url: string) {
    super();
    FakeWebSocket.instances.push(this);
  }

  send(data: string): void {
    this.sent.push(data);
  }

  close(): void {
    this.readyState = FakeWebSocket.CLOSED;
  }

  open(): void {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.(new Event("open"));
  }

  emitMessage(data: string): void {
    this.dispatchEvent(new MessageEvent("message", { data }));
  }

  failClosed(code: number, reason: string): void {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.(new CloseEvent("close", { code, reason }));
  }
}

function parseUri(value: string) {
  return {
    path: new URL(value).pathname,
    toString: () => value,
  };
}

describe("LSP client connection cleanup", () => {
  beforeEach(() => {
    lspClientManager.disconnectAll();
    FakeWebSocket.instances = [];
    vi.clearAllMocks();
    vi.stubGlobal("WebSocket", FakeWebSocket);
  });

  afterEach(() => {
    lspClientManager.disconnectAll();
    vi.unstubAllGlobals();
  });

  it("fully restores editor state when a ready TypeScript server closes unexpectedly", async () => {
    const providerDispose = vi.fn();
    const placeholderDispose = vi.fn();
    const primaryModel = {
      uri: parseUri("file:///workspace/Main.ts"),
      dispose: vi.fn(),
      setValue: vi.fn(),
    };
    const models = new Map<string, typeof primaryModel>([
      [primaryModel.uri.toString(), primaryModel],
    ]);
    const monaco = {
      Uri: { parse: parseUri },
      editor: {
        createModel: vi.fn((_value, _language, uri) => {
          const model = { uri, dispose: placeholderDispose, setValue: vi.fn() };
          models.set(uri.toString(), model);
          return model;
        }),
        getModel: vi.fn((uri) => models.get(uri.toString()) ?? null),
        getModels: vi.fn(() => [...models.values()]),
        setModelMarkers: vi.fn(),
      },
    };
    mocks.getMonacoInstance.mockReturnValue(monaco);
    mocks.registerLspProviders.mockReturnValue([{ dispose: providerDispose }]);

    lspClientManager.connect("session", "typescript");
    const socket = FakeWebSocket.instances[0];
    socket.open();
    socket.emitMessage(JSON.stringify({ status: "ready" }));

    const initializeRequest = JSON.parse(socket.sent[0]) as { id: number };
    socket.emitMessage(
      JSON.stringify({ jsonrpc: "2.0", id: initializeRequest.id, result: { capabilities: {} } }),
    );
    await vi.waitFor(() => {
      expect(lspClientManager.getStatus("session", "typescript")).toEqual({ state: "ready" });
    });

    const providerOptions = mocks.registerLspProviders.mock.calls[0][0] as {
      ensureModelsExist: (uris: string[], connectionKey: string) => void;
    };
    providerOptions.ensureModelsExist(["file:///workspace/Referenced.ts"], "session:typescript");

    socket.failClosed(1006, "language server crashed");

    expect(providerDispose).toHaveBeenCalledOnce();
    expect(mocks.setBuiltinTsSuppressed).toHaveBeenLastCalledWith(false);
    expect(placeholderDispose).toHaveBeenCalledOnce();
    expect(monaco.editor.setModelMarkers).toHaveBeenCalledWith(primaryModel, "lsp", []);
  });
});
