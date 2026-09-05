import { DEFAULT_PREVIEW_RUNTIME_OPTIONS, PreviewRuntimeVm } from "./preview-runtime-vm";
import {
  PREVIEW_RUNTIME_PROTOCOL_VERSION,
  isPreviewRuntimeRequest,
  type PreviewRuntimeOptions,
  type PreviewRuntimeRequest,
  type PreviewRuntimeResponse,
  type PreviewRuntimeSession,
} from "./preview-runtime-types";

export { findPreviewSnapshotNode } from "./preview-runtime-types";

const DEFAULT_WORKER_OPTIONS: PreviewRuntimeOptions = DEFAULT_PREVIEW_RUNTIME_OPTIONS;

export async function createPreviewRuntimeSession(
  options: PreviewRuntimeOptions = DEFAULT_WORKER_OPTIONS,
): Promise<PreviewRuntimeSession> {
  const runtime = await PreviewRuntimeVm.create(options);
  return {
    load: (source) => runtime.load(source),
    dispatch: (event) => runtime.dispatch(event),
    dispose: async () => runtime.dispose(),
  };
}

type PreviewWorkerScope = {
  postMessage: (message: PreviewRuntimeResponse) => void;
  addEventListener: (type: "message", listener: (event: MessageEvent<unknown>) => void) => void;
  importScripts?: (...urls: string[]) => void;
};

const workerScope = globalThis as unknown as PreviewWorkerScope;
if (
  typeof workerScope.importScripts === "function" &&
  typeof workerScope.postMessage === "function"
) {
  let runtimePromise: Promise<PreviewRuntimeSession> | undefined;
  let generation = 0;

  workerScope.addEventListener("message", (message) => {
    void handleWorkerMessage(workerScope, message.data, () => {
      runtimePromise ??= createPreviewRuntimeSession();
      return runtimePromise;
    });
  });

  async function handleWorkerMessage(
    scope: PreviewWorkerScope,
    value: unknown,
    getRuntime: () => Promise<PreviewRuntimeSession>,
  ): Promise<void> {
    if (!isPreviewRuntimeRequest(value)) {
      scope.postMessage({
        protocolVersion: PREVIEW_RUNTIME_PROTOCOL_VERSION,
        type: "failed",
        generation,
        failure: { code: "malformed-message", level: "error" },
      });
      return;
    }
    const request = value as PreviewRuntimeRequest;
    if (request.type === "dispose") {
      generation = request.generation;
      const runtime = runtimePromise ? await runtimePromise : undefined;
      await runtime?.dispose();
      runtimePromise = undefined;
      return;
    }
    if (request.generation < generation) return;
    generation = request.generation;

    try {
      const runtime = await getRuntime();
      const snapshot =
        request.type === "load"
          ? await runtime.load(request.source)
          : await runtime.dispatch(request.event);
      scope.postMessage({
        protocolVersion: PREVIEW_RUNTIME_PROTOCOL_VERSION,
        type: request.type === "load" ? "ready" : "snapshot",
        generation: request.generation,
        snapshot,
      });
    } catch (error) {
      const code = isPreviewRuntimeFailureCode(error) ? error.code : "runtime-error";
      scope.postMessage({
        protocolVersion: PREVIEW_RUNTIME_PROTOCOL_VERSION,
        type: "failed",
        generation: request.generation,
        failure: { code, level: "error" },
      });
    }
  }
}

function isPreviewRuntimeFailureCode(error: unknown): error is {
  code:
    | "runtime-error"
    | "unsupported-capability"
    | "budget-exceeded"
    | "malformed-message"
    | "initialization-failed"
    | "disposed";
} {
  return (
    typeof error === "object" &&
    error !== null &&
    "code" in error &&
    typeof error.code === "string" &&
    [
      "runtime-error",
      "unsupported-capability",
      "budget-exceeded",
      "malformed-message",
      "initialization-failed",
      "disposed",
    ].includes(error.code)
  );
}
