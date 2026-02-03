import type { ReactNode } from 'react';
import { forwardRef } from 'react';
import { cn } from './lib/utils';

type BorderSide = 'none' | 'left' | 'right';
type MarginSide = 'none' | 'right' | 'top';

interface SessionPanelProps {
  children: ReactNode;
  borderSide?: BorderSide;
  margin?: MarginSide;
  className?: string;
}

interface SessionPanelContentProps {
  children: ReactNode;
  className?: string;
  wrapperClassName?: string;
}

export function SessionPanel({
  children,
  borderSide = 'none',
  margin = 'none',
  className,
}: SessionPanelProps) {
  return (
    <div
      className={cn(
        // Base styles - always applied
        'h-full min-h-0 bg-card flex flex-col rounded-sm border border-border/50 p-2',
        // Margins
        margin === 'right' && 'mr-[5px]',
        margin === 'top' && 'mt-[5px]',
        // Custom classes
        className
      )}
    >
      {children}
    </div>
  );
}

export const SessionPanelContent = forwardRef<HTMLDivElement, SessionPanelContentProps>(
  function SessionPanelContent({ children, className, wrapperClassName }, ref) {
    return (
      <div className={cn('flex-1 min-h-0 rounded-lg bg-background h-full shadow-inner overflow-hidden', wrapperClassName)}>
        <div
          ref={ref}
          className={cn(
            'h-full overflow-y-auto overflow-x-hidden p-2',
            className
          )}
        >
          {children}
        </div>
      </div>
    );
  }
);
