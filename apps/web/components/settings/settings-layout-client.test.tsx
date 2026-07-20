import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

let pathname = "/settings/integrations/github";
const COPY_CONFIG_TEST_ID = "mock-copy-config";

const state = {
  configChat: { isOpen: false },
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
  useOptionalAppStore: (selector: (s: typeof state) => unknown, fallback: unknown) =>
    selector(state) ?? fallback,
}));

vi.mock("@/components/page-topbar", () => ({
  PageTopbar: ({ actions, title }: { actions?: ReactNode; title: string }) => (
    <div>
      <span data-testid="page-topbar-title">{title}</span>
      <div data-testid="page-topbar-actions">{actions}</div>
    </div>
  ),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/integrations/integration-copy-config-menu", () => ({
  IntegrationCopyConfigMenu: ({ sourceWorkspaceId }: { sourceWorkspaceId: string }) => (
    <div data-testid={COPY_CONFIG_TEST_ID} data-source-workspace-id={sourceWorkspaceId} />
  ),
}));

import { SettingsLayoutClient } from "./settings-layout-client";
import { useSettingsSaveContributor } from "./settings-save-provider";

function DirtySettings() {
  useSettingsSaveContributor({
    id: "dirty-settings",
    revision: 1,
    isDirty: true,
    save: vi.fn(),
    discard: vi.fn(),
  });
  return <div>Dirty settings</div>;
}

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
    expect(screen.getByTestId(COPY_CONFIG_TEST_ID).dataset.sourceWorkspaceId).toBe("ws-1");
  });

  it("shows copy config on workspace-scoped integration pages", () => {
    pathname = "/settings/workspace/ws-1/integrations/github";

    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByTestId(COPY_CONFIG_TEST_ID).dataset.sourceWorkspaceId).toBe("ws-1");
  });

  it("uses the workspace from scoped integration routes before store hydration catches up", () => {
    pathname = "/settings/workspace/ws-2/integrations/github";
    state.workspaces.activeId = "ws-1";

    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByTestId(COPY_CONFIG_TEST_ID).dataset.sourceWorkspaceId).toBe("ws-2");
  });

  it("falls back to the active workspace when a scoped route has invalid encoding", () => {
    pathname = "/settings/workspace/%E0%A4%A/integrations/github";
    state.workspaces.activeId = "ws-1";

    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByTestId(COPY_CONFIG_TEST_ID).dataset.sourceWorkspaceId).toBe("ws-1");
  });

  it("hosts the route save action and reserves safe-area scroll space", async () => {
    pathname = "/settings/general/appearance";

    render(
      <SettingsLayoutClient>
        <DirtySettings />
      </SettingsLayoutClient>,
    );

    expect(await screen.findByTestId("settings-floating-save")).toBeTruthy();
    expect(screen.getByTestId("settings-scroll-container").className).toContain(
      "safe-area-inset-bottom",
    );
  });

  it("preserves GitHub branding in the deployment App breadcrumb", () => {
    pathname = "/settings/system/github-app";

    render(
      <SettingsLayoutClient>
        <div>GitHub App settings</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByTestId("page-topbar-title").textContent).toBe("GitHub App");
  });
});
