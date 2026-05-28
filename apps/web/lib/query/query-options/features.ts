import { queryOptions } from "@tanstack/react-query";
import { fetchFeatureFlags } from "@/lib/api/domains/features-api";
import { qk } from "@/lib/query/keys";

/**
 * Shared queryOptions for runtime feature flags.
 *
 * Used by:
 * - CSR: useQuery(featuresQueryOptions())
 * - SSR: qc.prefetchQuery(featuresQueryOptions()) in server components
 *
 * staleTime is overridden to Infinity because feature flags are set
 * at deployment time and don't change during a browser session.
 * Refetch only happens on explicit cache invalidation (e.g. after
 * a settings change or manual override).
 *
 * Falls back to all-off defaults when the backend is unreachable —
 * the queryFn (fetchFeatureFlags) inherits the global retry policy.
 *
 * See docs/decisions/0007-runtime-feature-flags.md.
 */
export function featuresQueryOptions() {
  return queryOptions({
    queryKey: qk.features(),
    queryFn: () => fetchFeatureFlags(),
    staleTime: Infinity,
  });
}
