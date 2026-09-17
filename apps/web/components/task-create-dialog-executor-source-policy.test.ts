import { describe, expect, it } from "vitest";
import {
  deriveExecutorSourcePolicy,
  executorSourceIncompatibilityReasonKey,
  executorSourcePolicyReasonKey,
  shouldAutoSwitchFolderOnlyExecutor,
} from "./task-create-dialog-executor-source-policy";

describe("deriveExecutorSourcePolicy", () => {
  it("allows folders for Worktree and reports mixed sources", () => {
    const policy = deriveExecutorSourcePolicy({
      executorType: "worktree",
      counts: { sourceCount: 2, repositoryCount: 1, folderCount: 1 },
    });
    expect(policy).toMatchObject({
      mode: "mixed",
      folderAvailable: true,
      folderDisabledReason: undefined,
      incompatible: false,
    });
  });

  it("disables folders for a known remote executor with a recoverable reason", () => {
    const policy = deriveExecutorSourcePolicy({
      executorType: "ssh",
      counts: { sourceCount: 0, repositoryCount: 0, folderCount: 0 },
    });
    expect(policy.folderAvailable).toBe(false);
    expect(executorSourcePolicyReasonKey(policy.folderDisabledReason)).toBe(
      "task:foldersRequireHostExecutor",
    );
  });

  it("keeps unresolved executor state distinct from unsupported state", () => {
    const policy = deriveExecutorSourcePolicy({
      executorType: null,
      counts: { sourceCount: 0, repositoryCount: 0, folderCount: 0 },
    });
    expect(policy.folderDisabledReason).toBe("unknown_executor");
    expect(executorSourcePolicyReasonKey(policy.folderDisabledReason)).toBe(
      "task:addFolderExecutorUnknown",
    );
  });

  it("marks a retained folder as incompatible after a remote executor switch", () => {
    const policy = deriveExecutorSourcePolicy({
      executorType: "remote_docker",
      counts: { sourceCount: 1, repositoryCount: 0, folderCount: 1 },
    });
    expect(policy).toMatchObject({
      mode: "folder",
      incompatible: true,
      incompatibleReason: "folders_require_host_executor",
    });
  });

  it("blocks a local repository until its remote origin is verified", () => {
    const policy = deriveExecutorSourcePolicy({
      executorType: "ssh",
      counts: {
        sourceCount: 1,
        repositoryCount: 1,
        folderCount: 0,
        localRepositoryCount: 1,
        remoteOriginRepositoryCount: 0,
      },
    });
    expect(policy.incompatibleReason).toBe("repository_requires_remote_origin");
    expect(executorSourceIncompatibilityReasonKey(policy.incompatibleReason)).toBe(
      "task:repositoryRequiresRemoteOrigin",
    );
  });
});

describe("shouldAutoSwitchFolderOnlyExecutor", () => {
  it("only applies to an untouched empty Worktree draft", () => {
    const counts = { sourceCount: 0, repositoryCount: 0, folderCount: 0 };
    expect(
      shouldAutoSwitchFolderOnlyExecutor({
        executorType: "worktree",
        counts,
        executorChoiceTouched: false,
      }),
    ).toBe(true);
    expect(
      shouldAutoSwitchFolderOnlyExecutor({
        executorType: "worktree",
        counts,
        executorChoiceTouched: true,
      }),
    ).toBe(false);
  });
});
