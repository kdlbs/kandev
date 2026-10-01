import { groupByRepositoryName } from "@/lib/group-by-repo";
import type { VisibleRow } from "@/hooks/use-tree";
import type { ChangedFile } from "./changes-panel-helpers";
import {
  buildChangesTree,
  flattenChangesTree,
  type ChangesTreeNode,
} from "./changes-file-tree-model";
import type { CommitDetailTarget } from "@/lib/state/diff-target-types";
import type { CommitItem } from "./commit-row";
import {
  commitDetailTargetKey,
  type CommitInlineDetailSnapshot,
  type CommitInlineFile,
} from "./changes-inline-commit-state";
import type { FileInfo } from "@/lib/state/store";

const HISTORY_REPOSITORY_ROW_KIND = "history-repository" as const;

export type ChangesTimelineGroupDescriptor = {
  key: string;
  testId?: string;
  attributes?: Record<string, string>;
};

type ChangesHistoryRowBase = {
  key: string;
  sectionKey: string;
  groups: ChangesTimelineGroupDescriptor[];
};

export type ChangesHistoryTimelineRow =
  | (ChangesHistoryRowBase & {
      kind: "history-section";
      historyKind: "pr" | "commits";
      label: string;
      testId: string;
      count: number;
      collapsed: boolean;
      showActions: boolean;
    })
  | (ChangesHistoryRowBase & {
      kind: typeof HISTORY_REPOSITORY_ROW_KIND;
      historyKind: "pr" | "commits";
      repositoryName: string;
      count: number;
      collapsed: boolean;
      showActions: boolean;
    })
  | (ChangesHistoryRowBase & {
      kind: "pr-file";
      path: string;
      prKey?: string;
      status: FileInfo["status"];
      plus?: number;
      minus?: number;
      oldPath?: string;
      repositoryName: string;
    })
  | (ChangesHistoryRowBase & {
      kind: "commit";
      commit: CommitItem;
      target: CommitDetailTarget;
      targetKey: string;
      expanded: boolean;
      isLatest: boolean;
      detail: CommitInlineDetailSnapshot;
    })
  | (ChangesHistoryRowBase & {
      kind: "commit-directory";
      target: CommitDetailTarget;
      row: VisibleRow<ChangesTreeNode>;
    })
  | (ChangesHistoryRowBase & {
      kind: "commit-file";
      target: CommitDetailTarget;
      file: CommitInlineFile;
      treeMode: boolean;
      indentPx: number;
    })
  | (ChangesHistoryRowBase & {
      kind: "commit-status";
      target: CommitDetailTarget;
      status: "loading" | "error" | "empty";
      error?: string;
    });

export type ChangesHistoryPRFileInput = {
  path: string;
  prKey?: string;
  status: FileInfo["status"];
  plus?: number;
  minus?: number;
  oldPath?: string;
  repository_name?: string;
};

export type ChangesHistoryCommitInput = {
  commit: CommitItem;
  detail: CommitInlineDetailSnapshot;
};

type ChangesLayout = "flat" | "tree";

export type ChangesHistorySectionInput =
  | {
      kind: "pr";
      sectionKey: string;
      label: string;
      testId: string;
      collapsed: boolean;
      files: ChangesHistoryPRFileInput[];
      collapsedRepositories: ReadonlySet<string>;
    }
  | {
      kind: "commits";
      sectionKey: string;
      label: string;
      testId: string;
      collapsed: boolean;
      commits: ChangesHistoryCommitInput[];
      collapsedRepositories: ReadonlySet<string>;
      collapsedDirectories: ReadonlySet<string>;
      layout: ChangesLayout;
      showActions?: boolean;
    };

const CHANGES_KEY_PREFIX = "changes";
const COMMIT_FILE_ROW_KIND = "commit-file" as const;

function rowKey(...parts: unknown[]): string {
  return JSON.stringify([CHANGES_KEY_PREFIX, ...parts]);
}

export function changesHistoryRepositoryExpansionKey(
  sectionKey: string,
  repositoryName: string,
): string {
  return JSON.stringify([
    CHANGES_KEY_PREFIX,
    HISTORY_REPOSITORY_ROW_KIND,
    sectionKey,
    repositoryName,
  ]);
}

export function changesInlineDirectoryExpansionKey(
  sectionKey: string,
  targetKey: string,
  path: string,
): string {
  return JSON.stringify([CHANGES_KEY_PREFIX, "inline-directory", sectionKey, targetKey, path]);
}

