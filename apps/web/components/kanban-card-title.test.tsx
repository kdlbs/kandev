import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { CardTitle } from "./kanban-card-title";

type StoredTask = KanbanState["tasks"][number];

const observerEntries: Array<{ element: Element; callback: ResizeObserverCallback }> = [];

class CapturingResizeObserver {
  private readonly callback: ResizeObserverCallback;

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }

  observe(element: Element) {
    observerEntries.push({ element, callback: this.callback });
  }

  disconnect() {}
  unobserve() {}
}

function setGeometry(
  element: Element,
  overrides: { scrollHeight?: number; clientHeight?: number },
) {
  if (overrides.scrollHeight !== undefined) {
    Object.defineProperty(element, "scrollHeight", {
      configurable: true,
      value: overrides.scrollHeight,
    });
  }
  if (overrides.clientHeight !== undefined) {
    Object.defineProperty(element, "clientHeight", {
      configurable: true,
      value: overrides.clientHeight,
    });
  }
}

function fireResize(element: Element) {
  for (const entry of observerEntries) {
    if (entry.element === element) {
      act(() => entry.callback([], {} as ResizeObserver));
    }
  }
}

const PREVIEW_TRIGGER_TEST_ID = "task-title-preview-trigger";
const HOVER_CARD_TEST_ID = "task-title-hover-card";

function makeTask(overrides: Partial<StoredTask> = {}): StoredTask {
  return {
    id: "task-1",
    title: "A task title",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    position: 0,
    ...overrides,
  };
}

function renderCardTitle(task: StoredTask, extraTasks: StoredTask[] = []) {
  render(
    <StateProvider
      initialState={{ kanban: { workflowId: "wf-1", steps: [], tasks: [task, ...extraTasks] } }}
    >
      <CardTitle task={task} enableTitleHover />
    </StateProvider>,
  );
}

beforeEach(() => {
  observerEntries.length = 0;
  vi.stubGlobal("ResizeObserver", CapturingResizeObserver);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function openCard() {
  const trigger = screen.getByTestId(PREVIEW_TRIGGER_TEST_ID);
  fireEvent.click(trigger, { detail: 0 });
  await screen.findAllByTestId(HOVER_CARD_TEST_ID);
}

describe("CardTitle", () => {
  it("passes the task's description into the hover card", async () => {
    renderCardTitle(makeTask({ description: "A task description" }));

    await openCard();
    const card = screen.getAllByTestId(HOVER_CARD_TEST_ID)[0];
    expect(card.textContent).toContain("A task description");
  });

  it("passes the task's parent relationship into the hover card", async () => {
    renderCardTitle(makeTask({ parentTaskId: "parent-1" }), [
      makeTask({ id: "parent-1", title: "Parent task title" }),
    ]);

    await openCard();
    const parentSection = screen.getAllByTestId("task-title-hover-parent")[0];
    expect(parentSection.textContent).toContain("Parent task title");
  });

  it("mounts the hover trigger once the title is measured as truncated, even with no other content", () => {
    renderCardTitle(makeTask());
    expect(screen.queryByTestId(PREVIEW_TRIGGER_TEST_ID)).toBeNull();

    const titleEl = screen.getByTestId("task-card-title");
    // line-clamp-1 clips vertically, not horizontally: a wrapping multi-word
    // title never grows scrollWidth past clientWidth, so truncation must be
    // measured on the height axis line-clamp actually clips.
    setGeometry(titleEl, { scrollHeight: 88, clientHeight: 18 });
    fireResize(titleEl);

    expect(screen.getByTestId(PREVIEW_TRIGGER_TEST_ID)).not.toBeNull();
  });
});
