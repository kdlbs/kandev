import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const MAIN_WORKSPACE_ID = "ws-1";
const ARCHIVE_WORKSPACE_ID = "ws-10";
const MAIN_WORKSPACE_NAME = "Main Workspace";
const ARCHIVE_WORKSPACE_NAME = "Archive Workspace";

const state = {
  workspaces: {
    items: [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }],
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
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
    state.workspaces.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
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
  });

  it("only opens the active workspace subsection on workspace detail routes", () => {
    state.workspaces.items = [
      { id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME },
      { id: ARCHIVE_WORKSPACE_ID, name: ARCHIVE_WORKSPACE_NAME },
    ];

    const { rerender } = render(<WorkspacesGroup pathname="/settings/workspace" expanded />);

    expect(screen.getAllByRole("link", { name: "Repositories" })).toHaveLength(2);

    rerender(<WorkspacesGroup pathname="/settings/workspace/ws-10/repositories" expanded />);

    expect(screen.getByRole("link", { name: MAIN_WORKSPACE_NAME }).getAttribute("href")).toBe(
      "/settings/workspace/ws-1",
    );
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
