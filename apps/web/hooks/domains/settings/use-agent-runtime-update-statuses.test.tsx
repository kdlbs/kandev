import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentUpdateJob } from "@/lib/api";

const listAgentUpdateStatusesMock = vi.fn();

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  listAgentUpdateStatuses: (...args: unknown[]) => listAgentUpdateStatusesMock(...args),
}));

import { useAgentRuntimeUpdateStatuses } from "./use-agent-runtime-update-statuses";

function job(overrides: Partial<AgentUpdateJob> = {}): AgentUpdateJob {
  return {
    job_id: "job-1",
    agent_name: "claude-acp",
    status: "succeeded",
    started_at: "2026-01-01T00:00:00.000Z",
    ...overrides,
  };
}

afterEach(() => vi.clearAllMocks());

describe("useAgentRuntimeUpdateStatuses", () => {
  it("loads structural statuses into a page-local agent map", async () => {
    listAgentUpdateStatusesMock.mockResolvedValueOnce({
      statuses: [
        {
          agent_name: "claude-acp",
          package: "@agentclientprotocol/claude-agent-acp",
          default_version: "0.70.0",
          effective_version: "0.70.0",
          latest_version: "0.71.0",
          check_state: "update_available",
        },
        {
          agent_name: "codex-acp",
          package: "@agentclientprotocol/codex-acp",
          default_version: "1.6.0",
          effective_version: "1.6.0",
          check_state: "unknown",
        },
      ],
    });

    const { result } = renderHook(() => useAgentRuntimeUpdateStatuses({}));

    await waitFor(() => expect(result.current.statusByAgent["claude-acp"]).toBeDefined());
    expect(result.current.statusByAgent["claude-acp"]?.check_state).toBe("update_available");
    expect(result.current.statusByAgent["codex-acp"]?.check_state).toBe("unknown");
    expect(listAgentUpdateStatusesMock).toHaveBeenCalledWith({ cache: "no-store" });
  });

  it("refreshes once after a successful update job", async () => {
    listAgentUpdateStatusesMock
      .mockResolvedValueOnce({ statuses: [] })
      .mockResolvedValueOnce({ statuses: [] });
    const { result, rerender } = renderHook(({ jobs }) => useAgentRuntimeUpdateStatuses(jobs), {
      initialProps: { jobs: {} as Record<string, AgentUpdateJob> },
    });

    await waitFor(() => expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(1));
    rerender({ jobs: { "claude-acp": job() } });
    await waitFor(() => expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(2));
    expect(result.current.statusByAgent).toEqual({});
  });

  it("does not erase a last good map when a refresh fails", async () => {
    listAgentUpdateStatusesMock
      .mockResolvedValueOnce({
        statuses: [
          {
            agent_name: "claude-acp",
            package: "@agentclientprotocol/claude-agent-acp",
            default_version: "0.70.0",
            effective_version: "0.70.0",
            latest_version: "0.71.0",
            check_state: "update_available",
          },
        ],
      })
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({ statuses: [] });
    const { result, rerender } = renderHook(({ jobs }) => useAgentRuntimeUpdateStatuses(jobs), {
      initialProps: { jobs: {} as Record<string, AgentUpdateJob> },
    });

    await waitFor(() => expect(result.current.statusByAgent["claude-acp"]).toBeDefined());
    rerender({ jobs: { "claude-acp": job({ job_id: "job-2" }) } });
    await waitFor(() => expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(2));
    expect(result.current.statusByAgent["claude-acp"]?.check_state).toBe("update_available");

    rerender({ jobs: { "claude-acp": job({ job_id: "job-2" }) } });
    await waitFor(() => expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(3));
  });
});
