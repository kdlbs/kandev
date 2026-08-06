import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

let pathname = "/settings/integrations/github";
const COPY_CONFIG_TEST_ID = "mock-copy-config";

// The rendered current-page crumb; PageTopbar marks it, not the parents.
const CURRENT_PAGE = '[aria-current="page"]';
const WORKSPACES_HREF = "/settings/workspaces";
const WS_HREF = `${WORKSPACES_HREF}/ws-1`;

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
  executors: {
    items: [
      {
        id: "exec-1",
        name: "local-docker",
        profiles: [{ id: "exec-prof-1", name: "Alpha" }],
      },
    ],
  },
  automations: { items: [{ id: "auto-1", workspace_id: "ws-1", name: "Nightly sync" }] },
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
    pathname = `${WS_HREF}/integrations/github`;

    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByTestId(COPY_CONFIG_TEST_ID).dataset.sourceWorkspaceId).toBe("ws-1");
  });

  // The route the app actually renders. `integrationFromPathname` accepts both
  // spellings, so the source workspace has to be read off both too — parsing
  // only the legacy singular path left this action copying the *active*
  // workspace's credentials while the breadcrumb named the routed one.
  it.each([
    ["canonical", "/settings/workspaces/ws-2/integrations/github"],
    ["legacy", "/settings/workspace/ws-2/integrations/github"],
  ])("copies from the routed workspace, not the active one (%s path)", (_label, path) => {
    pathname = path;
    state.workspaces.activeId = "ws-1";

    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByTestId(COPY_CONFIG_TEST_ID).dataset.sourceWorkspaceId).toBe("ws-2");
  });

  // A URL that names a workspace and cannot resolve it has no safe source: the
  // active workspace is not what the route referred to, and copying its
  // credentials into the chosen target would be silent and wrong.
  it.each([
    ["deleted since the URL was saved", "/settings/workspaces/ws-deleted/integrations/github"],
    ["a malformed segment", "/settings/workspaces/%E0%A4%A/integrations/github"],
  ])("offers no copy action when the routed workspace is %s", (_label, path) => {
    pathname = path;
    state.workspaces.activeId = "ws-1";

    render(
      <SettingsLayoutClient>
        <div>Settings page</div>
      </SettingsLayoutClient>,
    );

    expect(screen.queryByTestId(COPY_CONFIG_TEST_ID)).toBeNull();
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
    pathname = `${WS_HREF}/repositories`;

    render(
      <SettingsLayoutClient>
        <div>Repositories</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByRole("link", { name: "Workspaces" }).getAttribute("href")).toBe(
      WORKSPACES_HREF,
    );
    expect(screen.getByRole("link", { name: "Default" }).getAttribute("href")).toBe(
      "/settings/workspaces/ws-1",
    );
  });

  it("titles the workspace overview with the workspace name", () => {
    pathname = WS_HREF;

    const { container } = render(
      <SettingsLayoutClient>
        <div>Overview</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByRole("link", { name: "Workspaces" }).getAttribute("href")).toBe(
      WORKSPACES_HREF,
    );
    // The workspace name is the current page crumb (no anchor of its own).
    expect(container.querySelector(`a[href="${WS_HREF}"]`)).toBeNull();
    expect(screen.getByText("Default").closest(CURRENT_PAGE)).toBeTruthy();
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
    expect(screen.getByText("Claude Code").closest(CURRENT_PAGE)).toBeTruthy();
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
    expect(screen.getByText("My Profile101").closest(CURRENT_PAGE)).toBeTruthy();
  });

  // Integrations is a page of its own, so an integration service hangs off it
  // rather than jumping from the workspace straight to "GitHub".
});

// The chains the URL-segment heuristic could not express: a section that is
// itself a page, and record pages whose parent is another record.
describe("SettingsLayoutClient record breadcrumbs", () => {
  afterEach(() => cleanup());

  it("links Integrations from an integration service page", () => {
    pathname = `${WS_HREF}/integrations/github`;

    render(
      <SettingsLayoutClient>
        <div>GitHub settings</div>
      </SettingsLayoutClient>,
    );

    expect(crumbHrefs()).toEqual([
      "/settings",
      WORKSPACES_HREF,
      "/settings/workspaces/ws-1",
      "/settings/workspaces/ws-1/integrations",
    ]);
    expect(screen.getByText("GitHub").closest(CURRENT_PAGE)).toBeTruthy();
  });

  // The automation id was skipped as unreadable and the title fell back to the
  // section segment, so this page read "… › Automations › Automations".
  it("titles an automation with its name under a linked Automations crumb", () => {
    pathname = `${WS_HREF}/automations/auto-1`;

    render(
      <SettingsLayoutClient>
        <div>Automation editor</div>
      </SettingsLayoutClient>,
    );

    expect(screen.getByRole("link", { name: "Automations" }).getAttribute("href")).toBe(
      "/settings/workspaces/ws-1/automations",
    );
    expect(screen.getByText("Nightly sync").closest(CURRENT_PAGE)).toBeTruthy();
  });

  it("puts an executor profile under its executor, under Executors", () => {
    pathname = "/settings/executor/exec-1/profile/exec-prof-1";

    render(
      <SettingsLayoutClient>
        <div>Profile editor</div>
      </SettingsLayoutClient>,
    );

    expect(crumbHrefs()).toEqual(["/settings", "/settings/executors", "/settings/executor/exec-1"]);
    expect(screen.getByRole("link", { name: "local-docker" })).toBeTruthy();
    expect(screen.getByText("Alpha").closest(CURRENT_PAGE)).toBeTruthy();
  });

  it("names the Executors page for a bare executor route", () => {
    pathname = "/settings/executors/exec-prof-1";

    render(
      <SettingsLayoutClient>
        <div>Profile editor</div>
      </SettingsLayoutClient>,
    );

    expect(crumbHrefs()).toEqual(["/settings", "/settings/executors"]);
    expect(screen.getByRole("link", { name: "Executors" })).toBeTruthy();
  });
});

/** Parent crumb hrefs in order, read off the rendered breadcrumb nav. */
function crumbHrefs(): string[] {
  const nav = screen.getByRole("navigation", { name: "breadcrumb" });
  return [...nav.querySelectorAll("a[href]")]
    .map((link) => link.getAttribute("href") ?? "")
    .filter((href) => href.startsWith("/settings"));
}
