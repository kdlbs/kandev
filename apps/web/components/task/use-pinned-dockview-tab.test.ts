import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useRef, type RefObject } from "react";
import { usePinnedDockviewTab, updatePinnedOffsets } from "./use-pinned-dockview-tab";

/**
 * Build a `.dv-tab` wrapper with an inner ref target, matching the layout
 * Dockview produces. `offsetWidth`, `marginLeft`, and `marginRight` are stubbed
 * because jsdom doesn't run layout.
 */
function buildTab(options: { container: HTMLElement; offsetWidth: number; margin?: number }): {
  dvTab: HTMLElement;
  inner: HTMLElement;
} {
  const dvTab = document.createElement("div");
  dvTab.className = "dv-tab";
  Object.defineProperty(dvTab, "offsetWidth", {
    configurable: true,
    value: options.offsetWidth,
  });
  if (options.margin !== undefined) {
    dvTab.style.marginLeft = `${options.margin}px`;
    dvTab.style.marginRight = `${options.margin}px`;
  }

  const inner = document.createElement("span");
  dvTab.appendChild(inner);
  options.container.appendChild(dvTab);

  return { dvTab, inner };
}

const PINNED_CLASS = "dv-pinned-tab";
const PIN_OFFSET_VAR = "--dv-pin-offset";

describe("updatePinnedOffsets", () => {
  it("returns without error when container is null", () => {
    expect(() => updatePinnedOffsets(null)).not.toThrow();
  });

  it("sets --dv-pin-offset to 0 on a single pinned tab", () => {
    const container = document.createElement("div");
    const { dvTab } = buildTab({ container, offsetWidth: 80 });
    dvTab.classList.add(PINNED_CLASS);

    updatePinnedOffsets(container);

    expect(dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("0px");
  });

  it("stacks multiple pinned tabs side-by-side by accumulating widths", () => {
    const container = document.createElement("div");
    const a = buildTab({ container, offsetWidth: 80 });
    const b = buildTab({ container, offsetWidth: 100 });
    const c = buildTab({ container, offsetWidth: 60 });
    a.dvTab.classList.add(PINNED_CLASS);
    b.dvTab.classList.add(PINNED_CLASS);
    c.dvTab.classList.add(PINNED_CLASS);

    updatePinnedOffsets(container);

    expect(a.dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("0px");
    expect(b.dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("80px");
    expect(c.dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("180px");
  });

  it("includes horizontal margins in the cumulative offset", () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    // Two pinned tabs with width=80 and margin: 0 2px each — the second tab's
    // offset should be 80 + 2 + 2 = 84, not 80. Attaching to document.body is
    // required for jsdom's getComputedStyle to reflect inline margins.
    const a = buildTab({ container, offsetWidth: 80, margin: 2 });
    const b = buildTab({ container, offsetWidth: 80, margin: 2 });
    a.dvTab.classList.add(PINNED_CLASS);
    b.dvTab.classList.add(PINNED_CLASS);

    try {
      updatePinnedOffsets(container);

      expect(a.dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("0px");
      expect(b.dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("84px");
    } finally {
      document.body.removeChild(container);
    }
  });

  it("skips non-pinned siblings", () => {
    const container = document.createElement("div");
    const pinned = buildTab({ container, offsetWidth: 80 });
    const notPinned = buildTab({ container, offsetWidth: 999 });
    const alsoPinned = buildTab({ container, offsetWidth: 50 });
    pinned.dvTab.classList.add(PINNED_CLASS);
    alsoPinned.dvTab.classList.add(PINNED_CLASS);

    updatePinnedOffsets(container);

    expect(pinned.dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("0px");
    expect(notPinned.dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("");
    // The non-pinned tab's width does not contribute to the cumulative offset.
    expect(alsoPinned.dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("80px");
  });
});

describe("usePinnedDockviewTab", () => {
  let resizeObserverMock: ReturnType<typeof vi.fn>;
  let mutationObserverMock: ReturnType<typeof vi.fn>;
  let disconnectCalls: number;

  beforeEach(() => {
    disconnectCalls = 0;
    const makeObserver = () => ({
      observe: vi.fn(),
      disconnect: vi.fn(() => {
        disconnectCalls += 1;
      }),
      unobserve: vi.fn(),
      takeRecords: vi.fn(() => []),
    });
    resizeObserverMock = vi.fn().mockImplementation(makeObserver);
    mutationObserverMock = vi.fn().mockImplementation(makeObserver);
    vi.stubGlobal("ResizeObserver", resizeObserverMock);
    vi.stubGlobal("MutationObserver", mutationObserverMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function renderWithTabRef(inner: HTMLElement) {
    return renderHook(() => {
      const ref = useRef<HTMLElement | null>(inner);
      usePinnedDockviewTab(ref as RefObject<HTMLElement | null>);
    });
  }

  it("adds the dv-pinned-tab class to the wrapping .dv-tab", () => {
    const container = document.createElement("div");
    const { dvTab, inner } = buildTab({ container, offsetWidth: 80 });

    renderWithTabRef(inner);

    expect(dvTab.classList.contains(PINNED_CLASS)).toBe(true);
  });

  it("computes the initial --dv-pin-offset on mount", () => {
    const container = document.createElement("div");
    const { dvTab, inner } = buildTab({ container, offsetWidth: 80 });

    renderWithTabRef(inner);

    expect(dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("0px");
  });

  it("registers a ResizeObserver on the .dv-tab and a MutationObserver on its parent", () => {
    const container = document.createElement("div");
    const { inner } = buildTab({ container, offsetWidth: 80 });

    renderWithTabRef(inner);

    expect(resizeObserverMock).toHaveBeenCalledOnce();
    expect(mutationObserverMock).toHaveBeenCalledOnce();
  });

  it("removes the class, CSS variable, and disconnects observers on unmount", () => {
    const container = document.createElement("div");
    const { dvTab, inner } = buildTab({ container, offsetWidth: 80 });

    const { unmount } = renderWithTabRef(inner);
    expect(dvTab.classList.contains(PINNED_CLASS)).toBe(true);
    expect(dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("0px");

    unmount();

    expect(dvTab.classList.contains(PINNED_CLASS)).toBe(false);
    expect(dvTab.style.getPropertyValue(PIN_OFFSET_VAR)).toBe("");
    // Both the resize + mutation observers should be disconnected.
    expect(disconnectCalls).toBe(2);
  });

  it("no-ops if the ref element is not inside a .dv-tab", () => {
    const orphan = document.createElement("span");

    expect(() => renderWithTabRef(orphan)).not.toThrow();
    // No observers should have been constructed.
    expect(resizeObserverMock).not.toHaveBeenCalled();
    expect(mutationObserverMock).not.toHaveBeenCalled();
  });
});
