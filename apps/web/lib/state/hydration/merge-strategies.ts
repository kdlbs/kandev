import type { Draft } from "immer";

/**
 * Merge strategies for different state shapes
 */

/**
 * Deep merge for nested objects - overwrites at leaf level only
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function deepMerge<T extends Record<string, any>>(
  target: Draft<T>,
  source: Partial<T>,
): void {
  for (const key in source) {
    const sourceValue = source[key];
    if (sourceValue === undefined) continue;

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const targetValue = (target as any)[key];

    // If both are plain objects, recurse
    if (
      targetValue &&
      typeof targetValue === "object" &&
      !Array.isArray(targetValue) &&
      sourceValue &&
      typeof sourceValue === "object" &&
      !Array.isArray(sourceValue)
    ) {
      deepMerge(targetValue, sourceValue);
    } else {
      // Otherwise, overwrite
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (target as any)[key] = sourceValue;
    }
  }
}

/**
 * Merge strategy for sessionId-keyed maps
 * Only merges sessions that don't exist or are not currently active
 * @param forceMergeSessionId - If provided, this session will be merged even if it's active
 */
export function mergeSessionMap<T>(
  target: Draft<Record<string, T>>,
  source: Record<string, T> | undefined,
  activeSessionId: string | null,
  forceMergeSessionId?: string | null,
): void {
  if (!source) return;

  for (const sessionId in source) {
    // Force merge if this is the forceMergeSessionId (for navigation refresh)
    const shouldForceMerge = forceMergeSessionId && sessionId === forceMergeSessionId;

    // Skip if this is the active session to avoid overwriting live data (unless force merge)
    if (!shouldForceMerge && sessionId === activeSessionId) continue;

    // Merge the session data
    if (shouldForceMerge || !(sessionId in target)) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (target as any)[sessionId] = source[sessionId];
    }
  }
}

/**
 * Merge strategy for arrays - replaces entire array if source is non-empty
 */
export function mergeArray<T>(target: Draft<T[]>, source: T[] | undefined): void {
  if (!source || source.length === 0) return;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (target as any).splice(0, target.length, ...source);
}

/**
 * Merge strategy for items with timestamps
 * Only overwrites if source is newer
 */
export function mergeWithTimestamp<T extends { updatedAt?: string }>(
  target: Draft<T>,
  source: T | undefined,
): void {
  if (!source) return;

  const targetTime = target.updatedAt ? new Date(target.updatedAt).getTime() : 0;
  const sourceTime = source.updatedAt ? new Date(source.updatedAt).getTime() : 0;

  // Only merge if source is newer or target has no timestamp
  if (sourceTime >= targetTime) {
    Object.assign(target, source);
  }
}

/**
 * Merge strategy for loading states
 * Preserves loading: true state to avoid flickering
 */
export function mergeLoadingState(
  target: Draft<{ loading: boolean; loaded: boolean }>,
  source: { loading?: boolean; loaded?: boolean } | undefined,
): void {
  if (!source) return;

  // Don't overwrite loading: true with loading: false from SSR
  if (target.loading && source.loading === false) {
    return;
  }

  if (source.loading !== undefined) target.loading = source.loading;
  if (source.loaded !== undefined) target.loaded = source.loaded;
}
