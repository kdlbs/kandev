import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = {
  workspaces: {
    items: [{ id: "ws-1", name: "Main Workspace" }],
  },
  settingsAgents: {
    items: [],
  },
  executors: {
    items: [],
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => undefined,
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
    state.workspaces.items = [{ id: "ws-1", name: "Main Workspace" }];
    state.settingsAgents.items = [];
    state.executors.items = [];
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

  it("keeps Voice Mode in the settings tree", () => {
    render(<SettingsTree pathname="/settings" />);

    expect(screen.getByRole("link", { name: "Voice Mode" }).getAttribute("href")).toBe(
      "/settings/voice-mode",
    );
  });
});
