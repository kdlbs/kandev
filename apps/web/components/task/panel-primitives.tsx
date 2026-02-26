import { forwardRef, type ReactNode, type HTMLAttributes } from "react";
import { cn } from "@kandev/ui/lib/utils";

/**
 * Reusable dockview panel layout primitives.
 *
 * PanelRoot  – outermost wrapper, fills the dockview content area
 * PanelBody  – scrollable (or non-scrollable) content region
 * PanelToolbar – fixed strip at the top/bottom of a panel
 */

type PanelRootProps = HTMLAttributes<HTMLDivElement> & {
  children: ReactNode;
  className?: string;
};

/** Fills the dockview content slot. Use as the outermost element in every panel. */
export function PanelRoot({ children, className, ...rest }: PanelRootProps) {
  return (
    <div className={cn("h-full flex flex-col min-h-0", className)} {...rest}>
      {children}
    </div>
  );
}

type PanelBodyProps = Omit<HTMLAttributes<HTMLDivElement>, "className"> & {
  children: ReactNode;
  className?: string;
  /** Add default p-3 padding. Default true. */
  padding?: boolean;
  /** Enable overflow scrolling. Default true. */
  scroll?: boolean;
};

/** Flexible content area that grows to fill remaining space. */
export const PanelBody = forwardRef<HTMLDivElement, PanelBodyProps>(function PanelBody(
  { children, className, padding = true, scroll = true, ...rest },
  ref,
) {
  return (
    <div
      ref={ref}
      className={cn(
        "flex-1 min-h-0 bg-card",
        scroll && "overflow-auto",
        padding && "p-3",
        className,
      )}
      {...rest}
    >
      {children}
    </div>
  );
});

type PanelToolbarProps = {
  children: ReactNode;
  className?: string;
};

/** Fixed toolbar strip. Doesn't scroll with content. */
export function PanelToolbar({ children, className }: PanelToolbarProps) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 px-3 py-1.5 border-b border-border shrink-0",
        className,
      )}
    >
      {children}
    </div>
  );
}

type PanelHeaderBarProps = {
  children?: ReactNode;
  className?: string;
};

/** Fixed-height panel header bar. Renders children directly. */
export function PanelHeaderBar({ children, className }: PanelHeaderBarProps) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 px-3 h-[30px] border-b border-border shrink-0 bg-card",
        className,
      )}
    >
      {children}
    </div>
  );
}

type PanelHeaderBarSplitProps = {
  left?: ReactNode;
  right?: ReactNode;
  className?: string;
};

/** Panel header bar with left/right slots separated by a spacer. */
export function PanelHeaderBarSplit({ left, right, className }: PanelHeaderBarSplitProps) {
  return (
    <PanelHeaderBar className={className}>
      <div className="flex items-center gap-1.5 min-w-0">{left}</div>
      <div className="flex-1" />
      <div className="flex items-center gap-1.5 min-w-0">{right}</div>
    </PanelHeaderBar>
  );
}

/** Fixed-height panel footer bar with border-t. Mirrors PanelHeaderBar but anchors to the bottom. */
export function PanelFooterBar({ children, className }: PanelHeaderBarProps) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 px-3 h-[30px] border-t border-border shrink-0 bg-card",
        className,
      )}
    >
      {children}
    </div>
  );
}
