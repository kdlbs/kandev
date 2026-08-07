import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mockAppState = {
  workspaces: { activeId: "ws-1" },
  kanban: { tasks: [] as Array<{ id: string; title: string }> },
  taskPRs: { byTaskId: {} as Record<string, unknown> },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockAppState) => unknown) => selector(mockAppState),
}));

vi.mock("@/components/github/pr-task-icon", () => ({
  PRTaskIcon: () => <span data-testid="pr-task-icon" />,
}));

import { pluginRegistry } from "@/lib/plugins/registry";
import { KanbanCardBody } from "./kanban-card-content";
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

      expect(screen.getByTestId("task-card-title").textContent).toBe(TASK.title);
      expect(screen.getByTestId(INDICATOR_TEST_ID)).toBeTruthy();
    } finally {
      consoleError.mockRestore();
    }
  });
});
