import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { IconChartBar } from "@tabler/icons-react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  routerPush: vi.fn(),
  toggleSettingsMode: vi.fn(),
  logout: vi.fn().mockResolvedValue(undefined),
  setImproveDialogOpen: vi.fn(),
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
  userSettings: { appStatusBarEnabled: true },
};

let officeEnabled = false;
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
  useFeature: () => officeEnabled,
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

const STATS_DESTINATION: FooterDestination = {
  id: "stats",
  label: "Stats",
  icon: IconChartBar,
  section: "insights",
  href: "/stats",
};

let insightDestinations: FooterDestination[] = [STATS_DESTINATION];

vi.mock("@/hooks/use-app-destinations", () => ({
  useStaticDestinations: () => insightDestinations,
}));

vi.mock("@/hooks/use-release-notes", () => ({
  useReleaseNotes: () => ({
    unseenEntries: [],
    latestVersion: "0.0.0",
    hasUnseen: false,
    dialogOpen: false,
    openDialog: vi.fn(),
    closeDialog: vi.fn(),
    hasNotes: false,
    showTopbarButton: false,
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
vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
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

import { AppSidebarFooter, MAX_INLINE_PLUGIN_FOOTER_ITEMS } from "./app-sidebar-footer";

function renderFooter(collapsed = false) {
  return render(
    <TooltipProvider>
      <AppSidebarFooter collapsed={collapsed} onToggleSettingsMode={mocks.toggleSettingsMode} />
    </TooltipProvider>,
  );
}

function resetFooterState() {
  officeEnabled = false;
  blockNavigation = false;
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
  window.localStorage.clear();
  document.cookie = "office-active-workspace=; path=/; max-age=0";
  mocks.routerPush.mockClear();
  mocks.toggleSettingsMode.mockClear();
  insightDestinations = [STATS_DESTINATION];
}

const KANBAN_HOME_HREF = "/?home=overview&workspaceId=kanban-1";
let blockNavigation = false;

const GEAR_TEST_ID = "sidebar-settings-gear";

describe("AppSidebarFooter", () => {
  beforeEach(resetFooterState);

  afterEach(() => cleanup());

  it("renders navigation icons as buttons so hover does not expose link URLs", () => {
    officeEnabled = true;

    renderFooter();

    const statsButton = screen.getByRole("button", { name: "Stats" });
    const officeButton = screen.getByRole("button", { name: "Office" });

    expect(statsButton).toBeTruthy();
    expect(officeButton).toBeTruthy();
    expect(statsButton.getAttribute("href")).toBeNull();
    expect(officeButton.getAttribute("href")).toBeNull();
    expect(screen.queryByRole("link", { name: "Stats" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Office" })).toBeNull();
  });

  it("navigates from the Stats and Office footer buttons when kanban is active", () => {
    officeEnabled = true;

    renderFooter();

    fireEvent.click(screen.getByRole("button", { name: "Stats" }));
    fireEvent.click(screen.getByRole("button", { name: "Office" }));

    expect(mocks.routerPush).toHaveBeenNthCalledWith(1, "/stats");
    expect(mocks.routerPush).toHaveBeenNthCalledWith(2, "/office?workspaceId=office-1");
    expect(window.localStorage.getItem("kandev.lastKanbanWorkspaceId")).toBe("kanban-1");
  });

  it("navigates to the last active office workspace when kanban is active", () => {
    officeEnabled = true;
    document.cookie = "office-active-workspace=office-2; path=/";

    renderFooter();

    fireEvent.click(screen.getByRole("button", { name: "Office" }));

    expect(mocks.routerPush).toHaveBeenCalledWith("/office?workspaceId=office-2");
  });

  it("navigates to office setup when no office workspace exists", () => {
    officeEnabled = true;
    state.workspaces.items = [{ id: "kanban-1", name: "Kanban", office_workflow_id: "" }];

    renderFooter();

    fireEvent.click(screen.getByRole("button", { name: "Office" }));

    expect(mocks.routerPush).toHaveBeenCalledWith("/office/setup?mode=new");
  });

  it("shows a Kanban button when an office workspace is active", () => {
    officeEnabled = true;
    state.workspaces.activeId = "office-1";
    window.localStorage.setItem("kandev.lastKanbanWorkspaceId", "kanban-1");

    renderFooter();

    expect(screen.queryByRole("button", { name: "Office" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Kanban" }));

    expect(mocks.routerPush).toHaveBeenCalledWith(KANBAN_HOME_HREF);
  });

  it("remembers the current office workspace when toggling back to kanban", () => {
    officeEnabled = true;
    state.workspaces.activeId = "office-2";
    window.localStorage.setItem("kandev.lastKanbanWorkspaceId", "kanban-1");

    renderFooter();

    fireEvent.click(screen.getByRole("button", { name: "Kanban" }));

    expect(document.cookie).toContain("office-active-workspace=office-2");
    expect(mocks.routerPush).toHaveBeenCalledWith(KANBAN_HOME_HREF);
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

  it("shows the connection fallback immediately after the theme control during an outage", () => {
    state.userSettings.appStatusBarEnabled = false;
    state.connection.issueSeverity = "unstable";

    renderFooter();

    const warning = screen.getByTestId("sidebar-connection-warning");
    expect(warning.getAttribute("aria-label")).toBe("Connection unstable. Reconnecting to Kandev.");
    expect(warning.getAttribute("data-connection-severity")).toBe("unstable");
    expect(screen.getByRole("button", { name: "Theme" }).nextElementSibling).toBe(warning);
  });

  it("does not duplicate the warning fallback when the app status bar is enabled", () => {
    state.connection.issueSeverity = "lost";

    renderFooter();

    expect(screen.queryByTestId("sidebar-connection-warning")).toBeNull();
  });
});

describe("AppSidebarFooter current-user chip", () => {
  beforeEach(() => {
    officeEnabled = false;
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
    expect(chip.textContent).toContain("Jane Doe");

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

function eightPluginDestinations(): FooterDestination[] {
  return [STATS_DESTINATION, ...pluginDestinations(8)];
}

const OVERFLOW_TRIGGER_TEST_ID = "sidebar-plugin-overflow-button";
const MORE_PLUGIN_ITEMS_LABEL = "More plugin items";

describe("AppSidebarFooter plugin insights items", () => {
  beforeEach(resetFooterState);

  afterEach(() => cleanup());

  it("renders a plugin insights destination as an icon button with the owner-namespaced testid, and navigates on click", () => {
    insightDestinations = [
      STATS_DESTINATION,
      {
        id: "plugin:acme:board",
        label: "Acme Board",
        icon: IconChartBar,
        section: "insights",
        href: "/plugins/acme",
        source: "plugin",
        pluginItemId: "board",
      },
    ];

    renderFooter();

    const button = screen.getByTestId("sidebar-plugin:acme:board-button");
    expect(button).not.toBeNull();
    expect(button.getAttribute("aria-label")).toBe("Acme Board");

    fireEvent.click(button);

    expect(mocks.routerPush).toHaveBeenCalledWith("/plugins/acme");
  });
});

describe("AppSidebarFooter plugin footer capacity and overflow", () => {
  beforeEach(resetFooterState);

  afterEach(() => cleanup());

  it("renders no overflow trigger when no plugin registers a sidebar-footer item (P = 0)", () => {
    insightDestinations = [STATS_DESTINATION];

    renderFooter();

    expect(screen.queryByTestId(OVERFLOW_TRIGGER_TEST_ID)).toBeNull();
  });

  it("renders all plugin buttons inline with no trigger when P is at the budget", () => {
    insightDestinations = [
      STATS_DESTINATION,
      ...pluginDestinations(MAX_INLINE_PLUGIN_FOOTER_ITEMS),
    ];

    renderFooter();

    for (let i = 0; i < MAX_INLINE_PLUGIN_FOOTER_ITEMS; i++) {
      expect(screen.getByTestId(`sidebar-plugin:acme-${i}:board-button`)).not.toBeNull();
    }
    expect(screen.queryByTestId(OVERFLOW_TRIGGER_TEST_ID)).toBeNull();
  });

  it("partitions the first over-budget item into the overflow trigger's menu", () => {
    insightDestinations = [
      STATS_DESTINATION,
      ...pluginDestinations(MAX_INLINE_PLUGIN_FOOTER_ITEMS + 1),
    ];

    renderFooter();

    for (let i = 0; i < MAX_INLINE_PLUGIN_FOOTER_ITEMS; i++) {
      expect(screen.getByTestId(`sidebar-plugin:acme-${i}:board-button`)).not.toBeNull();
    }
    const trigger = screen.getByTestId(OVERFLOW_TRIGGER_TEST_ID);
    expect(trigger).not.toBeNull();

    // The over-budget item's menu item shares its testid with what an inline
    // button would use (see spec.md#Rendered-identity); open the menu before
    // asserting presence, per spec.md#The-guarantee, rather than asserting
    // absence beforehand.
    const overIndex = MAX_INLINE_PLUGIN_FOOTER_ITEMS;
    fireEvent.click(trigger);
    const menuItem = screen.getByTestId(`sidebar-plugin:acme-${overIndex}:board-button`);
    expect(menuItem.textContent).toContain(`Acme Board ${overIndex}`);

    fireEvent.click(menuItem);
    expect(mocks.routerPush).toHaveBeenCalledWith(`/plugins/acme-${overIndex}`);
  });

  it("labels the overflow trigger's accessible name and tooltip from the sidebar:morePluginItems key", () => {
    insightDestinations = [
      STATS_DESTINATION,
      ...pluginDestinations(MAX_INLINE_PLUGIN_FOOTER_ITEMS + 1),
    ];

    renderFooter();

    const trigger = screen.getByTestId(OVERFLOW_TRIGGER_TEST_ID);
    expect(trigger.getAttribute("aria-label")).toBe(MORE_PLUGIN_ITEMS_LABEL);
    expect(screen.getByText(MORE_PLUGIN_ITEMS_LABEL)).not.toBeNull();
  });

  it("keeps the budget at 3 inline plus one trigger for 8 plugins, expanded, dropping none", () => {
    insightDestinations = eightPluginDestinations();

    renderFooter(false);

    for (let i = 0; i < MAX_INLINE_PLUGIN_FOOTER_ITEMS; i++) {
      expect(screen.getByTestId(`sidebar-plugin:acme-${i}:board-button`)).not.toBeNull();
    }
    const trigger = screen.getByTestId(OVERFLOW_TRIGGER_TEST_ID);
    fireEvent.click(trigger);
    for (let i = MAX_INLINE_PLUGIN_FOOTER_ITEMS; i < 8; i++) {
      expect(screen.getByTestId(`sidebar-plugin:acme-${i}:board-button`)).not.toBeNull();
    }
    const container = screen.getByTestId("sidebar-settings-gear").parentElement;
    expect(container?.className).toContain("flex-wrap");
  });

  it("keeps the same budget and trigger for 8 plugins when collapsed, in a non-wrapping column", () => {
    insightDestinations = eightPluginDestinations();

    renderFooter(true);

    for (let i = 0; i < MAX_INLINE_PLUGIN_FOOTER_ITEMS; i++) {
      expect(screen.getByTestId(`sidebar-plugin:acme-${i}:board-button`)).not.toBeNull();
    }
    expect(screen.getByTestId(OVERFLOW_TRIGGER_TEST_ID)).not.toBeNull();
    const container = screen.getByTestId("sidebar-settings-gear").parentElement;
    expect(container?.className).toContain("flex-col");
  });

  it("always renders stats inline, applying the budget to plugin entries alone", () => {
    insightDestinations = eightPluginDestinations();

    renderFooter();

    expect(screen.getByRole("button", { name: "Stats" })).not.toBeNull();
    for (let i = 0; i < MAX_INLINE_PLUGIN_FOOTER_ITEMS; i++) {
      expect(screen.getByTestId(`sidebar-plugin:acme-${i}:board-button`)).not.toBeNull();
    }
  });

  it("partitions by current registration order, so a re-enabled plugin moves from inline to the overflow menu", () => {
    // Simulates the post-re-enable order: p1 moved to the end of the plugin
    // run (spec's Ordering + Capacity re-enable scenario). The footer only
    // partitions whatever order it is given; producing that order is the
    // registry's job, exercised elsewhere.
    insightDestinations = [
      STATS_DESTINATION,
      { ...pluginDestination(2), id: "plugin:p2:board", label: "P2" },
      { ...pluginDestination(3), id: "plugin:p3:board", label: "P3" },
      { ...pluginDestination(4), id: "plugin:p4:board", label: "P4" },
      { ...pluginDestination(1), id: "plugin:p1:board", label: "P1" },
    ];

    renderFooter();

    expect(screen.getByTestId("sidebar-plugin:p2:board-button")).not.toBeNull();
    expect(screen.getByTestId("sidebar-plugin:p3:board-button")).not.toBeNull();
    expect(screen.getByTestId("sidebar-plugin:p4:board-button")).not.toBeNull();

    // p1's menu item shares its testid with what an inline button would use
    // (spec.md#Rendered-identity); open the menu before asserting presence.
    fireEvent.click(screen.getByTestId(OVERFLOW_TRIGGER_TEST_ID));
    expect(screen.getByTestId("sidebar-plugin:p1:board-button")).not.toBeNull();
  });
});
