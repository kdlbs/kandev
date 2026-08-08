import { describe, it, expect } from "vitest";
import { groupPathsByRepoName, repositoryScopesForMutation } from "./use-session-git";

describe("groupPathsByRepoName", () => {
  it("splits paths by repository_name, preserving insertion order per bucket", () => {
    const lookup = new Map([
      ["src/app.tsx", "frontend"],
      ["src/api.ts", "frontend"],
      ["handlers/task.go", "backend"],
    ]);
    const out = groupPathsByRepoName(["src/app.tsx", "handlers/task.go", "src/api.ts"], lookup);
    expect(out.get("frontend")).toEqual(["src/app.tsx", "src/api.ts"]);
    expect(out.get("backend")).toEqual(["handlers/task.go"]);
    expect(out.size).toBe(2);
  });

  it("falls back to empty-string bucket for paths missing a repo", () => {
    const out = groupPathsByRepoName(["unknown.txt"], new Map());
    expect(out.get("")).toEqual(["unknown.txt"]);
  });

  it("preserves single-repo callers (everything in one bucket)", () => {
    const lookup = new Map([
      ["a.ts", "only"],
      ["b.ts", "only"],
    ]);
    const out = groupPathsByRepoName(["a.ts", "b.ts"], lookup);
    expect(out.size).toBe(1);
    expect(out.get("only")).toEqual(["a.ts", "b.ts"]);
  });
});

describe("repositoryScopesForMutation", () => {
  it("excludes a file-only root when only named scopes have trackers", () => {
    expect(
      repositoryScopesForMutation(
        [{ repository_name: undefined }, { repository_name: "vendor" }],
        ["vendor"],
      ),
    ).toEqual(["vendor"]);
  });

  it("keeps an available root and adds available ancestors", () => {
    expect(
      repositoryScopesForMutation(
        [{ repository_name: "vendor/inner" }],
        ["", "vendor", "vendor/inner"],
      ),
    ).toEqual(["vendor/inner", "vendor", ""]);
  });

  it("preserves the legacy root fallback without per-repo status", () => {
    expect(repositoryScopesForMutation([{ repository_name: undefined }], [])).toEqual([""]);
  });
});
