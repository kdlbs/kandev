import { useAppStore } from "@/components/state-provider";
import type { FeatureName } from "@/lib/state/slices/features/types";

/**
 * Read a single feature flag. Returns `true` when the deployment opted in
 * (env var on the backend) and `false` otherwise — which is the production
 * default for every new flag. SSR populates the store via the layout, so
 * this hook is safe to call from any client component.
 *
 * See docs/decisions/0007-runtime-feature-flags.md for the rollout pattern.
 */
export function useFeature(name: FeatureName): boolean {
  return useAppStore((s) => s.features[name]);
}
