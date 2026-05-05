import { describe, it, expect } from "vitest";
import type { Branch } from "@/lib/types/http";
import { sortBranches, branchToOption, computeBranchPlaceholder } from "./task-create-dialog-pill";

function localBranch(name: string): Branch {
  return { name, type: "local" } as Branch;
}

function remoteBranch(name: string, remote = "origin"): Branch {
  return { name, type: "remote", remote } as Branch;
}

describe("sortBranches", () => {
  it("lifts main before master before develop before other branches", () => {
    const sorted = sortBranches([
      localBranch("feature/a"),
      localBranch("develop"),
      localBranch("master"),
      localBranch("main"),
    ]);
    expect(sorted.map((b) => b.name)).toEqual(["main", "master", "develop", "feature/a"]);
  });

  it("puts local before remote when both have the same preferred name", () => {
    const sorted = sortBranches([remoteBranch("main"), localBranch("main")]);
    expect(sorted.map((b) => b.type)).toEqual(["local", "remote"]);
  });

  it("leaves non-preferred branches in their original relative order", () => {
    const sorted = sortBranches([
      localBranch("feature/zeta"),
      localBranch("feature/alpha"),
      localBranch("feature/middle"),
    ]);
    expect(sorted.map((b) => b.name)).toEqual(["feature/zeta", "feature/alpha", "feature/middle"]);
  });

  it("does not mutate the input array", () => {
    const input = [localBranch("feature/a"), localBranch("main")];
    const snapshot = input.map((b) => b.name);
    sortBranches(input);
    expect(input.map((b) => b.name)).toEqual(snapshot);
  });
});

describe("branchToOption keywords", () => {
  function keywords(b: Branch): string[] {
    return branchToOption(b).keywords ?? [];
  }

  it("includes the leaf segment for slash-prefixed branches", () => {
    expect(keywords(localBranch("feat/scope/thing"))).toContain("thing");
  });

  it("splits on slashes, dots, underscores, and hyphens", () => {
    const kw = keywords(localBranch("feat/scope.thing_with-dash"));
    expect(kw).toEqual(expect.arrayContaining(["feat", "scope", "thing", "with", "dash"]));
  });

  it("includes the remote name when present", () => {
    expect(keywords(remoteBranch("main", "upstream"))).toContain("upstream");
  });

  it("dedupes repeated segments", () => {
    const kw = keywords(localBranch("foo/foo"));
    expect(kw.filter((k) => k === "foo")).toHaveLength(1);
  });
});

describe("computeBranchPlaceholder", () => {
  it("returns 'branch' when no repo is selected", () => {
    expect(computeBranchPlaceholder(false, false, 0)).toBe("branch");
  });

  it("returns 'loading…' while branches are loading", () => {
    expect(computeBranchPlaceholder(true, true, 0)).toBe("loading…");
  });

  it("returns 'no branches' when loaded but the list is empty", () => {
    expect(computeBranchPlaceholder(true, false, 0)).toBe("no branches");
  });

  it("returns 'branch' as the default with options available", () => {
    expect(computeBranchPlaceholder(true, false, 3)).toBe("branch");
  });
});
