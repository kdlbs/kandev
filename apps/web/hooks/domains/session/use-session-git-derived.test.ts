import { describe, expect, it } from "vitest";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { deriveSessionGitValues } from "./use-session-git-derived";

function status(overrides: Partial<GitStatusEntry> = {}): GitStatusEntry {
  return {
    branch: "feature/contribution",
    remote_branch: "contributor/feature",
    modified: [],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files: {},
    timestamp: null,
    ...overrides,
  };
}

describe("deriveSessionGitValues", () => {
  it("uses upstream-relative counts for remote actions", () => {
    const result = deriveSessionGitValues(
      status({
        ahead: 7,
        behind: 2,
        remote_ahead: 1,
        remote_behind: 3,
      }),
      false,
      [],
      [],
      [],
    );

    expect(result).toMatchObject({
      ahead: 7,
      behind: 2,
      remoteAhead: 1,
      remoteBehind: 3,
      pushAhead: 1,
      pullBehind: 3,
      canPush: true,
      canPull: true,
    });
  });

  it("falls back to base-ahead for a branch without an upstream", () => {
    const result = deriveSessionGitValues(
      status({
        remote_branch: null,
        ahead: 4,
        remote_ahead: 8,
        remote_behind: 2,
      }),
      false,
      [],
      [],
      [],
    );

    expect(result).toMatchObject({
      remoteAhead: 8,
      remoteBehind: 2,
      pushAhead: 4,
      pullBehind: 0,
      canPush: true,
      canPull: false,
    });
  });
});
