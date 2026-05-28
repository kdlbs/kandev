/**
 * Runtime feature-flag types and defaults.
 *
 * The shape mirrors the backend's /api/v1/features response
 * (FeaturesConfig in apps/backend/internal/common/config/config.go).
 * Every flag is a boolean, keyed by feature name. New flags are additive
 * — keep this shape stable.
 *
 * See docs/decisions/0007-runtime-feature-flags.md.
 */

export type FeatureFlags = {
  office: boolean;
};

export type FeatureName = keyof FeatureFlags;

/**
 * Default feature flags — all off. Production releases ship every flag
 * off until the deployment opts in via env var (e.g. KANDEV_FEATURES_OFFICE).
 * The SSR / TanStack Query layer overwrites this with whatever the backend reports.
 *
 * Production-safety invariant: every flag MUST default to false.
 */
export const defaultFeatureFlags: FeatureFlags = {
  office: false,
};
