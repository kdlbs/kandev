import {
  forwardRef,
  useCallback,
  useEffect,
  useRef,
  useState,
  type ForwardedRef,
  type RefObject,
  type ReactNode,
  type HTMLAttributes,
} from "react";
import { IconDots } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
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
  "box-border flex min-w-0 items-center gap-1.5 px-2.5 shrink-0 border-border/80 bg-card/95 text-xs text-foreground";
const PANEL_HEADER_BAR_CLASS =
  "h-[1.875rem] min-h-[1.875rem] [@media(max-width:47.999rem)]:h-12 [@media(max-width:47.999rem)]:min-h-12 [@media(pointer:coarse)]:h-12 [@media(pointer:coarse)]:min-h-12 [@media(max-width:47.999rem)]:[&_button]:min-h-11 [@media(max-width:47.999rem)]:[&_button]:min-w-11 [@media(max-width:47.999rem)]:[&_a]:min-h-11 [@media(max-width:47.999rem)]:[&_a]:min-w-11 [@media(max-width:47.999rem)]:[&_[role=button]]:min-h-11 [@media(max-width:47.999rem)]:[&_[role=button]]:min-w-11 [@media(max-width:47.999rem)]:[&_input]:min-h-11 [@media(max-width:47.999rem)]:[&_select]:min-h-11 [@media(max-width:47.999rem)]:[&_textarea]:min-h-11 [@media(pointer:coarse)]:[&_button]:min-h-11 [@media(pointer:coarse)]:[&_button]:min-w-11 [@media(pointer:coarse)]:[&_a]:min-h-11 [@media(pointer:coarse)]:[&_a]:min-w-11 [@media(pointer:coarse)]:[&_[role=button]]:min-h-11 [@media(pointer:coarse)]:[&_[role=button]]:min-w-11 [@media(pointer:coarse)]:[&_input]:min-h-11 [@media(pointer:coarse)]:[&_select]:min-h-11 [@media(pointer:coarse)]:[&_textarea]:min-h-11";
const PANEL_ACTION_CURSOR_CLASS =
  "[&_button:not(:disabled)]:cursor-pointer [&_[role=button]:not([aria-disabled=true])]:cursor-pointer";

export function shouldUsePanelHeaderOverflow(panelWidth: number, overflowAt: number): boolean {
  return panelWidth > 0 && panelWidth < overflowAt;
}

function usePanelHeaderOverflow(
  ref: RefObject<HTMLDivElement | null>,
  overflowAt: number | undefined,
): boolean {
  const [overflowed, setOverflowed] = useState(false);

  useEffect(() => {
    if (!overflowAt || typeof ResizeObserver === "undefined") {
      setOverflowed(false);
      return;
    }
    const element = ref.current;
    if (!element) return;
    const measure = () => {
      const next = shouldUsePanelHeaderOverflow(element.getBoundingClientRect().width, overflowAt);
      setOverflowed((current) => (current === next ? current : next));
    };
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    measure();
    return () => observer.disconnect();
  }, [overflowAt, ref]);

  return overflowed;
}

function assignRef<T>(ref: ForwardedRef<T>, value: T | null) {
  if (typeof ref === "function") {
    ref(value);
  } else if (ref) {
    ref.current = value;
  }
}

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

type PanelToolbarProps = HTMLAttributes<HTMLDivElement> & { children: ReactNode };

type PanelBarProps = HTMLAttributes<HTMLDivElement> & {
  borderClassName: "border-b" | "border-t";
};

const PanelBar = forwardRef<HTMLDivElement, PanelBarProps>(function PanelBar(
  { children, className, borderClassName, ...rest },
  ref,
) {
  return (
    <div
      ref={ref}
      data-panel-header={borderClassName === "border-b" ? "true" : undefined}
      className={cn(PANEL_BAR_CLASS, PANEL_ACTION_CURSOR_CLASS, borderClassName, className)}
      {...rest}
    >
      {children}
    </div>
  );
});

/** Fixed header toolbar strip. Doesn't scroll with content. */
export const PanelToolbar = forwardRef<HTMLDivElement, PanelToolbarProps>(function PanelToolbar(
  { children, className, ...rest },
  ref,
) {
  return (
    <PanelHeaderBar ref={ref} className={className} {...rest}>
      {children}
    </PanelHeaderBar>
  );
});

type PanelHeaderBarProps = HTMLAttributes<HTMLDivElement> & { children?: ReactNode };

/** Fixed-height panel header bar. Renders children directly. */
export const PanelHeaderBar = forwardRef<HTMLDivElement, PanelHeaderBarProps>(
  function PanelHeaderBar({ children, className, ...rest }, ref) {
    return (
      <PanelBar
        ref={ref}
        borderClassName="border-b"
        className={cn(PANEL_HEADER_BAR_CLASS, className)}
        {...rest}
      >
        {children}
      </PanelBar>
    );
  },
);

