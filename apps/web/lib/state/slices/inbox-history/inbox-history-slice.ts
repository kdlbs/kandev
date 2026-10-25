import type { StateCreator } from "zustand";
import type { AppState } from "../../app-state-types";
import type {
  InboxHistorySlice,
  InboxHistorySliceState,
  InboxHistoryWorkspaceState,
} from "./types";

export const defaultInboxHistoryState: InboxHistorySliceState = {
  inboxHistory: {
    byWorkspaceId: {},
    generationByWorkspaceId: {},
  },
};

const emptyWorkspaceState = (): InboxHistoryWorkspaceState => ({
  bundles: [],
  total: 0,
  hasMore: false,
  isLoadingMore: false,
  loadMoreError: false,
  status: "idle",
  appliedGeneration: 0,
});

// Typed against the full `AppState` (the zustand slices-pattern shape), not
// this slice's own narrower type -- keeps `set` structurally identical to the
// root store's `set`, so composing this slice in store.ts needs no `as any`
// escape (ARCH-FRONTEND-ROOT-STATE-CAST), mirroring the needs-you-inbox slice.
type ImmerSet = Parameters<
  StateCreator<AppState, [["zustand/immer", never]], [], InboxHistorySlice>
>[0];

export const createInboxHistorySlice = (set: ImmerSet): InboxHistorySlice => ({
  ...defaultInboxHistoryState,

  beginInboxHistoryRead: (workspaceId) => {
    let generation = 0;
    set((draft) => {
      const next = (draft.inboxHistory.generationByWorkspaceId[workspaceId] ?? 0) + 1;
      draft.inboxHistory.generationByWorkspaceId[workspaceId] = next;
      const workspace = draft.inboxHistory.byWorkspaceId[workspaceId] ?? emptyWorkspaceState();
      workspace.status = "loading";
      workspace.isLoadingMore = false;
      workspace.loadMoreError = false;
      draft.inboxHistory.byWorkspaceId[workspaceId] = workspace;
      generation = next;
    });
    return generation;
  },

  setInboxHistoryPage: (workspaceId, generation, page) =>
    set((draft) => {
      if (draft.inboxHistory.generationByWorkspaceId[workspaceId] !== generation) return;
      draft.inboxHistory.byWorkspaceId[workspaceId] = {
        bundles: page.bundles,
        total: page.total,
        hasMore: page.hasMore,
        nextCursor: page.nextCursor,
        isLoadingMore: false,
        loadMoreError: false,
        status: "ready",
        appliedGeneration: generation,
      };
    }),

  beginInboxHistoryLoadMore: (workspaceId, generation) => {
    let accepted = false;
    set((draft) => {
      if (draft.inboxHistory.generationByWorkspaceId[workspaceId] !== generation) return;
      const workspace = draft.inboxHistory.byWorkspaceId[workspaceId];
      if (
        !workspace ||
        workspace.status !== "ready" ||
        !workspace.hasMore ||
        !workspace.nextCursor ||
        workspace.isLoadingMore
      ) {
        return;
      }
      workspace.isLoadingMore = true;
      workspace.loadMoreError = false;
      accepted = true;
    });
    return accepted;
  },

  appendInboxHistoryPage: (workspaceId, generation, page) =>
    set((draft) => {
      if (draft.inboxHistory.generationByWorkspaceId[workspaceId] !== generation) return;
      const workspace = draft.inboxHistory.byWorkspaceId[workspaceId];
      if (!workspace) return;
      workspace.bundles.push(...page.bundles);
      workspace.total = page.total;
      workspace.hasMore = page.hasMore;
      workspace.nextCursor = page.nextCursor;
      workspace.isLoadingMore = false;
      workspace.loadMoreError = false;
    }),

  setInboxHistoryLoadMoreError: (workspaceId, generation) =>
    set((draft) => {
      if (draft.inboxHistory.generationByWorkspaceId[workspaceId] !== generation) return;
      const workspace = draft.inboxHistory.byWorkspaceId[workspaceId];
      if (!workspace) return;
      workspace.isLoadingMore = false;
      workspace.loadMoreError = true;
    }),

  setInboxHistoryError: (workspaceId, generation) =>
    set((draft) => {
      if (draft.inboxHistory.generationByWorkspaceId[workspaceId] !== generation) return;
      draft.inboxHistory.byWorkspaceId[workspaceId] = {
        ...emptyWorkspaceState(),
        status: "error",
        appliedGeneration: generation,
      };
    }),
});
