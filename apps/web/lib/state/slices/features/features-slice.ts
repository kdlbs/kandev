import type { StateCreator } from "zustand";
import type { FeaturesSlice, FeaturesSliceState } from "./types";

export const defaultFeaturesState: FeaturesSliceState = {
  // All flags default to false. Production releases ship every flag off
  // until the deployment opts in via env var (e.g. KANDEV_FEATURES_OFFICE).
  // The SSR layer overwrites this with whatever the backend reports.
  features: {
    office: false,
  },
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
