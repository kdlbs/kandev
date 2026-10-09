import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { IconChartBar } from "@tabler/icons-react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { t } from "@/lib/i18n";

const mocks = vi.hoisted(() => ({
  routerPush: vi.fn(),
  toggleSettingsMode: vi.fn(),
  logout: vi.fn().mockResolvedValue(undefined),
  setImproveDialogOpen: vi.fn(),
  openReleaseNotes: vi.fn(),
}));

// Shared with the `@kandev/ui/dropdown-menu` mock below so both the mock and
// the assertions that scope to it stay in lockstep. Defined via `vi.hoisted`
// (not a plain module-scope `const`) because `vi.mock` factories run during
// this file's static import of `./app-sidebar-footer` (see that import
// below), which executes before any later top-level `const` in this file
// would have run.
const overflowMenuTestIds = vi.hoisted(() => ({
  content: "sidebar-plugin-overflow-content",
}));

const state = {
  workspaces: {
    activeId: "kanban-1" as string | null,
    items: [
      { id: "kanban-1", name: "Kanban", office_workflow_id: "" },
      { id: "office-1", name: "Office", office_workflow_id: "wf-office" },
      { id: "office-2", name: "Office 2", office_workflow_id: "wf-office-2" },
    ],
  },
  appSidebar: { settingsMode: false, improveDialogOpen: false },
  setImproveDialogOpen: mocks.setImproveDialogOpen,
  auth: {
    mode: "disabled" as string,
    user: null as { display_name: string; email: string } | null,
  },
  connection: { issueSeverity: "none" as "none" | "unstable" | "lost" },
  userSettings: { appStatusBarEnabled: true, startupPage: "task_overview" },
};

const DEFAULT_PATHNAME = "/tasks/session-1";
let pathname = DEFAULT_PATHNAME;

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({
    push: (...args: [string, { onNavigated?: () => void }?]) => {
      // Forward verbatim so callers passing no options still assert as a
      // single-argument call.
      mocks.routerPush(...args);
      // The real router runs onNavigated only once the push commits; the
      // unsaved-changes guard can cancel it, which `blockNavigation` models.
      if (!blockNavigation) args[1]?.onNavigated?.();
    },
  }),
  usePathname: () => pathname,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => false,
}));

// The footer renders its insight buttons from the navigation manifest; the
// manifest itself is covered in `lib/navigation/core-destinations.test.ts` and
// `lib/navigation/plugin-destinations.test.ts`. `insightDestinations` is
// mutable so individual tests can inject plugin entries alongside `stats`.
type FooterDestination = {
  id: string;
  label: string;
  icon: typeof IconChartBar;
  section: string;
  href: string;
  source?: "plugin";
  pluginItemId?: string;
};

const STATS_LABEL = "Stats";
const STATS_BUTTON_TEST_ID = "sidebar-stats-button";
const ARIA_LABEL_ATTRIBUTE = "aria-label";

const STATS_DESTINATION: FooterDestination = {
  id: "stats",
  label: STATS_LABEL,
  icon: IconChartBar,
  section: "insights",
  href: "/stats",
};

let insightDestinations: FooterDestination[] = [STATS_DESTINATION];
let releaseNotesAvailable = false;
let releaseNotesUnseen = true;
let releaseNotificationsEnabled = true;

vi.mock("@/hooks/use-app-destinations", () => ({
  useStaticDestinations: () => insightDestinations,
}));

vi.mock("@/hooks/use-release-notes", () => ({
  useReleaseNotes: () => ({
    unseenEntries: [],
    latestVersion: "0.0.0",
    hasUnseen: releaseNotesAvailable && releaseNotesUnseen,
    dialogOpen: false,
    openDialog: mocks.openReleaseNotes,
    closeDialog: vi.fn(),
    hasNotes: releaseNotesAvailable,
    showTopbarButton: releaseNotesAvailable && releaseNotesUnseen && releaseNotificationsEnabled,
  }),
}));

vi.mock("@/components/improve-kandev-dialog", () => ({
  ImproveKandevDialog: () => null,
}));

vi.mock("@/components/release-notes/release-notes-dialog", () => ({
  ReleaseNotesDialog: () => null,
}));

vi.mock("@/components/theme-toggle", () => ({
  ThemeToggle: () => <button type="button">Theme</button>,
}));

vi.mock("@/lib/api/domains/auth-api", () => ({
  logout: mocks.logout,
}));

