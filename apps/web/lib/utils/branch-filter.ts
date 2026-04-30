// Custom scorer for the branch combobox in the new-task dialog.
//
// cmdk's default `command-score` filter is biased toward short strings and
// doesn't understand path-like separators (`/`, `-`, `_`, `.`), so long
// branches such as `origin/feature/foo-bar-baz` rank poorly when the user
// types a fragment like `bar`. This scorer is path-aware, ranks word-boundary
// matches above mid-word matches, and falls back to a plain subsequence
// (fuzzy) match so misspellings still surface results.
//
// The signature matches the cmdk `<Command filter>` prop: returns a number
// in [0, 1]; cmdk treats >0 as "include" and sorts items in descending order.

const SEGMENT_RE = /[/_.\-\s]+/g;

const SCORE_EXACT = 1.0;
const SCORE_LEAF_EXACT = 0.95;
const SCORE_FULL_PREFIX = 0.9;
const SCORE_LEAF_PREFIX = 0.85;
const SCORE_SEGMENT_PREFIX = 0.75;
const SCORE_FULL_SUBSTRING = 0.65;
const SCORE_LEAF_SUBSTRING = 0.6;
const SCORE_SEGMENT_SUBSTRING = 0.5;
const SCORE_KEYWORD_PREFIX = 0.7;
const SCORE_KEYWORD_SUBSTRING = 0.45;
const SCORE_SUBSEQUENCE_BASE = 0.3;

function leafSegment(value: string): string {
  const idx = value.lastIndexOf("/");
  return idx >= 0 ? value.slice(idx + 1) : value;
}

function segments(value: string): string[] {
  return value.split(SEGMENT_RE).filter((s) => s.length > 0);
}

function isSubsequence(needle: string, haystack: string): boolean {
  if (needle.length === 0) return true;
  let i = 0;
  for (let j = 0; j < haystack.length && i < needle.length; j++) {
    if (haystack[j] === needle[i]) i++;
  }
  return i === needle.length;
}

// Subsequence density: how tightly the matched chars cluster in the haystack.
// 1.0 means contiguous, lower means more spread out.
function subsequenceDensity(needle: string, haystack: string): number {
  if (needle.length === 0) return 1;
  let first = -1;
  let last = -1;
  let i = 0;
  for (let j = 0; j < haystack.length && i < needle.length; j++) {
    if (haystack[j] === needle[i]) {
      if (first < 0) first = j;
      last = j;
      i++;
    }
  }
  if (i < needle.length || first < 0) return 0;
  const span = last - first + 1;
  return needle.length / span;
}

function scoreSegments(segs: string[], q: string, prefixScore: number, subScore: number): number {
  let best = 0;
  for (const seg of segs) {
    if (seg === q) return prefixScore + 0.05;
    if (seg.startsWith(q)) best = Math.max(best, prefixScore);
    else if (seg.includes(q)) best = Math.max(best, subScore);
  }
  return best;
}

function scoreKeywords(keywords: string[] | undefined, q: string): number {
  if (!keywords || keywords.length === 0) return 0;
  let best = 0;
  for (const raw of keywords) {
    if (!raw) continue;
    const k = raw.toLowerCase();
    if (k === q) best = Math.max(best, SCORE_KEYWORD_PREFIX + 0.05);
    else if (k.startsWith(q)) best = Math.max(best, SCORE_KEYWORD_PREFIX);
    else if (k.includes(q)) best = Math.max(best, SCORE_KEYWORD_SUBSTRING);
  }
  return best;
}

// scoreBranch returns a numeric match score between value+keywords and the
// search query. Returns 0 when no signal at all is found.
export function scoreBranch(value: string, search: string, keywords?: string[]): number {
  const q = search.trim().toLowerCase();
  if (q.length === 0) return SCORE_LEAF_SUBSTRING; // keep current order; show all
  const v = value.toLowerCase();
  const leaf = leafSegment(v);

  if (v === q) return SCORE_EXACT;
  if (leaf === q) return SCORE_LEAF_EXACT;
  if (v.startsWith(q)) return SCORE_FULL_PREFIX;
  if (leaf.startsWith(q)) return SCORE_LEAF_PREFIX;

  const segScore = scoreSegments(segments(v), q, SCORE_SEGMENT_PREFIX, SCORE_SEGMENT_SUBSTRING);
  if (segScore > 0) return segScore;

  if (v.includes(q)) return SCORE_FULL_SUBSTRING - lengthPenalty(v);
  if (leaf.includes(q)) return SCORE_LEAF_SUBSTRING - lengthPenalty(leaf);

  const kwScore = scoreKeywords(keywords, q);
  if (kwScore > 0) return kwScore;

  // Last resort: subsequence (fuzzy) match. Density tightens the score so
  // a clustered match outranks a wildly spread one.
  if (isSubsequence(q, v)) {
    const density = subsequenceDensity(q, v);
    return SCORE_SUBSEQUENCE_BASE * density;
  }
  return 0;
}

// lengthPenalty subtracts a tiny amount so that, between two substring
// matches, the shorter haystack ranks higher. Capped to avoid pushing a
// genuine match below the subsequence tier.
function lengthPenalty(s: string): number {
  const p = Math.min(s.length / 1000, 0.05);
  return p;
}
