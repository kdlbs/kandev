import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { useControlCachedUsage } from "./use-control-cached-usage";

const fixture = vi.hoisted(() => ({
  read: vi.fn(),
  state: {
    agentProfiles: {
      items: [
        { id: "p1", workspace_id: "w1" },
        { id: "p2", workspace_id: "w2" },
      ],
    },
  },
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: typeof fixture.state) => unknown) => select(fixture.state),
}));
vi.mock("@/lib/state/shared-resource-reads", () => ({ stateReadScopeIdentity: () => "owner" }));
vi.mock("@/lib/api/domains/provider-usage-api", () => ({
  readCachedProfileExecutionUsage: fixture.read,
}));

beforeEach(() => {
  fixture.read.mockReset();
});
afterEach(cleanup);

describe("cached-only account usage", () => {
  it("reads only in-scope profiles and preserves observed zero", async () => {
    const entry = {
      execution_id: "execution",
      session_id: "session",
      report: {
        source_known: true,
        reason: "ok",
        limited: false,
        model: "sol",
        current: {
          provider: "openai",
          account_ref: "account",
          fetched_at: "2026-10-19T00:00:00Z",
          windows: [
            {
              label: "5 hours",
              utilization_pct: 0,
              utilization_observed: true,
              reset_at: "",
              reset_at_observed: false,
            },
          ],
        },
      },
    };
    fixture.read.mockResolvedValue({
      profile_id: "p1",
      cached_only: true,
      executions: [entry],
    });
    const revision = {};
    const { result } = renderHook(() => useControlCachedUsage(["w1"], revision));
    await waitFor(() => expect(result.current.data?.[0]?.status).toBe("ready"));
    expect(fixture.read).toHaveBeenCalledTimes(1);
    expect(fixture.read.mock.calls[0][0]).toBe("p1");
    expect(result.current.data?.[0]?.entries).toEqual([entry]);
  });

  it.each([
    { profile_id: "p1", executions: [] },
    { profile_id: "foreign", cached_only: true, executions: [] },
    {
      profile_id: "p1",
      cached_only: true,
      executions: [{ execution_id: "e", session_id: "s", report: {} }],
    },
  ])("rejects missing cache-only proof or incompatible facts: %j", async (response) => {
    fixture.read.mockResolvedValue(response);
    const revision = {};
    const { result } = renderHook(() => useControlCachedUsage(["w1"], revision));
    await waitFor(() => expect(result.current.data?.[0]?.status).toBe("unsupported"));
    expect(result.current.data?.[0]?.entries).toEqual([]);
  });

  it("keeps an unsupported cached route unavailable without a provider-fetching fallback", async () => {
    fixture.read.mockImplementation(async () => {
      throw new ApiError("unsupported", 404, null);
    });
    const revision = {};
    const { result } = renderHook(() => useControlCachedUsage(["w1"], revision));
    await waitFor(() => expect(result.current.data?.[0]?.status).toBe("unsupported"));
    expect(fixture.read).toHaveBeenCalledTimes(1);
  });

  it("does not read any profiles while the authorized workspace snapshot is absent", async () => {
    const { result } = renderHook(() => useControlCachedUsage(["w1"], undefined));
    expect(result.current.status).toBe("loading");
    expect(fixture.read).not.toHaveBeenCalled();
  });
});
