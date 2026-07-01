import { useQuery } from "@tanstack/react-query";
import { defaultFeaturesState } from "@/lib/state/slices/features/features-slice";
import { featureFlagsQueryOptions } from "@/lib/query/query-options";
import type { FeatureName } from "@/lib/state/slices/features/types";

/**
 * Read a single feature flag. Returns `true` when the deployment opted in
 * (env var on the backend) and `false` otherwise — which is the production
 * default for every new flag. Boot/app-state hydration seeds the Query cache,
 * and the hook falls back to all-off defaults until the backend response lands.
 *
 * See docs/decisions/0007-runtime-feature-flags.md for the rollout pattern.
 */
export function useFeature(name: FeatureName): boolean {
  const query = useQuery(featureFlagsQueryOptions());
  return query.data?.[name] ?? defaultFeaturesState.features[name];
}
