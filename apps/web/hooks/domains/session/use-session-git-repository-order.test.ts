import { describe, expect, it } from "vitest";
import {
  isRepositoryScopeAncestor,
  repositoryScopesWithAvailableAncestors,
  repositoryScopeWaves,
  runRepositoryScopeWaves,
  shouldSkipRepositoryScope,
} from "./use-session-git-repository-order";

const ROOT_SCOPE = "";
const OUTER_SCOPE = "vendor/outer";
const INNER_SCOPE = "vendor/outer/vendor/inner";
const OTHER_SCOPE = "vendor/other";

describe("repository scope mutation order", () => {
  it("groups nested scopes into deepest-first waves and keeps siblings together", () => {
    expect(repositoryScopeWaves([ROOT_SCOPE, OUTER_SCOPE, INNER_SCOPE, OTHER_SCOPE])).toEqual([
      [INNER_SCOPE],
      [OTHER_SCOPE, OUTER_SCOPE],
      [ROOT_SCOPE],
    ]);
  });

  it("treats the workspace root as an ancestor of every named scope", () => {
    expect(isRepositoryScopeAncestor(ROOT_SCOPE, OUTER_SCOPE)).toBe(true);
    expect(isRepositoryScopeAncestor(OUTER_SCOPE, INNER_SCOPE)).toBe(true);
    expect(isRepositoryScopeAncestor(OTHER_SCOPE, INNER_SCOPE)).toBe(false);
  });

  it("includes clean tracked parents without unrelated clean siblings", () => {
    expect(
      repositoryScopesWithAvailableAncestors(
        [INNER_SCOPE],
        [ROOT_SCOPE, OUTER_SCOPE, INNER_SCOPE, OTHER_SCOPE],
      ),
    ).toEqual([INNER_SCOPE, OUTER_SCOPE, ROOT_SCOPE]);
    expect(repositoryScopesWithAvailableAncestors(["frontend"], ["frontend", "backend"])).toEqual([
      "frontend",
    ]);
  });

  it("blocks only ancestors after a child failure", () => {
    expect(shouldSkipRepositoryScope(ROOT_SCOPE, [OUTER_SCOPE])).toBe(true);
    expect(shouldSkipRepositoryScope(OUTER_SCOPE, [INNER_SCOPE])).toBe(true);
    expect(shouldSkipRepositoryScope(OTHER_SCOPE, [OUTER_SCOPE])).toBe(false);
    expect(shouldSkipRepositoryScope(OUTER_SCOPE, [OTHER_SCOPE])).toBe(false);
  });

  it("preserves single-scope and sibling-only operations", () => {
    expect(repositoryScopeWaves([ROOT_SCOPE])).toEqual([[ROOT_SCOPE]]);
    expect(repositoryScopeWaves(["frontend", "backend"])).toEqual([["backend", "frontend"]]);
  });

  it("runs children before parents, blocks failed ancestors, and keeps siblings independent", async () => {
    const calls: string[] = [];
    const results = await runRepositoryScopeWaves(
      [ROOT_SCOPE, OUTER_SCOPE, INNER_SCOPE, OTHER_SCOPE],
      async (scope) => {
        calls.push(scope);
        return { success: scope !== INNER_SCOPE };
      },
      (scope, failedScopes) => ({
        success: false,
        skipped: `${scope} blocked by ${failedScopes.join(",")}`,
      }),
    );

    expect(calls).toEqual([INNER_SCOPE, OTHER_SCOPE]);
    expect(results.find((entry) => entry.repository_name === OUTER_SCOPE)?.skipped).toBe(true);
    expect(results.find((entry) => entry.repository_name === ROOT_SCOPE)?.skipped).toBe(true);
    expect(results.find((entry) => entry.repository_name === OTHER_SCOPE)?.skipped).toBe(false);
  });
});
