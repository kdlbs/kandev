import { describe, it, expect } from "vitest";
import type { Executor } from "@/lib/types/http";
import { computeExecutorHint } from "./task-create-dialog-options";

function exec(id: string, type: Executor["type"]): Executor {
  return { id, type, name: id } as Executor;
}

const WORKTREE_SINGLE = "A git worktree will be created from the base branch.";
const WORKTREE_MULTI =
  "A git worktree will be created for each repository in a parent folder. The agent runs in that parent folder so it can see every worktree side by side.";
const LOCAL = "The agent will run directly on the repository.";

describe("computeExecutorHint", () => {
  const executors = [exec("wt", "worktree"), exec("loc", "local")];

  it("returns the multi-repo worktree hint when more than one repo is selected", () => {
    expect(computeExecutorHint(executors, "wt", 2)).toBe(WORKTREE_MULTI);
  });

  it("returns the single-repo worktree hint when exactly one repo is selected", () => {
    expect(computeExecutorHint(executors, "wt", 1)).toBe(WORKTREE_SINGLE);
  });

  it("returns the local hint regardless of repoCount", () => {
    expect(computeExecutorHint(executors, "loc", 1)).toBe(LOCAL);
    expect(computeExecutorHint(executors, "loc", 5)).toBe(LOCAL);
  });

  it("returns null for an unknown executor id", () => {
    expect(computeExecutorHint(executors, "nope", 1)).toBeNull();
  });

  it("returns null for an unrecognised executor type", () => {
    const odd = [exec("x", "remote" as Executor["type"])];
    expect(computeExecutorHint(odd, "x", 1)).toBeNull();
  });
});
