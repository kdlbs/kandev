import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as popupMenu from "./popup-menu";
import { computePopupMenuStyle, PopupMenu, PopupMenuItem } from "./popup-menu";

afterEach(() => {
  cleanup();
  Object.defineProperty(window, "visualViewport", { configurable: true, value: undefined });
});

describe("popup menu geometry", () => {
  it("provides a visual-viewport-aware geometry helper", () => {
    expect(typeof (popupMenu as Record<string, unknown>).computePopupMenuStyle).toBe("function");
  });

  it("keeps an above-composer menu inside an offset phone visual viewport", () => {
    const style = computePopupMenuStyle({
      position: { x: 350, y: 560 },
      placement: "above",
      viewport: { offsetLeft: 12, offsetTop: 100, width: 360, height: 500 },
    });

    expect(style.left).toBe(20);
    expect(style.width).toBe(344);
    expect(style.top).toBe(552);
    expect(style.maxHeight).toBe(280);
    expect(Number(style.top) - Number(style.maxHeight)).toBe(272);
    expect(style.transform).toBe("translateY(-100%)");
    expect(style.bottom).toBeUndefined();
  });

  it("uses a focused desktop width instead of spanning the available viewport", () => {
    const style = computePopupMenuStyle({
      position: { x: 100, y: 700 },
      placement: "above",
      viewport: { offsetLeft: 0, offsetTop: 0, width: 1200, height: 800 },
    });

    expect(style.width).toBe(420);
  });

  it("anchors the rendered bottom edge instead of reserving unused maximum height", () => {
    const style = computePopupMenuStyle({
      position: { x: 100, y: 700 },
      placement: "above",
      viewport: { offsetLeft: 0, offsetTop: 0, width: 1200, height: 800 },
    });

    expect(style.top).toBe(692);
    expect(style.transform).toBe("translateY(-100%)");
  });

  it("reflows while the mobile visual viewport changes", () => {
    const viewport = Object.assign(new EventTarget(), {
      offsetLeft: 0,
      offsetTop: 0,
      width: 360,
      height: 500,
    });
    Object.defineProperty(window, "visualViewport", { configurable: true, value: viewport });
    render(
      <PopupMenu
        isOpen
        position={null}
        clientRect={() => new DOMRect(16, 240, 1, 20)}
        title="References"
        selectedIndex={0}
        onClose={() => undefined}
      >
        Result
      </PopupMenu>,
    );
    const menu = screen.getByText("References").parentElement?.parentElement;
    expect(menu?.style.width).toBe("344px");

    act(() => {
      viewport.width = 300;
      viewport.dispatchEvent(new Event("resize"));
    });

    expect(menu?.style.width).toBe("284px");
  });
});

describe("popup menu interaction", () => {
  it("exposes one labelled listbox with semantic selectable options", () => {
    render(
      <PopupMenu
        isOpen
        position={{ x: 16, y: 240 }}
        title="References"
        selectedIndex={0}
        onClose={() => undefined}
      >
        <PopupMenuItem
          icon={<span aria-hidden="true">#</span>}
          label="#ENG-123"
          description="Fix authentication"
          isSelected
          onClick={() => undefined}
          onMouseEnter={() => undefined}
        />
      </PopupMenu>,
    );

    expect(screen.getByRole("listbox", { name: "References" })).toBeTruthy();
    expect(
      screen
        .getByRole("option", { name: /#ENG-123.*Fix authentication/ })
        .getAttribute("aria-selected"),
    ).toBe("true");
  });

  it("keeps composer focus when an option is pressed", () => {
    render(
      <PopupMenu
        isOpen
        position={{ x: 16, y: 240 }}
        title="References"
        selectedIndex={0}
        onClose={() => undefined}
      >
        <PopupMenuItem
          icon={<span aria-hidden="true">#</span>}
          label="#ENG-123"
          isSelected
          onClick={() => undefined}
          onMouseEnter={() => undefined}
        />
      </PopupMenu>,
    );

    expect(fireEvent.pointerDown(screen.getByRole("option"))).toBe(false);
  });

  it("dismisses from an outside pointer press", () => {
    const onClose = vi.fn();
    render(
      <PopupMenu
        isOpen
        position={{ x: 16, y: 240 }}
        title="References"
        selectedIndex={0}
        onClose={onClose}
      >
        Result
      </PopupMenu>,
    );

    fireEvent.pointerDown(document.body);

    expect(onClose).toHaveBeenCalledOnce();
  });
});
