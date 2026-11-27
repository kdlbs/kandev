import { describe, expect, it } from "vitest";
import type { ChangedFile } from "./changes-panel-helpers";
import {
  changedFileTargetsFromSelectionKeys,
  changesFileSelectionKey,
  groupChangedFileTargetsByRepository,
} from "./changes-timeline-selection";

const FRONTEND_REPOSITORY = "frontend";
const BACKEND_REPOSITORY = "backend";
const DUPLICATE_PATH = "src/file-0.ts";

function file(
  path: string,
  repositoryName?: string,
  changeLayer?: ChangedFile["changeLayer"],
): ChangedFile {
  return {
    path,
    repositoryName,
    changeLayer,
    status: "modified",
    staged: false,
    plus: 1,
    minus: 0,
    oldPath: undefined,
  };
}

describe("Changes file selection identity", () => {
  it("keeps equal paths and delimiter characters distinct across repositories and sections", () => {
    const samePathInFirstRepo = changesFileSelectionKey("unstaged", file("a::b", "repo::one"));
    const samePathInSecondRepo = changesFileSelectionKey("unstaged", file("a::b", "repo::two"));
    const samePathInOtherSection = changesFileSelectionKey("staged", file("a::b", "repo::one"));

    expect(new Set([samePathInFirstRepo, samePathInSecondRepo, samePathInOtherSection]).size).toBe(
      3,
    );
  });

  it("decodes selected identities into repository-specific operation targets", () => {
    const keys = [
      changesFileSelectionKey("unstaged", file("same.ts", FRONTEND_REPOSITORY)),
      changesFileSelectionKey("unstaged", file("same.ts", BACKEND_REPOSITORY)),
      "not-a-selection-key",
    ];

    expect(changedFileTargetsFromSelectionKeys(keys)).toEqual([
      { path: "same.ts", repositoryName: FRONTEND_REPOSITORY },
      { path: "same.ts", repositoryName: BACKEND_REPOSITORY },
    ]);
    expect(groupChangedFileTargetsByRepository(changedFileTargetsFromSelectionKeys(keys))).toEqual([
      { repositoryName: FRONTEND_REPOSITORY, paths: ["same.ts"] },
      { repositoryName: BACKEND_REPOSITORY, paths: ["same.ts"] },
    ]);
  });

  it("groups large selections in input order and removes duplicate paths per repository", () => {
    const uniqueTargets = Array.from({ length: 50_000 }, (_, index) => ({
      path: `src/file-${index}.ts`,
      repositoryName: BACKEND_REPOSITORY,
    }));
    const groups = groupChangedFileTargetsByRepository([
      ...uniqueTargets,
      { path: DUPLICATE_PATH, repositoryName: BACKEND_REPOSITORY },
      { path: DUPLICATE_PATH, repositoryName: FRONTEND_REPOSITORY },
      { path: DUPLICATE_PATH, repositoryName: FRONTEND_REPOSITORY },
    ]);

    expect(groups).toHaveLength(2);
    expect(groups[0].repositoryName).toBe(BACKEND_REPOSITORY);
    expect(groups[0].paths).toHaveLength(50_000);
    expect(groups[0].paths[0]).toBe("src/file-0.ts");
    expect(groups[0].paths.at(-1)).toBe("src/file-49999.ts");
    expect(groups[1]).toEqual({
      repositoryName: FRONTEND_REPOSITORY,
      paths: [DUPLICATE_PATH],
    });
  });
});
