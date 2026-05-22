"use client";

import { useMemo } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTree, type VisibleRow } from "@/hooks/use-tree";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { useMultiSelect } from "@/hooks/use-multi-select";
import { FileRow } from "./changes-panel-file-row";
import type { FileInfo } from "@/lib/state/store";
import type { OpenDiffOptions } from "./changes-diff-target";

type ChangedFile = {
  path: string;
  status: FileInfo["status"];
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
  repositoryName?: string;
};

type TreeNode = {
  name: string;
  path: string;
  isDir: boolean;
  children?: TreeNode[];
  file?: ChangedFile;
};

/**
 * Build a hierarchical tree from a flat list of changed files. Single-child
 * directory chains stay separate nodes — useTree's `chainCollapse` merges them
 * into one row at render time.
 */
function buildChangesTree(files: ChangedFile[]): TreeNode[] {
  const root: TreeNode = { name: "", path: "", isDir: true, children: [] };
  for (const file of files) {
    const parts = file.path.split("/");
    let current = root;
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      const isLast = i === parts.length - 1;
      const partPath = parts.slice(0, i + 1).join("/");
      if (isLast) {
        current.children!.push({ name: part, path: file.path, isDir: false, file });
      } else {
        let child = current.children!.find((c) => c.isDir && c.name === part);
        if (!child) {
          child = { name: part, path: partPath, isDir: true, children: [] };
          current.children!.push(child);
        }
        current = child;
      }
    }
  }
  return sortNodes(root.children ?? []);
}

function sortNodes(nodes: TreeNode[]): TreeNode[] {
  return nodes
    .sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    })
    .map((n) => (n.isDir && n.children ? { ...n, children: sortNodes(n.children) } : n));
}

const GET_PATH = (n: TreeNode) => n.path;
const GET_CHILDREN = (n: TreeNode) => n.children;
const IS_DIR = (n: TreeNode) => n.isDir;

type ChangesTreeProps = {
  files: ChangedFile[];
  pendingStageFiles: Set<string>;
  onOpenDiff: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string) => void;
  onStage: (path: string, repo?: string) => void;
  onUnstage: (path: string, repo?: string) => void;
  onDiscard: (path: string, repo?: string) => void;
  variant: "unstaged" | "staged";
  /** Shared with the parent FileListSection so the BulkActionBar reflects
   *  selections made in tree mode. */
  multiSelect: ReturnType<typeof useMultiSelect>;
};

export function ChangesTree({
  files,
  pendingStageFiles,
  onOpenDiff,
  onEditFile,
  onStage,
  onUnstage,
  onDiscard,
  variant,
  multiSelect,
}: ChangesTreeProps) {
  const tree = useMemo(() => buildChangesTree(files), [files]);
  const { visibleRows, toggle } = useTree<TreeNode>({
    nodes: tree,
    getPath: GET_PATH,
    getChildren: GET_CHILDREN,
    isDir: IS_DIR,
    defaultExpanded: "all",
    chainCollapse: true,
  });

  const activeFilePath = useDockviewStore((s) => s.activeFilePath);

  return (
    <ul data-testid={`${variant}-file-tree`} className="space-y-0.5">
      {visibleRows.map((row) =>
        row.isDir ? (
          <TreeDirRow key={row.path} row={row} onToggle={() => toggle(row.path)} />
        ) : (
          <FileRow
            key={`${row.node.file?.repositoryName ?? ""}:${row.path}`}
            file={row.node.file!}
            isPending={pendingStageFiles.has(`${row.node.file?.repositoryName ?? ""}::${row.path}`)}
            isSelected={multiSelect.isSelected(row.path)}
            isActive={row.path === activeFilePath}
            onSelect={multiSelect.handleClick}
            onOpenDiff={onOpenDiff}
            onStage={onStage}
            onUnstage={onUnstage}
            onDiscard={onDiscard}
            onEditFile={onEditFile}
            treeMode
            indentPx={row.depth * 12}
          />
        ),
      )}
    </ul>
  );
}

