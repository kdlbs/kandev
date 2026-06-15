import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";

const mocks = vi.hoisted(() => ({
  routerPush: vi.fn(),
  setActiveTask: vi.fn(),
  setActiveSession: vi.fn(),
  openQuickChat: vi.fn(),
  dialogTaskSessionId: null as string | null,
  dialogWillNavigate: false,
}));

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
  setActiveTask: mocks.setActiveTask,
  setActiveSession: mocks.setActiveSession,
};
let officeEnabled = false;
let pathname = "/";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));
vi.mock("@/hooks/use-quick-chat-launcher", () => ({
  useQuickChatLauncher: () => mocks.openQuickChat,
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => officeEnabled,
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.routerPush }),
  usePathname: () => pathname,
}));
vi.mock("@/app/office/components/new-task-dialog", () => ({
  NewTaskDialog: () => <div data-testid="office-new-task-dialog" />,
}));
vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: ({
    onSuccess,
  }: {
    onSuccess?: (
      task: { id: string },
      mode: "create" | "edit",
      meta?: { taskSessionId?: string | null; willNavigate?: boolean },
    ) => void;
  }) => (
    <button
      type="button"
      data-testid="regular-task-create-dialog"
      onClick={() =>
        onSuccess?.({ id: "t-new" }, "create", {
          taskSessionId: mocks.dialogTaskSessionId,
          willNavigate: mocks.dialogWillNavigate,
        })
      }
    >
      regular dialog
    </button>
  ),
}));
vi.mock("@/components/task/new-subtask-dialog", () => ({
  NewSubtaskDialog: () => <div data-testid="new-subtask-dialog" />,
}));

import { AppSidebarNewTaskItem } from "./app-sidebar-new-task-item";

const SUBTASK_TESTID = "sidebar-new-subtask";
const OFFICE_DIALOG_TESTID = "office-new-task-dialog";
const REGULAR_DIALOG_TESTID = "regular-task-create-dialog";

function resetTestState() {
  state.workspaces.activeId = "ws-1";
  state.kanban.workflowId = "wf-1";
  state.kanban.steps = [{ id: "s1", title: "Todo" }];
  state.kanban.tasks = [{ id: "t-1", title: "Parent task" }];
  state.tasks.activeTaskId = null;
  mocks.routerPush.mockClear();
  mocks.setActiveTask.mockClear();
  mocks.setActiveSession.mockClear();
  mocks.openQuickChat.mockClear();
  mocks.dialogTaskSessionId = null;
  mocks.dialogWillNavigate = false;
  officeEnabled = false;
  pathname = "/";
}

beforeEach(resetTestState);
afterEach(() => cleanup());

describe("AppSidebarNewTaskItem dialog routing", () => {
  it("uses the regular task-create dialog when office is disabled", () => {
    officeEnabled = false;
    renderItem(false);
    expect(screen.getByTestId(REGULAR_DIALOG_TESTID)).toBeTruthy();
    expect(screen.queryByTestId(OFFICE_DIALOG_TESTID)).toBeNull();
  });

  it("uses the regular dialog when office is enabled but NOT on an office route", () => {
    // The bug: office-on alone routed to the Office dialog even in Kanban mode.
    // Gating is now on the actual /office route, so home stays on the Kanban dialog.
    officeEnabled = true;
    pathname = "/";
    renderItem(false);
    expect(screen.getByTestId(REGULAR_DIALOG_TESTID)).toBeTruthy();
    expect(screen.queryByTestId(OFFICE_DIALOG_TESTID)).toBeNull();
  });

  it("uses the office new-issue dialog when inside an office route", async () => {
    officeEnabled = true;
    pathname = "/office";
    renderItem(false);
    // NewTaskDialog is lazy-loaded (next/dynamic), so it resolves asynchronously.
    expect(await screen.findByTestId(OFFICE_DIALOG_TESTID)).toBeTruthy();
    expect(screen.queryByTestId(REGULAR_DIALOG_TESTID)).toBeNull();
  });

  it("renders no dialog when there is no active workspace", () => {
    state.workspaces.activeId = null;
    renderItem(false);
    expect(screen.queryByTestId(REGULAR_DIALOG_TESTID)).toBeNull();
    expect(screen.queryByTestId(OFFICE_DIALOG_TESTID)).toBeNull();
  });
});

