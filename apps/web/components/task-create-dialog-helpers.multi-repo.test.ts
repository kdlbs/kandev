import { describe, it, expect } from "vitest";
import { buildRepositoriesPayload, findDuplicateRemoteRepo } from "./task-create-dialog-helpers";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import type { PRInfo } from "@/hooks/domains/github/use-pr-info-by-url";

/** Minimal TaskRemoteRepoRow builder for the dedup tests. */
function remoteRow(key: string, url: string): TaskRemoteRepoRow {
  return { key, url, branch: "", source: "paste" };
}

/** Builds a `prInfoByUrl` stub for `buildRepositoriesPayload`. The submit
 * path only ever reads `info(url)` from the per-URL cache; the test stub
 * mirrors that shape with a plain Record lookup. */
function prInfoStub(map: Record<string, PRInfo>) {
  return {
    info: (url: string) => map[url],
  };
}

describe("buildRepositoriesPayload — unified rows", () => {
  it("maps each row in order, dropping empty ones silently", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [
        { key: "r0", repositoryId: "repo-front", branch: "main" },
        { key: "r1", repositoryId: "repo-back", branch: "develop" },
        { key: "r2", branch: "" }, // no repo picked yet — dropped
        { key: "r3", repositoryId: "repo-shared", branch: "" },
      ],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      { repository_id: "repo-front", base_branch: "main", checkout_branch: undefined },
      { repository_id: "repo-back", base_branch: "develop", checkout_branch: undefined },
      { repository_id: "repo-shared", base_branch: undefined, checkout_branch: undefined },
    ]);
  });

  it("emits local_path + default_branch for discovered (on-machine) rows", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [
        { key: "r0", localPath: "/home/me/projects/local-project", branch: "trunk" },
        { key: "r1", repositoryId: "repo-back", branch: "main" },
      ],
      discoveredRepositories: [
        { path: "/home/me/projects/local-project", default_branch: "trunk" },
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ] as any,
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "trunk",
        checkout_branch: undefined,
        local_path: "/home/me/projects/local-project",
        default_branch: "trunk",
      },
      { repository_id: "repo-back", base_branch: "main", checkout_branch: undefined },
    ]);
  });
});

// Regression for the "new branch on local executor" bug: the chip's branch
// is the working branch on disk (e.g. "feature/x"), not the integration
// branch. We must send it as `checkout_branch`, with `base_branch` anchored
// to the repo's `default_branch`. Without this, agentctl recomputes
// merge-base(HEAD, origin/feature/x) which collapses to HEAD and the
// changes panel is empty after refresh.
describe("buildRepositoriesPayload — local executor branch split (core)", () => {
  it("rowBranch != default_branch → swap into checkout_branch", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [{ key: "r0", repositoryId: "repo-1", branch: "feature/x" }],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "main" }] as any,
      isLocalExecutor: true,
    });
    expect(payload).toEqual([
      { repository_id: "repo-1", base_branch: "main", checkout_branch: "feature/x" },
    ]);
  });

  it("rowBranch matches default_branch → no checkout_branch", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [{ key: "r0", repositoryId: "repo-1", branch: "main" }],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "main" }] as any,
      isLocalExecutor: true,
    });
    expect(payload).toEqual([
      { repository_id: "repo-1", base_branch: "main", checkout_branch: undefined },
    ]);
  });

  it("localPath row uses discoveredRepositories.default_branch", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [{ key: "r0", localPath: "/p/r", branch: "feature/y" }],
      discoveredRepositories: [
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        { path: "/p/r", default_branch: "main" } as any,
      ],
      isLocalExecutor: true,
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "main",
        checkout_branch: "feature/y",
        local_path: "/p/r",
        default_branch: "main",
      },
    ]);
  });
});

