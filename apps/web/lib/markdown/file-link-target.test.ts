import { describe, expect, it } from "vitest";
import {
  buildMarkdownFileRootAliases,
  isBlockedMarkdownImageSource,
  resolveMarkdownFileTarget,
  type MarkdownFileRootAlias,
} from "./file-link-target";

const workspaceRoot = "/root/.kandev/tasks/example";
const PLUGIN_BUNDLE_PATH = "plugin/ui/bundle.js";
const GUIDE_PATH = "docs/guide.md";
const primaryWorktreeRoot = `${workspaceRoot}/kandev`;
const siblingWorktreeRoot = `${workspaceRoot}/plugin`;
const primaryRepositoryId = "repo-primary";
const siblingRepositoryId = "repo-sibling";
const primarySourceRoot = "/home/jcfs/projects/kandev";
const siblingSourceRoot = "/home/jcfs/projects/plugin";
const singleRepositoryId = "repo-1";
const singleSourceRoot = "/home/jcfs/projects/kandev";
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
          { repositoryId: primaryRepositoryId, path: primaryWorktreeRoot },
          { repositoryId: siblingRepositoryId, path: siblingWorktreeRoot },
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

  it("does not authorize session worktrees absent from the task repository set", () => {
    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        taskRepositoryIds: [primaryRepositoryId],
        repositories: [
          { id: primaryRepositoryId, localPath: primarySourceRoot },
          { id: siblingRepositoryId, localPath: siblingSourceRoot },
        ],
        sessionWorktrees: [
          { repositoryId: primaryRepositoryId, path: primaryWorktreeRoot },
          { repositoryId: siblingRepositoryId, path: siblingWorktreeRoot },
        ],
      }),
    ).toEqual([
      {
        repositoryId: primaryRepositoryId,
        sourceRoot: primarySourceRoot,
        workspaceRelativeRoot: "kandev",
      },
    ]);
  });

  it("does not infer repository aliases from session worktrees alone", () => {
    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        repositories: [{ id: siblingRepositoryId, localPath: siblingSourceRoot }],
        sessionWorktrees: [{ repositoryId: siblingRepositoryId, path: siblingWorktreeRoot }],
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
        sessionWorktrees: [{ repositoryId: "other-repo", path: primaryWorktreeRoot }],
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
        sessionWorktrees: [{ repositoryId: singleRepositoryId, path: primaryWorktreeRoot }],
      }),
    ).toEqual([]);

    expect(
      buildMarkdownFileRootAliases({
        workspaceRoot,
        taskRepositoryIds: [singleRepositoryId],
        repositories: [{ id: singleRepositoryId, localPath: singleSourceRoot }],
        sessionWorktrees: [
          { repositoryId: singleRepositoryId, path: primaryWorktreeRoot },
          { repositoryId: singleRepositoryId, path: `${workspaceRoot}/other` },
        ],
      }),
    ).toEqual([]);
  });
});

describe("resolveMarkdownFileTarget mapped paths", () => {
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
    ).toEqual({ kind: "file", path: PLUGIN_BUNDLE_PATH });
  });

  it("maps Windows workspace and registered source paths case-insensitively", () => {
    const windowsWorkspaceRoot = String.raw`C:\Users\Me\worktree`;
    const windowsAliases: MarkdownFileRootAlias[] = [
      {
        repositoryId: siblingRepositoryId,
        sourceRoot: String.raw`C:\Users\Me\source`,
        workspaceRelativeRoot: "plugin",
      },
    ];

    expect(
      resolveMarkdownFileTarget(String.raw`c:\users\me\worktree\src\file.ts:4`, {
        workspaceRoot: windowsWorkspaceRoot,
      }),
    ).toEqual({ kind: "file", path: "src/file.ts" });
    expect(
      resolveMarkdownFileTarget("/c:/users/me/source/ui/bundle.js:61", {
        workspaceRoot: windowsWorkspaceRoot,
        fileRootAliases: windowsAliases,
      }),
    ).toEqual({ kind: "file", path: PLUGIN_BUNDLE_PATH });
    expect(
      resolveMarkdownFileTarget(String.raw`C:\Windows\System32\kernel32.dll`, {
        workspaceRoot: windowsWorkspaceRoot,
        fileRootAliases: windowsAliases,
      }),
    ).toEqual({ kind: "file", path: "C:/Windows/System32/kernel32.dll" });
    expect(resolveMarkdownFileTarget("c://example.com/file.ts")).toBeNull();
  });

  it("blocks contained paths that cannot be opened as files", () => {
    expect(
      resolveMarkdownFileTarget(`${workspaceRoot}/LICENSE`, {
        workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "blocked" });
    expect(
      resolveMarkdownFileTarget(`${siblingSourceRoot}/Makefile`, {
        workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "blocked" });
  });
});