export function changesHistorySectionRowKey(sectionKey: string): string {
  return rowKey("history-section", sectionKey);
}

export function changesCommitExpansionKey(sectionKey: string, target: CommitDetailTarget): string {
  return JSON.stringify([
    CHANGES_KEY_PREFIX,
    "commit-expansion",
    sectionKey,
    commitDetailTargetKey(target),
  ]);
}

export function buildChangesHistoryTimelineRows(
  sections: ChangesHistorySectionInput[],
  collapsedCommitKeys: ReadonlySet<string>,
): ChangesHistoryTimelineRow[] {
  const rows: ChangesHistoryTimelineRow[] = [];
  for (const section of sections)
    appendChangesHistorySectionRows(rows, section, collapsedCommitKeys);
  return rows;
}

function appendChangesHistorySectionRows(
  rows: ChangesHistoryTimelineRow[],
  section: ChangesHistorySectionInput,
  collapsedCommitKeys: ReadonlySet<string>,
): void {
  const sectionGroup: ChangesTimelineGroupDescriptor = {
    key: `history-section:${section.sectionKey}`,
    testId: section.testId,
  };
  const count = section.kind === "pr" ? section.files.length : section.commits.length;
  rows.push({
    kind: "history-section",
    key: changesHistorySectionRowKey(section.sectionKey),
    sectionKey: section.sectionKey,
    historyKind: section.kind,
    label: section.label,
    testId: section.testId,
    count,
    collapsed: section.collapsed,
    showActions: section.kind === "commits" ? section.showActions !== false : false,
    groups: [sectionGroup],
  });
  if (section.collapsed || count === 0) return;

  if (section.kind === "pr") {
    appendPRHistoryRows(rows, section, sectionGroup, {
      key: `history-pr-files-list:${section.sectionKey}`,
      testId: "pr-files-list",
      attributes: { role: "list" },
    });
    return;
  }
  appendCommitHistoryRows(rows, section, sectionGroup, collapsedCommitKeys);
}

function appendPRHistoryRows(
  rows: ChangesHistoryTimelineRow[],
  section: Extract<ChangesHistorySectionInput, { kind: "pr" }>,
  sectionGroup: ChangesTimelineGroupDescriptor,
  fileListGroup: ChangesTimelineGroupDescriptor,
): void {
  const groups = groupByRepositoryName(section.files, (file) => file.repository_name);
  const showRepositoryHeaders = groups.length > 1 || (groups[0]?.repositoryName ?? "") !== "";
  for (const group of groups) {
    const repositoryKey = changesHistoryRepositoryExpansionKey(
      section.sectionKey,
      group.repositoryName,
    );
    const collapsed = section.collapsedRepositories.has(repositoryKey);
    const repositoryGroup: ChangesTimelineGroupDescriptor = {
      key: repositoryKey,
      ...(showRepositoryHeaders
        ? {
            testId: "pr-files-repo-group",
            attributes: { "data-repository-name": group.repositoryName || "" },
          }
        : {}),
    };
    if (showRepositoryHeaders) {
      rows.push({
        kind: HISTORY_REPOSITORY_ROW_KIND,
        key: rowKey(HISTORY_REPOSITORY_ROW_KIND, section.sectionKey, group.repositoryName),
        sectionKey: section.sectionKey,
        historyKind: "pr",
        repositoryName: group.repositoryName,
        count: group.items.length,
        collapsed,
        showActions: false,
        groups: [sectionGroup, fileListGroup, repositoryGroup],
      });
    }
    if (collapsed) continue;
    appendPRFileRows(
      rows,
      section,
      group,
      showRepositoryHeaders
        ? [sectionGroup, fileListGroup, repositoryGroup]
        : [sectionGroup, fileListGroup],
    );
  }
}

function appendPRFileRows(
  rows: ChangesHistoryTimelineRow[],
  section: Extract<ChangesHistorySectionInput, { kind: "pr" }>,
  group: { repositoryName: string; items: ChangesHistoryPRFileInput[] },
  groups: ChangesTimelineGroupDescriptor[],
): void {
  for (const file of group.items) {
    rows.push({
      kind: "pr-file",
      key: rowKey("pr-file", section.sectionKey, group.repositoryName, file.prKey ?? "", file.path),
      sectionKey: section.sectionKey,
      path: file.path,
      ...(file.prKey ? { prKey: file.prKey } : {}),
      status: file.status,
      plus: file.plus,
      minus: file.minus,
      oldPath: file.oldPath,
      repositoryName: group.repositoryName,
      groups,
    });
  }
}