describe("buildRepositoriesPayload — local executor branch split (edge cases)", () => {
  it("fresh-branch flow: skips the split so the picked base is preserved as base_branch", () => {
    // When the user enables "Fork a new branch", the chip's branch is the
    // BASE TO FORK FROM (e.g. "develop"), not a working branch. The backend
    // creates a new branch from that base and rewrites base_branch to the
    // new branch name. If we split here, base_branch would land on the
    // repo's default ("main") and the fork would happen from main instead
    // of develop — silently wrong.
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [{ key: "r0", repositoryId: "repo-1", branch: "develop" }],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "main" }] as any,
      isLocalExecutor: true,
      freshBranch: { confirmDiscard: false, consentedDirtyFiles: [] },
    });
    expect(payload).toEqual([
      {
        repository_id: "repo-1",
        base_branch: "develop",
        checkout_branch: undefined,
        fresh_branch: true,
        confirm_discard: false,
        consented_dirty_files: [],
      },
    ]);
  });

  it("falls through when default_branch is unknown (legacy repos)", () => {
    // Repos created before the backend probe fix may have an unset
    // default_branch in the workspace store. If we synthesize base_branch=
    // rowBranch here (as the original draft did), we reproduce the very bug
    // this PR fixes: agentctl recomputes merge-base(HEAD, origin/<rowBranch>)
    // → collapses to HEAD → empty changes panel. Better to leave the legacy
    // shape alone — the next backend createRepository call will populate
    // default_branch via the gitref probe.
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [{ key: "r0", repositoryId: "repo-1", branch: "feature/x" }],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "" }] as any,
      isLocalExecutor: true,
    });
    expect(payload).toEqual([
      { repository_id: "repo-1", base_branch: "feature/x", checkout_branch: undefined },
    ]);
  });

  it("non-local executor leaves rowBranch as base_branch (worktree flow unchanged)", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [{ key: "r0", repositoryId: "repo-1", branch: "main" }],
      discoveredRepositories: [],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspaceRepositories: [{ id: "repo-1", default_branch: "main" }] as any,
      isLocalExecutor: false,
    });
    expect(payload).toEqual([
      { repository_id: "repo-1", base_branch: "main", checkout_branch: undefined },
    ]);
  });
});

describe("buildRepositoriesPayload — single-row and URL mode (core)", () => {
  it("URL mode produces a single github_url entry per non-empty row", () => {
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [
        { key: "remote-0", url: "github.com/owner/repo", branch: "feature-x", source: "paste" },
      ],
      repositories: [{ key: "r0", repositoryId: "ignored", branch: "ignored" }],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "feature-x",
        checkout_branch: undefined,
        github_url: "github.com/owner/repo",
      },
    ]);
  });

  it("single-row workspace repo: payload mirrors the row", () => {
    const payload = buildRepositoriesPayload({
      useRemote: false,
      remoteRepos: [],
      repositories: [{ key: "r0", repositoryId: "repo-only", branch: "main" }],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      { repository_id: "repo-only", base_branch: "main", checkout_branch: undefined },
    ]);
  });
});

