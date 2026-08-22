// Runtime feature-flag state. The shape mirrors the backend's
// /api/v1/features response (FeaturesConfig in
// apps/backend/internal/common/config/config.go). New flags are additive:
// declare the all-off default once and derive the names and state from it.
// See docs/decisions/0007-runtime-feature-flags.md.

export const defaultFeatureFlags = {
  // New release toggles start disabled in every frontend state. The SSR layer
  // overwrites this with the backend's effective values after startup.
  office: false,
  auth: false,
  multiTenancy: false,
  dynamicAgentRouting: false,
  claudeBackgroundPromptHandoff: false,
  claudeMidTurnSteering: false,
} as const;

export type FeatureName = keyof typeof defaultFeatureFlags;

export type FeatureFlags = {
  [K in FeatureName]: boolean;
};

export type FeaturesSliceState = {
  features: FeatureFlags;
};

export type FeaturesSliceActions = {
  setFeatures: (features: FeatureFlags) => void;
};

export type FeaturesSlice = FeaturesSliceState & FeaturesSliceActions;