function appendCommitHistoryRows(
  rows: ChangesHistoryTimelineRow[],
  section: Extract<ChangesHistorySectionInput, { kind: "commits" }>,
  sectionGroup: ChangesTimelineGroupDescriptor,
  collapsedCommitKeys: ReadonlySet<string>,
): void {
  const commitListGroup: ChangesTimelineGroupDescriptor = {
    key: `history-commits-list:${section.sectionKey}`,
    testId: "commits-list",
    attributes: { role: "list" },
  };
  const groups = groupByRepositoryName(section.commits, (item) => item.commit.repository_name);
  const singleUnscopedRepo = groups.length <= 1 && (groups[0]?.repositoryName ?? "") === "";
  for (const group of groups) {
    appendCommitRepositoryRows(rows, {
      section,
      sectionGroup,
      commitListGroup,
      group,
      showRepositoryHeader: !singleUnscopedRepo,
      collapsedCommitKeys,
    });
  }
}

function appendCommitRepositoryRows(
  rows: ChangesHistoryTimelineRow[],
  options: {
    section: Extract<ChangesHistorySectionInput, { kind: "commits" }>;
    sectionGroup: ChangesTimelineGroupDescriptor;
    commitListGroup: ChangesTimelineGroupDescriptor;
    group: { repositoryName: string; items: ChangesHistoryCommitInput[] };
    showRepositoryHeader: boolean;
    collapsedCommitKeys: ReadonlySet<string>;
  },
): void {
  const { section, sectionGroup, commitListGroup, group, showRepositoryHeader } = options;
  const repositoryName = group.repositoryName;
  const repositoryKey = changesHistoryRepositoryExpansionKey(section.sectionKey, repositoryName);
  const collapsed = section.collapsedRepositories.has(repositoryKey);
  const repositoryGroup: ChangesTimelineGroupDescriptor = {
    key: repositoryKey,
    ...(showRepositoryHeader
      ? {
          testId: "commits-repo-group",
          attributes: { "data-repository-name": repositoryName || "" },
        }
      : {}),
  };
  if (showRepositoryHeader) {
    rows.push({
      kind: HISTORY_REPOSITORY_ROW_KIND,
      key: rowKey(HISTORY_REPOSITORY_ROW_KIND, section.sectionKey, repositoryName),
      sectionKey: section.sectionKey,
      historyKind: "commits",
      repositoryName,
      count: group.items.length,
      collapsed,
      showActions: section.showActions !== false,
      groups: [sectionGroup, commitListGroup],
    });
  }
  if (collapsed) return;

  const latestIndex = group.items.findIndex((item) => item.commit.pushed !== true);
  group.items.forEach((item, index) => {
    const commitGroups = showRepositoryHeader
      ? [sectionGroup, commitListGroup, repositoryGroup]
      : [sectionGroup, commitListGroup];
    appendCommitRows(rows, {
      section,
      item,
      index,
      latestIndex,
      groups: commitGroups,
      collapsedCommitKeys: options.collapsedCommitKeys,
    });
  });
}

function appendCommitRows(
  rows: ChangesHistoryTimelineRow[],
  options: {
    section: Extract<ChangesHistorySectionInput, { kind: "commits" }>;
    item: ChangesHistoryCommitInput;
    index: number;
    latestIndex: number;
    groups: ChangesTimelineGroupDescriptor[];
    collapsedCommitKeys: ReadonlySet<string>;
  },
): void {
  const { section, item, index, latestIndex } = options;
  const { commit, detail } = item;
  const targetKey = commitDetailTargetKey(commit.detailTarget);
  const expanded = !options.collapsedCommitKeys.has(
    changesCommitExpansionKey(section.sectionKey, commit.detailTarget),
  );
  const groups = [
    ...options.groups,
    {
      key: `history-commit:${section.sectionKey}:${targetKey}`,
      attributes: { id: inlineCommitFilesId(commit) },
    },
  ];
  rows.push({
    kind: "commit",
    key: rowKey("commit", section.sectionKey, targetKey),
    sectionKey: section.sectionKey,
    commit,
    target: commit.detailTarget,
    targetKey,
    expanded,
    isLatest: index === latestIndex,
    detail,
    groups,
  });
  if (!expanded) return;
  if (detail.status === "loading" || detail.status === "idle") {
    appendCommitStatusRow({ rows, section, commit, targetKey, status: "loading", groups });
  } else if (detail.status === "error") {
    appendCommitStatusRow({
      rows,
      section,
      commit,
      targetKey,
      status: "error",
      groups,
      error: detail.error,
    });
  } else if (detail.files.length === 0) {
    appendCommitStatusRow({ rows, section, commit, targetKey, status: "empty", groups });
  } else {
    appendCommitFileRows(rows, section, item, groups, targetKey);
  }
}

