import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { UseCoordinatorSidebarEntriesResult } from "@/app/coordinator/use-coordinator-sidebar-entries";

const mocks = vi.hoisted(() => ({
  entries: undefined as unknown as UseCoordinatorSidebarEntriesResult,
  featureOn: true,
  activeWorkspaceId: null as string | null,
  sectionExpanded: {} as Record<string, boolean>,
  toggleSection: vi.fn(),
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
    }),
}));

import { MobileCoordinatorsSection } from "./mobile-coordinators-section";

function coordinator(id: string, name: string): Coordinator {
  return {
    id,
    workspace_id: "ws-1",
    name,
    agent_profile_id: "agent-1",
    executor_profile_id: "exec-1",
    task_agent_profile_id: "ap-1",
    task_executor_profile_id: "ep-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
  };
}

const TWO = [coordinator("c-1", "Planner"), coordinator("c-2", "Reviewer")];
const onNavigate = vi.fn();

beforeEach(() => {
  mocks.featureOn = true;
  mocks.activeWorkspaceId = "ws-1";
  mocks.sectionExpanded = {};
  mocks.toggleSection.mockReset();
  onNavigate.mockReset();
  mocks.entries = { coordinators: TWO, badgeByCoordinatorId: new Map() };
});

afterEach(() => cleanup());

describe("MobileCoordinatorsSection", () => {
  it("renders nothing while the coordinator list has not loaded", () => {
    mocks.entries = { coordinators: undefined, badgeByCoordinatorId: new Map() };
    const { container } = render(<MobileCoordinatorsSection onNavigate={onNavigate} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when the feature is off or no workspace is active", () => {
    mocks.featureOn = false;
    const off = render(<MobileCoordinatorsSection onNavigate={onNavigate} />);
    expect(off.container.firstChild).toBeNull();
    off.unmount();

    mocks.featureOn = true;
    mocks.activeWorkspaceId = null;
    const none = render(<MobileCoordinatorsSection onNavigate={onNavigate} />);
    expect(none.container.firstChild).toBeNull();
  });

  it("starts expanded with rows in list order, badges and Needs you links", () => {
    mocks.entries = {
      coordinators: TWO,
      badgeByCoordinatorId: new Map([
        ["c-1", 0],
        ["c-2", 4],
      ]),
    };
    render(<MobileCoordinatorsSection onNavigate={onNavigate} />);

    const header = screen.getByRole("button", { name: /Coordinators/ });
    expect(header.getAttribute("aria-expanded")).toBe("true");
    expect(header.getAttribute("aria-controls")).toBe("mobile-coordinators-body");
    const rows = screen.getAllByTestId(/^mobile-sidebar-coordinator-c-/);
    expect(rows.map((r) => r.getAttribute("data-testid"))).toEqual([
      "mobile-sidebar-coordinator-c-1",
      "mobile-sidebar-coordinator-c-2",
    ]);
    expect(rows[0].getAttribute("href")).toBe("/workspaces/ws-1/coordinator/c-1");
    expect(rows[0].textContent).toBe("Planner");
    expect(rows[1].textContent).toContain("Reviewer");
    expect(rows[1].textContent).toContain("4");
    expect(screen.queryByTestId("mobile-coordinators-collapsed-summary")).toBeNull();
  });

  it("toggles through the persisted sidebar section state with an expanded default", () => {
    render(<MobileCoordinatorsSection onNavigate={onNavigate} />);
    fireEvent.click(screen.getByRole("button", { name: /Coordinators/ }));
    expect(mocks.toggleSection).toHaveBeenCalledWith("coordinators", true);
  });

  it("shows the badge sum when folded and hides the rows", () => {
    mocks.sectionExpanded = { coordinators: false };
    mocks.entries = {
      coordinators: TWO,
      badgeByCoordinatorId: new Map([
        ["c-1", 2],
        ["c-2", 3],
      ]),
    };
    render(<MobileCoordinatorsSection onNavigate={onNavigate} />);

    expect(screen.getByTestId("mobile-coordinators-collapsed-summary").textContent).toBe("5");
    expect(screen.queryByTestId("mobile-sidebar-coordinator-c-1")).toBeNull();
  });

  it("shows the coordinator count when folded and no coordinator has a badge", () => {
    mocks.sectionExpanded = { coordinators: false };
    render(<MobileCoordinatorsSection onNavigate={onNavigate} />);

    expect(screen.getByTestId("mobile-coordinators-collapsed-summary").textContent).toBe("2");
  });

  it("renders the set-up row linking to the coordinators settings tab when there are none", () => {
    mocks.entries = { coordinators: [], badgeByCoordinatorId: new Map() };
    render(<MobileCoordinatorsSection onNavigate={onNavigate} />);

    const empty = screen.getByTestId("mobile-sidebar-coordinators-empty");
    expect(empty.textContent).toBe("Set up a coordinator");
    expect(empty.getAttribute("href")).toBe("/settings/workspaces/ws-1/coordinators");
  });

  it("links the header shortcut to the coordinator route", () => {
    render(<MobileCoordinatorsSection onNavigate={onNavigate} />);

    const shortcut = screen.getByTestId("mobile-coordinators-open-list");
    expect(shortcut.getAttribute("href")).toBe("/workspaces/ws-1/coordinator");
    expect(shortcut.getAttribute("aria-label")).toBe("Open coordinators");
  });

  it("calls onNavigate when a row is tapped", () => {
    render(<MobileCoordinatorsSection onNavigate={onNavigate} />);
    fireEvent.click(screen.getByTestId("mobile-sidebar-coordinator-c-1"));
    expect(onNavigate).toHaveBeenCalledTimes(1);
  });
});
