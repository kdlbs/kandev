import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { installBrowserDemo } from "./install";
import { createBootPayload, createDemoState, DEMO_STORAGE_KEY } from "./scenario";
import type { DemoWorkerRequest } from "./protocol";

class FakeWorker extends EventTarget {
  static instances: FakeWorker[] = [];
  requests: DemoWorkerRequest[] = [];
  constructor() {
    super();
    FakeWorker.instances.push(this);
  }
  postMessage(message: DemoWorkerRequest) {
    this.requests.push(message);
    if (message.kind === "init") {
      this.dispatchEvent(
        new MessageEvent("message", {
          data: { kind: "result", id: message.id, value: createBootPayload(createDemoState()) },
        }),
      );
    }
  }
}
const nativeFetch = window.fetch;
const nativeWebSocket = window.WebSocket;
beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  FakeWorker.instances = [];
  vi.stubGlobal("Worker", FakeWorker);
});
afterEach(() => {
  vi.unstubAllGlobals();
  window.fetch = nativeFetch;
  window.WebSocket = nativeWebSocket;
  history.replaceState({}, "", "/");
});
it("restores and saves this tab's snapshot without overwriting shared local storage", async () => {
  sessionStorage.setItem(DEMO_STORAGE_KEY, "this-tab");
  localStorage.setItem(DEMO_STORAGE_KEY, "other-tab");
  await installBrowserDemo();
  const worker = FakeWorker.instances[0];
  expect(worker.requests[0]).toMatchObject({ kind: "init", persistedState: "this-tab" });
  worker.dispatchEvent(
    new MessageEvent("message", { data: { kind: "persist", state: "updated-tab" } }),
  );
  expect(sessionStorage.getItem(DEMO_STORAGE_KEY)).toBe("updated-tab");
  expect(localStorage.getItem(DEMO_STORAGE_KEY)).toBe("other-tab");
});
