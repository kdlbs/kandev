"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { IconMessagePlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useVisualViewportOffset } from "@/hooks/use-visual-viewport-offset";
import {
  resolveSelectionCommentDockPosition,
  type SelectionCommentDockPosition,
  type SelectionCommentRect,
} from "./selection-comment-dock-position";

const DOCK_HEIGHT = 56;
const DOCK_WIDTH = 160;

type SelectionCommentHost = {
  ownerRef: RefObject<HTMLElement | null>;
  boundaryRef?: RefObject<HTMLElement | null>;
  reserveRef?: RefObject<HTMLElement | null>;
  portalContainer?: HTMLElement | null;
  setReservation: (id: string, height: number | null) => void;
};

const SelectionCommentHostContext = createContext<SelectionCommentHost | null>(null);
const SelectionCommentDockReservationContext = createContext(0);

function rectFromElement(element: Element | null | undefined): SelectionCommentRect | null {
  if (!element?.isConnected) return null;
  const rect = element.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return null;
  return {
    left: rect.left,
    top: rect.top,
    right: rect.right,
    bottom: rect.bottom,
    width: rect.width,
    height: rect.height,
  };
}

function readSafeAreaInsets(): { top: number; right: number; bottom: number; left: number } {
  const probe = document.createElement("div");
  probe.style.cssText = [
    "position:fixed",
    "visibility:hidden",
    "pointer-events:none",
    "padding-top:env(safe-area-inset-top, 0px)",
    "padding-right:env(safe-area-inset-right, 0px)",
    "padding-bottom:env(safe-area-inset-bottom, 0px)",
    "padding-left:env(safe-area-inset-left, 0px)",
  ].join(";");
  document.body.append(probe);
  const style = window.getComputedStyle(probe);
  const insets = {
    top: Number.parseFloat(style.paddingTop) || 0,
    right: Number.parseFloat(style.paddingRight) || 0,
    bottom: Number.parseFloat(style.paddingBottom) || 0,
    left: Number.parseFloat(style.paddingLeft) || 0,
  };
  probe.remove();
  return insets;
}

function restoreReservation(
  target: HTMLElement,
  baseInline: string,
  baseComputed: string,
  height: number,
) {
  if (height > 0) target.style.paddingBottom = `calc(${baseComputed} + ${height}px)`;
  else target.style.paddingBottom = baseInline;
}

export function SelectionCommentHostProvider({
  ownerRef,
  boundaryRef,
  reserveRef,
  portalContainer,
  children,
}: {
  ownerRef: RefObject<HTMLElement | null>;
  boundaryRef?: RefObject<HTMLElement | null>;
  reserveRef?: RefObject<HTMLElement | null>;
  portalContainer?: HTMLElement | null;
  children: ReactNode;
}) {
  const reservationsRef = useRef(new Map<string, number>());
  const [reservationHeight, setReservationHeight] = useState(0);
  const targetBaseRef = useRef<{ target: HTMLElement; inline: string; computed: string } | null>(
    null,
  );

  const setReservation = useCallback(
    (id: string, height: number | null) => {
      const reservations = reservationsRef.current;
      if (height === null || height <= 0) reservations.delete(id);
      else reservations.set(id, height);

      const occupiedHeight = Math.max(0, ...reservations.values());
      setReservationHeight(occupiedHeight);
      const target = reserveRef?.current;
      if (!target) return;
      if (occupiedHeight > 0) {
        if (targetBaseRef.current?.target !== target) {
          targetBaseRef.current = {
            target,
            inline: target.style.paddingBottom,
            computed: window.getComputedStyle(target).paddingBottom || "0px",
          };
        }
        const base = targetBaseRef.current;
        restoreReservation(target, base.inline, base.computed, occupiedHeight);
      } else if (targetBaseRef.current?.target === target) {
        target.style.paddingBottom = targetBaseRef.current.inline;
        targetBaseRef.current = null;
      }
    },
    [reserveRef],
  );

  useEffect(
    () => () => {
      const base = targetBaseRef.current;
      if (base) base.target.style.paddingBottom = base.inline;
      targetBaseRef.current = null;
      reservationsRef.current.clear();
    },
    [],
  );

  const value = useMemo(
    () => ({ ownerRef, boundaryRef, reserveRef, portalContainer, setReservation }),
    [boundaryRef, ownerRef, portalContainer, reserveRef, setReservation],
  );

  return (
    <SelectionCommentHostContext.Provider value={value}>
      <SelectionCommentDockReservationContext.Provider value={reservationHeight}>
        {children}
      </SelectionCommentDockReservationContext.Provider>
    </SelectionCommentHostContext.Provider>
  );
}

export function useSelectionCommentDockReservation() {
  return useContext(SelectionCommentDockReservationContext);
}

export function useSelectionCommentDockPresentation() {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  return isMobile || !isFinePointer;
}

type DockViewportOffset = {
  viewportLeft: number;
  viewportTop: number;
  viewportWidth: number;
  viewportHeight: number;
};

function getVisibleViewportRect({
  viewportLeft,
  viewportTop,
  viewportWidth,
  viewportHeight,
}: DockViewportOffset): SelectionCommentRect {
  if (window.visualViewport && viewportWidth > 0 && viewportHeight > 0) {
    return {
      left: viewportLeft,
      top: viewportTop,
      right: viewportLeft + viewportWidth,
      bottom: viewportTop + viewportHeight,
      width: viewportWidth,
      height: viewportHeight,
    };
  }
  return {
    left: 0,
    top: 0,
    right: window.innerWidth,
    bottom: window.innerHeight,
    width: window.innerWidth,
    height: window.innerHeight,
  };
}

