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

vi.mock("@/components/system-metrics/topbar-metrics", () => ({
  TopbarMetrics: () => <div data-testid="topbar-metrics" />,
}));

vi.mock("./mobile-menu-sheet", () => ({
  MobileMenuSheet: () => null,
}));

const quickChatMocks = vi.hoisted(() => ({ openQuickChat: vi.fn() }));

vi.mock("@/hooks/use-quick-chat-launcher", () => ({
  useQuickChatLauncher: () => quickChatMocks.openQuickChat,
}));

const LEFT_ACTIONS_TEST_ID = "topbar-left-actions";
const QUICK_CHAT_TEST_ID = "mobile-quick-chat-button";

afterEach(() => {
  cleanup();
  quickChatMocks.openQuickChat.mockClear();
});

function renderHeader(title: string, workspaceId?: string, onSearchChange?: () => void) {
  return render(
    <StateProvider>
      <KanbanHeaderMobile
        title={title}
        workspaceId={workspaceId}
        onSearchChange={onSearchChange}
        workspaceLabel="/root/kandev"
        showHealthIndicator={false}
        onOpenHealthDialog={() => undefined}
      />
    </StateProvider>,
  );
}

describe("KanbanHeaderMobile", () => {
  it("links the Kandev brand home and omits the redundant Home title", () => {
    renderHeader("Home", "workspace-1");

    expect(screen.getByRole("link", { name: "Kandev home" }).getAttribute("href")).toBe(
      "/?workspaceId=workspace-1",
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
    renderHeader("Home", "workspace-1");

    fireEvent.click(screen.getByTestId(QUICK_CHAT_TEST_ID));
    expect(quickChatMocks.openQuickChat).toHaveBeenCalledTimes(1);
  });

  it("hides the quick chat action without an active workspace", () => {
    renderHeader("Home");

    expect(screen.queryByTestId(QUICK_CHAT_TEST_ID)).toBeNull();
  });

  it("places quick chat immediately before search", () => {
    renderHeader("Home", "workspace-1", vi.fn());

    const quickChat = screen.getByTestId(QUICK_CHAT_TEST_ID);
    const search = screen.getByTestId("mobile-search-toggle");
    expect(quickChat.nextElementSibling).toBe(search);
  });
});
