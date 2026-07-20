import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  getAvailableIntegrationLinks,
  getGitHubIntegrationStatus,
  IntegrationsMenu,
  IntegrationsTopbarLinks,
  MobileIntegrationsSection,
} from "./integrations-menu";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { NavItem } from "@/lib/plugins/types";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { GitHubStatus } from "@/lib/types/github";
import type { Workspace } from "@/lib/types/http";
import { TooltipProvider } from "@kandev/ui/tooltip";

const useGitHubStatusMock = vi.hoisted(() => vi.fn());
const useGitLabAvailableMock = vi.hoisted(() => vi.fn());
const useJiraAvailableMock = vi.hoisted(() => vi.fn());
const useLinearAvailableMock = vi.hoisted(() => vi.fn());
const useFeatureMock = vi.hoisted(() => vi.fn());
const activeWorkspaceRef = vi.hoisted(() => ({
  id: null as string | null,
  items: [] as Array<{ id: string }>,
}));

vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: useGitHubStatusMock,
}));

vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useGitLabAvailable: useGitLabAvailableMock,
}));

vi.mock("@/hooks/domains/jira/use-jira-availability", () => ({
  useJiraAvailable: useJiraAvailableMock,
}));

vi.mock("@/hooks/domains/linear/use-linear-availability", () => ({
  useLinearAvailable: useLinearAvailableMock,
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: useFeatureMock,
}));

vi.mock("@/hooks/domains/workspace/use-workspaces", () => ({
  useWorkspaces: () => ({
    items: activeWorkspaceRef.items,
    activeId: activeWorkspaceRef.id,
    activeWorkspace:
      activeWorkspaceRef.items.find((workspace) => workspace.id === activeWorkspaceRef.id) ?? null,
  }),
}));

function status(overrides: Partial<GitHubStatus>): GitHubStatus {
  return {
    authenticated: false,
    username: "",
    auth_method: "none",
    token_configured: false,
    required_scopes: [],
    ...overrides,
  };
}

function mockAvailability({
  githubReady,
  gitlabReady = false,
  jiraAvailable,
  linearAvailable,
}: {
  githubReady: boolean;
  gitlabReady?: boolean;
  jiraAvailable: boolean;
  linearAvailable: boolean;
}) {
  useGitHubStatusMock.mockReturnValue({
    status: githubReady ? status({ token_configured: true }) : status({}),
    loading: false,
  });
  useGitLabAvailableMock.mockReturnValue(gitlabReady);
  useJiraAvailableMock.mockReturnValue(jiraAvailable);
  useLinearAvailableMock.mockReturnValue(linearAvailable);
}

function workspace(id: string): Workspace {
  return {
    id,
    name: id,
    owner_id: "user-1",
    created_at: "2026-01-01T00:00:00.000Z",
    updated_at: "2026-01-01T00:00:00.000Z",
  } as Workspace;
}

function renderWithQuery(component: Parameters<typeof render>[0]) {
  const queryClient = makeQueryClient();
  queryClient.setQueryDefaults(qk.workspaces.all(), { staleTime: Infinity });
  queryClient.setQueryData(
    qk.workspaces.all(),
    activeWorkspaceRef.items.map((item) => workspace(item.id)),
  );
  return render(createElement(QueryClientProvider, { client: queryClient }, component));
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  activeWorkspaceRef.id = null;
  activeWorkspaceRef.items = [];
});

describe("getGitHubIntegrationStatus", () => {
  it("shows checking while GitHub status is loading and not configured", () => {
    expect(getGitHubIntegrationStatus(status({}), true)).toEqual({
      ready: false,
      label: "Checking",
    });
  });

  it("treats a configured token as ready even before live auth is green", () => {
    expect(getGitHubIntegrationStatus(status({ token_configured: true }), false)).toEqual({
      ready: true,
      label: "Configured",
    });
  });

  it("uses the GitHub page for authenticated status", () => {
    expect(getGitHubIntegrationStatus(status({ authenticated: true }), false)).toEqual({
      ready: true,
      label: "Connected",
    });
  });

  it("shows setup only when no auth or token is configured", () => {
    expect(getGitHubIntegrationStatus(status({}), false)).toEqual({
      ready: false,
      label: "Setup",
    });
  });
});

