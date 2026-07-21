import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DashboardData } from "@/lib/state/slices/office/types";

const getDashboardMock = vi.hoisted(() => vi.fn());
const setDashboardMock = vi.hoisted(() => vi.fn());

const state = {
  workspaces: { activeId: "workspace-1" },
  office: {
    dashboard: null as DashboardData | null,
    agentProfiles: [],
    routing: {
      byWorkspace: {},
      knownProviders: [],
      preview: { byWorkspace: {} },
    },
    providerHealth: { byWorkspace: {} },
  },
  setDashboard: setDashboardMock,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/use-office-refetch", () => ({
  useOfficeRefetch: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-api", () => ({
  getDashboard: getDashboardMock,
}));

import { OfficePageClient } from "./page-client";

function dashboard(): DashboardData {
  return {
    agent_count: 1,
    running_count: 0,
    paused_count: 0,
    error_count: 0,
    tasks_in_progress: 0,
    open_tasks: 0,
    blocked_tasks: 0,
    month_spend_subcents: 0,
    pending_approvals: 0,
    recent_activity: [],
    task_count: 2,
    skill_count: 3,
    routine_count: 4,
    run_activity: [],
    task_breakdown: { open: 0, in_progress: 0, blocked: 0, done: 0 },
    recent_tasks: [],
    agent_summaries: [],
  };
}

describe("OfficePageClient boot hydration", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    state.workspaces.activeId = "workspace-1";
    state.office.dashboard = null;
  });

  it("does not fetch dashboard data when Go boot state already hydrated it", async () => {
    state.office.dashboard = dashboard();

    render(<OfficePageClient initialDashboard={null} />);

    await waitFor(() => {
      expect(getDashboardMock).not.toHaveBeenCalled();
    });
  });

  it("fetches dashboard data when neither SSR props nor boot state provided it", async () => {
    const data = dashboard();
    getDashboardMock.mockResolvedValue(data);

    render(<OfficePageClient initialDashboard={null} />);

    await waitFor(() => {
      expect(getDashboardMock).toHaveBeenCalledWith("workspace-1");
    });
    await waitFor(() => {
      expect(setDashboardMock).toHaveBeenCalledWith(data);
    });
  });

  it("refetches dashboard data when the active workspace changes", async () => {
    state.office.dashboard = dashboard();
    getDashboardMock.mockResolvedValue({ ...dashboard(), agent_count: 2 });

    const { rerender } = render(<OfficePageClient initialDashboard={null} />);

    expect(getDashboardMock).not.toHaveBeenCalled();

    state.workspaces.activeId = "workspace-2";
    rerender(<OfficePageClient initialDashboard={null} />);

    await waitFor(() => {
      expect(getDashboardMock).toHaveBeenCalledWith("workspace-2");
    });
  });
});
