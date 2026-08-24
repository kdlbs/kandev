import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mockAppState = {
  workspaces: { activeId: "ws-1" },
  kanban: { tasks: [] as Array<{ id: string; title: string; parentTaskId?: string }> },
  kanbanMulti: { snapshots: {} as Record<string, { tasks: Array<{ id: string }> }> },
  taskPRs: { byTaskId: {} as Record<string, unknown> },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockAppState) => unknown) => selector(mockAppState),
}));

vi.mock("@/components/github/pr-task-icon", () => ({
  PRTaskIcon: () => <span data-testid="pr-task-icon" />,
}));

vi.mock("@/components/gitlab/mr-task-icon", () => ({
  MRTaskIcon: () => <span data-testid="mr-task-icon" />,
}));

import { pluginRegistry } from "@/lib/plugins/registry";
import { KanbanCardBody, renderTaskStatusIcon } from "./kanban-card-content";
import type { Task } from "./kanban-card";

const TASK: Task = {
  id: "task-1",
  title: "Fix the bug",
  workflowStepId: "step-1",
};

const NOTES_PLUGIN_ID = "kandev-plugin-notes";
const SECOND_PLUGIN_ID = "kandev-plugin-second";
const TAGS_TEST_ID = "plugin-tags";
const TAGS_SLOT = "task-card-tags";
const INDICATOR_TEST_ID = "plugin-indicator";
const TITLE_TEST_ID = "task-card-title";
const SLOT_PROPS_TEXT = "task-1|ws-1|step-1";

function SlotPropsProbe({ testId, slotProps }: { testId: string; slotProps?: unknown }) {
  const props = slotProps as { taskId: string; workspaceId: string; workflowStepId: string };
  return (
    <span data-testid={testId}>
      {props.taskId}|{props.workspaceId}|{props.workflowStepId}
    </span>
  );
}

afterEach(() => {
  cleanup();
  mockAppState.kanban.tasks = [];
  pluginRegistry.unregisterPlugin(NOTES_PLUGIN_ID);
  pluginRegistry.unregisterPlugin(SECOND_PLUGIN_ID);
});

describe("KanbanCardBody — task-card-indicators slot", () => {
  it("renders no extra markup when no plugin is registered for the slot (AC14)", () => {
    const { container } = render(<KanbanCardBody task={TASK} repositoryChips={[]} />);
    expect(container.querySelector(`[data-testid="${INDICATOR_TEST_ID}"]`)).toBeNull();
  });

  it("renders a registered indicator with taskId/workspaceId/workflowStepId as slotProps (AC13)", () => {
    function Indicator({ slotProps }: { slotProps?: unknown }) {
      return <SlotPropsProbe testId={INDICATOR_TEST_ID} slotProps={slotProps} />;
    }
    pluginRegistry.forPlugin(NOTES_PLUGIN_ID).registerComponent("task-card-indicators", Indicator);

    render(<KanbanCardBody task={TASK} repositoryChips={[]} />);

    expect(screen.getByTestId(INDICATOR_TEST_ID).textContent).toBe(SLOT_PROPS_TEXT);
  });
});

describe("Kanban task status motion", () => {
  it("animates the fallback running status on an HTML wrapper", () => {
    const { container } = render(<>{renderTaskStatusIcon(TASK, true, false, false)}</>);
    const animated = container.querySelector(".animate-spin");

    expect(animated?.tagName).toBe("SPAN");
    const svg = animated?.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg?.classList.contains("animate-spin")).toBe(false);
  });
});

