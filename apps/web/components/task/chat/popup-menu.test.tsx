import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PopupMenu, PopupMenuItem, type PopupMenuProps } from "./popup-menu";
import { positionPopupMenu } from "./popup-menu-position";

let contentHeight = 120;

beforeEach(() => {
  contentHeight = 120;
  // Happy DOM does not lay out boxes. Supply measurements, but run the real
  // Floating UI DOM platform and middleware, including its WebKit conversion.
  vi.stubGlobal("CSS", { supports: vi.fn(() => false) });
  vi.spyOn(document.documentElement, "clientWidth", "get").mockReturnValue(1200);
  vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(800);
  vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockImplementation(function (
    this: HTMLElement,
  ) {
    return this.style.position === "fixed" ? parseFloat(this.style.width) || 420 : 0;
  });
  vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockImplementation(function (
    this: HTMLElement,
  ) {
    return this.style.position === "fixed"
      ? Math.min(contentHeight, parseFloat(this.style.maxHeight || "280"))
      : 0;
  });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
    this: HTMLElement,
  ) {
    if (this.style.position === "fixed") {
      return new DOMRect(
        parseFloat(this.style.left) || 0,
        parseFloat(this.style.top) || 0,
        this.offsetWidth,
        this.offsetHeight,
      );
    }
    return new DOMRect(0, 0, 0, this.parentElement?.style.position === "fixed" ? 32 : 0);
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  Object.defineProperty(window, "visualViewport", { configurable: true, value: undefined });
});

function setViewport(values: {
  offsetLeft?: number;
  offsetTop?: number;
  width: number;
  height: number;
}) {
  const viewport = Object.assign(new EventTarget(), { offsetLeft: 0, offsetTop: 0, ...values });
  Object.defineProperty(window, "visualViewport", { configurable: true, value: viewport });
  return viewport;
}

function mountMenu(props: Partial<PopupMenuProps> = {}) {
  const view = render(
    <PopupMenu
      isOpen
      testId="geometry-menu"
      position={{ x: 100, y: 700 }}
      title="References"
      selectedIndex={0}
      onClose={() => undefined}
      {...props}
    >
      Result
    </PopupMenu>,
  );
  return { ...view, menu: screen.getByTestId("geometry-menu") };
}

async function expectPositioned(menu: HTMLElement) {
  await waitFor(() => expect(menu.style.visibility).toBe("visible"));
}

describe("popup menu geometry", () => {
  it("keeps an above-composer menu inside an offset phone visual viewport", async () => {
    setViewport({ offsetLeft: 12, offsetTop: 100, width: 360, height: 500 });
    const { menu } = mountMenu({ position: { x: 350, y: 560 } });
    await expectPositioned(menu);
    expect(menu.style.left).toBe("20px");
    expect(menu.style.width).toBe("344px");
    expect(menu.style.maxHeight).toBe("280px");
    expect(menu.getBoundingClientRect().bottom).toBe(552);
  });

  it("uses a focused desktop width without the Visual Viewport API", async () => {
    const { menu } = mountMenu();
    await expectPositioned(menu);
    expect(menu.style.width).toBe("420px");
    expect(menu.getBoundingClientRect().bottom).toBe(692);
  });

  it("derives the padded viewport span even when the menu initially renders narrower", async () => {
    setViewport({ width: 600, height: 500 });
    const menu = document.createElement("div");
    Object.assign(menu.style, { position: "fixed", width: "180px" });
    document.body.append(menu);
    const stop = positionPopupMenu(menu, () => new DOMRect(100, 400, 1, 20), "above");
    try {
      await expectPositioned(menu);
      expect(menu.style.width).toBe("420px");
      expect(menu.style.maxHeight).toBe("280px");
    } finally {
      stop();
      menu.remove();
    }
  });

  it("keeps short results adjacent and shrinks long results before moving over the caret", async () => {
    setViewport({ width: 393, height: 420 });
    const { menu } = mountMenu({ position: { x: 17, y: 157 } });
    await expectPositioned(menu);
    expect(menu.getBoundingClientRect().bottom).toBe(149);
    contentHeight = 400;
    fireEvent(window, new Event("resize"));
    await waitFor(() => expect(menu.offsetHeight).toBe(141));
    expect(menu.getBoundingClientRect().bottom).toBe(149);
  });

  // @covers AC-UI-COMPOSER-OVERLAY-001.1
  it("clamps an above-composer menu to a software-keyboard visual viewport", async () => {
    setViewport({ width: 393, height: 420 });
    const { menu } = mountMenu({ position: { x: 16, y: 560 } });
    await expectPositioned(menu);
    expect(menu.getBoundingClientRect().bottom).toBe(412);
    expect(menu.getBoundingClientRect().top).toBeGreaterThanOrEqual(8);
  });

  it("preserves ordinary below-caret placement from the client rectangle's bottom", async () => {
    const { menu } = mountMenu({
      position: null,
      clientRect: () => new DOMRect(100, 80, 1, 20),
      placement: "below",
    });
    await expectPositioned(menu);
    expect(menu.style.top).toBe("108px");
    expect(menu.style.maxHeight).toBe("280px");
    expect(menu.style.transform).toBe("");
  });

  // @covers AC-UI-COMPOSER-OVERLAY-001.1
  it("normalizes Safari client coordinates before sizing against the viewport offset", async () => {
    vi.mocked(CSS.supports).mockImplementation(
      (property) => property === "-webkit-backdrop-filter",
    );
    setViewport({ offsetTop: 321, width: 393, height: 336 });
    const { menu } = mountMenu({
      position: null,
      clientRect: () => new DOMRect(17, 157, 1, 20),
    });
    await expectPositioned(menu);
    expect(parseFloat(menu.style.maxHeight)).toBe(141);
    // Fixed CSS coordinates include WebKit's 321px visual offset; raw client
    // coordinates do not. The bottom stays 8px above the normalized caret.
    expect(parseFloat(menu.style.top) + menu.offsetHeight).toBe(157 + 321 - 8);
  });

  it("keeps a header and touch row when a Chromium anchor is above the visible area", async () => {
    setViewport({ offsetTop: 700, width: 393, height: 150 });
    const { menu } = mountMenu({ position: { x: 17, y: 683 } });
    await expectPositioned(menu);
    expect(menu.offsetHeight).toBeGreaterThanOrEqual(84);
    expect(menu.getBoundingClientRect().top).toBeGreaterThanOrEqual(708);
    expect(menu.getBoundingClientRect().bottom).toBeLessThanOrEqual(842);
  });

  it("stays contained even when there is not enough room for a full row", async () => {
    setViewport({ offsetTop: 700, width: 393, height: 50 });
    const { menu } = mountMenu({ position: { x: 17, y: 683 } });
    await expectPositioned(menu);
    expect(menu.offsetHeight).toBe(34);
    expect(menu.getBoundingClientRect().top).toBe(708);
  });
});

