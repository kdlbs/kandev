import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { describe, expect, it, vi } from "vitest";
import { createSessionRuntimeSlice, purgeSessionRuntimeState } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

const SESSION = "session-1";
const FIRST_TURN = "turn-1";
const NEXT_TURN = "turn-2";
const FIRST_CHANGE = "2026-11-21T10:00:00.000Z";
const LATER_CHANGE = "2026-11-21T10:03:00.000Z";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

const model = { configId: "model", kind: "changed", label: "model", value: "Sonnet" } as const;
const mode = { configId: "mode", kind: "changed", label: "mode", value: "Plan" } as const;
const reasoning = {
  configId: "effort",
  kind: "changed",
  label: "reasoning",
  value: "High",
} as const;

describe("session configuration feedback", () => {
  it("merges changes by setting across pauses and preserves the row's position", () => {
    const store = makeStore();
    const record = store.getState().recordSessionConfigurationFeedback;
    record(SESSION, { turnId: FIRST_TURN, createdAt: FIRST_CHANGE, entries: [model] });
    const initialId = store.getState().configurationFeedback.bySessionId[SESSION][0].id;
    record(SESSION, { turnId: FIRST_TURN, createdAt: FIRST_CHANGE, entries: [mode] });
    record(SESSION, { turnId: FIRST_TURN, createdAt: LATER_CHANGE, entries: [reasoning] });
    const updatedModel = { ...model, value: "Opus" };
    record(SESSION, { turnId: FIRST_TURN, createdAt: LATER_CHANGE, entries: [updatedModel] });

    expect(store.getState().configurationFeedback.bySessionId[SESSION]).toEqual([
      {
        id: initialId,
        turnId: FIRST_TURN,
        createdAt: FIRST_CHANGE,
        entries: [updatedModel, mode, reasoning],
        retry: undefined,
      },
    ]);
  });

  it("routes a late reply to its original group without changing the next group or another session", () => {
    const store = makeStore();
    const record = store.getState().recordSessionConfigurationFeedback;
    record(SESSION, { turnId: FIRST_TURN, createdAt: FIRST_CHANGE, entries: [model] });
    record(SESSION, { turnId: NEXT_TURN, createdAt: LATER_CHANGE, entries: [reasoning] });
    record("other-session", { turnId: FIRST_TURN, createdAt: FIRST_CHANGE, entries: [mode] });
    record(SESSION, { turnId: FIRST_TURN, createdAt: FIRST_CHANGE, entries: [mode] });

    expect(
      store.getState().configurationFeedback.bySessionId[SESSION].map((row) => row.entries),
    ).toEqual([[model, mode], [reasoning]]);
    expect(store.getState().configurationFeedback.bySessionId["other-session"][0].entries).toEqual([
      mode,
    ]);
  });

  it("clears a superseded retry while keeping the group and purges rows on session removal", () => {
    const store = makeStore();
    const record = store.getState().recordSessionConfigurationFeedback;
    const retry = { run: vi.fn(), executionId: "execution-1" };
    record(SESSION, { turnId: null, createdAt: FIRST_CHANGE, entries: [model], retry });
    record(SESSION, { turnId: null, createdAt: FIRST_CHANGE, entries: [mode] });
    expect(store.getState().configurationFeedback.bySessionId[SESSION][0].retry).toEqual(retry);

    record(SESSION, { turnId: null, createdAt: LATER_CHANGE, entries: [], retry: null });
    expect(store.getState().configurationFeedback.bySessionId[SESSION][0]).toMatchObject({
      entries: [model, mode],
      retry: undefined,
    });
    store.setState((draft) => {
      purgeSessionRuntimeState(draft, SESSION);
    });
    expect(store.getState().configurationFeedback.bySessionId[SESSION]).toBeUndefined();
  });
});
