import { expect, vi, type Mock } from "vitest";
type LspManager = (typeof import("./lsp-client-manager"))["lspClientManager"];

export type LspManagerMocks = {
  getMonacoInstance: Mock;
  registerLspProviders: Mock;
};

export class FakeWebSocket extends EventTarget {
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

function createTestModel(uri: string) {
  return {
    uri: parseUri(uri),
    dispose: vi.fn(),
    setValue: vi.fn(),
  };
}

export type TestModel = ReturnType<typeof createTestModel>;

export function createDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
}

export function publishDiagnostic(socket: FakeWebSocket, uri: string, message: string): void {
  socket.emitMessage(
    JSON.stringify({
      jsonrpc: "2.0",
      method: "textDocument/publishDiagnostics",
      params: {
        uri,
        diagnostics: [
          {
            range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } },
            message,
          },
        ],
      },
    }),
  );
}

export function markerMessages(
  markersByUri: Map<string, Map<string, Array<{ message: string }>>>,
  uri: string,
) {
  return [...(markersByUri.get(uri)?.values() ?? [])].flat().map((marker) => marker.message);
}

export function createLspManagerHarness(manager: LspManager, mocks: LspManagerMocks) {
  function createMonacoHarness(primaryUris: string[]) {
    const models = new Map<string, TestModel>();
    for (const uri of primaryUris) models.set(uri, createTestModel(uri));
    const modelCreationListeners = new Set<(model: TestModel) => void>();
    const markersByUri = new Map<string, Map<string, Array<{ message: string }>>>();
    const monaco = {
      Uri: { parse: parseUri },
      MarkerSeverity: { Error: 8, Warning: 4, Info: 2, Hint: 1 },
      editor: {
        createModel: vi.fn(
          (_value: string, _language: string | undefined, uri: ReturnType<typeof parseUri>) => {
            const model = createTestModel(uri.toString());
            model.dispose.mockImplementation(() => models.delete(uri.toString()));
            models.set(uri.toString(), model);
            for (const listener of modelCreationListeners) listener(model);
            return model;
          },
        ),
        onDidCreateModel: vi.fn((listener: (model: TestModel) => void) => {
          modelCreationListeners.add(listener);
          return { dispose: () => modelCreationListeners.delete(listener) };
        }),
        getModel: vi.fn((uri: ReturnType<typeof parseUri>) => models.get(uri.toString()) ?? null),
        getModels: vi.fn(() => [...models.values()]),
        setModelMarkers: vi.fn(
          (model: TestModel, owner: string, markers: Array<{ message: string }>) => {
            const owners = markersByUri.get(model.uri.toString()) ?? new Map();
            if (markers.length === 0) owners.delete(owner);
            else owners.set(owner, markers);
            markersByUri.set(model.uri.toString(), owners);
          },
        ),
      },
    };
    mocks.getMonacoInstance.mockReturnValue(monaco);
    return { markersByUri, models, monaco };
  }

  function requireModel(models: Map<string, TestModel>, uri: string): TestModel {
    const model = models.get(uri);
    if (!model) throw new Error(`expected Monaco model for ${uri}`);
    return model;
  }

  async function connectReadyWithRelease(sessionId: string, workspacePath?: string) {
    const release = manager.connect(sessionId, "typescript");
    const socket = FakeWebSocket.instances.at(-1);
    if (!socket) throw new Error("expected an LSP WebSocket");
    socket.open();
    socket.emitMessage(JSON.stringify({ status: "ready", workspacePath }));
    const initializeRequest = JSON.parse(socket.sent[0]) as { id: number };
    socket.emitMessage(
      JSON.stringify({ jsonrpc: "2.0", id: initializeRequest.id, result: { capabilities: {} } }),
    );
    await vi.waitFor(() => {
      expect(manager.getStatus(sessionId, "typescript")).toEqual({ state: "ready" });
    });
    return { socket, release };
  }

  async function connectReady(sessionId: string, workspacePath?: string): Promise<FakeWebSocket> {
    return (await connectReadyWithRelease(sessionId, workspacePath)).socket;
  }

  function createPlaceholder(providerIndex: number, uri: string): void {
    const providerOptions = mocks.registerLspProviders.mock.calls[providerIndex][0] as {
      ensureModelsExist: (uris: string[]) => void;
    };
    providerOptions.ensureModelsExist([uri]);
  }

  return {
    connectReady,
    connectReadyWithRelease,
    createMonacoHarness,
    createPlaceholder,
    requireModel,
  };
}