function appendCommitStatusRow(options: {
  rows: ChangesHistoryTimelineRow[];
  section: Extract<ChangesHistorySectionInput, { kind: "commits" }>;
  commit: CommitItem;
  targetKey: string;
  status: "loading" | "error" | "empty";
  groups: ChangesTimelineGroupDescriptor[];
  error?: string;
}): void {
  options.rows.push(
    historyCommitStatusRow({
      sectionKey: options.section.sectionKey,
      targetKey: options.targetKey,
      target: options.commit.detailTarget,
      status: options.status,
      groups: options.groups,
      error: options.error,
    }),
  );
}

function appendCommitFileRows(
  rows: ChangesHistoryTimelineRow[],
  section: Extract<ChangesHistorySectionInput, { kind: "commits" }>,
  item: ChangesHistoryCommitInput,
  groups: ChangesTimelineGroupDescriptor[],
  targetKey: string,
): void {
  const { commit, detail } = item;
  if (section.layout !== "tree") {
    for (const file of detail.files) {
      rows.push({
        kind: COMMIT_FILE_ROW_KIND,
        key: rowKey(COMMIT_FILE_ROW_KIND, section.sectionKey, targetKey, file.path),
        sectionKey: section.sectionKey,
        target: commit.detailTarget,
        file,
        treeMode: false,
        indentPx: 0,
        groups,
      });
    }
    return;
  }
  const changedFiles: ChangedFile[] = detail.files.map((file) => ({
    path: file.path,
    status: file.status,
    staged: false,
    plus: file.plus,
    minus: file.minus,
    oldPath: file.oldPath,
    isSymlink: file.isSymlink,
    repositoryName: file.repositoryName,
  }));
  const visibleRows = flattenChangesTree(buildChangesTree(changedFiles), (path) =>
    section.collapsedDirectories.has(
      changesInlineDirectoryExpansionKey(section.sectionKey, targetKey, path),
    ),
  );
  const filesByPath = new Map(detail.files.map((file) => [file.path, file]));
  for (const row of visibleRows) {
    if (row.isDir) {
      rows.push({
        kind: "commit-directory",
        key: rowKey("commit-directory", section.sectionKey, targetKey, row.path),
        sectionKey: section.sectionKey,
        target: commit.detailTarget,
        row,
        groups,
      });
      continue;
    }
    const file = row.node.file;
    if (!file) continue;
    const inlineFile = filesByPath.get(file.path);
    if (!inlineFile) continue;
    rows.push({
      kind: COMMIT_FILE_ROW_KIND,
      key: rowKey(COMMIT_FILE_ROW_KIND, section.sectionKey, targetKey, file.path),
      sectionKey: section.sectionKey,
      target: commit.detailTarget,
      file: inlineFile,
      treeMode: true,
      indentPx: row.depth * 12,
      groups,
    });
  }
}

function historyCommitStatusRow(options: {
  sectionKey: string;
  targetKey: string;
  target: CommitDetailTarget;
  status: "loading" | "error" | "empty";
  groups: ChangesTimelineGroupDescriptor[];
  error?: string;
}): ChangesHistoryTimelineRow {
  const { sectionKey, targetKey, target, status, groups, error } = options;
  return {
    kind: "commit-status",
    key: rowKey("commit-status", sectionKey, targetKey, status),
    sectionKey,
    target,
    status,
    ...(error ? { error } : {}),
    groups,
  };
}

function inlineCommitFilesId(commit: CommitItem): string {
  return `commit-inline-files-${commit.detailTarget.source}-${commit.commit_sha}-${commit.repository_name ?? "root"}`.replace(
    /[^a-zA-Z0-9_-]/g,
    "-",
  );
}
