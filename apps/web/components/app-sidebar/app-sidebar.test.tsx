import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { APP_SIDEBAR_EXPANDED_WIDTH } from "./app-sidebar-constants";

const navigationMock = vi.hoisted(() => ({
  pathname: "/",
}));
const officeRouteMock = vi.hoisted(() => ({
  inOffice: false,
  mode: null as "office" | "kanban" | "unknown" | null,
}));
const footerMock = vi.hoisted(() => ({
  onLayout: null as (() => void) | null,
}));

// The AppSidebar pulls in a lot of children that touch the dockview / kanban
// data layer. For unit testing the collapse + section toggle behaviour we stub
// the children to keep the test focused on the shell.
vi.mock("./app-sidebar-header", () => ({
  AppSidebarHeader: ({
    collapsed,
    onToggleCollapse,
  }: {
    collapsed: boolean;
    onToggleCollapse: () => void;
  }) => (
    <button
      type="button"
      onClick={onToggleCollapse}
      data-testid="header-toggle"
      data-collapsed={collapsed ? "true" : "false"}
    >
      header
    </button>
  ),
}));

vi.mock("./app-sidebar-primary-nav", () => ({
  AppSidebarPrimaryNav: () => <div data-testid="primary-nav" />,
}));

vi.mock("./sections/tasks-section", () => ({
  TasksSection: ({ collapsed }: { collapsed: boolean }) => (
    <div data-testid="tasks-section" data-collapsed={collapsed ? "true" : "false"}>
      tasks
    </div>
  ),
}));
vi.mock("./sections/canvases-section", () => ({
  CanvasesSection: ({ collapsed }: { collapsed: boolean }) => (
    <div data-testid="canvases-section" data-collapsed={collapsed ? "true" : "false"} />
  ),
}));
vi.mock("./sections/projects-section", () => ({
  ProjectsSection: () => <div data-testid="projects-section" />,
}));
vi.mock("./sections/agents-section", () => ({
  AgentsSection: () => <div data-testid="agents-section" />,
}));
vi.mock("./sections/integrations-section", () => ({
  IntegrationsSection: () => <div data-testid="integrations-section" />,
}));
vi.mock("./sections/office-navigation-section", () => ({
  OfficeNavigationSection: ({ section }: { section?: "all" | "work" | "office" }) => (
    <div data-testid={`office-navigation-section-${section ?? "all"}`} />
  ),
}));
vi.mock("./app-sidebar-footer", async () => {
  const { useLayoutEffect } = await vi.importActual<typeof import("react")>("react");

  return {
    AppSidebarFooter: () => {
      useLayoutEffect(() => footerMock.onLayout?.(), []);
      return <div data-testid="footer" />;
    },
  };
});
vi.mock("./app-sidebar-settings-mode", () => ({
  AppSidebarSettingsMode: () => <div data-testid="settings-mode" />,
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => navigationMock.pathname,
}));

vi.mock("@/hooks/use-in-office", () => ({
  useInOffice: () => officeRouteMock.inOffice,
  // Mode is derived from the active workspace now, not the route. These specs
  // cover section composition per mode, so they pin a resolved mode directly;
  // the unresolved case has its own spec below.
  useOfficeModeState: () =>
    officeRouteMock.mode ?? (officeRouteMock.inOffice ? "office" : "kanban"),
}));

vi.mock("@/hooks/use-workflows", () => ({
  useEnsureWorkspaceWorkflows: () => {},
}));

// Same reason as the workflows hook above: this suite covers the sidebar's
// layout and section composition, not the workspace-scoped data loads it
// hoists. `use-office-workspace-data.test.ts` covers those.
vi.mock("@/hooks/use-office-workspace-data", () => ({
  useOfficeWorkspaceData: () => {},
}));

const storeState = {
  features: {
    office: false,
    canvases: false,
  },
  workspaces: {
    activeId: undefined as string | undefined,
  },
  appSidebar: {
    collapsed: false,
    sectionExpanded: {
      tasks: true,
      "office-work": true,
      "office-workspace": true,
      projects: false,
      agents: false,
      integrations: false,
      canvases: false,
      automations: false,
      settings: false,
    },
    width: APP_SIDEBAR_EXPANDED_WIDTH,
    settingsMode: false,
  },
  toggleAppSidebar: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarWidth: vi.fn(),
  toggleAppSidebarSettingsMode: vi.fn(),
  setAppSidebarSettingsMode: vi.fn((_settingsMode: boolean) => {}),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

// The Automations section fetches its own list; this suite is about sidebar
// composition, so it only needs the calls to resolve.
vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomations: vi.fn().mockResolvedValue([]),
  listAutomationSummaries: vi.fn().mockResolvedValue([]),
}));

