import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  openQuickChat: vi.fn(),
}));

const state = {
  workspaces: { activeId: "ws-1" as string | null },
  office: { inboxCountByWorkspaceId: {} as Record<string, number> },
  quickChat: {
    isOpen: false,
    sessions: [] as Array<{
      sessionId: string;
      workspaceId: string;
      kind: "chat";
      taskId?: string;
    }>,
    unseenIdleByWorkspace: {} as Record<string, Record<string, true>>,
  },
  taskSessions: { items: {} as Record<string, { state: string; task_id: string }> },
  prepareProgress: { bySessionId: {} as Record<string, { status: string }> },
};
const QUICK_CHAT_LABEL = "Quick Chat";
const QUICK_CHAT_RUNNING_LABEL = "Quick Chat, agent working";
const QUICK_CHAT_UNSEEN_LABEL = "Quick Chat, new response";
let mode: "office" | "kanban" | "unknown" = "kanban";
let pathname = "/";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/use-in-office", () => ({
  useOfficeModeState: () => mode,
}));

vi.mock("@/hooks/use-quick-chat-launcher", () => ({
  useQuickChatLauncher: () => mocks.openQuickChat,
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => pathname,
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("./app-sidebar-new-task-item", () => ({
  AppSidebarNewTaskItem: ({ collapsed }: { collapsed: boolean }) => (
    <div data-testid="new-task-item" data-collapsed={collapsed ? "true" : "false"} />
  ),
}));

import { AppSidebarPrimaryNav } from "./app-sidebar-primary-nav";

function renderNav(collapsed: boolean) {
  return render(
    <TooltipProvider>
      <AppSidebarPrimaryNav collapsed={collapsed} />
    </TooltipProvider>,
  );
}

describe("AppSidebarPrimaryNav", () => {
  beforeEach(() => {
    state.workspaces.activeId = "ws-1";
    state.office.inboxCountByWorkspaceId = {};
    state.quickChat.isOpen = false;
    state.quickChat.sessions = [];
    state.quickChat.unseenIdleByWorkspace = {};
    state.taskSessions.items = {};
    state.prepareProgress.bySessionId = {};
    mode = "kanban";
    pathname = "/";
    mocks.openQuickChat.mockClear();
  });

  afterEach(() => cleanup());

  it("keeps Quick Chat reachable when the sidebar rail is collapsed", () => {
    renderNav(true);

    expect(screen.queryByTestId("quick-chat-unseen-dot")).toBeNull();
    screen.getByRole("button", { name: QUICK_CHAT_LABEL }).click();
    expect(mocks.openQuickChat).toHaveBeenCalledOnce();
  });

  it("renders an unseen marker on the collapsed Quick Chat rail entry", () => {
    state.quickChat.sessions = [
      { sessionId: "session-1", workspaceId: "ws-1", kind: "chat", taskId: "task-1" },
    ];
    state.taskSessions.items = {
      "session-1": { state: "COMPLETED", task_id: "task-1" },
    };
    state.quickChat.unseenIdleByWorkspace = { "ws-1": { "session-1": true } };
    renderNav(true);
    const quickChat = screen.getByRole("button", { name: QUICK_CHAT_UNSEEN_LABEL });

    const indicator = quickChat.querySelector('[data-testid="quick-chat-activity-indicator"]');
    expect(indicator?.getAttribute("data-state")).toBe("finished");
  });

  it("prioritizes a running activity marker over an unseen response", () => {
    state.quickChat.sessions = [
      { sessionId: "session-1", workspaceId: "ws-1", kind: "chat", taskId: "task-1" },
    ];
    state.taskSessions.items = {
      "session-1": { state: "RUNNING", task_id: "task-1" },
    };
    state.quickChat.unseenIdleByWorkspace = { "ws-1": { "session-1": true } };
    renderNav(true);

    const quickChat = screen.getByRole("button", { name: QUICK_CHAT_RUNNING_LABEL });
    expect(
      quickChat
        .querySelector('[data-testid="quick-chat-activity-indicator"]')
        ?.getAttribute("data-state"),
    ).toBe("running");
  });

  it("omits the standalone Quick Chat row while expanded", () => {
    renderNav(false);
    expect(screen.queryByRole("button", { name: QUICK_CHAT_LABEL })).toBeNull();
  });

  it("no longer carries a Runs row — automations are their own section", () => {
    // The destination moved into the Automations section, whose header links to
    // it and whose rows are the automations themselves. Two nav entries pointing
    // at the same place is the thing that made "Runs" hard to place.
    renderNav(false);

    expect(screen.queryByRole("link", { name: "Runs" })).toBeNull();
  });

  it("omits Quick Chat when the rail is collapsed but there is no workspace", () => {
    state.workspaces.activeId = null;
    renderNav(true);
    expect(screen.queryByRole("button", { name: QUICK_CHAT_LABEL })).toBeNull();
  });

  it("links Home to an explicit overview while keeping it active at the root route", () => {
    renderNav(false);

    const home = screen.getByRole("link", { name: "Home" });
    expect(home.getAttribute("href")).toBe("/?home=overview&workspaceId=ws-1");
    expect(home.className).toContain("before:bg-primary");
  });

  it("does not expose a Kanban Home link before workspace mode resolves", () => {
    mode = "unknown";
    renderNav(false);

    expect(screen.queryByRole("link", { name: "Home" })).toBeNull();
  });
});
