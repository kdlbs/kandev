import { cleanup, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { IconBrandGithub, IconBrandGitlab, IconChartBar, IconHexagon } from "@tabler/icons-react";
import type { HTMLAttributes, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import type { ResolvedDestination } from "@/lib/navigation/types";

const navigationMock = vi.hoisted(() => ({
  pathname: "/",
  isFinePointer: true,
}));

const collapsibleMock = vi.hoisted(() => ({
  open: false,
}));

const destinationsMock = vi.hoisted(() => vi.fn());

const storeState = {
  userSettings: { sidebarFastActionsEnabled: true },
  workspaces: { activeId: "ws-1" as string | null },
  appSidebar: {
    sectionExpanded: {
      integrations: false,
    },
  },
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
};

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => navigationMock.pathname,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

// The section renders whatever the navigation manifest resolves for the sidebar's
// integrations group; availability gating and plugin merging are covered in
// `lib/navigation/core-destinations.test.ts`.
vi.mock("@/hooks/use-app-destinations", () => ({
  useAppDestinations: destinationsMock,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: navigationMock.isFinePointer }),
}));

vi.mock("@kandev/ui/collapsible", () => ({
  Collapsible: ({ children, open }: { children: ReactNode; open?: boolean }) => {
    collapsibleMock.open = !!open;
    return <div>{children}</div>;
  },
  CollapsibleContent: ({
    children,
    ...props
  }: HTMLAttributes<HTMLDivElement> & { children: ReactNode }) =>
    collapsibleMock.open ? <div {...props}>{children}</div> : null,
}));

import { IntegrationsSection } from "./integrations-section";

function destination(
  id: string,
  label: string,
  icon: ResolvedDestination["icon"],
): ResolvedDestination {
  return { id, label, icon, section: "integrations", href: `/${id}` };
}

/** Plugin entries arrive namespaced, with the raw item id kept for test ids. */
function pluginDestination(
  itemId: string,
  label: string,
  icon: ResolvedDestination["icon"],
): ResolvedDestination {
  return {
    id: `plugin:${itemId}`,
    pluginItemId: itemId,
    label,
    icon,
    section: "integrations",
    href: `/${itemId}`,
    source: "plugin",
  };
}

const AZURE = destination("azure-devops", "Azure DevOps", AzureDevOpsIcon);
const GITHUB = destination("github", "GitHub", IconBrandGithub);
const GITLAB = destination("gitlab", "GitLab", IconBrandGitlab);
const JIRA = destination("jira", "Jira", IconHexagon);
const LINEAR = destination("linear", "Linear", IconHexagon);
const PLUGIN_PAGE = pluginDestination("cost-per-model", "Cost per Model", IconChartBar);
const PLUGIN_TEST_ID = `plugin-nav-item-${PLUGIN_PAGE.pluginItemId}`;

function renderSection() {
  return render(
    <TooltipProvider>
      <IntegrationsSection collapsed={false} />
    </TooltipProvider>,
  );
}

function resetIntegrationState() {
  storeState.userSettings.sidebarFastActionsEnabled = true;
  navigationMock.pathname = "/";
  navigationMock.isFinePointer = true;
  storeState.appSidebar.sectionExpanded.integrations = false;
  storeState.toggleAppSidebarSection.mockClear();
  storeState.setAppSidebarCollapsed.mockClear();
  destinationsMock.mockReturnValue([GITHUB, JIRA]);
}

const INTEGRATIONS_BODY_TEST_ID = "sidebar-section-integrations";

