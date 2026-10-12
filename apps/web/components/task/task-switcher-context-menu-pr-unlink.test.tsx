import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";
import type { TaskSwitcherItem } from "./task-switcher-types";

const WORKFLOW_ID = "workflow-1";
const STEP_ID = "step-1";
const workflowMoveMock = vi.hoisted(() => ({
  move: vi.fn(),
  isMoving: false,
}));
const unlinkMenuMock = vi.hoisted(() => ({
  choices: [] as Array<{
    associationId: string;
    owner: string;
    repo: string;
    number: number;
    pending: boolean;
  }>,
  isLoading: false,
  onOpenChange: vi.fn(),
  unlink: vi.fn(),
  canUnlink: true,
}));

vi.mock("@/hooks/domains/kanban/use-workflow-move", () => ({
  useWorkflowMove: () => workflowMoveMock,
}));
vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => false,
}));
vi.mock("@/hooks/domains/github/use-task-pr-unlink-menu", () => ({
  useTaskPRUnlinkMenu: () => unlinkMenuMock,
}));

afterEach(cleanup);

beforeEach(() => {
  workflowMoveMock.move.mockReset();
  workflowMoveMock.move.mockResolvedValue({ disposition: "committed", response: {} });
  unlinkMenuMock.choices = [];
  unlinkMenuMock.isLoading = false;
  unlinkMenuMock.onOpenChange.mockReset();
  unlinkMenuMock.unlink.mockReset();
});

function renderTaskRow(overrides: Partial<TaskSwitcherItem> = {}) {
  const task: TaskSwitcherItem = {
    id: "task-1",
    title: "Task 1",
    state: "IN_PROGRESS",
    ...overrides,
  };
  render(
    <StateProvider>
      <ToastProvider>
        <TaskItemWithContextMenu
          task={task}
          onArchiveTask={vi.fn()}
          onEditTask={vi.fn()}
          onRenameTask={vi.fn()}
        >
          <div data-testid="task-row">Task 1</div>
        </TaskItemWithContextMenu>
      </ToastProvider>
    </StateProvider>,
  );
}

async function openContextMenu() {
  fireEvent.contextMenu(screen.getByTestId("task-row"));
  await screen.findByRole("menuitem", { name: /color/i });
}

describe("TaskItemWithContextMenu PR unlink actions", () => {
  it("hydrates on menu open and keeps unlink choices inside Edit", async () => {
    unlinkMenuMock.choices = [
      {
        associationId: "association-api-42",
        owner: "acme",
        repo: "api",
        number: 42,
        pending: false,
      },
    ];
    renderTaskRow({
      prInfo: { number: 42, state: "Open" },
      workflowId: WORKFLOW_ID,
      workflowStepId: STEP_ID,
    });

    await openContextMenu();

    expect(unlinkMenuMock.onOpenChange).toHaveBeenCalledWith(true);
    const edit = screen.getByTestId("task-row-edit-submenu");
    fireEvent.pointerMove(edit, { pointerType: "mouse" });
    expect(await screen.findByRole("menuitem", { name: "Edit task" })).not.toBeNull();
    expect(
      await screen.findByRole("menuitem", { name: "Remove acme/api #42 from task" }),
    ).not.toBeNull();
    expect(screen.getByRole("menuitem", { name: "Rename" })).not.toBeNull();
    expect(screen.getByRole("menuitem", { name: "Duplicate" })).not.toBeNull();
  });

  it("shows a disabled loading row while task PR associations hydrate", async () => {
    unlinkMenuMock.isLoading = true;
    renderTaskRow({ prInfo: { number: 42, state: "Open" } });

    await openContextMenu();
    fireEvent.pointerMove(screen.getByTestId("task-row-edit-submenu"), { pointerType: "mouse" });

    const loadingItem = await screen.findByRole("menuitem", {
      name: "Loading pull requests...",
    });
    expect(loadingItem.getAttribute("aria-disabled")).toBe("true");
  });
});