describe("resolveMarkdownFileTarget native workspace base", () => {
  it("keeps an unqualified link in the checkout that emitted it", () => {
    expect(
      resolveMarkdownFileTarget("implementation_plans/node26/task-00.md", {
        workspaceRoot,
        relativeBasePath: primaryWorktreeRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "file", path: "kandev/implementation_plans/node26/task-00.md" });
  });

  it("leaves links from a parent-root session unchanged", () => {
    expect(
      resolveMarkdownFileTarget(GUIDE_PATH, {
        workspaceRoot,
        relativeBasePath: workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "file", path: GUIDE_PATH });
  });

  it("does not double-prefix an explicit source qualifier", () => {
    expect(
      resolveMarkdownFileTarget(PLUGIN_BUNDLE_PATH, {
        workspaceRoot,
        relativeBasePath: primaryWorktreeRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "file", path: PLUGIN_BUNDLE_PATH });
  });
});

describe("resolveMarkdownFileTarget native workspace boundaries", () => {
  it("does not search a sibling repository for a duplicate filename", () => {
    expect(
      resolveMarkdownFileTarget("README.md", {
        workspaceRoot,
        relativeBasePath: siblingWorktreeRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "file", path: "plugin/README.md" });
  });

  it("keeps containment under the confirmed base", () => {
    expect(
      resolveMarkdownFileTarget("../../etc/passwd.md", {
        workspaceRoot,
        relativeBasePath: primaryWorktreeRoot,
        fileRootAliases: aliases,
      }),
    ).toBeNull();
    expect(
      resolveMarkdownFileTarget("secret/key.md", {
        workspaceRoot,
        relativeBasePath: "/home/other-checkout",
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "blocked" });
  });

  it("preserves an explicit prefix from worktree placement without a source alias", () => {
    expect(
      resolveMarkdownFileTarget("secondary/README.md", {
        workspaceRoot,
        relativeBasePath: primaryWorktreeRoot,
        sourcePrefixes: ["kandev", "secondary"],
      }),
    ).toEqual({ kind: "file", path: "secondary/README.md" });
    expect(
      resolveMarkdownFileTarget("kandev/README.md", {
        workspaceRoot,
        relativeBasePath: primaryWorktreeRoot,
        sourcePrefixes: ["kandev", "secondary"],
      }),
    ).toEqual({ kind: "file", path: "kandev/README.md" });
  });

  it("fails closed for an ambiguous expanded workspace without a base", () => {
    expect(
      resolveMarkdownFileTarget(GUIDE_PATH, {
        workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "blocked" });
    expect(
      resolveMarkdownFileTarget(GUIDE_PATH, {
        workspaceRoot,
        sourcePrefixes: ["kandev", "secondary"],
      }),
    ).toEqual({ kind: "blocked" });
  });

  it("retains root-relative behavior when the only checkout is the workspace root", () => {
    expect(
      resolveMarkdownFileTarget(GUIDE_PATH, {
        workspaceRoot,
        fileRootAliases: [
          {
            repositoryId: singleRepositoryId,
            sourceRoot: singleSourceRoot,
            workspaceRelativeRoot: "",
          },
        ],
      }),
    ).toEqual({ kind: "file", path: GUIDE_PATH });
  });

  it("matches a case-variant Windows source prefix", () => {
    const windowsWorkspaceRoot = String.raw`C:\Users\Me\worktree`;
    const windowsBase = String.raw`C:\Users\Me\worktree\master`;
    expect(
      resolveMarkdownFileTarget("MASTER/README.md", {
        workspaceRoot: windowsWorkspaceRoot,
        relativeBasePath: windowsBase,
        sourcePrefixes: ["master"],
      }),
    ).toEqual({ kind: "file", path: "MASTER/README.md" });
  });

  it("maps a Windows native workspace case-insensitively", () => {
    const windowsWorkspaceRoot = String.raw`C:\Users\Me\worktree`;
    expect(
      resolveMarkdownFileTarget(GUIDE_PATH, {
        workspaceRoot: windowsWorkspaceRoot,
        relativeBasePath: String.raw`c:\users\me\worktree\master`,
        fileRootAliases: [
          {
            repositoryId: primaryRepositoryId,
            sourceRoot: primarySourceRoot,
            workspaceRelativeRoot: "master",
          },
        ],
      }),
    ).toEqual({ kind: "file", path: "master/docs/guide.md" });
  });
});

describe("resolveMarkdownFileTarget safety boundaries", () => {
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

  it("preserves absolute read paths and leaves web links alone", () => {
    expect(
      resolveMarkdownFileTarget("/home/other-project/secret.md", {
        workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "file", path: "/home/other-project/secret.md" });
    expect(
      resolveMarkdownFileTarget("/home/other-project/%E0%A4%A", {
        workspaceRoot,
        fileRootAliases: aliases,
      }),
    ).toEqual({ kind: "blocked" });
    expect(resolveMarkdownFileTarget("https://example.com/docs.md", { workspaceRoot })).toBeNull();
    expect(resolveMarkdownFileTarget("/office/tasks/KAN-42", { workspaceRoot })).toBeNull();
  });

  it("keeps less common Unix host roots absolute and supports extensionless Windows files", () => {
    expect(resolveMarkdownFileTarget("/srv/reports/report.md", { workspaceRoot })).toEqual({
      kind: "file",
      path: "/srv/reports/report.md",
    });
    expect(
      resolveMarkdownFileTarget(String.raw`C:\Windows\System32\drivers\etc\hosts`, {
        workspaceRoot: String.raw`C:\workspace`,
      }),
    ).toEqual({ kind: "file", path: "C:/Windows/System32/drivers/etc/hosts" });
  });

  it("blocks rejected relative image paths but keeps external image sources", () => {
    expect(isBlockedMarkdownImageSource("../chart.png")).toBe(true);
    expect(isBlockedMarkdownImageSource("%2e%2e/chart.png")).toBe(true);
    expect(isBlockedMarkdownImageSource("https://example.com/chart.png")).toBe(false);
    expect(isBlockedMarkdownImageSource("/assets/chart.png")).toBe(false);
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

describe("ADR 0016 Markdown absolute reads", () => {
  it.each([
    "/tmp/pdf-timeout-verify/minimax-ontest-progressive.md",
    "/private/tmp/chart%20one.png",
    "/Users/person/report.md",
    "/workspace/sibling/report.md",
  ])("keeps the executor path %s", (href) => {
    expect(resolveMarkdownFileTarget(href, { workspaceRoot: "/workspace/current" })).toEqual({
      kind: "file",
      path: decodeURIComponent(href),
    });
  });
});
