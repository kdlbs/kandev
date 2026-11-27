import { beforeEach, describe, expect, it } from "vitest";
import type { BootPayload } from "@/src/boot-payload";
import { resetKanbanPreviewState } from "@/lib/local-storage";
import {
  applyBrowserDemoDefaults,
  browserDemoStartPath,
  serializeDemoHttpResponseBody,
} from "./install";
import { createBootPayload, createDemoState } from "./scenario";

beforeEach(() => localStorage.clear());

describe("browser demo start path", () => {
  it("opens the audit logging task and its seeded session by default", () => {
    expect(browserDemoStartPath(createBootPayload(createDemoState()))).toBe(
      "/t/demo-task-audit?sessionId=demo-session-audit",
    );
  });

  it("falls back to home when the visitor has deleted the audit task", () => {
    const state = createDemoState();
    state.tasks = state.tasks.filter((task) => task.id !== "demo-task-audit");
    state.sessions = state.sessions.filter((session) => session.task_id !== "demo-task-audit");

    expect(browserDemoStartPath(createBootPayload(state))).toBe("/");
  });

  it("does not select a session belonging to a different task", () => {
    const payload = createBootPayload(createDemoState());
    payload.initialState!.taskSessionsByTask!.itemsByTaskId["demo-task-audit"] =
      payload.initialState!.taskSessionsByTask!.itemsByTaskId["demo-task-auth"];

    expect(browserDemoStartPath(payload)).toBe("/");
  });
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
