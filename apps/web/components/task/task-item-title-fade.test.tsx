import { act, cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskItem } from "./task-item";

const observerEntries: Array<{ element: Element; callback: ResizeObserverCallback }> = [];
const DATA_TRUNCATED_ATTRIBUTE = "data-truncated";
const TRUNCATED = "true";
const NOT_TRUNCATED = "false";

class CapturingResizeObserver {
  constructor(private readonly callback: ResizeObserverCallback) {}

  observe(element: Element) {
    observerEntries.push({ element, callback: this.callback });
  }

  disconnect() {}
  unobserve() {}
}

function setGeometry(element: Element, scrollWidth: number, clientWidth: number) {
  Object.defineProperties(element, {
    scrollWidth: { configurable: true, value: scrollWidth },
    clientWidth: { configurable: true, value: clientWidth },
  });
}

function fireResize(element: Element) {
  const entry = [...observerEntries].reverse().find((candidate) => candidate.element === element);
  expect(entry, "TaskItem should observe title overflow changes").toBeDefined();
  if (!entry) return;
  act(() => entry.callback([], {} as ResizeObserver));
}

function renderTaskItem(title: string) {
  const props: ComponentProps<typeof TaskItem> = { title, state: "REVIEW" };
  return render(
    <StateProvider>
      <TooltipProvider>
        <TaskItem {...props} />
      </TooltipProvider>
    </StateProvider>,
  );
}

function titleElement(): HTMLSpanElement {
  const row = screen.getByTestId("sidebar-task-item");
  const title = row.querySelector<HTMLSpanElement>("span.overflow-hidden");
  if (!title) throw new Error("Task title element was not rendered");
  return title;
}

beforeEach(() => {
  observerEntries.length = 0;
  vi.stubGlobal("ResizeObserver", CapturingResizeObserver);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("TaskItem title fade", () => {
  it("marks clipped titles while preserving the complete title text", () => {
    const fullTitle = "Investigate a very long task title that continues past its row";
    renderTaskItem(fullTitle);

    const title = titleElement();
    setGeometry(title, 220, 120);
    fireResize(title);

    expect(title.getAttribute(DATA_TRUNCATED_ATTRIBUTE)).toBe(TRUNCATED);
    expect(title.textContent).toBe(fullTitle);
  });

  it("removes the clipped state when a resize gives the title enough width", () => {
    renderTaskItem("A title that initially overflows the row");

    const title = titleElement();
    setGeometry(title, 180, 120);
    fireResize(title);
    expect(title.getAttribute(DATA_TRUNCATED_ATTRIBUTE)).toBe(TRUNCATED);

    setGeometry(title, 120, 120);
    fireResize(title);

    expect(title.getAttribute(DATA_TRUNCATED_ATTRIBUTE)).toBe(NOT_TRUNCATED);
  });

  it("recomputes clipping when the title changes without a resize", () => {
    const { rerender } = renderTaskItem("Short title");

    const title = titleElement();
    setGeometry(title, 72, 120);
    fireResize(title);
    expect(title.getAttribute(DATA_TRUNCATED_ATTRIBUTE)).toBe(NOT_TRUNCATED);

    setGeometry(title, 210, 120);
    rerender(
      <StateProvider>
        <TooltipProvider>
          <TaskItem title="A longer title after a server update" state="REVIEW" />
        </TooltipProvider>
      </StateProvider>,
    );

    expect(title.getAttribute(DATA_TRUNCATED_ATTRIBUTE)).toBe(TRUNCATED);
    expect(title.textContent).toBe("A longer title after a server update");
  });
});
