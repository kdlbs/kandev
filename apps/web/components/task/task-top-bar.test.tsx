import { useEffect } from "react";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { defaultState } from "@/lib/state/default-state";
import { TaskTopBar } from "./task-top-bar";
import type { TaskActionsMenuBoardRow } from "@/hooks/use-task-actions-menu";

afterEach(() => cleanup());

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/hooks/domains/session/use-session-git", () => ({
  useSessionGit: () => ({ branch: "", renameBranch: vi.fn() }),
}));

vi.mock("@/components/task/executor-settings-button", () => ({
  ExecutorSettingsButton: () => <button data-testid="executor-settings-button">executor</button>,
}));

vi.mock("@/components/task/task-unarchive-button", () => ({
  TaskUnarchiveButton: ({ taskId }: { taskId?: string | null }) => (
    <button data-testid="task-unarchive-button">{taskId}</button>
  ),
}));

vi.mock("@/components/task/port-forward-dialog", () => ({
  PortForwardButton: () => <button>ports</button>,
}));

vi.mock("@/components/task/document/document-controls", () => ({
  DocumentControls: () => null,
}));

vi.mock("@/components/vcs-split-button", () => ({
  VcsSplitButton: () => <button>vcs</button>,
}));

vi.mock("@/components/github/pr-topbar-button", () => ({
  PRTopbarButton: () => <button data-testid="pr-topbar-button">#1472</button>,
}));

vi.mock("@/components/gitlab/mr-topbar-button", () => ({
  MRTopbarButton: () => null,
}));

vi.mock("@/components/integrations/registered-change-request-status", () => ({
  RegisteredChangeRequestStatus: ({
    taskId,
    sessionId,
    surface,
  }: {
    taskId: string | null;
    sessionId?: string | null;
    surface: string;
  }) => (
    <span data-testid={`registered-change-request-${surface}`}>
      {taskId}:{sessionId}
    </span>
  ),
}));

vi.mock("@/components/jira/jira-ticket-button", () => ({
  JiraTicketButton: () => null,
  extractJiraKey: () => null,
}));

vi.mock("@/components/linear/linear-issue-button", () => ({
  LinearIssueButton: () => null,
  extractLinearKey: () => null,
}));

vi.mock("@/hooks/domains/jira/use-jira-availability", () => ({
  useJiraAvailable: () => false,
}));

vi.mock("@/hooks/domains/linear/use-linear-availability", () => ({
  useLinearAvailable: () => false,
}));

vi.mock("@/components/task/workflow-stepper", () => ({
  WorkflowStepper: () => null,
}));

vi.mock("@/components/task/layout-preset-selector", () => ({
  LayoutPresetSelector: () => null,
}));

vi.mock("@/components/task/editors-menu", () => ({
  EditorsMenu: () => null,
}));

vi.mock("@/components/task/quick-chat-button", () => ({
  QuickChatButton: () => null,
}));

vi.mock("@/components/integrations/integrations-menu", () => ({
  IntegrationsMenu: () => null,
}));

vi.mock("@/components/task/branch-path-popover", () => ({
  BranchPathPopover: () => null,
}));

describe("TaskTopBar executor environment controls", () => {
  it("hides the executor environment button for filesystem executors", () => {
    renderTopBar(<TaskTopBar taskId="task-1" remoteExecutorType="worktree" />);

    expect(screen.queryByTestId("executor-settings-button")).toBeNull();
  });

  it("shows the executor environment button for Docker executors", () => {
    renderTopBar(<TaskTopBar taskId="task-1" remoteExecutorType="local_docker" />);

    expect(screen.getByTestId("executor-settings-button")).toBeTruthy();
  });

  it("shows the executor environment button for Kubernetes executors", () => {
    renderTopBar(<TaskTopBar taskId="task-1" remoteExecutorType="k8s" />);

    expect(screen.getByTestId("executor-settings-button")).toBeTruthy();
  });
});

