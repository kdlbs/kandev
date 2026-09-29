import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const layoutState = vi.hoisted(() => ({
  shouldFloat: false,
  kanbanWidth: 640,
  containerWidth: 1000,
  calls: [] as Array<[boolean, number]>,
}));
const responsiveState = vi.hoisted(() => ({ isMobile: false, isFinePointer: true }));

vi.mock("@/hooks/use-kanban-layout", () => ({
  useKanbanLayout: (open: boolean, widthPx: number) => {
    layoutState.calls.push([open, widthPx]);
    return {
      containerRef: { current: null },
      shouldFloat: layoutState.shouldFloat,
      kanbanWidth: layoutState.kanbanWidth,
      containerWidth: layoutState.containerWidth,
    };
  },
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsiveState,
}));

import { RightSidePanel, type RightSidePanelProps } from "./right-side-panel";

const PANEL_ID = "test-panel";
const BACKDROP_ID = `${PANEL_ID}-backdrop`;
const MAIN_ID = "main-content";
const CONTENT_ID = "panel-content";
const RESIZE_HANDLE_ID = `${PANEL_ID}-resize-handle`;

function renderPanel(props: Partial<RightSidePanelProps> = {}) {
  const onClose = vi.fn();
  const onWidthChange = vi.fn();
  const utils = render(
    <RightSidePanel
      open
      onClose={onClose}
      widthPx={500}
      onWidthChange={onWidthChange}
      backdropLabel="Close panel"
      panelTestId={PANEL_ID}
      main={<button data-testid="main-content">main</button>}
      {...props}
    >
      <button data-testid="panel-content">panel</button>
    </RightSidePanel>,
  );
  return { ...utils, onClose, onWidthChange };
}

afterEach(() => {
  cleanup();
  layoutState.shouldFloat = false;
  layoutState.kanbanWidth = 640;
  layoutState.containerWidth = 1000;
  layoutState.calls = [];
  responsiveState.isMobile = false;
  responsiveState.isFinePointer = true;
});

