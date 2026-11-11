import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { IconMessage } from "@tabler/icons-react";
import { SelectionActionDock, SelectionActionDockButton } from "./selection-action-dock";

afterEach(cleanup);

function renderDock(portalContainer: HTMLElement | null = null) {
  const onActivate = vi.fn();
  const host = document.createElement("div");
  host.getBoundingClientRect = () =>
    ({ left: 20, width: 300, bottom: 600, top: 100, right: 320, height: 500 }) as DOMRect;
  render(
    <SelectionActionDock
      hostRef={{ current: host }}
      portalContainer={portalContainer}
      label="Selection actions"
      testId="dock"
    >
      <SelectionActionDockButton
        icon={IconMessage}
        label="Comment on selection"
        text="Comment"
        onActivate={onActivate}
        testId="dock-comment"
      />
    </SelectionActionDock>,
  );
  return { onActivate };
}

describe("SelectionActionDock", () => {
  it("renders a labelled toolbar at the host's bottom edge", () => {
    renderDock();
    const toolbar = screen.getByRole("toolbar", { name: "Selection actions" });

    expect(toolbar.parentElement).toBe(document.body);
    expect(toolbar.style.position).toBe("fixed");
    expect(toolbar.style.top).toBe("552px");
    expect(toolbar.style.left).toBe("20px");
    expect(toolbar.style.width).toBe("300px");
    expect(toolbar.style.height).toBe("48px");
  });

  it("positions relative to a modal portal container", () => {
    const dialog = document.createElement("div");
    dialog.getBoundingClientRect = () =>
      ({ left: 10, top: 50, width: 380, bottom: 750, right: 390, height: 700 }) as DOMRect;
    document.body.appendChild(dialog);
    renderDock(dialog);
    const toolbar = screen.getByRole("toolbar", { name: "Selection actions" });

    expect(toolbar.parentElement).toBe(dialog);
    expect(toolbar.style.position).toBe("absolute");
    expect(toolbar.style.top).toBe("502px");
    expect(toolbar.style.left).toBe("10px");
    dialog.remove();
  });

  it("activates without collapsing the current selection", () => {
    const { onActivate } = renderDock();
    const button = screen.getByRole("button", { name: "Comment on selection" });

    expect(fireEvent.pointerDown(button)).toBe(false);
    expect(fireEvent.mouseDown(button)).toBe(false);
    fireEvent.click(button);

    expect(onActivate).toHaveBeenCalledTimes(1);
    expect(button.className).toContain("min-h-11");
    expect(button.className).toContain("cursor-pointer");
  });
});
