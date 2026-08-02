import type { StateCreator } from "zustand";
import { defaultFeatureFlags, type FeaturesSlice, type FeaturesSliceState } from "./types";

export const defaultFeaturesState: FeaturesSliceState = {
  features: { ...defaultFeatureFlags },
};

type ImmerSet = Parameters<
  StateCreator<FeaturesSlice, [["zustand/immer", never]], [], FeaturesSlice>
>[0];

export const createFeaturesSlice: StateCreator<
  FeaturesSlice,
  [["zustand/immer", never]],
  [],
  FeaturesSlice
> = (set: ImmerSet) => ({
  ...defaultFeaturesState,
  setFeatures: (features) =>
    set((draft) => {
      draft.features = features;
    }),
});