describe("KanbanCardBody — task-card-tags slot", () => {
  it("renders no extra markup when no plugin is registered for the slot (AC3)", () => {
    const { container } = render(<KanbanCardBody task={TASK} repositoryChips={[]} />);
    expect(container.querySelector(`[data-testid="${TAGS_TEST_ID}"]`)).toBeNull();
  });

  it("renders a registered tags component with taskId/workspaceId/workflowStepId as slotProps (AC2)", () => {
    function Tags({ slotProps }: { slotProps?: unknown }) {
      return <SlotPropsProbe testId={TAGS_TEST_ID} slotProps={slotProps} />;
    }
    pluginRegistry.forPlugin(NOTES_PLUGIN_ID).registerComponent(TAGS_SLOT, Tags);

    render(<KanbanCardBody task={TASK} repositoryChips={[]} />);

    expect(screen.getByTestId(TAGS_TEST_ID).textContent).toBe(SLOT_PROPS_TEXT);
  });

  it("renders as its own row, distinct from the title row that hosts task-card-indicators (AC2)", () => {
    function Tags() {
      return <span data-testid={TAGS_TEST_ID} />;
    }
    function Indicator() {
      return <span data-testid={INDICATOR_TEST_ID} />;
    }
    pluginRegistry.forPlugin(NOTES_PLUGIN_ID).registerComponent(TAGS_SLOT, Tags);
    pluginRegistry.forPlugin(NOTES_PLUGIN_ID).registerComponent("task-card-indicators", Indicator);

    render(<KanbanCardBody task={TASK} repositoryChips={[]} />);

    const titleRow = screen.getByTestId("kanban-card-title-row");
    const tags = screen.getByTestId(TAGS_TEST_ID);
    const indicator = screen.getByTestId(INDICATOR_TEST_ID);
    expect(titleRow.contains(indicator)).toBe(true);
    expect(titleRow.contains(tags)).toBe(false);
  });

  it("renders both plugins registered for the slot, in registration order (AC4)", () => {
    function First() {
      return <span data-testid="tags-first" />;
    }
    function Second() {
      return <span data-testid="tags-second" />;
    }
    pluginRegistry.forPlugin(NOTES_PLUGIN_ID).registerComponent(TAGS_SLOT, First);
    pluginRegistry.forPlugin(SECOND_PLUGIN_ID).registerComponent(TAGS_SLOT, Second);

    render(<KanbanCardBody task={TASK} repositoryChips={[]} />);

    const first = screen.getByTestId("tags-first");
    const second = screen.getByTestId("tags-second");
    expect(first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("isolates a throwing task-card-tags component so the card and other slot components still render (AC6)", () => {
    function ThrowingTags(): never {
      throw new Error("boom");
    }
    function OtherIndicator() {
      return <span data-testid={INDICATOR_TEST_ID} />;
    }
    // Suppress the expected React error-boundary console noise for this test.
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      pluginRegistry.forPlugin(NOTES_PLUGIN_ID).registerComponent(TAGS_SLOT, ThrowingTags);
      pluginRegistry
        .forPlugin(NOTES_PLUGIN_ID)
        .registerComponent("task-card-indicators", OtherIndicator);

      render(<KanbanCardBody task={TASK} repositoryChips={[]} />);

      expect(screen.getByTestId(TITLE_TEST_ID).textContent).toBe(TASK.title);
      expect(screen.getByTestId(INDICATOR_TEST_ID)).toBeTruthy();
    } finally {
      consoleError.mockRestore();
    }
  });
});

describe("KanbanCardBody — linked PR/MR badges (AC30)", () => {
  it("renders both the PR badge and the MR badge, PR first", () => {
    const { container } = render(<KanbanCardBody task={TASK} repositoryChips={[]} />);
    const title = screen.getByTestId(TITLE_TEST_ID);
    const row = title.parentElement;
    expect(row).not.toBeNull();
    const prIcon = screen.getByTestId("pr-task-icon");
    const mrIcon = screen.getByTestId("mr-task-icon");
    expect(container.contains(prIcon)).toBe(true);
    expect(container.contains(mrIcon)).toBe(true);
    // AC30: PR badge before MR badge in document order.
    expect(prIcon.compareDocumentPosition(mrIcon) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

describe("KanbanCardBody — title hover card gating", () => {
  it("does not mount a hover card trigger when enableTitleHover is unset (drag preview)", () => {
    render(<KanbanCardBody task={TASK} repositoryChips={[]} />);
    const title = screen.getByTestId(TITLE_TEST_ID);
    expect(title.getAttribute("data-slot")).not.toBe("hover-card-trigger");
  });

  it("does not mount a hover card trigger for a childless task", () => {
    render(<KanbanCardBody task={TASK} repositoryChips={[]} enableTitleHover />);
    const title = screen.getByTestId(TITLE_TEST_ID);
    expect(title.closest('[data-testid="task-title-preview-trigger"]')).toBeNull();
  });

  it("mounts the hover card trigger when an active direct subtask exists", () => {
    mockAppState.kanban.tasks = [{ id: "child-1", title: "Child", parentTaskId: TASK.id }];
    render(<KanbanCardBody task={TASK} repositoryChips={[]} enableTitleHover />);
    const title = screen.getByTestId(TITLE_TEST_ID);
    expect(title.closest('[data-testid="task-title-preview-trigger"]')).not.toBeNull();
  });
});
