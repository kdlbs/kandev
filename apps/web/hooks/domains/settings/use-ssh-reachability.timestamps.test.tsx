import { act, cleanup, render, screen } from "@testing-library/react";
import { useLayoutEffect } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { SSHReachabilityCard } from "@/components/settings/ssh-reachability-card";
import type { AppState } from "@/lib/state/store";
import type { SSHReachabilityRecord } from "@/lib/types/http-ssh";
import type { StoreApi } from "zustand";

const transport = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: transport,
}));

const A = "executor/a";
const STALE = "ssh-reachability-stale";
const HOST = "ssh-reachability-host";
const AGE = "ssh-reachability-age";
const LAST_SUCCESS = "ssh-reachability-last-success";
const NO_PROBE = "Not probed yet.";
const NO_SUCCESS = "No successful probe recorded.";
const NOW = new Date("2026-10-06T20:00:00.000Z");
const requests: { resolve: (value: SSHReachabilityRecord) => void }[] = [];
let store: StoreApi<AppState>;

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

function mountCard() {
  return render(
    <StateProvider>
      <CaptureStore />
      <SSHReachabilityCard executorId={A} />
    </StateProvider>,
  );
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
  requests.length = 0;
  transport.mockReset();
  transport.mockImplementation(
    () =>
      new Promise<SSHReachabilityRecord>((resolve) => {
        requests.push({ resolve });
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

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.4
// @covers AC-EXECUTORS-SSH-REACHABILITY-002.15
describe("SSH reachability wire timestamps", () => {
  it.each(["0", "2026-02-30T20:00:00Z"])(
    "malformed %s neither arms a stale clock nor presents a meaningful age",
    async (timestamp) => {
      const timerSpy = vi.spyOn(window, "setTimeout");
      mountCard();
      await act(async () => {
        requests[0].resolve(record(A, { checked_at: timestamp, last_success_at: timestamp }));
      });
      expect.soft(timerSpy.mock.calls.filter(([, delay]) => delay === 0)).toHaveLength(0);
      expect.soft(screen.queryByTestId(STALE)).toBeNull();
      expect.soft(screen.getByTestId(AGE).textContent).toBe(NO_PROBE);
      expect.soft(screen.getByTestId(LAST_SUCCESS).textContent).toBe(NO_SUCCESS);
      expect(currentRecord().checked_at).toBe(timestamp);
    },
  );

  it.each([
    { timestamp: NOW.toISOString(), checkedMs: NOW.getTime() },
    { timestamp: "2026-10-06T20:00:00.123456789Z", checkedMs: NOW.getTime() + 123 },
    { timestamp: "2026-10-06T22:00:00.123456789+02:00", checkedMs: NOW.getTime() + 123 },
    { timestamp: "1969-12-31T23:59:59.999999999Z", checkedMs: -1 },
  ])(
    "valid $timestamp retains the millisecond stale boundary",
    async ({ timestamp, checkedMs }) => {
      vi.setSystemTime(new Date(checkedMs));
      mountCard();
      await act(async () => {
        requests[0].resolve(record(A, { checked_at: timestamp, last_success_at: timestamp }));
      });
      expect(screen.getByTestId(AGE).textContent).toContain("Last probe:");
      expect(screen.getByTestId(LAST_SUCCESS).textContent).toContain("Last success:");
      await act(async () => vi.advanceTimersByTime(180_000));
      expect(screen.queryByTestId(STALE)).toBeNull();
      await act(async () => vi.advanceTimersByTime(1));
      expect(screen.getByTestId(STALE)).toBeTruthy();
      expect(currentRecord().checked_at).toBe(timestamp);
    },
  );
});

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.15
describe("independent reachability timestamp fields", () => {
  it("invalid success age preserves a valid completed-probe stale clock", async () => {
    mountCard();
    await act(async () => requests[0].resolve(record(A, { last_success_at: "0" })));
    expect(screen.getByTestId(AGE).textContent).toContain("Last probe:");
    expect(screen.getByTestId(LAST_SUCCESS).textContent).toBe(NO_SUCCESS);
    await act(async () => vi.advanceTimersByTime(180_001));
    expect(screen.getByTestId(STALE)).toBeTruthy();
    expect(screen.getByTestId(HOST).textContent).toContain(`${A}-host`);
  });

  it("invalid completion age preserves a valid last-success age", async () => {
    mountCard();
    await act(async () => requests[0].resolve(record(A, { checked_at: "2026-02-30T20:00:00Z" })));
    expect(screen.getByTestId(AGE).textContent).toBe(NO_PROBE);
    expect(screen.getByTestId(LAST_SUCCESS).textContent).toContain("Last success:");
    await act(async () => vi.advanceTimersByTime(180_001));
    expect(screen.queryByTestId(STALE)).toBeNull();
    expect(screen.getByTestId(HOST).textContent).toContain(`${A}-host`);
  });
});

function currentRecord() {
  return store.getState().sshReachability.byExecutorId[A];
}
