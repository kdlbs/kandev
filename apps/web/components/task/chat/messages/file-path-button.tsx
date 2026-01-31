'use client';

import { memo, useCallback } from 'react';
import { cn, toRelativePath } from '@/lib/utils';

type FilePathButtonProps = {
  filePath: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  variant?: 'badge' | 'list-item';
  className?: string;
};

/**
 * Check if a file path looks like a valid, openable file (not a directory or invalid path).
 * Returns false for:
 * - Empty paths or just whitespace
 * - Single dot "." (current directory)
 * - Paths ending with "/" (directories)
 * - Paths without a file extension in the last segment (likely directories)
 */
function isOpenableFilePath(path: string): boolean {
  if (!path || !path.trim()) return false;
  const trimmed = path.trim();
  if (trimmed === '.') return false;
  if (trimmed.endsWith('/')) return false;
  // Get the last segment of the path
  const lastSegment = trimmed.split('/').pop() || '';
  // If last segment has no extension and doesn't start with a dot (hidden file), it's likely a directory
  if (!lastSegment.includes('.')) return false;
  return true;
}

export const FilePathButton = memo(function FilePathButton({ filePath, worktreePath, onOpenFile, variant = 'badge', className = '' }: FilePathButtonProps) {
  const relativePath = toRelativePath(filePath, worktreePath);
  const canOpen = isOpenableFilePath(filePath);

  const handleClick = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    onOpenFile?.(filePath);
  }, [onOpenFile, filePath]);

  const baseStyles = 'font-mono truncate cursor-pointer text-indigo-400 hover:underline transition-colors block w-full text-left';
  const variantStyles = variant === 'badge'
    ? 'text-xs px-1.5 py-0.5 rounded bg-indigo-500/20 hover:bg-indigo-500/30'
    : 'flex items-center px-2 py-1 rounded hover:bg-indigo-500/20';

  if (onOpenFile && canOpen) {
    return (
      <button
        type="button"
        onClick={handleClick}
        className={cn(baseStyles, variantStyles, className)}
        title={filePath}
      >
        {relativePath}
      </button>
    );
  }

  const inactiveStyles = variant === 'badge'
    ? 'text-xs text-muted-foreground/60 truncate font-mono bg-muted/30 px-1.5 py-0.5 rounded block w-full'
    : 'px-2 py-1 font-mono text-muted-foreground truncate block w-full';

  return (
    <span className={cn(inactiveStyles, className)} title={filePath}>
      {relativePath}
    </span>
  );
});
