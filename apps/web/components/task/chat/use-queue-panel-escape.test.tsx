import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useQueuePanelEscape } from "./use-queue-panel-escape";

afterEach(cleanup);

// @covers AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.5
it("leaves Escape to a nested timestamp drawer and still closes the queue outside it", () => {
  const onClose = vi.fn();
  renderHook(() => useQueuePanelEscape(true, onClose));
  const drawer = document.createElement("div");
  drawer.setAttribute("data-queue-escape-boundary", "");
  drawer.setAttribute("data-state", "open");
  const control = document.createElement("button");
  drawer.append(control);
  document.body.append(drawer);
  try {
    const nestedEscape = new KeyboardEvent("keydown", {
      key: "Escape",
      bubbles: true,
      cancelable: true,
    });
    control.dispatchEvent(nestedEscape);
    expect(onClose).not.toHaveBeenCalled();
    expect(nestedEscape.defaultPrevented).toBe(false);
    document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    expect(onClose).not.toHaveBeenCalled();
    drawer.setAttribute("data-state", "closed");
    const queueEscape = new KeyboardEvent("keydown", {
      key: "Escape",
      bubbles: true,
      cancelable: true,
    });
    document.body.dispatchEvent(queueEscape);
    expect(onClose).toHaveBeenCalledOnce();
    expect(queueEscape.defaultPrevented).toBe(true);
  } finally {
    drawer.remove();
  }
});
