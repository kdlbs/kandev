import { beforeEach, describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";
import type { WorkloadRunObservation, WorkloadCapabilities } from "@/lib/types/background-work";
import { canRunWorkloadAction } from "@/lib/background-work/capabilities";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

const SESSION_ID = "session-1";

function declaredCapabilities(): WorkloadCapabilities {
  return {
    discovery: "snapshot",
    output: "stream",
    transcript: false,
    parentage: false,
    reasoning_summary: false,
    attributable_usage: false,
    actions: {
      stop: { supported: true, available: true },
      interrupt: { supported: true, available: true },
      write_input: { supported: true, available: true },
    },
  };
}

function noDeclaration(): WorkloadCapabilities {
  return {
    discovery: "",
    output: "",
    transcript: false,
    parentage: false,
    reasoning_summary: false,
    attributable_usage: false,
  };
}

function observation(overrides: Partial<WorkloadRunObservation> = {}): WorkloadRunObservation {
  return {
    work_id: "work-1",
    title: "CI poller",
    kind: "shell",
    state: "running",
    capabilities: declaredCapabilities(),
    revision: 1,
    ...overrides,
  };
}

describe("background workload store merges", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("applies a committed unknown transition with its durable revision", () => {
    store.getState().setBackgroundWorkloads(SESSION_ID, [observation()]);

    store.getState().updateBackgroundWorkload(
      SESSION_ID,
      observation({
        state: "unknown",
        revision: 3,
        capabilities: noDeclaration(),
      }),
    );

    const [workload] = store.getState().backgroundWork.workloadsBySessionId[SESSION_ID]!;
    expect(workload!.state).toBe("unknown");
    expect(workload!.revision).toBe(3);
  });

  it("keeps static support but clears availability when a committed projection omits capabilities", () => {
    store.getState().setBackgroundWorkloads(SESSION_ID, [observation()]);

    store.getState().updateBackgroundWorkload(
      SESSION_ID,
      observation({
        state: "completed",
        revision: 2,
        exit_code: 0,
        capabilities: noDeclaration(),
      }),
    );

    const [workload] = store.getState().backgroundWork.workloadsBySessionId[SESSION_ID]!;
    expect(workload!.state).toBe("completed");
    expect(workload!.exit_code).toBe(0);
    expect(workload!.capabilities.actions?.stop).toEqual({ supported: true, available: false });
    expect(canRunWorkloadAction(workload!, "stop")).toBe(false);
    expect(canRunWorkloadAction(workload!, "interrupt")).toBe(false);
    expect(canRunWorkloadAction(workload!, "write_input")).toBe(false);
  });

  it.each(["completed", "failed", "ended", "unknown"] as const)(
    "renders no actionable controls after a capability-less %s projection",
    (state) => {
      store.getState().setBackgroundWorkloads(SESSION_ID, [observation()]);

      store
        .getState()
        .updateBackgroundWorkload(
          SESSION_ID,
          observation({ state, revision: 5, capabilities: noDeclaration() }),
        );

      const [workload] = store.getState().backgroundWork.workloadsBySessionId[SESSION_ID]!;
      expect(workload!.state).toBe(state);
      expect(canRunWorkloadAction(workload!, "stop")).toBe(false);
      expect(canRunWorkloadAction(workload!, "interrupt")).toBe(false);
      expect(canRunWorkloadAction(workload!, "write_input")).toBe(false);
    },
  );

  it("does not reopen a terminal workload with a stale running observation", () => {
    store
      .getState()
      .setBackgroundWorkloads(SESSION_ID, [
        observation({ state: "completed", revision: 4, exit_code: 0 }),
      ]);

    store
      .getState()
      .updateBackgroundWorkload(SESSION_ID, observation({ state: "running", revision: 5 }));

    const [workload] = store.getState().backgroundWork.workloadsBySessionId[SESSION_ID]!;
    expect(workload!.state).toBe("completed");
    expect(workload!.exit_code).toBe(0);
  });
});

describe("background workload capability declarations", () => {
  it("uses a declared capability for a running workload (live Codex child)", () => {
    const workload = observation({ kind: "subagent" });
    expect(canRunWorkloadAction(workload, "stop")).toBe(true);
    expect(canRunWorkloadAction(workload, "interrupt")).toBe(true);
  });

  it("never infers an action from a missing capability map", () => {
    const workload = observation({ capabilities: noDeclaration() });
    expect(canRunWorkloadAction(workload, "stop")).toBe(false);
  });

  it("never renders a control for unknown activity even when a stale declaration says available", () => {
    const workload = observation({ state: "unknown" });
    expect(canRunWorkloadAction(workload, "stop")).toBe(false);
  });
});
