/**
 * Parses a GitHub repository URL of the form
 *   https://github.com/<owner>/<repo>[.git][/][?query][#fragment]
 * (scheme and `www.` are optional).
 *
 * Returns `{ owner, repo }` on a match, or `null` for empty input or any URL
 * that doesn't match the repo pattern. The input is trimmed before matching;
 * a `.git` suffix and a single trailing slash are tolerated and stripped, and
 * an optional `?...` query string or `#...` fragment is allowed before EOL so
 * canonical share URLs like `github.com/owner/repo?tab=readme` parse cleanly.
 *
 * PR URLs (`/pull/<n>`) intentionally do NOT match here — callers that need
 * to recognize PRs layer their own regex on top before falling back to this
 * helper.
 */
export function parseGitHubRepoUrl(url: string): { owner: string; repo: string } | null {
  const trimmed = url.trim();
  if (!trimmed) return null;
  const match = trimmed.match(
    /(?:https?:\/\/)?(?:www\.)?github\.com\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+?)(?:\.git)?\/?(?:[?#].*)?$/,
  );
  if (!match) return null;
  return { owner: match[1], repo: match[2] };
}
