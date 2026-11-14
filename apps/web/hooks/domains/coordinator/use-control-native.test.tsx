import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import type { NativeDashboard } from "@/lib/types/control-native";
import { useNativeDashboards } from "./use-control-native";

const api = vi.hoisted(() => ({ native: vi.fn() }));
vi.mock("@/lib/api/domains/control-native-api", async (original) => ({
  ...(await original<typeof import("@/lib/api/domains/control-native-api")>()),
  readNativeDashboard: api.native,
}));

const nativeDashboard: NativeDashboard = {
  workspace_id: "alpha",
  generated_at: "2026-11-09T12:00:00Z",
  since: "2026-11-08T12:00:00Z",
  basis: "recorded_task_session_queue_transition_state",
  inference_state: "unknown",
  tasks: { IN_PROGRESS: 2 },
  sessions: { RUNNING: 1 },
  waiting_input_tasks: 0,
  queued_messages: 1,
  transitions_24h: 0,
  completed_tasks_24h: 0,
  task_stages: [],
};

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

beforeEach(() => api.native.mockReset().mockResolvedValue(nativeDashboard));
afterEach(cleanup);

describe("useNativeDashboards read ownership", () => {
  it("retains a dated row after a transient error and clears it after access is denied", async () => {
    const { result, rerender } = renderHook(
      ({ revision }) => useNativeDashboards(["alpha"], revision),
      { wrapper, initialProps: { revision: {} } },
    );
    await waitFor(() => expect(result.current.data?.[0]?.status).toBe("ready"));
    const currentRow = result.current.data?.[0];
    const receivedAt = currentRow && "receivedAt" in currentRow ? currentRow.receivedAt : undefined;
    expect(receivedAt).toBeTruthy();

    api.native.mockRejectedValueOnce(new Error("temporary network error"));
    rerender({ revision: {} });
    await waitFor(() => expect(api.native).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(result.current.data?.[0]).toMatchObject({ status: "error", stale: true }),
    );
    expect(result.current.data?.[0]?.data).toEqual(nativeDashboard);
    const staleRow = result.current.data?.[0];
    expect(staleRow && "receivedAt" in staleRow ? staleRow.receivedAt : undefined).toBe(receivedAt);

    api.native.mockRejectedValueOnce(new ApiError("denied", 403, {}));
    rerender({ revision: {} });
    await waitFor(() => expect(api.native).toHaveBeenCalledTimes(3));
    await waitFor(() => expect(result.current.data?.[0]?.status).toBe("denied"));
    expect(result.current.data?.[0]?.data).toBeUndefined();
  });

  it("does not publish a response from a retired workspace identity", async () => {
    const pending = Promise.withResolvers<NativeDashboard>();
    api.native.mockReturnValueOnce(pending.promise).mockResolvedValueOnce({
      ...nativeDashboard,
      queued_messages: 2,
    });
    const { result } = renderHook(
      ({ revision }) => ({
        read: useNativeDashboards(["alpha"], revision),
        store: useAppStoreApi(),
      }),
      { wrapper, initialProps: { revision: {} } },
    );
    await waitFor(() => expect(api.native).toHaveBeenCalledTimes(1));
    const oldSignal = api.native.mock.calls[0][1].init.signal as AbortSignal;

    act(() =>
      result.current.store.setState((state) => ({
        workspaceContextGeneration: state.workspaceContextGeneration + 1,
      })),
    );
    expect(result.current.read.status).toBe("loading");
    await waitFor(() => expect(api.native).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.read.data?.[0]?.data?.queued_messages).toBe(2));
    expect(oldSignal.aborted).toBe(true);

    await act(async () => {
      pending.resolve(nativeDashboard);
      await Promise.resolve();
    });
    expect(result.current.read.data?.[0]?.data?.queued_messages).toBe(2);
  });
});
