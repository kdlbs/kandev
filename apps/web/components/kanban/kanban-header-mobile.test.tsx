import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { KanbanHeaderMobile } from "./kanban-header-mobile";

vi.mock("@/components/page-topbar", () => ({
  PageTopbar: ({
    title,
    leading,
    leftActions,
    actions,
    freeWidth,
  }: {
    title?: string;
    leading?: ReactNode;
    leftActions?: ReactNode;
    actions?: ReactNode;
    freeWidth?: "lead" | "actions";
  }) => (
    <header data-free-width={freeWidth}>
      {leading}
      <span data-testid="topbar-title">{title}</span>
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

/**
 * `currentPage` — not the title text — decides whether this is the Home header.
 * The component used to derive that from `title === "Home"`, which was true only
 * in English, so these tests pass the discriminant explicitly now.
 */
function renderHeader(
  title: string,
  workspaceId?: string,
  onSearchChange?: () => void,
  currentPage: "kanban" | "tasks" = "kanban",
) {
  return render(
    <StateProvider>
      <KanbanHeaderMobile
        title={title}
        currentPage={currentPage}
        workspaceId={workspaceId}
        onSearchChange={onSearchChange}
        workspaceLabel="/root/kandev"
      />
    </StateProvider>,
  );
}

describe("KanbanHeaderMobile", () => {
  it("links the Kandev brand home and names the page through the title crumb", () => {
    renderHeader("Home", ACTIVE_WORKSPACE_ID);

    expect(screen.getByRole("link", { name: "Kandev home" }).getAttribute("href")).toBe(
      `/?home=overview&workspaceId=${ACTIVE_WORKSPACE_ID}`,
    );
    expect(screen.getByTestId("topbar-title").textContent).toBe("Home");
    // The old two-line title/workspace stack is gone.
    expect(screen.getByTestId(LEFT_ACTIONS_TEST_ID).textContent).toBe("");
  });

  it("renders the title crumb without repeating the workspace label", () => {
    renderHeader("Tasks", undefined, undefined, "tasks");

    expect(screen.getByTestId("topbar-title").textContent).toBe("Tasks");
    // The picker owns workspace identity; the bar never repeats it.
    expect(screen.queryByText("/root/kandev")).toBeNull();
    // The scrolling strip carries actions only; page context is the crumb's job.
    const actionStrip = screen.getByTestId("mobile-topbar-action-strip");
    expect(actionStrip.textContent).not.toContain("Tasks");
  });

  it("opens quick chat from the header action when a workspace is active", () => {
    renderHeader("Home", ACTIVE_WORKSPACE_ID);

    fireEvent.click(screen.getByTestId(QUICK_CHAT_TEST_ID));
    expect(quickChatMocks.openQuickChat).toHaveBeenCalledTimes(1);
  });

  it("opens quick terminal immediately before quick chat", () => {
    renderHeader("Home", ACTIVE_WORKSPACE_ID);

    const terminalTarget = screen.getByTestId("mobile-quick-terminal-hit-target");
    const quickChatTarget = screen.getByTestId("mobile-quick-chat-hit-target");
    expect(terminalTarget.nextElementSibling).toBe(quickChatTarget);

    fireEvent.click(screen.getByTestId(QUICK_TERMINAL_TEST_ID));
    expect(quickChatMocks.openQuickTerminal).toHaveBeenCalledTimes(1);
  });

  it("hides the quick chat action without an active workspace", () => {
    renderHeader("Home");

    expect(screen.queryByTestId(QUICK_CHAT_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(QUICK_TERMINAL_TEST_ID)).toBeNull();
  });

  it("places quick chat immediately before search", () => {
    renderHeader("Home", "workspace-1", vi.fn());

    const quickChat = screen.getByTestId("mobile-quick-chat-hit-target");
    const search = screen.getByTestId("mobile-search-toggle");
    expect(quickChat.nextElementSibling).toBe(search);
  });

  it("hands the bar's leftover width to the scrolling action strip", () => {
    renderHeader("Home", ACTIVE_WORKSPACE_ID, vi.fn());

    // The strip is `flex-1`, so its base size is zero. If the lead zone keeps
    // the slack the strip resolves to zero width, the actions become
    // unreachable, and the bar renders brand, empty middle, menu.
    const bar = screen.getByTestId("mobile-topbar-action-strip").closest("header");
    expect(bar?.getAttribute("data-free-width")).toBe("actions");
  });

  it("keeps the brand and menu outside the middle action strip", () => {
    renderHeader("Home", ACTIVE_WORKSPACE_ID, vi.fn());

    const strip = screen.getByTestId("mobile-topbar-action-strip");
    const menu = screen.getByTestId("mobile-topbar-menu");
    expect(strip.parentElement).toBe(menu.parentElement);
    expect(strip.previousElementSibling).not.toBe(menu);
    expect(menu.previousElementSibling).toBe(strip);
  });

  it("uses the shared compact icon geometry for native mobile actions", () => {
    renderHeader("Home", ACTIVE_WORKSPACE_ID, vi.fn());

    for (const id of [
      QUICK_TERMINAL_TEST_ID,
      QUICK_CHAT_TEST_ID,
      "mobile-search-toggle",
      "mobile-topbar-menu",
    ]) {
      expect(screen.getByTestId(id).className).not.toContain("!size-11");
    }
    expect(screen.getByTestId("mobile-quick-terminal-hit-target").className).toContain("h-11");
    expect(screen.getByTestId("mobile-quick-chat-hit-target").className).toContain("h-11");
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
