import { describe, it, expect } from "vitest";
import {
  aggregateFilesAcrossRepos,
  groupPathsByRepoName,
  repositoryScopesForMutation,
} from "./use-session-git";

describe("aggregateFilesAcrossRepos", () => {
  it("stamps path from the map key when a status entry lacks its own path", () => {
    // Archived-task DB snapshots can replay cumulative-diff entries whose
    // older shape only sat under the key (no `path` field). The changes tree
    // splits on file.path, so the fallback must not surface undefined.
    const out = aggregateFilesAcrossRepos([], {
      files: {
        "apps/a.go": { path: "apps/a.go", status: "modified", staged: false },
        "readme.md": { status: "modified", staged: false } as never,
      },
    } as never);
    expect(out.map((f) => f.path).sort()).toEqual(["apps/a.go", "readme.md"]);
  });

  it("stamps path per repo in multi-repo statuses lacking it", () => {
    const out = aggregateFilesAcrossRepos(
      [
        {
          repository_name: "frontend",
          status: {
            files: { "app.tsx": { status: "modified", staged: false } as never },
          },
        } as never,
      ],
      undefined,
    );
    expect(out).toEqual([
      { status: "modified", staged: false, path: "app.tsx", repository_name: "frontend" },
    ]);
  });

  it("preserves existing paths and falls back to empty list without status", () => {
    expect(aggregateFilesAcrossRepos([], undefined)).toEqual([]);
  });

  it("restores path and repository from composite keys", () => {
    // Multi-repo cumulative payloads key files as `<repo>\x00<path>`. If such
    // an entry ever lacks the stamped `path`, the backfill must split the key
    // (like splitReviewFileKey) instead of stamping the NUL composite into
    // FileInfo.path — the changes tree splits on '/', so a composite path
    // would surface as one bogus filename and break per-repo grouping.
    const out = aggregateFilesAcrossRepos([], {
      files: {
        "repoA\u0000lib/util.ts": { status: "added", staged: false } as never,
      },
    } as never);
    expect(out).toEqual([
      { status: "added", staged: false, path: "lib/util.ts", repository_name: "repoA" },
    ]);
  });

  it("keeps same relative paths distinct across repositories", () => {
    const out = aggregateFilesAcrossRepos([], {
      files: {
        "repoA\u0000README.md": { status: "modified", staged: false } as never,
        "repoB\u0000README.md": { status: "modified", staged: false } as never,
      },
    } as never);

    expect(out).toEqual([
      { status: "modified", staged: false, path: "README.md", repository_name: "repoA" },
      { status: "modified", staged: false, path: "README.md", repository_name: "repoB" },
    ]);
  });
});

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
