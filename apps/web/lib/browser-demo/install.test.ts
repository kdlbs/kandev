import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BootPayload } from "@/src/boot-payload";
import { resetKanbanPreviewState } from "@/lib/local-storage";
import {
  applyBrowserDemoDefaults,
  installBrowserDemo,
  rejectPendingRequests,
  serializeDemoHttpResponseBody,
} from "./install";

const nativeFetch = window.fetch;
const nativeWebSocket = window.WebSocket;

beforeEach(() => localStorage.clear());

afterEach(() => {
  window.fetch = nativeFetch;
  window.WebSocket = nativeWebSocket;
  vi.unstubAllGlobals();
});

describe("applyBrowserDemoDefaults", () => {
  it("disables preview-on-click before the browser demo mounts", () => {
    const payload = {
      version: 1,
      initialState: {
        userSettings: {
          enablePreviewOnClick: true,
          loaded: true,
        },
      },
    } as BootPayload;

    const result = applyBrowserDemoDefaults(payload);

    expect(result.initialState?.userSettings?.enablePreviewOnClick).toBe(false);
    expect(result.initialState?.userSettings?.loaded).toBe(true);
    expect(payload.initialState?.userSettings?.enablePreviewOnClick).toBe(true);
  });

  it("leaves payloads without user settings unchanged", () => {
    const payload: BootPayload = { version: 1, initialState: {} };

    expect(applyBrowserDemoDefaults(payload)).toBe(payload);
  });
});

describe("browser demo preview state", () => {
  it("starts with the task preview closed even after an earlier demo visit", () => {
    localStorage.setItem("kandev.kanban.preview.open", "true");
    localStorage.setItem("kandev.kanban.preview.selectedTask", '"demo-task-audit"');
    localStorage.setItem("kandev.kanban.preview.width", "560");

    resetKanbanPreviewState();

    expect(localStorage.getItem("kandev.kanban.preview.open")).toBeNull();
    expect(localStorage.getItem("kandev.kanban.preview.selectedTask")).toBeNull();
    expect(localStorage.getItem("kandev.kanban.preview.width")).toBe("560");
  });
});

describe("browser demo response serialization", () => {
  it("passes YAML exports through as text instead of JSON-quoting them", () => {
    const yaml = 'version: 1\ntype: kandev_workflow\nworkflows:\n  - name: "Release"\n';

    expect(serializeDemoHttpResponseBody({ status: 200, body: yaml, bodyFormat: "text" })).toBe(
      yaml,
    );
    expect(serializeDemoHttpResponseBody({ status: 200, body: { created: ["Release"] } })).toBe(
      '{"created":["Release"]}',
    );
  });
});

describe("browser demo worker failures", () => {
  it("clears and rejects every pending worker request", () => {
    const firstReject = vi.fn();
    const secondReject = vi.fn();
    const pending = new Map([
      ["first", { resolve: vi.fn(), reject: firstReject }],
      ["second", { resolve: vi.fn(), reject: secondReject }],
    ]);
    const error = new Error("worker crashed");

    rejectPendingRequests(pending, error);

    expect(pending.size).toBe(0);
    expect(firstReject).toHaveBeenCalledWith(error);
    expect(secondReject).toHaveBeenCalledWith(error);
  });

  it("rejects installation and terminates when the worker emits an error", async () => {
    const demoWorker = new FakeWorker();
    vi.stubGlobal(
      "Worker",
      vi.fn(function WorkerMock() {
        return demoWorker;
      }),
    );

    const installation = installBrowserDemo();
    const error = new Error("failed to load demo worker");
    demoWorker.dispatchEvent(new ErrorEvent("error", { error, message: error.message }));

    await expect(installation).rejects.toBe(error);
    expect(demoWorker.terminate).toHaveBeenCalledOnce();
  });
});

class FakeWorker extends EventTarget {
  postMessage = vi.fn();
  terminate = vi.fn();
}
