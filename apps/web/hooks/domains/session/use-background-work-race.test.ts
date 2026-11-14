import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { create, useStore } from "zustand";
import { immer } from "zustand/middleware/immer";
import {
  createSessionRuntimeSlice,
  purgeSessionRuntimeState,
} from "@/lib/state/slices/session-runtime/session-runtime-slice";
import type { SessionRuntimeSlice } from "@/lib/state/slices/session-runtime/types";
import type { WorkloadRunObservation } from "@/lib/types/background-work";

const listBackgroundWorkloads = vi.fn();
let store: ReturnType<typeof makeStore>;

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

vi.mock("@/lib/api/domains/background-work-api", () => ({
  listBackgroundWorkloads: (...args: unknown[]) => listBackgroundWorkloads(...args),
  executeBackgroundAction: vi.fn(),
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: SessionRuntimeSlice) => unknown) => useStore(store, selector),
  useAppStoreApi: () => store,
}));

import { useBackgroundWork } from "./use-background-work";

const SESSION_ID = "session-a";
const work: WorkloadRunObservation = {
  work_id: "detached-job",
  run_id: "run-1",
  title: "Detached background job",
  kind: "shell",
  state: "running",
  revision: 1,
  capabilities: {
    discovery: "events_only",
    output: "none",
    transcript: false,
    parentage: false,
    reasoning_summary: false,
    attributable_usage: false,
  },
};

function deferRead() {
  let resolve!: (value: { workloads: WorkloadRunObservation[] }) => void;
  listBackgroundWorkloads.mockReturnValueOnce(
    new Promise<{ workloads: WorkloadRunObservation[] }>((done) => {
      resolve = done;
    }),
  );
  return (workloads: WorkloadRunObservation[]) => resolve({ workloads });
}

beforeEach(() => {
  vi.resetAllMocks();
  store = makeStore();
});

describe("background workload snapshot and live update ordering", () => {
  it("keeps a running job received while the initial empty snapshot is in flight", async () => {
    const finishRead = deferRead();
    const { result } = renderHook(() => useBackgroundWork(SESSION_ID));

    act(() => store.getState().updateBackgroundWorkload(SESSION_ID, work));
    expect(result.current.runningWorkloads).toEqual([work]);

    await act(async () => finishRead([]));

    expect(result.current.runningWorkloads).toEqual([work]);
    expect(result.current.isInitialLoading).toBe(false);
  });

  it("merges independent snapshot rows without dropping a job discovered live", async () => {
    const finishRead = deferRead();
    const { result } = renderHook(() => useBackgroundWork(SESSION_ID));
    act(() => store.getState().updateBackgroundWorkload(SESSION_ID, work));
    const history = { ...work, work_id: "history", state: "completed" as const };

    await act(async () => finishRead([history]));

    expect(result.current.workloads).toEqual([history, work]);
  });

  it("keeps a live terminal transition while removing unchanged omitted history", async () => {
    const history = { ...work, work_id: "history", state: "completed" as const };
    store.getState().setBackgroundWorkloads(SESSION_ID, [work, history]);
    const finishRead = deferRead();
    const { result } = renderHook(() => useBackgroundWork(SESSION_ID));
    const completed = { ...work, state: "completed" as const, revision: 2 };
    act(() => store.getState().updateBackgroundWorkload(SESSION_ID, completed));

    await act(async () => finishRead([]));

    expect(result.current.workloads).toEqual([completed]);
    expect(result.current.runningWorkloads).toEqual([]);
  });
});