describe("IntegrationsSection", () => {
  beforeEach(resetIntegrationState);

  afterEach(() => cleanup());

  it("keeps destinations behind a disclosure while collapsed", () => {
    renderSection();
    expect(screen.getByRole("button", { name: "Integrations" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(screen.getByTestId("integration-header-shortcut-github")).toBeTruthy();
    expect(screen.queryByTestId(INTEGRATIONS_BODY_TEST_ID)).toBeNull();
    screen.getByRole("button", { name: "Integrations" }).click();
    expect(storeState.toggleAppSidebarSection).toHaveBeenCalledWith("integrations", false);
  });

  it("shows every resolved integration once when expanded", () => {
    storeState.appSidebar.sectionExpanded.integrations = true;
    destinationsMock.mockReturnValue([AZURE, GITHUB, GITLAB, JIRA, LINEAR]);
    renderSection();
    const body = document.getElementById(INTEGRATIONS_BODY_TEST_ID)!;
    for (const destination of [AZURE, GITHUB, GITLAB, JIRA, LINEAR]) {
      expect(within(body).getAllByRole("link", { name: destination.label })).toHaveLength(1);
    }
    expect(within(body).getAllByTestId("azure-devops-icon")).toHaveLength(1);
  });

  it("renders plugin nav items after the first-party links, with their own test id", () => {
    storeState.appSidebar.sectionExpanded.integrations = true;
    destinationsMock.mockReturnValue([GITHUB, PLUGIN_PAGE]);

    renderSection();

    const pluginRow = screen.getByTestId(PLUGIN_TEST_ID);
    expect(pluginRow.getAttribute("href")).toBe(PLUGIN_PAGE.href);
    expect(pluginRow.textContent).toContain(PLUGIN_PAGE.label);
  });

  it("keeps plugin items out of the header shortcut strip", () => {
    destinationsMock.mockReturnValue([PLUGIN_PAGE]);

    const { container } = renderSection();

    // Regression for the empty headerAction slot: AppSidebarSection renders
    // a "shrink-0 mr-1 flex items-center" wrapper whenever headerAction is
    // non-null, even with zero shortcuts inside it.
    expect(screen.queryAllByTestId("integration-header-shortcut")).toEqual([]);
    expect(container.querySelector(".shrink-0.mr-1")).toBeNull();
  });

  it("shows the section when only plugin integration items exist", () => {
    storeState.appSidebar.sectionExpanded.integrations = true;
    destinationsMock.mockReturnValue([PLUGIN_PAGE]);

    renderSection();

    expect(screen.getByTestId(PLUGIN_TEST_ID)).toBeTruthy();
  });

  it("offers integration settings even with no configured providers", () => {
    storeState.appSidebar.sectionExpanded.integrations = true;
    destinationsMock.mockReturnValue([]);
    renderSection();
    expect(screen.getByRole("link", { name: "Integration settings" }).getAttribute("href")).toBe(
      "/settings/workspaces/ws-1/integrations",
    );
    expect(screen.queryByRole("link", { name: "GitHub" })).toBeNull();
  });
});

describe("IntegrationsSection header shortcuts", () => {
  beforeEach(resetIntegrationState);
  afterEach(() => cleanup());

  it("hides optional header icons while retaining integration destinations", () => {
    storeState.userSettings.sidebarFastActionsEnabled = false;
    storeState.appSidebar.sectionExpanded.integrations = true;
    renderSection();
    expect(screen.queryByTestId("integration-header-shortcut-github")).toBeNull();
    const body = document.getElementById(INTEGRATIONS_BODY_TEST_ID)!;
    expect(within(body).getByRole("link", { name: "GitHub" }).getAttribute("href")).toBe(
      GITHUB.href,
    );
  });

  it("shows first-party shortcuts in manifest order while closed and keeps the action independent", () => {
    destinationsMock.mockReturnValue([GITHUB, PLUGIN_PAGE, JIRA, GITLAB]);
    renderSection();

    const shortcuts = Array.from(
      document.querySelectorAll<HTMLElement>('[data-testid^="integration-header-shortcut-"]'),
    );
    expect(shortcuts.map((shortcut) => shortcut.dataset.destinationId)).toEqual([
      "github",
      "jira",
      "gitlab",
    ]);
    expect(shortcuts.every((shortcut) => shortcut.getAttribute("aria-label"))).toBe(true);
    expect(screen.getByRole("button", { name: "Integrations" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(screen.queryByTestId(PLUGIN_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("integration-header-shortcut-plugin:cost-per-model")).toBeNull();

    shortcuts[0].click();

    expect(storeState.toggleAppSidebarSection).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Integrations" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
  });

  it.each([
    { finePointer: true, expectedIds: ["azure-devops", "github", "gitlab", "jira"] },
    { finePointer: false, expectedIds: ["azure-devops", "github"] },
  ])(
    "limits header shortcuts by pointer capacity: $finePointer",
    ({ finePointer, expectedIds }) => {
      navigationMock.isFinePointer = finePointer;
      storeState.appSidebar.sectionExpanded.integrations = true;
      destinationsMock.mockReturnValue([AZURE, GITHUB, GITLAB, JIRA, LINEAR, PLUGIN_PAGE]);
      renderSection();

      const shortcuts = Array.from(
        document.querySelectorAll<HTMLElement>('[data-testid^="integration-header-shortcut-"]'),
      );
      expect(shortcuts.map((shortcut) => shortcut.dataset.destinationId)).toEqual(expectedIds);
      const body = document.getElementById(INTEGRATIONS_BODY_TEST_ID)!;
      expect(within(body).getByRole("link", { name: "Linear" })).toBeTruthy();
      expect(within(body).getByTestId(PLUGIN_TEST_ID)).toBeTruthy();
      if (!finePointer) {
        for (const shortcut of shortcuts) {
          expect(shortcut.className).toContain("[@media(pointer:coarse)]:size-11");
        }
      }
    },
  );

  it("marks the active integration destination on nested routes", () => {
    navigationMock.pathname = "/github/issues";
    storeState.appSidebar.sectionExpanded.integrations = true;
    renderSection();
    expect(
      screen.getByTestId("integration-header-shortcut-github").getAttribute("aria-current"),
    ).toBe("page");
  });
});
