import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { IconBrandGithub, IconBrandGitlab, IconChartBar, IconHexagon } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import type { ResolvedDestination } from "@/lib/navigation/types";

const navigationMock = vi.hoisted(() => ({
  pathname: "/",
}));

const collapsibleMock = vi.hoisted(() => ({
  open: false,
}));

const destinationsMock = vi.hoisted(() => vi.fn());

const storeState = {
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

vi.mock("@kandev/ui/collapsible", () => ({
  Collapsible: ({ children, open }: { children: ReactNode; open?: boolean }) => {
    collapsibleMock.open = !!open;
    return <div>{children}</div>;
  },
  CollapsibleContent: ({ children }: { children: ReactNode }) =>
    collapsibleMock.open ? <div>{children}</div> : null,
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

describe("IntegrationsSection", () => {
  beforeEach(() => {
    navigationMock.pathname = "/";
    storeState.appSidebar.sectionExpanded.integrations = false;
    storeState.toggleAppSidebarSection.mockClear();
    storeState.setAppSidebarCollapsed.mockClear();
    destinationsMock.mockReturnValue([GITHUB, JIRA]);
  });

  afterEach(() => cleanup());

  it("keeps destinations behind a disclosure while collapsed", () => {
    renderSection();
    expect(screen.getByRole("button", { name: "Integrations" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(screen.queryByRole("link", { name: "GitHub" })).toBeNull();
    screen.getByRole("button", { name: "Integrations" }).click();
    expect(storeState.toggleAppSidebarSection).toHaveBeenCalledWith("integrations", false);
  });

  it("shows every resolved integration once when expanded", () => {
    storeState.appSidebar.sectionExpanded.integrations = true;
    destinationsMock.mockReturnValue([AZURE, GITHUB, GITLAB, JIRA, LINEAR]);
    renderSection();
    for (const destination of [AZURE, GITHUB, GITLAB, JIRA, LINEAR]) {
      expect(screen.getAllByRole("link", { name: destination.label })).toHaveLength(1);
    }
    expect(screen.getAllByTestId("azure-devops-icon")).toHaveLength(1);
  });

  it("marks the active integration destination on nested routes", () => {
    navigationMock.pathname = "/github/issues";
    storeState.appSidebar.sectionExpanded.integrations = true;
    renderSection();
    expect(screen.getByRole("link", { name: "GitHub" }).getAttribute("aria-current")).toBe("page");
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
