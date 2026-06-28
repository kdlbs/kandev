import { describe, expect, it, beforeEach } from "vitest";
import { shouldWaitForLastUsedExecutorProfile } from "./task-create-dialog-effects";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import type { StoreSelections } from "@/components/task-create-dialog-types";

const WORKTREE_EXECUTOR_ID = "exec-worktree";
const WORKTREE_PROFILE_ID = "profile-b";

function makeWorktreeExecutor(): StoreSelections["executors"][number] {
  return {
    id: WORKTREE_EXECUTOR_ID,
    type: "worktree",
    profiles: [{ id: WORKTREE_PROFILE_ID, executor_id: WORKTREE_EXECUTOR_ID, name: "B" }],
  } as unknown as StoreSelections["executors"][number];
}

beforeEach(() => {
  window.localStorage.removeItem(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID);
});

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
});
