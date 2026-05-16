// Runtime feature-flag state. The shape mirrors the backend's
// /api/v1/features response (FeaturesConfig in
// apps/backend/internal/common/config/config.go). Every flag is a boolean,
// keyed by feature name. New flags are additive — keep this shape stable.
// See docs/decisions/0007-runtime-feature-flags.md.

export type FeatureFlags = {
  office: boolean;
};

export type FeatureName = keyof FeatureFlags;

export type FeaturesSliceState = {
  features: FeatureFlags;
};

export type FeaturesSliceActions = {
  setFeatures: (features: FeatureFlags) => void;
};

export type FeaturesSlice = FeaturesSliceState & FeaturesSliceActions;
