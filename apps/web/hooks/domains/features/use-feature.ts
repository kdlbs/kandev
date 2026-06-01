import { useQuery } from "@tanstack/react-query";
import { featuresQueryOptions } from "@/lib/query/query-options/features";
import type { FeatureName } from "@/lib/features";

/**
 * Read a single feature flag. Returns `true` when the deployment opted in
 * (env var on the backend) and `false` otherwise — which is the production
 * default for every new flag.
 *
 * Data is seeded from SSR prefetch (via QueryProvider) so this hook is safe
 * to call from any client component without an extra network round-trip.
 *
 * Falls back to `false` when the cache is empty (e.g. during hydration
 * before the query resolves) to preserve the production-safety invariant
 * that unrecognised flags are off.
 *
 * See docs/decisions/0007-runtime-feature-flags.md.
 */
export function useFeature(name: FeatureName): boolean {
  const { data } = useQuery({
    ...featuresQueryOptions(),
    select: (flags) => flags[name],
  });
  // Default to false when cache is not yet populated.
  return data ?? false;
}