describe("RightSidePanel layout", () => {
  it("renders the panel inline beside the main content without a backdrop", () => {
    renderPanel();
    expect(screen.getByTestId(MAIN_ID)).toBeTruthy();
    const panel = screen.getByTestId(PANEL_ID);
    expect(panel.className).toContain("border-l");
    expect(panel.className).not.toContain("fixed");
    expect(panel.style.width).toBe("500px");
    expect(screen.queryByTestId(BACKDROP_ID)).toBeNull();
  });

  it("floats over the main content with a backdrop that closes through onClose", () => {
    layoutState.shouldFloat = true;
    const { onClose } = renderPanel();
    const panel = screen.getByTestId(PANEL_ID);
    expect(panel.className).toContain("fixed");
    expect(panel.style.maxWidth).toBe("95vw");
    fireEvent.click(screen.getByTestId(BACKDROP_ID));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("renders only the main content while closed", () => {
    renderPanel({ open: false });
    expect(screen.getByTestId(MAIN_ID)).toBeTruthy();
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
  });

  it("sizes measured main content to the layout hook's width", () => {
    renderPanel();
    const measured = screen.getByTestId(MAIN_ID).parentElement as HTMLElement;
    expect(measured.style.width).toBe("640px");
  });

  it("keeps fluid main content unsized in both layouts, at one tree position", () => {
    const { rerender, onClose, onWidthChange } = renderPanel({ mainSizing: "fluid" });
    const mainBefore = screen.getByTestId(MAIN_ID);
    expect((mainBefore.parentElement as HTMLElement).style.width).toBe("");
    layoutState.shouldFloat = true;
    rerender(
      <RightSidePanel
        open
        onClose={onClose}
        widthPx={500}
        onWidthChange={onWidthChange}
        backdropLabel="Close panel"
        panelTestId={PANEL_ID}
        mainSizing="fluid"
        main={<button data-testid="main-content">main</button>}
      >
        <button data-testid="panel-content">panel</button>
      </RightSidePanel>,
    );
    expect(screen.getByTestId(MAIN_ID)).toBe(mainBefore);
    expect((mainBefore.parentElement as HTMLElement).style.width).toBe("");
  });

  it("keeps the panel content mounted across inline, floating and full screen relayouts", () => {
    const { rerender, onClose, onWidthChange } = renderPanel({
      mainSizing: "fluid",
      mobileFullScreen: true,
    });
    const contentBefore = screen.getByTestId(CONTENT_ID);
    const relayout = () =>
      rerender(
        <RightSidePanel
          open
          onClose={onClose}
          widthPx={500}
          onWidthChange={onWidthChange}
          backdropLabel="Close panel"
          panelTestId={PANEL_ID}
          mainSizing="fluid"
          mobileFullScreen
          main={<button data-testid="main-content">main</button>}
        >
          <button data-testid="panel-content">panel</button>
        </RightSidePanel>,
      );
    layoutState.shouldFloat = true;
    relayout();
    expect(screen.getByTestId(CONTENT_ID)).toBe(contentBefore);
    expect(screen.getByTestId(PANEL_ID).className).toContain("fixed");
    responsiveState.isMobile = true;
    relayout();
    expect(screen.getByTestId(CONTENT_ID)).toBe(contentBefore);
    expect(screen.queryByTestId(BACKDROP_ID)).toBeNull();
    layoutState.shouldFloat = false;
    responsiveState.isMobile = false;
    relayout();
    expect(screen.getByTestId(CONTENT_ID)).toBe(contentBefore);
    expect(screen.getByTestId(PANEL_ID).className).toContain("border-l");
  });
});

describe("RightSidePanel width clamp", () => {
  it("floors the rendered width to the fine-pointer minimum and feeds it to the layout rule", () => {
    renderPanel({ widthPx: 100 });
    expect(screen.getByTestId(PANEL_ID).style.width).toBe("320px");
    expect(layoutState.calls.at(-1)).toEqual([true, 320]);
  });

  it("floors the rendered width to the coarse-pointer minimum", () => {
    responsiveState.isFinePointer = false;
    renderPanel({ widthPx: 100 });
    expect(screen.getByTestId(PANEL_ID).style.width).toBe("380px");
  });

  it("resizes from the left edge and reports the floored width to the caller", () => {
    const { onWidthChange } = renderPanel({ widthPx: 500 });
    fireEvent.mouseDown(screen.getByTestId(RESIZE_HANDLE_ID), { clientX: 800 });
    fireEvent.mouseMove(window, { clientX: 700 });
    expect(onWidthChange).toHaveBeenLastCalledWith(600);
    fireEvent.mouseMove(window, { clientX: 1200 });
    expect(onWidthChange).toHaveBeenLastCalledWith(320);
    fireEvent.mouseUp(window);
    onWidthChange.mockClear();
    fireEvent.mouseMove(window, { clientX: 100 });
    expect(onWidthChange).not.toHaveBeenCalled();
  });
});

describe("RightSidePanel mobileFullScreen", () => {
  it("covers the viewport with no backdrop or handle below the mobile breakpoint", () => {
    responsiveState.isMobile = true;
    layoutState.shouldFloat = true;
    renderPanel({ mobileFullScreen: true, mainSizing: "fluid" });
    const panel = screen.getByTestId(PANEL_ID);
    expect(panel.className).toContain("fixed");
    expect(panel.className).toContain("inset-x-0");
    expect(panel.className).toContain("bottom-[var(--app-status-bar-height)]");
    expect(panel.style.width).toBe("");
    expect(screen.queryByTestId(BACKDROP_ID)).toBeNull();
    expect(screen.queryByTestId(RESIZE_HANDLE_ID)).toBeNull();
  });

  it("is a no-op at desktop widths", () => {
    renderPanel({ mobileFullScreen: true });
    expect(screen.getByTestId(RESIZE_HANDLE_ID)).toBeTruthy();
    expect(screen.getByTestId(PANEL_ID).style.width).toBe("500px");
  });

  it("is off by default: a mobile viewport keeps the ordinary layout", () => {
    responsiveState.isMobile = true;
    renderPanel();
    expect(screen.getByTestId(RESIZE_HANDLE_ID)).toBeTruthy();
    expect(screen.getByTestId(PANEL_ID).style.width).toBe("500px");
  });
});

describe("RightSidePanel closeOnEscape", () => {
  it("closes on Escape only while focus is inside the panel", () => {
    const { onClose } = renderPanel({ closeOnEscape: true });
    fireEvent.keyDown(screen.getByTestId(MAIN_ID), { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.keyDown(screen.getByTestId(CONTENT_ID), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("ignores an Escape that an inner layer already consumed", () => {
    const { onClose } = renderPanel({ closeOnEscape: true });
    const inner = screen.getByTestId(CONTENT_ID);
    inner.addEventListener("keydown", (event) => event.preventDefault());
    fireEvent.keyDown(inner, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
  });

  it("ignores other keys, and Escape when the caller did not opt in", () => {
    const { onClose } = renderPanel({ closeOnEscape: true });
    fireEvent.keyDown(screen.getByTestId(CONTENT_ID), { key: "Enter" });
    expect(onClose).not.toHaveBeenCalled();
    cleanup();
    const second = renderPanel();
    fireEvent.keyDown(screen.getByTestId(CONTENT_ID), { key: "Escape" });
    expect(second.onClose).not.toHaveBeenCalled();
  });
});
