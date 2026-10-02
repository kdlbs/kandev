import type { Coordinator } from "@/lib/api/domains/coordinator-api";

const LAST_USED_PREFIX = "kandev.coordinatorCopilot.lastUsed.";

function lastUsedKey(workspaceId: string): string {
  return `${LAST_USED_PREFIX}${workspaceId}`;
}

/** An unreadable or malformed value reads as absent. */
export function readLastUsed(workspaceId: string): string | null {
  try {
    const value = window.localStorage.getItem(lastUsedKey(workspaceId));
    return value && value.trim() !== "" ? value : null;
  } catch {
    return null;
  }
}

/** A failed write is ignored: the choice then lasts for the open panel only. */
export function writeLastUsed(workspaceId: string, coordinatorId: string): void {
  try {
    window.localStorage.setItem(lastUsedKey(workspaceId), coordinatorId);
  } catch {
    // Storage is unavailable; the choice is not remembered.
  }
}

export function clearLastUsed(workspaceId: string): void {
  try {
    window.localStorage.removeItem(lastUsedKey(workspaceId));
  } catch {
    // Nothing stored that could be stale.
  }
}

/**
 * The coordinator the panel opens with: the last used one while the list still
 * holds it, else the first in list order. A stale stored value is forgotten.
 * Only the switcher writes last used.
 */
export function chooseCoordinator(
  workspaceId: string,
  coordinators: readonly Coordinator[],
): Coordinator | undefined {
  const stored = readLastUsed(workspaceId);
  const remembered = stored ? coordinators.find((c) => c.id === stored) : undefined;
  if (stored && !remembered) clearLastUsed(workspaceId);
  return remembered ?? coordinators[0];
}
