import { describe, it, expect } from "vitest";
import {
  buildFileTree,
  getCumulativeReviewRepositoryNames,
  isReviewMultiRepo,
  resolvePRReviewRepositoryName,
  reviewFileKey,
  sanitizeReviewRepositoryName,
  splitReviewFileKey,
  type ReviewFile,
} from "./types";

const SEP = "\u0000";

const APP_PATH = "src/app.tsx";

function file(overrides: Partial<ReviewFile>): ReviewFile {
  return {
    path: APP_PATH,
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
      file({ path: APP_PATH, repository_name: "frontend", repository_id: "f" }),
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

  it("places nested submodule scopes beneath their workspace directory hierarchy", () => {
    const tree = buildFileTree([
      file({ path: "README.md" }),
      file({ path: "src/main.ts", repository_name: "vendor/outer" }),
      file({ path: "README.md", repository_name: "vendor/outer/vendor/inner" }),
    ]);

    const vendor = tree.find((node) => node.name === "vendor");
    const outer = vendor?.children?.find((node) => node.name === "outer");
    const nestedVendor = outer?.children?.find((node) => node.name === "vendor");
    const inner = nestedVendor?.children?.find((node) => node.name === "inner");

    expect(vendor).toBeDefined();
    expect(outer?.isSubmodule).toBe(true);
    expect(outer?.repositoryName).toBe("vendor/outer");
    expect(inner?.isSubmodule).toBe(true);
    expect(inner?.repositoryName).toBe("vendor/outer/vendor/inner");
    expect(inner?.children?.[0].file?.repository_name).toBe("vendor/outer/vendor/inner");
  });

  it("merges repeated directory segments within a repo despite interleaved input", () => {
    const tree = buildFileTree([
      file({ path: "src/a.ts", repository_name: "frontend", repository_id: "f" }),
      file({ path: "README.md", repository_name: "frontend", repository_id: "f" }),
      file({ path: "src/b.ts", repository_name: "frontend", repository_id: "f" }),
      file({ path: "main.go", repository_name: "backend", repository_id: "b" }),
    ]);

    const frontend = tree.find((node) => node.name === "frontend");
    const srcNodes = frontend?.children?.filter((node) => node.name === "src") ?? [];
    expect(srcNodes).toHaveLength(1);
    expect(srcNodes[0].children?.map((node) => node.file?.path)).toEqual(["src/a.ts", "src/b.ts"]);
  });

  it("keeps identical relative directories distinct across repositories", () => {
    const tree = buildFileTree([
      file({ path: "src/frontend.ts", repository_name: "frontend", repository_id: "f" }),
      file({ path: "src/backend.ts", repository_name: "backend", repository_id: "b" }),
    ]);

    const srcNodes = tree.map((root) => root.children?.find((node) => node.name === "src"));
    expect(srcNodes).toHaveLength(2);
    expect(srcNodes.map((node) => node?.children?.[0].file?.repository_name)).toEqual([
      "frontend",
      "backend",
    ]);
  });
});

// reviewFileKey + splitReviewFileKey are the dedup primitive for the
// multi-repo review state. They have to round-trip exactly, including
// path values that contain the path-separator characters most likely to
// appear in real user input. The NUL byte (FILE_KEY_SEP) is the one
// character that can never legitimately appear in a path or repo name on
// any supported filesystem, which is why it's the separator.
describe("reviewFileKey", () => {
  it("returns the bare path when no repository_name is set", () => {
    expect(reviewFileKey({ path: "README.md" })).toBe("README.md");
  });

  it("keeps an explicit root scope distinct from a legacy bare path", () => {
    expect(reviewFileKey({ path: "src/index.ts", repository_name: "" })).toBe(`${SEP}src/index.ts`);
  });

  it("joins repository_name and path with the NUL separator", () => {
    expect(reviewFileKey({ path: "README.md", repository_name: "frontend" })).toBe(
      `frontend${SEP}README.md`,
    );
  });

  it("preserves slashes, dots, and spaces in the path", () => {
    expect(reviewFileKey({ path: "src/sub dir/file.ts", repository_name: "repo" })).toBe(
      `repo${SEP}src/sub dir/file.ts`,
    );
  });

  it("disambiguates same-named files in different repos", () => {
    const a = reviewFileKey({ path: "README.md", repository_name: "frontend" });
    const b = reviewFileKey({ path: "README.md", repository_name: "backend" });
    expect(a).not.toBe(b);
  });

  it("keeps mixed layers distinct without colliding with repository keys", () => {
    const staged = reviewFileKey({ path: "mixed.ts", change_layer: "staged" });
    const repositoryFile = reviewFileKey({ path: "staged", repository_name: "mixed.ts" });

    expect(staged).toBe(`mixed.ts${SEP}${SEP}staged`);
    expect(staged).not.toBe(repositoryFile);
  });
});

