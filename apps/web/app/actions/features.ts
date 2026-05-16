"use server";

import { getBackendConfig } from "@/lib/config";
import { defaultFeaturesState } from "@/lib/state/slices/features/features-slice";
import type { FeatureFlags } from "@/lib/state/slices/features/types";

const { apiBaseUrl } = getBackendConfig();

// Server-side feature-flag fetch. Called once per request from the root
// layout so every page renders with the deployment's flags already in the
// store. Falls back to defaults (all-off) when the backend is unreachable,
// so a dev-server restart doesn't crash page rendering.
//
// Parsing is type-driven via `defaultFeaturesState.features`: every key
// declared on FeatureFlags is read from the response and Boolean-coerced;
// missing or non-bool values fall through to the default. Adding a flag
// is therefore one entry in FeatureFlags + the default — no edit here.
//
// See docs/decisions/0007-runtime-feature-flags.md.
export async function getFeatureFlagsAction(): Promise<FeatureFlags> {
  const url = `${apiBaseUrl}/api/v1/features`;
  const defaults = defaultFeaturesState.features;
  try {
    const response = await fetch(url, { cache: "no-store" });
    if (!response.ok) {
      return defaults;
    }
    const data = (await response.json()) as Record<string, unknown>;
    return normalizeFlags(data, defaults);
  } catch {
    return defaults;
  }
}

function normalizeFlags(data: Record<string, unknown>, defaults: FeatureFlags): FeatureFlags {
  const out = { ...defaults };
  for (const key of Object.keys(out) as (keyof FeatureFlags)[]) {
    const value = data[key];
    if (typeof value === "boolean") {
      out[key] = value;
    }
  }
  return out;
}
