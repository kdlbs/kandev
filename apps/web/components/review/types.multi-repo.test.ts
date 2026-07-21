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
    expect(reviewFileKey({ path: "src/index.ts", repository_name: "" })).toBe("src/index.ts");
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

  it("treats a key with empty repository_name as a legacy bare-path key", () => {
    // reviewFileKey({path, repository_name: ""}) returns the bare path,
    // so split should mirror that — no NUL in input means repo is "".
    expect(splitReviewFileKey(APP_PATH)).toEqual({ repositoryName: "", path: APP_PATH });
  });

  it("round-trips through reviewFileKey", () => {
    const cases = [
      { path: "README.md" },
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
