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
  availableAgents: { items: [{ name: "claude", display_name: "Claude Code" }] },
  settingsAgents: {
    items: [{ name: "claude", profiles: [{ id: "prof-1", name: "My Profile101" }] }],
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

// Keep the real PageShell/PageTopbar so the scroll-container and breadcrumb
// assertions test real markup; stub only the nav sheet (store/plugin-heavy)
// and the affordance hook (reads sidebar state this test doesn't model).
vi.mock("@/components/navigation/app-nav-sheet", () => ({
  AppNavSheet: () => null,
}));

vi.mock("@/hooks/use-home-affordance", () => ({
  useHomeAffordance: () => ({ mode: "phone", href: "/?home=overview" }),
}));

vi.mock("@/components/app-status-bar/app-status-surface-provider", () => ({
  AppStatusDrawerTrigger: () => null,
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
import { i18n } from "@/lib/i18n";

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
    expect(screen.getByTestId("settings-scroll-container").className).toContain(
      "app-status-bar-height",
    );
  });

  it("translates the Task behavior breadcrumb and keeps the shared scroll owner", async () => {
    pathname = "/settings/preferences/task-behavior";
    await i18n.changeLanguage("pseudo");
    try {
      render(
        <SettingsLayoutClient>
          <div>Queue settings</div>
        </SettingsLayoutClient>,
      );

      expect(screen.getByText("Ţàśķ Ɓēĥàvĩōŕ")).toBeTruthy();
      expect(screen.getByTestId("settings-scroll-container").className).toContain(
        "overflow-y-auto",
      );
    } finally {
      await i18n.changeLanguage("en");
    }
  });
});

describe("SettingsLayoutClient breadcrumbs", () => {
  afterEach(() => cleanup());

  it("renders the Settings crumb as a phone-only link with static desktop text", () => {
    pathname = "/settings/preferences/appearance";

    render(
      <SettingsLayoutClient>
        <div>Appearance settings</div>
      </SettingsLayoutClient>,
    );

    const link = screen.getByRole("link", { name: "Settings" });
    expect(link.getAttribute("href")).toBe("/settings");
    expect(link.className).toContain("md:hidden");
    const desktopText = screen
      .getAllByText("Settings")
      .find((el) => el.tagName === "SPAN" && el.className.includes("md:inline"));
    expect(desktopText).toBeTruthy();
  });

  it("links Workspaces and the workspace name on workspace sub-pages", () => {
    pathname = "/settings/workspaces/ws-1/repositories";

    render(
      <SettingsLayoutClient>
        <div>Repositories</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByRole("link", { name: "Workspaces" }).getAttribute("href")).toBe(
      "/settings/workspaces",
    );
    expect(screen.getByRole("link", { name: "Default" }).getAttribute("href")).toBe(
      "/settings/workspaces/ws-1",
    );
  });

  it("titles the workspace overview with the workspace name", () => {
    pathname = "/settings/workspaces/ws-1";

    const { container } = render(
      <SettingsLayoutClient>
        <div>Overview</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByRole("link", { name: "Workspaces" }).getAttribute("href")).toBe(
      "/settings/workspaces",
    );
    // The workspace name is the current page crumb (no anchor of its own).
    expect(container.querySelector('a[href="/settings/workspaces/ws-1"]')).toBeNull();
    expect(screen.getByText("Default").closest('[aria-current="page"]')).toBeTruthy();
  });

  it("links Agents and titles the create page with the display name", () => {
    pathname = "/settings/agents/claude";

    const { container } = render(
      <SettingsLayoutClient>
        <div>Agent setup</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByRole("link", { name: "Agents" }).getAttribute("href")).toBe(
      "/settings/agents",
    );
    expect(container.querySelector('a[href="/settings/agents/claude"]')).toBeNull();
    expect(screen.getByText("Claude Code").closest('[aria-current="page"]')).toBeTruthy();
  });

  it("titles profile pages with the profile name, directly under Agents", () => {
    pathname = "/settings/agents/claude/profiles/prof-1";

    const { container } = render(
      <SettingsLayoutClient>
        <div>Profile editor</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByRole("link", { name: "Agents" }).getAttribute("href")).toBe(
      "/settings/agents",
    );
    // The saved agent has no page of its own, so no agent-name crumb.
    expect(container.querySelector('a[href="/settings/agents/claude"]')).toBeNull();
    expect(screen.getByText("My Profile101").closest('[aria-current="page"]')).toBeTruthy();
  });
});