import { AppSidebar } from "./app-sidebar";

// The app mounts the sidebar inside the root layout's TooltipProvider
// (app/layout.tsx), and sidebar sections use tooltips for their rail buttons
// and header shortcuts. Rendering bare would test it outside its real context.
function sidebar() {
  return (
    <TooltipProvider>
      <AppSidebar />
    </TooltipProvider>
  );
}

function renderSidebar() {
  return render(sidebar());
}

const TASKS_SECTION = "tasks-section";
const INTEGRATIONS_SECTION = "integrations-section";
const OFFICE_WORK_SECTION = "office-navigation-section-work";
const OFFICE_OFFICE_SECTION = "office-navigation-section-office";

function resetSidebarState() {
  navigationMock.pathname = "/";
  officeRouteMock.inOffice = false;
  officeRouteMock.mode = null;
  storeState.appSidebar.collapsed = false;
  storeState.appSidebar.settingsMode = false;
  storeState.appSidebar.sectionExpanded.canvases = false;
  storeState.features.canvases = false;
  storeState.toggleAppSidebar = vi.fn();
  storeState.toggleAppSidebarSection = vi.fn();
  storeState.toggleAppSidebarSettingsMode = vi.fn();
  storeState.setAppSidebarSettingsMode = vi.fn((_settingsMode: boolean) => {});
  footerMock.onLayout = null;
}

