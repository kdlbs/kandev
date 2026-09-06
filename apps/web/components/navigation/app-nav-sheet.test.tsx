import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { JSX } from "react";
import { AppNavSheet } from "./app-nav-sheet";
import { AppNavSections, useAppNavDialogs } from "./app-nav-sections";

const mocks = vi.hoisted(() => ({
  routerPush: vi.fn(),
  openStatusDrawer: vi.fn(),
  openHealthDialog: vi.fn(),
  setTheme: vi.fn(),
}));

let resolvedTheme: "light" | "dark" = "light";
const THEME_TOGGLE_TEST_ID = "mobile-theme-toggle-button";
const ARIA_LABEL = "aria-label";

const state = {
  features: { canvases: false },
  workspaces: { activeId: "ws-1" as string | null },
};

let healthHasIssues = false;
let statusSeverity: "none" | "unstable" | "lost" = "none";
let workspaceActionsRegistrations: Array<{
  registrationId: string;
  pluginId: string;
  Component: (props: { slotProps?: unknown }) => JSX.Element;
}> = [];

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.routerPush }),
  usePathname: () => "/settings",
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/use-in-office", () => ({
  useInOffice: () => false,
}));

type NavRegistration = {
  pluginId: string;
  id: string;
  label: string;
  path: string;
  section?: string;
};

// Mutable so a specific test can inject plugin registrations (e.g. an
// `insights`-section item) without affecting the rest of the suite, which
// exercises the default no-plugins case.
let navRegistrations: NavRegistration[] = [];

vi.mock("@/lib/plugins/registry", () => ({
  usePluginRegistry: () => ({
    getNavRegistrations: () => navRegistrations,
    getSlotRegistrations: (name: string) =>
      name === "sidebar-workspace-actions" ? workspaceActionsRegistrations : [],
  }),
}));

vi.mock("@/hooks/use-nav-availability", () => ({
  useNavAvailability: () => ({
    "azure-devops": false,
    github: false,
    gitlab: false,
    jira: false,
    linear: false,
  }),
}));

vi.mock("@/components/app-status-bar/app-status-surface-provider", () => ({
  useAppStatusDrawer: () => ({
    enabled: true,
    issueSeverity: statusSeverity,
    openStatusDrawer: mocks.openStatusDrawer,
  }),
}));

vi.mock("@/hooks/use-system-health-indicator", () => ({
  useSystemHealthIndicator: () => ({
    hasIssues: healthHasIssues,
    issues: [],
    dialogOpen: false,
    openDialog: mocks.openHealthDialog,
    closeDialog: vi.fn(),
  }),
}));

vi.mock("@/components/improve-kandev-dialog", () => ({
  ImproveKandevDialog: ({ open }: { open: boolean }) => (
    <div data-testid="improve-dialog" data-open={open} />
  ),
}));

vi.mock("@/components/system-health/health-indicator", () => ({
  HealthIssuesDialog: () => <div data-testid="health-dialog" />,
}));

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({ resolvedTheme, setTheme: mocks.setTheme }),
}));

type SectionsProps = Parameters<typeof AppNavSections>[0];

function SectionsHost({
  omitSections,
  omitDestinations,
}: {
  omitSections?: SectionsProps["omitSections"];
  omitDestinations?: SectionsProps["omitDestinations"];
}) {
  const controls = useAppNavDialogs(() => {});
  return (
    <>
      <AppNavSections
        onNavigate={() => {}}
        omitSections={omitSections}
        omitDestinations={omitDestinations}
        controls={controls}
      />
      {controls.dialogs}
    </>
  );
}

describe("AppNavSheet", () => {
  beforeEach(() => {
    healthHasIssues = false;
    resolvedTheme = "light";
    navRegistrations = [];
    workspaceActionsRegistrations = [];
    vi.clearAllMocks();
  });
  afterEach(cleanup);

  it("opens from the trigger and offers the manifest destinations plus pageNav", () => {
    render(<AppNavSheet pageNav={<span data-testid="page-nav" />} />);

    fireEvent.click(screen.getByTestId("app-nav-trigger"));

    const sheet = screen.getByTestId("app-nav-sheet");
    expect(sheet).not.toBeNull();
    expect(screen.getByTestId("page-nav")).not.toBeNull();
    for (const label of ["Home", "Tasks", "Stats", "Settings"]) {
      expect(screen.getByRole("link", { name: label })).not.toBeNull();
    }
    // pageNav renders above the global sections.
    const nav = sheet.querySelector("nav");
    const pageNavIndex = [...(nav?.children ?? [])].findIndex((el) =>
      el.matches('[data-testid="page-nav"]'),
    );
    expect(pageNavIndex).toBe(0);
  });

  it("routes the Home row through the manifest href", () => {
    render(<AppNavSheet />);

    fireEvent.click(screen.getByTestId("app-nav-trigger"));

    expect(screen.getByRole("link", { name: "Home" }).getAttribute("href")).toBe(
      "/?home=overview&workspaceId=ws-1",
    );
  });

  it("exposes workspace actions through the shared phone navigation sheet", () => {
    let captured: unknown;
    workspaceActionsRegistrations = [
      {
        registrationId: "workspace-action",
        pluginId: "plugin-1",
        Component: ({ slotProps }) => {
          captured = slotProps;
          return <button type="button" data-testid="mobile-workspace-action" />;
        },
      },
    ];

    render(<AppNavSheet />);
    fireEvent.click(screen.getByTestId("app-nav-trigger"));

    expect(screen.getByTestId("mobile-workspace-action")).not.toBeNull();
    expect(captured).toEqual({ workspaceId: "ws-1", presentation: "mobile" });
  });
});

