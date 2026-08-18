import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { WorkspaceScopeProvider, useWorkspaceScope } from "./workspace-scope-provider";

const OFFICE = {
  id: "office-1",
  name: "Office",
  owner_id: "owner",
  office_workflow_id: "wf-office",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};
const KANBAN = { ...OFFICE, id: "kanban-1", name: "Kanban", office_workflow_id: null };

function Probe() {
  const { mode, workspaceId } = useWorkspaceScope();
  return <span data-testid="scope">{`${mode}:${workspaceId ?? "none"}`}</span>;
}

function renderScope(initialState: Partial<AppState>): string {
  render(
    <StateProvider initialState={initialState}>
      <WorkspaceScopeProvider>
        <Probe />
      </WorkspaceScopeProvider>
    </StateProvider>,
  );
  return screen.getByTestId("scope").textContent ?? "";
}

afterEach(() => cleanup());

describe("WorkspaceScopeProvider", () => {
  it("reports office mode for an active office workspace", () => {
    expect(renderScope({ workspaces: { items: [KANBAN, OFFICE], activeId: OFFICE.id } })).toBe(
      "office:office-1",
    );
  });

  it("reports kanban mode for an active kanban workspace", () => {
    expect(renderScope({ workspaces: { items: [KANBAN, OFFICE], activeId: KANBAN.id } })).toBe(
      "kanban:kanban-1",
    );
  });

  it("reports unknown while the workspace list is unhydrated", () => {
    // The frame every boot passes through. Reading it as kanban is what makes
    // the sidebar paint one mode and swap to the other.
    expect(renderScope({ workspaces: { items: [], activeId: OFFICE.id } })).toBe("unknown:none");
  });

  it("resolves a populated list with no active workspace without showing Office mode", () => {
    expect(renderScope({ workspaces: { items: [KANBAN], activeId: "deleted-workspace" } })).toBe(
      "kanban:none",
    );
  });
});
