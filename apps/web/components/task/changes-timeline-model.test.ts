import { describe, expect, it } from "vitest";
import type { ChangedFile } from "./changes-panel-helpers";
import {
  buildChangesHistoryTimelineRows,
  buildChangesTimelineRows,
  changesCommitExpansionKey,
  changesDirectoryExpansionKey,
  changesRepositoryExpansionKey,
} from "./changes-timeline-model";
import type { CommitItem } from "./commit-row";
import type { CommitInlineDetailSnapshot } from "./changes-inline-commit-state";

const MODIFIED_STATUS = "modified" as const;
const BACKEND_REPOSITORY = "backend";
const COMMITS_SECTION_TEST_ID = "commits-section";
const HISTORY_SECTION_ROW_KIND = "history-section";
const HISTORY_REPOSITORY_ROW_KIND = "history-repository";

function file(path: string, repositoryName?: string): ChangedFile {
  return {
    path,
    repositoryName,
    status: MODIFIED_STATUS,
    staged: false,
    plus: 1,
    minus: 0,
    oldPath: undefined,
  };
}

describe("buildChangesTimelineRows", () => {
  it("keeps the complete ordered file collection in flat layout", () => {
    const files = Array.from({ length: 50_000 }, (_, index) =>
      file(`file-${String(index).padStart(5, "0")}.ts`),
    );
    const rows = buildChangesTimelineRows({
      layout: "flat",
      sections: [{ variant: "unstaged", files, collapsed: false, selectedKeys: [] }],
      collapsedRepositories: new Set(),
      collapsedDirectories: new Set(),
    });

    expect(rows).toHaveLength(50_001);
    expect(rows[1]).toMatchObject({ kind: "file", file: files[0] });
    expect(rows.at(-1)).toMatchObject({ kind: "file", file: files.at(-1) });
    expect(new Set(rows.map((row) => row.key)).size).toBe(rows.length);
  });

  it("keeps repository headers and actions scoped when equal paths appear in two repos", () => {
    const rows = buildChangesTimelineRows({
      layout: "flat",
      sections: [
        {
          variant: "unstaged",
          files: [file("same.ts", "frontend"), file("same.ts", "backend")],
          collapsed: false,
          selectedKeys: [],
        },
      ],
      collapsedRepositories: new Set(),
      collapsedDirectories: new Set(),
    });

    expect(
      rows.filter((row) => row.kind === "repository").map((row) => row.repositoryName),
    ).toEqual(["frontend", "backend"]);
    expect(rows.filter((row) => row.kind === "file").map((row) => row.file.repositoryName)).toEqual(
      ["frontend", "backend"],
    );
  });

  it("flattens expanded tree rows and omits children of collapsed directories", () => {
    const files = [file("src/a.ts"), file("src/b.ts")];
    const expandedRows = buildChangesTimelineRows({
      layout: "tree",
      sections: [{ variant: "unstaged", files, collapsed: false, selectedKeys: [] }],
      collapsedRepositories: new Set(),
      collapsedDirectories: new Set(),
    });
    const collapsedRows = buildChangesTimelineRows({
      layout: "tree",
      sections: [{ variant: "unstaged", files, collapsed: false, selectedKeys: [] }],
      collapsedRepositories: new Set(),
      collapsedDirectories: new Set([changesDirectoryExpansionKey("unstaged", "", "src")]),
    });

    expect(expandedRows.map((row) => row.kind)).toEqual(["section", "directory", "file", "file"]);
    expect(collapsedRows.map((row) => row.kind)).toEqual(["section", "directory"]);
  });

  it("omits files when a repository is collapsed", () => {
    const rows = buildChangesTimelineRows({
      layout: "flat",
      sections: [
        {
          variant: "unstaged",
          files: [file("a.ts", "frontend"), file("b.ts", "backend")],
          collapsed: false,
          selectedKeys: [],
        },
      ],
      collapsedRepositories: new Set([changesRepositoryExpansionKey("unstaged", "frontend")]),
      collapsedDirectories: new Set(),
    });

    expect(rows.filter((row) => row.kind === "file").map((row) => row.file.path)).toEqual(["b.ts"]);
  });
});

