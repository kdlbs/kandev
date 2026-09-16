import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { SSHReachabilityRecord } from "@/lib/types/http-ssh";

const getSSHExecutorReachability = vi.fn();
const probeSSHExecutorReachability = vi.fn();

vi.mock("@/lib/api/domains/ssh-api", () => ({
  getSSHExecutorReachability: (...args: unknown[]) => getSSHExecutorReachability(...args),
  probeSSHExecutorReachability: (...args: unknown[]) => probeSSHExecutorReachability(...args),
}));

import { SSHReachabilityCard, isReachabilityStale } from "./ssh-reachability-card";

const EXECUTOR_ID = "executor-1";
const BASE_TIMESTAMP = "2026-09-17T00:00:00.000Z";
const STATE_TESTID = "ssh-reachability-state";

function record(overrides: Partial<SSHReachabilityRecord> = {}): SSHReachabilityRecord {
  return {
    executor_id: EXECUTOR_ID,
    state: "reachable",
    reason: "",
    consecutive_failures: 0,
    host: "10.0.0.5",
    checked_at: BASE_TIMESTAMP,
    last_success_at: BASE_TIMESTAMP,
    updated_at: BASE_TIMESTAMP,
    probing_enabled: true,
    probe_interval_seconds: 60,
    persisted: true,
    ...overrides,
  };
}

function renderCard(executorId = EXECUTOR_ID) {
  return render(
    <StateProvider>
      <SSHReachabilityCard executorId={executorId} />
    </StateProvider>,
  );
}

function PushReachability({ record: pushed }: { record: SSHReachabilityRecord }) {
  const storeApi = useAppStoreApi();
  return (
    <button type="button" onClick={() => storeApi.getState().setSSHReachability(pushed)}>
      push
    </button>
  );
}

function renderCardWithPush(pushed: SSHReachabilityRecord) {
  return render(
    <StateProvider>
      <SSHReachabilityCard executorId={EXECUTOR_ID} />
      <PushReachability record={pushed} />
    </StateProvider>,
  );
}

async function waitForState(text: string) {
  await waitFor(() => expect(screen.getByTestId(STATE_TESTID).textContent).toBe(text));
}

