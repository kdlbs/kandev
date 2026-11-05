import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { createSessionRuntimeSlice, purgeSessionRuntimeState } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

const CAP = 200 * 1024;
const work: WorkloadRunObservation = {
  work_id: "one",
  run_id: "run-1",
  title: "one",
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
function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

describe("background output window bookkeeping", () => {
  it("counts actual evictions in JavaScript code units across capped appends", () => {
    const store = makeStore();
    store.getState().setBackgroundWorkloads("s", [work]);
    const initial = store.getState().backgroundWork.outputWindowsBySessionId.s!.one!;
    store
      .getState()
      .appendBackgroundWorkloadOutput("s", { work_id: "one", chunk: "é😀", offset: 900 });
    expect(store.getState().backgroundWork.outputWindowsBySessionId.s!.one!.evicted).toBe(0);
    store.getState().appendBackgroundWorkloadOutput("s", {
      work_id: "one",
      chunk: "x".repeat(CAP + 7),
      offset: 999,
    });
    const state = store.getState().backgroundWork;
    expect(state.workloadsBySessionId.s![0]!.output).toHaveLength(CAP);
    expect(state.workloadsBySessionId.s![0]!.output_truncated).toBe(true);
    expect(state.outputWindowsBySessionId.s!.one).toMatchObject({
      generation: initial.generation,
      evicted: 10,
    });
  });

  it("preserves identical snapshots and status updates, but replaces changed output or run", () => {
    const store = makeStore();
    const s = store.getState();
    s.setBackgroundWorkloads("s", [{ ...work, output: "before" }]);
    s.appendBackgroundWorkloadOutput("s", { work_id: "one", chunk: "x".repeat(CAP), offset: 0 });
    const existing = store.getState().backgroundWork.workloadsBySessionId.s![0]!;
    const window = store.getState().backgroundWork.outputWindowsBySessionId.s!.one!;
    s.setBackgroundWorkloads("s", [{ ...existing }]);
    s.updateBackgroundWorkload("s", { ...work, revision: 2, state: "waiting" });
    expect(store.getState().backgroundWork.outputWindowsBySessionId.s!.one).toEqual(window);
    s.updateBackgroundWorkload("s", { ...work, revision: 1, output: "rejected" });
    expect(store.getState().backgroundWork.outputWindowsBySessionId.s!.one).toEqual(window);
    s.updateBackgroundWorkload("s", { ...work, revision: 3, output: "replacement" });
    const replacement = store.getState().backgroundWork.outputWindowsBySessionId.s!.one!;
    expect(replacement.generation).not.toBe(window.generation);
    expect(replacement.evicted).toBe(0);
    s.setBackgroundWorkloads("s", [
      { ...work, revision: 4, run_id: "run-2", output: "replacement" },
    ]);
    expect(store.getState().backgroundWork.outputWindowsBySessionId.s!.one!.generation).not.toBe(
      replacement.generation,
    );
  });

  it("removes sidecars on list removal, clear, and purge without affecting another session", () => {
    const store = makeStore();
    const s = store.getState();
    s.setBackgroundWorkloads("s", [work]);
    s.setBackgroundWorkloads("other", [work]);
    const generation = store.getState().backgroundWork.outputWindowsBySessionId.s!.one!.generation;
    const other = store.getState().backgroundWork.outputWindowsBySessionId.other!.one!;
    s.setBackgroundWorkloads("s", []);
    expect(store.getState().backgroundWork.outputWindowsBySessionId.s?.one).toBeUndefined();
    s.setBackgroundWorkloads("s", [work]);
    expect(store.getState().backgroundWork.outputWindowsBySessionId.s!.one!.generation).not.toBe(
      generation,
    );
    s.clearBackgroundWork("s");
    expect(store.getState().backgroundWork.outputWindowsBySessionId.s).toBeUndefined();
    s.setBackgroundWorkloads("s", [work]);
    store.setState((draft) => {
      purgeSessionRuntimeState(draft, "s");
    });
    expect(store.getState().backgroundWork.outputWindowsBySessionId.s).toBeUndefined();
    expect(store.getState().backgroundWork.outputWindowsBySessionId.other!.one).toEqual(other);
  });
});
