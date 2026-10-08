import type { StateCreator } from "zustand";
import type { AppState } from "../../app-state-types";
import type { CoordinatorsSlice, CoordinatorsSliceState } from "./types";

export const defaultCoordinatorsState: CoordinatorsSliceState = {
  coordinators: { items: [], loaded: false, loading: false },
};

// Typed against the full `AppState`, not this slice's own narrower type, so
// `set` is structurally identical to the root store's `set` and composing
// this slice in store.ts needs no `as any` escape (ARCH-FRONTEND-ROOT-STATE-CAST).
type ImmerSet = Parameters<
  StateCreator<AppState, [["zustand/immer", never]], [], CoordinatorsSlice>
>[0];

export const createCoordinatorsSlice = (set: ImmerSet): CoordinatorsSlice => ({
  ...defaultCoordinatorsState,
  setCoordinators: (items) =>
    set((draft) => {
      draft.coordinators.items = items;
      draft.coordinators.loaded = true;
    }),
  setCoordinatorsLoading: (loading) =>
    set((draft) => {
      draft.coordinators.loading = loading;
    }),
  addCoordinator: (coordinator) =>
    set((draft) => {
      draft.coordinators.items.push(coordinator);
    }),
  updateCoordinator: (coordinator) =>
    set((draft) => {
      const idx = draft.coordinators.items.findIndex((item) => item.id === coordinator.id);
      if (idx >= 0) {
        draft.coordinators.items[idx] = coordinator;
      }
    }),
  removeCoordinator: (id) =>
    set((draft) => {
      draft.coordinators.items = draft.coordinators.items.filter((item) => item.id !== id);
    }),
});
