import type { FeatureFlags } from "./types";

export const defaultFeaturesState: { features: FeatureFlags } = {
  // All flags default to false. Production releases ship every flag off
  // until the deployment opts in via env var (e.g. KANDEV_FEATURES_OFFICE).
  // Query boot seeding overwrites this with whatever the backend reports.
  features: {
    office: false,
    plugins: false,
  },
};
