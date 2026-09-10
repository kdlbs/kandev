import type React from "react";
import type { FileTreeNode } from "@/lib/types/backend";
import type { FileInfo } from "@/lib/state/store";
import type { VisibleRow } from "@/hooks/use-tree";

type GitFileStatus = FileInfo["status"] | undefined;

export type TreeNodeRowProps = {
  row: VisibleRow<FileTreeNode>;
  activeFolderPath: string;
  activeFilePath?: string | null;
  visibleLoadingPaths: Set<string>;
  fileStatuses: Map<string, GitFileStatus>;
  tree: FileTreeNode | null;
  treeRef?: React.RefObject<FileTreeNode | null>;
  onToggleExpand: (node: FileTreeNode) => void;
  onOpenFile: (path: string) => void;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onDownloadFile?: (path: string) => Promise<boolean>;
  onUploadFilesHere?: (path: string) => void;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  isSelectedFn?: (path: string) => boolean;
  onSelect?: (path: string, e: React.MouseEvent) => boolean;
  isDragging?: boolean;
  dragOverPath?: string | null;
  onDragStart?: (path: string, e: React.DragEvent) => void;
  onDragEnd?: () => void;
  onDragOver?: (path: string, e: React.DragEvent) => void;
  onDragLeave?: (e: React.DragEvent) => void;
  onDrop?: (targetPath: string, e: React.DragEvent) => void;
  selectedCount?: number;
  selectedPaths?: Set<string>;
  showTouchActions?: boolean;
  onAddToChatContext?: (node: FileTreeNode) => void;
};

function rowIdentity(props: TreeNodeRowProps): unknown[] {
  const path = props.row.path;
  return [
    props.row.node,
    props.row.path,
    props.row.depth,
    props.row.displayName,
    props.row.isExpanded,
    props.row.isDir,
    props.activeFilePath === path,
    props.activeFolderPath === path,
    props.visibleLoadingPaths.has(path),
    props.fileStatuses.get(path),
    props.isSelectedFn?.(path),
    Boolean(props.isDragging && props.isSelectedFn?.(path)),
    props.dragOverPath === path,
    props.treeRef,
    props.treeRef ? undefined : props.tree,
    props.onToggleExpand,
    props.onOpenFile,
    props.onDeleteFile,
    props.onRenameFile,
    props.onDownloadFile,
    props.onUploadFilesHere,
    props.setTree,
    props.isSelectedFn,
    props.onSelect,
    props.onDragStart,
    props.onDragEnd,
    props.onDragOver,
    props.onDragLeave,
    props.onDrop,
    props.selectedCount,
    props.selectedPaths,
    props.showTouchActions,
    props.onAddToChatContext,
  ];
}

export function areTreeNodeRowPropsEqual(
  previous: TreeNodeRowProps,
  next: TreeNodeRowProps,
): boolean {
  const previousIdentity = rowIdentity(previous);
  const nextIdentity = rowIdentity(next);
  return previousIdentity.every((value, index) => Object.is(value, nextIdentity[index]));
}
