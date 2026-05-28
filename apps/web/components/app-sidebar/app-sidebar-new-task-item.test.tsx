import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";

function renderItem(collapsed: boolean) {
  return render(
    <TooltipProvider>
      <AppSidebarNewTaskItem collapsed={collapsed} />
    </TooltipProvider>,
  );
}

const state = {
  workspaces: { activeId: "ws-1" as string | null },
  kanban: {
    workflowId: "wf-1" as string | null,
    steps: [{ id: "s1", title: "Todo" }],
    tasks: [{ id: "t-1", title: "Parent task" }] as Array<{ id: string; title: string }>,
  },
  tasks: { activeTaskId: null as string | null },
};
let officeEnabled = false;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => officeEnabled,
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => "/",
}));
vi.mock("@/app/office/components/new-task-dialog", () => ({
  NewTaskDialog: () => <div data-testid="office-new-task-dialog" />,
}));
vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: () => <div data-testid="regular-task-create-dialog" />,
}));
vi.mock("@/components/task/new-subtask-dialog", () => ({
  NewSubtaskDialog: () => <div data-testid="new-subtask-dialog" />,
}));

import { AppSidebarNewTaskItem } from "./app-sidebar-new-task-item";

const SUBTASK_TESTID = "sidebar-new-subtask";

describe("AppSidebarNewTaskItem", () => {
  beforeEach(() => {
    state.workspaces.activeId = "ws-1";
    state.kanban.workflowId = "wf-1";
    state.kanban.steps = [{ id: "s1", title: "Todo" }];
    state.kanban.tasks = [{ id: "t-1", title: "Parent task" }];
    state.tasks.activeTaskId = null;
    officeEnabled = false;
  });

  afterEach(() => cleanup());

  it("uses the regular task-create dialog when office is disabled", () => {
    officeEnabled = false;
    renderItem(false);
    expect(screen.getByTestId("regular-task-create-dialog")).toBeTruthy();
    expect(screen.queryByTestId("office-new-task-dialog")).toBeNull();
  });

  it("uses the office new-issue dialog when office is enabled", () => {
    officeEnabled = true;
    renderItem(false);
    expect(screen.getByTestId("office-new-task-dialog")).toBeTruthy();
    expect(screen.queryByTestId("regular-task-create-dialog")).toBeNull();
  });

  it("renders no dialog when there is no active workspace", () => {
    state.workspaces.activeId = null;
    renderItem(false);
    expect(screen.queryByTestId("regular-task-create-dialog")).toBeNull();
    expect(screen.queryByTestId("office-new-task-dialog")).toBeNull();
  });

  it("offers a subtask affordance when a task is active in regular mode", () => {
    state.tasks.activeTaskId = "t-1";
    renderItem(false);
    expect(screen.getByTestId(SUBTASK_TESTID)).toBeTruthy();
    expect(screen.getByTestId("new-subtask-dialog")).toBeTruthy();
  });

  it("hides the subtask affordance when no task is active", () => {
    state.tasks.activeTaskId = null;
    renderItem(false);
    expect(screen.queryByTestId(SUBTASK_TESTID)).toBeNull();
  });

  it("hides the subtask affordance in office mode", () => {
    officeEnabled = true;
    state.tasks.activeTaskId = "t-1";
    renderItem(false);
    expect(screen.queryByTestId(SUBTASK_TESTID)).toBeNull();
  });

  it("hides the subtask affordance when the rail is collapsed", () => {
    state.tasks.activeTaskId = "t-1";
    renderItem(true);
    expect(screen.queryByTestId(SUBTASK_TESTID)).toBeNull();
  });
});
