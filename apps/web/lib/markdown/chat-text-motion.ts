export const CHAT_TEXT_DURATION_MS = 140;
export type TextRun = { start: number; end: number; receivedAt: number };

/** Keep only recent append ranges; rewrites have no reliable reveal boundary. */
export function appendedTextRuns(
  previous: string,
  next: string,
  runs: TextRun[],
  now: number,
): TextRun[] {
  if (!next.startsWith(previous)) return [];
  const active = runs.filter((run) => now - run.receivedAt < CHAT_TEXT_DURATION_MS);
  if (next.length === previous.length) return active;
  const last = active.at(-1);
  if (last && last.end === previous.length && now - last.receivedAt < 16) {
    return [...active.slice(0, -1), { ...last, end: next.length }];
  }
  return [...active, { start: previous.length, end: next.length, receivedAt: now }];
}

export type MotionTextPart = { text: string; receivedAt?: number };
export function splitMotionText(
  value: string,
  offset: number | undefined,
  source: string,
  runs: TextRun[],
): MotionTextPart[] {
  if (offset === undefined || source.slice(offset, offset + value.length) !== value)
    return [{ text: value }];
  const parts: MotionTextPart[] = [];
  let cursor = 0;
  for (const run of runs) {
    const start = Math.max(cursor, run.start - offset);
    const end = Math.min(value.length, run.end - offset);
    if (start >= end) continue;
    if (start > cursor) parts.push({ text: value.slice(cursor, start) });
    parts.push({ text: value.slice(start, end), receivedAt: run.receivedAt });
    cursor = end;
  }
  if (cursor < value.length) parts.push({ text: value.slice(cursor) });
  return parts;
}
