export type ArrivalSnapshot = { sessionId: string | null; ids: readonly string[]; live: boolean };

/** Only appends to an already visible, reconciled transcript are entrances. */
export function getNewArrivalIds(
  previous: ArrivalSnapshot | null,
  next: ArrivalSnapshot,
): ReadonlySet<string> {
  if (!previous?.live || !next.live || previous.sessionId !== next.sessionId) return new Set();
  if (previous.ids.some((id, index) => next.ids[index] !== id)) return new Set();
  return new Set(next.ids.slice(previous.ids.length));
}
