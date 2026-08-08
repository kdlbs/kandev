import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { KanbanHeaderMobile } from "./kanban-header-mobile";

vi.mock("@/components/page-topbar", () => ({
  PageTopbar: ({
    backLabel,
    leading,
    leftActions,
    actions,
  }: {
    backLabel?: string;
    leading?: ReactNode;
    leftActions?: ReactNode;
    actions?: ReactNode;
  }) => (
    <header>
      {leading}
      <span>{backLabel}</span>
      <div data-testid="topbar-left-actions">{leftActions}</div>
      <div>{actions}</div>
    </header>
  ),
}));

vi.mock("./mobile-menu-sheet", () => ({
  MobileMenuSheet: () => null,
}));

const quickChatMocks = vi.hoisted(() => ({
  openQuickChat: vi.fn(),
  openQuickTerminal: vi.fn(),
}));
const statusDrawerState = vi.hoisted(() => ({
  issueSeverity: "none" as "none" | "unstable" | "lost",
}));

vi.mock("@/hooks/use-quick-chat-launcher", () => ({
  useQuickChatLauncher: () => quickChatMocks.openQuickChat,
}));

vi.mock("@/hooks/use-quick-terminal-launcher", () => ({
  useQuickTerminalLauncher: () => quickChatMocks.openQuickTerminal,
}));

vi.mock("@/components/app-status-bar/app-status-surface-provider", () => ({
  useAppStatusDrawer: () => statusDrawerState,
}));

const LEFT_ACTIONS_TEST_ID = "topbar-left-actions";
const QUICK_CHAT_TEST_ID = "mobile-quick-chat-button";
const QUICK_TERMINAL_TEST_ID = "mobile-quick-terminal-button";
const ACTIVE_WORKSPACE_ID = "workspace-1";

afterEach(() => {
  cleanup();
  quickChatMocks.openQuickChat.mockClear();
  quickChatMocks.openQuickTerminal.mockClear();
  statusDrawerState.issueSeverity = "none";
});

function renderHeader(title: string, workspaceId?: string, onSearchChange?: () => void) {
  return render(
    <StateProvider>
      <KanbanHeaderMobile
        title={title}
        workspaceId={workspaceId}
        onSearchChange={onSearchChange}
        workspaceLabel="/root/kandev"
      />
    </StateProvider>,
  );
}

describe("KanbanHeaderMobile", () => {
  it("links the Kandev brand home and omits the redundant Home title", () => {
    renderHeader("Home", ACTIVE_WORKSPACE_ID);

    expect(screen.getByRole("link", { name: "Kandev home" }).getAttribute("href")).toBe(
      `/?home=overview&workspaceId=${ACTIVE_WORKSPACE_ID}`,
    );
    expect(screen.getByTestId(LEFT_ACTIONS_TEST_ID).textContent).toBe("");
  });

  it("renders page title and workspace label for non-Home pages", () => {
    renderHeader("Tasks");

    const leftActions = screen.getByTestId(LEFT_ACTIONS_TEST_ID);
    expect(leftActions.textContent).toContain("Tasks");
    expect(leftActions.textContent).toContain("/root/kandev");
    expect(leftActions.firstElementChild?.className).toContain("max-w-[38vw]");
  });

  it("opens quick chat from the header action when a workspace is active", () => {
    renderHeader("Home", ACTIVE_WORKSPACE_ID);

    fireEvent.click(screen.getByTestId(QUICK_CHAT_TEST_ID));
    expect(quickChatMocks.openQuickChat).toHaveBeenCalledTimes(1);
  });

  it("opens quick terminal immediately before quick chat", () => {
    renderHeader("Home", ACTIVE_WORKSPACE_ID);

    const terminal = screen.getByTestId(QUICK_TERMINAL_TEST_ID);
    const quickChat = screen.getByTestId(QUICK_CHAT_TEST_ID);
    expect(terminal.nextElementSibling).toBe(quickChat);

    fireEvent.click(terminal);
    expect(quickChatMocks.openQuickTerminal).toHaveBeenCalledTimes(1);
  });

  it("hides the quick chat action without an active workspace", () => {
    renderHeader("Home");

    expect(screen.queryByTestId(QUICK_CHAT_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(QUICK_TERMINAL_TEST_ID)).toBeNull();
  });

  it("places quick chat immediately before search", () => {
    renderHeader("Home", "workspace-1", vi.fn());

    const quickChat = screen.getByTestId(QUICK_CHAT_TEST_ID);
    const search = screen.getByTestId("mobile-search-toggle");
    expect(quickChat.nextElementSibling).toBe(search);
  });

  it("describes a connectivity warning on the persistent Home menu trigger", () => {
    statusDrawerState.issueSeverity = "lost";
    renderHeader("Home");

    expect(
      screen.getByRole("button", {
        name: "Connection lost for at least 10 seconds. Live updates may be stale. Open menu",
      }),
    ).toBeTruthy();
  });
});
