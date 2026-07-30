import { beforeEach, describe, it, expect } from "vitest";
import { create, type StoreApi, type UseBoundStore } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createUISlice } from "./ui-slice";
import type { UISlice } from "./types";

function makeStore() {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createUISlice as any)(...a) })),
  );
}

type UIStore = UseBoundStore<StoreApi<UISlice>>;

describe("transcript auto-scroll actions", () => {
  let store: UIStore;

  beforeEach(() => {
    store = makeStore();
  });

  it("defaults every session to enabled", () => {
    expect(store.getState().transcriptAutoScroll.enabledBySessionId["session-a"]).toBeUndefined();
  });

  it("setTranscriptAutoScrollEnabled records a per-session preference", () => {
    store.getState().setTranscriptAutoScrollEnabled("session-a", false);
    expect(store.getState().transcriptAutoScroll.enabledBySessionId["session-a"]).toBe(false);

    store.getState().setTranscriptAutoScrollEnabled("session-a", true);
    expect(store.getState().transcriptAutoScroll.enabledBySessionId["session-a"]).toBe(true);
  });

  it("setTranscriptAutoScrollEnabled does not affect other sessions", () => {
    store.getState().setTranscriptAutoScrollEnabled("session-a", false);
    expect(store.getState().transcriptAutoScroll.enabledBySessionId["session-b"]).toBeUndefined();
  });

  it("setTranscriptScrollTop records the last known scrollTop per session", () => {
    store.getState().setTranscriptScrollTop("session-a", 250);
    expect(store.getState().transcriptAutoScroll.scrollTopBySessionId["session-a"]).toBe(250);

    store.getState().setTranscriptScrollTop("session-a", 400);
    expect(store.getState().transcriptAutoScroll.scrollTopBySessionId["session-a"]).toBe(400);
  });

  it("setTranscriptVirtuosoState records an opaque per-session snapshot", () => {
    const snapshot = { ranges: [{ startIndex: 0, endIndex: 10, size: 40 }], scrollTop: 300 };
    store.getState().setTranscriptVirtuosoState("session-a", snapshot);
    expect(store.getState().transcriptAutoScroll.virtuosoStateBySessionId["session-a"]).toEqual(
      snapshot,
    );
  });
});
