import {
  PREVIEW_RUNTIME_PROTOCOL_VERSION,
  PreviewRuntimeError,
  isPreviewRuntimeResponse,
  type PreviewEvent,
  type PreviewRuntimeRequest,
  type PreviewRuntimeResponse,
  type PreviewSnapshot,
} from "./preview-runtime-types";

export type PreviewRuntimeWorker = {
  postMessage: (message: PreviewRuntimeRequest) => void;
  terminate: () => void;
  onmessage: ((event: MessageEvent<unknown>) => void) | null;
  onerror: ((event: ErrorEvent) => void) | null;
};

export type PreviewRuntimeClientOptions = {
  workerFactory?: () => PreviewRuntimeWorker;
};

export type PreviewRuntimeClient = {
  load(source: string): Promise<PreviewSnapshot>;
  dispatch(event: PreviewEvent): Promise<PreviewSnapshot>;
  dispose(): void;
};

type PendingRequest = {
  resolve: (snapshot: PreviewSnapshot) => void;
  reject: (error: PreviewRuntimeError) => void;
};

export function createPreviewRuntime(
  options: PreviewRuntimeClientOptions = {},
): PreviewRuntimeClient {
  return new PreviewRuntimeClientImpl(options.workerFactory ?? createWorker);
}

class PreviewRuntimeClientImpl implements PreviewRuntimeClient {
  private readonly worker: PreviewRuntimeWorker | undefined;
  private readonly pending = new Map<number, PendingRequest>();
  private generation = 0;
  private disposed = false;

  constructor(workerFactory: () => PreviewRuntimeWorker) {
    try {
      this.worker = workerFactory();
      this.worker.onmessage = (event) => this.handleMessage(event.data);
      this.worker.onerror = () => this.failPending("initialization-failed");
    } catch {
      this.worker = undefined;
    }
  }

  load(source: string): Promise<PreviewSnapshot> {
    return this.send({ type: "load", source });
  }

  dispatch(event: PreviewEvent): Promise<PreviewSnapshot> {
    return this.send({ type: "dispatch", event });
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.generation += 1;
    if (this.worker) {
      try {
        this.worker.postMessage({
          protocolVersion: PREVIEW_RUNTIME_PROTOCOL_VERSION,
          type: "dispose",
          generation: this.generation,
        });
      } catch {
        // Termination below still releases the browser worker.
      }
      this.worker.terminate();
    }
    this.failPending("disposed");
  }

  private send(
    request: { type: "load"; source: string } | { type: "dispatch"; event: PreviewEvent },
  ): Promise<PreviewSnapshot> {
    if (this.disposed) return Promise.reject(new PreviewRuntimeError("disposed"));
    const generation = ++this.generation;
    if (!this.worker) return Promise.reject(new PreviewRuntimeError("initialization-failed"));
    const message = {
      protocolVersion: PREVIEW_RUNTIME_PROTOCOL_VERSION,
      generation,
      ...request,
    } as PreviewRuntimeRequest;
    return new Promise<PreviewSnapshot>((resolve, reject) => {
      this.pending.set(generation, { resolve, reject });
      try {
        this.worker?.postMessage(message);
      } catch {
        this.pending.delete(generation);
        reject(new PreviewRuntimeError("initialization-failed"));
      }
    });
  }

  private handleMessage(value: unknown): void {
    if (!isPreviewRuntimeResponse(value)) {
      this.failPending("malformed-message");
      return;
    }
    const response = value as PreviewRuntimeResponse;
    const pending = this.pending.get(response.generation);
    if (!pending || response.generation !== this.generation) return;
    this.pending.delete(response.generation);
    if (response.type === "failed") pending.reject(new PreviewRuntimeError(response.failure.code));
    else pending.resolve(response.snapshot);
  }

  private failPending(code: PreviewRuntimeError["code"]): void {
    const error = new PreviewRuntimeError(code);
    for (const pending of this.pending.values()) pending.reject(error);
    this.pending.clear();
  }
}

function createWorker(): PreviewRuntimeWorker {
  return new Worker(new URL("./preview-runtime.worker.ts", import.meta.url), {
    type: "module",
  });
}
