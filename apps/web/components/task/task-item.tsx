'use client';

import { memo, useState } from 'react';
import { IconLoader2 } from '@tabler/icons-react';
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
  stepName?: string;
  state?: TaskState;
  isSelected?: boolean;
  onClick?: () => void;
  diffStats?: DiffStats;
  updatedAt?: string;
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
  stepName,
  state,
  isSelected = false,
  onClick,
  diffStats,
  updatedAt,
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

  // Determine what to show on the right side (below step name)
  const renderRightMeta = () => {
    if (isInProgress) {
      return <IconLoader2 className="h-3.5 w-3.5 text-blue-500 animate-spin" />;
    }
    if (hasDiffStats) {
      return (
        <span className="text-[11px] font-mono text-muted-foreground">
          <span className="text-emerald-500">+{diffStats.additions}</span>
          {' '}
          <span className="text-rose-500">-{diffStats.deletions}</span>
        </span>
      );
    }
    if (updatedAt) {
      return (
        <span className="text-[11px] text-muted-foreground/60">
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
        'group relative flex w-full items-center gap-2 px-3 py-2 text-left text-sm outline-none cursor-pointer',
        'transition-colors duration-75',
        'hover:bg-foreground/[0.05]',
        isSelected && 'bg-primary/10'
      )}
    >
      {/* Selection indicator */}
      <div
        className={cn(
          'absolute left-0 top-0 bottom-0 w-[2px] transition-opacity',
          isSelected ? 'bg-primary opacity-100' : 'opacity-0'
        )}
      />

      {/* Content */}
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="line-clamp-1 min-w-0 text-[13px] font-medium text-foreground">
          {title}
        </span>
        {description && (
          <span className="text-[11px] text-muted-foreground/60 truncate">
            {description}
          </span>
        )}
      </div>

      {/* Right side: step name + meta, or action buttons on hover */}
      <div className="relative flex items-center shrink-0">
        <div
          className={cn(
            'flex flex-col items-end gap-0.5 transition-opacity duration-100',
            effectiveMenuOpen ? 'opacity-0' : 'group-hover:opacity-0'
          )}
        >
          {stepName && (
            <span className="text-[10px] text-muted-foreground/70 bg-foreground/[0.06] px-1.5 py-px rounded-[6px]">
              {stepName}
            </span>
          )}
          {renderRightMeta()}
        </div>

        {/* Action buttons - hidden by default, visible on hover */}
        <div
          className={cn(
            'absolute right-0 flex items-center gap-0.5',
            'transition-opacity duration-100',
            effectiveMenuOpen ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'
          )}
        >
          <TaskItemMenu
            open={effectiveMenuOpen}
            onOpenChange={(open) => {
              if (!open && isDeleting) return;
              setMenuOpen(open);
            }}
            onRename={onRename}
            onDuplicate={onDuplicate}
            onReview={onReview}
            onDelete={onDelete}
            isDeleting={isDeleting}
          />
        </div>
      </div>
    </div>
  );
});