// Radix Tooltip's hover/focus-triggered visibility isn't modelled well by
// jsdom; render both the trigger and its content unconditionally, matching
// this repo's convention elsewhere (see agents-section.test.tsx) so tooltip
// text is directly assertable without simulating hover or focus.
vi.mock("@kandev/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

// Radix dropdown primitives rely on pointer/portal behaviour that jsdom
// doesn't model well; render them as plain elements so clicks reach the
// current-user chip's own logic (see app-sidebar-workspace-picker.test.tsx).
// `DropdownMenuContent`'s wrapper carries a fixed testid (not gated on open
// state, matching this repo's existing dropdown-menu mock convention) so
// capacity tests can scope `within()` to it and prove an item is actually
// inside the overflow menu rather than merely present somewhere in the
// document.
vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenuLabel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuSeparator: () => <hr />,
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid={overflowMenuTestIds.content}>{children}</div>
  ),
  DropdownMenuItem: ({
    children,
    onClick,
    "data-testid": testId,
  }: {
    children: React.ReactNode;
    onClick?: () => void;
    "data-testid"?: string;
  }) => (
    <button type="button" data-testid={testId} onClick={() => onClick?.()}>
      {children}
    </button>
  ),
}));

import { AppSidebarFooter } from "./app-sidebar-footer";

function renderFooter(collapsed = false, layoutManaged = false) {
  return render(
    <TooltipProvider>
      <AppSidebarFooter
        collapsed={collapsed}
        onToggleSettingsMode={mocks.toggleSettingsMode}
        layoutManaged={layoutManaged}
      />
    </TooltipProvider>,
  );
}

function resetFooterState() {
  blockNavigation = false;
  releaseNotesAvailable = false;
  releaseNotesUnseen = true;
  releaseNotificationsEnabled = true;
  mocks.openReleaseNotes.mockClear();
  mocks.setImproveDialogOpen.mockClear();
  pathname = DEFAULT_PATHNAME;
  state.workspaces.activeId = "kanban-1";
  state.workspaces.items = [
    { id: "kanban-1", name: "Kanban", office_workflow_id: "" },
    { id: "office-1", name: "Office", office_workflow_id: "wf-office" },
    { id: "office-2", name: "Office 2", office_workflow_id: "wf-office-2" },
  ];
  state.appSidebar.settingsMode = false;
  state.auth = { mode: "disabled", user: null };
  state.connection.issueSeverity = "none";
  state.userSettings.appStatusBarEnabled = true;
  mocks.routerPush.mockClear();
  mocks.toggleSettingsMode.mockClear();
  insightDestinations = [STATS_DESTINATION];
}

let blockNavigation = false;

// Where the settings gear lands when it closes settings mode: the active
// workspace's home (kanban-1 in this suite's default state).
const KANBAN_HOME_HREF = "/?home=overview&workspaceId=kanban-1";
const GEAR_TEST_ID = "sidebar-settings-gear";

