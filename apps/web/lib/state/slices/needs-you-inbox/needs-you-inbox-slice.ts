import type { StateCreator } from "zustand";
import type { AppState } from "../../app-state-types";
import type {
  NeedsYouInboxSlice,
  NeedsYouInboxSliceState,
  NeedsYouInboxWorkspaceState,
} from "./types";

export const defaultNeedsYouInboxState: NeedsYouInboxSliceState = {
  needsYouInbox: {
    byWorkspaceId: {},
    generationByWorkspaceId: {},
    refreshTick: 0,
  },
};

const emptyWorkspaceState = (): NeedsYouInboxWorkspaceState => ({
  bundles: [],
  count: 0,
  hiddenCount: 0,
  nextSnoozeExpiry: null,
  hasMore: false,
  status: "idle",
  appliedGeneration: 0,
});

// Typed against the full `AppState` (the zustand slices-pattern shape), not
// this slice's own narrower type -- that keeps `set` structurally identical
// to the root store's `set`, so composing this slice in store.ts needs no
// `as any` escape (ARCH-FRONTEND-ROOT-STATE-CAST).
type ImmerSet = Parameters<
  StateCreator<AppState, [["zustand/immer", never]], [], NeedsYouInboxSlice>
>[0];

/**
 * Typed as a plain factory over `set` (mirrors the review slice): every
 * action here only needs to write, never read prior state through `get`.
 */
export const createNeedsYouInboxSlice = (set: ImmerSet): NeedsYouInboxSlice => ({
  ...defaultNeedsYouInboxState,

  beginNeedsYouInboxRead: (workspaceId) => {
    let generation = 0;
    set((draft) => {
      const next = (draft.needsYouInbox.generationByWorkspaceId[workspaceId] ?? 0) + 1;
      draft.needsYouInbox.generationByWorkspaceId[workspaceId] = next;
      const workspace = draft.needsYouInbox.byWorkspaceId[workspaceId] ?? emptyWorkspaceState();
      workspace.status = "loading";
      draft.needsYouInbox.byWorkspaceId[workspaceId] = workspace;
      generation = next;
    });
    return generation;
  },

  setNeedsYouInboxPage: (workspaceId, generation, page) =>
    set((draft) => {
      if (draft.needsYouInbox.generationByWorkspaceId[workspaceId] !== generation) return;
      draft.needsYouInbox.byWorkspaceId[workspaceId] = {
        bundles: page.bundles,
        count: page.count,
        hiddenCount: page.hiddenCount,
        nextSnoozeExpiry: page.nextSnoozeExpiry,
        hasMore: page.hasMore,
        status: "ready",
        appliedGeneration: generation,
      };
    }),

  setNeedsYouInboxError: (workspaceId, generation) =>
    set((draft) => {
      if (draft.needsYouInbox.generationByWorkspaceId[workspaceId] !== generation) return;
      // A failed read clears the rows it was replacing in the SAME update
      // (design-02#Failure-and-recovery) -- stale rows over an absent badge
      // is the disagreement AC .12 forbids.
      draft.needsYouInbox.byWorkspaceId[workspaceId] = {
        ...emptyWorkspaceState(),
        status: "error",
        appliedGeneration: generation,
      };
    }),

  seedNeedsYouInboxBoot: (workspaceId, seed) =>
    set((draft) => {
      // Generation 0 seeds only; if a real read already applied (generation
      // >= 1 was issued), boot arriving late must not overwrite fresher rows.
      if (draft.needsYouInbox.generationByWorkspaceId[workspaceId]) return;
      if (draft.needsYouInbox.byWorkspaceId[workspaceId]) return;
      draft.needsYouInbox.byWorkspaceId[workspaceId] = {
        bundles: [],
        count: seed.count,
        hiddenCount: 0,
        nextSnoozeExpiry: seed.nextSnoozeExpiry,
        hasMore: seed.hasMore,
        status: "idle",
        appliedGeneration: 0,
      };
    }),

  bumpNeedsYouInboxRefreshTick: () =>
    set((draft) => {
      draft.needsYouInbox.refreshTick += 1;
    }),
});
