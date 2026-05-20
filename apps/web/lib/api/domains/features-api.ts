import { fetchJson, type ApiRequestOptions } from "../client";
import type { FeatureFlags } from "@/lib/state/slices/features/types";

// Mirrors the backend response shape from
// apps/backend/cmd/kandev/helpers.go GET /api/v1/features.
// New flags are additive boolean keys.
export function fetchFeatureFlags(options?: ApiRequestOptions): Promise<FeatureFlags> {
  return fetchJson<FeatureFlags>("/api/v1/features", options);
}
