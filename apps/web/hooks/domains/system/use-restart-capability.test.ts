import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RestartCapability } from "@/lib/types/system";

const mocks = vi.hoisted(() => ({
  fetchRestartCapability: vi.fn(),
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  fetchRestartCapability: mocks.fetchRestartCapability,
}));

import { useRestartCapability } from "./use-restart-capability";

beforeEach(() => {
  mocks.fetchRestartCapability.mockReset();
});

describe("useRestartCapability", () => {
  it("keeps the loading state distinct until capability detection resolves", async () => {
    let resolveCapability!: (capability: RestartCapability) => void;
    mocks.fetchRestartCapability.mockReturnValueOnce(
      new Promise<RestartCapability>((resolve) => {
        resolveCapability = resolve;
      }),
    );

    const { result } = renderHook(() => useRestartCapability());

    expect(result.current).toEqual({ status: "loading", capability: undefined });

    await act(async () => {
      resolveCapability({ supported: true, mode: "supervisor", adapter: "supervisor" });
    });
    await waitFor(() => expect(result.current.status).toBe("resolved"));
    expect(result.current.capability).toEqual({
      supported: true,
      mode: "supervisor",
      adapter: "supervisor",
    });
  });

  it("reports capability detection failures as unavailable", async () => {
    mocks.fetchRestartCapability.mockRejectedValueOnce(new Error("connection refused"));

    const { result } = renderHook(() => useRestartCapability());

    await waitFor(() =>
      expect(result.current).toEqual({ status: "unavailable", capability: null }),
    );
  });
});
