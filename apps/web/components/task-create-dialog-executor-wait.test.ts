import { describe, expect, it } from "vitest";
import { shouldWaitForLastUsedExecutorProfile } from "./task-create-dialog-effects";
import type { StoreSelections } from "@/components/task-create-dialog-types";

const WORKTREE_EXECUTOR_ID = "exec-worktree";
const WORKTREE_PROFILE_ID = "profile-b";
const LOCAL_EXECUTOR_ID = "exec-local";
const LOCAL_PROFILE_ID = "profile-local";

function makeWorktreeExecutor(): StoreSelections["executors"][number] {
  return {
    id: WORKTREE_EXECUTOR_ID,
    type: "worktree",
    profiles: [{ id: WORKTREE_PROFILE_ID, executor_id: WORKTREE_EXECUTOR_ID, name: "B" }],
  } as unknown as StoreSelections["executors"][number];
}

function makeLocalExecutor(): StoreSelections["executors"][number] {
  return {
    id: LOCAL_EXECUTOR_ID,
    type: "local",
    profiles: [{ id: LOCAL_PROFILE_ID, executor_id: LOCAL_EXECUTOR_ID, name: "Local" }],
  } as unknown as StoreSelections["executors"][number];
}

describe("shouldWaitForLastUsedExecutorProfile", () => {
  it("waits only while a valid last-used executor profile can restore", () => {
    const worktreeExecutor = makeWorktreeExecutor();

    expect(
      shouldWaitForLastUsedExecutorProfile({
        executors: [worktreeExecutor],
        workspaceDefaults: null,
        lastUsedExecutorProfileId: WORKTREE_PROFILE_ID,
        noRepository: false,
        preferLocalExecutor: false,
      }),
    ).toBe(true);
    expect(
      shouldWaitForLastUsedExecutorProfile({
        executors: [worktreeExecutor],
        workspaceDefaults: null,
        lastUsedExecutorProfileId: "missing-profile",
        noRepository: false,
        preferLocalExecutor: false,
      }),
    ).toBe(false);
  });

  it("does not wait for a valid profile from another resolved executor", () => {
    expect(
      shouldWaitForLastUsedExecutorProfile({
        executors: [makeLocalExecutor(), makeWorktreeExecutor()],
        workspaceDefaults: null,
        lastUsedExecutorProfileId: LOCAL_PROFILE_ID,
        noRepository: false,
        preferLocalExecutor: false,
      }),
    ).toBe(false);
  });
});