describe("AppSidebar", () => {
  beforeEach(resetSidebarState);

  afterEach(() => {
    cleanup();
  });

  it("renders the expanded nav inside a clipped animation layer", () => {
    renderSidebar();
    expect(screen.getByTestId("app-sidebar").getAttribute("data-collapsed")).toBe("false");
    expect(screen.getByTestId(TASKS_SECTION)).toBeTruthy();
    expect(screen.getByTestId("projects-section")).toBeTruthy();
    expect(screen.getByTestId("agents-section")).toBeTruthy();
    expect(screen.queryByTestId("settings-section")).toBeNull();
    expect(screen.getByTestId("app-sidebar-content").classList).toContain("overflow-hidden");
    expect(screen.getByTestId("app-sidebar").classList).not.toContain("overflow-hidden");
  });

  it("renders office navigation without kanban-only sections in office mode", () => {
    officeRouteMock.inOffice = true;
    navigationMock.pathname = "/office";

    renderSidebar();

    expect(screen.getByTestId(OFFICE_WORK_SECTION)).toBeTruthy();
    expect(screen.getByTestId(OFFICE_OFFICE_SECTION)).toBeTruthy();
    expect(screen.queryByTestId(TASKS_SECTION)).toBeNull();
    expect(screen.queryByTestId(INTEGRATIONS_SECTION)).toBeNull();
  });

  it("keeps office navigation on shared routes when an office workspace is active", () => {
    officeRouteMock.inOffice = true;
    navigationMock.pathname = "/stats";

    renderSidebar();

    expect(screen.getByTestId(OFFICE_WORK_SECTION)).toBeTruthy();
    expect(screen.getByTestId(OFFICE_OFFICE_SECTION)).toBeTruthy();
    expect(screen.queryByTestId(TASKS_SECTION)).toBeNull();
    expect(screen.queryByTestId(INTEGRATIONS_SECTION)).toBeNull();
  });

  it("orders office navigation sections around entity groups", () => {
    officeRouteMock.inOffice = true;
    navigationMock.pathname = "/office";

    renderSidebar();

    const nav = screen.getByRole("navigation");
    const expectedSections = [
      "primary-nav",
      OFFICE_WORK_SECTION,
      "projects-section",
      "agents-section",
      OFFICE_OFFICE_SECTION,
    ];
    expect(
      Array.from(nav.querySelectorAll("[data-testid]"))
        .map((node) => node.getAttribute("data-testid"))
        .filter((id): id is string => id !== null && expectedSections.includes(id)),
    ).toEqual(expectedSections);
  });

  it("renders collapsed when store reports collapsed=true", () => {
    storeState.appSidebar.collapsed = true;
    renderSidebar();
    expect(screen.getByTestId("app-sidebar").getAttribute("data-collapsed")).toBe("true");
    expect(screen.getByTestId(TASKS_SECTION).getAttribute("data-collapsed")).toBe("true");
  });

  it("invokes toggleAppSidebar when the header collapse button is clicked", () => {
    renderSidebar();
    fireEvent.click(screen.getByTestId("header-toggle"));
    expect(storeState.toggleAppSidebar).toHaveBeenCalledOnce();
  });

  it("enters settings mode on initial mount for a deep settings route", async () => {
    navigationMock.pathname =
      "/settings/agents/opencode-acp/profiles/1f593628-6752-4972-95ab-5c8c3e7eaeab";

    renderSidebar();

    await waitFor(() => {
      expect(storeState.setAppSidebarSettingsMode).toHaveBeenCalledWith(true);
    });
    expect(storeState.toggleAppSidebarSettingsMode).not.toHaveBeenCalled();
  });

  it("keeps settings open when the gear is clicked before route synchronization", async () => {
    navigationMock.pathname = "/settings";
    storeState.toggleAppSidebarSettingsMode = vi.fn(() => {
      storeState.appSidebar.settingsMode = !storeState.appSidebar.settingsMode;
    });
    storeState.setAppSidebarSettingsMode = vi.fn((settingsMode: boolean) => {
      storeState.appSidebar.settingsMode = settingsMode;
    });
    footerMock.onLayout = () => storeState.toggleAppSidebarSettingsMode();

    renderSidebar();

    await waitFor(() => {
      expect(storeState.toggleAppSidebarSettingsMode).toHaveBeenCalledOnce();
      expect(storeState.setAppSidebarSettingsMode).toHaveBeenCalledOnce();
    });
    expect(storeState.appSidebar.settingsMode).toBe(true);
  });

  it("exits settings mode when navigating from a settings route to a non-settings route", async () => {
    navigationMock.pathname = "/settings/agents";
    storeState.appSidebar.settingsMode = true;

    const { rerender } = renderSidebar();

    navigationMock.pathname = "/office/tasks";
    rerender(sidebar());

    await waitFor(() => {
      expect(storeState.setAppSidebarSettingsMode).toHaveBeenCalledWith(false);
    });
    expect(storeState.toggleAppSidebarSettingsMode).not.toHaveBeenCalled();
  });
});

describe("AppSidebar canvas routes", () => {
  beforeEach(resetSidebarState);

  afterEach(() => {
    cleanup();
  });

  it("does not force-expand the Canvases section for a direct canvas route", async () => {
    navigationMock.pathname = "/canvases/canvas-1";
    storeState.features.canvases = true;

    renderSidebar();

    await waitFor(() => expect(screen.getByTestId("canvases-section")).toBeTruthy());
    expect(storeState.toggleAppSidebarSection).not.toHaveBeenCalledWith("canvases");
    expect(storeState.appSidebar.sectionExpanded.canvases).toBe(false);
  });
});

describe("AppSidebar before the workspace resolves", () => {
  beforeEach(() => {
    navigationMock.pathname = "/";
    officeRouteMock.inOffice = false;
    officeRouteMock.mode = "unknown";
    storeState.appSidebar.collapsed = false;
    storeState.appSidebar.settingsMode = false;
  });

  afterEach(() => {
    officeRouteMock.mode = null;
    cleanup();
  });

  it("renders only mode-independent rows instead of guessing kanban", () => {
    // Mode comes from the active workspace. On the first frame of a boot that
    // ships no workspace list it is genuinely unknown, and painting kanban
    // sections there means visibly swapping them for Office ones a tick later.
    renderSidebar();

    expect(screen.getByTestId("primary-nav")).toBeTruthy();
    expect(screen.queryByTestId(TASKS_SECTION)).toBeNull();
    expect(screen.queryByTestId(INTEGRATIONS_SECTION)).toBeNull();
    expect(screen.queryByTestId(OFFICE_WORK_SECTION)).toBeNull();
  });
});
