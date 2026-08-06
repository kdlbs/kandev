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

describe("SettingsLayoutClient", () => {
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

  it("translates the Message Queue breadcrumb and keeps the shared scroll owner", async () => {
    pathname = "/settings/general/message-queue";
    await i18n.changeLanguage("pseudo");
    try {
      render(
        <SettingsLayoutClient>
          <div>Queue settings</div>
        </SettingsLayoutClient>,
      );

      expect(screen.getByText("Ḿēśśàĝē Qũēũē")).toBeTruthy();
      expect(screen.getByTestId("settings-scroll-container").className).toContain(
        "overflow-y-auto",
      );
    } finally {
      await i18n.changeLanguage("en");
    }
  });
});

describe("SettingsLayoutClient workspace breadcrumbs", () => {
  beforeEach(() => {
    pathname = "/settings/workspace/ws-2/secrets";
    state.workspaces.activeId = "ws-1";
    state.setActiveWorkspace.mockClear();
  });

  afterEach(() => cleanup());

  it("includes the workspace name in workspace-scoped breadcrumbs", () => {
    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    // "Settings" renders twice: a phone-only link and the desktop static text.
    expect(screen.getByTestId("page-topbar-breadcrumbs").textContent).toBe(
      "SettingsSettingsArchiveSecrets",
    );
    expect(screen.getByRole("link", { name: "Archive" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-2",
    );
  });

  it("renders the Settings crumb as a phone-only link with static desktop text", () => {
    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    // On desktop /settings hands straight back to the remembered page, so the
    // crumb must not be a link there — otherwise the unsaved-changes guard
    // offers "Discard and leave" and then does not leave.
    const link = screen.getByRole("link", { name: "Settings" });
    expect(link.getAttribute("href")).toBe("/settings");
    expect(link.className).toContain("md:hidden");
    const desktopText = screen
      .getAllByText("Settings")
      .find((el) => el.tagName === "SPAN" && el.className.includes("md:inline"));
    expect(desktopText).toBeTruthy();
  });

  it("keeps the automations parent after the workspace breadcrumb", () => {
    pathname = "/settings/workspace/ws-2/automations/new";

    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    const breadcrumbs = screen.getByTestId("page-topbar-breadcrumbs");
    // The settings crumb chain, in order. Asserted by href rather than by text:
    // the Settings crumb renders twice (phone link + desktop static text) and
    // the title sits inside a BreadcrumbPage wrapper, so counting text nodes
    // measures the markup instead of the chain.
    expect(
      Array.from(breadcrumbs.querySelectorAll('a[href^="/settings"]')).map((link) =>
        link.getAttribute("href"),
      ),
    ).toEqual(["/settings", "/settings/workspace/ws-2", "/settings/workspace/ws-2/automations"]);
    expect(breadcrumbs.textContent).toContain("Archive");
    expect(breadcrumbs.textContent?.endsWith("New")).toBe(true);
  });
});
