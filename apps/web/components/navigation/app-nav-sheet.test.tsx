import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppNavSheet } from "./app-nav-sheet";
import { AppNavSections, useAppNavDialogs } from "./app-nav-sections";

const mocks = vi.hoisted(() => ({
  routerPush: vi.fn(),
  openStatusDrawer: vi.fn(),
  openHealthDialog: vi.fn(),
}));

const state = {
  workspaces: { activeId: "ws-1" as string | null },
};

let healthHasIssues = false;

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

vi.mock("@/lib/plugins/registry", () => ({
  usePluginRegistry: () => ({ getNavRegistrations: () => [] }),
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
    issueSeverity: "none",
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

function SectionsHost({ omitSections }: { omitSections?: Parameters<
    typeof AppNavSections
  >[0]["omitSections"] }) {
  const controls = useAppNavDialogs(() => {});
  return (
    <>
      <AppNavSections onNavigate={() => {}} omitSections={omitSections} controls={controls} />
      {controls.dialogs}
    </>
  );
}

describe("AppNavSheet", () => {
  beforeEach(() => {
    healthHasIssues = false;
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
});

describe("AppNavSections", () => {
  beforeEach(() => {
    healthHasIssues = false;
    vi.clearAllMocks();
  });
  afterEach(cleanup);

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
});
