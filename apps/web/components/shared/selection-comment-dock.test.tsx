import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useRef, type ReactNode } from "react";
import { SelectionCommentDock, SelectionCommentHostProvider } from "./selection-comment-dock";

const responsive = vi.hoisted(() => ({ isMobile: false, isFinePointer: true }));
const COMMENT_LABEL = "Comment";
const COMMENT_ACTION_NAME = "Comment on selection";
const SELECTION_COMMENT_DOCK_TEST_ID = "selection-comment-dock";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ ...responsive }),
}));

const box = (left: number, top: number, width: number, height: number) => ({
  left,
  top,
  right: left + width,
  bottom: top + height,
  width,
  height,
  x: left,
  y: top,
  toJSON: () => ({}),
});

function Host({ children }: { children: ReactNode }) {
  const ownerRef = useRef<HTMLDivElement>(null);
  const boundaryRef = useRef<HTMLDivElement>(null);
  const reserveRef = useRef<HTMLDivElement>(null);
  const ownerRect = box(40, 100, 320, 660);
  const boundaryRect = box(40, 620, 320, 140);
  return (
    <SelectionCommentHostProvider
      ownerRef={ownerRef}
      boundaryRef={boundaryRef}
      reserveRef={reserveRef}
    >
      <div
        ref={(element) => {
          ownerRef.current = element;
          if (element) vi.spyOn(element, "getBoundingClientRect").mockReturnValue(ownerRect);
        }}
        data-testid="owner"
        style={{ height: 600 }}
      >
        <div ref={reserveRef} data-testid="reserve" style={{ paddingBottom: "12px" }} />
        <div
          ref={(element) => {
            boundaryRef.current = element;
            if (element) vi.spyOn(element, "getBoundingClientRect").mockReturnValue(boundaryRect);
          }}
          data-testid="boundary"
        />
        {children}
      </div>
    </SelectionCommentHostProvider>
  );
}

afterEach(() => {
  cleanup();
  responsive.isMobile = false;
  responsive.isFinePointer = true;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("SelectionCommentDock", () => {
  it("renders for a coarse pointer inside its owner and clears the composer", () => {
    responsive.isFinePointer = false;
    const onOpen = vi.fn();
    render(
      <Host>
        <SelectionCommentDock
          active
          label={COMMENT_LABEL}
          accessibleName={COMMENT_ACTION_NAME}
          onOpen={onOpen}
        />
      </Host>,
    );

    const reserve = screen.getByTestId("reserve");
    const dock = screen.getByTestId(SELECTION_COMMENT_DOCK_TEST_ID);
    const button = screen.getByRole("button", { name: COMMENT_ACTION_NAME });
    expect(dock).not.toBeNull();
    expect(button.textContent).toContain(COMMENT_LABEL);
    expect(button.getAttribute("style")).toContain("min-height: 44px");
    expect(dock.getAttribute("style")).toContain("left: 120px");
    expect(dock.getAttribute("style")).toContain("top: 564px");
    expect((reserve as HTMLDivElement).style.paddingBottom).toBe("calc(12px + 56px)");

    fireEvent.pointerDown(button);
    fireEvent.click(button);
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it("uses the phone dock even with a fine pointer and restores reserved space on close", () => {
    responsive.isMobile = true;
    const { rerender } = render(
      <Host>
        <SelectionCommentDock
          active
          label={COMMENT_LABEL}
          accessibleName={COMMENT_ACTION_NAME}
          onOpen={vi.fn()}
        />
      </Host>,
    );
    const reserve = screen.getByTestId("reserve") as HTMLDivElement;

    expect(screen.getByTestId(SELECTION_COMMENT_DOCK_TEST_ID)).not.toBeNull();
    expect(reserve.style.paddingBottom).toBe("calc(12px + 56px)");

    rerender(
      <Host>
        <SelectionCommentDock
          active={false}
          label={COMMENT_LABEL}
          accessibleName={COMMENT_ACTION_NAME}
          onOpen={vi.fn()}
        />
      </Host>,
    );

    expect(screen.queryByTestId(SELECTION_COMMENT_DOCK_TEST_ID)).toBeNull();
    expect((screen.getByTestId("reserve") as HTMLDivElement).style.paddingBottom).toBe("12px");
  });

  it("does not mount for a wide fine-pointer surface", () => {
    render(
      <Host>
        <SelectionCommentDock
          active
          label={COMMENT_LABEL}
          accessibleName={COMMENT_ACTION_NAME}
          onOpen={vi.fn()}
        />
      </Host>,
    );

    expect(screen.queryByTestId(SELECTION_COMMENT_DOCK_TEST_ID)).toBeNull();
  });

  it("disconnects geometry observers and event listeners when the dock closes", () => {
    responsive.isMobile = true;
    const disconnect = vi.fn();
    class MockResizeObserver {
      observe = vi.fn();
      disconnect = disconnect;
      unobserve = vi.fn();
      constructor(_callback: ResizeObserverCallback) {}
    }
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    const removeSpy = vi.spyOn(window, "removeEventListener");
    const { unmount } = render(
      <Host>
        <SelectionCommentDock
          active
          label={COMMENT_LABEL}
          accessibleName={COMMENT_ACTION_NAME}
          onOpen={vi.fn()}
        />
      </Host>,
    );

    unmount();

    expect(disconnect).toHaveBeenCalled();
    expect(removeSpy).toHaveBeenCalledWith("resize", expect.any(Function));
    expect(removeSpy).toHaveBeenCalledWith("scroll", expect.any(Function), true);
  });
});
