import { beforeEach, describe, expect, it, vi } from "vitest";

const identity = {
  userId: "user-1",
  workspaceId: "workspace-1",
  taskId: "task-1",
  canvasId: "canvas-1",
};

const uuidValues: string[] = [];

vi.doMock("@/lib/uuid", () => ({
  generateUUID: () => uuidValues.shift() ?? "uuid-fallback",
}));

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
  uuidValues.length = 0;
  vi.resetModules();
});

describe("canvas presentation tab identity", () => {
  it("rotates a copied session namespace while keeping the source receipt isolated", async () => {
    uuidValues.push("source-page", "source-tab", "duplicate-page", "duplicate-tab");
    const source = await import("./canvas-presentation-storage");
    source.markCanvasPresented(identity);
    const sourceKey = source.canvasPresentationKey(identity);

    vi.resetModules();
    const duplicate = await import("./canvas-presentation-storage");

    expect(duplicate.wasCanvasPresented(identity)).toBe(false);
    duplicate.markCanvasPresented(identity);
    expect(duplicate.wasCanvasPresented(identity)).toBe(true);
    expect(window.sessionStorage.getItem(sourceKey)).toBe("1");
  });

  it("reuses the tab identity after a normal reload releases its page owner", async () => {
    uuidValues.push("source-page", "source-tab", "reloaded-page");
    const source = await import("./canvas-presentation-storage");
    source.markCanvasPresented(identity);
    window.dispatchEvent(new PageTransitionEvent("pagehide", { persisted: false }));

    vi.resetModules();
    const reloaded = await import("./canvas-presentation-storage");

    expect(reloaded.wasCanvasPresented(identity)).toBe(true);
  });
});
