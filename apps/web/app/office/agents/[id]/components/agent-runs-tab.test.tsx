import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { StateProvider } from "@/components/state-provider";
import { makeQueryClient } from "@/lib/query/client";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { AgentRunsTab } from "./agent-runs-tab";

const listRunsMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-runs-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-runs-api")>(
    "@/lib/api/domains/office-runs-api",
  );
  return {
    ...actual,
    listRuns: listRunsMock,
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const ceo = {
  id: "agent-ceo",
  workspaceId: "ws-1",
  name: "CEO",
  role: "ceo",
  status: "idle",
  agentProfileId: "profile-1",
  createdAt: "2026-05-04T00:00:00Z",
  updatedAt: "2026-05-04T00:00:00Z",
  permissions: {},
  pauseReason: "",
  budgetMonthlyCents: 0,
  maxConcurrentSessions: 1,
} as AgentProfile;

function renderTab(agent = ceo) {
  const queryClient = makeQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <StateProvider initialState={{ workspaces: { activeId: "ws-1", items: [] } }}>
        <AgentRunsTab agent={agent} />
      </StateProvider>
    </QueryClientProvider>,
  );
}

describe("AgentRunsTab", () => {
  // Pins the regression where the API returns snake_case but the
  // frontend filter read camelCase, so every run was filtered out and
  // the tab silently rendered "No runs yet" even when runs existed.
  // If you re-introduce camelCase in Run, this test fails.
  it("renders runs from a snake_case API response and filters by agent_profile_id", async () => {
    listRunsMock.mockResolvedValueOnce({
      runs: [
        {
          id: "wake-1",
          agent_profile_id: "agent-ceo",
          reason: "task_assigned",
          status: "finished",
          requested_at: "2026-05-04T12:00:00Z",
        },
        {
          id: "wake-2",
          // Different agent — must be filtered out.
          agent_profile_id: "agent-other",
          reason: "task_comment",
          status: "finished",
          requested_at: "2026-05-04T12:01:00Z",
        },
      ],
    });

    renderTab();

    // The CEO's run should appear; the other agent's run should not.
    await waitFor(() => {
      expect(screen.getByText("task_assigned")).toBeTruthy();
    });
    expect(screen.queryByText("task_comment")).toBeNull();
    // Empty-state copy must NOT be visible when runs exist.
    expect(screen.queryByText(/no runs yet/i)).toBeNull();
  });

  it("renders the empty state when no runs match the agent", async () => {
    listRunsMock.mockResolvedValueOnce({ runs: [] });

    renderTab();

    await waitFor(() => {
      expect(screen.getByText(/no runs yet/i)).toBeTruthy();
    });
  });
});
