import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ChangesTimelineViewport } from "./changes-timeline-viewport";

type TestRow = { key: string; index: number };
const TIMELINE_ROW_SELECTOR = "[data-changes-timeline-row]";
const FIRST_ROW_TEST_ID = "focus-row-0";
const resizeObservers: ControlledResizeObserver[] = [];

class ControlledResizeObserver {
  private readonly targets = new Set<Element>();

  constructor(private readonly callback: ResizeObserverCallback) {
    resizeObservers.push(this);
  }

  observe(target: Element) {
    this.targets.add(target);
  }

  unobserve(target: Element) {
    this.targets.delete(target);
  }

  disconnect() {
    this.targets.clear();
  }

  emit() {
    const entries = [...this.targets].map((target) => ({
      target,
      contentRect: { width: 800, height: 600 },
      borderBoxSize: [{ inlineSize: 800, blockSize: 600 }],
    })) as unknown as ResizeObserverEntry[];
    this.callback(entries, this as unknown as ResizeObserver);
  }
}

function NavigationViewport({
  rows,
  contextKey = "task-a/session-a/environment-a",
  focusRequest,
}: {
  rows: TestRow[];
  contextKey?: string;
  focusRequest?: { rowKey: string; token: number };
}) {
  const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(null);
  return (
    <div
      ref={setScrollElement}
      data-testid="scroll-owner"
      style={{ height: 600, overflow: "auto" }}
    >
      <ChangesTimelineViewport
        rows={rows}
        scrollElement={scrollElement}
        contextKey={contextKey}
        focusRequest={focusRequest}
        estimateSize={() => 28}
        renderRow={(row) => (
          <button data-changes-row-focus data-testid={`focus-row-${row.index}`}>
            Row {row.index}
          </button>
        )}
      />
    </div>
  );
}

beforeEach(() => {
  resizeObservers.length = 0;
  vi.stubGlobal("ResizeObserver", ControlledResizeObserver);
});

afterEach(() => {
  cleanup();
  resizeObservers.length = 0;
  vi.unstubAllGlobals();
});

describe("ChangesTimelineViewport interactions", () => {
  it("moves focus to logical rows and retains one owner while a menu is open", async () => {
    const rows = Array.from({ length: 2_000 }, (_, index) => ({ key: `row-${index}`, index }));
    const view = render(<NavigationViewport rows={rows} />);
    const scrollOwner = screen.getByTestId("scroll-owner");

    act(() => resizeObservers.forEach((observer) => observer.emit()));
    await waitFor(() => expect(screen.getByTestId(FIRST_ROW_TEST_ID)).toBeTruthy());
    screen.getByTestId(FIRST_ROW_TEST_ID).focus();
    fireEvent.keyDown(screen.getByTestId(FIRST_ROW_TEST_ID), { key: "End" });

    await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("focus-row-1999")));
    expect(view.container.querySelectorAll(TIMELINE_ROW_SELECTOR).length).toBeLessThanOrEqual(120);

    const menu = document.createElement("div");
    menu.setAttribute("role", "menu");
    const menuItem = document.createElement("button");
    menuItem.setAttribute("role", "menuitem");
    menu.append(menuItem);
    document.body.append(menu);
    menuItem.focus();
    Object.defineProperty(scrollOwner, "scrollHeight", { configurable: true, value: 56_000 });
    Object.defineProperty(scrollOwner, "clientHeight", { configurable: true, value: 600 });
    act(() => {
      scrollOwner.scrollTop = 0;
      scrollOwner.dispatchEvent(new Event("scroll"));
    });

    await waitFor(() => expect(screen.getByTestId("focus-row-1999")).toBeTruthy());
    expect(view.container.querySelectorAll(TIMELINE_ROW_SELECTOR).length).toBeLessThanOrEqual(120);
    menu.remove();
  });

  it("focuses an externally requested row after the timeline changes", async () => {
    const rows = Array.from({ length: 2_000 }, (_, index) => ({ key: `row-${index}`, index }));
    const view = render(<NavigationViewport rows={rows} />);
    const scrollOwner = screen.getByTestId("scroll-owner");
    Object.defineProperty(scrollOwner, "scrollHeight", { configurable: true, value: 56_000 });
    Object.defineProperty(scrollOwner, "clientHeight", { configurable: true, value: 600 });
    act(() => resizeObservers.forEach((observer) => observer.emit()));
    await waitFor(() => expect(screen.getByTestId(FIRST_ROW_TEST_ID)).toBeTruthy());

    view.rerender(
      <NavigationViewport rows={rows} focusRequest={{ rowKey: "row-1999", token: 1 }} />,
    );

    await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("focus-row-1999")));
    expect(view.container.querySelectorAll(TIMELINE_ROW_SELECTOR).length).toBeLessThanOrEqual(120);
  });

  it("clears retained focus and scroll when task or environment context changes", async () => {
    const rows = Array.from({ length: 500 }, (_, index) => ({ key: `row-${index}`, index }));
    const view = render(<NavigationViewport rows={rows} />);
    const scrollOwner = screen.getByTestId("scroll-owner");
    act(() => resizeObservers.forEach((observer) => observer.emit()));
    await waitFor(() => expect(screen.getByTestId(FIRST_ROW_TEST_ID)).toBeTruthy());
    screen.getByTestId(FIRST_ROW_TEST_ID).focus();
    fireEvent.keyDown(screen.getByTestId(FIRST_ROW_TEST_ID), { key: "End" });
    await waitFor(() => expect(screen.getByTestId("focus-row-499")).toBeTruthy());
    expect(document.activeElement).toBe(screen.getByTestId("focus-row-499"));

    view.rerender(<NavigationViewport rows={rows} contextKey="task-b/session-b/environment-b" />);

    await waitFor(() => expect(scrollOwner.scrollTop).toBe(0));
    await waitFor(() => expect(screen.queryByTestId("focus-row-499")).toBeNull());
    expect(document.activeElement).not.toBe(screen.getByTestId(FIRST_ROW_TEST_ID));
  });

  it("moves focus to the nearest surviving row when its owner is removed", async () => {
    const rows = Array.from({ length: 40 }, (_, index) => ({ key: `row-${index}`, index }));
    const view = render(<NavigationViewport rows={rows} />);
    act(() => resizeObservers.forEach((observer) => observer.emit()));
    await waitFor(() => expect(screen.getByTestId("focus-row-10")).toBeTruthy());
    screen.getByTestId("focus-row-10").focus();

    const updatedRows = rows.filter((row) => row.index !== 10);
    view.rerender(<NavigationViewport rows={updatedRows} />);

    await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("focus-row-11")));
    expect(screen.queryByTestId("focus-row-10")).toBeNull();
  });
});
