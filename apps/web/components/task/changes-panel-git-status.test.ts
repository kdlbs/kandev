import { describe, expect, it } from "vitest";
import {
  deriveChangesPanelGitStatus,
  deriveChangesPanelToolbarStatus,
} from "./changes-panel-git-status";
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

describe("recoverable Git detail failures", () => {
  it("names a hydrated legacy repository when its details are unavailable", () => {
    const result = deriveChangesPanelGitStatus({
      gitStatus: status({
        repository_name: "frontend",
        status_state: "ready",
        files_complete: true,
        detail_state: "unavailable",
      }),
    });

    expect(result.unavailable).toBe(true);
    expect(result.membershipReady).toBe(true);
    expect(result.failedRepositories).toEqual(["frontend"]);
  });

  it("warns for unavailable details on complete membership without a refresh companion", () => {
    const result = deriveChangesPanelGitStatus({
      statusByRepo: [
        {
          repository_name: "frontend",
          status: status({
            status_state: "ready",
            files_complete: true,
            detail_state: "unavailable",
            files: {
              "src/a.ts": { path: "src/a.ts", status: "modified", staged: false },
            },
          }),
        },
      ],
    });

    expect(result).toMatchObject({
      hasPriorData: true,
      membershipReady: true,
      unavailable: true,
      failedRepositories: ["frontend"],
    });
  });

  it("marks only repositories with unavailable file facets as failed", () => {
    const result = deriveChangesPanelGitStatus({
      statusByRepo: [
        { repository_name: "backend", status: status() },
        {
          repository_name: "frontend",
          status: status({
            files: {
              "src/a.ts": {
                path: "src/a.ts",
                status: "modified",
                staged: false,
                staged_change: { status: "modified", diff_state: "unavailable" },
              },
            },
          }),
        },
      ],
    });

    expect(result).toMatchObject({
      hasPriorData: true,
      membershipReady: true,
      unavailable: true,
      failedRepositories: ["frontend"],
    });
  });

  it.each(["too_large", "binary", "truncated", "budget_exceeded"] as const)(
    "does not warn when unavailable details were intentionally skipped (%s)",
    (diff_skip_reason) => {
      const result = deriveChangesPanelGitStatus({
        statusByRepo: [
          {
            repository_name: "frontend",
            status: status({
              files: {
                "src/a.ts": {
                  path: "src/a.ts",
                  status: "modified",
                  staged: false,
                  unstaged_change: {
                    status: "modified",
                    diff_state: "unavailable",
                    diff_skip_reason,
                  },
                },
              },
            }),
          },
        ],
      });

      expect(result.unavailable).toBe(false);
      expect(result.failedRepositories).toEqual([]);
    },
  );
});

describe("deriveChangesPanelToolbarStatus", () => {
  const ready = deriveChangesPanelGitStatus({ gitStatus: status() });

  it("prioritizes an active Git retry over the unavailable warning", () => {
    const unavailable = deriveChangesPanelGitStatus({
      gitStatus: status({ status_state: "unavailable" }),
      environmentRefresh: { state: "pending" },
    });

    expect(deriveChangesPanelToolbarStatus(unavailable, true)).toBe("loading");
  });

  it("keeps loading priority while detail failure recovery is pending", () => {
    const detailFailure = deriveChangesPanelGitStatus({
      statusByRepo: [
        {
          repository_name: "frontend",
          status: status({ detail_state: "unavailable" }),
        },
      ],
      repositoryRefresh: { frontend: { state: "pending" } },
    });

    expect(deriveChangesPanelToolbarStatus(detailFailure, false)).toBe("loading");
  });

  it("prioritizes unresolved Git failure over unrelated pending details", () => {
    const unavailable = deriveChangesPanelGitStatus({
      gitStatus: status({ status_state: "unavailable", detail_state: "pending" }),
    });

    expect(deriveChangesPanelToolbarStatus(unavailable, true)).toBe("unavailable");
  });

  it("shows loading for commit detail reads and hides the status when ready", () => {
    expect(deriveChangesPanelToolbarStatus(ready, true)).toBe("loading");
    expect(deriveChangesPanelToolbarStatus(ready, false)).toBeNull();
  });
});
