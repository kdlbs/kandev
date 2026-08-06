/** True when `ancestor` owns the gitlink recorded by `scope`. */
export function isRepositoryScopeAncestor(ancestor: string, scope: string): boolean {
  if (ancestor === scope) return false;
  if (ancestor === "") return scope !== "";
  return scope.startsWith(`${ancestor}/`);
}

/**
 * Adds only tracked repository scopes that are ancestors of changed scopes.
 * Clean parents need to participate in stage-all commits so they can record
 * the child gitlink after the child commit; unrelated clean siblings do not.
 */
export function repositoryScopesWithAvailableAncestors(
  scopes: Iterable<string>,
  availableScopes: Iterable<string>,
): string[] {
  const requested = Array.from(new Set(scopes));
  const available = new Set(availableScopes);
  const result = new Set(requested);
  for (const scope of requested) {
    if (scope === "") continue;
    const parts = scope.split("/");
    for (let length = 1; length < parts.length; length += 1) {
      const ancestor = parts.slice(0, length).join("/");
      if (available.has(ancestor)) result.add(ancestor);
    }
    if (available.has("")) result.add("");
  }
  return Array.from(result);
}

/**
 * Groups unique repository scopes into dependency-safe waves. The deepest
 * repositories run first; scopes at the same depth are independent and may
 * run in parallel. Empty scope is the real workspace root and therefore has
 * depth zero.
 */
export function repositoryScopeWaves(scopes: Iterable<string>): string[][] {
  const byDepth = new Map<number, Set<string>>();
  for (const scope of new Set(scopes)) {
    const depth = scope === "" ? 0 : scope.split("/").length;
    const bucket = byDepth.get(depth) ?? new Set<string>();
    bucket.add(scope);
    byDepth.set(depth, bucket);
  }
  return Array.from(byDepth.entries())
    .sort(([left], [right]) => right - left)
    .map(([, scopesAtDepth]) => Array.from(scopesAtDepth).sort((a, b) => a.localeCompare(b)));
}

/** A failed child prevents an ancestor from recording a stale gitlink. */
export function shouldSkipRepositoryScope(scope: string, failedScopes: Iterable<string>): boolean {
  for (const failedScope of failedScopes) {
    if (isRepositoryScopeAncestor(scope, failedScope)) return true;
  }
  return false;
}

export type RepositoryScopeWaveResult<T> = {
  repository_name: string;
  result: T;
  skipped: boolean;
};

/**
 * Runs scope operations wave by wave and records blocked ancestors without
 * invoking them. The caller supplies the skipped result so this helper stays
 * independent of the Git operation wire shape.
 */
export async function runRepositoryScopeWaves<T extends { success: boolean }>(
  scopes: Iterable<string>,
  operation: (scope: string) => Promise<T>,
  skippedResult: (scope: string, failedScopes: string[]) => T,
): Promise<RepositoryScopeWaveResult<T>[]> {
  const failedScopes: string[] = [];
  const results: RepositoryScopeWaveResult<T>[] = [];
  for (const wave of repositoryScopeWaves(scopes)) {
    const runnable = wave.filter((scope) => !shouldSkipRepositoryScope(scope, failedScopes));
    const blocked = wave.filter((scope) => shouldSkipRepositoryScope(scope, failedScopes));
    for (const scope of blocked) {
      results.push({
        repository_name: scope,
        result: skippedResult(scope, failedScopes),
        skipped: true,
      });
    }
    const waveResults = await Promise.all(
      runnable.map(async (scope) => ({ repository_name: scope, result: await operation(scope) })),
    );
    for (const entry of waveResults) {
      results.push({ ...entry, skipped: false });
      if (!entry.result.success) failedScopes.push(entry.repository_name);
    }
  }
  return results;
}
