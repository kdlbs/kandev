import { describe, expect, it } from "vitest";

import { resolveDiffExpansion } from "./review-diff-list";
import type { ReviewFile } from "./types";

const BACKEND_BASE = "release/24.x";
const MULTI_REPO_BASES = { frontend: "main", backend: BACKEND_BASE };

function file(partial: Partial<ReviewFile>): ReviewFile {
  return {
    path: "src/example.ts",
    diff: "@@ -1 +1 @@\n-a\n+b\n",
    status: "modified",
    additions: 1,
    deletions: 1,
    staged: false,
    source: "uncommitted",
    ...partial,
  };
}

describe("resolveDiffExpansion", () => {
  it("enables expansion for uncommitted (working-tree) files against HEAD", () => {
    expect(resolveDiffExpansion(file({ source: "uncommitted" }), {})).toEqual({
      enableExpansion: true,
      baseRef: "HEAD",
    });
  });

  // Regression: #1097 narrowed the gate to `source === "uncommitted"`, which
  // dropped expansion for committed-source rows — the bulk of a review. They
  // must expand against the repo's base branch (not HEAD, which already
  // contains the committed changes).
  it("enables expansion for committed files against the repo base branch", () => {
    expect(
      resolveDiffExpansion(file({ source: "committed", repository_name: undefined }), {}, "main"),
    ).toEqual({ enableExpansion: true, baseRef: "main" });
  });

  it("uses the per-repo base branch for committed multi-repo files", () => {
    const f = file({ source: "committed", repository_name: "backend" });
    expect(resolveDiffExpansion(f, MULTI_REPO_BASES, "main")).toEqual({
      enableExpansion: true,
      baseRef: BACKEND_BASE,
    });
  });

  it("prefers the exact cumulative-diff base ref for a committed branch worktree", () => {
    const f = {
      ...file({ source: "committed", repository_name: "backend-feature-x" }),
      base_ref: "4f5c2f7",
    } as ReviewFile;
    expect(resolveDiffExpansion(f, MULTI_REPO_BASES, "main")).toEqual({
      enableExpansion: true,
      baseRef: "4f5c2f7",
    });
  });

  // Multi-repo guard: a committed file whose repo isn't in the map must NOT
  // borrow the single-repo fallback (another repo's base branch) — that would
  // fetch the wrong "old" content and silently drop expansion. Disable instead.
  it("does not borrow another repo's base branch for an unmapped multi-repo file", () => {
    const f = file({ source: "committed", repository_name: "infra" });
    expect(resolveDiffExpansion(f, MULTI_REPO_BASES, "main")).toEqual({
      enableExpansion: false,
      baseRef: "HEAD",
    });
  });

  // Multi-repo guard, unnamed-file case: ReviewDiffList passes an undefined
  // fallback when the task has multiple repos, so a committed file with no
  // repository_name disables expansion rather than borrowing an arbitrary base.
  it("disables expansion for an unnamed committed file when the fallback is undefined", () => {
    const f = file({ source: "committed", repository_name: undefined });
    expect(resolveDiffExpansion(f, MULTI_REPO_BASES, undefined)).toEqual({
      enableExpansion: false,
      baseRef: "HEAD",
    });
  });

  // Single-repo files carry no repository_name; the map is keyed by the real
  // repo name, so the lookup misses and the sole-base fallback applies.
  it("uses the fallback base branch for a single-repo committed file", () => {
    const f = file({ source: "committed", repository_name: undefined });
    expect(resolveDiffExpansion(f, { "E2E Repo": "main" }, "main")).toEqual({
      enableExpansion: true,
      baseRef: "main",
    });
  });

  // Uncommitted files always diff against HEAD regardless of repo; the repo
  // scoping for content fetches happens via the separate `repo` prop.
  it("expands uncommitted multi-repo files against HEAD", () => {
    const f = file({ source: "uncommitted", repository_name: "backend" });
    expect(resolveDiffExpansion(f, MULTI_REPO_BASES)).toEqual({
      enableExpansion: true,
      baseRef: "HEAD",
    });
  });

  it("disables expansion for committed files when no base branch is known", () => {
    expect(resolveDiffExpansion(file({ source: "committed" }), {})).toEqual({
      enableExpansion: false,
      baseRef: "HEAD",
    });
  });

  it("disables expansion for PR files (working tree is not the PR head)", () => {
    expect(resolveDiffExpansion(file({ source: "pr" }), {}, "main")).toEqual({
      enableExpansion: false,
      baseRef: "HEAD",
    });
  });

  it("disables expansion for untracked files (synthetic /dev/null hunk)", () => {
    expect(resolveDiffExpansion(file({ source: "uncommitted", status: "untracked" }), {})).toEqual({
      enableExpansion: false,
      baseRef: "HEAD",
    });
  });
});
