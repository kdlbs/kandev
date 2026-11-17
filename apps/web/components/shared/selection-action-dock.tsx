"use client";

import { useLayoutEffect, useRef, type ElementType, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { dockedBarStyle, useDockedBarPosition } from "@/hooks/use-docked-bar-position";
import { SELECTION_ACTION_DOCK_ATTRIBUTE } from "@/hooks/use-touch-selection-tracking";

export const SELECTION_ACTION_DOCK_HEIGHT_PX = 48;
/** Marks the element whose visible bottom edge a selection action dock attaches to. */
export const SELECTION_DOCK_HOST_SELECTOR = "[data-selection-dock-host]";

/** Resolves the nearest dock host above `rootRef` after every render. */
export function useSelectionDockHost(rootRef: RefObject<HTMLElement | null>) {
  const hostRef = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    hostRef.current = rootRef.current?.closest<HTMLElement>(SELECTION_DOCK_HOST_SELECTOR) ?? null;
  });
  return hostRef;
}

type SelectionActionDockProps = {
  hostRef?: RefObject<HTMLElement | null>;
  portalContainer?: HTMLElement | null;
  label: string;
  testId?: string;
  children: ReactNode;
};

/**
 * Touch presentation for text-selection actions. Native selection menus are
 * positioned beside the selection, so these actions dock as compact icon
 * buttons at the bottom-right of the hosting surface, or directly above the
 * software keyboard. The surrounding row stays transparent to taps.
 */
export function SelectionActionDock({
  hostRef,
  portalContainer,
  label,
  testId,
  children,
}: SelectionActionDockProps) {
  const position = useDockedBarPosition({
    containerRef: hostRef,
    barHeight: SELECTION_ACTION_DOCK_HEIGHT_PX,
  });
  if (typeof document === "undefined") return null;
  const portalRect = portalContainer?.getBoundingClientRect();
  const origin = portalRect ? { left: portalRect.left, top: portalRect.top } : undefined;

  return createPortal(
    <div
      className="pointer-events-none z-40 flex items-center justify-end px-2"
      style={{
        position: portalContainer ? "absolute" : "fixed",
        ...dockedBarStyle(position, origin),
        height: SELECTION_ACTION_DOCK_HEIGHT_PX,
      }}
    >
      <div
        role="toolbar"
        aria-label={label}
        data-testid={testId}
        {...{ [SELECTION_ACTION_DOCK_ATTRIBUTE]: "" }}
        className="pointer-events-auto flex items-center gap-1"
      >
        {children}
      </div>
    </div>,
    portalContainer ?? document.body,
  );
}

export function SelectionActionDockButton({
  icon: Icon,
  label,
  onActivate,
  testId,
}: {
  icon: ElementType;
  label: string;
  onActivate: () => void;
  testId?: string;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      data-testid={testId}
      className="flex min-h-11 min-w-11 shrink-0 cursor-pointer items-center justify-center rounded transition-transform duration-150 ease-out active:scale-[0.96]"
      onPointerDown={(event) => event.preventDefault()}
      onMouseDown={(event) => event.preventDefault()}
      onClick={onActivate}
    >
      <span className="inline-flex h-8 w-8 items-center justify-center rounded bg-primary text-primary-foreground shadow-lg hover:bg-primary/90">
        <Icon className="h-4 w-4" aria-hidden="true" />
      </span>
    </button>
  );
}
