import { describe, expect, it } from "vitest";
import {
  buildMarkdownFileRootAliases,
  resolveMarkdownFileTarget,
  type MarkdownFileRootAlias,
} from "./file-link-target";

const workspaceRoot = "/root/.kandev/tasks/example";
const primaryRepositoryId = "repo-primary";
const siblingRepositoryId = "repo-sibling";
const primarySourceRoot = "/home/jcfs/projects/kandev";
const siblingSourceRoot = "/home/jcfs/projects/plugin";
const singleRepositoryId = "repo-1";
const singleSourceRoot = "/home/jcfs/projects/kandev";

describe("buildMarkdownFileRootAliases", () => {
  it("maps task-linked repositories to their active worktree roots", () => {
    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        taskRepositoryIds: [primaryRepositoryId, siblingRepositoryId],
        repositories: [
          { id: primaryRepositoryId, localPath: primarySourceRoot },
          { id: siblingRepositoryId, localPath: siblingSourceRoot },
        ],
        sessionWorktrees: [
          { repositoryId: primaryRepositoryId, path: `${workspaceRoot}/kandev` },
          { repositoryId: siblingRepositoryId, path: `${workspaceRoot}/plugin` },
        ],
      }),
    ).toEqual([
      {
        repositoryId: primaryRepositoryId,
        sourceRoot: primarySourceRoot,
        workspaceRelativeRoot: "kandev",
      },
      {
        repositoryId: siblingRepositoryId,
        sourceRoot: siblingSourceRoot,
        workspaceRelativeRoot: "plugin",
      },
    ]);
  });

  it("supports a repository worktree that is the workspace root", () => {
    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        taskRepositoryIds: [singleRepositoryId],
        repositories: [{ id: singleRepositoryId, localPath: singleSourceRoot }],
        sessionWorktrees: [{ repositoryId: singleRepositoryId, path: workspaceRoot }],
      }),
    ).toEqual([
      {
        repositoryId: singleRepositoryId,
        sourceRoot: singleSourceRoot,
        workspaceRelativeRoot: "",
      },
    ]);
  });

  it("uses session worktree identities when the task projection is stale", () => {
    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        taskRepositoryIds: [primaryRepositoryId],
        repositories: [
          { id: primaryRepositoryId, localPath: primarySourceRoot },
          { id: siblingRepositoryId, localPath: siblingSourceRoot },
        ],
        sessionWorktrees: [
          { repositoryId: primaryRepositoryId, path: `${workspaceRoot}/kandev` },
          { repositoryId: siblingRepositoryId, path: `${workspaceRoot}/plugin` },
        ],
      }),
    ).toEqual([
      {
        repositoryId: primaryRepositoryId,
        sourceRoot: primarySourceRoot,
        workspaceRelativeRoot: "kandev",
      },
      {
        repositoryId: siblingRepositoryId,
        sourceRoot: siblingSourceRoot,
        workspaceRelativeRoot: "plugin",
      },
    ]);
  });

  it("does not infer repository aliases from session worktrees alone", () => {
    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        repositories: [{ id: siblingRepositoryId, localPath: siblingSourceRoot }],
        sessionWorktrees: [{ repositoryId: siblingRepositoryId, path: `${workspaceRoot}/plugin` }],
      }),
    ).toEqual([]);
  });
});

describe("buildMarkdownFileRootAliases fail-closed cases", () => {
  it("fails closed when repository identity or workspace containment is incomplete", () => {
    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        taskRepositoryIds: [singleRepositoryId],
        repositories: [{ id: singleRepositoryId, localPath: singleSourceRoot }],
        sessionWorktrees: [{ repositoryId: "other-repo", path: `${workspaceRoot}/kandev` }],
      }),
    ).toEqual([]);

    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        taskRepositoryIds: ["repo-1"],
        repositories: [{ id: "repo-1", localPath: "/home/jcfs/projects/kandev" }],
        sessionWorktrees: [{ repositoryId: singleRepositoryId, path: "/tmp/kandev" }],
      }),
    ).toEqual([]);
  });

  it("omits ambiguous source or worktree identities", () => {
    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        taskRepositoryIds: [singleRepositoryId],
        repositories: [
          { id: singleRepositoryId, localPath: singleSourceRoot },
          { id: singleRepositoryId, localPath: "/home/jcfs/other-kandev" },
        ],
        sessionWorktrees: [{ repositoryId: singleRepositoryId, path: `${workspaceRoot}/kandev` }],
      }),
    ).toEqual([]);

    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        taskRepositoryIds: [singleRepositoryId],
        repositories: [{ id: singleRepositoryId, localPath: singleSourceRoot }],
        sessionWorktrees: [
          { repositoryId: singleRepositoryId, path: `${workspaceRoot}/kandev` },
          { repositoryId: singleRepositoryId, path: `${workspaceRoot}/other` },
        ],
      }),
    ).toEqual([]);
  });
});

describe("resolveMarkdownFileTarget", () => {
  const aliases: MarkdownFileRootAlias[] = [
    {
      repositoryId: primaryRepositoryId,
      sourceRoot: primarySourceRoot,
      workspaceRelativeRoot: "kandev",
    },
    {
      repositoryId: siblingRepositoryId,
      sourceRoot: siblingSourceRoot,
      workspaceRelativeRoot: "plugin",
    },
  ];

  it("resolves active workspace paths before repository aliases", () => {
    expect(
      resolveMarkdownFileTarget(`${workspaceRoot}/kandev/docs/README.md:12:4`, {
        workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "file", path: "kandev/docs/README.md" });
  });

  it("maps registered source paths to the active worktree", () => {
    expect(
      resolveMarkdownFileTarget(`${siblingSourceRoot}/ui/bundle.js:61`, {
        workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "file", path: "plugin/ui/bundle.js" });
  });

  it("rejects traversal and ambiguous aliases", () => {
    expect(
      resolveMarkdownFileTarget(`${siblingSourceRoot}/../kandev/secret.md`, {
        workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "blocked" });

    expect(
      resolveMarkdownFileTarget(`${siblingSourceRoot}/ui/bundle.js`, {
        workspaceRoot,
        fileRootAliases: [
          ...aliases,
          {
            repositoryId: "repo-other",
            sourceRoot: siblingSourceRoot,
            workspaceRelativeRoot: "other",
          },
        ],
      }),
    ).toEqual({ kind: "blocked" });
  });

  it("classifies unmatched host paths as blocked and leaves web links alone", () => {
    expect(
      resolveMarkdownFileTarget("/home/other-project/secret.md", {
        workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "blocked" });
    expect(
      resolveMarkdownFileTarget("/home/other-project/%E0%A4%A", {
        workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "blocked" });
    expect(resolveMarkdownFileTarget("https://example.com/docs.md", { workspaceRoot })).toBeNull();
    expect(resolveMarkdownFileTarget("/office/tasks/KAN-42", { workspaceRoot })).toBeNull();
  });

  it("preserves relative and workspace-root file links", () => {
    expect(resolveMarkdownFileTarget("docs/README.md", { workspaceRoot })).toEqual({
      kind: "file",
      path: "docs/README.md",
    });
    expect(resolveMarkdownFileTarget("/docs/README.md", { workspaceRoot })).toEqual({
      kind: "file",
      path: "docs/README.md",
    });
  });
});