beforeEach(() => {
  getSSHExecutorReachability.mockReset();
  probeSSHExecutorReachability.mockReset();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("isReachabilityStale", () => {
  it("is false when there is no probing cadence or no completed probe", () => {
    expect(isReachabilityStale(null, 60, true, Date.now())).toBe(false);
    expect(isReachabilityStale(BASE_TIMESTAMP, 60, false, Date.now())).toBe(false);
  });

  it("is false within three intervals and true just past it", () => {
    const checkedMs = Date.parse(BASE_TIMESTAMP);
    expect(isReachabilityStale(BASE_TIMESTAMP, 60, true, checkedMs + 3 * 60_000)).toBe(false);
    expect(isReachabilityStale(BASE_TIMESTAMP, 60, true, checkedMs + 3 * 60_000 + 1)).toBe(true);
  });
});

describe("SSHReachabilityCard rendering", () => {
  it("renders state, host, failure count and age for a reachable record", async () => {
    getSSHExecutorReachability.mockResolvedValue(record());
    renderCard();
    await waitFor(() => expect(screen.getByTestId(STATE_TESTID)).toBeTruthy());
    expect(screen.getByTestId("ssh-reachability-host").textContent).toContain("10.0.0.5");
    expect(screen.queryByTestId("ssh-reachability-reason")).toBeNull();
  });

  it("renders reason and message for an unreachable record", async () => {
    getSSHExecutorReachability.mockResolvedValue(
      record({
        state: "unreachable",
        reason: "timeout",
        message: "dial tcp: i/o timeout",
        consecutive_failures: 3,
      }),
    );
    renderCard();
    await waitFor(() => expect(screen.getByTestId("ssh-reachability-reason")).toBeTruthy());
    expect(screen.getByTestId("ssh-reachability-message").textContent).toBe(
      "dial tcp: i/o timeout",
    );
    expect(screen.getByTestId("ssh-reachability-failures").textContent).toContain("3");
  });

  it("marks a record older than three intervals as stale", async () => {
    const old = new Date(Date.now() - 4 * 60_000).toISOString();
    getSSHExecutorReachability.mockResolvedValue(
      record({ probe_interval_seconds: 60, checked_at: old }),
    );
    renderCard();
    await waitFor(() => expect(screen.getByTestId("ssh-reachability-stale")).toBeTruthy());
  });

  it("does not mark a fresh record as stale", async () => {
    getSSHExecutorReachability.mockResolvedValue(record({ probe_interval_seconds: 60 }));
    renderCard();
    await waitFor(() => expect(screen.getByTestId(STATE_TESTID)).toBeTruthy());
    expect(screen.queryByTestId("ssh-reachability-stale")).toBeNull();
  });
});

describe("SSHReachabilityCard live updates", () => {
  it("applies a pushed executor.reachability.changed record live, without a reload", async () => {
    getSSHExecutorReachability.mockResolvedValue(record({ state: "reachable" }));
    renderCardWithPush(record({ state: "unreachable", reason: "auth", message: "boom" }));
    await waitForState("Reachable");
    fireEvent.click(screen.getByRole("button", { name: "push" }));
    await waitForState("Unreachable");
    expect(getSSHExecutorReachability).toHaveBeenCalledTimes(1);
  });

  it("replaces a stored record with a reset (null checked_at, newer updated_at)", async () => {
    getSSHExecutorReachability.mockResolvedValue(
      record({ state: "unreachable", reason: "timeout", updated_at: BASE_TIMESTAMP }),
    );
    renderCardWithPush(
      record({
        state: "unknown",
        reason: "",
        checked_at: null,
        last_success_at: null,
        updated_at: "2026-09-17T00:01:00.000Z",
      }),
    );
    await waitForState("Unreachable");
    fireEvent.click(screen.getByRole("button", { name: "push" }));
    await waitForState("Unknown");
  });
});

describe("SSHReachabilityCard refresh cadence", () => {
  it("says periodic probing is off and starts no refresh timer when probing_enabled is false", async () => {
    const setIntervalSpy = vi.spyOn(window, "setInterval");
    getSSHExecutorReachability.mockResolvedValue(
      record({ probing_enabled: false, probe_interval_seconds: 0 }),
    );
    renderCard();
    await waitFor(() => expect(screen.getByTestId("ssh-reachability-probing-off")).toBeTruthy());
    // Exclude testing-library's own internal waitFor polling interval (50ms);
    // this asserts the card itself never armed a refresh timer.
    const ownIntervalCalls = setIntervalSpy.mock.calls.filter((call) => call[1] !== 50);
    expect(ownIntervalCalls).toHaveLength(0);
    expect(screen.queryByTestId(STATE_TESTID)?.textContent).not.toBe("Unreachable");
  });

  it("starts a refresh timer at the configured probe interval when probing is enabled", async () => {
    const setIntervalSpy = vi.spyOn(window, "setInterval");
    getSSHExecutorReachability.mockResolvedValue(record({ probe_interval_seconds: 45 }));
    renderCard();
    await waitFor(() => expect(screen.getByTestId(STATE_TESTID)).toBeTruthy());
    expect(setIntervalSpy).toHaveBeenCalledWith(expect.any(Function), 45_000);
  });
});

describe("SSHReachabilityCard errors and probe-now", () => {
  it("reports reachability as not known, never unreachable, when the initial load fails", async () => {
    getSSHExecutorReachability.mockRejectedValue(new Error("network error"));
    renderCard();
    await waitFor(() => expect(screen.getByTestId("ssh-reachability-not-known")).toBeTruthy());
    expect(screen.queryByTestId(STATE_TESTID)).toBeNull();
  });

  it("runs an immediate probe and applies the result", async () => {
    getSSHExecutorReachability.mockResolvedValue(
      record({ state: "unreachable", reason: "network" }),
    );
    probeSSHExecutorReachability.mockResolvedValue(record({ state: "reachable" }));
    renderCard();
    await waitForState("Unreachable");
    fireEvent.click(screen.getByTestId("ssh-reachability-probe-now"));
    await waitForState("Reachable");
    expect(probeSSHExecutorReachability).toHaveBeenCalledWith(EXECUTOR_ID);
  });
});
