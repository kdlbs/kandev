"use client";

import { useCallback, useState } from "react";
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

export function ExportFileTree({
  tree,
  files,
  selectedPaths,
  onSelectedPathsChange,
  previewPath,
  onPreviewPathChange,
}: ExportFileTreeProps) {
  const [search, setSearch] = useState("");
  const [expanded, setExpanded] = useState<Set<string>>(() => {
    // Expand all directories by default
    const dirs = new Set<string>();
    function walk(nodes: FileTreeNode[]) {
      for (const n of nodes) {
        if (n.isDir) {
          dirs.add(n.path);
          walk(n.children);
        }
      }
    }
    walk(tree);
    return dirs;
  });

  const toggleExpand = useCallback((path: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }, []);

  const toggleSelection = useCallback(
    (node: FileTreeNode) => {
      const paths = getDescendantFilePaths(node);
      const allSelected = paths.every((p) => selectedPaths.has(p));
      const next = new Set(selectedPaths);
      for (const p of paths) {
        if (allSelected) next.delete(p);
        else next.add(p);
      }
      onSelectedPathsChange(next);
    },
    [selectedPaths, onSelectedPathsChange],
  );

  const lowerSearch = search.toLowerCase();
  const matchesSearch = (node: FileTreeNode): boolean => {
    if (!lowerSearch) return true;
    if (node.name.toLowerCase().includes(lowerSearch)) return true;
    return node.children.some(matchesSearch);
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
        {tree.filter(matchesSearch).map((node) => (
          <TreeRow
            key={node.path}
            node={node}
            depth={0}
            expanded={expanded}
            selectedPaths={selectedPaths}
            previewPath={previewPath}
            onToggleExpand={toggleExpand}
            onToggleSelection={toggleSelection}
            onPreview={onPreviewPathChange}
            matchesSearch={matchesSearch}
            allFiles={files}
          />
        ))}
      </div>
    </div>
  );
}

interface TreeRowProps {
  node: FileTreeNode;
  depth: number;
  expanded: Set<string>;
  selectedPaths: Set<string>;
  previewPath: string | null;
  onToggleExpand: (path: string) => void;
  onToggleSelection: (node: FileTreeNode) => void;
  onPreview: (path: string) => void;
  matchesSearch: (node: FileTreeNode) => boolean;
  allFiles: ExportFile[];
}

function TreeRow({
  node,
  depth,
  expanded,
  selectedPaths,
  previewPath,
  onToggleExpand,
  onToggleSelection,
  onPreview,
  matchesSearch,
  allFiles,
}: TreeRowProps) {
  const isExpanded = expanded.has(node.path);
  const descendants = getDescendantFilePaths(node);
  const allSelected = descendants.every((p) => selectedPaths.has(p));
  const someSelected = !allSelected && descendants.some((p) => selectedPaths.has(p));
  const isActive = !node.isDir && previewPath === node.path;

  const handleClick = () => {
    if (node.isDir) {
      onToggleExpand(node.path);
    } else {
      onPreview(node.path);
    }
  };

  let ChevronIcon: typeof IconChevronDown | null = null;
  if (node.isDir) ChevronIcon = isExpanded ? IconChevronDown : IconChevronRight;

  let FileIcon = IconFile;
  if (node.isDir) FileIcon = isExpanded ? IconFolderOpen : IconFolder;

  let checkedState: boolean | "indeterminate" = false;
  if (allSelected) checkedState = true;
  else if (someSelected) checkedState = "indeterminate";

  return (
    <>
      <div
        className={cn(
          "flex items-center gap-1.5 px-2 py-1 text-sm cursor-pointer hover:bg-accent/50",
          isActive && "bg-accent",
        )}
        style={{ paddingLeft: `${depth * 16 + 8}px` }}
        onClick={handleClick}
      >
        <Checkbox
          checked={checkedState}
          onCheckedChange={() => onToggleSelection(node)}
          onClick={(e) => e.stopPropagation()}
          className="cursor-pointer shrink-0"
        />
        {ChevronIcon && <ChevronIcon className="h-3.5 w-3.5 text-muted-foreground shrink-0" />}
        <FileIcon className="h-4 w-4 text-muted-foreground shrink-0" />
        <span className="truncate">{node.name}</span>
      </div>
      {node.isDir &&
        isExpanded &&
        node.children
          .filter(matchesSearch)
          .map((child) => (
            <TreeRow
              key={child.path}
              node={child}
              depth={depth + 1}
              expanded={expanded}
              selectedPaths={selectedPaths}
              previewPath={previewPath}
              onToggleExpand={onToggleExpand}
              onToggleSelection={onToggleSelection}
              onPreview={onPreview}
              matchesSearch={matchesSearch}
              allFiles={allFiles}
            />
          ))}
    </>
  );
}
