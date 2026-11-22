import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useQueuePanelEscape } from "./use-queue-panel-escape";

afterEach(cleanup);

const ESCAPE_BOUNDARY = "data-queue-escape-boundary";
const DATA_STATE = "data-state";

// @covers AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.5
it("leaves Escape to a nested timestamp drawer and still closes the queue outside it", () => {
  const onClose = vi.fn();
  renderHook(() => useQueuePanelEscape(true, onClose));
  const drawer = document.createElement("div");
  drawer.setAttribute(ESCAPE_BOUNDARY, "");
  drawer.setAttribute(DATA_STATE, "open");
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
    drawer.setAttribute(DATA_STATE, "closed");
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

it.each(["instant-open", "delayed-open"])(
  "leaves the first Escape to an open %s automation tooltip",
  (state) => {
    const onClose = vi.fn();
    renderHook(() => useQueuePanelEscape(true, onClose));
    const tooltip = document.createElement("div");
    tooltip.setAttribute(ESCAPE_BOUNDARY, "");
    tooltip.setAttribute(DATA_STATE, state);
    document.body.append(tooltip);
    try {
      const escape = new KeyboardEvent("keydown", {
        key: "Escape",
        bubbles: true,
        cancelable: true,
      });
      document.body.dispatchEvent(escape);
      expect(onClose).not.toHaveBeenCalled();
      expect(escape.defaultPrevented).toBe(false);
      tooltip.setAttribute(DATA_STATE, "closed");
      document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
      expect(onClose).toHaveBeenCalledOnce();
    } finally {
      tooltip.remove();
    }
  },
);
