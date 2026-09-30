import { describe, expect, it } from "vitest";
import { deriveChangesPanelGitStatus } from "./changes-panel-git-status";
import type {
  GitStatusEntry,
  GitStatusRefreshState,
} from "@/lib/state/slices/session-runtime/types";

function status(overrides: Partial<GitStatusEntry> = {}): GitStatusEntry {
  return {
    branch: null,
    remote_branch: null,
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

describe("deriveChangesPanelGitStatus", () => {
  it("allows the clean state only after complete successful membership", () => {
    const result = deriveChangesPanelGitStatus({ gitStatus: status() });

    expect(result).toMatchObject({
      hasPriorData: true,
      membershipReady: true,
      loading: false,
      unavailable: false,
      detailsPending: false,
      failedRepositories: [],
    });
  });

  it("keeps the empty state hidden while the first membership request is pending", () => {
    const pending: GitStatusRefreshState = { state: "pending" };
    const result = deriveChangesPanelGitStatus({ environmentRefresh: pending });

    expect(result.hasPriorData).toBe(false);
    expect(result.membershipReady).toBe(false);
    expect(result.loading).toBe(true);
    expect(result.showEmpty).toBe(false);
  });

  it("shows failure while preserving prior complete membership", () => {
    const result = deriveChangesPanelGitStatus({
      gitStatus: status({
        files: { "src/a.ts": { path: "src/a.ts", status: "modified", staged: false } },
      }),
      environmentRefresh: { state: "unavailable", error_code: "status_timeout" },
    });

    expect(result.hasPriorData).toBe(true);
    expect(result.unavailable).toBe(true);
    expect(result.membershipReady).toBe(false);
    expect(result.showEmpty).toBe(false);
  });

  it("reports failed repositories without hiding complete sibling files", () => {
    const result = deriveChangesPanelGitStatus({
      statusByRepo: [{ repository_name: "frontend", status: status() }],
      repositoryRefresh: {
        backend: { state: "unavailable", error_code: "status_timeout" },
      },
    });

    expect(result.hasPriorData).toBe(true);
    expect(result.unavailable).toBe(true);
    expect(result.failedRepositories).toEqual(["backend"]);
  });

  it("keeps file membership ready while reporting pending diff details", () => {
    const result = deriveChangesPanelGitStatus({
      gitStatus: status({
        detail_state: "pending",
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: false,
            diff_state: "pending",
          },
        },
      }),
    });

    expect(result.membershipReady).toBe(true);
    expect(result.detailsPending).toBe(true);
  });

  it("rejects summary-only and unavailable entries as membership", () => {
    const summaryOnly = deriveChangesPanelGitStatus({
      gitStatus: status({ files: undefined, files_complete: false }),
      environmentRefresh: { state: "pending" },
    });
    const unavailable = deriveChangesPanelGitStatus({
      gitStatus: status({ status_state: "unavailable" }),
    });

    expect(summaryOnly.hasPriorData).toBe(false);
    expect(summaryOnly.loading).toBe(true);
    expect(unavailable.membershipReady).toBe(false);
    expect(unavailable.unavailable).toBe(true);
  });
});