describe("AppSidebarFooter", () => {
  beforeEach(resetFooterState);

  afterEach(() => cleanup());

  it("renders Stats as a direct footer link outside the utilities menu", () => {
    renderFooter();

    const footer = screen.getByTestId("sidebar-footer");
    const statsLink = within(footer).getByTestId(STATS_BUTTON_TEST_ID);

    expect(statsLink).toBeTruthy();
    expect(statsLink.getAttribute("href")).toBe("/stats");
    expect(statsLink.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(STATS_LABEL);
    expect(statsLink.getAttribute("aria-current")).toBeNull();
    expect(within(overflowMenuContent()).queryByTestId(STATS_BUTTON_TEST_ID)).toBeNull();

    const orderedControls = [
      within(footer).getByTestId("sidebar-settings-gear"),
      statsLink,
      screen.getByRole("button", { name: "Theme" }),
      within(footer).getByTestId(OVERFLOW_TRIGGER_TEST_ID),
    ];
    for (let index = 0; index < orderedControls.length - 1; index++) {
      expect(
        orderedControls[index].compareDocumentPosition(orderedControls[index + 1]) &
          Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    }
  });

  it("links to Stats from the footer", () => {
    renderFooter();

    expect(screen.getByTestId(STATS_BUTTON_TEST_ID).getAttribute("href")).toBe("/stats");
  });

  it("marks Stats as the current navigation destination", () => {
    pathname = "/stats";
    renderFooter();

    const statsLink = screen.getByRole("link", { name: STATS_LABEL });
    expect(statsLink.getAttribute("aria-current")).toBe("page");
    expect(statsLink.getAttribute("href")).toBe("/stats");
  });

  it("leaves plugin insight destinations to the saved sidebar layout", () => {
    insightDestinations = [
      STATS_DESTINATION,
      {
        id: "plugin:acme:board",
        label: "Acme Board",
        icon: IconChartBar,
        section: "insights",
        href: "/plugins/acme",
        source: "plugin",
      },
    ];

    renderFooter(false, true);

    expect(screen.getByTestId(STATS_BUTTON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId("sidebar-plugin:acme:board-button")).toBeNull();
  });

  it("does not render a mode switch button", () => {
    // Mode follows the active workspace, and workspaces are switched through
    // the picker in the sidebar header. The footer used to carry a dedicated
    // Office↔Kanban button; it is gone, not merely feature-gated off.
    renderFooter();

    expect(screen.queryByTestId("sidebar-office-button")).toBeNull();
    expect(screen.queryByTestId("sidebar-kanban-button")).toBeNull();
  });
});

describe("AppSidebarFooter settings gear", () => {
  beforeEach(resetFooterState);
  afterEach(() => cleanup());

  it("navigates to /settings when the gear opens settings mode from a non-settings route", () => {
    pathname = DEFAULT_PATHNAME;
    state.appSidebar.settingsMode = false;

    renderFooter();
    fireEvent.click(screen.getByTestId(GEAR_TEST_ID));

    expect(mocks.routerPush).toHaveBeenCalledWith("/settings", expect.anything());
    expect(mocks.toggleSettingsMode).toHaveBeenCalledOnce();
  });

  it("does not navigate when the gear reopens settings mode while already on a settings route", () => {
    pathname = "/settings/agents";
    state.appSidebar.settingsMode = false;

    renderFooter();
    fireEvent.click(screen.getByTestId(GEAR_TEST_ID));

    expect(mocks.routerPush).not.toHaveBeenCalled();
    expect(mocks.toggleSettingsMode).toHaveBeenCalledOnce();
  });

  it("leaves the settings surface when the gear closes an open settings mode", () => {
    pathname = "/settings/general/appearance";
    state.appSidebar.settingsMode = true;

    renderFooter();
    fireEvent.click(screen.getByTestId(GEAR_TEST_ID));

    // Swapping the sidebar back while the main panel stayed on a settings page
    // left kanban navigation beside an open settings page.
    expect(mocks.routerPush).toHaveBeenCalledWith(KANBAN_HOME_HREF, expect.anything());
    expect(mocks.toggleSettingsMode).toHaveBeenCalledOnce();
  });

  // The unsaved-changes guard cancels the push when the user picks "Continue
  // editing". Toggling anyway left the URL in Settings with the sidebar already
  // back on kanban navigation.
  it("keeps the sidebar in settings mode when the guard cancels the navigation", () => {
    pathname = "/settings/general/appearance";
    state.appSidebar.settingsMode = true;
    blockNavigation = true;

    renderFooter();
    fireEvent.click(screen.getByTestId(GEAR_TEST_ID));

    expect(mocks.routerPush).toHaveBeenCalledWith(KANBAN_HOME_HREF, expect.anything());
    expect(mocks.toggleSettingsMode).not.toHaveBeenCalled();
  });

  it("does not open settings mode when the guard cancels the way in", () => {
    pathname = DEFAULT_PATHNAME;
    state.appSidebar.settingsMode = false;
    blockNavigation = true;

    renderFooter();
    fireEvent.click(screen.getByTestId(GEAR_TEST_ID));

    expect(mocks.routerPush).toHaveBeenCalledWith("/settings", expect.anything());
    expect(mocks.toggleSettingsMode).not.toHaveBeenCalled();
  });

  it("does not navigate when the gear closes settings mode off a settings route", () => {
    pathname = "/tasks";
    state.appSidebar.settingsMode = true;

    renderFooter();
    fireEvent.click(screen.getByTestId(GEAR_TEST_ID));

    expect(mocks.routerPush).not.toHaveBeenCalled();
    expect(mocks.toggleSettingsMode).toHaveBeenCalledOnce();
  });
});

describe("AppSidebarFooter connection fallback", () => {
  beforeEach(() => {
    state.userSettings.appStatusBarEnabled = true;
    state.connection.issueSeverity = "none";
  });

  afterEach(cleanup);

  it("keeps the connection fallback visible during an outage", () => {
    state.userSettings.appStatusBarEnabled = false;
    state.connection.issueSeverity = "unstable";

    renderFooter();

    const warning = screen.getByTestId("sidebar-connection-warning");
    expect(warning.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(
      "Connection unstable. Reconnecting to Kandev.",
    );
    expect(warning.getAttribute("data-connection-severity")).toBe("unstable");
    expect(warning.closest('[data-testid="sidebar-footer"]')).not.toBeNull();
  });

  it("does not duplicate the warning fallback when the app status bar is enabled", () => {
    state.connection.issueSeverity = "lost";

    renderFooter();

    expect(screen.queryByTestId("sidebar-connection-warning")).toBeNull();
  });
});

describe("AppSidebarFooter current-user chip", () => {
  beforeEach(() => {
    pathname = DEFAULT_PATHNAME;
    state.appSidebar.settingsMode = false;
    state.auth = { mode: "disabled", user: null };
    mocks.logout.mockClear();
  });

  afterEach(() => cleanup());

  it("does not render the current-user chip in disabled auth mode", () => {
    state.auth = { mode: "disabled", user: null };

    renderFooter();

    expect(screen.queryByTestId("current-user-chip")).toBeNull();
  });

  it("does not render the current-user chip when enabled mode has no user yet", () => {
    state.auth = { mode: "enabled", user: null };

    renderFooter();

    expect(screen.queryByTestId("current-user-chip")).toBeNull();
  });

  it("renders the current-user chip and logs out when enabled with a user", () => {
    state.auth = {
      mode: "enabled",
      user: { display_name: "Jane Doe", email: "jane@example.com" },
    };

    renderFooter();

    const chip = screen.getByTestId("current-user-chip");
    expect(chip.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe("Jane Doe");

    fireEvent.click(screen.getByTestId("current-user-logout"));

    expect(mocks.logout).toHaveBeenCalledOnce();
  });
});

function pluginDestination(i: number): FooterDestination {
  return {
    id: `plugin:acme-${i}:board`,
    label: `Acme Board ${i}`,
    icon: IconChartBar,
    section: "insights",
    href: `/plugins/acme-${i}`,
    source: "plugin" as const,
    pluginItemId: "board",
  };
}

function pluginDestinations(count: number): FooterDestination[] {
  return Array.from({ length: count }, (_, i) => pluginDestination(i));
}

const OVERFLOW_TRIGGER_TEST_ID = "sidebar-footer-more-button";

function overflowMenuContent() {
  return screen.getByTestId(overflowMenuTestIds.content);
}

describe("AppSidebarFooter utilities menu", () => {
  beforeEach(resetFooterState);
  afterEach(cleanup);

  it("keeps Improve Kandev in the menu while Stats stays in the footer", () => {
    renderFooter();
    expect(screen.getByTestId(OVERFLOW_TRIGGER_TEST_ID).getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(
      t("common:showMoreActions"),
    );
    const menu = within(overflowMenuContent());
    expect(menu.queryByTestId(STATS_BUTTON_TEST_ID)).toBeNull();
    expect(screen.getByTestId(STATS_BUTTON_TEST_ID)).toBeTruthy();
    fireEvent.click(menu.getByTestId("sidebar-improve-kandev-button"));
    expect(mocks.setImproveDialogOpen).toHaveBeenCalledWith(true);
  });

  it("opens available release notes from the utilities menu", () => {
    releaseNotesAvailable = true;
    renderFooter();
    fireEvent.click(within(overflowMenuContent()).getByTestId("sidebar-release-notes-button"));
    expect(mocks.openReleaseNotes).toHaveBeenCalledOnce();
  });

  it("omits release notes when unavailable", () => {
    renderFooter();
    expect(screen.queryByTestId("sidebar-release-notes-button")).toBeNull();
  });

  it.each(["seen", "notifications disabled"])(
    "keeps available release notes reachable when %s",
    (state) => {
      releaseNotesAvailable = true;
      releaseNotesUnseen = state !== "seen";
      releaseNotificationsEnabled = state !== "notifications disabled";
      renderFooter();
      fireEvent.click(within(overflowMenuContent()).getByTestId("sidebar-release-notes-button"));
      expect(mocks.openReleaseNotes).toHaveBeenCalledOnce();
    },
  );

  it.each([false, true])(
    "keeps every plugin reachable in registration order, collapsed=%s",
    (collapsed) => {
      insightDestinations = [STATS_DESTINATION, ...pluginDestinations(8)];
      renderFooter(collapsed);
      const menu = overflowMenuContent();
      expect(within(menu).queryByTestId(STATS_BUTTON_TEST_ID)).toBeNull();
      expect(screen.getByTestId(STATS_BUTTON_TEST_ID)).toBeTruthy();
      const plugins = menu.querySelectorAll('[data-testid^="sidebar-plugin:"]');
      expect(Array.from(plugins, (item) => item.textContent)).toEqual(
        Array.from({ length: 8 }, (_, i) => `Acme Board ${i}`),
      );
      fireEvent.click(within(menu).getByTestId("sidebar-plugin:acme-7:board-button"));
      expect(mocks.routerPush).toHaveBeenCalledWith("/plugins/acme-7");
    },
  );

  it("reflects changed registration order without dropping or duplicating entries", () => {
    insightDestinations = [STATS_DESTINATION, pluginDestination(2), pluginDestination(1)];
    renderFooter();
    const plugins = overflowMenuContent().querySelectorAll('[data-testid^="sidebar-plugin:"]');
    expect(Array.from(plugins, (item) => item.textContent)).toEqual([
      "Acme Board 2",
      "Acme Board 1",
    ]);
  });
});
