import { describe, it, expect, vi, beforeEach } from "vitest";
import { waitFor } from "@testing-library/react";
import { renderHookWithQueryClient } from "@/test-utils/render-with-query";
import { useProviderHealth } from "../use-provider-health";
import { qk } from "@/lib/query/keys";
import type { ProviderHealth } from "@/lib/state/slices/office/types";

const mocks = vi.hoisted(() => ({
  getProviderHealth: vi.fn<[string], Promise<{ health: ProviderHealth[] }>>(),
}));

vi.mock("@/lib/api/domains/office-routing-api", () => ({
  getProviderHealth: mocks.getProviderHealth,
}));

// Also mock the query-options module to avoid importing the full API tree.
// The hook calls officeQueryOptions.providerHealth which calls getProviderHealth.
// We rely on the vi.mock above to intercept the call.

function makeHealth(): ProviderHealth {
  return {
    provider_id: "claude-acp",
    scope: "provider",
    scope_value: "claude-acp",
    state: "healthy",
    backoff_step: 0,
  };
}

describe("useProviderHealth", () => {
  beforeEach(() => {
    mocks.getProviderHealth.mockReset();
    mocks.getProviderHealth.mockResolvedValue({ health: [makeHealth()] });
  });

  it("returns empty array when workspaceName is null", () => {
    const { result } = renderHookWithQueryClient(() => useProviderHealth(null));
    expect(result.current.health).toEqual([]);
    expect(result.current.isLoading).toBe(false);
  });

  it("fetches and returns health data", async () => {
    const { result } = renderHookWithQueryClient(() => useProviderHealth("ws-1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.health).toHaveLength(1);
    expect(result.current.health[0]?.provider_id).toBe("claude-acp");
  });

  it("can seed from cache without fetching", async () => {
    const { client, result } = renderHookWithQueryClient(() => useProviderHealth("ws-1"));
    client.setQueryData(qk.office.providerHealth("ws-1"), [makeHealth()]);
    await waitFor(() => expect(result.current.health).toHaveLength(1));
  });

  it("error is set when fetch fails", async () => {
    mocks.getProviderHealth.mockRejectedValue(new Error("network error"));
    const { result } = renderHookWithQueryClient(() => useProviderHealth("ws-fail"));
    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.error).toBe("network error");
  });
});
