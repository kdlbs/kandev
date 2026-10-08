import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { UseCoordinatorSidebarEntriesResult } from "@/app/coordinator/use-coordinator-sidebar-entries";

const mocks = vi.hoisted(() => ({
  entries: undefined as unknown as UseCoordinatorSidebarEntriesResult,
  featureOn: true,
  activeWorkspaceId: null as string | null,
  sectionExpanded: {} as Record<string, boolean>,
  pathname: "/",
  toggleSection: vi.fn(),
  setCollapsed: vi.fn(),
}));

vi.mock("@/app/coordinator/use-coordinator-sidebar-entries", () => ({
  useCoordinatorSidebarEntries: () => mocks.entries,
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => mocks.featureOn,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { activeId: mocks.activeWorkspaceId },
      appSidebar: { sectionExpanded: mocks.sectionExpanded },
      toggleAppSidebarSection: mocks.toggleSection,
      setAppSidebarCollapsed: mocks.setCollapsed,
    }),
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => mocks.pathname,
}));

import { CoordinatorsSection } from "./coordinators-section";

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

function renderSection(collapsed = false) {
  return render(
    <TooltipProvider>
      <CoordinatorsSection collapsed={collapsed} />
    </TooltipProvider>,
  );
}

const TWO = [coordinator("c-1", "Planner"), coordinator("c-2", "Reviewer")];

beforeEach(() => {
  mocks.featureOn = true;
  mocks.activeWorkspaceId = "ws-1";
  mocks.sectionExpanded = {};
  mocks.pathname = "/";
  mocks.entries = { coordinators: TWO, badgeByCoordinatorId: new Map() };
});

afterEach(() => cleanup());

describe("CoordinatorsSection", () => {
  it("renders nothing while the coordinator list has not loaded", () => {
    mocks.entries = { coordinators: undefined, badgeByCoordinatorId: new Map() };
    const { container } = renderSection();
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when the feature is off or no workspace is active", () => {
    mocks.featureOn = false;
    const off = renderSection();
    expect(off.container.firstChild).toBeNull();
    off.unmount();

    mocks.featureOn = true;
    mocks.activeWorkspaceId = null;
    const noWorkspace = renderSection();
    expect(noWorkspace.container.firstChild).toBeNull();
  });

  it("renders rows in list order with their badges, each linking to its Needs you screen", () => {
    mocks.entries = {
      coordinators: TWO,
      badgeByCoordinatorId: new Map([
        ["c-1", 0],
        ["c-2", 4],
      ]),
    };
    renderSection();

    const rows = screen.getAllByTestId(/^sidebar-coordinator-c-/);
    expect(rows.map((r) => r.getAttribute("data-testid"))).toEqual([
      "sidebar-coordinator-c-1",
      "sidebar-coordinator-c-2",
    ]);
    expect(rows[0].getAttribute("href")).toBe("/workspaces/ws-1/coordinator/c-1");
    expect(rows[0].textContent).toBe("Planner");
    expect(rows[1].getAttribute("href")).toBe("/workspaces/ws-1/coordinator/c-2");
    expect(rows[1].textContent).toContain("Reviewer");
    expect(rows[1].textContent).toContain("4");
    expect(screen.queryByTestId("sidebar-coordinators-empty")).toBeNull();
  });

  it("marks the row whose pages are open as active", () => {
    mocks.pathname = "/workspaces/ws-1/coordinator/c-2/queue";
    renderSection();

    expect(screen.getByTestId("sidebar-coordinator-c-2").className).toContain("before:bg-primary");
    expect(screen.getByTestId("sidebar-coordinator-c-1").className).not.toContain(
      "before:bg-primary",
    );
  });

  it("draws the section with the coordinator icon, not the agent-profile robot", () => {
    renderSection(true);
    const rail = screen.getByRole("button", { name: "Coordinators" });
    expect(rail.querySelector("svg.tabler-icon-user-cog")).not.toBeNull();
    expect(rail.querySelector("svg.tabler-icon-robot")).toBeNull();
  });

  it("shows the badge sum beside the label when folded", () => {
    mocks.sectionExpanded = { coordinators: false };
    mocks.entries = {
      coordinators: TWO,
      badgeByCoordinatorId: new Map([
        ["c-1", 2],
        ["c-2", 3],
      ]),
    };
    renderSection();

    expect(screen.getByTestId("sidebar-section-collapsed-summary").textContent).toBe("5");
  });

  it("shows the coordinator count when folded and no coordinator has a badge", () => {
    mocks.sectionExpanded = { coordinators: false };
    renderSection();

    expect(screen.getByTestId("sidebar-section-collapsed-summary").textContent).toBe("2");
  });

  it("starts expanded when nothing is persisted, and hides the summary", () => {
    renderSection();

    expect(screen.getByTestId("sidebar-coordinator-c-1")).toBeTruthy();
    expect(screen.queryByTestId("sidebar-section-collapsed-summary")).toBeNull();
  });

  it("renders the set-up row linking to the coordinators settings tab when there are none", () => {
    mocks.entries = { coordinators: [], badgeByCoordinatorId: new Map() };
    renderSection();

    const empty = screen.getByTestId("sidebar-coordinators-empty");
    expect(empty.textContent).toBe("Set up a coordinator");
    expect(empty.getAttribute("href")).toBe("/settings/workspaces/ws-1/coordinators");
  });

  it("links the header shortcut to the coordinator route", () => {
    renderSection();

    const shortcut = screen.getByTestId("coordinators-open-list");
    expect(shortcut.getAttribute("href")).toBe("/workspaces/ws-1/coordinator");
    expect(shortcut.getAttribute("aria-label")).toBe("Open coordinators");
  });
});
