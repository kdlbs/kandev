import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useFeature } from "./use-feature";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import type { FeatureFlags } from "@/lib/features";

// Mock the features API so tests don't hit the network.
vi.mock("@/lib/api/domains/features-api", () => ({
  fetchFeatureFlags: vi.fn(),
}));

import { fetchFeatureFlags } from "@/lib/api/domains/features-api";
const mockFetch = vi.mocked(fetchFeatureFlags);

function makeWrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client }, children);
  };
}

describe("useFeature", () => {
  let client: QueryClient;

  beforeEach(() => {
    client = createTestQueryClient();
    vi.clearAllMocks();
  });

  it("returns false before the query resolves (production-safety invariant)", () => {
    // Simulate pending network request — never resolves in this test.
    mockFetch.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useFeature("office"), {
      wrapper: makeWrapper(client),
    });
    expect(result.current).toBe(false);
  });

  it("returns false when the flag is disabled", async () => {
    const flags: FeatureFlags = { office: false };
    mockFetch.mockResolvedValue(flags);

    const { result } = renderHook(() => useFeature("office"), {
      wrapper: makeWrapper(client),
    });

    await waitFor(() => {
      expect(result.current).toBe(false);
    });
  });

  it("returns true when the flag is enabled", async () => {
    const flags: FeatureFlags = { office: true };
    mockFetch.mockResolvedValue(flags);

    const { result } = renderHook(() => useFeature("office"), {
      wrapper: makeWrapper(client),
    });

    await waitFor(() => {
      expect(result.current).toBe(true);
    });
  });

  it("reads from the cache when the query is already seeded", async () => {
    const flags: FeatureFlags = { office: true };
    // Pre-seed the cache (as SSR prefetch would do).
    client.setQueryData(["features"], flags);

    const { result } = renderHook(() => useFeature("office"), {
      wrapper: makeWrapper(client),
    });

    // Synchronously available from cache — no waitFor needed.
    expect(result.current).toBe(true);
    // fetchFeatureFlags should not have been called.
    expect(mockFetch).not.toHaveBeenCalled();
  });
});