describe("buildRepositoriesPayload — PR URL inference (per-row prInfoByUrl)", () => {
  // Fork PR: the displayed branch equals the PR head (auto-selected for visual
  // consistency with the pasted URL), but that branch doesn't live on origin.
  // The payload must anchor base_branch to the PR's actual target (from the
  // GitHub API) so the backend has a ref it can resolve.
  it("fork PR auto-selection: base_branch comes from PR's target, not displayed branch", () => {
    const url = "https://github.com/kdlbs/kandev/pull/977";
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [{ key: "remote-0", url, branch: "jira-hosted-path-auth", source: "paste" }],
      prInfoByUrl: prInfoStub({
        [url]: {
          prHeadBranch: "jira-hosted-path-auth",
          prBaseBranch: "main",
          prNumber: 977,
          suggestedTitle: "PR #977: x",
        },
      }),
      repositories: [],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "main",
        checkout_branch: "jira-hosted-path-auth",
        github_url: url,
      },
    ]);
  });

  // User picked a non-PR-head branch from the dropdown after pasting a PR URL.
  // We respect their override and drop checkout_branch entirely: their pick is
  // treated as the base they want to work from, not as a PR-head checkout.
  it("user-overridden base branch beats PR's target when row.branch differs from PR head", () => {
    const url = "https://github.com/owner/repo/pull/42";
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [{ key: "remote-0", url, branch: "develop", source: "paste" }],
      prInfoByUrl: prInfoStub({
        [url]: {
          prHeadBranch: "feature/x",
          prBaseBranch: "main",
          prNumber: 42,
          suggestedTitle: "PR #42: x",
        },
      }),
      repositories: [],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "develop",
        checkout_branch: undefined,
        github_url: url,
      },
    ]);
  });

  // Same-repo PR: PR head exists on origin, so base_branch = PR head is fine.
  // PR's base from API is still preferred when available, so the auto-selected
  // case still anchors to the PR's actual target rather than the head.
  it("same-repo PR auto-selection: still prefers PR target over PR head", () => {
    const url = "https://github.com/owner/repo/pull/10";
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [{ key: "remote-0", url, branch: "feature/x", source: "paste" }],
      prInfoByUrl: prInfoStub({
        [url]: {
          prHeadBranch: "feature/x",
          prBaseBranch: "main",
          prNumber: 10,
          suggestedTitle: "PR #10: x",
        },
      }),
      repositories: [],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "main",
        checkout_branch: "feature/x",
        github_url: url,
      },
    ]);
  });
});

describe("buildRepositoriesPayload — checkout_branch gating on PR-auto-selection", () => {
  // Regression for PR review feedback: checkout_branch was set even when the
  // user had overridden the row's branch to something other than the PR head.
  // The contract is: checkout_branch is only meaningful when we're carrying
  // forward the PR's head as the auto-selection. Any user override drops it.
  const url = "https://github.com/owner/repo/pull/42";
  const prInfo = {
    prHeadBranch: "feature/x",
    prBaseBranch: "main",
    prNumber: 42,
    suggestedTitle: "PR #42: x",
  };

  it("row.branch === prHeadBranch → payload carries checkout_branch", () => {
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [{ key: "remote-0", url, branch: "feature/x", source: "paste" }],
      prInfoByUrl: prInfoStub({ [url]: prInfo }),
      repositories: [],
      discoveredRepositories: [],
    });
    expect(payload[0]).toMatchObject({ checkout_branch: "feature/x" });
  });

  it("row.branch !== prHeadBranch → payload has NO checkout_branch", () => {
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [{ key: "remote-0", url, branch: "develop", source: "paste" }],
      prInfoByUrl: prInfoStub({ [url]: prInfo }),
      repositories: [],
      discoveredRepositories: [],
    });
    expect(payload[0]?.checkout_branch).toBeUndefined();
  });
});

describe("buildRepositoriesPayload — multi-row PR/repo mix", () => {
  it("multiple PR rows: each row resolves base/checkout independently from its own PR info", () => {
    const urlA = "https://github.com/acme/site/pull/1";
    const urlB = "https://github.com/acme/api/pull/2";
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [
        { key: "r0", url: urlA, branch: "fork-a", source: "paste" },
        { key: "r1", url: urlB, branch: "fork-b", source: "paste" },
      ],
      prInfoByUrl: prInfoStub({
        [urlA]: {
          prHeadBranch: "fork-a",
          prBaseBranch: "main",
          prNumber: 1,
          suggestedTitle: "PR #1: a",
        },
        [urlB]: {
          prHeadBranch: "fork-b",
          prBaseBranch: "trunk",
          prNumber: 2,
          suggestedTitle: "PR #2: b",
        },
      }),
      repositories: [],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "main",
        checkout_branch: "fork-a",
        github_url: urlA,
      },
      {
        repository_id: "",
        base_branch: "trunk",
        checkout_branch: "fork-b",
        github_url: urlB,
      },
    ]);
  });

  it("non-PR rows (plain repo URL) skip PR inference and use row.branch as base_branch", () => {
    // Mixed list: row 0 is a PR (anchored to PR base), row 1 is a plain repo
    // URL (its branch is the integration branch as-is). prInfoByUrl.info()
    // returns undefined for the repo URL so the PR branch is left alone.
    const prUrl = "https://github.com/acme/site/pull/1";
    const repoUrl = "https://github.com/acme/api";
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [
        { key: "r0", url: prUrl, branch: "fork-a", source: "paste" },
        { key: "r1", url: repoUrl, branch: "develop", source: "paste" },
      ],
      prInfoByUrl: prInfoStub({
        [prUrl]: {
          prHeadBranch: "fork-a",
          prBaseBranch: "main",
          prNumber: 1,
          suggestedTitle: "PR #1: a",
        },
      }),
      repositories: [],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      { repository_id: "", base_branch: "main", checkout_branch: "fork-a", github_url: prUrl },
      {
        repository_id: "",
        base_branch: "develop",
        checkout_branch: undefined,
        github_url: repoUrl,
      },
    ]);
  });
});

