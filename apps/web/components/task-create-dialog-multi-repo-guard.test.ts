import { describe, expect, it } from "vitest";
import { getMultiRepoExecutorDisabledReason } from "./task-create-dialog-multi-repo-guard";

describe("getMultiRepoExecutorDisabledReason", () => {
  it.each(["worktree", "local_docker", "ssh", "sprites"])(
    "allows multi-repository tasks on %s",
    (executorType) => {
      expect(getMultiRepoExecutorDisabledReason(executorType)).toBeNull();
    },
  );

  it.each(["local", "local_pc"])(
    "keeps %s disabled until initial launch supports siblings",
    (executorType) => {
      expect(getMultiRepoExecutorDisabledReason(executorType)).toBe(
        "Multi-repo tasks are unavailable on Local until its initial launch path can project sibling repositories.",
      );
    },
  );

  it("keeps Remote Docker disabled until it can create task instances", () => {
    expect(getMultiRepoExecutorDisabledReason("remote_docker")).toBe(
      "Multi-repo tasks are unavailable on Remote Docker until it supports creating task instances.",
    );
  });

  it("explains that unknown executor types are unsupported", () => {
    expect(getMultiRepoExecutorDisabledReason("remote_vps")).toBe(
      "Multi-repo tasks are not supported by this executor.",
    );
  });
});
