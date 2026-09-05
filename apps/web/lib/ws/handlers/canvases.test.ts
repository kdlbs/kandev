import { afterEach, describe, expect, it, vi } from "vitest";
import {
  getCanvasLifecycleRevision,
  invalidateCanvasLifecycle,
  subscribeCanvasLifecycle,
} from "@/lib/canvas-lifecycle";
import { registerCanvasesHandlers } from "./canvases";

const LIFECYCLE_ACTIONS = [
  "canvas.created",
  "canvas.release.activated",
  "canvas.release.permission_required",
  "canvas.promoted",
  "canvas.archived",
  "canvas.restored",
  "canvas.removed",
] as const;

afterEach(() => {
  vi.restoreAllMocks();
});

describe("canvas lifecycle WebSocket handlers", () => {
  it("invalidates every visible canvas projection action", () => {
    const listener = vi.fn();
    const unsubscribe = subscribeCanvasLifecycle(listener);
    const handlers = registerCanvasesHandlers({} as never);
    const before = getCanvasLifecycleRevision();

    for (const action of LIFECYCLE_ACTIONS) {
      handlers[action]?.({} as never);
    }

    expect(getCanvasLifecycleRevision()).toBe(before + LIFECYCLE_ACTIONS.length);
    expect(listener).toHaveBeenCalledTimes(LIFECYCLE_ACTIONS.length);
    unsubscribe();
  });

  it("does not notify an unsubscribed projection", () => {
    const listener = vi.fn();
    const unsubscribe = subscribeCanvasLifecycle(listener);
    unsubscribe();

    invalidateCanvasLifecycle();

    expect(listener).not.toHaveBeenCalled();
  });
});
