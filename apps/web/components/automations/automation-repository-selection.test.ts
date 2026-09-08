import { describe, expect, it } from "vitest";
import {
  normalizeRepositorySelections,
  resolveExecutorType,
  type ExecutorLike,
  type RepositorySelection,
} from "./automation-repository-selection";

describe("resolveExecutorType", () => {
  const executors: ExecutorLike[] = [
    {
      type: "worktree",
      profiles: [{ id: "profile-worktree" }],
    },
    {
      type: "local_pc",
      profiles: [{ id: "profile-local" }],
    },
    {
      // Excluded from resolution, mirroring config-section.tsx's existing
      // "local" system-executor exclusion.
      type: "local",
      profiles: [{ id: "profile-hidden-local" }],
    },
  ];

  it("resolves the executor type for a known profile id", () => {
    expect(resolveExecutorType(executors, "profile-worktree")).toBe("worktree");
    expect(resolveExecutorType(executors, "profile-local")).toBe("local_pc");
  });

  it("returns undefined for an unknown profile id", () => {
    expect(resolveExecutorType(executors, "profile-missing")).toBeUndefined();
  });

  it("excludes profiles belonging to the 'local' system executor", () => {
    expect(resolveExecutorType(executors, "profile-hidden-local")).toBeUndefined();
  });

  it("prefers a profile's own executor_type over its parent executor's type", () => {
    const withOverride: ExecutorLike[] = [
      {
        type: "worktree",
        profiles: [{ id: "profile-override", executor_type: "ssh" }],
      },
    ];
    expect(resolveExecutorType(withOverride, "profile-override")).toBe("ssh");
  });
});

describe("normalizeRepositorySelections", () => {
  const two: RepositorySelection[] = [
    { kind: "registered", id: "repo-a" },
    { kind: "registered", id: "repo-b" },
  ];

  it("keeps every selection when the executor supports multi-repo", () => {
    const result = normalizeRepositorySelections(two, {
      supportsMultiRepo: true,
    });
    expect(result).toEqual(two);
  });

  it("truncates to the first selection when the executor doesn't support multi-repo", () => {
    const result = normalizeRepositorySelections(two, {
      supportsMultiRepo: false,
    });
    expect(result).toEqual([{ kind: "registered", id: "repo-a" }]);
  });

  it("leaves an empty or single-entry list untouched", () => {
    expect(normalizeRepositorySelections([], { supportsMultiRepo: false })).toEqual([]);
    expect(normalizeRepositorySelections([two[0]], { supportsMultiRepo: false })).toEqual([two[0]]);
  });
});
