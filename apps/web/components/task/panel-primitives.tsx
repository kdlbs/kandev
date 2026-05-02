import { forwardRef, type ReactNode, type HTMLAttributes } from "react";
import { cn } from "@kandev/ui/lib/utils";

/**
 * Reusable dockview panel layout primitives.
 *
 * PanelRoot - outermost wrapper, fills the dockview content area
 * PanelBody - scrollable (or non-scrollable) content region
 * PanelToolbar - fixed header strip for panel actions
 */

const PANEL_ROOT_CLASS = "h-full min-h-0 flex flex-col bg-card text-card-foreground";
const PANEL_BAR_CLASS =
  "flex items-center gap-1.5 h-[30px] px-2.5 shrink-0 border-border/80 bg-card/95 text-xs text-foreground";
const PANEL_ACTION_CURSOR_CLASS =
  "[&_button:not(:disabled)]:cursor-pointer [&_[role=button]:not([aria-disabled=true])]:cursor-pointer";

type PanelRootProps = HTMLAttributes<HTMLDivElement> & {
  children: ReactNode;
  className?: string;
};

/** Fills the dockview content slot. Use as the outermost element in every panel. */
export const PanelRoot = forwardRef<HTMLDivElement, PanelRootProps>(function PanelRoot(
  { children, className, ...rest },
  ref,
) {
  return (
    <div ref={ref} className={cn(PANEL_ROOT_CLASS, className)} {...rest}>
      {children}
    </div>
  );
});

type PanelBodyProps = Omit<HTMLAttributes<HTMLDivElement>, "className"> & {
  children: ReactNode;
  className?: string;
  /** Add default p-2.5 padding. Default true. */
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
        "flex-1 min-h-0 bg-card text-card-foreground",
        scroll && "overflow-auto",
        padding && "p-2.5",
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

type PanelBarProps = {
  children?: ReactNode;
  className?: string;
  borderClassName: "border-b" | "border-t";
};

function PanelBar({ children, className, borderClassName }: PanelBarProps) {
  return (
    <div className={cn(PANEL_BAR_CLASS, PANEL_ACTION_CURSOR_CLASS, borderClassName, className)}>
      {children}
    </div>
  );
}

/** Fixed header toolbar strip. Doesn't scroll with content. */
export function PanelToolbar({ children, className }: PanelToolbarProps) {
  return <PanelHeaderBar className={className}>{children}</PanelHeaderBar>;
}

type PanelHeaderBarProps = {
  children?: ReactNode;
  className?: string;
};

/** Fixed-height panel header bar. Renders children directly. */
export function PanelHeaderBar({ children, className }: PanelHeaderBarProps) {
  return (
    <PanelBar borderClassName="border-b" className={className}>
      {children}
    </PanelBar>
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
      <div className="flex items-center gap-1.5 min-w-0 overflow-hidden">{left}</div>
      <div className="flex-1" />
      <div className="flex items-center gap-1.5 min-w-0 overflow-hidden">{right}</div>
    </PanelHeaderBar>
  );
}

/** Fixed-height panel footer bar with border-t. Mirrors PanelHeaderBar but anchors to the bottom. */
export function PanelFooterBar({ children, className }: PanelHeaderBarProps) {
  return (
    <PanelBar borderClassName="border-t" className={className}>
      {children}
    </PanelBar>
  );
}
