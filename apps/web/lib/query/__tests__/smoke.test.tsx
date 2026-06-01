import React from "react";
import { describe, it, expect } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";

/**
 * Minimal smoke test for the TanStack Query foundation.
 *
 * Asserts that a basic useQuery hook transitions from loading → success
 * with the expected data. This is the smallest possible regression check
 * for the Wave 0 installation.
 */
describe("TanStack Query smoke test", () => {
  it("transitions from loading to data=1", async () => {
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
      },
    });

    function wrapper({ children }: { children: React.ReactNode }) {
      return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    }

    const { result } = renderHook(
      () => useQuery({ queryKey: ["__smoke"], queryFn: () => Promise.resolve(1) }),
      { wrapper },
    );

    // Initially loading
    expect(result.current.isLoading).toBe(true);
    expect(result.current.data).toBeUndefined();

    // Resolves to data=1
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toBe(1);
  });
});