type RepoTreeGroupProps = {
  variant: "unstaged" | "staged";
  repositoryName: string;
  displayName?: string;
  files: ChangedFile[];
  pendingStageFiles: Set<string>;
  collapsed: boolean;
  onToggle: () => void;
  onOpenDiff: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string) => void;
  onStage: (path: string, repo?: string) => void;
  onUnstage: (path: string, repo?: string) => void;
  onDiscard: (path: string, repo?: string) => void;
  primaryLabel: string;
  secondaryLabel?: string;
  onRepoAction?: (repo: string) => void;
  onRepoSecondaryAction?: (repo: string) => void;
  multiSelect: ReturnType<typeof useMultiSelect>;
};

export function RepoTreeGroup(props: RepoTreeGroupProps) {
  const {
    variant,
    repositoryName,
    displayName,
    files,
    pendingStageFiles,
    collapsed,
    onToggle,
    primaryLabel,
    secondaryLabel,
    onRepoAction,
    onRepoSecondaryAction,
  } = props;
  const label = displayName || repositoryName || "Repository";
  const stop = (e: React.MouseEvent) => e.stopPropagation();
  return (
    <div data-testid="changes-repo-group" data-repository-name={repositoryName || ""}>
      <div className="flex items-center justify-between gap-2 px-1 py-0.5">
        <button
          type="button"
          className="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground/80 uppercase tracking-wide cursor-pointer hover:text-foreground/80 min-w-0"
          data-testid="changes-repo-header"
          aria-expanded={!collapsed}
          onClick={onToggle}
        >
          {collapsed ? (
            <IconChevronRight className="h-3 w-3 text-muted-foreground/50 shrink-0" />
          ) : (
            <IconChevronDown className="h-3 w-3 text-muted-foreground/50 shrink-0" />
          )}
          <span className="truncate">{label}</span>
          <span className="text-muted-foreground/50 normal-case tracking-normal">
            {files.length}
          </span>
        </button>
        {(onRepoAction || onRepoSecondaryAction) && (
          <div className="flex items-center gap-1" onClick={stop}>
            {onRepoAction && (
              <Button
                size="sm"
                variant="ghost"
                className="h-5 text-[10px] px-1.5 cursor-pointer"
                data-testid="repo-group-action"
                onClick={() => onRepoAction(repositoryName)}
              >
                {primaryLabel}
              </Button>
            )}
            {onRepoSecondaryAction && secondaryLabel && (
              <Button
                size="sm"
                variant="ghost"
                className="h-5 text-[10px] px-1.5 cursor-pointer text-muted-foreground"
                data-testid="repo-group-secondary-action"
                onClick={() => onRepoSecondaryAction(repositoryName)}
              >
                {secondaryLabel}
              </Button>
            )}
          </div>
        )}
      </div>
      {!collapsed && (
        <ChangesTree
          files={files}
          pendingStageFiles={pendingStageFiles}
          onOpenDiff={props.onOpenDiff}
          onEditFile={props.onEditFile}
          onStage={props.onStage}
          onUnstage={props.onUnstage}
          onDiscard={props.onDiscard}
          variant={variant}
          multiSelect={props.multiSelect}
        />
      )}
    </div>
  );
}

function TreeDirRow({ row, onToggle }: { row: VisibleRow<TreeNode>; onToggle: () => void }) {
  return (
    <li>
      <button
        type="button"
        className="flex items-center w-full gap-1 px-1 py-0.5 -mx-1 rounded-md hover:bg-muted/60 cursor-pointer text-xs text-foreground/70"
        style={{ paddingLeft: row.depth * 12 + 4 }}
        onClick={onToggle}
        data-testid={`tree-dir-${row.path.replace(/[/\\]/g, "-")}`}
      >
        {row.isExpanded ? (
          <IconChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
        ) : (
          <IconChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
        )}
        <span className="truncate">{row.displayName}</span>
      </button>
    </li>
  );
}
