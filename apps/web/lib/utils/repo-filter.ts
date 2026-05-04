import { scoreBranch } from "./branch-filter";

// scoreRepo wraps scoreBranch but drops weak subsequence-only matches.
//
// scoreBranch's fuzzy fallback returns small non-zero scores when search
// characters appear in order across an arbitrary haystack — useful for
// branch typos, but noisy for repo paths. A query like "arthur" was
// matching long paths like "playground/fun/thm/.../lxd-alpine-builder"
// because a-r-t-h-u-r appears in order across path segments.
//
// scoreBranch awards >= 0.45 for any substring/prefix/word-boundary hit,
// so a 0.4 floor preserves real matches while filtering coincidences.
export function scoreRepo(value: string, search: string, keywords?: string[]): number {
  const score = scoreBranch(value, search, keywords);
  return score >= 0.4 ? score : 0;
}
