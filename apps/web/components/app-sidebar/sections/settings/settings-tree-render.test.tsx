import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const MAIN_WORKSPACE_ID = "ws-1";
const ARCHIVE_WORKSPACE_ID = "ws-10";
const MAIN_WORKSPACE_NAME = "Main Workspace";
const ARCHIVE_WORKSPACE_NAME = "Archive Workspace";

const setActiveWorkspaceMock = vi.hoisted(() => vi.fn());
const workspaceState = vi.hoisted(() => ({
  activeId: "ws-1" as string | null,
  items: [{ id: "ws-1", name: "Main Workspace" }],
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (s: {
      workspaces: { activeId: string | null };
      setActiveWorkspace: typeof setActiveWorkspaceMock;
    }) => unknown,
  ) =>
    selector({
      workspaces: { activeId: workspaceState.activeId },
      setActiveWorkspace: setActiveWorkspaceMock,
    }),
}));

vi.mock("@/hooks/domains/workspace/use-workspaces", () => ({
  useWorkspaces: () => ({
    items: workspaceState.items,
    activeId: workspaceState.activeId,
    activeWorkspace:
      workspaceState.items.find((workspace) => workspace.id === workspaceState.activeId) ?? null,
  }),
}));

vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => undefined,
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => ({
    agentProfiles: [],
    availableAgents: [],
    availableTools: [],
    executors: [],
    settingsAgents: [],
    settingsData: {
      agentsLoaded: true,
      capabilitiesLoaded: true,
      executorsLoaded: true,
    },
  }),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => false,
}));

vi.mock("@kandev/ui/collapsible", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  const CollapsibleContext = React.createContext(false);
  return {
    Collapsible: ({ open, children }: { open?: boolean; children: ReactNode }) =>
      React.createElement(CollapsibleContext.Provider, { value: Boolean(open) }, children),
    CollapsibleContent: ({ children, className }: { children: ReactNode; className?: string }) => {
      const open = React.useContext(CollapsibleContext);
      return open ? React.createElement("div", { className }, children) : null;
    },
  };
});

import { SettingsTree } from "./settings-tree";
import { WorkspacesGroup } from "./workspaces-group";

