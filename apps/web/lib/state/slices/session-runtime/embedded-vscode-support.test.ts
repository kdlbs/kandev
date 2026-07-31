import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice, purgeSessionRuntimeState } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

describe("embeddedVscodeSupport", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("defaults to no recorded support so consumers fail closed", () => {
    expect(store.getState().embeddedVscodeSupport.bySessionId["session-1"]).toBeUndefined();
  });

  it("records support per session", () => {
    store.getState().setEmbeddedVscodeSupport("session-1", true);
    store.getState().setEmbeddedVscodeSupport("session-2", false);

    expect(store.getState().embeddedVscodeSupport.bySessionId["session-1"]).toBe(true);
    expect(store.getState().embeddedVscodeSupport.bySessionId["session-2"]).toBe(false);
  });

  it("replaces a stale value when the session's executor capability changes", () => {
    store.getState().setEmbeddedVscodeSupport("session-1", true);
    store.getState().setEmbeddedVscodeSupport("session-1", false);

    expect(store.getState().embeddedVscodeSupport.bySessionId["session-1"]).toBe(false);
  });

  it("drops the entry when the session's runtime state is purged", () => {
    store.getState().setEmbeddedVscodeSupport("session-1", true);
    store.getState().setEmbeddedVscodeSupport("session-2", true);

    store.setState((draft) => {
      purgeSessionRuntimeState(draft, "session-1");
    });

    expect(store.getState().embeddedVscodeSupport.bySessionId["session-1"]).toBeUndefined();
    expect(store.getState().embeddedVscodeSupport.bySessionId["session-2"]).toBe(true);
  });
});
