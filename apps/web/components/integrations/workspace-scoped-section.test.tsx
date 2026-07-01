import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const state = {
  workspaces: {
    activeId: "ws-active",
    items: [
      { id: "ws-active", name: "Active" },
      { id: "ws-route", name: "Route" },
    ],
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/domains/workspace/use-workspaces", () => ({
  useWorkspaces: () => ({
    items: state.workspaces.items,
    activeId: state.workspaces.activeId,
  }),
}));

import { WorkspaceScopedSection } from "./workspace-scoped-section";

describe("WorkspaceScopedSection", () => {
  afterEach(() => cleanup());

  it("uses an explicit route workspace before the active workspace", () => {
    render(
      <WorkspaceScopedSection workspaceId="ws-route">
        {(workspaceId) => <div data-testid="workspace-id">{workspaceId}</div>}
      </WorkspaceScopedSection>,
    );

    expect(screen.getByTestId("workspace-id").textContent).toBe("ws-route");
  });
});