describe("SettingsTree rendering", () => {
  beforeEach(() => {
    workspaceState.activeId = MAIN_WORKSPACE_ID;
    workspaceState.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
    setActiveWorkspaceMock.mockClear();
  });

  afterEach(() => cleanup());

  it("renders workspace repository and workflow links when Workspaces is open", () => {
    render(<WorkspacesGroup pathname="/settings/workspace" expanded />);

    expect(screen.getByRole("link", { name: "Repositories" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/repositories",
    );
    expect(screen.getByRole("link", { name: "Workflows" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/workflows",
    );
    expect(screen.getByRole("link", { name: "Automations" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/automations",
    );
    expect(screen.getByRole("link", { name: "Integrations" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/integrations",
    );
  });

  it("opens the active workspace by default when the settings tree opens", () => {
    workspaceState.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];

    render(<SettingsTree pathname="/settings" />);

    expect(screen.getByRole("link", { name: `${MAIN_WORKSPACE_NAME} Active` })).toBeTruthy();
    expect(screen.getByRole("link", { name: ARCHIVE_WORKSPACE_NAME })).toBeTruthy();
    expect(screen.queryByText("[active]")).toBeNull();
    expect(screen.getByRole("link", { name: "Repositories" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/repositories",
    );
    expect(screen.getByRole("link", { name: "Automations" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/automations",
    );
  });

  it("uses an accordion for workspace subsections", () => {
    workspaceState.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];

    render(<WorkspacesGroup pathname="/settings" expanded />);

    expect(screen.getByRole("link", { name: "Repositories" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1/repositories",
    );

    fireEvent.click(screen.getByRole("button", { name: "Expand Archive Workspace" }));

    expect(screen.getByRole("button", { name: "Expand Main Workspace" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Repositories" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-10/repositories",
    );
  });

  it("only opens the routed workspace subsection on workspace detail routes", () => {
    workspaceState.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];

    const { rerender } = render(<WorkspacesGroup pathname="/settings/workspace" expanded />);

    expect(screen.getAllByRole("link", { name: "Repositories" })).toHaveLength(1);

    rerender(<WorkspacesGroup pathname="/settings/workspace/ws-10/repositories" expanded />);

    expect(
      screen.getByRole("link", { name: `${MAIN_WORKSPACE_NAME} Active` }).getAttribute("href"),
    ).toBe("/settings/workspace/ws-1");
    const repositoryLinks = screen.getAllByRole("link", { name: "Repositories" });
    const workflowLinks = screen.getAllByRole("link", { name: "Workflows" });

    expect(repositoryLinks).toHaveLength(1);
    expect(workflowLinks).toHaveLength(1);
    expect(repositoryLinks[0].getAttribute("href")).toBe("/settings/workspace/ws-10/repositories");
    expect(workflowLinks[0].getAttribute("href")).toBe("/settings/workspace/ws-10/workflows");
    expect(screen.getByRole("link", { name: ARCHIVE_WORKSPACE_NAME }).getAttribute("href")).toBe(
      "/settings/workspace/ws-10",
    );
  });

  it("opens workspace integrations when a workspace integration route is active", () => {
    workspaceState.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];

    render(<WorkspacesGroup pathname="/settings/workspace/ws-10/integrations/github" expanded />);

    expect(screen.getByRole("link", { name: "GitHub" }).getAttribute("href")).toBe(
      "/settings/workspace/ws-10/integrations/github",
    );
    expect(screen.getByRole("button", { name: "Expand Main Workspace" })).toBeTruthy();
  });

  it("keeps Voice Mode in the settings tree as a standalone active leaf", () => {
    render(<SettingsTree pathname="/settings" />);

    expect(screen.getByRole("link", { name: "Voice Mode" }).getAttribute("href")).toBe(
      "/settings/voice-mode",
    );

    cleanup();

    render(<SettingsTree pathname="/settings/voice-mode" />);

    expect(screen.getByRole("link", { name: "Voice Mode" }).className).toContain(
      "before:bg-primary",
    );
    expect(screen.queryByRole("link", { name: "Appearance" })).toBeNull();
  });
});

describe("WorkspacesGroup active workspace presentation", () => {
  beforeEach(() => {
    workspaceState.activeId = MAIN_WORKSPACE_ID;
    workspaceState.items = [
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
    ];
    setActiveWorkspaceMock.mockClear();
  });

  afterEach(() => cleanup());

  it("keeps the active workspace first even when the API returns it later", () => {
    render(<WorkspacesGroup pathname="/settings" expanded />);

    const workspaceLinks = getWorkspaceRootLinks();

    expect(workspaceLinks.map((link) => link.textContent)).toEqual([
      `${MAIN_WORKSPACE_NAME}Active`,
      ARCHIVE_WORKSPACE_NAME,
    ]);
  });

  it("expands another workspace without changing the active workspace", () => {
    render(<WorkspacesGroup pathname="/settings" expanded />);

    fireEvent.click(screen.getByRole("button", { name: "Expand Archive Workspace" }));

    expect(setActiveWorkspaceMock).not.toHaveBeenCalled();
    expect(getWorkspaceRootLinks()[0].textContent).toBe(`${MAIN_WORKSPACE_NAME}Active`);
    expect(screen.getByRole("link", { name: `${MAIN_WORKSPACE_NAME} Active` })).toBeTruthy();
  });
});

describe("WorkspacesGroup integration route sync", () => {
  beforeEach(() => {
    workspaceState.activeId = MAIN_WORKSPACE_ID;
    workspaceState.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];
  });

  afterEach(() => cleanup());

  it("opens workspace integrations after navigating into an integration route", async () => {
    const { rerender } = render(
      <WorkspacesGroup pathname="/settings/workspace/ws-10/repositories" expanded />,
    );

    expect(screen.queryByRole("link", { name: "GitHub" })).toBeNull();

    rerender(<WorkspacesGroup pathname="/settings/workspace/ws-10/integrations/github" expanded />);

    expect((await screen.findByRole("link", { name: "GitHub" })).getAttribute("href")).toBe(
      "/settings/workspace/ws-10/integrations/github",
    );
  });
});

function getWorkspaceRootLinks(): HTMLAnchorElement[] {
  return screen.getAllByRole("link").filter((link): link is HTMLAnchorElement => {
    const href = link.getAttribute("href");
    return Boolean(href?.match(/^\/settings\/workspace\/[^/]+$/));
  });
}
