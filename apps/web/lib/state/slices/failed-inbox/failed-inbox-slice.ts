import type { StateCreator } from "zustand";
import type { AppState } from "../../app-state-types";
import type { FailedInboxSlice, FailedInboxSliceState, FailedInboxWorkspaceState } from "./types";

export const defaultFailedInboxState: FailedInboxSliceState = {
  failedInbox: {
    byWorkspaceId: {},
    generationByWorkspaceId: {},
    readRevisionByWorkspaceId: {},
  },
};

const emptyWorkspaceState = (readAtWorkspaceRevision = 0): FailedInboxWorkspaceState => ({
  rows: [],
  count: 0,
  truncated: false,
  status: "idle",
  appliedGeneration: 0,
  readAtWorkspaceRevision,
});

// Typed against the full `AppState` (the zustand slices-pattern shape), not
// this slice's own narrower type -- that keeps `set`/`get` structurally
// identical to the root store's, so composing this slice in store.ts needs
// no `as any` escape (ARCH-FRONTEND-ROOT-STATE-CAST).
type ImmerSet = Parameters<
  StateCreator<AppState, [["zustand/immer", never]], [], FailedInboxSlice>
>[0];
type ImmerGet = Parameters<
  StateCreator<AppState, [["zustand/immer", never]], [], FailedInboxSlice>
>[1];

/**
 * Takes `get` (unlike the needs-you-inbox slice) so a genuine workspace
 * switch can be detected even when it happened while nothing in this slice
 * was called to observe it -- see `beginFailedInboxRead`.
 */
export const createFailedInboxSlice = (set: ImmerSet, get: ImmerGet): FailedInboxSlice => ({
  ...defaultFailedInboxState,

  beginFailedInboxRead: (workspaceId) => {
    // Read from `workspaces.activeIdRevision`, the app-wide source of truth
    // bumped on every actual workspace switch, rather than a copy local to
    // this slice: this slice's own actions only ever run while the Inbox
    // page is mounted, so a locally-shadowed "last workspace seen" value
    // cannot detect a switch that happened while the Inbox was closed.
    const currentRevision = get().workspaces.activeIdRevision ?? 0;
    let generation = 0;
    set((draft) => {
      const next = (draft.failedInbox.generationByWorkspaceId[workspaceId] ?? 0) + 1;
      draft.failedInbox.generationByWorkspaceId[workspaceId] = next;
      draft.failedInbox.readRevisionByWorkspaceId[workspaceId] = currentRevision;
      const existing = draft.failedInbox.byWorkspaceId[workspaceId] ?? emptyWorkspaceState();
      const isStaleWorkspaceRevision = existing.readAtWorkspaceRevision !== currentRevision;
      // A same-workspace refresh trigger (periodic tick, tab change,
      // foreground return) leaves already-`ready` data alone, so it cannot
      // hide the badge behind rows it is still showing. Any read whose
      // workspace revision has moved on always resets to `loading` and
      // clears any cache from an earlier visit, even if that visit ended
      // `ready` -- whether the switch happened while this workspace's tab
      // was open or while the Inbox was closed entirely.
      if (isStaleWorkspaceRevision || existing.status !== "ready") {
        draft.failedInbox.byWorkspaceId[workspaceId] = {
          ...emptyWorkspaceState(currentRevision),
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
      const requestRevision = draft.failedInbox.readRevisionByWorkspaceId[workspaceId] ?? 0;
      const activeWorkspaceId = get().workspaces.activeId;
      const activeRevision = get().workspaces.activeIdRevision ?? 0;
      if (
        (activeWorkspaceId !== null && activeWorkspaceId !== workspaceId) ||
        activeRevision !== requestRevision
      ) {
        return;
      }
      draft.failedInbox.byWorkspaceId[workspaceId] = {
        rows: page.rows,
        count: page.count,
        truncated: page.truncated,
        status: "ready",
        appliedGeneration: generation,
        readAtWorkspaceRevision: requestRevision,
      };
    }),

  setFailedInboxError: (workspaceId, generation) =>
    set((draft) => {
      if (draft.failedInbox.generationByWorkspaceId[workspaceId] !== generation) return;
      const requestRevision = draft.failedInbox.readRevisionByWorkspaceId[workspaceId] ?? 0;
      const activeWorkspaceId = get().workspaces.activeId;
      const activeRevision = get().workspaces.activeIdRevision ?? 0;
      if (
        (activeWorkspaceId !== null && activeWorkspaceId !== workspaceId) ||
        activeRevision !== requestRevision
      ) {
        return;
      }
      // A failed read clears the rows it was replacing in the SAME update
      // (design-01#Failure-and-recovery) -- stale rows over an absent count
      // is a disagreement the count and the list must never show.
      draft.failedInbox.byWorkspaceId[workspaceId] = {
        ...emptyWorkspaceState(
          draft.failedInbox.byWorkspaceId[workspaceId]?.readAtWorkspaceRevision ?? 0,
        ),
        status: "error",
        appliedGeneration: generation,
      };
    }),
});
