'use client';

import { memo, useState, useCallback, useMemo } from 'react';
import {
  IconChevronDown,
  IconChevronRight,
  IconMessage,
  IconAlertTriangle,
} from '@tabler/icons-react';
import { Checkbox } from '@kandev/ui/checkbox';
import { cn } from '@kandev/ui/lib/utils';
import { FileIcon } from '@/components/ui/file-icon';
import { FileStatusIcon } from '@/components/task/file-status-icon';
import type { ReviewFile, FileTreeNode } from './types';
import { buildFileTree } from './types';

type ReviewFileTreeProps = {
  files: ReviewFile[];
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
  commentCountByFile: Record<string, number>;
  selectedFile: string | null;
  onSelectFile: (path: string) => void;
  onToggleReviewed: (path: string, reviewed: boolean) => void;
};

export const ReviewFileTree = memo(function ReviewFileTree({
  files,
  reviewedFiles,
  staleFiles,
  commentCountByFile,
  selectedFile,
  onSelectFile,
  onToggleReviewed,
}: ReviewFileTreeProps) {
  const tree = useMemo(() => buildFileTree(files), [files]);

  return (
    <div className="flex flex-col h-full overflow-y-auto text-sm">
      <div className="px-3 py-2 text-xs font-medium text-muted-foreground uppercase tracking-wide border-b border-border">
        Files
      </div>
      <div className="py-1">
        {tree.map((node) => (
          <TreeNode
            key={node.path}
            node={node}
            reviewedFiles={reviewedFiles}
            staleFiles={staleFiles}
            commentCountByFile={commentCountByFile}
            selectedFile={selectedFile}
            onSelectFile={onSelectFile}
            onToggleReviewed={onToggleReviewed}
            depth={0}
          />
        ))}
      </div>
    </div>
  );
});

type TreeNodeProps = {
  node: FileTreeNode;
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
  commentCountByFile: Record<string, number>;
  selectedFile: string | null;
  onSelectFile: (path: string) => void;
  onToggleReviewed: (path: string, reviewed: boolean) => void;
  depth: number;
};

function FileNode({ node, reviewedFiles, staleFiles, commentCountByFile, selectedFile, onSelectFile, onToggleReviewed, depth }: Omit<TreeNodeProps, 'node'> & { node: FileTreeNode }) {
  const file = node.file!;
  const isReviewed = reviewedFiles.has(file.path);
  const isStale = staleFiles.has(file.path);
  const commentCount = commentCountByFile[file.path] ?? 0;
  const isSelected = selectedFile === file.path;
  return (
    <div className={cn('flex items-center gap-1.5 px-2 py-1 cursor-pointer transition-colors group', isSelected ? 'bg-accent/50' : 'hover:bg-muted/50')} style={{ paddingLeft: `${depth * 12 + 8}px` }} onClick={() => onSelectFile(file.path)}>
      <Checkbox checked={isReviewed && !isStale} onCheckedChange={(checked) => { onToggleReviewed(file.path, checked === true); }} onClick={(e) => e.stopPropagation()} className="h-3.5 w-3.5" />
      <FileIcon fileName={node.name} className="h-4 w-4 shrink-0" />
      <span className="text-xs truncate flex-1">{node.name}</span>
      {isStale && <IconAlertTriangle className="h-3 w-3 text-yellow-500 shrink-0" />}
      {commentCount > 0 && <span className="flex items-center gap-0.5 text-[10px] text-blue-500"><IconMessage className="h-3 w-3" />{commentCount}</span>}
      <FileStatusIcon status={file.status as 'modified' | 'added' | 'deleted' | 'untracked' | 'renamed'} />
      <span className="text-[10px] text-muted-foreground/60 whitespace-nowrap">
        {file.additions > 0 && <span className="text-emerald-500">+{file.additions}</span>}
        {file.additions > 0 && file.deletions > 0 && ' '}
        {file.deletions > 0 && <span className="text-rose-500">-{file.deletions}</span>}
      </span>
    </div>
  );
}

function TreeNode({ node, reviewedFiles, staleFiles, commentCountByFile, selectedFile, onSelectFile, onToggleReviewed, depth }: TreeNodeProps) {
  const [expanded, setExpanded] = useState(true);
  const handleToggle = useCallback(() => { setExpanded((prev) => !prev); }, []);
  const sharedProps = { reviewedFiles, staleFiles, commentCountByFile, selectedFile, onSelectFile, onToggleReviewed };

  if (node.isDir) {
    const dirFiles = collectFiles(node);
    const dirReviewed = dirFiles.filter((f) => reviewedFiles.has(f.path) && !staleFiles.has(f.path)).length;
    return (
      <div>
        <button type="button" className="flex items-center w-full gap-1 px-2 py-1 hover:bg-muted/50 transition-colors cursor-pointer" style={{ paddingLeft: `${depth * 12 + 8}px` }} onClick={handleToggle}>
          {expanded ? <IconChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" /> : <IconChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
          <span className="text-xs text-muted-foreground truncate">{node.name}</span>
          <span className="ml-auto text-[10px] text-muted-foreground/60">{dirReviewed}/{dirFiles.length}</span>
        </button>
        {expanded && node.children && (
          <div>{node.children.map((child) => (<TreeNode key={child.path} node={child} {...sharedProps} depth={depth + 1} />))}</div>
        )}
      </div>
    );
  }

  return <FileNode node={node} {...sharedProps} depth={depth} />;
}

/** Collect all leaf files from a tree node recursively */
function collectFiles(node: FileTreeNode): ReviewFile[] {
  if (!node.isDir) return node.file ? [node.file] : [];
  const result: ReviewFile[] = [];
  for (const child of node.children ?? []) {
    result.push(...collectFiles(child));
  }
  return result;
}
