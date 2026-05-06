import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { AgentRunsTab } from "./agent-runs-tab";

// Hoisted mock so the listRuns import is replaced before the component
// imports it. Tests configure the mock per-case.
const listRunsMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
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

    render(
      <StateProvider initialState={{ workspaces: { activeId: "ws-1", items: [] } }}>
        <AgentRunsTab agent={ceo} />
      </StateProvider>,
    );

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

    render(
      <StateProvider initialState={{ workspaces: { activeId: "ws-1", items: [] } }}>
        <AgentRunsTab agent={ceo} />
      </StateProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText(/no runs yet/i)).toBeTruthy();
    });
  });
});
