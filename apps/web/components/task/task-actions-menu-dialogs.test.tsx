import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { useTaskActionsMenu, type TaskActionsMenuBoardRow } from "@/hooks/use-task-actions-menu";
import { TaskActionsMenuDialogs } from "./task-actions-menu-dialogs";

const capturedEditDialogProps: Array<{ open: boolean; focusReturnRef?: { current: unknown } }> = [];
type CapturedLinkDialogProps = { open: boolean; focusReturnRef?: { current: unknown } };
const capturedLinkDialogProps: Record<string, CapturedLinkDialogProps[]> = {
  pr: [],
  issue: [],
  mr: [],
  external: [],
};

vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: (props: { open: boolean; focusReturnRef?: { current: unknown } }) => {
    capturedEditDialogProps.push({ open: props.open, focusReturnRef: props.focusReturnRef });
    return props.open ? <div data-testid="edit-dialog-open" /> : null;
  },
}));

vi.mock("@/components/task/task-github-pr-dialog", () => ({
  TaskGitHubPRDialog: (props: CapturedLinkDialogProps) => {
    capturedLinkDialogProps.pr.push(props);
    return props.open ? <div data-testid="pr-dialog-open" /> : null;
  },
}));

vi.mock("@/components/task/task-github-issue-dialog", () => ({
  TaskGitHubIssueDialog: (props: CapturedLinkDialogProps) => {
    capturedLinkDialogProps.issue.push(props);
    return props.open ? <div data-testid="issue-dialog-open" /> : null;
  },
}));

vi.mock("@/components/gitlab/task-mr-link-dialog", () => ({
  TaskMRLinkDialog: (props: CapturedLinkDialogProps) => {
    capturedLinkDialogProps.mr.push(props);
    return props.open ? <div data-testid="mr-dialog-open" /> : null;
  },
}));

vi.mock("@/components/task/task-external-link-dialog", () => ({
  TaskExternalLinkDialog: (props: CapturedLinkDialogProps) => {
    capturedLinkDialogProps.external.push(props);
    return props.open ? <div data-testid="external-dialog-open" /> : null;
  },
}));

afterEach(() => {
  cleanup();
  capturedEditDialogProps.length = 0;
  for (const key of Object.keys(capturedLinkDialogProps)) capturedLinkDialogProps[key].length = 0;
});

const TASK_ID = "task-1";

const BOARD_ROW: TaskActionsMenuBoardRow = {
  id: TASK_ID,
  title: "Fix the sidebar",
  workflowStepId: "step-1",
};

function Harness({ boardRow }: { boardRow: TaskActionsMenuBoardRow | null }) {
  const menu = useTaskActionsMenu({
    taskId: TASK_ID,
    taskTitle: "Fix the sidebar",
    workspaceId: "ws-1",
    isArchived: false,
    boardRow,
    onArchive: () => undefined,
    onDelete: () => undefined,
  });
  return (
    <>
      <button data-testid="open-edit" onClick={() => menu.setShowEditDialog(true)}>
        Edit
      </button>
      <button data-testid="open-pr" onClick={() => menu.setShowPRDialog(true)}>
        Link PR
      </button>
      <button data-testid="open-issue" onClick={() => menu.setShowIssueDialog(true)}>
        Link issue
      </button>
      <button data-testid="open-mr" onClick={() => menu.setShowMRDialog(true)}>
        Link MR
      </button>
      <button data-testid="open-external" onClick={() => menu.setExternalLinkProvider("jira")}>
        Link Jira
      </button>
      <TaskActionsMenuDialogs
        taskId={TASK_ID}
        taskTitle="Fix the sidebar"
        workspaceId="ws-1"
        boardRow={boardRow}
        menu={menu}
      />
    </>
  );
}

function renderHarness(boardRow: TaskActionsMenuBoardRow | null) {
  return render(
    <ToastProvider>
      <StateProvider>
        <Harness boardRow={boardRow} />
      </StateProvider>
    </ToastProvider>,
  );
}

