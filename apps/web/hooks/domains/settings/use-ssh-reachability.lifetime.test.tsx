import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StrictMode, useLayoutEffect, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { SSHReachabilityCard } from "@/components/settings/ssh-reachability-card";
import type { ApiRequestOptions } from "@/lib/api/client";
import type { AppState } from "@/lib/state/store";
import type { SSHReachabilityRecord } from "@/lib/types/http-ssh";
import type { StoreApi } from "zustand";
import { useSSHReachability } from "./use-ssh-reachability";

const transport = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: transport,
}));

type Reachability = ReturnType<typeof useSSHReachability>;
type Pending = {
  path: string;
  options?: ApiRequestOptions;
  resolve: (record: SSHReachabilityRecord) => void;
  reject: (error: Error) => void;
};
const requests: Pending[] = [];
const snapshots = new Map<string, Reachability>();
let store: StoreApi<AppState>;
const A = "executor/a";
const B = "executor/b";
const NOT_KNOWN = "ssh-reachability-not-known";
const STALE = "ssh-reachability-stale";
const HOST = "ssh-reachability-host";
const NOW = new Date("2026-10-06T20:00:00.000Z");

function record(id: string, changes: Partial<SSHReachabilityRecord> = {}): SSHReachabilityRecord {
  return {
    executor_id: id,
    state: "reachable",
    reason: "",
    host: `${id}-host`,
    consecutive_failures: 0,
    checked_at: NOW.toISOString(),
    last_success_at: NOW.toISOString(),
    updated_at: NOW.toISOString(),
    probing_enabled: true,
    probe_interval_seconds: 60,
    persisted: true,
    ...changes,
  };
}

function CaptureStore() {
  const api = useAppStoreApi();
  useLayoutEffect(() => {
    store = api;
  }, [api]);
  return null;
}

function Providers({ children }: { children: ReactNode }) {
  return (
    <StateProvider>
      <CaptureStore />
      {children}
    </StateProvider>
  );
}

function Reader({ executorId, name = "reader" }: { executorId: string; name?: string }) {
  const result = useSSHReachability(executorId);
  useLayoutEffect(() => {
    snapshots.set(name, result);
  }, [name, result]);
  return null;
}

function current(name = "reader") {
  const snapshot = snapshots.get(name);
  if (!snapshot) throw new Error(`missing reader ${name}`);
  return snapshot;
}

function mountReader(id = A) {
  const tree = (executorId: string) => (
    <Providers>
      <Reader executorId={executorId} />
    </Providers>
  );
  const view = render(tree(id));
  return { ...view, select: (executorId: string) => view.rerender(tree(executorId)) };
}

function mountCard(id = A) {
  const tree = (executorId: string) => (
    <Providers>
      <SSHReachabilityCard executorId={executorId} />
    </Providers>
  );
  const view = render(tree(id));
  return { ...view, select: (executorId: string) => view.rerender(tree(executorId)) };
}

function probeButton() {
  return screen.getByTestId("ssh-reachability-probe-now") as HTMLButtonElement;
}

async function settle(request: Pending, id: string, outcome = "success") {
  await act(async () => {
    if (outcome === "failure") request.reject(new Error("transport failed"));
    else request.resolve(record(id));
  });
}

