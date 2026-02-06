'use client';

import { ReactNode, memo, useCallback } from 'react';
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { cn } from '@/lib/utils';

type ExpandableRowProps = {
  /** The icon to display (will transition to chevron on hover when expandable) */
  icon: ReactNode;
  /** Header content displayed next to the icon */
  header: ReactNode;
  /** Whether there is expandable content */
  hasExpandableContent: boolean;
  /** Current expanded state */
  isExpanded: boolean;
  /** Callback when toggled */
  onToggle: () => void;
  /** Expanded content (rendered outside clickable area) */
  children?: ReactNode;
};

export const ExpandableRow = memo(function ExpandableRow({
  icon,
  header,
  hasExpandableContent,
  isExpanded,
  onToggle,
  children,
}: ExpandableRowProps) {
  const handleClick = useCallback(() => {
    if (hasExpandableContent) onToggle();
  }, [hasExpandableContent, onToggle]);

  return (
    <div className="w-full group/expandable">
      {/* Clickable header row */}
      <div
        className={cn(
          'flex items-start gap-3 w-full rounded px-2 py-1 -mx-2 transition-colors',
          hasExpandableContent && 'hover:bg-muted/50 cursor-pointer'
        )}
        onClick={handleClick}
      >
        {/* Icon with hover-to-show chevron */}
        <div className="flex-shrink-0 mt-0.5 relative w-4 h-4">
          <div
            className={cn(
              'absolute inset-0 transition-opacity',
              hasExpandableContent && 'group-hover/expandable:opacity-0'
            )}
          >
            {icon}
          </div>
          {hasExpandableContent &&
            (isExpanded ? (
              <IconChevronDown className="h-4 w-4 text-muted-foreground absolute inset-0 opacity-0 group-hover/expandable:opacity-100 transition-opacity" />
            ) : (
              <IconChevronRight className="h-4 w-4 text-muted-foreground absolute inset-0 opacity-0 group-hover/expandable:opacity-100 transition-opacity" />
            ))}
        </div>

        {/* Header content */}
        <div className="flex-1 min-w-0 pt-0.5">{header}</div>
      </div>

      {/* Expanded content - outside clickable area */}
      {isExpanded && hasExpandableContent && children && (
        <div className="mt-2 ml-7">{children}</div>
      )}
    </div>
  );
});