describe("AppNavSections", () => {
  beforeEach(() => {
    healthHasIssues = false;
    statusSeverity = "none";
    navRegistrations = [];
    workspaceActionsRegistrations = [];
    vi.clearAllMocks();
  });
  afterEach(cleanup);

  // Office renders its own Tasks row (pointing inside Office) above these
  // sections, so the manifest's kanban-bound Tasks has to go without taking
  // Home — the only other primary destination — with it.
  it("drops a single named destination and keeps the rest of its section", () => {
    render(<SectionsHost omitDestinations={["tasks"]} />);

    expect(screen.queryByRole("link", { name: "Tasks" })).toBeNull();
    expect(screen.getByRole("link", { name: "Home" })).not.toBeNull();
    expect(screen.getByTestId("app-nav-primary")).not.toBeNull();
  });

  it.each([
    ["unstable", "Connection unstable. Reconnecting to Kandev."],
    ["lost", "Connection lost for at least 10 seconds. Live updates may be stale."],
  ] as const)("keeps Status in the button's accessible name while %s", (severity, description) => {
    statusSeverity = severity;
    render(<SectionsHost />);

    // A bare aria-label of the connection sentence would replace the visible
    // "Status" and break label-in-name for voice control.
    const status = screen.getByTestId("mobile-home-status-button");
    const name = status.getAttribute(ARIA_LABEL) ?? "";
    expect(name).toContain("Status");
    expect(name).toContain(description);
    expect(screen.getByRole("button", { name: /Status/ })).toBe(status);
  });

  it("leaves the Status button's name as the visible text while healthy", () => {
    render(<SectionsHost />);

    const status = screen.getByTestId("mobile-home-status-button");
    expect(status.getAttribute(ARIA_LABEL)).toBeNull();
    expect(status.textContent).toContain("Status");
  });

  it("drops the primary section when the caller omits it", () => {
    render(<SectionsHost omitSections={["primary"]} />);

    expect(screen.queryByTestId("app-nav-primary")).toBeNull();
    // The utility tail stays.
    expect(screen.getByTestId("mobile-improve-kandev-button")).not.toBeNull();
  });

  it("hides the health row while the system is healthy", () => {
    render(<SectionsHost />);

    expect(screen.queryByTestId("app-nav-health-button")).toBeNull();
  });

  it("shows the health row when issues exist", () => {
    healthHasIssues = true;
    render(<SectionsHost />);

    expect(screen.getByTestId("app-nav-health-button")).not.toBeNull();
  });

  it("orders the Utilities group's manifest rows as Stats, Settings, then a plugin sidebar-footer item", () => {
    navRegistrations = [
      {
        pluginId: "acme",
        id: "board",
        label: "Acme Board",
        path: "/plugins/acme",
        section: "sidebar-footer",
      },
    ];

    render(<SectionsHost />);

    const links = screen.getAllByRole("link").map((link) => link.textContent);
    const stats = links.indexOf("Stats");
    const settings = links.indexOf("Settings");
    const plugin = links.indexOf("Acme Board");

    expect(stats).toBeGreaterThanOrEqual(0);
    expect(settings).toBeGreaterThan(stats);
    expect(plugin).toBeGreaterThan(settings);
  });

  // The phone Utilities group is deliberately uncapped (spec.md#Capacity-and-
  // overflow: "the phone surface is uncapped"), unlike the desktop footer's
  // MAX_INLINE_PLUGIN_FOOTER_ITEMS budget — well over that budget (8, per
  // spec.md's own "well over the budget" scenario) must still render every
  // item as a row, with no overflow menu of its own.
  it("renders every plugin sidebar-footer item as a row, uncapped, with no overflow menu", () => {
    navRegistrations = Array.from({ length: 8 }, (_, i) => ({
      pluginId: "acme",
      id: `board-${i}`,
      label: `Acme Board ${i}`,
      path: `/plugins/acme-${i}`,
      section: "sidebar-footer",
    }));

    render(<SectionsHost />);

    for (let i = 0; i < 8; i++) {
      expect(screen.getByRole("link", { name: `Acme Board ${i}` })).not.toBeNull();
    }
    expect(screen.queryByTestId("sidebar-plugin-overflow-button")).toBeNull();
  });
});

describe("AppNavSections theme toggle", () => {
  beforeEach(() => {
    healthHasIssues = false;
    resolvedTheme = "light";
    navRegistrations = [];
    workspaceActionsRegistrations = [];
    vi.clearAllMocks();
  });
  afterEach(cleanup);

  // #2514 shipped this on the kanban drawer's own utility rows. That surface
  // draws the shared block now, so the row has to live here or the feature
  // disappears from every mobile menu.
  it("switches to dark from a light theme", () => {
    render(<SectionsHost />);

    const toggle = screen.getByTestId(THEME_TOGGLE_TEST_ID);
    expect(toggle.getAttribute(ARIA_LABEL)).toBe("Switch to Dark Mode");
    expect(toggle.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(toggle);
    expect(mocks.setTheme).toHaveBeenCalledWith("dark");
  });

  it("switches back to light from a dark theme", () => {
    resolvedTheme = "dark";
    render(<SectionsHost />);

    const toggle = screen.getByTestId(THEME_TOGGLE_TEST_ID);
    expect(toggle.getAttribute(ARIA_LABEL)).toBe("Switch to Light Mode");
    expect(toggle.getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(toggle);
    expect(mocks.setTheme).toHaveBeenCalledWith("light");
  });
});