async function startProbe(name = "reader") {
  await act(async () => {
    void current(name).probeNow();
  });
  return requests.at(-1)!;
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
  requests.length = 0;
  snapshots.clear();
  transport.mockReset();
  transport.mockImplementation(
    (path: string, options?: ApiRequestOptions) =>
      new Promise<SSHReachabilityRecord>((resolve, reject) => {
        requests.push({ path, options, resolve, reject });
      }),
  );
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const request of requests) request.resolve(record(A));
  });
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.13
// @covers AC-EXECUTORS-SSH-REACHABILITY-002.14
// @covers AC-EXECUTORS-SSH-REACHABILITY-002.10
describe("committed reachability visit", () => {
  it("retired initial load cannot mark B not-known", async () => {
    const view = mountCard();
    const a = requests[0];
    view.select(B);
    expect(requests[1].path).toBe("/api/v1/ssh/executors/executor%2Fb/reachability");
    await settle(a, A, "failure");
    expect(screen.queryByTestId(NOT_KNOWN)).toBeNull();
    await settle(requests[1], B);
    expect(screen.getByTestId(HOST).textContent).toContain(`${B}-host`);
  });

  it("B can probe while A is pending", async () => {
    const view = mountCard();
    await settle(requests[0], A);
    fireEvent.click(probeButton());
    const aProbe = requests[1];
    expect(probeButton().disabled).toBe(true);
    view.select(B);
    expect(probeButton().disabled).toBe(false);
    fireEvent.click(probeButton());
    const bProbe = requests[3];
    expect(bProbe.path).toBe("/api/v1/ssh/executors/executor%2Fb/reachability/probe");
    expect(bProbe.options?.init?.method).toBe("POST");
    expect(probeButton().disabled).toBe(true);
    await settle(aProbe, A);
    expect(probeButton().disabled).toBe(true);
    expect(store.getState().sshReachability.byExecutorId[A]).toEqual(record(A));
    await settle(bProbe, B);
    expect(probeButton().disabled).toBe(false);
    expect(screen.getByTestId(HOST).textContent).toContain(`${B}-host`);
    await settle(requests[2], B);
  });

  it("current GET failure still renders not-known", async () => {
    mountCard();
    await settle(requests[0], A, "failure");
    expect(screen.getByTestId(NOT_KNOWN)).toBeTruthy();
    expect(screen.queryByTestId("ssh-reachability-state")).toBeNull();
  });

  it("own probe disables until completion and presents success", async () => {
    mountCard();
    await settle(requests[0], A, "failure");
    fireEvent.click(probeButton());
    expect(probeButton().disabled).toBe(true);
    await settle(requests[1], A);
    expect(probeButton().disabled).toBe(false);
    expect(screen.queryByTestId(NOT_KNOWN)).toBeNull();
    expect(screen.getByTestId("ssh-reachability-state").textContent).toBe("Reachable");
  });
});

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.13
describe("return visits and keyed records", () => {
  it.each(["success", "failure"])("retired A-B-A initial %s is inert", async (outcome) => {
    const view = mountReader();
    const oldA = requests[0];
    view.select(B);
    view.select(A);
    await settle(oldA, A, outcome);
    expect(current().loadError).toBe(false);
    expect(store.getState().sshReachability.byExecutorId).toEqual({});
    await settle(requests[2], A);
    expect(current().record?.host).toBe(`${A}-host`);
    await settle(requests[1], B, "failure");
    expect(current().loadError).toBe(false);
  });

  it.each(["success", "failure"])(
    "retired refresh %s preserves keyed evidence",
    async (outcome) => {
      const view = mountReader();
      await settle(requests[0], A);
      await act(async () => vi.advanceTimersByTime(60_000));
      const refresh = requests[1];
      view.select(B);
      const newerA = record(A, { updated_at: "2026-10-06T20:02:00.000Z", host: "new-A" });
      const resetB = record(B, {
        updated_at: "2026-10-06T20:03:00.000Z",
        checked_at: null,
        state: "unknown",
      });
      act(() => {
        store.getState().setSSHReachability(newerA);
        store.getState().setSSHReachability(resetB);
      });
      await settle(refresh, A, outcome);
      expect(store.getState().sshReachability.byExecutorId).toEqual({ [A]: newerA, [B]: resetB });
      expect(current().loadError).toBe(false);
      await settle(requests[2], B);
      expect(current().record).toEqual(resetB);
    },
  );

  it.each(["success", "failure"])(
    "retired probe %s cannot affect a new A visit",
    async (outcome) => {
      const view = mountReader();
      await settle(requests[0], A);
      const oldProbe = await startProbe();
      view.select(B);
      view.select(A);
      const newProbe = await startProbe();
      await settle(oldProbe, A, outcome);
      expect(current().loadError).toBe(false);
      expect(current().probing).toBe(true);
      await settle(newProbe, A);
      expect(current().probing).toBe(false);
    },
  );
});