describe("buildChangesHistoryTimelineRows", () => {
  it("bounds expanded historical files as individual rows in a full target scope", () => {
    const files = Array.from({ length: 50_000 }, (_, index) => ({
      path: `src/file-${String(index).padStart(5, "0")}.ts`,
      status: MODIFIED_STATUS,
      plus: 2,
      minus: 1,
      repositoryName: BACKEND_REPOSITORY,
    }));
    const commit = historyCommit("same-sha", BACKEND_REPOSITORY);
    const rows = buildChangesHistoryTimelineRows(
      [
        {
          kind: "commits",
          sectionKey: "local",
          label: "Commits",
          testId: COMMITS_SECTION_TEST_ID,
          collapsed: false,
          commits: [{ commit, detail: { expanded: true, status: "loaded", files } }],
          collapsedRepositories: new Set(),
          collapsedDirectories: new Set(),
          layout: "flat",
        },
      ],
      new Set(),
    );

    const fileRows = rows.filter((row) => row.kind === "commit-file");
    expect(fileRows).toHaveLength(50_000);
    expect(fileRows[0]).toMatchObject({ kind: "commit-file", file: files[0] });
    expect(fileRows.at(-1)).toMatchObject({ kind: "commit-file", file: files.at(-1) });
    expect(new Set(rows.map((row) => row.key)).size).toBe(rows.length);
    expect(fileRows[0]?.groups).toHaveLength(4);
  });
});

describe("buildChangesHistoryTimelineRows: provider files", () => {
  it("preserves 50,000 provider files in one repository scope", () => {
    const files = Array.from({ length: 50_000 }, (_, index) => ({
      path: `src/provider-file-${String(index).padStart(5, "0")}.ts`,
      prKey: "github:workspace/repository#42",
      status: MODIFIED_STATUS,
      plus: 1,
      minus: 0,
      repository_name: BACKEND_REPOSITORY,
    }));
    const rows = buildChangesHistoryTimelineRows(
      [
        {
          kind: "pr",
          sectionKey: "current-pr",
          label: "Pull request files",
          testId: "pr-files-section",
          collapsed: false,
          files,
          collapsedRepositories: new Set(),
        },
      ],
      new Set(),
    );

    const fileRows = rows.filter((row) => row.kind === "pr-file");
    expect(rows[0]).toMatchObject({ kind: HISTORY_SECTION_ROW_KIND, count: 50_000 });
    expect(fileRows).toHaveLength(50_000);
    expect(fileRows[0]).toMatchObject({
      path: files[0]?.path,
      prKey: "github:workspace/repository#42",
    });
    expect(fileRows.at(-1)).toMatchObject({
      path: files.at(-1)?.path,
      repositoryName: BACKEND_REPOSITORY,
    });
    expect(fileRows[0]?.groups).toContainEqual({
      key: "history-pr-files-list:current-pr",
      testId: "pr-files-list",
      attributes: { role: "list" },
    });
    expect(new Set(rows.map((row) => row.key)).size).toBe(rows.length);
  });
});

describe("buildChangesHistoryTimelineRows: commit headers", () => {
  it("preserves 50,000 commit headers in their complete source scope", () => {
    const commits = Array.from({ length: 50_000 }, (_, index) => ({
      commit: historyCommit(`sha-${String(index).padStart(5, "0")}`, BACKEND_REPOSITORY),
      detail: emptyDetail(),
    }));
    const collapsedCommitKeys = new Set(
      commits.map(({ commit }) => changesCommitExpansionKey("local", commit.detailTarget)),
    );
    const rows = buildChangesHistoryTimelineRows(
      [
        {
          kind: "commits",
          sectionKey: "local",
          label: "Commits",
          testId: COMMITS_SECTION_TEST_ID,
          collapsed: false,
          commits,
          collapsedRepositories: new Set(),
          collapsedDirectories: new Set(),
          layout: "flat",
        },
      ],
      collapsedCommitKeys,
    );

    const commitRows = rows.filter((row) => row.kind === "commit");
    expect(rows[0]).toMatchObject({ kind: HISTORY_SECTION_ROW_KIND, count: 50_000 });
    expect(commitRows).toHaveLength(50_000);
    expect(commitRows[0]).toMatchObject({ target: commits[0]?.commit.detailTarget });
    expect(commitRows.at(-1)).toMatchObject({ target: commits.at(-1)?.commit.detailTarget });
    expect(new Set(rows.map((row) => row.key)).size).toBe(rows.length);
  });

  it("keeps an expanded loading commit without adding a virtual status row", () => {
    const commit = historyCommit("pending-commit", BACKEND_REPOSITORY);
    const rows = buildChangesHistoryTimelineRows(
      [
        {
          kind: "commits",
          sectionKey: "local",
          label: "Commits",
          testId: COMMITS_SECTION_TEST_ID,
          collapsed: false,
          commits: [{ commit, detail: { expanded: true, status: "loading", files: [] } }],
          collapsedRepositories: new Set(),
          collapsedDirectories: new Set(),
          layout: "flat",
        },
      ],
      new Set(),
    );

    expect(rows.map((row) => row.kind)).toEqual([
      HISTORY_SECTION_ROW_KIND,
      HISTORY_REPOSITORY_ROW_KIND,
      "commit",
    ]);
  });
});

