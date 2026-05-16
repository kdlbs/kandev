"use client";

import { useState } from "react";
import {
  IconFile,
  IconFolder,
  IconFolderOpen,
  IconChevronRight,
  IconChevronDown,
  IconSearch,
} from "@tabler/icons-react";
import { Input } from "@kandev/ui/input";
import { Checkbox } from "@kandev/ui/checkbox";
import { cn } from "@/lib/utils";
import { useTree, type VisibleRow } from "@/hooks/use-tree";
import type { ExportFile, FileTreeNode } from "./export-types";
import { getDescendantFilePaths } from "./export-utils";

interface ExportFileTreeProps {
  tree: FileTreeNode[];
  files: ExportFile[];
  selectedPaths: Set<string>;
  onSelectedPathsChange: (next: Set<string>) => void;
  previewPath: string | null;
  onPreviewPathChange: (path: string) => void;
}

// Stable adapter identities so useTree's visibleRows memoisation is not
// invalidated on every parent render.
const EXPORT_GET_PATH = (n: FileTreeNode) => n.path;
const EXPORT_GET_CHILDREN = (n: FileTreeNode) => n.children;
const EXPORT_IS_DIR = (n: FileTreeNode) => n.isDir;

type CheckState = boolean | "indeterminate";

function getCheckState(node: FileTreeNode, selectedPaths: Set<string>): CheckState {
  const descendants = getDescendantFilePaths(node);
  if (descendants.every((p) => selectedPaths.has(p))) return true;
  if (descendants.some((p) => selectedPaths.has(p))) return "indeterminate";
  return false;
}

export function ExportFileTree({
  tree,
  selectedPaths,
  onSelectedPathsChange,
  previewPath,
  onPreviewPathChange,
}: ExportFileTreeProps) {
  const [search, setSearch] = useState("");

  const { visibleRows, toggle } = useTree<FileTreeNode>({
    nodes: tree,
    getPath: EXPORT_GET_PATH,
    getChildren: EXPORT_GET_CHILDREN,
    isDir: EXPORT_IS_DIR,
    defaultExpanded: "all",
    search,
    searchMode: "hide",
  });

  const toggleSelection = (node: FileTreeNode) => {
    const paths = getDescendantFilePaths(node);
    const allSelected = paths.every((p) => selectedPaths.has(p));
    const next = new Set(selectedPaths);
    for (const p of paths) {
      if (allSelected) next.delete(p);
      else next.add(p);
    }
    onSelectedPathsChange(next);
  };

  return (
    <div className="w-[350px] shrink-0 border-r border-border flex flex-col">
      <div className="p-2 border-b border-border">
        <div className="relative">
          <IconSearch className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
          <Input
            placeholder="Search files..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-8 h-8 text-sm"
          />
        </div>
      </div>
      <div className="flex-1 overflow-y-auto py-1">
        {visibleRows.map((row) => (
          <ExportTreeRow
            key={row.path}
            row={row}
            isActive={!row.isDir && previewPath === row.path}
            checkState={getCheckState(row.node, selectedPaths)}
            onClick={() => {
              if (row.isDir) toggle(row.path);
              else onPreviewPathChange(row.path);
            }}
            onToggleCheck={() => toggleSelection(row.node)}
          />
        ))}
      </div>
    </div>
  );
}

interface ExportTreeRowProps {
  row: VisibleRow<FileTreeNode>;
  isActive: boolean;
  checkState: CheckState;
  onClick: () => void;
  onToggleCheck: () => void;
}

function ExportTreeRow({ row, isActive, checkState, onClick, onToggleCheck }: ExportTreeRowProps) {
  let FileIconComp = IconFile;
  if (row.isDir) FileIconComp = row.isExpanded ? IconFolderOpen : IconFolder;
  let ChevronIcon: typeof IconChevronDown | null = null;
  if (row.isDir) ChevronIcon = row.isExpanded ? IconChevronDown : IconChevronRight;
  return (
    <div
      className={cn(
        "flex items-center gap-1.5 px-2 py-1 text-sm cursor-pointer hover:bg-accent/50",
        isActive && "bg-accent",
      )}
      style={{ paddingLeft: `${row.depth * 16 + 8}px` }}
      onClick={onClick}
    >
      <Checkbox
        checked={checkState}
        onCheckedChange={onToggleCheck}
        onClick={(e) => e.stopPropagation()}
        className="cursor-pointer shrink-0"
      />
      {ChevronIcon && <ChevronIcon className="h-3.5 w-3.5 text-muted-foreground shrink-0" />}
      <FileIconComp className="h-4 w-4 text-muted-foreground shrink-0" />
      <span className="truncate">{row.displayName}</span>
    </div>
  );
}
