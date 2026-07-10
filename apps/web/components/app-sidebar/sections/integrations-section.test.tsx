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
});