describe("buildChangesHistoryTimelineRows: collapsed scopes", () => {
  it("omits descendants for collapsed sections, repositories, and commits", () => {
    const commit = historyCommit("same-sha", BACKEND_REPOSITORY);
    const detail: CommitInlineDetailSnapshot = {
      expanded: true,
      status: "loaded",
      files: [{ path: "src/a.ts", status: MODIFIED_STATUS, plus: 1, minus: 0 }],
    };
    const commitSection = {
      kind: "commits" as const,
      sectionKey: "local",
      label: "Commits",
      testId: COMMITS_SECTION_TEST_ID,
      collapsed: false,
      commits: [{ commit, detail }],
      collapsedRepositories: new Set<string>(),
      collapsedDirectories: new Set<string>(),
      layout: "flat" as const,
    };
    const collapsedCommit = buildChangesHistoryTimelineRows(
      [commitSection],
      new Set([changesCommitExpansionKey("local", commit.detailTarget)]),
    );
    const collapsedRepository = buildChangesHistoryTimelineRows(
      [
        {
          ...commitSection,
          collapsedRepositories: new Set([
            JSON.stringify(["changes", HISTORY_REPOSITORY_ROW_KIND, "local", BACKEND_REPOSITORY]),
          ]),
        },
      ],
      new Set(),
    );
    const collapsedSection = buildChangesHistoryTimelineRows(
      [{ ...commitSection, collapsed: true }],
      new Set(),
    );

    expect(collapsedCommit.map((row) => row.kind)).toEqual([
      HISTORY_SECTION_ROW_KIND,
      HISTORY_REPOSITORY_ROW_KIND,
      "commit",
    ]);
    expect(collapsedRepository.map((row) => row.kind)).toEqual([
      HISTORY_SECTION_ROW_KIND,
      HISTORY_REPOSITORY_ROW_KIND,
    ]);
    expect(collapsedSection.map((row) => row.kind)).toEqual([HISTORY_SECTION_ROW_KIND]);
  });
});

describe("buildChangesHistoryTimelineRows: repository identity", () => {
  it("does not merge same-SHA commits from different repositories", () => {
    const rows = buildChangesHistoryTimelineRows(
      [
        {
          kind: "commits",
          sectionKey: "local",
          label: "Commits",
          testId: COMMITS_SECTION_TEST_ID,
          collapsed: false,
          commits: [
            { commit: historyCommit("same-sha", "frontend"), detail: emptyDetail() },
            { commit: historyCommit("same-sha", BACKEND_REPOSITORY), detail: emptyDetail() },
          ],
          collapsedRepositories: new Set(),
          collapsedDirectories: new Set(),
          layout: "flat",
        },
      ],
      new Set(),
    );

    const commitRows = rows.filter((row) => row.kind === "commit");
    expect(commitRows.map((row) => row.targetKey)).toHaveLength(2);
    expect(new Set(commitRows.map((row) => row.key)).size).toBe(2);
    expect(commitRows[0]?.groups.map((group) => group.testId)).toContain("commits-list");
  });
});

function historyCommit(sha: string, repositoryName: string): CommitItem {
  return {
    commit_sha: sha,
    commit_message: "Historical change",
    insertions: 1,
    deletions: 0,
    statsAvailable: true,
    detailTarget: { source: "local", sha, repo: repositoryName },
    repository_name: repositoryName,
  };
}

function emptyDetail(): CommitInlineDetailSnapshot {
  return { expanded: false, status: "idle", files: [] };
}
