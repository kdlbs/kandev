import { describe, expect, it } from "vitest";
import type { PreviewRuntimeResponse, PreviewSnapshot } from "./preview-runtime-types";
import { createPreviewRuntime, type PreviewRuntimeWorker } from "./preview-runtime";

const snapshot: PreviewSnapshot = {
  protocolVersion: 1,
  root: {
    id: "preview-node-1",
    tagName: "body",
    attributes: {},
    styles: {},
    children: [],
    eventTypes: [],
  },
  resources: [],
  diagnostics: [],
};

class FakeWorker implements PreviewRuntimeWorker {
  messages: unknown[] = [];
  onmessage: ((event: MessageEvent<unknown>) => void) | null = null;
  onerror: ((event: ErrorEvent) => void) | null = null;

  postMessage(message: unknown): void {
    this.messages.push(message);
  }

  terminate(): void {}

  respond(response: PreviewRuntimeResponse): void {
    this.onmessage?.({ data: response } as MessageEvent<unknown>);
  }
}

describe("preview runtime client", () => {
  it("correlates worker snapshots with the active generation", async () => {
    const worker = new FakeWorker();
    const client = createPreviewRuntime({ workerFactory: () => worker });
    const result = client.load("<p>Preview</p>");

    expect(worker.messages).toHaveLength(1);
    expect(worker.messages[0]).toMatchObject({ type: "load", generation: 1 });
    worker.respond({ protocolVersion: 1, type: "ready", generation: 1, snapshot });

    await expect(result).resolves.toBe(snapshot);
    client.dispose();
  });

  it("rejects pending requests when the worker is disposed", async () => {
    const worker = new FakeWorker();
    const client = createPreviewRuntime({ workerFactory: () => worker });
    const result = client.load("<p>Preview</p>");

    client.dispose();

    await expect(result).rejects.toMatchObject({ code: "disposed" });
    expect(worker.messages.at(-1)).toMatchObject({ type: "dispose" });
  });
});
