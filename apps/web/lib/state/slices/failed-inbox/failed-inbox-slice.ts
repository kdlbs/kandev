import type { StateCreator } from "zustand";
import type { AppState } from "../../app-state-types";
import type { FailedInboxSlice, FailedInboxSliceState, FailedInboxWorkspaceState } from "./types";

export const defaultFailedInboxState: FailedInboxSliceState = {
  failedInbox: {
    byWorkspaceId: {},
    generationByWorkspaceId: {},
    activeWorkspaceId: null,
  },
};

const emptyWorkspaceState = (): FailedInboxWorkspaceState => ({
  rows: [],
  count: 0,
  truncated: false,
  status: "idle",
  appliedGeneration: 0,
});

// Typed against the full `AppState` (the zustand slices-pattern shape), not
// this slice's own narrower type -- that keeps `set` structurally identical
// to the root store's `set`, so composing this slice in store.ts needs no
// `as any` escape (ARCH-FRONTEND-ROOT-STATE-CAST).
type ImmerSet = Parameters<
  StateCreator<AppState, [["zustand/immer", never]], [], FailedInboxSlice>
>[0];

/**
 * Typed as a plain factory over `set` (mirrors the needs-you-inbox slice):
 * every action here only needs to write, never read prior state through
 * `get`.
 */
export const createFailedInboxSlice = (set: ImmerSet): FailedInboxSlice => ({
  ...defaultFailedInboxState,

  beginFailedInboxRead: (workspaceId) => {
    let generation = 0;
    set((draft) => {
      const next = (draft.failedInbox.generationByWorkspaceId[workspaceId] ?? 0) + 1;
      draft.failedInbox.generationByWorkspaceId[workspaceId] = next;
      const isWorkspaceChange = draft.failedInbox.activeWorkspaceId !== workspaceId;
      draft.failedInbox.activeWorkspaceId = workspaceId;
      const existing = draft.failedInbox.byWorkspaceId[workspaceId] ?? emptyWorkspaceState();
      // A same-workspace refresh trigger (periodic tick, tab change,
      // foreground return) leaves already-`ready` data alone, so it cannot
      // hide the badge behind rows it is still showing. A genuine workspace
      // switch always resets to `loading` and clears any cache from an
      // earlier visit in this session, even if that visit ended `ready`.
      if (isWorkspaceChange || existing.status !== "ready") {
        draft.failedInbox.byWorkspaceId[workspaceId] = {
          ...emptyWorkspaceState(),
          status: "loading",
        };
      }
      generation = next;
    });
    return generation;
  },

  setFailedInboxPage: (workspaceId, generation, page) =>
    set((draft) => {
      if (draft.failedInbox.generationByWorkspaceId[workspaceId] !== generation) return;
      draft.failedInbox.byWorkspaceId[workspaceId] = {
        rows: page.rows,
        count: page.count,
        truncated: page.truncated,
        status: "ready",
        appliedGeneration: generation,
      };
    }),

  setFailedInboxError: (workspaceId, generation) =>
    set((draft) => {
      if (draft.failedInbox.generationByWorkspaceId[workspaceId] !== generation) return;
      // A failed read clears the rows it was replacing in the SAME update
      // (design-01#Failure-and-recovery) -- stale rows over an absent count
      // is a disagreement the count and the list must never show.
      draft.failedInbox.byWorkspaceId[workspaceId] = {
        ...emptyWorkspaceState(),
        status: "error",
        appliedGeneration: generation,
      };
    }),
});
