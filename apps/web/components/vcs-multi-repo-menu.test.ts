import { describe, expect, it } from "vitest";
import { isRemoteActionBlockedForRepository } from "./vcs-multi-repo-menu";

describe("multi-repository remote action policy", () => {
  it("blocks only the selected PR repository when its history diverged", () => {
    expect(isRemoteActionBlockedForRepository("push", "frontend", "frontend", true, true)).toBe(
      true,
    );
    expect(isRemoteActionBlockedForRepository("push", "backend", "frontend", true, true)).toBe(
      false,
    );
    expect(isRemoteActionBlockedForRepository("pull", "backend", "frontend", true, true)).toBe(
      false,
    );
  });

  it("fails closed for every repository when the blocked repository is unknown", () => {
    expect(isRemoteActionBlockedForRepository("pull", "frontend", undefined, false, true)).toBe(
      true,
    );
  });
});