describe("background workload read ownership", () => {
  it("does not publish an older consumer's result or settle a newer pending read", async () => {
    const finishOlder = deferRead();
    const first = renderHook(() => useBackgroundWork(SESSION_ID));
    const finishNewer = deferRead();
    const second = renderHook(() => useBackgroundWork(SESSION_ID));

    await act(async () => finishOlder([work]));

    expect(first.result.current.workloads).toEqual([]);
    expect(first.result.current.isLoading).toBe(true);
    expect(first.result.current.isInitialLoading).toBe(true);
    expect(second.result.current.isInitialLoading).toBe(true);

    await act(async () => finishNewer([]));

    expect(first.result.current.isLoading).toBe(false);
    expect(first.result.current.isInitialLoading).toBe(false);
    expect(second.result.current.isInitialLoading).toBe(false);
  });

  it("does not resurrect rows after a newer consumer accepts an empty snapshot", async () => {
    store.getState().setBackgroundWorkloads(SESSION_ID, [work]);
    const finishOlder = deferRead();
    const first = renderHook(() => useBackgroundWork(SESSION_ID));
    const finishNewer = deferRead();
    renderHook(() => useBackgroundWork(SESSION_ID));

    await act(async () => finishNewer([]));
    await act(async () => finishOlder([work]));

    expect(first.result.current.workloads).toEqual([]);
  });

  it("keeps a successful replacement read when an older read fails", async () => {
    let failOlder!: (error: Error) => void;
    listBackgroundWorkloads.mockReturnValueOnce(new Promise((_, reject) => (failOlder = reject)));
    const first = renderHook(() => useBackgroundWork(SESSION_ID));
    const finishNewer = deferRead();
    const second = renderHook(() => useBackgroundWork(SESSION_ID));
    await act(async () => finishNewer([work]));

    await act(async () => failOlder(new Error("old read failed")));

    expect(first.result.current.workloads).toEqual([work]);
    expect(first.result.current.loadError).toBe(false);
    expect(second.result.current.loadError).toBe(false);
    expect(second.result.current.isLoading).toBe(false);
  });

  it("fences stale rows and readiness across A-B-A session changes", async () => {
    const finishFirstA = deferRead();
    const { result, rerender } = renderHook(({ sessionId }) => useBackgroundWork(sessionId), {
      initialProps: { sessionId: SESSION_ID },
    });
    const finishB = deferRead();
    rerender({ sessionId: "session-b" });
    const finishLatestA = deferRead();
    rerender({ sessionId: SESSION_ID });

    await act(async () => finishFirstA([work]));
    expect(result.current.workloads).toEqual([]);
    expect(result.current.isInitialLoading).toBe(true);

    const sibling = { ...work, work_id: "sibling" };
    await act(async () => finishB([sibling]));
    await act(async () => finishLatestA([]));
    expect(result.current.workloads).toEqual([]);
    expect(result.current.isInitialLoading).toBe(false);
    expect(store.getState().backgroundWork.workloadsBySessionId["session-b"]).toEqual([sibling]);
  });

  it.each(["clear", "purge"])("ignores a response after session %s", async (operation) => {
    const replacement = { ...work, work_id: "replacement", run_id: "run-2" };
    store.getState().setBackgroundWorkloads("session-b", [replacement]);
    const finishRead = deferRead();
    renderHook(() => useBackgroundWork(SESSION_ID));
    act(() => {
      if (operation === "clear") store.getState().clearBackgroundWork(SESSION_ID);
      else store.setState((draft) => purgeSessionRuntimeState(draft, SESSION_ID));
      store.getState().updateBackgroundWorkload(SESSION_ID, replacement);
    });

    await act(async () => finishRead([work]));

    const state = store.getState().backgroundWork;
    expect(state.workloadsBySessionId[SESSION_ID]).toEqual([replacement]);
    expect(state.workloadsBySessionId["session-b"]).toEqual([replacement]);
    expect(state.loadingBySessionId[SESSION_ID]).toBeUndefined();
  });

  it("finishes the shared cache read after its initiating consumer unmounts", async () => {
    const finishRead = deferRead();
    const first = renderHook(() => useBackgroundWork(SESSION_ID));
    first.unmount();

    await act(async () => finishRead([work]));

    expect(store.getState().backgroundWork.workloadsBySessionId[SESSION_ID]).toEqual([work]);
    expect(store.getState().backgroundWork.loadingBySessionId[SESSION_ID]).toBe(false);
  });
});
