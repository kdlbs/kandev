import { groupByRepositoryName, isSingleRepoGroup } from "@/lib/group-by-repo";
import type { VisibleRow } from "@/hooks/use-tree";
import type { ChangedFile } from "./changes-panel-helpers";
import {
  buildChangesTree,
  flattenChangesTree,
  type ChangesTreeNode,
} from "./changes-file-tree-model";
import type { ChangesFileSection } from "./changes-timeline-selection";
import type { ChangesHistoryTimelineRow } from "./changes-timeline-history-model";

export {
  buildChangesHistoryTimelineRows,
  changesCommitExpansionKey,
  changesHistoryRepositoryExpansionKey,
  changesHistorySectionRowKey,
  changesInlineDirectoryExpansionKey,
} from "./changes-timeline-history-model";
export type {
  ChangesHistoryCommitInput,
  ChangesHistoryPRFileInput,
  ChangesHistorySectionInput,
  ChangesHistoryTimelineRow,
  ChangesTimelineGroupDescriptor,
} from "./changes-timeline-history-model";

type ChangesLayout = "flat" | "tree";

export type ChangesTimelineRow =
  | {
      kind: "section";
      key: string;
      variant: ChangesFileSection;
      count: number;
      collapsed: boolean;
      singleRepository: boolean;
    }
  | {
      kind: "repository";
      key: string;
      repositoryGroupKey: string;
      variant: ChangesFileSection;
      repositoryName: string;
      count: number;
      collapsed: boolean;
      treeMode: boolean;
    }
  | {
      kind: "directory";
      key: string;
      variant: ChangesFileSection;
      repositoryName: string;
      repositoryGroupKey?: string;
      row: VisibleRow<ChangesTreeNode>;
      nested: boolean;
    }
  | {
      kind: "file";
      key: string;
      variant: ChangesFileSection;
      file: ChangedFile;
      repositoryGroupKey?: string;
      repositoryName?: string;
      treeMode: boolean;
      indentPx: number;
    }
  | {
      kind: "selection";
      key: string;
      variant: ChangesFileSection;
      selectedKeys: string[];
    }
  | ChangesHistoryTimelineRow;

export type ChangesTimelineSectionInput = {
  variant: ChangesFileSection;
  files: ChangedFile[];
  collapsed: boolean;
  selectedKeys: string[];
};

const CHANGES_KEY_PREFIX = "changes";

export function changesRepositoryExpansionKey(
  variant: ChangesFileSection,
  repositoryName: string,
): string {
  return JSON.stringify([CHANGES_KEY_PREFIX, "repository", variant, repositoryName]);
}

export function changesDirectoryExpansionKey(
  variant: ChangesFileSection,
  repositoryName: string,
  path: string,
): string {
  return JSON.stringify([CHANGES_KEY_PREFIX, "directory", variant, repositoryName, path]);
}

function rowKey(...parts: unknown[]): string {
  return JSON.stringify([CHANGES_KEY_PREFIX, ...parts]);
}

export function buildChangesTimelineRows(args: {
  layout: ChangesLayout;
  sections: ChangesTimelineSectionInput[];
  collapsedRepositories: ReadonlySet<string>;
  collapsedDirectories: ReadonlySet<string>;
}): ChangesTimelineRow[] {
  const rows: ChangesTimelineRow[] = [];
  for (const section of args.sections) appendWorkingTreeSectionRows(rows, section, args);
  return rows;
}

function appendWorkingTreeSectionRows(
  rows: ChangesTimelineRow[],
  section: ChangesTimelineSectionInput,
  options: Pick<
    Parameters<typeof buildChangesTimelineRows>[0],
    "layout" | "collapsedRepositories" | "collapsedDirectories"
  >,
): void {
  const groups = groupByRepositoryName(section.files, (file) => file.repositoryName);
  const singleRepository = section.files.length > 0 && isSingleRepoGroup(groups);
  rows.push({
    kind: "section",
    key: rowKey("section", section.variant),
    variant: section.variant,
    count: section.files.length,
    collapsed: section.collapsed,
    singleRepository,
  });
  if (section.collapsed || section.files.length === 0) return;

  if (singleRepository) {
    appendWorkingTreeFiles({
      rows,
      section,
      files: groups[0].items,
      layout: options.layout,
      collapsedDirectories: options.collapsedDirectories,
      repositoryName: "",
      nested: false,
    });
  } else {
    for (const group of groups) {
      const repositoryGroupKey = changesRepositoryExpansionKey(
        section.variant,
        group.repositoryName,
      );
      const collapsed = options.collapsedRepositories.has(repositoryGroupKey);
      rows.push({
        kind: "repository",
        key: rowKey("repository", section.variant, group.repositoryName),
        repositoryGroupKey,
        variant: section.variant,
        repositoryName: group.repositoryName,
        count: group.items.length,
        collapsed,
        treeMode: options.layout === "tree",
      });
      if (!collapsed) {
        appendWorkingTreeFiles({
          rows,
          section,
          files: group.items,
          layout: options.layout,
          collapsedDirectories: options.collapsedDirectories,
          repositoryName: group.repositoryName,
          nested: options.layout === "tree",
          repositoryGroupKey,
        });
      }
    }
  }

  if (section.selectedKeys.length > 0) {
    rows.push({
      kind: "selection",
      key: rowKey("selection", section.variant),
      variant: section.variant,
      selectedKeys: section.selectedKeys,
    });
  }
}

function appendWorkingTreeFiles(options: {
  rows: ChangesTimelineRow[];
  section: ChangesTimelineSectionInput;
  files: ChangedFile[];
  layout: ChangesLayout;
  collapsedDirectories: ReadonlySet<string>;
  repositoryName: string;
  nested: boolean;
  repositoryGroupKey?: string;
}): void {
  const {
    rows,
    section,
    files,
    layout,
    collapsedDirectories,
    repositoryName,
    nested,
    repositoryGroupKey,
  } = options;
  if (layout !== "tree") {
    for (const file of files) {
      rows.push({
        kind: "file",
        key: rowKey("file", section.variant, repositoryName, file.changeLayer ?? "", file.path),
        variant: section.variant,
        file,
        ...(repositoryGroupKey ? { repositoryGroupKey, repositoryName } : {}),
        treeMode: false,
        indentPx: 0,
      });
    }
    return;
  }

  const tree = buildChangesTree(files);
  const visibleRows = flattenChangesTree(tree, (path) =>
    collapsedDirectories.has(changesDirectoryExpansionKey(section.variant, repositoryName, path)),
  );
  for (const treeRow of visibleRows) {
    if (treeRow.isDir) {
      rows.push({
        kind: "directory",
        key: rowKey("directory", section.variant, repositoryName, treeRow.path),
        variant: section.variant,
        repositoryName,
        ...(repositoryGroupKey ? { repositoryGroupKey } : {}),
        row: treeRow,
        nested,
      });
      continue;
    }
    const file = treeRow.node.file;
    if (!file) continue;
    rows.push({
      kind: "file",
      key: rowKey("file", section.variant, repositoryName, file.changeLayer ?? "", file.path),
      variant: section.variant,
      file,
      ...(repositoryGroupKey ? { repositoryGroupKey, repositoryName } : {}),
      treeMode: true,
      indentPx: (nested ? 12 : 0) + treeRow.depth * 12,
    });
  }
}