describe("AppSidebarNewTaskItem row actions", () => {
  it("opens quick chat from the trailing action beside New Task", () => {
    renderItem(false);
    screen.getByTestId("sidebar-quick-chat-shortcut").click();
    expect(mocks.openQuickChat).toHaveBeenCalledOnce();
  });

  it("hides the quick chat shortcut when the rail is collapsed", () => {
    renderItem(true);
    expect(screen.queryByTestId("sidebar-quick-chat-shortcut")).toBeNull();
  });

  it("hides the quick chat shortcut when there is no active workspace", () => {
    state.workspaces.activeId = null;
    renderItem(false);
    expect(screen.queryByTestId("sidebar-quick-chat-shortcut")).toBeNull();
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

  it("offers the subtask affordance in office mode too (compact subtask dialog)", async () => {
    officeEnabled = true;
    pathname = "/office";
    state.tasks.activeTaskId = "t-1";
    renderItem(false);
    // Primary New Task uses the office dialog (lazy-loaded), but subtasks still
    // go through the compact NewSubtaskDialog regardless of mode.
    expect(await screen.findByTestId(OFFICE_DIALOG_TESTID)).toBeTruthy();
    expect(screen.getByTestId(SUBTASK_TESTID)).toBeTruthy();
    expect(screen.getByTestId("new-subtask-dialog")).toBeTruthy();
  });

  it("hides the subtask affordance when the rail is collapsed", () => {
    state.tasks.activeTaskId = "t-1";
    renderItem(true);
    expect(screen.queryByTestId(SUBTASK_TESTID)).toBeNull();
  });
});

describe("AppSidebarNewTaskItem creation success", () => {
  it("focuses the created task after regular sidebar task creation succeeds", () => {
    renderItem(false);
    screen.getByTestId(REGULAR_DIALOG_TESTID).click();
    expect(mocks.setActiveTask).toHaveBeenCalledWith("t-new");
    expect(mocks.setActiveSession).not.toHaveBeenCalled();
    expect(mocks.routerPush).toHaveBeenCalledWith("/t/t-new");
  });

  it("focuses the created session after starting a sidebar task with an agent", () => {
    mocks.dialogTaskSessionId = "s-new";
    renderItem(false);
    screen.getByTestId(REGULAR_DIALOG_TESTID).click();
    expect(mocks.setActiveSession).toHaveBeenCalledWith("t-new", "s-new");
    expect(mocks.setActiveTask).not.toHaveBeenCalled();
    expect(mocks.routerPush).toHaveBeenCalledWith("/t/t-new");
  });

  it("does not push twice when the regular task dialog already navigates", () => {
    mocks.dialogWillNavigate = true;
    renderItem(false);
    screen.getByTestId(REGULAR_DIALOG_TESTID).click();
    expect(mocks.setActiveTask).toHaveBeenCalledWith("t-new");
    expect(mocks.routerPush).not.toHaveBeenCalled();
  });

  it("focuses the created session without pushing when the dialog already navigates", () => {
    mocks.dialogTaskSessionId = "s-new";
    mocks.dialogWillNavigate = true;
    renderItem(false);
    screen.getByTestId(REGULAR_DIALOG_TESTID).click();
    expect(mocks.setActiveSession).toHaveBeenCalledWith("t-new", "s-new");
    expect(mocks.setActiveTask).not.toHaveBeenCalled();
    expect(mocks.routerPush).not.toHaveBeenCalled();
  });
});
