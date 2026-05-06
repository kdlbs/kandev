"use client";

import { useCallback, useState, type ReactNode } from "react";
import {
  IconChevronRight,
  IconChevronDown,
  IconFolder,
  IconFolderOpen,
  IconFileText,
} from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { cn } from "@/lib/utils";

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

/** Collect all leaf (file) paths under a node */
function getLeafPaths(node: FileTreeNode): string[] {
  if (!node.isDir) return [node.path];
  return node.children.flatMap(getLeafPaths);
}

/** Collapse single-child directory chains into "a/b/c" display names */
function collapseChain(node: FileTreeNode): { displayName: string; node: FileTreeNode } {
  let current = node;
  let name = current.name;
  while (current.isDir && current.children.length === 1 && current.children[0].isDir) {
    current = current.children[0];
    name = `${name}/${current.name}`;
  }
  return { displayName: name, node: current };
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
  const [expanded, setExpanded] = useState<Set<string>>(() => {
    if (!defaultExpanded) return new Set<string>();
    const dirs = new Set<string>();
    function walk(list: FileTreeNode[]) {
      for (const n of list) {
        if (n.isDir) {
          dirs.add(n.path);
          walk(n.children);
        }
      }
    }
    walk(nodes);
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

  const toggleCheck = useCallback(
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
      {nodes.map((node) => (
        <TreeNodeRow
          key={node.path}
          node={node}
          depth={0}
          expanded={expanded}
          onToggleExpand={toggleExpand}
          selectedPath={selectedPath}
          onSelectPath={onSelectPath}
          checkedPaths={checkedPaths}
          showCheckboxes={showCheckboxes}
          onToggleCheck={toggleCheck}
          renderExtra={renderExtra}
        />
      ))}
    </div>
  );
}

interface TreeNodeRowProps {
  node: FileTreeNode;
  depth: number;
  expanded: Set<string>;
  onToggleExpand: (path: string) => void;
  selectedPath?: string | null;
  onSelectPath?: (path: string) => void;
  checkedPaths?: Set<string>;
  showCheckboxes: boolean;
  onToggleCheck: (node: FileTreeNode) => void;
  renderExtra?: (node: FileTreeNode) => ReactNode;
}

function TreeNodeRow({
  node,
  depth,
  expanded,
  onToggleExpand,
  selectedPath,
  onSelectPath,
  checkedPaths,
  showCheckboxes,
  onToggleCheck,
  renderExtra,
}: TreeNodeRowProps) {
  const { displayName, node: effectiveNode } = collapseChain(node);
  const isExpanded = expanded.has(effectiveNode.path);
  const isActive = !effectiveNode.isDir && selectedPath === effectiveNode.path;

  const handleClick = () => {
    if (effectiveNode.isDir) {
      onToggleExpand(effectiveNode.path);
    } else {
      onSelectPath?.(effectiveNode.path);
    }
  };

  const checkState = (() => {
    if (!showCheckboxes || !checkedPaths) return undefined;
    const leaves = getLeafPaths(effectiveNode);
    const allChecked = leaves.every((p) => checkedPaths.has(p));
    if (allChecked) return true;
    const someChecked = leaves.some((p) => checkedPaths.has(p));
    if (someChecked) return "indeterminate" as const;
    return false;
  })();

  const FolderIcon = isExpanded ? IconFolderOpen : IconFolder;
  const ChevronIcon = isExpanded ? IconChevronDown : IconChevronRight;

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
        {showCheckboxes && checkState !== undefined && (
          <Checkbox
            checked={checkState}
            onCheckedChange={() => onToggleCheck(effectiveNode)}
            onClick={(e) => e.stopPropagation()}
            className="cursor-pointer shrink-0"
          />
        )}
        {effectiveNode.isDir && (
          <ChevronIcon className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        )}
        {effectiveNode.isDir ? (
          <FolderIcon className="h-4 w-4 text-muted-foreground shrink-0" />
        ) : (
          <IconFileText className="h-4 w-4 text-muted-foreground shrink-0" />
        )}
        <span className="truncate">{displayName}</span>
        {renderExtra?.(effectiveNode)}
      </div>
      {effectiveNode.isDir &&
        isExpanded &&
        effectiveNode.children.map((child) => (
          <TreeNodeRow
            key={child.path}
            node={child}
            depth={depth + 1}
            expanded={expanded}
            onToggleExpand={onToggleExpand}
            selectedPath={selectedPath}
            onSelectPath={onSelectPath}
            checkedPaths={checkedPaths}
            showCheckboxes={showCheckboxes}
            onToggleCheck={onToggleCheck}
            renderExtra={renderExtra}
          />
        ))}
    </>
  );
}
