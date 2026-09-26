import { describe, expect, it } from "vitest";
import type { FileInfo, GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { deriveMultiRepoSummary } from "./use-session-git-summary";

type StatusByRepo = Parameters<typeof deriveMultiRepoSummary>[0];
const ROOT_SCOPE = "";
const OUTER_SCOPE = "vendor/outer";
const INNER_SCOPE = "vendor/outer/vendor/inner";
const NEW_BRANCH = "new-branch";

function status(overrides: Partial<GitStatusEntry> = {}): GitStatusEntry {
  return {
    branch: "main",
    remote_branch: "origin/main",
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

function repoStatus(repository_name: string, overrides: Partial<GitStatusEntry> = {}) {
  return { repository_name, status: status(overrides) } satisfies StatusByRepo[number];
}

function file(repository_name?: string, staged = false): FileInfo {
  return { path: "README.md", status: "modified", staged, repository_name };
}

describe("deriveMultiRepoSummary", () => {
  it("keeps the root control and status beside nested scopes", () => {
    const result = deriveMultiRepoSummary(
      [repoStatus(ROOT_SCOPE, { ahead: 1 }), repoStatus(OUTER_SCOPE), repoStatus(INNER_SCOPE)],
      [file(ROOT_SCOPE, true), file(INNER_SCOPE)],
      [INNER_SCOPE, OUTER_SCOPE, ROOT_SCOPE],
    );

    expect(result.repoNamesForControls).toEqual([ROOT_SCOPE, OUTER_SCOPE, INNER_SCOPE]);
    expect(result.perRepoStatus).toEqual([
      expect.objectContaining({ repository_name: ROOT_SCOPE, ahead: 1, hasStaged: true }),
      expect.objectContaining({ repository_name: OUTER_SCOPE }),
      expect.objectContaining({
        repository_name: INNER_SCOPE,
        hasUnstaged: true,
      }),
    ]);
  });

  it("omits a bare workspace root when only named repositories have status", () => {
    const result = deriveMultiRepoSummary(
      [repoStatus("backend"), repoStatus("frontend", { ahead: 2 })],
      [file("frontend")],
      ["frontend"],
    );

    expect(result.repoNamesForControls).toEqual(["backend", "frontend"]);
    expect(result.perRepoStatus.map(({ repository_name }) => repository_name)).toEqual([
      "backend",
      "frontend",
    ]);
  });

  it("does not promote a file-only root into per-repo status", () => {
    const result = deriveMultiRepoSummary(
      [repoStatus(OUTER_SCOPE)],
      [file(ROOT_SCOPE), file(OUTER_SCOPE)],
      [ROOT_SCOPE, OUTER_SCOPE],
    );

    expect(result.repoNamesForControls).toEqual([OUTER_SCOPE]);
    expect(result.perRepoStatus).toEqual([
      expect.objectContaining({ repository_name: OUTER_SCOPE, hasUnstaged: true }),
    ]);
  });

  it("reports both layers for one mixed file", () => {
    const mixed = {
      ...file("frontend"),
      staged_change: { status: "modified" as const },
      unstaged_change: { status: "modified" as const },
    };
    const result = deriveMultiRepoSummary([repoStatus("frontend")], [mixed], ["frontend"]);

    expect(result.perRepoStatus).toEqual([
      expect.objectContaining({
        repository_name: "frontend",
        hasStaged: true,
        hasUnstaged: true,
      }),
    ]);
  });

  it("keeps base divergence separate from upstream push and pull counts", () => {
    const result = deriveMultiRepoSummary(
      [
        repoStatus("contribution", {
          ahead: 7,
          behind: 2,
          remote_branch: "contributor/feature",
          remote_ahead: 1,
          remote_behind: 0,
        }),
      ],
      [file("contribution")],
      ["contribution"],
    );

    expect(result.perRepoStatus).toEqual([
      expect.objectContaining({
        repository_name: "contribution",
        ahead: 7,
        behind: 2,
        remoteAhead: 1,
        remoteBehind: 0,
        pushAhead: 1,
        pullBehind: 0,
        hasUpstream: true,
      }),
    ]);
  });

  it("falls back to base-ahead only when no upstream exists", () => {
    const result = deriveMultiRepoSummary(
      [repoStatus(NEW_BRANCH, { ahead: 3, behind: 0, remote_branch: null })],
      [file(NEW_BRANCH)],
      [NEW_BRANCH],
    );

    expect(result.perRepoStatus).toEqual([
      expect.objectContaining({
        repository_name: NEW_BRANCH,
        pushAhead: 3,
        pullBehind: 0,
        hasUpstream: false,
      }),
    ]);
  });
});
