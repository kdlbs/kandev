'use client';

import { type ReactNode } from 'react';
import { IconChevronRight } from '@tabler/icons-react';
import { cn } from '@/lib/utils';

type SidebarButtonProps = {
  icon?: ReactNode;
  label: string;
  count?: number;
  isExpanded?: boolean;
  onToggle?: () => void;
  compact?: boolean;
};

export function SidebarButton({
  icon,
  label,
  count,
  isExpanded,
  onToggle,
  compact = false,
}: SidebarButtonProps) {
  const isExpandable = onToggle !== undefined;

  return (
    <button
      type="button"
      onClick={onToggle}
      className={cn(
        'group flex w-full items-center gap-2 rounded-[6px] text-[13px] select-none outline-none',
        'focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-ring',
        compact ? 'py-[3px]' : 'py-[5px]',
        'px-2',
        'hover:bg-foreground/[0.07]',
        isExpandable && 'cursor-pointer'
      )}
      aria-expanded={isExpandable ? isExpanded : undefined}
    >
      {icon && (
        <span className="flex h-3.5 w-3.5 shrink-0 items-center justify-center">
          {icon}
        </span>
      )}
      <span className="flex-1 truncate text-left font-medium text-foreground/80">
        {label}
      </span>
      {count !== undefined && (
        <span className="text-xs text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity">
          {count}
        </span>
      )}
      {isExpandable && (
        <IconChevronRight
          className={cn(
            'h-3.5 w-3.5 shrink-0 text-muted-foreground opacity-0 group-hover:opacity-100 transition-all duration-150',
            isExpanded && 'rotate-90'
          )}
        />
      )}
    </button>
  );
}