describe("getAvailableIntegrationLinks", () => {
  it("returns only configured integration destinations", () => {
    expect(
      getAvailableIntegrationLinks({
        githubReady: true,
        gitlabReady: false,
        jiraAvailable: false,
        linearAvailable: true,
      }),
    ).toEqual([
      { id: "github", label: "GitHub", href: "/github" },
      { id: "linear", label: "Linear", href: "/linear" },
    ]);
  });

  it("returns no setup destinations when integrations are unavailable", () => {
    expect(
      getAvailableIntegrationLinks({
        githubReady: false,
        gitlabReady: false,
        jiraAvailable: false,
        linearAvailable: false,
      }),
    ).toEqual([]);
  });
});

describe("IntegrationsMenu", () => {
  it("opens configured integration links on hover", async () => {
    mockAvailability({ githubReady: true, jiraAvailable: true, linearAvailable: false });

    renderWithQuery(createElement(IntegrationsMenu, {}));

    const trigger = screen.getByRole("button", { name: "Integrations" });
    expect(screen.queryByText("GitHub")).toBeNull();

    fireEvent.pointerEnter(trigger);

    expect(await screen.findByText("GitHub")).toBeTruthy();
    expect(screen.getByText("Jira")).toBeTruthy();
    expect(screen.queryByText("Linear")).toBeNull();
  });

  it("does not render when no integrations are configured", () => {
    mockAvailability({ githubReady: false, jiraAvailable: false, linearAvailable: false });

    renderWithQuery(createElement(IntegrationsMenu, {}));

    expect(screen.queryByRole("button", { name: "Integrations" })).toBeNull();
  });

  it("passes the active workspace id to the per-workspace availability hooks", () => {
    activeWorkspaceRef.id = "ws-active";
    activeWorkspaceRef.items = [{ id: "ws-active" }];
    mockAvailability({ githubReady: true, jiraAvailable: true, linearAvailable: true });

    renderWithQuery(createElement(IntegrationsMenu, {}));

    // Jira and Linear are per-workspace: they must be scoped to the active
    // workspace so the sidebar reflects the workspace the user is viewing.
    expect(useJiraAvailableMock).toHaveBeenCalledWith("ws-active");
    expect(useLinearAvailableMock).toHaveBeenCalledWith("ws-active");
  });

  it("falls back to null scope when the active workspace id is stale", () => {
    // The active workspace was removed but activeId was not reconciled. Scoping
    // to the deleted id would hide the links; fall back to null instead so the
    // backend's default-workspace resolution applies.
    activeWorkspaceRef.id = "ws-deleted";
    activeWorkspaceRef.items = [{ id: "ws-remaining" }];
    mockAvailability({ githubReady: false, jiraAvailable: true, linearAvailable: true });

    renderWithQuery(createElement(IntegrationsMenu, {}));

    expect(useJiraAvailableMock).toHaveBeenCalledWith(null);
    expect(useLinearAvailableMock).toHaveBeenCalledWith(null);
  });
});

function renderWithTooltip(component: Parameters<typeof render>[0]) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- TooltipProvider requires children in its type but createElement passes them as 3rd arg
  return renderWithQuery(createElement(TooltipProvider, {} as any, component));
}

describe("IntegrationsTopbarLinks", () => {
  it("renders an icon link for each configured integration", () => {
    mockAvailability({ githubReady: true, jiraAvailable: false, linearAvailable: true });

    renderWithTooltip(createElement(IntegrationsTopbarLinks, {}));

    const githubLink = screen.getByRole("link", { name: "GitHub" });
    const linearLink = screen.getByRole("link", { name: "Linear" });
    expect(githubLink.getAttribute("href")).toBe("/github");
    expect(linearLink.getAttribute("href")).toBe("/linear");
    expect(screen.queryByRole("link", { name: "Jira" })).toBeNull();
  });

  it("renders nothing when no integrations are configured", () => {
    mockAvailability({ githubReady: false, jiraAvailable: false, linearAvailable: false });

    const { container } = renderWithTooltip(createElement(IntegrationsTopbarLinks, {}));
    expect(container.firstChild).toBeNull();
  });
});