describe("TaskTopBar GitHub issue link", () => {
  it("shows the linked issue before linked pull requests", () => {
    renderTopBar(
      <TaskTopBar
        taskId="task-1"
        issueUrl="https://github.com/kdlbs/kandev/issues/1470"
        issueNumber={1470}
      />,
    );

    const issue = screen.getByTestId("issue-topbar-button");
    const pr = screen.getByTestId("pr-topbar-button");

    expect(issue.textContent).toContain("#1470");
    expect(screen.getByLabelText("Task status and attention").className).toContain(
      "[&_[data-testid=issue-topbar-button]]:h-7",
    );
    expect(screen.getByLabelText("Task status and attention").className).not.toContain("[&_a]");
    expect(issue.compareDocumentPosition(pr) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

describe("TaskTopBar registered change requests", () => {
  it("mounts plugin status beside native review controls", () => {
    renderTopBar(<TaskTopBar taskId="task-1" activeSessionId="session-1" />);

    const native = screen.getByTestId("pr-topbar-button");
    const plugin = screen.getByTestId("registered-change-request-topbar");

    expect(plugin.textContent).toBe("task-1:session-1");
    expect(native.compareDocumentPosition(plugin) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

describe("TaskTopBar repository crumb", () => {
  function repositoryCrumb(label: string) {
    return within(screen.getByRole("navigation", { name: "breadcrumb" })).queryByText(label);
  }

  it("names the task's repository so an open task says which project it belongs to", () => {
    renderTopBar(
      <TaskTopBar taskId="task-1" taskTitle={TASK_TITLE} repositoryLabel="kdlbs/kandev" />,
    );

    const crumb = repositoryCrumb("kdlbs/kandev");
    expect(crumb).toBeTruthy();
    // Orientation, not navigation: there is no repository route to land on.
    expect(crumb!.closest("a")).toBeNull();
    // Truncation is what protects the title, so the full name lives in `title`.
    expect(crumb!.getAttribute("title")).toBe("kdlbs/kandev");
  });

  it("renders no repository crumb for a task with no repository", () => {
    renderTopBar(<TaskTopBar taskId="task-1" taskTitle={TASK_TITLE} />);

    // Any static crumb, not just a slug-shaped one: a repository with no
    // provider falls back to a bare name like "scratchpad", which a
    // slash-matching query would happily miss.
    const breadcrumb = screen.getByRole("navigation", { name: "breadcrumb" });
    expect(breadcrumb.querySelector("[title]")).toBeNull();
  });
});

describe("TaskTopBar archived task controls", () => {
  it("passes the task ID to the unarchive button", () => {
    renderTopBar(<TaskTopBar taskId="task-1" isArchived />);

    expect(screen.getByTestId("task-unarchive-button").textContent).toBe("task-1");
  });
});

function renderTopBar(ui: React.ReactNode) {
  return render(
    <StateProvider>
      <ToastProvider>{ui}</ToastProvider>
    </StateProvider>,
  );
}

const TASK_TITLE = "Fix the sidebar";
const TRIGGER_TEST_ID = "task-topbar-actions-menu";

const NORMAL_BOARD_ROW: TaskActionsMenuBoardRow = {
  id: "task-1",
  title: TASK_TITLE,
  workflowStepId: "step-1",
};

function getTrigger() {
  return screen.getByTestId(TRIGGER_TEST_ID);
}

function openMenu() {
  const trigger = getTrigger();
  fireEvent.pointerDown(trigger, { button: 0, pointerId: 1 });
  fireEvent.click(trigger);
  return trigger;
}

function expectMenuOpen(open: boolean) {
  expect(getTrigger().getAttribute("aria-expanded")).toBe(String(open));
}

describe("TaskTopBar actions menu trigger", () => {
  it("renders no trigger when the top bar has no subject task", () => {
    renderTopBar(<TaskTopBar taskId={null} actionsMenuBoardRow={null} />);

    expect(screen.queryByTestId(TRIGGER_TEST_ID)).toBeNull();
  });

  it("renders the trigger last in the right control group, with the More options accessible name", () => {
    renderTopBar(
      <TaskTopBar taskId="task-1" taskTitle={TASK_TITLE} actionsMenuBoardRow={NORMAL_BOARD_ROW} />,
    );

    const trigger = getTrigger();
    expect(trigger.getAttribute("aria-label")).toBe("More options");
    expect(trigger.getAttribute("aria-haspopup")).toBe("menu");
    expectMenuOpen(false);

    // Positioned after every other control already rendered in the group
    // (AC-TASKS-TASK-ACTIONS-MENU-001.2): every earlier control precedes it
    // in document order.
    const controls = screen.getAllByRole("button");
    expect(controls.at(-1)).toBe(trigger);
  });

  it("opens a menu presenting Edit, Archive, and Delete for a normal (non-archived, resolvable) subject", () => {
    renderTopBar(
      <TaskTopBar taskId="task-1" taskTitle={TASK_TITLE} actionsMenuBoardRow={NORMAL_BOARD_ROW} />,
    );

    openMenu();

    expect(screen.getByRole("menuitem", { name: "Edit" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Archive" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Delete" })).toBeTruthy();
  });

  it("renders the trigger and identifier-only tier (Archive/Delete only) when the board row can't be resolved (AC-TASKS-TASK-ACTIONS-MENU-002.5)", () => {
    renderTopBar(<TaskTopBar taskId="task-1" taskTitle={TASK_TITLE} actionsMenuBoardRow={null} />);

    expect(screen.queryByTestId(TRIGGER_TEST_ID)).toBeTruthy();
    openMenu();

    expect(screen.getByRole("menuitem", { name: "Archive" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Delete" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Edit" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Move to" })).toBeNull();
  });

  it("presents only Delete (no Archive, no Edit) for an archived subject", () => {
    renderTopBar(
      <TaskTopBar
        taskId="task-1"
        taskTitle={TASK_TITLE}
        isArchived
        actionsMenuBoardRow={NORMAL_BOARD_ROW}
      />,
    );

    openMenu();

    expect(screen.getByRole("menuitem", { name: "Delete" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Archive" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Edit" })).toBeNull();
  });
});

function StoreProbe({ onReady }: { onReady: (store: ReturnType<typeof useAppStoreApi>) => void }) {
  const store = useAppStoreApi();
  useEffect(() => onReady(store), [store, onReady]);
  return null;
}

describe("TaskTopBar actions menu — subject removed from the board (AC-TASKS-TASK-ACTIONS-MENU-004.5)", () => {
  // The subject leaving `kanban.tasks` alone is board-row loss, not genuine
  // removal (AC-TASKS-TASK-ACTIONS-MENU-002.6/004.1c): it must demote the
  // menu in place, never close it. `actionsMenuBoardRow` is a prop this
  // harness holds static, so only a real "gone" signal (task.deleted, or
  // task.updated with archived_at) may close the menu here, and this test has
  // no WS client wired up to send one.
  it("keeps an open menu open when the subject merely leaves the board's task collections", () => {
    let store: ReturnType<typeof useAppStoreApi> | null = null;
    renderTopBar(
      <StateProvider
        initialState={{
          kanban: {
            ...defaultState.kanban,
            tasks: [
              {
                id: "task-1",
                workflowId: "workflow-1",
                workflowStepId: "step-1",
                title: TASK_TITLE,
                position: 0,
              },
            ],
          },
        }}
      >
        <StoreProbe
          onReady={(api) => {
            store = api;
          }}
        />
        <TaskTopBar taskId="task-1" taskTitle={TASK_TITLE} actionsMenuBoardRow={NORMAL_BOARD_ROW} />
      </StateProvider>,
    );

    openMenu();
    expectMenuOpen(true);

    act(() => {
      store!.setState((state) => ({ kanban: { ...state.kanban, tasks: [] } }));
    });

    expectMenuOpen(true);
  });

  it("does not close the menu for a task that has simply not loaded into the board yet", () => {
    renderTopBar(
      <TaskTopBar taskId="task-1" taskTitle={TASK_TITLE} actionsMenuBoardRow={NORMAL_BOARD_ROW} />,
    );

    openMenu();

    expectMenuOpen(true);
  });
});
