export type PendingPtyStart = { cancelled: boolean };

export const pendingPtyStarts = new Map<string, PendingPtyStart>();

/** Marks a detached terminal start as explicitly closed. */
export function cancelPtyTerminalStart(ownerId: string): void {
  const pending = pendingPtyStarts.get(ownerId);
  if (pending) pending.cancelled = true;
}
