import { describe, expect, it } from "vitest";
import { defaultState, mergeInitialState } from "./default-state";
import type { HydrationState } from "./store";

describe("turn hydration state", () => {
  it("defaults and deep-merges loaded session markers", () => {
    const state = mergeInitialState({
      turns: {
        bySession: { "session-1": [] },
        activeBySession: {},
        loadedBySession: {},
      },
    } as unknown as HydrationState);

    expect(defaultState.turns.loadedBySession).toEqual({});
    expect(state.turns.loadedBySession).toEqual({ "session-1": true });
  });
});
