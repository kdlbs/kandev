import type { StateCreator } from "zustand";
import type { AppState } from "../../app-state-types";
import type { FailedInboxSlice, FailedInboxSliceState, FailedInboxWorkspaceState } from "./types";

export const defaultFailedInboxState: FailedInboxSliceState = {
  failedInbox: {
    byWorkspaceId: {},
    generationByWorkspaceId: {},
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
      const workspace = draft.failedInbox.byWorkspaceId[workspaceId] ?? emptyWorkspaceState();
      // A background refresh of already-`ready` data leaves status alone
      // (AC-UI-INBOX-FAILED-001.16's count "shall remain rendered"): only a
      // never-read or previously-errored workspace moves to `loading`, so a
      // periodic tick, tab change, or foreground-return refresh cannot hide
      // the badge behind rows it is still showing.
      if (workspace.status !== "ready") {
        workspace.status = "loading";
      }
      draft.failedInbox.byWorkspaceId[workspaceId] = workspace;
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
