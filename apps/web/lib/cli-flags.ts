import type { CLIFlag } from "@/lib/types/http";

/**
 * Deep-equality comparison for a profile's cli_flags list. Used by the
 * profile dirty-detection paths in both the agent setup page and the
 * per-profile editor. The list order matters — it matches the order the
 * backend stores and the order CLIFlagsField renders.
 */
export function areCLIFlagsEqual(
  a: CLIFlag[] | null | undefined,
  b: CLIFlag[] | null | undefined,
): boolean {
  const left = a ?? [];
  const right = b ?? [];
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i++) {
    if (
      left[i].flag !== right[i].flag ||
      left[i].enabled !== right[i].enabled ||
      (left[i].description ?? "") !== (right[i].description ?? "")
    ) {
      return false;
    }
  }
  return true;
}
