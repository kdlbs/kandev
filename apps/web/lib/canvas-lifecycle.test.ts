import { describe, expect, it } from "vitest";
import {
  getCanvasLifecycleHints,
  getCanvasLifecycleRevision,
  recordCanvasLifecycle,
} from "@/lib/canvas-lifecycle";

describe("canvas lifecycle hints", () => {
  it("retains identity hints for lifecycle events", () => {
    const before = getCanvasLifecycleRevision();
    recordCanvasLifecycle("canvas.release.activated", {
      canvas_id: "canvas-retained",
      task_id: "task-retained",
      workspace_id: "workspace-retained",
    });

    expect(getCanvasLifecycleRevision()).toBe(before + 1);
    expect(getCanvasLifecycleHints().at(-1)).toMatchObject({
      action: "canvas.release.activated",
      payload: {
        canvas_id: "canvas-retained",
        task_id: "task-retained",
      },
    });
  });
});
