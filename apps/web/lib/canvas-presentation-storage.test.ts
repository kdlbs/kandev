import { beforeEach, describe, expect, it } from "vitest";
import {
  canvasPresentationKey,
  markCanvasPresented,
  wasCanvasPresented,
  type CanvasPresentationIdentity,
} from "./canvas-presentation-storage";

const identity: CanvasPresentationIdentity = {
  userId: "user-1",
  workspaceId: "workspace-1",
  taskId: "task-1",
  canvasId: "canvas-1",
};

beforeEach(() => {
  window.sessionStorage.clear();
});

describe("canvas presentation receipts", () => {
  it("keys receipts by the complete user, workspace, task, and canvas identity", () => {
    expect(canvasPresentationKey(identity)).not.toBe(
      canvasPresentationKey({ ...identity, userId: "user-2" }),
    );
    expect(canvasPresentationKey(identity)).not.toBe(
      canvasPresentationKey({ ...identity, workspaceId: "workspace-2" }),
    );
    expect(canvasPresentationKey(identity)).not.toBe(
      canvasPresentationKey({ ...identity, taskId: "task-2" }),
    );
    expect(canvasPresentationKey(identity)).not.toBe(
      canvasPresentationKey({ ...identity, canvasId: "canvas-2" }),
    );
  });

  it("persists a receipt for the browser tab", () => {
    expect(wasCanvasPresented(identity)).toBe(false);

    markCanvasPresented(identity);

    expect(wasCanvasPresented(identity)).toBe(true);
    expect(wasCanvasPresented({ ...identity, canvasId: "canvas-2" })).toBe(false);
  });

  it("falls back to tab memory when sessionStorage cannot be written", () => {
    const blocked = { ...identity, canvasId: "canvas-blocked" };
    const setItem = window.sessionStorage.setItem;
    window.sessionStorage.setItem = () => {
      throw new Error("storage blocked");
    };

    markCanvasPresented(blocked);

    window.sessionStorage.setItem = setItem;
    expect(wasCanvasPresented(blocked)).toBe(true);
  });
});