describe("MobileIntegrationsSection", () => {
  const HELLO_LABEL = "Hello Integration";
  const HELLO_PATH = "/plugins/hello";

  // Track every plugin registered through this suite so the singleton
  // pluginRegistry is fully cleared after each test — a new test adding a new
  // plugin id can't leak nav items into the ones that follow.
  const registeredPluginIds: string[] = [];

  function registerNavItem(pluginId: string, item: NavItem) {
    registeredPluginIds.push(pluginId);
    pluginRegistry.forPlugin(pluginId).registerNavItem(item);
  }

  function registerHelloIntegrationItem() {
    registerNavItem("plugin-a", {
      id: "hello",
      label: HELLO_LABEL,
      path: HELLO_PATH,
      section: "integrations",
    });
  }

  afterEach(() => {
    for (const id of registeredPluginIds) pluginRegistry.unregisterPlugin(id);
    registeredPluginIds.length = 0;
  });

  function renderMobileSection(onNavigate = vi.fn()) {
    return {
      onNavigate,
      ...renderWithQuery(createElement(MobileIntegrationsSection, { onNavigate })),
    };
  }

  it("renders a touch row for each configured first-party link and closes the sheet on click", () => {
    useFeatureMock.mockReturnValue(false);
    mockAvailability({ githubReady: true, jiraAvailable: false, linearAvailable: true });

    const { onNavigate } = renderMobileSection();

    const githubLink = screen.getByRole("link", { name: "GitHub" });
    expect(githubLink.getAttribute("href")).toBe("/github");
    expect(screen.getByRole("link", { name: "Linear" }).getAttribute("href")).toBe("/linear");
    expect(screen.queryByRole("link", { name: "Jira" })).toBeNull();

    fireEvent.click(githubLink);
    expect(onNavigate).toHaveBeenCalledTimes(1);
  });

  it("renders plugin nav items targeting the integrations section, gated on the plugins flag", () => {
    useFeatureMock.mockImplementation((flag: string) => flag === "plugins");
    mockAvailability({ githubReady: false, jiraAvailable: false, linearAvailable: false });
    registerHelloIntegrationItem();
    registerNavItem("plugin-b", {
      id: "main-item",
      label: "Main Item",
      path: "/plugins/main",
      section: "main",
    });

    renderMobileSection();

    const pluginLink = screen.getByTestId("plugin-nav-item-hello");
    expect(pluginLink.getAttribute("href")).toBe(HELLO_PATH);
    expect(screen.getByText(HELLO_LABEL)).toBeTruthy();
    // A "main" section plugin item belongs to the top-level nav, not here.
    expect(screen.queryByTestId("plugin-nav-item-main-item")).toBeNull();
  });

  it("hides plugin nav items when the plugins feature flag is off", () => {
    useFeatureMock.mockReturnValue(false);
    mockAvailability({ githubReady: true, jiraAvailable: false, linearAvailable: false });
    registerHelloIntegrationItem();

    renderMobileSection();

    expect(screen.getByRole("link", { name: "GitHub" })).toBeTruthy();
    expect(screen.queryByTestId("plugin-nav-item-hello")).toBeNull();
  });

  it("renders when only plugin items exist and no first-party links are configured", () => {
    useFeatureMock.mockImplementation((flag: string) => flag === "plugins");
    mockAvailability({ githubReady: false, jiraAvailable: false, linearAvailable: false });
    registerHelloIntegrationItem();

    renderMobileSection();

    expect(screen.getByText("Integrations")).toBeTruthy();
    expect(screen.getByTestId("plugin-nav-item-hello")).toBeTruthy();
  });

  it("renders nothing when there are no links and no plugin items", () => {
    useFeatureMock.mockImplementation((flag: string) => flag === "plugins");
    mockAvailability({ githubReady: false, jiraAvailable: false, linearAvailable: false });

    const { container } = renderMobileSection();

    expect(container.firstChild).toBeNull();
  });
});
