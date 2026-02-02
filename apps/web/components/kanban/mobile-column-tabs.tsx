'use client';

import { useRef, useEffect } from 'react';
import { cn } from '@/lib/utils';
import { Badge } from '@kandev/ui/badge';
import type { WorkflowStep } from '../kanban-column';

type MobileColumnTabsProps = {
  columns: WorkflowStep[];
  activeIndex: number;
  taskCounts: Record<string, number>;
  onColumnChange: (index: number) => void;
};

export function MobileColumnTabs({
  columns,
  activeIndex,
  taskCounts,
  onColumnChange,
}: MobileColumnTabsProps) {
  const tabsRef = useRef<HTMLDivElement>(null);
  const activeTabRef = useRef<HTMLButtonElement>(null);

  // Scroll active tab into view when it changes
  useEffect(() => {
    if (activeTabRef.current && tabsRef.current) {
      const tabRect = activeTabRef.current.getBoundingClientRect();
      const containerRect = tabsRef.current.getBoundingClientRect();

      const isTabVisible =
        tabRect.left >= containerRect.left && tabRect.right <= containerRect.right;

      if (!isTabVisible) {
        activeTabRef.current.scrollIntoView({
          behavior: 'smooth',
          block: 'nearest',
          inline: 'center',
        });
      }
    }
  }, [activeIndex]);

  return (
    <div
      ref={tabsRef}
      className="flex overflow-x-auto scrollbar-hide border-b border-border px-4 gap-1"
    >
      {columns.map((column, index) => (
        <button
          key={column.id}
          ref={index === activeIndex ? activeTabRef : null}
          onClick={() => onColumnChange(index)}
          className={cn(
            'flex items-center gap-2 px-3 py-2 text-sm font-medium whitespace-nowrap transition-colors',
            'border-b-2 -mb-px',
            index === activeIndex
              ? 'border-primary text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground'
          )}
        >
          <div className={cn('w-2 h-2 rounded-full flex-shrink-0', column.color)} />
          <span className="truncate max-w-[100px]">{column.title}</span>
          <Badge variant="secondary" className="text-xs h-5 px-1.5">
            {taskCounts[column.id] ?? 0}
          </Badge>
        </button>
      ))}
    </div>
  );
}
