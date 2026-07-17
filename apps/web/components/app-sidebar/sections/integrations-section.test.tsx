import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const navigationMock = vi.hoisted(() => ({
  pathname: "/",
}));

const collapsibleMock = vi.hoisted(() => ({
  open: false,
}));

const linksMock = vi.hoisted(() =>
  vi.fn(() => [
    { id: "github", label: "GitHub", href: "/github" },
    { id: "jira", label: "Jira", href: "/jira" },
  ]),
);

const storeState = {
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

vi.mock("@/components/integrations/integrations-menu", () => ({
  useConfiguredIntegrationLinks: linksMock,
}));

const pluginsMock = vi.hoisted(() => ({
  enabled: true,
  navItems: [] as Array<{
    id: string;
    label: string;
    path: string;
    icon?: string;
    section?: string;
  }>,
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => pluginsMock.enabled,
}));

vi.mock("@/lib/plugins/registry", () => ({
  usePluginRegistry: () => ({
    getNavItems: () => pluginsMock.navItems,
  }),
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
    linksMock.mockReturnValue([
      { id: "github", label: "GitHub", href: "/github" },
      { id: "jira", label: "Jira", href: "/jira" },
    ]);
    pluginsMock.enabled = true;
    pluginsMock.navItems = [];
  });

  afterEach(() => cleanup());

  it("keeps integration shortcuts visible while the section accordion is closed", () => {
    linksMock.mockReturnValue([
      { id: "github", label: "GitHub", href: "/github" },
      { id: "gitlab", label: "GitLab", href: "/gitlab" },
      { id: "jira", label: "Jira", href: "/jira" },
      { id: "linear", label: "Linear", href: "/linear" },
      { id: "sentry", label: "Sentry", href: "/sentry" },
    ]);

    renderSection();

    const shortcuts = screen.getAllByTestId("integration-header-shortcut");
    expect(shortcuts.map((shortcut) => shortcut.getAttribute("aria-label"))).toEqual([
      "GitHub",
      "GitLab",
      "Jira",
      "Linear",
    ]);
    expect(shortcuts.map((shortcut) => shortcut.getAttribute("href"))).toEqual([
      "/github",
      "/gitlab",
      "/jira",
      "/linear",
    ]);
    expect(screen.queryByRole("link", { name: "Sentry" })).toBeNull();
  });

  it("limits shortcuts to four integrations and leaves the full list in the expanded section", () => {
    storeState.appSidebar.sectionExpanded.integrations = true;
    linksMock.mockReturnValue([
      { id: "github", label: "GitHub", href: "/github" },
      { id: "gitlab", label: "GitLab", href: "/gitlab" },
      { id: "jira", label: "Jira", href: "/jira" },
      { id: "linear", label: "Linear", href: "/linear" },
      { id: "sentry", label: "Sentry", href: "/sentry" },
    ]);

    renderSection();

    expect(screen.getAllByTestId("integration-header-shortcut")).toHaveLength(4);
    expect(screen.getByRole("link", { name: "Sentry" })).toBeTruthy();
  });

  const costPerModelItem = {
    id: "cost-per-model",
    label: "Cost per Model",
    path: "/cost-per-model",
    icon: "chart",
    section: "integrations",
  };
  const costPerModelTestId = `plugin-nav-item-${costPerModelItem.id}`;

  it("renders plugin nav items registered with section integrations after the first-party links", () => {
    storeState.appSidebar.sectionExpanded.integrations = true;
    pluginsMock.navItems = [
      costPerModelItem,
      { id: "hello", label: "Hello", path: "/hello", section: "main" },
    ];

    renderSection();

    const pluginRow = screen.getByTestId(costPerModelTestId);
    expect(pluginRow.getAttribute("href")).toBe(costPerModelItem.path);
    expect(screen.queryByRole("link", { name: "Hello" })).toBeNull();
  });

  it("shows the section when only plugin integration items exist, with no empty header-action slot", () => {
    storeState.appSidebar.sectionExpanded.integrations = true;
    linksMock.mockReturnValue([]);
    pluginsMock.navItems = [costPerModelItem];

    const { container } = renderSection();

    expect(screen.getByTestId(costPerModelTestId)).toBeTruthy();
    // Regression for the empty headerAction slot: AppSidebarSection renders
    // a "shrink-0 mr-1 flex items-center" wrapper whenever headerAction is
    // non-null, even with zero shortcuts inside it.
    expect(container.querySelector(".shrink-0.mr-1")).toBeNull();
  });

  it("hides plugin items (and an otherwise empty section) when the plugins feature is off", () => {
    storeState.appSidebar.sectionExpanded.integrations = true;
    linksMock.mockReturnValue([]);
    pluginsMock.enabled = false;
    pluginsMock.navItems = [costPerModelItem];

    const { container } = renderSection();

    expect(container.textContent).toBe("");
  });
});
