import { beforeEach, describe, expect, it } from "vitest";
import type { BootPayload } from "@/src/boot-payload";
import { resetKanbanPreviewState } from "@/lib/local-storage";
import { applyBrowserDemoDefaults } from "./install";

beforeEach(() => localStorage.clear());

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