function getDockPortalContainer(host: SelectionCommentHost, owner: HTMLElement) {
  return host.portalContainer ?? owner.closest<HTMLElement>('[role="dialog"]') ?? document.body;
}

function shouldShowDock(
  active: boolean,
  docked: boolean,
  host: SelectionCommentHost | null,
): host is SelectionCommentHost {
  return active && docked && host !== null;
}

function getDockPosition({
  host,
  owner,
  container,
  safeArea,
  viewportOffset,
}: {
  host: SelectionCommentHost;
  owner: HTMLElement;
  container: HTMLElement;
  safeArea: ReturnType<typeof readSafeAreaInsets>;
  viewportOffset: DockViewportOffset;
}) {
  const boundary = host.boundaryRef?.current ?? null;
  if (!owner.isConnected || (boundary && !boundary.isConnected)) return null;
  const portalRect =
    container === document.body || container === document.documentElement
      ? null
      : container.getBoundingClientRect();
  return resolveSelectionCommentDockPosition({
    ownerRect: rectFromElement(owner),
    boundaryRect: rectFromElement(boundary),
    viewportRect: getVisibleViewportRect(viewportOffset),
    dockWidth: DOCK_WIDTH,
    dockHeight: DOCK_HEIGHT,
    safeArea,
    portalOrigin: portalRect
      ? {
          left: portalRect.left,
          top: portalRect.top,
          scrollLeft: container.scrollLeft,
          scrollTop: container.scrollTop,
        }
      : undefined,
  });
}

function useSelectionCommentDockLayout(
  active: boolean,
  docked: boolean,
  host: SelectionCommentHost | null,
  viewportOffset: DockViewportOffset,
) {
  const [position, setPosition] = useState<SelectionCommentDockPosition | null>(null);
  const [portalRoot, setPortalRoot] = useState<HTMLElement | null>(null);
  const portalPosition = useRef<"fixed" | "absolute">("fixed");

  useEffect(() => {
    if (!shouldShowDock(active, docked, host)) {
      setPosition(null);
      setPortalRoot(null);
      return;
    }

    const owner = host.ownerRef.current;
    if (!owner?.isConnected) {
      setPosition(null);
      setPortalRoot(null);
      return;
    }

    const container = getDockPortalContainer(host, owner);
    setPortalRoot(container);
    portalPosition.current =
      container === document.body || container === document.documentElement ? "fixed" : "absolute";

    const safeArea = readSafeAreaInsets();
    const update = () => {
      setPosition(getDockPosition({ host, owner, container, safeArea, viewportOffset }));
    };
    update();

    const boundary = host.boundaryRef?.current ?? null;
    const resizeObserver =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(update);
    resizeObserver?.observe(owner);
    if (boundary) resizeObserver?.observe(boundary);
    const onScroll = () => update();
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onScroll);
    window.visualViewport?.addEventListener("resize", onScroll);
    window.visualViewport?.addEventListener("scroll", onScroll);
    return () => {
      resizeObserver?.disconnect();
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onScroll);
      window.visualViewport?.removeEventListener("resize", onScroll);
      window.visualViewport?.removeEventListener("scroll", onScroll);
    };
  }, [
    active,
    docked,
    host,
    viewportOffset.viewportHeight,
    viewportOffset.viewportLeft,
    viewportOffset.viewportTop,
    viewportOffset.viewportWidth,
  ]);

  return { position, portalRoot, portalPosition };
}

export function SelectionCommentDock({
  active,
  label,
  accessibleName,
  onOpen,
  testId = "selection-comment-dock-action",
}: {
  active: boolean;
  label: string;
  accessibleName: string;
  onOpen: () => void;
  testId?: string;
}) {
  const { t } = useTranslation();
  const host = useContext(SelectionCommentHostContext);
  const docked = useSelectionCommentDockPresentation();
  const viewportOffset = useVisualViewportOffset();
  const dockId = useId();
  const { position, portalRoot, portalPosition } = useSelectionCommentDockLayout(
    active,
    docked,
    host,
    viewportOffset,
  );

  useEffect(() => {
    if (!host) return;
    host.setReservation(dockId, active && docked && position ? position.clearance : null);
    return () => host.setReservation(dockId, null);
  }, [active, dockId, docked, host, position]);

  if (!active || !docked || !host || !position || !portalRoot) return null;
  const owner = host.ownerRef.current;
  if (!owner?.isConnected) return null;

  return createPortal(
    <div
      data-testid="selection-comment-dock"
      role="toolbar"
      aria-label={label}
      className="z-50 flex items-center justify-center rounded-xl border border-border/70 bg-popover/95 p-1 shadow-lg backdrop-blur"
      style={{
        position: portalPosition.current,
        left: position.left,
        top: position.top,
        width: position.width,
        height: position.height,
        pointerEvents: "auto",
      }}
    >
      <Button
        type="button"
        size="sm"
        aria-label={accessibleName}
        title={t("task:commentCmdShiftC")}
        data-testid={testId}
        data-selection-comment-dock-action="true"
        className="h-11 min-h-11 w-full min-w-11 cursor-pointer justify-center gap-1.5 rounded-lg px-3 text-sm font-medium"
        style={{ minHeight: 44, minWidth: 44 }}
        onPointerDown={(event) => {
          if (event.pointerType !== "touch") event.preventDefault();
        }}
        onMouseDown={(event) => event.preventDefault()}
        onClick={onOpen}
      >
        <IconMessagePlus className="h-4 w-4" aria-hidden="true" />
        {label}
      </Button>
    </div>,
    portalRoot,
  );
}
