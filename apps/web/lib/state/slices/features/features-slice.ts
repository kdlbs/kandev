import type { Draft } from "immer";
import { defaultFeatureFlags, type FeaturesSlice, type FeaturesSliceState } from "./types";

export const defaultFeaturesState: FeaturesSliceState = {
  features: { ...defaultFeatureFlags },
};

type ImmerSet = (updater: (draft: Draft<FeaturesSlice>) => void) => void;

export const createFeaturesSlice = (set: ImmerSet): FeaturesSlice => ({
  ...defaultFeaturesState,
  setFeatures: (features) =>
    set((draft) => {
      draft.features = features;
    }),
});
