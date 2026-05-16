"use client";

import { useCallback, type ReactNode } from "react";
import {
  IconChevronRight,
  IconChevronDown,
  IconFolder,
  IconFolderOpen,
  IconFileText,
} from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { cn } from "@/lib/utils";
import { useTree, type VisibleRow } from "@/hooks/use-tree";

export interface FileTreeNode {
  name: string;
  path: string;
  isDir: boolean;
  children: FileTreeNode[];
  content?: string;
}

interface FileTreeProps {
  nodes: FileTreeNode[];
  selectedPath?: string | null;
  onSelectPath?: (path: string) => void;
  checkedPaths?: Set<string>;
  onCheckedPathsChange?: (paths: Set<string>) => void;
  showCheckboxes?: boolean;
  renderExtra?: (node: FileTreeNode) => ReactNode;
  defaultExpanded?: boolean;
}

/** Collect all leaf (file) paths under a node. */
function getLeafPaths(node: FileTreeNode): string[] {
  if (!node.isDir) return [node.path];
  return node.children.flatMap(getLeafPaths);
}

type CheckState = boolean | "indeterminate";

function getCheckState(node: FileTreeNode, checkedPaths: Set<string>): CheckState {
  const leaves = getLeafPaths(node);
  if (leaves.every((p) => checkedPaths.has(p))) return true;
  if (leaves.some((p) => checkedPaths.has(p))) return "indeterminate";
  return false;
}

export function FileTree({
  nodes,
  selectedPath,
  onSelectPath,
  checkedPaths,
  onCheckedPathsChange,
  showCheckboxes = false,
  renderExtra,
  defaultExpanded = false,
}: FileTreeProps) {
  const { visibleRows, toggle } = useTree<FileTreeNode>({
    nodes,
    getPath: (n) => n.path,
    getChildren: (n) => n.children,
    isDir: (n) => n.isDir,
    chainCollapse: true,
    defaultExpanded: defaultExpanded ? "all" : undefined,
  });

  const handleToggleCheck = useCallback(
    (node: FileTreeNode) => {
      if (!checkedPaths || !onCheckedPathsChange) return;
      const paths = getLeafPaths(node);
      const allChecked = paths.every((p) => checkedPaths.has(p));
      const next = new Set(checkedPaths);
      for (const p of paths) {
        if (allChecked) next.delete(p);
        else next.add(p);
      }
      onCheckedPathsChange(next);
    },
    [checkedPaths, onCheckedPathsChange],
  );

  return (
    <div className="overflow-y-auto py-1">
      {visibleRows.map((row) => (
        <FileTreeRow
          key={row.path}
          row={row}
          isActive={!row.isDir && selectedPath === row.path}
          checkState={
            showCheckboxes && checkedPaths ? getCheckState(row.node, checkedPaths) : undefined
          }
          showCheckboxes={showCheckboxes}
          onClick={() => {
            if (row.isDir) toggle(row.path);
            else onSelectPath?.(row.path);
          }}
          onToggleCheck={() => handleToggleCheck(row.node)}
          renderExtra={renderExtra}
        />
      ))}
    </div>
  );
}

interface FileTreeRowProps {
  row: VisibleRow<FileTreeNode>;
  isActive: boolean;
  checkState?: CheckState;
  showCheckboxes: boolean;
  onClick: () => void;
  onToggleCheck: () => void;
  renderExtra?: (node: FileTreeNode) => ReactNode;
}

function FileTreeRow({
  row,
  isActive,
  checkState,
  showCheckboxes,
  onClick,
  onToggleCheck,
  renderExtra,
}: FileTreeRowProps) {
  const FolderIcon = row.isExpanded ? IconFolderOpen : IconFolder;
  const ChevronIcon = row.isExpanded ? IconChevronDown : IconChevronRight;

  return (
    <div
      className={cn(
        "flex items-center gap-1.5 px-2 py-1 text-sm cursor-pointer hover:bg-accent/50",
        isActive && "bg-accent",
      )}
      style={{ paddingLeft: `${row.depth * 16 + 8}px` }}
      onClick={onClick}
    >
      {showCheckboxes && checkState !== undefined && (
        <Checkbox
          checked={checkState}
          onCheckedChange={onToggleCheck}
          onClick={(e) => e.stopPropagation()}
          className="cursor-pointer shrink-0"
        />
      )}
      {row.isDir && <ChevronIcon className="h-3.5 w-3.5 text-muted-foreground shrink-0" />}
      {row.isDir ? (
        <FolderIcon className="h-4 w-4 text-muted-foreground shrink-0" />
      ) : (
        <IconFileText className="h-4 w-4 text-muted-foreground shrink-0" />
      )}
      <span className="truncate">{row.displayName}</span>
      {renderExtra?.(row.node)}
    </div>
  );
}
