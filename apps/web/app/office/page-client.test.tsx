/* eslint-disable sonarjs/no-duplicate-string */
import { cleanup, render, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { DashboardData } from "@/lib/state/slices/office/types";

const getDashboardMock = vi.hoisted(() => vi.fn());
const listAgentProfilesMock = vi.hoisted(() => vi.fn(async () => ({ agents: [] })));
const setDashboardMock = vi.hoisted(() => vi.fn());
const setOfficeAgentProfilesMock = vi.hoisted(() => vi.fn());

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
  setOfficeAgentProfiles: setOfficeAgentProfilesMock,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/use-office-refetch", () => ({
  useOfficeRefetch: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-api", () => ({
  getDashboard: getDashboardMock,
  listAgentProfiles: listAgentProfilesMock,
}));

vi.mock("./components/routing/provider-health-card", () => ({
  ProviderHealthCard: () => null,
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
    const queryClient = makeQueryClient();
    const data = dashboard();
    state.office.dashboard = data;
    queryClient.setQueryData(qk.office.dashboard("workspace-1"), data);

    renderOfficePage(queryClient);

    await waitFor(() => {
      expect(getDashboardMock).not.toHaveBeenCalled();
    });
  });

  it("fetches dashboard data when neither SSR props nor boot state provided it", async () => {
    const data = dashboard();
    getDashboardMock.mockResolvedValue(data);

    renderOfficePage();

    await waitFor(() => {
      expect(getDashboardMock).toHaveBeenCalledWith("workspace-1", expect.anything());
    });
    await waitFor(() => {
      expect(setDashboardMock).toHaveBeenCalledWith(data);
    });
  });

  it("refetches dashboard data when the active workspace changes", async () => {
    state.office.dashboard = dashboard();
    getDashboardMock.mockResolvedValue({ ...dashboard(), agent_count: 2 });

    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.office.dashboard("workspace-1"), state.office.dashboard);
    const { rerender } = renderOfficePage(queryClient);

    expect(getDashboardMock).not.toHaveBeenCalled();

    state.workspaces.activeId = "workspace-2";
    rerender(
      <QueryClientProvider client={queryClient}>
        <OfficePageClient initialDashboard={null} />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(getDashboardMock).toHaveBeenCalledWith("workspace-2", expect.anything());
    });
  });
});

function renderOfficePage(queryClient = makeQueryClient()) {
  return render(
    <QueryClientProvider client={queryClient}>
      <OfficePageClient initialDashboard={null} />
    </QueryClientProvider>,
  );
}