describe("buildRepositoriesPayload — URL trimming for prInfoByUrl lookup", () => {
  // Regression: the cache lookup in buildRepositoriesPayload must use the
  // same canonical (trimmed) URL key that the rest of the dialog uses for
  // ensure() / info(). Without trimming, stray whitespace on the pasted URL
  // would silently miss the PR-info cache and skip base-branch anchoring.
  it("trims row.url before looking up prInfoByUrl.info", () => {
    const canonical = "https://github.com/owner/repo/pull/42";
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [
        { key: "remote-0", url: `  ${canonical}  `, branch: "feature/x", source: "paste" },
      ],
      prInfoByUrl: prInfoStub({
        [canonical]: {
          prHeadBranch: "feature/x",
          prBaseBranch: "main",
          prNumber: 42,
          suggestedTitle: "PR #42: x",
        },
      }),
      repositories: [],
      discoveredRepositories: [],
    });
    expect(payload).toEqual([
      {
        repository_id: "",
        base_branch: "main",
        checkout_branch: "feature/x",
        github_url: canonical,
      },
    ]);
  });
});

describe("findDuplicateRemoteRepo", () => {
  const PR_1116 = "https://github.com/kdlbs/kandev/pull/1116";
  const REPO_URL = "https://github.com/kdlbs/kandev";
  const REPO_LABEL = "kdlbs/kandev";

  it("flags two rows with the identical URL, naming the repo", () => {
    expect(findDuplicateRemoteRepo([remoteRow("r0", PR_1116), remoteRow("r1", PR_1116)])).toBe(
      REPO_LABEL,
    );
  });

  it("flags two different PRs of the same repo", () => {
    expect(
      findDuplicateRemoteRepo([
        remoteRow("r0", PR_1116),
        remoteRow("r1", "https://github.com/kdlbs/kandev/pull/1117"),
      ]),
    ).toBe(REPO_LABEL);
  });

  it("flags a PR URL and a plain repo URL of the same repo", () => {
    expect(findDuplicateRemoteRepo([remoteRow("r0", PR_1116), remoteRow("r1", REPO_URL)])).toBe(
      REPO_LABEL,
    );
  });

  it("compares case-insensitively but reports the first row's casing", () => {
    expect(
      findDuplicateRemoteRepo([
        remoteRow("r0", "https://github.com/KdLbS/Kandev"),
        remoteRow("r1", "https://github.com/kdlbs/kandev/pull/9"),
      ]),
    ).toBe("KdLbS/Kandev");
  });

  it("returns null for genuinely different repos", () => {
    expect(
      findDuplicateRemoteRepo([
        remoteRow("r0", PR_1116),
        remoteRow("r1", "https://github.com/kdlbs/other/pull/1"),
      ]),
    ).toBeNull();
  });

  it("ignores empty and unparseable rows (no false positives)", () => {
    expect(
      findDuplicateRemoteRepo([
        remoteRow("r0", ""),
        remoteRow("r1", "not a url"),
        remoteRow("r2", "totally garbage"),
        remoteRow("r3", REPO_URL),
      ]),
    ).toBeNull();
  });
});
