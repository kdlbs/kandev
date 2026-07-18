import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { APP_SIDEBAR_EXPANDED_WIDTH } from "./app-sidebar-constants";

const navigationMock = vi.hoisted(() => ({
  pathname: "/",
}));
const officeRouteMock = vi.hoisted(() => ({
  inOffice: false,
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
}));

vi.mock("@/hooks/use-workflows", () => ({
  useEnsureWorkspaceWorkflows: () => {},
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: (name: keyof typeof storeState.features) => storeState.features[name],
}));

const storeState = {
  features: {
    office: false,
    plugins: false,
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

import { AppSidebar } from "./app-sidebar";

describe("AppSidebar", () => {
  beforeEach(() => {
    navigationMock.pathname = "/";
    officeRouteMock.inOffice = false;
    storeState.appSidebar.collapsed = false;
    storeState.appSidebar.settingsMode = false;
    storeState.toggleAppSidebar = vi.fn();
    storeState.toggleAppSidebarSection = vi.fn();
    storeState.toggleAppSidebarSettingsMode = vi.fn();
    storeState.setAppSidebarSettingsMode = vi.fn((_settingsMode: boolean) => {});
    footerMock.onLayout = null;
  });

  afterEach(() => {
    cleanup();
  });

  it("renders the expanded nav inside a clipped animation layer", () => {
    render(<AppSidebar />);
    expect(screen.getByTestId("app-sidebar").getAttribute("data-collapsed")).toBe("false");
    expect(screen.getByTestId("tasks-section")).toBeTruthy();
    expect(screen.getByTestId("projects-section")).toBeTruthy();
    expect(screen.getByTestId("agents-section")).toBeTruthy();
    expect(screen.queryByTestId("settings-section")).toBeNull();
    expect(screen.getByTestId("app-sidebar-content").classList).toContain("overflow-hidden");
    expect(screen.getByTestId("app-sidebar").classList).not.toContain("overflow-hidden");
  });

  it("renders office navigation without kanban-only sections in office mode", () => {
    officeRouteMock.inOffice = true;
    navigationMock.pathname = "/office";

    render(<AppSidebar />);

    expect(screen.getByTestId("office-navigation-section-work")).toBeTruthy();
    expect(screen.getByTestId("office-navigation-section-office")).toBeTruthy();
    expect(screen.queryByTestId("tasks-section")).toBeNull();
    expect(screen.queryByTestId("integrations-section")).toBeNull();
  });

  it("orders office navigation sections around entity groups", () => {
    officeRouteMock.inOffice = true;
    navigationMock.pathname = "/office";

    render(<AppSidebar />);

    const nav = screen.getByRole("navigation");
    const expectedSections = [
      "primary-nav",
      "office-navigation-section-work",
      "projects-section",
      "agents-section",
      "office-navigation-section-office",
    ];
    expect(
      Array.from(nav.querySelectorAll("[data-testid]"))
        .map((node) => node.getAttribute("data-testid"))
        .filter((id): id is string => id !== null && expectedSections.includes(id)),
    ).toEqual(expectedSections);
  });

  it("renders collapsed when store reports collapsed=true", () => {
    storeState.appSidebar.collapsed = true;
    render(<AppSidebar />);
    expect(screen.getByTestId("app-sidebar").getAttribute("data-collapsed")).toBe("true");
    expect(screen.getByTestId("tasks-section").getAttribute("data-collapsed")).toBe("true");
  });

  it("invokes toggleAppSidebar when the header collapse button is clicked", () => {
    render(<AppSidebar />);
    fireEvent.click(screen.getByTestId("header-toggle"));
    expect(storeState.toggleAppSidebar).toHaveBeenCalledOnce();
  });

  it("enters settings mode on initial mount for a deep settings route", async () => {
    navigationMock.pathname =
      "/settings/agents/opencode-acp/profiles/1f593628-6752-4972-95ab-5c8c3e7eaeab";

    render(<AppSidebar />);

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

    render(<AppSidebar />);

    await waitFor(() => {
      expect(storeState.toggleAppSidebarSettingsMode).toHaveBeenCalledOnce();
      expect(storeState.setAppSidebarSettingsMode).toHaveBeenCalledOnce();
    });
    expect(storeState.appSidebar.settingsMode).toBe(true);
  });

  it("exits settings mode when navigating from a settings route to a non-settings route", async () => {
    navigationMock.pathname = "/settings/agents";
    storeState.appSidebar.settingsMode = true;

    const { rerender } = render(<AppSidebar />);

    navigationMock.pathname = "/office/tasks";
    rerender(<AppSidebar />);

    await waitFor(() => {
      expect(storeState.setAppSidebarSettingsMode).toHaveBeenCalledWith(false);
    });
    expect(storeState.toggleAppSidebarSettingsMode).not.toHaveBeenCalled();
  });
});
