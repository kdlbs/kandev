import { describe, it, expect } from "vitest";
import { buildFileTree, type ReviewFile } from "./types";

function file(overrides: Partial<ReviewFile>): ReviewFile {
  return {
    path: "src/app.tsx",
    diff: "",
    status: "modified",
    additions: 1,
    deletions: 0,
    staged: false,
    source: "uncommitted",
    ...overrides,
  };
}

describe("buildFileTree — multi-repo", () => {
  it("falls back to flat tree when files have no repository_name", () => {
    const tree = buildFileTree([file({ path: "src/a.ts" }), file({ path: "src/b.ts" })]);
    expect(tree[0].isRepoRoot).toBeUndefined();
    expect(tree[0].name).toContain("src");
  });

  it("falls back to flat tree when only one repository is present", () => {
    const tree = buildFileTree([
      file({ path: "src/a.ts", repository_name: "frontend", repository_id: "f" }),
      file({ path: "src/b.ts", repository_name: "frontend", repository_id: "f" }),
    ]);
    expect(tree[0].isRepoRoot).toBeUndefined();
  });

  it("groups by repo when 2+ distinct repositories are present", () => {
    const tree = buildFileTree([
      file({ path: "src/app.tsx", repository_name: "frontend", repository_id: "f" }),
      file({ path: "src/api.ts", repository_name: "frontend", repository_id: "f" }),
      file({ path: "handlers/task.go", repository_name: "backend", repository_id: "b" }),
    ]);
    expect(tree).toHaveLength(2);
    expect(tree.map((n) => n.name).sort()).toEqual(["backend", "frontend"]);
    for (const root of tree) {
      expect(root.isRepoRoot).toBe(true);
      expect(root.repositoryId).toBeDefined();
    }
  });

  it("repo roots are not collapsed even when they have a single child", () => {
    const tree = buildFileTree([
      file({ path: "lonely.ts", repository_name: "shared", repository_id: "s" }),
      file({ path: "src/x.ts", repository_name: "main", repository_id: "m" }),
    ]);
    const shared = tree.find((n) => n.name === "shared");
    expect(shared).toBeDefined();
    expect(shared?.isRepoRoot).toBe(true);
    expect(shared?.children).toHaveLength(1);
    expect(shared?.children?.[0].name).toBe("lonely.ts");
  });

  it("preserves file paths inside each repo (no leakage between repos)", () => {
    const tree = buildFileTree([
      file({ path: "README.md", repository_name: "frontend", repository_id: "f" }),
      file({ path: "README.md", repository_name: "backend", repository_id: "b" }),
    ]);
    const frontend = tree.find((n) => n.name === "frontend");
    const backend = tree.find((n) => n.name === "backend");
    expect(frontend?.children?.[0].name).toBe("README.md");
    expect(backend?.children?.[0].name).toBe("README.md");
    // File node back-refs point to the right repo.
    expect(frontend?.children?.[0].file?.repository_id).toBe("f");
    expect(backend?.children?.[0].file?.repository_id).toBe("b");
  });
});
