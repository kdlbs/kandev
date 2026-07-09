import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

let pathname = "/settings/integrations/github";

const state = {
  workspaces: {
    activeId: "ws-1",
    items: [
      { id: "ws-1", name: "Default" },
      { id: "ws-2", name: "Archive" },
    ],
  },
  setActiveWorkspace: vi.fn(),
};

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => pathname,
  useRouter: () => ({ replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/components/page-topbar", () => ({
  PageTopbar: ({ actions }: { actions?: ReactNode }) => (
    <div data-testid="page-topbar-actions">{actions}</div>
  ),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/task/workspace-switcher", () => ({
  WorkspaceSwitcher: () => <div data-testid="mock-workspace-switcher" />,
}));

vi.mock("@/components/integrations/integration-copy-config-menu", () => ({
  IntegrationCopyConfigMenu: ({ sourceWorkspaceId }: { sourceWorkspaceId: string }) => (
    <div data-testid="mock-copy-config" data-source-workspace-id={sourceWorkspaceId} />
  ),
}));

import { SettingsLayoutClient } from "./settings-layout-client";

describe("SettingsLayoutClient integrations actions", () => {
  beforeEach(() => {
    pathname = "/settings/integrations/github";
    state.workspaces.activeId = "ws-1";
    state.setActiveWorkspace.mockClear();
  });

  afterEach(() => cleanup());

  it("keeps copy config available without rendering the workspace switcher", () => {
    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    expect(screen.queryByTestId("integration-workspace-switcher")).toBeNull();
    expect(screen.queryByTestId("mock-workspace-switcher")).toBeNull();
    expect(screen.getByTestId("mock-copy-config").dataset.sourceWorkspaceId).toBe("ws-1");
  });

  it("shows copy config on workspace-scoped integration pages", () => {
    pathname = "/settings/workspace/ws-1/integrations/github";

    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByTestId("mock-copy-config").dataset.sourceWorkspaceId).toBe("ws-1");
  });

  it("uses the workspace from scoped integration routes before store hydration catches up", () => {
    pathname = "/settings/workspace/ws-2/integrations/github";
    state.workspaces.activeId = "ws-1";

    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByTestId("mock-copy-config").dataset.sourceWorkspaceId).toBe("ws-2");
  });
});
