import { describe, expect, it } from "vitest";
import type { Repository, TaskSessionWorktree } from "@/lib/types/http";
import {
  buildFileBrowserRepositoryLabels,
  labelFileBrowserPath,
  workspaceInventoryRevision,
} from "./file-browser-repository-labels";

const WORKSPACE_PATH = "/tmp/task-root";

const worktree = (
  id: string,
  repositoryId: string,
  path: string,
  position: number,
): TaskSessionWorktree =>
  ({
    id: `association-${id}`,
    session_id: "session",
    worktree_id: id,
    repository_id: repositoryId,
    worktree_path: path,
    position,
  }) as TaskSessionWorktree;

const repository = (id: string, name: string, owner: string): Repository =>
  ({ id, name, provider_owner: owner, provider_name: name }) as Repository;

describe("file browser repository labels", () => {
  it("labels only the exact active checkout roots and keeps canonical search paths", () => {
    const labels = buildFileBrowserRepositoryLabels(
      WORKSPACE_PATH,
      [worktree("wt-1", "repo-1", `${WORKSPACE_PATH}/raw-relocated-name`, 0)],
      [repository("repo-1", "kdlbs-kandev", "kdlbs")],
    );

    expect(labels).toEqual({ "raw-relocated-name": "kdlbs-kandev" });
    expect(labelFileBrowserPath("raw-relocated-name/README.md", labels)).toBe(
      "kdlbs-kandev/README.md",
    );
    expect(labelFileBrowserPath("raw-relocated-name-lookalike/README.md", labels)).toBe(
      "raw-relocated-name-lookalike/README.md",
    );
    expect(labelFileBrowserPath("notes.txt", labels)).toBe("notes.txt");
  });

  it("disambiguates duplicate repository names with their existing owner identity", () => {
    expect(
      buildFileBrowserRepositoryLabels(
        WORKSPACE_PATH,
        [
          worktree("wt-1", "repo-1", `${WORKSPACE_PATH}/one`, 0),
          worktree("wt-2", "repo-2", `${WORKSPACE_PATH}/two`, 1),
        ],
        [repository("repo-1", "api", "kdlbs"), repository("repo-2", "api", "acme")],
      ),
    ).toEqual({ one: "api (kdlbs · repo-1)", two: "api (acme · repo-2)" });
  });

  it("disambiguates repositories with the same name and owner", () => {
    expect(
      buildFileBrowserRepositoryLabels(
        WORKSPACE_PATH,
        [
          worktree("wt-1", "repo-1", `${WORKSPACE_PATH}/one`, 0),
          worktree("wt-2", "repo-2", `${WORKSPACE_PATH}/two`, 1),
        ],
        [repository("repo-1", "api", "kdlbs"), repository("repo-2", "api", "kdlbs")],
      ),
    ).toEqual({ one: "api (kdlbs · repo-1)", two: "api (kdlbs · repo-2)" });
  });

  it("changes inventory revision when a checkout is replaced without changing count", () => {
    const previous = [worktree("old-id", "repo-1", `${WORKSPACE_PATH}/old`, 0)];
    const replacement = [worktree("new-id", "repo-1", `${WORKSPACE_PATH}/new`, 0)];
    expect(workspaceInventoryRevision(previous)).not.toBe(workspaceInventoryRevision(replacement));
  });

  it("does not label a path outside the task root or without verified repository metadata", () => {
    expect(
      buildFileBrowserRepositoryLabels(
        WORKSPACE_PATH,
        [
          worktree("wt-1", "repo-1", "/tmp/other/root", 0),
          worktree("wt-2", "repo-2", `${WORKSPACE_PATH}/lookalike`, 1),
        ],
        [repository("repo-1", "external", "owner")],
      ),
    ).toEqual({});
  });
});
