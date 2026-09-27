import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { UseCoordinatorSidebarEntriesResult } from "@/app/coordinator/use-coordinator-sidebar-entries";

let entriesResult: UseCoordinatorSidebarEntriesResult;

vi.mock("@/app/coordinator/use-coordinator-sidebar-entries", () => ({
  useCoordinatorSidebarEntries: () => entriesResult,
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/",
}));

import { AppSidebarCoordinatorRows } from "./app-sidebar-coordinator-rows";

function coordinator(id: string, name: string): Coordinator {
  return {
    id,
    workspace_id: "ws-1",
    name,
    agent_profile_id: "agent-1",
    executor_profile_id: "exec-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
  };
}

function renderRows(collapsed = false) {
  return render(
    <TooltipProvider>
      <AppSidebarCoordinatorRows workspaceId="ws-1" collapsed={collapsed} />
    </TooltipProvider>,
  );
}

afterEach(() => cleanup());

describe("AppSidebarCoordinatorRows", () => {
  it("renders nothing while the coordinator list has not loaded", () => {
    entriesResult = { coordinators: undefined, badgeByCoordinatorId: new Map() };
    renderRows();

    expect(screen.queryByTestId("sidebar-coordinator-generic")).toBeNull();
  });

  it("renders one generic Coordinator entry with no badge when the workspace has none", () => {
    entriesResult = { coordinators: [], badgeByCoordinatorId: new Map() };
    renderRows();

    const entry = screen.getByTestId("sidebar-coordinator-generic");
    expect(entry.getAttribute("href")).toBe("/workspaces/ws-1/coordinator");
    expect(entry.querySelector(".bg-primary")).toBeNull();
  });

  it("renders one row per coordinator, by name, linking to its Needs you screen", () => {
    entriesResult = {
      coordinators: [coordinator("c-1", "Planner"), coordinator("c-2", "Reviewer")],
      badgeByCoordinatorId: new Map([
        ["c-1", 0],
        ["c-2", 4],
      ]),
    };
    renderRows();

    const planner = screen.getByTestId("sidebar-coordinator-c-1");
    expect(planner.getAttribute("href")).toBe("/workspaces/ws-1/coordinator/c-1");
    expect(planner.textContent).toContain("Planner");
    expect(planner.textContent).not.toMatch(/\d/);

    const reviewer = screen.getByTestId("sidebar-coordinator-c-2");
    expect(reviewer.getAttribute("href")).toBe("/workspaces/ws-1/coordinator/c-2");
    expect(reviewer.textContent).toContain("4");
  });
});
