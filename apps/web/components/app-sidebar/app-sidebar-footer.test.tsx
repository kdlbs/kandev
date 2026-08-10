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
};

let officeEnabled = false;
let appStatusBarEnabled = true;
const DEFAULT_PATHNAME = "/tasks/session-1";
let pathname = DEFAULT_PATHNAME;

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.routerPush }),
  usePathname: () => pathname,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: (name: string) => (name === "office" ? officeEnabled : appStatusBarEnabled),
}));

// The footer renders its insight buttons from the navigation manifest; the
// manifest itself is covered in `lib/navigation/core-destinations.test.ts`.
vi.mock("@/hooks/use-app-destinations", () => ({
  useStaticDestinations: () => [
    {
      id: "stats",
      label: "Stats",
      icon: IconChartBar,
      section: "insights",
      href: "/stats",
    },
  ],
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

import { AppSidebarFooter } from "./app-sidebar-footer";

function renderFooter() {
  return render(
    <TooltipProvider>
      <AppSidebarFooter collapsed={false} onToggleSettingsMode={mocks.toggleSettingsMode} />
    </TooltipProvider>,
  );
}

describe("AppSidebarFooter", () => {
  beforeEach(() => {
    officeEnabled = false;
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
    appStatusBarEnabled = true;
    window.localStorage.clear();
    document.cookie = "office-active-workspace=; path=/; max-age=0";
    mocks.routerPush.mockClear();
    mocks.toggleSettingsMode.mockClear();
  });

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

    expect(mocks.routerPush).toHaveBeenCalledWith("/?home=overview&workspaceId=kanban-1");
  });

  it("remembers the current office workspace when toggling back to kanban", () => {
    officeEnabled = true;
    state.workspaces.activeId = "office-2";
    window.localStorage.setItem("kandev.lastKanbanWorkspaceId", "kanban-1");

    renderFooter();

    fireEvent.click(screen.getByRole("button", { name: "Kanban" }));

    expect(document.cookie).toContain("office-active-workspace=office-2");
    expect(mocks.routerPush).toHaveBeenCalledWith("/?home=overview&workspaceId=kanban-1");
  });
  it("navigates to /settings when the gear opens settings mode from a non-settings route", () => {
    pathname = DEFAULT_PATHNAME;
    state.appSidebar.settingsMode = false;

    renderFooter();
    fireEvent.click(screen.getByTestId("sidebar-settings-gear"));

    expect(mocks.routerPush).toHaveBeenCalledWith("/settings");
    expect(mocks.toggleSettingsMode).toHaveBeenCalledOnce();
  });

  it("does not navigate when the gear reopens settings mode while already on a settings route", () => {
    pathname = "/settings/agents";
    state.appSidebar.settingsMode = false;

    renderFooter();
    fireEvent.click(screen.getByTestId("sidebar-settings-gear"));

    expect(mocks.routerPush).not.toHaveBeenCalled();
    expect(mocks.toggleSettingsMode).toHaveBeenCalledOnce();
  });

  it("does not navigate when the gear closes an already-open settings mode", () => {
    pathname = "/settings";
    state.appSidebar.settingsMode = true;

    renderFooter();
    fireEvent.click(screen.getByTestId("sidebar-settings-gear"));

    expect(mocks.routerPush).not.toHaveBeenCalled();
    expect(mocks.toggleSettingsMode).toHaveBeenCalledOnce();
  });
});

describe("AppSidebarFooter connection fallback", () => {
  beforeEach(() => {
    appStatusBarEnabled = true;
    state.connection.issueSeverity = "none";
  });

  afterEach(cleanup);

  it("shows the connection fallback immediately after the theme control during an outage", () => {
    appStatusBarEnabled = false;
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
