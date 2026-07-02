/**
 * Best-effort match between a walkthrough step's `file` (as authored by the
 * agent, often repo-relative) and a diff/review file path (which may carry a
 * repo prefix or be otherwise qualified). Exact match wins; otherwise either
 * path may be a path-suffix of the other.
 */
export function walkthroughFileMatches(diffPath: string, stepFile: string): boolean {
  if (!diffPath || !stepFile) return false;
  if (diffPath === stepFile) return true;
  return (
    diffPath.endsWith(`/${stepFile}`) ||
    stepFile.endsWith(`/${diffPath}`) ||
    diffPath.endsWith(stepFile) ||
    stepFile.endsWith(diffPath)
  );
}
