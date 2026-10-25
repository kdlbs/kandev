export function resolveRemainingThreadId(
  previous: readonly string[],
  next: readonly string[],
  current: string | null,
): string | null {
  const remaining = new Set(next);
  if (current && remaining.has(current)) return current;
  const index = current ? previous.indexOf(current) : -1;
  if (index !== -1) {
    const successor = previous.slice(index + 1).find((id) => remaining.has(id));
    if (successor) return successor;
    const predecessor = previous
      .slice(0, index)
      .reverse()
      .find((id) => remaining.has(id));
    if (predecessor) return predecessor;
  }
  return next[0] ?? null;
}
