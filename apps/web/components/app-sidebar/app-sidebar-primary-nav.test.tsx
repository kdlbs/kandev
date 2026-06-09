import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  openQuickChat: vi.fn(),
}));

const state = {
  workspaces: { activeId: "ws-1" as string | null },
  office: { inboxCount: 0 },
};
let inOffice = false;
let pathname = "/";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/use-in-office", () => ({
  useInOffice: () => inOffice,
}));

vi.mock("@/hooks/use-quick-chat-launcher", () => ({
  useQuickChatLauncher: () => mocks.openQuickChat,
}));

vi.mock("next/navigation", () => ({
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
    state.office.inboxCount = 0;
    inOffice = false;
    pathname = "/";
    mocks.openQuickChat.mockClear();
  });

  afterEach(() => cleanup());

  it("keeps Quick Chat reachable when the sidebar rail is collapsed", () => {
    renderNav(true);

    screen.getByRole("button", { name: "Quick Chat" }).click();
    expect(mocks.openQuickChat).toHaveBeenCalledOnce();
  });

  it("omits the standalone Quick Chat row while expanded", () => {
    renderNav(false);

    expect(screen.queryByRole("button", { name: "Quick Chat" })).toBeNull();
  });
});