function LayoutAttempt({ action }: { action: () => void }) {
  useLayoutEffect(() => action(), [action]);
  return null;
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.4
// @covers AC-EXECUTORS-SSH-REACHABILITY-002.13
describe("reachability clock lifetime", () => {
  it("current fresh card becomes stale only after its deadline despite failed refreshes", async () => {
    mountCard();
    await settle(requests[0], A);
    const accepted = store.getState().sshReachability.byExecutorId[A];
    expect(screen.queryByTestId(STALE)).toBeNull();

    await act(async () => vi.advanceTimersByTime(180_000));
    await act(async () => {
      for (const request of requests.slice(1)) request.reject(new Error("refresh failed"));
    });
    expect(requests).toHaveLength(4);
    expect(store.getState().sshReachability.byExecutorId[A]).toBe(accepted);
    expect(screen.queryByTestId(STALE)).toBeNull();

    await act(async () => vi.advanceTimersByTime(1));
    expect(screen.getByTestId(STALE)).toBeTruthy();
    expect(store.getState().sshReachability.byExecutorId[A]).toBe(accepted);
  });

  it("retired clock callback leaves B fresh until B's own clock callback runs", async () => {
    const timerSpy = vi.spyOn(window, "setTimeout");
    const view = mountCard();
    await settle(requests[0], A);
    const oldClock = timerSpy.mock.calls.find(([, delay]) => delay === 180_001)?.[0] as () => void;
    expect(oldClock).toBeTypeOf("function");

    timerSpy.mockClear();
    view.select(B);
    await settle(requests[1], B);
    const ownClock = timerSpy.mock.calls.find(([, delay]) => delay === 180_001)?.[0] as () => void;
    expect(ownClock).toBeTypeOf("function");
    const accepted = store.getState().sshReachability.byExecutorId[B];
    vi.setSystemTime(new Date(NOW.getTime() + 180_001));

    await act(async () => oldClock());
    expect(screen.queryByTestId(STALE)).toBeNull();
    expect(screen.getByTestId(HOST).textContent).toContain(`${B}-host`);
    expect(store.getState().sshReachability.byExecutorId[B]).toBe(accepted);

    await act(async () => ownClock());
    expect(screen.getByTestId(STALE)).toBeTruthy();
    expect(store.getState().sshReachability.byExecutorId[B]).toBe(accepted);
  });
});

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.13
// @covers AC-EXECUTORS-SSH-REACHABILITY-002.14
describe("admission, disposal and independent owners", () => {
  it("retained load/probe actions are inert after commit cleanup", async () => {
    const intervalSpy = vi.spyOn(window, "setInterval");
    const tree = (id: string, retire?: () => void) => (
      <Providers>
        <Reader executorId={id} />
        {retire && <LayoutAttempt action={retire} />}
      </Providers>
    );
    const view = render(tree(A));
    await settle(requests[0], A);
    const refresh = intervalSpy.mock.calls.find(([, delay]) => delay === 60_000)?.[0] as () => void;
    const probe = current().probeNow;
    expect(refresh).toBeTypeOf("function");
    await act(async () => refresh());
    expect(requests).toHaveLength(2);
    await settle(requests[1], A);
    const retire = () => {
      refresh();
      void probe();
    };
    view.rerender(tree(B, retire));
    expect(requests).toHaveLength(3);
    expect(requests[2].path).toBe("/api/v1/ssh/executors/executor%2Fb/reachability");
    await act(async () => retire());
    expect(requests).toHaveLength(3);
    await settle(requests[2], B);
    expect(current().probing).toBe(false);
  });

  it.each(["success", "failure"])(
    "probe %s after unmount cannot publish or admit more work",
    async (outcome) => {
      const view = mountReader();
      await settle(requests[0], A);
      const pending = await startProbe();
      const retained = current().probeNow;
      const accepted = store.getState().sshReachability.byExecutorId[A];
      view.unmount();
      await act(async () => {
        if (outcome === "failure") pending.reject(new Error("retired"));
        else
          pending.resolve(record(A, { host: "obsolete", updated_at: "2026-10-06T20:01:00.000Z" }));
        void retained();
      });
      expect(requests).toHaveLength(2);
      expect(store.getState().sshReachability.byExecutorId[A]).toEqual(accepted);
    },
  );
});

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.13
describe("remount and sibling owners", () => {
  it("same-executor remount remains independent of the retired instance", async () => {
    const first = mountReader();
    await settle(requests[0], A);
    const oldProbe = await startProbe();
    const retiredStore = store;
    const oldRecord = retiredStore.getState().sshReachability.byExecutorId[A];
    first.unmount();
    mountReader();
    const newStore = store;
    const currentProbe = await startProbe();
    await act(async () => oldProbe.resolve(record(A, { host: "obsolete" })));
    expect(retiredStore.getState().sshReachability.byExecutorId[A]).toEqual(oldRecord);
    expect(newStore.getState().sshReachability.byExecutorId).toEqual({});
    expect(current().probing).toBe(true);
    await settle(currentProbe, A);
    expect(current().probing).toBe(false);
    await settle(requests[2], A);
  });

  it.each(["success", "failure"])(
    "StrictMode retired initial %s cannot settle the replacement",
    async (outcome) => {
      render(
        <StrictMode>
          <Providers>
            <Reader executorId={A} />
          </Providers>
        </StrictMode>,
      );
      expect(requests).toHaveLength(2);
      await settle(requests[0], A, outcome);
      expect(current().loadError).toBe(false);
      expect(store.getState().sshReachability.byExecutorId).toEqual({});
      await settle(requests[1], A);
      expect(current().record?.host).toBe(`${A}-host`);
    },
  );

  it.each([A, B])(
    "independent current owners survive sibling retirement (%s)",
    async (secondId) => {
      const tree = (first: boolean) => (
        <Providers>
          {first && <Reader key="one" executorId={A} name="one" />}
          <Reader key="two" executorId={secondId} name="two" />
        </Providers>
      );
      const view = render(tree(true));
      await settle(requests[0], A);
      await settle(requests[1], secondId);
      const firstProbe = await startProbe("one");
      const secondProbe = await startProbe("two");
      view.rerender(tree(false));
      await settle(firstProbe, A, "failure");
      expect(current("two").loadError).toBe(false);
      expect(current("two").probing).toBe(true);
      await settle(secondProbe, secondId);
      expect(current("two").probing).toBe(false);
    },
  );
});

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.3
// @covers AC-EXECUTORS-SSH-REACHABILITY-002.10
// @covers AC-EXECUTORS-SSH-REACHABILITY-002.14
describe("current visit controls", () => {
  it.each(["success", "failure"])(
    "superseded same-executor refresh %s cannot replace latest outcome",
    async (outcome) => {
      mountReader();
      await settle(requests[0], A);
      await act(async () => vi.advanceTimersByTime(60_000));
      await act(async () => vi.advanceTimersByTime(60_000));
      const accepted = record(A, { host: "newest", updated_at: "2026-10-06T20:02:00.000Z" });
      await act(async () => requests[2].resolve(accepted));
      await settle(requests[1], A, outcome);
      expect(current().loadError).toBe(false);
      expect(current().record).toEqual(accepted);
    },
  );

  it("overlapping own probes keep pending until both admissions settle", async () => {
    mountReader();
    await settle(requests[0], A);
    const first = await startProbe();
    const second = await startProbe();
    await settle(second, A);
    expect(current().probing).toBe(true);
    await settle(first, A);
    expect(current().probing).toBe(false);
  });

  it("current refresh failure retains record; older success clears error without replacing newer reset", async () => {
    mountReader();
    await settle(requests[0], A);
    await act(async () => vi.advanceTimersByTime(60_000));
    await settle(requests[1], A, "failure");
    expect(current().loadError).toBe(true);
    expect(current().record?.state).toBe("reachable");
    const reset = record(A, {
      state: "unknown",
      checked_at: null,
      updated_at: "2026-10-06T20:10:00.000Z",
    });
    act(() => store.getState().setSSHReachability(reset));
    await act(async () => vi.advanceTimersByTime(60_000));
    await settle(requests[2], A);
    expect(current().loadError).toBe(false);
    expect(current().record).toEqual(reset);
  });

  it("current probe failure shows not-known and a cached failure keeps rendered evidence", async () => {
    mountCard();
    await settle(requests[0], A, "failure");
    fireEvent.click(probeButton());
    await settle(requests[1], A, "failure");
    expect(probeButton().disabled).toBe(false);
    expect(screen.getByTestId(NOT_KNOWN)).toBeTruthy();
    act(() => store.getState().setSSHReachability(record(A)));
    fireEvent.click(probeButton());
    await settle(requests[2], A, "failure");
    expect(screen.queryByTestId(NOT_KNOWN)).toBeNull();
    expect(screen.getByTestId(HOST).textContent).toContain(`${A}-host`);
    expect(probeButton().disabled).toBe(false);
  });
});

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.13
it("separate real providers retain independent current owners", async () => {
  const first = render(
    <Providers>
      <Reader executorId={A} name="one" />
    </Providers>,
  );
  const firstStore = store;
  render(
    <Providers>
      <Reader executorId={A} name="two" />
    </Providers>,
  );
  const secondStore = store;
  expect(firstStore).not.toBe(secondStore);
  await settle(requests[0], A);
  const firstProbe = await startProbe("one");
  first.unmount();
  await settle(firstProbe, A, "failure");
  await settle(requests[1], A);
  expect(current("two").loadError).toBe(false);
  const secondProbe = await startProbe("two");
  expect(current("two").probing).toBe(true);
  await settle(secondProbe, A);
  expect(current("two").record?.host).toBe(`${A}-host`);
  expect(current("two").probing).toBe(false);
});

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.13
it.each(["success", "failure"])(
  "pending GET %s after unmount leaves the retired store untouched",
  async (outcome) => {
    const view = mountReader();
    const pending = requests[0];
    view.unmount();
    await settle(pending, A, outcome);
    expect(store.getState().sshReachability.byExecutorId).toEqual({});
  },
);

function ProbeOnCommit() {
  const result = useSSHReachability(A);
  useLayoutEffect(() => {
    snapshots.set("reader", result);
  }, [result]);
  useLayoutEffect(() => {
    void result.probeNow();
  }, [result.probeNow]);
  return null;
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.13
// @covers AC-EXECUTORS-SSH-REACHABILITY-002.14
it("StrictMode retired probe finalizer cannot release the current probe", async () => {
  render(
    <StrictMode>
      <Providers>
        <ProbeOnCommit />
      </Providers>
    </StrictMode>,
  );
  const probes = requests.filter((request) => request.options?.init?.method === "POST");
  expect(probes).toHaveLength(2);
  await settle(probes[0], A, "failure");
  expect(current().loadError).toBe(false);
  expect(current().probing).toBe(true);
  await settle(probes[1], A);
  expect(current().probing).toBe(false);
});
