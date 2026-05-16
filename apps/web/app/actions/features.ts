"use server";

import { getBackendConfig } from "@/lib/config";
import type { FeatureFlags } from "@/lib/state/slices/features/types";

const { apiBaseUrl } = getBackendConfig();

// Server-side feature-flag fetch. Called once per request from the root
// layout so every page renders with the deployment's flags already in the
// store. Falls back to all-off if the backend is unreachable, so a
// dev-server restart doesn't crash page rendering.
//
// See docs/decisions/0007-runtime-feature-flags.md.
export async function getFeatureFlagsAction(): Promise<FeatureFlags> {
  const url = `${apiBaseUrl}/api/v1/features`;
  try {
    const response = await fetch(url, { cache: "no-store" });
    if (!response.ok) {
      return { office: false };
    }
    const data = (await response.json()) as Partial<FeatureFlags>;
    return { office: Boolean(data.office) };
  } catch {
    return { office: false };
  }
}
