import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { JSX } from "react";

const mocks = vi.hoisted(() => ({
  routerPush: vi.fn(),
  setActiveTask: vi.fn(),
  setActiveSession: vi.fn(),
  openQuickChat: vi.fn(),
  setImproveDialogOpen: vi.fn(),
  openQuickTerminal: vi.fn(),
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

const WORKSPACE_ID = "ws-1";
const WORKSPACE_NAME = "Default Workspace";

const state = {
  workspaces: {
    activeId: WORKSPACE_ID as string | null,
    items: [{ id: WORKSPACE_ID, name: WORKSPACE_NAME }],
  },
  appSidebar: { improveDialogOpen: false },
  kanban: {
    workflowId: "wf-1" as string | null,
    steps: [{ id: "s1", title: "Todo" }],
  },
  setActiveTask: mocks.setActiveTask,
  setActiveSession: mocks.setActiveSession,
  setImproveDialogOpen: mocks.setImproveDialogOpen,
};
const QUICK_TERMINAL_TEST_ID = "sidebar-quick-terminal-shortcut";
const QUICK_CHAT_TEST_ID = "sidebar-quick-chat-shortcut";
let officeEnabled = false;
let pathname = "/";

// sidebar-workspace-actions plugin slot registrations (A1-A5). Empty by
// default so the existing dialog-routing/row-action tests above are
// unaffected; individual tests below set this before rendering.
let workspaceActionsRegistrations: Array<{
  registrationId: string;
  pluginId: string;
  Component: (props: { slotProps?: unknown }) => JSX.Element;
}> = [];

vi.mock("@/lib/plugins/registry", () => ({
  usePluginRegistry: () => ({
    getSlotRegistrations: (name: string) =>
      name === "sidebar-workspace-actions" ? workspaceActionsRegistrations : [],
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));
vi.mock("@/hooks/use-quick-chat-launcher", () => ({
  useQuickChatLauncher: () => mocks.openQuickChat,
}));
vi.mock("@/hooks/use-quick-terminal-launcher", () => ({
  useQuickTerminalLauncher: () => mocks.openQuickTerminal,
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
    open,
    onSuccess,
  }: {
    open?: boolean;
    onSuccess?: (
      task: { id: string },
      mode: "create" | "edit",
      meta?: { taskSessionId?: string | null; willNavigate?: boolean },
    ) => void;
  }) => (
    <button
      type="button"
      data-testid="regular-task-create-dialog"
      data-open={open ? "true" : "false"}
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
import { AppSidebarNewTaskItem } from "./app-sidebar-new-task-item";
import { requestNewTaskCreation } from "@/lib/desktop/new-task-request";

const OFFICE_DIALOG_TESTID = "office-new-task-dialog";
const REGULAR_DIALOG_TESTID = "regular-task-create-dialog";

function setImproveWorkspaceActive() {
  state.workspaces.activeId = "ws-improve";
  state.workspaces.items = [
    { id: WORKSPACE_ID, name: WORKSPACE_NAME },
    { id: "ws-improve", name: "Improve Kandev" },
  ];
}

function resetTestState() {
  state.workspaces.activeId = WORKSPACE_ID;
  state.workspaces.items = [{ id: WORKSPACE_ID, name: WORKSPACE_NAME }];
  state.appSidebar.improveDialogOpen = false;
  state.kanban.workflowId = "wf-1";
  state.kanban.steps = [{ id: "s1", title: "Todo" }];
  mocks.routerPush.mockClear();
  mocks.setActiveTask.mockClear();
  mocks.setActiveSession.mockClear();
  mocks.openQuickChat.mockClear();
  mocks.setImproveDialogOpen.mockClear();
  mocks.openQuickTerminal.mockClear();
  mocks.dialogTaskSessionId = null;
  mocks.dialogWillNavigate = false;
  officeEnabled = false;
  pathname = "/";
  workspaceActionsRegistrations = [];
}

beforeEach(resetTestState);
afterEach(() => cleanup());

describe("AppSidebarNewTaskItem dialog routing", () => {
  it("opens a queued New Task request after its listener remounts", () => {
    act(() => requestNewTaskCreation());

    renderItem(false);

    expect(screen.getByTestId(REGULAR_DIALOG_TESTID).dataset.open).toBe("true");
  });

  it("opens its existing task-create flow for a shared New Task request", () => {
    renderItem(false);

    act(() => requestNewTaskCreation());

    expect(screen.getByTestId(REGULAR_DIALOG_TESTID).dataset.open).toBe("true");
  });

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
    // NewTaskDialog is lazy-loaded by the SPA dynamic adapter, so it resolves asynchronously.
    expect(await screen.findByTestId(OFFICE_DIALOG_TESTID)).toBeTruthy();
    expect(screen.queryByTestId(REGULAR_DIALOG_TESTID)).toBeNull();
  });

  it("renders no dialog when there is no active workspace", () => {
    state.workspaces.activeId = null;
    renderItem(false);
    expect(screen.queryByTestId(REGULAR_DIALOG_TESTID)).toBeNull();
    expect(screen.queryByTestId(OFFICE_DIALOG_TESTID)).toBeNull();
  });

  it("opens the shared Improve Kandev dialog inside the dedicated Improve Kandev workspace", () => {
    setImproveWorkspaceActive();
    renderItem(false);
    // The item does not mount a dialog itself in the improve workspace — the
    // footer-hosted Improve Kandev dialog opens via the shared store flag.
    expect(screen.queryByTestId(REGULAR_DIALOG_TESTID)).toBeNull();

    screen.getByTestId("create-task-button").click();

    expect(mocks.setImproveDialogOpen).toHaveBeenCalledWith(true);
  });

  it("routes the shared New Task request to the Improve Kandev dialog in the improve workspace", () => {
    setImproveWorkspaceActive();
    renderItem(false);

    act(() => requestNewTaskCreation());

    expect(mocks.setImproveDialogOpen).toHaveBeenCalledWith(true);
    expect(mocks.setImproveDialogOpen).toHaveBeenCalledTimes(1);
  });

  it("keeps the regular dialog in a non-improve workspace", () => {
    renderItem(false);
    expect(screen.getByTestId(REGULAR_DIALOG_TESTID)).toBeTruthy();
    expect(mocks.setImproveDialogOpen).not.toHaveBeenCalled();
  });
});

describe("AppSidebarNewTaskItem row actions", () => {
  it("opens quick terminal from the action immediately left of Quick Chat", () => {
    renderItem(false);

    const terminal = screen.getByTestId(QUICK_TERMINAL_TEST_ID);
    const quickChat = screen.getByTestId(QUICK_CHAT_TEST_ID);
    expect(terminal.nextElementSibling).toBe(quickChat);

    terminal.click();
    expect(mocks.openQuickTerminal).toHaveBeenCalledOnce();
  });

  it("does not show the terminal tooltip when focus returns after closing", async () => {
    renderItem(false);

    const terminal = screen.getByTestId(QUICK_TERMINAL_TEST_ID);
    fireEvent.pointerEnter(terminal);
    expect(await screen.findByRole("tooltip")).toBeTruthy();
    fireEvent.focus(terminal);

    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("shows the terminal tooltip on pointer hover", async () => {
    renderItem(false);

    const terminal = screen.getByTestId(QUICK_TERMINAL_TEST_ID);
    fireEvent.pointerEnter(terminal);

    expect((await screen.findByRole("tooltip")).textContent).toBe("Quick terminal");

    fireEvent.pointerLeave(terminal);
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("opens quick chat from the trailing action beside New Task", () => {
    renderItem(false);
    screen.getByTestId(QUICK_CHAT_TEST_ID).click();
    expect(mocks.openQuickChat).toHaveBeenCalledOnce();
  });

  it("hides the quick chat shortcut when the rail is collapsed", () => {
    renderItem(true);
    expect(screen.queryByTestId(QUICK_CHAT_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(QUICK_TERMINAL_TEST_ID)).toBeNull();
  });

  it("hides the quick chat shortcut when there is no active workspace", () => {
    state.workspaces.activeId = null;
    renderItem(false);
    expect(screen.queryByTestId(QUICK_CHAT_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(QUICK_TERMINAL_TEST_ID)).toBeNull();
  });
});

describe("AppSidebarNewTaskItem sidebar-workspace-actions plugin slot", () => {
  const PLUGIN_TEST_ID = "plugin-workspace-action";
  const INSET_TEST_ID = "create-task-button";

  function registerPlugin(Component: (props: { slotProps?: unknown }) => JSX.Element) {
    workspaceActionsRegistrations = [{ registrationId: "reg-1", pluginId: "plugin-1", Component }];
  }

  it("A1: renders a registered component after Quick Terminal and Quick Chat", () => {
    registerPlugin(() => <button type="button" data-testid={PLUGIN_TEST_ID} />);
    renderItem(false);

    const terminal = screen.getByTestId(QUICK_TERMINAL_TEST_ID);
    const quickChat = screen.getByTestId(QUICK_CHAT_TEST_ID);
    const plugin = screen.getByTestId(PLUGIN_TEST_ID);
    expect(terminal.nextElementSibling).toBe(quickChat);
    expect(quickChat.nextElementSibling).toBe(plugin);
  });

  it("A2: forwards the active workspace id and label as slotProps", () => {
    let captured: unknown;
    registerPlugin(({ slotProps }) => {
      captured = slotProps;
      return <button type="button" data-testid={PLUGIN_TEST_ID} />;
    });
    renderItem(false);

    expect(captured).toEqual({ workspaceId: WORKSPACE_ID, workspaceLabel: WORKSPACE_NAME });
  });

  it("A3: widens the reserved inset only once the slot has a registration", () => {
    renderItem(false);
    expect(screen.getByTestId(INSET_TEST_ID).className).toContain("pr-16");
    cleanup();

    registerPlugin(() => <button type="button" data-testid={PLUGIN_TEST_ID} />);
    renderItem(false);
    expect(screen.getByTestId(INSET_TEST_ID).className).not.toContain("pr-16");
    expect(screen.getByTestId(INSET_TEST_ID).className).toContain("pr-20");
  });

  it("A4: renders no plugin markup when the sidebar is collapsed or no workspace is active", () => {
    registerPlugin(() => <button type="button" data-testid={PLUGIN_TEST_ID} />);

    renderItem(true);
    expect(screen.queryByTestId(PLUGIN_TEST_ID)).toBeNull();
    cleanup();

    state.workspaces.activeId = null;
    renderItem(false);
    expect(screen.queryByTestId(PLUGIN_TEST_ID)).toBeNull();
  });

  it("A5: a throwing plugin component leaves Quick Terminal and Quick Chat functional", () => {
    registerPlugin(() => {
      throw new Error("boom");
    });
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    renderItem(false);

    expect(screen.queryByTestId(PLUGIN_TEST_ID)).toBeNull();
    const terminal = screen.getByTestId(QUICK_TERMINAL_TEST_ID);
    terminal.click();
    expect(mocks.openQuickTerminal).toHaveBeenCalledOnce();
    screen.getByTestId(QUICK_CHAT_TEST_ID).click();
    expect(mocks.openQuickChat).toHaveBeenCalledOnce();

    consoleError.mockRestore();
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
