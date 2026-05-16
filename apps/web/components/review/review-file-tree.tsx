"use client";

import { memo, useMemo, useRef } from "react";
import {
  IconChevronDown,
  IconChevronRight,
  IconMessage,
  IconAlertTriangle,
  IconSearch,
  IconX,
} from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { cn } from "@kandev/ui/lib/utils";
import { FileIcon } from "@/components/ui/file-icon";
import { useTree, type VisibleRow } from "@/hooks/use-tree";
import type { ReviewFile, FileTreeNode } from "./types";
import { buildFileTree, reviewFileKey } from "./types";

type ReviewFileTreeProps = {
  files: ReviewFile[];
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
  commentCountByFile: Record<string, number>;
  selectedFile: string | null;
  filter: string;
  onFilterChange: (value: string) => void;
  onSelectFile: (path: string) => void;
  onToggleReviewed: (path: string, reviewed: boolean) => void;
};

// Stable adapter identities so useTree's visibleRows memoisation is not
// invalidated on every parent render.
const REVIEW_GET_PATH = (n: FileTreeNode) => n.path;
const REVIEW_GET_CHILDREN = (n: FileTreeNode) => n.children;
const REVIEW_IS_DIR = (n: FileTreeNode) => Boolean(n.isDir);

export const ReviewFileTree = memo(function ReviewFileTree({
  files,
  reviewedFiles,
  staleFiles,
  commentCountByFile,
  selectedFile,
  filter,
  onFilterChange,
  onSelectFile,
  onToggleReviewed,
}: ReviewFileTreeProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const tree = useMemo(() => buildFileTree(files), [files]);

  const { visibleRows, toggle } = useTree<FileTreeNode>({
    nodes: tree,
    getPath: REVIEW_GET_PATH,
    getChildren: REVIEW_GET_CHILDREN,
    isDir: REVIEW_IS_DIR,
    defaultExpanded: "all",
  });

  return (
    <div className="flex flex-col h-full text-sm">
      <ReviewFilterInput ref={inputRef} value={filter} onChange={onFilterChange} />
      <div className="py-1 overflow-y-auto flex-1">
        {visibleRows.map((row) => (
          <ReviewTreeRow
            key={row.path}
            row={row}
            reviewedFiles={reviewedFiles}
            staleFiles={staleFiles}
            commentCountByFile={commentCountByFile}
            selectedFile={selectedFile}
            onSelectFile={onSelectFile}
            onToggleReviewed={onToggleReviewed}
            onToggleDir={() => toggle(row.path)}
          />
        ))}
      </div>
    </div>
  );
});

interface ReviewFilterInputProps {
  ref: React.RefObject<HTMLInputElement | null>;
  value: string;
  onChange: (v: string) => void;
}

function ReviewFilterInput({ ref, value, onChange }: ReviewFilterInputProps) {
  return (
    <div className="px-2 py-2 shrink-0">
      <div className="flex items-center gap-1.5 px-2 py-1 rounded-md bg-muted/50 border border-border/50 focus-within:border-border focus-within:bg-muted/80 transition-colors">
        <IconSearch className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        <input
          ref={ref}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="Filter changed files"
          className="flex-1 bg-transparent text-[13px] text-foreground placeholder:text-muted-foreground outline-none min-w-0"
        />
        {value && (
          <button
            onClick={() => {
              onChange("");
              ref.current?.focus();
            }}
            className="text-muted-foreground hover:text-foreground cursor-pointer"
          >
            <IconX className="h-3 w-3" />
          </button>
        )}
      </div>
    </div>
  );
}

interface ReviewTreeRowProps {
  row: VisibleRow<FileTreeNode>;
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
  commentCountByFile: Record<string, number>;
  selectedFile: string | null;
  onSelectFile: (path: string) => void;
  onToggleReviewed: (path: string, reviewed: boolean) => void;
  onToggleDir: () => void;
}

function ReviewTreeRow(props: ReviewTreeRowProps) {
  if (props.row.isDir) return <ReviewDirRow {...props} />;
  return <ReviewFileRow {...props} />;
}

function ReviewDirRow({ row, onToggleDir }: ReviewTreeRowProps) {
  const isRepoRoot = Boolean(row.node.isRepoRoot);
  const fileCount = isRepoRoot ? countLeafFiles(row.node) : 0;
  return (
    <div data-testid={isRepoRoot ? "repo-root-node" : "dir-node"}>
      <button
        type="button"
        className={cn(
          "flex items-center w-full gap-1 px-2 py-1 hover:bg-muted/50 transition-colors cursor-pointer",
          isRepoRoot && "border-t border-border/40 first:border-t-0 mt-1 first:mt-0",
        )}
        style={{ paddingLeft: `${row.depth * 12 + 8}px` }}
        onClick={onToggleDir}
      >
        {row.isExpanded ? (
          <IconChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        ) : (
          <IconChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        )}
        <span
          className={cn(
            "text-[13px] truncate",
            isRepoRoot ? "font-medium text-foreground" : "text-muted-foreground",
          )}
        >
          {row.node.name}
        </span>
        {isRepoRoot && fileCount > 0 && (
          <span className="ml-auto text-[10px] text-muted-foreground/70">{fileCount}</span>
        )}
      </button>
    </div>
  );
}

function ReviewFileRow({
  row,
  reviewedFiles,
  staleFiles,
  commentCountByFile,
  selectedFile,
  onSelectFile,
  onToggleReviewed,
}: ReviewTreeRowProps) {
  const file = row.node.file as ReviewFile;
  // Composite key from reviewFileKey() so two same-name files in different
  // repos don't share reviewed/stale/comment-count slots.
  const key = reviewFileKey(file);
  const isReviewed = reviewedFiles.has(key);
  const isStale = staleFiles.has(key);
  const commentCount = commentCountByFile[key] ?? 0;
  const isSelected = selectedFile === key;
  return (
    <div
      className={cn(
        "flex items-center gap-1.5 px-2 py-1 cursor-pointer transition-colors group",
        isSelected ? "bg-accent/50" : "hover:bg-muted/50",
      )}
      style={{ paddingLeft: `${row.depth * 12 + 8}px` }}
      onClick={() => onSelectFile(key)}
    >
      <Checkbox
        checked={isReviewed && !isStale}
        onCheckedChange={(checked) => onToggleReviewed(key, checked === true)}
        onClick={(e) => e.stopPropagation()}
        className="h-3.5 w-3.5"
      />
      <FileIcon fileName={row.node.name} className="h-4 w-4 shrink-0" />
      <span className="text-[13px] truncate flex-1">{row.node.name}</span>
      {isStale && <IconAlertTriangle className="h-3 w-3 text-yellow-500 shrink-0" />}
      {commentCount > 0 && (
        <span className="flex items-center gap-0.5 text-[10px] text-blue-500">
          <IconMessage className="h-3 w-3" />
          {commentCount}
        </span>
      )}
    </div>
  );
}

function countLeafFiles(node: FileTreeNode): number {
  if (!node.isDir) return 1;
  let total = 0;
  for (const child of node.children ?? []) total += countLeafFiles(child);
  return total;
}