function rerenderHarness(
  rerender: (ui: React.ReactNode) => void,
  boardRow: TaskActionsMenuBoardRow | null,
) {
  rerender(
    <ToastProvider>
      <StateProvider>
        <Harness boardRow={boardRow} />
      </StateProvider>
    </ToastProvider>,
  );
}

describe("TaskActionsMenuDialogs — Edit dialog stays mounted through a board-row loss (AC-TASKS-TASK-ACTIONS-MENU-001.12)", () => {
  it("opens the Edit dialog for a resolved board row", () => {
    renderHarness(BOARD_ROW);
    fireEvent.click(screen.getByTestId("open-edit"));

    expect(screen.getByTestId("edit-dialog-open")).toBeTruthy();
  });

  it("renders no Edit dialog at all when the board row has never resolved", () => {
    renderHarness(null);

    expect(capturedEditDialogProps).toHaveLength(0);
  });

  it("closes the Edit dialog via its own open prop, not an abrupt unmount, when the board row disappears while it is open", () => {
    const { rerender } = renderHarness(BOARD_ROW);
    fireEvent.click(screen.getByTestId("open-edit"));
    expect(screen.getByTestId("edit-dialog-open")).toBeTruthy();
    capturedEditDialogProps.length = 0;

    rerenderHarness(rerender, null);

    // The dialog closed via `open` flipping to false in this same render,
    // not by the wrapper disappearing from the tree: a real unmount would
    // never give TaskCreateDialog the chance to run its own close-focus
    // transition.
    expect(screen.queryByTestId("edit-dialog-open")).toBeNull();
    expect(capturedEditDialogProps).toHaveLength(1);
    expect(capturedEditDialogProps[0]?.open).toBe(false);
  });

  it("passes a focusReturnRef pointing at the trigger to the Edit dialog", () => {
    renderHarness(BOARD_ROW);
    fireEvent.click(screen.getByTestId("open-edit"));

    expect(capturedEditDialogProps.at(-1)?.focusReturnRef).toBeTruthy();
  });
});

describe("TaskActionsMenuDialogs — Link dialogs get a focusReturnRef (AC-TASKS-TASK-ACTIONS-MENU-001.12)", () => {
  it("passes a focusReturnRef pointing at the trigger to the GitHub PR dialog", () => {
    renderHarness(BOARD_ROW);
    fireEvent.click(screen.getByTestId("open-pr"));

    expect(screen.getByTestId("pr-dialog-open")).toBeTruthy();
    expect(capturedLinkDialogProps.pr.at(-1)?.focusReturnRef).toBeTruthy();
  });

  it("passes a focusReturnRef pointing at the trigger to the GitHub Issue dialog", () => {
    renderHarness(BOARD_ROW);
    fireEvent.click(screen.getByTestId("open-issue"));

    expect(screen.getByTestId("issue-dialog-open")).toBeTruthy();
    expect(capturedLinkDialogProps.issue.at(-1)?.focusReturnRef).toBeTruthy();
  });

  it("passes a focusReturnRef pointing at the trigger to the GitLab MR dialog", () => {
    renderHarness(BOARD_ROW);
    fireEvent.click(screen.getByTestId("open-mr"));

    expect(screen.getByTestId("mr-dialog-open")).toBeTruthy();
    expect(capturedLinkDialogProps.mr.at(-1)?.focusReturnRef).toBeTruthy();
  });

  it("passes a focusReturnRef pointing at the trigger to the external link dialog", () => {
    renderHarness(BOARD_ROW);
    fireEvent.click(screen.getByTestId("open-external"));

    expect(screen.getByTestId("external-dialog-open")).toBeTruthy();
    expect(capturedLinkDialogProps.external.at(-1)?.focusReturnRef).toBeTruthy();
  });
});
