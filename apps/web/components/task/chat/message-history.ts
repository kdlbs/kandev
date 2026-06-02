import type { Message } from "@/lib/types/http";

/** Extract the user's previously sent text messages for a session, newest-first.
 *  Consecutive duplicates *in the original message stream* are collapsed —
 *  duplicates separated by an assistant or tool turn stay, since the user
 *  meaningfully repeated themselves and should be able to recall the older
 *  one from history. Empty/whitespace-only messages are skipped. */
export function extractUserHistory(messages: readonly Message[]): string[] {
  const out: string[] = [];
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m.author_type !== "user" || m.type !== "message") continue;
    const content = m.content ?? "";
    if (!content.trim()) continue;
    const newer = messages[i + 1];
    const isAdjacentUserDuplicate =
      newer?.author_type === "user" &&
      newer.type === "message" &&
      (newer.content ?? "") === content;
    if (isAdjacentUserDuplicate) continue;
    out.push(content);
  }
  return out;
}

export type HistoryState = {
  /** Position within the history list (0 = most recent). `null` means the user
   *  is currently editing their own draft, not viewing history. */
  index: number | null;
};

/** Decide the next history navigation state when ArrowUp/ArrowDown is pressed.
 *  Returns `null` if the press should be deferred (e.g. no history, or at the
 *  oldest entry on ArrowUp — caller should treat as a no-op). */
export function navigateHistory(
  state: HistoryState,
  direction: "up" | "down",
  historyLength: number,
): HistoryState | null {
  if (historyLength === 0) return null;
  if (direction === "up") {
    const next = state.index === null ? 0 : state.index + 1;
    if (next >= historyLength) return null;
    return { index: next };
  }
  if (state.index === null) return null;
  if (state.index === 0) return { index: null };
  return { index: state.index - 1 };
}

/** Lightweight subsequence-fuzzy scorer. Returns `null` if `needle`'s
 *  characters cannot be found in `haystack` in order (case-insensitive).
 *  Higher score = better match; consecutive matches and word-boundary hits
 *  rank higher. Shorter haystacks tie-break ahead of longer ones. */
export function fuzzyScore(needle: string, haystack: string): number | null {
  if (!needle) return 1;
  const n = needle.toLowerCase();
  const h = haystack.toLowerCase();
  let hi = 0;
  let score = 0;
  let consecutive = 0;
  for (let ni = 0; ni < n.length; ni++) {
    const c = n.charAt(ni);
    const next = h.indexOf(c, hi);
    if (next === -1) return null;
    if (next === hi) {
      consecutive++;
      score += 2 + consecutive;
    } else {
      consecutive = 0;
      score += 1;
    }
    const prev = next === 0 ? "" : h.charAt(next - 1);
    if (next === 0 || /[\s\-_/.,(){}[\]]/.test(prev)) score += 2;
    hi = next + 1;
  }
  return score - haystack.length * 0.01;
}

export type SearchHit = {
  /** Original index in the history array (lets the caller jump the history
   *  index when the user picks a search result). */
  index: number;
  content: string;
  score: number;
};

/** Score every history entry against `query` and return matches sorted
 *  best-first. An empty query returns every entry in newest-first order
 *  (preserving the original index). */
export function searchHistory(history: readonly string[], query: string): SearchHit[] {
  if (!query) {
    return history.map((content, index) => ({ index, content, score: 0 }));
  }
  const hits: SearchHit[] = [];
  for (let i = 0; i < history.length; i++) {
    const score = fuzzyScore(query, history[i]);
    if (score !== null) hits.push({ index: i, content: history[i], score });
  }
  hits.sort((a, b) => b.score - a.score);
  return hits;
}
