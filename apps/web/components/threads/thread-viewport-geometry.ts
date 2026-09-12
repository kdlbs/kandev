export function nearestTaskId(
  candidateIds: readonly string[],
  board: HTMLElement | null,
  elements: ReadonlyMap<string, HTMLElement>,
): string | null {
  if (candidateIds.length === 0) return null;
  if (!board) return candidateIds[0] ?? null;
  const boardRect = board.getBoundingClientRect();
  const boardCenter = (boardRect.left + boardRect.right) / 2;
  let nearest: string | null = null;
  let nearestDistance = Number.POSITIVE_INFINITY;
  for (const id of candidateIds) {
    const element = elements.get(id);
    if (!element) continue;
    const rect = element.getBoundingClientRect();
    const distance = Math.abs((rect.left + rect.right) / 2 - boardCenter);
    if (distance < nearestDistance) {
      nearest = id;
      nearestDistance = distance;
    }
  }
  return nearest ?? candidateIds[0] ?? null;
}
