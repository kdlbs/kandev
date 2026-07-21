export type WalkthroughTargetFile = {
  path: string;
  repository_name?: string;
};

export type WalkthroughTargetStep = {
  file: string;
  repo?: string;
};

function normalizePath(path: string): string[] {
  return path.trim().replaceAll("\\", "/").replace(/^\.\//, "").split("/").filter(Boolean);
}

function normalizeRepo(repo: string | undefined): string {
  return (repo ?? "").trim();
}

function segmentsMatchSuffix(left: string[], right: string[]): boolean {
  if (left.length === 0 || right.length === 0) return false;
  const shorter = left.length <= right.length ? left : right;
  const longer = left.length <= right.length ? right : left;
  const offset = longer.length - shorter.length;
  return shorter.every((segment, index) => longer[offset + index] === segment);
}

/**
 * Best-effort match between a walkthrough step's `file` and a diff/review file
 * path. Exact match wins; suffix matching is allowed only on path-segment
 * boundaries so `bar.ts` cannot match `foobar.ts`.
 */
export function walkthroughFileMatches(diffPath: string, stepFile: string): boolean {
  const diffSegments = normalizePath(diffPath);
  const stepSegments = normalizePath(stepFile);
  return segmentsMatchSuffix(diffSegments, stepSegments);
}

function repositoryMatches(file: WalkthroughTargetFile, step: WalkthroughTargetStep): boolean {
  const stepRepo = normalizeRepo(step.repo);
  const fileRepo = normalizeRepo(file.repository_name);
  if (!stepRepo) return !fileRepo;
  if (!fileRepo) return false;
  return fileRepo === stepRepo;
}

function isRepoAmbiguous(
  file: WalkthroughTargetFile,
  step: WalkthroughTargetStep,
  allFiles: WalkthroughTargetFile[] | undefined,
): boolean {
  if (step.repo || !allFiles || !file.repository_name) return false;
  const repos = new Set(
    allFiles
      .filter((candidate) => walkthroughFileMatches(candidate.path, step.file))
      .map((candidate) => normalizeRepo(candidate.repository_name))
      .filter(Boolean),
  );
  return repos.size > 1;
}

export function walkthroughStepMatchesFile(
  file: WalkthroughTargetFile,
  step: WalkthroughTargetStep,
  allFiles?: WalkthroughTargetFile[],
): boolean {
  if (!repositoryMatches(file, step)) return false;
  if (!walkthroughFileMatches(file.path, step.file)) return false;
  if (isRepoAmbiguous(file, step, allFiles)) return false;
  return true;
}