describe("resolvePRReviewRepositoryName", () => {
  it("uses the sanitized workspace repository name instead of the provider repo name", () => {
    expect(
      resolvePRReviewRepositoryName({ repository_id: "repo-1", repo: "widgets" }, "acme/widgets"),
    ).toBe("acme-widgets");
  });

  it("falls back to the provider repo name for legacy or unhydrated workspace data", () => {
    expect(resolvePRReviewRepositoryName({ repo: "widgets" }, "acme/widgets")).toBe("widgets");
    expect(resolvePRReviewRepositoryName({ repository_id: "repo-1", repo: "widgets" })).toBe(
      "widgets",
    );
  });
});

describe("sanitizeReviewRepositoryName", () => {
  it.each([
    ["acme/widgets", "acme-widgets"],
    ["owner\\repo", "owner-repo"],
    ["weird:name space", "weird-name-space"],
    ["with..dots", "with..dots"],
    ["--a///b..", "a-b"],
    ["修复/登录", ""],
  ])("mirrors the backend repository directory sanitizer for %s", (input, expected) => {
    expect(sanitizeReviewRepositoryName(input)).toBe(expected);
  });
});

describe("isReviewMultiRepo", () => {
  it("detects multiple named git-status repositories before task links hydrate", () => {
    expect(isReviewMultiRepo(0, ["frontend", "backend"])).toBe(true);
  });

  it("uses task links during partial status hydration", () => {
    expect(isReviewMultiRepo(2, ["frontend"])).toBe(true);
  });

  it("keeps one named or legacy repository in single-repo mode", () => {
    expect(isReviewMultiRepo(1, ["frontend"])).toBe(false);
    expect(isReviewMultiRepo(1, [""])).toBe(false);
  });
});

describe("getCumulativeReviewRepositoryNames", () => {
  it("returns distinct stamped repository names", () => {
    expect(
      getCumulativeReviewRepositoryNames({
        a: { repository_name: "frontend" },
        b: { repository_name: "backend" },
        c: { repository_name: "frontend" },
        d: {},
      }),
    ).toEqual(["frontend", "backend"]);
  });
});

describe("splitReviewFileKey", () => {
  it("returns empty repositoryName for legacy bare-path keys", () => {
    expect(splitReviewFileKey("README.md")).toEqual({ repositoryName: "", path: "README.md" });
  });

  it("splits a composite key on the NUL separator", () => {
    expect(splitReviewFileKey(`frontend${SEP}${APP_PATH}`)).toEqual({
      repositoryName: "frontend",
      path: APP_PATH,
    });
  });

  it("splits an explicit empty repository_name root key", () => {
    expect(splitReviewFileKey(`${SEP}${APP_PATH}`)).toEqual({
      repositoryName: "",
      path: APP_PATH,
    });
  });

  it("round-trips a layer-qualified repository key", () => {
    const key = reviewFileKey({
      path: APP_PATH,
      repository_name: "frontend",
      change_layer: "unstaged",
    });

    expect(splitReviewFileKey(key)).toEqual({
      repositoryName: "frontend",
      path: APP_PATH,
      changeLayer: "unstaged",
    });
  });

  it("keeps a legacy bare-path key without a repository scope", () => {
    expect(splitReviewFileKey(APP_PATH)).toEqual({ repositoryName: "", path: APP_PATH });
  });

  it("round-trips through reviewFileKey", () => {
    const cases = [
      { path: "README.md" },
      { path: "root.md", repository_name: "" },
      { path: "src/index.ts", repository_name: "frontend" },
      { path: "deep/nested/path/file.go", repository_name: "backend" },
      { path: "with spaces/file.md", repository_name: "shared" },
      // Repo names can themselves contain dashes and slashes.
      { path: "x.ts", repository_name: "owner/repo-name" },
    ];
    for (const original of cases) {
      const key = reviewFileKey(original);
      const round = splitReviewFileKey(key);
      expect(round.path).toBe(original.path);
      expect(round.repositoryName).toBe(original.repository_name ?? "");
    }
  });
});
