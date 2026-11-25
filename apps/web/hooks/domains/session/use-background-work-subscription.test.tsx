import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import type { WorkloadRunObservation } from "@/lib/types/background-work";

const { listBackgroundWorkloads } = vi.hoisted(() => ({
  listBackgroundWorkloads: vi.fn(),
}));

vi.mock("@/lib/api/domains/background-work-api", () => ({
  listBackgroundWorkloads: (...args: unknown[]) => listBackgroundWorkloads(...args),
  executeBackgroundAction: vi.fn(),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));

import { useBackgroundWork } from "./use-background-work";

const SESSION_ID = "session-1";
const runningWorkload: WorkloadRunObservation = {
  work_id: "work-1",
  run_id: "run-1",
  title: "Detached background job",
  kind: "shell",
  state: "running",
  revision: 1,
  capabilities: {
    discovery: "snapshot",
    output: "stream",
    transcript: false,
    parentage: false,
    reasoning_summary: false,
    attributable_usage: false,
  },
};

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function makeWrapper() {
  const initialState = {
    connection: { ...defaultState.connection, status: "connected" as const },
  };
  return function Wrapper({ children }: { children: ReactNode }) {
    return <StateProvider initialState={initialState}>{children}</StateProvider>;
  };
}

const originalClient = getWebSocketClient();

beforeEach(() => {
  listBackgroundWorkloads.mockReset();
});

afterEach(() => {
  cleanup();
  setWebSocketClient(originalClient);
});

describe("useBackgroundWork session subscription reconciliation", () => {
  it("reads the snapshot again after the session subscription is ready", async () => {
    const ready = deferred();
    const subscribeSessionWithReady = vi.fn(() => ({
      ready: ready.promise,
      unsubscribe: vi.fn(),
    }));
    setWebSocketClient({ subscribeSessionWithReady } as unknown as WebSocketClient);
    listBackgroundWorkloads
      .mockResolvedValueOnce({ workloads: [] })
      .mockResolvedValueOnce({ workloads: [runningWorkload] });

    const { result } = renderHook(
      () => ({ backgroundWork: useBackgroundWork(SESSION_ID), store: useAppStoreApi() }),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(listBackgroundWorkloads).toHaveBeenCalledTimes(1));
    expect(result.current.backgroundWork.runningWorkloads).toEqual([]);
    expect(subscribeSessionWithReady).toHaveBeenCalledWith(SESSION_ID);

    await act(async () => ready.resolve());

    await waitFor(() => {
      expect(result.current.backgroundWork.runningWorkloads).toEqual([runningWorkload]);
    });
  });

  it("refreshes workloads after the session subscription reconnects", async () => {
    const firstReady = deferred();
    const reconnectedReady = deferred();
    const readiness = [firstReady, reconnectedReady];
    let subscriptionIndex = 0;
    const unsubscribe = vi.fn();
    const subscribeSessionWithReady = vi.fn(() => ({
      ready: readiness[subscriptionIndex++]!.promise,
      unsubscribe,
    }));
    setWebSocketClient({ subscribeSessionWithReady } as unknown as WebSocketClient);
    listBackgroundWorkloads
      .mockResolvedValueOnce({ workloads: [] })
      .mockResolvedValueOnce({ workloads: [] })
      .mockResolvedValueOnce({ workloads: [runningWorkload] });

    const { result } = renderHook(
      () => ({ backgroundWork: useBackgroundWork(SESSION_ID), store: useAppStoreApi() }),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(listBackgroundWorkloads).toHaveBeenCalledTimes(1));
    await act(async () => firstReady.resolve());
    await waitFor(() => expect(listBackgroundWorkloads).toHaveBeenCalledTimes(2));

    act(() => result.current.store.getState().setConnectionStatus("reconnecting"));
    act(() => result.current.store.getState().setConnectionStatus("connected"));
    await waitFor(() => expect(subscribeSessionWithReady).toHaveBeenCalledTimes(2));
    expect(unsubscribe).toHaveBeenCalledTimes(1);

    await act(async () => reconnectedReady.resolve());

    await waitFor(() => {
      expect(result.current.backgroundWork.runningWorkloads).toEqual([runningWorkload]);
    });
  });
});
