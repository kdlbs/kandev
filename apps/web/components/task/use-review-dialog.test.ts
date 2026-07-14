import { describe, expect, it } from "vitest";
import { buildReviewGitStatusFiles } from "./use-review-dialog";
import type { FileInfo, GitStatusEntry } from "@/lib/state/slices/session-runtime/types";

type GitStatusByRepo = Array<{ repository_name: string; status: GitStatusEntry }>;

const README_PATH = "README.md";

function file(path = README_PATH): FileInfo {
  return { path, status: "modified", staged: false };
}

function status(files: Record<string, FileInfo>): GitStatusEntry {
  return { files } as GitStatusEntry;
}

describe("buildReviewGitStatusFiles", () => {
  it("keeps legacy bare files for a single-repository task", () => {
    const legacyFiles = { [README_PATH]: file() };
    const perRepoFiles = { [README_PATH]: file() };

    const result = buildReviewGitStatusFiles(
      status(legacyFiles),
      [{ repository_name: "frontend", status: status(perRepoFiles) }],
      1,
    );

    expect(result).toEqual({ files: legacyFiles, isMultiRepo: false });
    expect(result.files?.[README_PATH]?.repository_name).toBeUndefined();
  });

  it("uses one named status as a bare fallback while legacy status hydrates", () => {
    const perRepoFiles = { [README_PATH]: file() };

    const result = buildReviewGitStatusFiles(
      undefined,
      [{ repository_name: "frontend", status: status(perRepoFiles) }],
      1,
    );

    expect(result).toEqual({ files: perRepoFiles, isMultiRepo: false });
    expect(result.files?.[README_PATH]?.repository_name).toBeUndefined();
  });

  it("uses stable composite keys while multi-repo statuses hydrate", () => {
    const frontend = { [README_PATH]: file() };
    const backend = { [README_PATH]: file() };

    const partiallyHydrated = buildReviewGitStatusFiles(
      status(frontend),
      [{ repository_name: "frontend", status: status(frontend) }],
      2,
    );
    const fullyHydrated = buildReviewGitStatusFiles(
      status(backend),
      [
        { repository_name: "frontend", status: status(frontend) },
        { repository_name: "backend", status: status(backend) },
      ],
      2,
    );

    expect(partiallyHydrated.isMultiRepo).toBe(true);
    expect(Object.keys(partiallyHydrated.files ?? {})).toEqual(["frontend\u0000README.md"]);
    expect(Object.keys(fullyHydrated.files ?? {}).sort()).toEqual([
      "backend\u0000README.md",
      "frontend\u0000README.md",
    ]);
  });

  it("falls back to per-repo status count when task repositories are not hydrated", () => {
    const frontend = { "src/app.ts": file("src/app.ts") };
    const backend = { "cmd/main.go": file("cmd/main.go") };
    const statuses: GitStatusByRepo = [
      { repository_name: "frontend", status: status(frontend) },
      { repository_name: "backend", status: status(backend) },
    ];

    const result = buildReviewGitStatusFiles(status(frontend), statuses, 0);

    expect(result.isMultiRepo).toBe(true);
    expect(Object.keys(result.files ?? {}).sort()).toEqual([
      "backend\u0000cmd/main.go",
      "frontend\u0000src/app.ts",
    ]);
  });
});
