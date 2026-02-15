import type { ReactNode } from 'react';
import { forwardRef, useRef, useEffect, useCallback } from 'react';
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
        'session-panel-wrapper h-full min-h-0 bg-card flex flex-col rounded-sm border border-border/50 p-2',
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
    const internalRef = useRef<HTMLDivElement>(null);
    /**
     * Continuously updated by the scroll listener so we always have
     * the latest good scrollTop before the browser resets it on display:none.
     */
    const savedScrollTopRef = useRef(0);
    const isHiddenRef = useRef(false);

    // Merge forwarded ref with internal ref
    const setRefs = useCallback(
      (node: HTMLDivElement | null) => {
        internalRef.current = node;
        if (typeof ref === 'function') ref(node);
        else if (ref) ref.current = node;
      },
      [ref]
    );

    // Save scrollTop on every scroll event so it's always up-to-date
    useEffect(() => {
      const el = internalRef.current;
      if (!el) return;

      const onScroll = () => {
        if (!isHiddenRef.current) {
          savedScrollTopRef.current = el.scrollTop;
        }
      };

      el.addEventListener('scroll', onScroll);
      return () => el.removeEventListener('scroll', onScroll);
    }, []);

    // Detect hide/show via ResizeObserver and restore scroll position
    useEffect(() => {
      const el = internalRef.current;
      if (!el) return;

      const observer = new ResizeObserver((entries) => {
        for (const entry of entries) {
          const { width, height } = entry.contentRect;

          if (width === 0 || height === 0) {
            isHiddenRef.current = true;
            return;
          }

          if (isHiddenRef.current) {
            isHiddenRef.current = false;
            const saved = savedScrollTopRef.current;
            requestAnimationFrame(() => {
              el.scrollTop = saved;
            });
          }
        }
      });

      observer.observe(el);
      return () => observer.disconnect();
    }, []);

    return (
      <div className={cn('session-panel-content-wrapper flex-1 min-h-0 rounded-lg h-full shadow-inner overflow-hidden', wrapperClassName)}>
        <div
          ref={setRefs}
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