type PanelHeaderBarSplitProps = HTMLAttributes<HTMLDivElement> & {
  left?: ReactNode;
  right?: ReactNode;
  /** Actions shown when the panel is too narrow for the full action cluster. */
  overflow?: ReactNode;
  /** Panel width in CSS pixels at which `overflow` replaces `right`. */
  overflowAt?: number;
  /** Hide the left slot when the overflow action menu is active. */
  hideLeftWhenOverflow?: boolean;
  /** Keep the right slot visible alongside the overflow menu. */
  hideRightWhenOverflow?: boolean;
  /** Primary action kept visible alongside the overflow menu. */
  rightWhenOverflow?: ReactNode;
  /** Left content replacement that keeps the primary left action visible when overflowed. */
  leftWhenOverflow?: ReactNode;
  leftClassName?: string;
  rightClassName?: string;
};

function shouldRenderPanelHeaderSlot(
  isOverflowed: boolean,
  hideWhenOverflow: boolean,
  replacement: ReactNode,
): boolean {
  if (!isOverflowed) return true;
  return !hideWhenOverflow || replacement !== undefined;
}

function PanelHeaderSplitSlot({
  visible,
  isOverflowed,
  replacement,
  children,
  className,
  shrinkWhenReplaced = false,
}: {
  visible: boolean;
  isOverflowed: boolean;
  replacement?: ReactNode;
  children?: ReactNode;
  className?: string;
  shrinkWhenReplaced?: boolean;
}) {
  if (!visible) return null;
  const content = isOverflowed && replacement !== undefined ? replacement : children;
  return (
    <div
      className={cn(
        "flex min-w-0 max-w-full items-center gap-1.5 overflow-hidden",
        shrinkWhenReplaced && isOverflowed && replacement !== undefined && "shrink-0",
        className,
      )}
    >
      {content}
    </div>
  );
}

/** Panel header bar with left/right slots separated by a spacer. */
export const PanelHeaderBarSplit = forwardRef<HTMLDivElement, PanelHeaderBarSplitProps>(
  function PanelHeaderBarSplit(
    {
      left,
      right,
      overflow,
      overflowAt,
      hideLeftWhenOverflow = false,
      hideRightWhenOverflow = true,
      rightWhenOverflow,
      leftWhenOverflow,
      leftClassName,
      rightClassName,
      className,
      ...rest
    },
    ref,
  ) {
    const headerRef = useRef<HTMLDivElement>(null);
    const isOverflowed = usePanelHeaderOverflow(headerRef, overflow ? overflowAt : undefined);
    const setHeaderRef = useCallback(
      (node: HTMLDivElement | null) => {
        headerRef.current = node;
        assignRef(ref, node);
      },
      [ref],
    );
    const showLeft = shouldRenderPanelHeaderSlot(
      isOverflowed,
      hideLeftWhenOverflow,
      leftWhenOverflow,
    );
    const showRight = shouldRenderPanelHeaderSlot(isOverflowed, hideRightWhenOverflow, undefined);
    return (
      <PanelHeaderBar
        ref={setHeaderRef}
        className={className}
        data-panel-overflow={isOverflowed ? "true" : undefined}
        {...rest}
      >
        <PanelHeaderSplitSlot
          visible={showLeft}
          isOverflowed={isOverflowed}
          replacement={leftWhenOverflow}
          className={leftClassName}
          shrinkWhenReplaced
        >
          {left}
        </PanelHeaderSplitSlot>
        <div className="min-w-0 flex-1" />
        <PanelHeaderSplitSlot
          visible={showRight}
          isOverflowed={isOverflowed}
          className={cn("shrink-0", rightClassName)}
        >
          {right}
        </PanelHeaderSplitSlot>
        {isOverflowed && rightWhenOverflow ? (
          <div className="flex shrink-0 items-center gap-1.5">{rightWhenOverflow}</div>
        ) : null}
        {isOverflowed && overflow}
      </PanelHeaderBar>
    );
  },
);

export function PanelHeaderOverflowMenu({
  label,
  children,
  testId = "panel-header-overflow",
}: {
  label: string;
  children: ReactNode;
  testId?: string;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={controlSizingClassName(
            "icon",
            "shrink-0 cursor-pointer text-muted-foreground hover:text-foreground",
          )}
          aria-label={label}
          data-testid={testId}
        >
          <IconDots className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-[min(70dvh,28rem)] overflow-y-auto">
        {children}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Fixed-height panel footer bar with border-t. Mirrors PanelHeaderBar but anchors to the bottom. */
export const PanelFooterBar = forwardRef<HTMLDivElement, PanelHeaderBarProps>(
  function PanelFooterBar({ children, className, ...rest }, ref) {
    return (
      <PanelBar ref={ref} borderClassName="border-t" className={className} {...rest}>
        {children}
      </PanelBar>
    );
  },
);