describe("popup menu viewport updates", () => {
  it("refreshes a stable virtual caret when same-size results rerender", async () => {
    let caretY = 240;
    const clientRect = () => new DOMRect(16, caretY, 1, 20);
    const popup = (result: string) => (
      <PopupMenu
        isOpen
        position={null}
        clientRect={clientRect}
        testId="live-caret-menu"
        title="References"
        selectedIndex={0}
        onClose={vi.fn()}
      >
        {result}
      </PopupMenu>
    );
    const { rerender } = render(popup("First"));
    const menu = screen.getByTestId("live-caret-menu");
    await expectPositioned(menu);
    expect(menu.getBoundingClientRect().bottom).toBe(232);

    caretY = 200;
    rerender(popup("Later"));
    await waitFor(() => expect(menu.getBoundingClientRect().bottom).toBe(192));
  });

  it("reflows while the mobile visual viewport changes", async () => {
    const viewport = setViewport({ width: 360, height: 500 });
    const { menu } = mountMenu({ position: null, clientRect: () => new DOMRect(16, 240, 1, 20) });
    await expectPositioned(menu);
    expect(menu.style.width).toBe("344px");

    act(() => {
      viewport.width = 300;
      viewport.dispatchEvent(new Event("resize"));
    });

    await waitFor(() => expect(menu.style.width).toBe("284px"));
  });

  // @covers AC-UI-COMPOSER-OVERLAY-001.2
  it("reflows an open menu above the software keyboard", async () => {
    const viewport = setViewport({ width: 393, height: 600 });
    const { menu } = mountMenu({ position: { x: 16, y: 560 } });
    await expectPositioned(menu);
    expect(menu.getBoundingClientRect().bottom).toBe(552);

    act(() => {
      viewport.height = 420;
      viewport.dispatchEvent(new Event("resize"));
    });

    await waitFor(() => expect(menu.getBoundingClientRect().bottom).toBe(412));
    act(() => {
      viewport.offsetTop = 100;
      viewport.dispatchEvent(new Event("scroll"));
    });
    await waitFor(() => expect(menu.getBoundingClientRect().bottom).toBe(512));
  });

  it("removes subscriptions and ignores pending positioning after closing", async () => {
    const viewport = setViewport({ width: 393, height: 600 });
    const addListener = vi.spyOn(viewport, "addEventListener");
    const removeListener = vi.spyOn(viewport, "removeEventListener");
    const { menu, unmount } = mountMenu();
    unmount();
    await act(async () => {
      // A callback already queued by the browser must also be inert after cleanup.
      const resize = addListener.mock.calls.find(([event]) => event === "resize")?.[1];
      expect(resize).toBeDefined();
      (resize as EventListener)(new Event("resize"));
    });
    expect(menu.style.visibility).toBe("hidden");
    expect(menu.style.top).toBe("");
    expect(removeListener.mock.calls.map(([event]) => event)).toEqual(
      expect.arrayContaining(["resize", "scroll"]),
    );
  });

  it("does not render without an anchor", () => {
    render(
      <PopupMenu isOpen position={null} title="References" selectedIndex={0} onClose={vi.fn()}>
        Result
      </PopupMenu>,
    );
    expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  });

  it("positions when a live anchor becomes available without changing its callback", async () => {
    let rect: DOMRect | null = null;
    const clientRect = () => rect;
    const popup = () => (
      <PopupMenu
        isOpen
        position={null}
        clientRect={clientRect}
        title="References"
        selectedIndex={0}
        onClose={vi.fn()}
      >
        Result
      </PopupMenu>
    );
    const { rerender } = render(popup());
    expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
    rect = new DOMRect(16, 240, 1, 20);
    rerender(popup());
    expect(await screen.findByRole("listbox", { name: "References" })).toBeTruthy();
  });
});

describe("popup menu interaction", () => {
  it("exposes one labelled listbox with semantic selectable options", async () => {
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

    expect(await screen.findByRole("listbox", { name: "References" })).toBeTruthy();
    expect(
      screen
        .getByRole("option", { name: /#ENG-123.*Fix authentication/ })
        .getAttribute("aria-selected"),
    ).toBe("true");
  });

  it("keeps composer focus when an option is pressed", async () => {
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

    expect(fireEvent.pointerDown(await screen.findByRole("option"))).toBe(false);
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
