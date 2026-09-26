import { fireEvent, screen } from "@testing-library/react";
import { vi } from "vitest";

/**
 * Drives a dnd-kit PointerSensor drag in happy-dom: mocks per-row droppable
 * rects for collision detection, patches isPrimary (happy-dom omits the
 * primary-pointer computation), and dispatches down / move / move / up.
 * The first move only activates the drag (distance constraint); the second
 * dispatches DragMove so collision detection resolves the target.
 */
export function simulateReorderDrag(
  handle: HTMLElement,
  rowCount: number,
  targetClientY: number,
): void {
  const original = HTMLElement.prototype.getBoundingClientRect;
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
    this: HTMLElement,
  ) {
    const inner = this.querySelector?.('[data-testid="queue-entry"]') as HTMLElement | null;
    const rows = screen.queryAllByTestId("queue-entry");
    const index = inner ? rows.indexOf(inner) : -1;
    if (index >= 0) {
      const top = index * 40;
      return {
        x: 0,
        y: top,
        top,
        bottom: top + 36,
        left: 0,
        right: 400,
        width: 400,
        height: 36,
      } as DOMRect;
    }
    return original.call(this);
  });

  // isPrimary is an own property created by the PointerEvent constructor
  // (prototype patching is shadowed), so a capture-phase listener rewrites it
  // before React's root handler runs.
  const patchPrimary = (event: Event) => {
    Object.defineProperty(event, "isPrimary", { configurable: true, value: true });
  };
  document.addEventListener("pointerdown", patchPrimary, true);
  fireEvent.pointerDown(handle, {
    pointerId: 0,
    pointerType: "mouse",
    button: 0,
    clientX: 10,
    clientY: (rowCount - 1) * 40 + 10,
  });
  document.removeEventListener("pointerdown", patchPrimary, true);
  fireEvent.pointerMove(document.body, {
    pointerId: 0,
    pointerType: "mouse",
    clientX: 10,
    clientY: targetClientY,
  });
  fireEvent.pointerMove(document.body, {
    pointerId: 0,
    pointerType: "mouse",
    clientX: 10,
    clientY: targetClientY + 2,
  });
  fireEvent.pointerUp(document.body, {
    pointerId: 0,
    pointerType: "mouse",
    clientX: 10,
    clientY: targetClientY + 2,
  });
}
