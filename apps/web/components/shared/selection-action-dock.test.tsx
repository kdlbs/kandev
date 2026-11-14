import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { IconMessage } from "@tabler/icons-react";
import { SelectionActionDock, SelectionActionDockButton } from "./selection-action-dock";

afterEach(cleanup);

const DOCK_LABEL = "Selection actions";
const COMMENT_LABEL = "Comment on selection";

function renderDock(portalContainer: HTMLElement | null = null) {
  const onActivate = vi.fn();
  const host = document.createElement("div");
  host.getBoundingClientRect = () =>
    ({ left: 20, width: 300, bottom: 600, top: 100, right: 320, height: 500 }) as DOMRect;
  render(
    <SelectionActionDock
      hostRef={{ current: host }}
      portalContainer={portalContainer}
      label={DOCK_LABEL}
      testId="dock"
    >
      <SelectionActionDockButton
        icon={IconMessage}
        label={COMMENT_LABEL}
        onActivate={onActivate}
        testId="dock-comment"
      />
    </SelectionActionDock>,
  );
  return { onActivate };
}

describe("SelectionActionDock", () => {
  it("places a compact toolbar at the host's bottom-right without covering the row", () => {
    renderDock();
    const toolbar = screen.getByRole("toolbar", { name: DOCK_LABEL });
    const positioner = toolbar.parentElement!;

    expect(positioner.parentElement).toBe(document.body);
    expect(positioner.style.position).toBe("fixed");
    expect(positioner.style.top).toBe("552px");
    expect(positioner.style.left).toBe("20px");
    expect(positioner.style.width).toBe("300px");
    expect(positioner.style.height).toBe("48px");
    // The row itself stays transparent and lets taps reach the content below.
    expect(positioner.classList.contains("pointer-events-none")).toBe(true);
    expect(positioner.classList.contains("justify-end")).toBe(true);
    expect(positioner.className).not.toContain("bg-");
    expect(toolbar.classList.contains("pointer-events-auto")).toBe(true);
    expect(toolbar.className).not.toContain("border-t");
  });

  it("positions relative to a modal portal container", () => {
    const dialog = document.createElement("div");
    dialog.getBoundingClientRect = () =>
      ({ left: 10, top: 50, width: 380, bottom: 750, right: 390, height: 700 }) as DOMRect;
    document.body.appendChild(dialog);
    renderDock(dialog);
    const positioner = screen.getByRole("toolbar", { name: DOCK_LABEL }).parentElement!;

    expect(positioner.parentElement).toBe(dialog);
    expect(positioner.style.position).toBe("absolute");
    expect(positioner.style.top).toBe("502px");
    expect(positioner.style.left).toBe("10px");
    dialog.remove();
  });

  it("renders an icon-only square action that keeps the current selection", () => {
    const { onActivate } = renderDock();
    const button = screen.getByRole("button", { name: COMMENT_LABEL });

    expect(button.textContent).toBe("");
    expect(button.getAttribute("title")).toBe(COMMENT_LABEL);
    expect(button.className).toContain("min-h-11");
    expect(button.className).toContain("min-w-11");
    expect(button.className).toContain("cursor-pointer");
    const square = button.firstElementChild!;
    expect(square.className).toContain("h-8");
    expect(square.className).toContain("w-8");
    expect(square.className).toContain("bg-primary");

    expect(fireEvent.pointerDown(button)).toBe(false);
    expect(fireEvent.mouseDown(button)).toBe(false);
    fireEvent.click(button);
    expect(onActivate).toHaveBeenCalledTimes(1);
  });
});
