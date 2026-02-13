'use client';

import { memo, useState } from 'react';
import { IconArchive, IconLoader2 } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import type { TaskState } from '@/lib/types/http';
import { TaskItemMenu } from './task-item-menu';

type DiffStats = {
  additions: number;
  deletions: number;
};

type TaskItemProps = {
  title: string;
  description?: string;
  state?: TaskState;
  isSelected?: boolean;
  onClick?: () => void;
  diffStats?: DiffStats;
  updatedAt?: string;
  onArchive?: () => void;
  onRename?: () => void;
  onDuplicate?: () => void;
  onReview?: () => void;
  onDelete?: () => void;
  isDeleting?: boolean;
};

// Helper to format relative time
function formatRelativeTime(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSecs = Math.floor(diffMs / 1000);
  const diffMins = Math.floor(diffSecs / 60);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffSecs < 60) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

export const TaskItem = memo(function TaskItem({
  title,
  description,
  state,
  isSelected = false,
  onClick,
  diffStats,
  updatedAt,
  onArchive,
  onRename,
  onDuplicate,
  onReview,
  onDelete,
  isDeleting,
}: TaskItemProps) {
  const [menuOpen, setMenuOpen] = useState(false);

  // Effective open state: keep menu open while deleting
  const effectiveMenuOpen = menuOpen || isDeleting;

  const isInProgress = state === 'IN_PROGRESS' || state === 'SCHEDULING';
  const hasDiffStats = diffStats && (diffStats.additions > 0 || diffStats.deletions > 0);

  // Determine what to show on the right side
  const renderRightContent = () => {
    if (isInProgress) {
      return (
        <IconLoader2 className="h-4 w-4 text-blue-500 animate-spin" />
      );
    }
    if (hasDiffStats) {
      return (
        <div className="flex items-center gap-1 text-xs font-mono rounded-md border border-border/50 px-1.5 py-0.5">
          <span className="text-emerald-500">+{diffStats.additions}</span>
          <span className="text-rose-500">-{diffStats.deletions}</span>
        </div>
      );
    }
    if (updatedAt) {
      return (
        <span className="text-xs text-muted-foreground">
          {formatRelativeTime(updatedAt)}
        </span>
      );
    }
    return null;
  };

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onClick?.();
        }
      }}
      className={cn(
        'group relative flex w-full items-start gap-2 rounded-md border p-2 text-left text-sm outline-none cursor-pointer',
        'transition-all duration-100',
        isSelected
          ? 'bg-primary/10 border-primary/30'
          : 'bg-background/50 border-border hover:bg-background/80 hover:border-border'
      )}
    >
      {/* Content */}
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="line-clamp-2 min-w-0 font-medium font-sans text-foreground">
          {title}
        </span>
        {description && (
          <span className="flex items-center gap-1.5 text-xs text-foreground/60 truncate">
            {description}
          </span>
        )}
      </div>

      {/* Right side: Spinner / Diff stats / Time (default) or Action buttons (on hover) */}
      <div className="relative flex items-center shrink-0 self-stretch">
        {/* Default content - visible by default, hidden on hover */}
        <div
          className={cn(
            'transition-opacity duration-150',
            !effectiveMenuOpen && 'group-hover:opacity-0'
          )}
        >
          {renderRightContent()}
        </div>

        {/* Action buttons - hidden by default, visible on hover */}
        <div
          className={cn(
            'absolute right-0 top-0 bottom-0 flex flex-col justify-center gap-0.5',
            'transition-opacity duration-150',
            effectiveMenuOpen ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'
          )}
        >
          {/* 3-dot menu button */}
          <TaskItemMenu
            open={effectiveMenuOpen}
            onOpenChange={(open) => {
              // Prevent closing while deleting
              if (!open && isDeleting) return;
              setMenuOpen(open);
            }}
            onRename={onRename}
            onDuplicate={onDuplicate}
            onReview={onReview}
            onDelete={onDelete}
            isDeleting={isDeleting}
          />

          {/* Archive button */}
          <button
            type="button"
            className={cn(
              'flex h-6 w-6 items-center justify-center rounded-md cursor-pointer',
              'text-muted-foreground hover:text-foreground hover:bg-foreground/10',
              'transition-colors'
            )}
            onClick={(e) => {
              e.stopPropagation();
              onArchive?.();
            }}
            aria-label="Archive task"
          >
            <IconArchive className="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
  );
});
