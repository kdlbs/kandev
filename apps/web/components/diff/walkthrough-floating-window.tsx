"use client";

import {
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
  type RefObject,
} from "react";
import { useAppStore } from "@/components/state-provider";
import { useFileEditors } from "@/hooks/use-file-editors";
import { revealFileAtLine, type OpenFileFn } from "@/lib/diff/walkthrough-reveal";
import {
  clearWalkthroughEditorAnchor,
  computeWalkthroughConnectorPath,
  isWalkthroughAnchorTargetVisible,
  useWalkthroughEditorAnchor,
  type WalkthroughViewportRect,
} from "@/lib/walkthrough-editor-anchor";
import { WalkthroughStepInner, useHasActiveWalkthroughStep } from "./walkthrough-step-card";
import { cn } from "@kandev/ui/lib/utils";

const DESKTOP_QUERY = "(min-width: 640px)";
const CARD_WIDTH = 400;
const VIEWPORT_MARGIN = 16;

function viewportRectFromDomRect(rect: DOMRect): WalkthroughViewportRect {
  return {
    left: rect.left,
    top: rect.top,
    right: rect.right,
    bottom: rect.bottom,
    width: rect.width,
    height: rect.height,
  };
}

function clampPosition(pos: { x: number; y: number }, width: number, height: number) {
  return {
    x: Math.min(window.innerWidth - width - VIEWPORT_MARGIN, Math.max(VIEWPORT_MARGIN, pos.x)),
    y: Math.min(window.innerHeight - height - VIEWPORT_MARGIN, Math.max(VIEWPORT_MARGIN, pos.y)),
  };
}

function defaultDesktopPosition() {
  return {
    x: Math.max(VIEWPORT_MARGIN, window.innerWidth - CARD_WIDTH - 24),
    y: 80,
  };
}

function useIsDesktop() {
  const [isDesktop, setIsDesktop] = useState(false);
  useEffect(() => {
    const media = window.matchMedia(DESKTOP_QUERY);
    const update = () => setIsDesktop(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  return isDesktop;
}

function useDraggableWindow(cardRef: RefObject<HTMLDivElement | null>, isDesktop: boolean) {
  const [position, setPosition] = useState<{ x: number; y: number } | null>(null);
  useEffect(() => {
    if (!isDesktop) return;
    setPosition((current) => current ?? defaultDesktopPosition());
  }, [isDesktop]);

  const onPointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!isDesktop) return;
    const target = event.target as HTMLElement;
    if (!target.closest("[data-walkthrough-drag-handle]")) return;
    if (target.closest("button,input,textarea,a")) return;
    const card = cardRef.current;
    if (!card) return;
    event.preventDefault();
    card.setPointerCapture(event.pointerId);
    const rect = card.getBoundingClientRect();
    const offset = { x: event.clientX - rect.left, y: event.clientY - rect.top };
    const onMove = (moveEvent: PointerEvent) => {
      setPosition(
        clampPosition(
          { x: moveEvent.clientX - offset.x, y: moveEvent.clientY - offset.y },
          rect.width,
          rect.height,
        ),
      );
    };
    const onUp = () => {
      card.releasePointerCapture(event.pointerId);
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  };

  return { position, onPointerDown };
}

function useCardRect(
  cardRef: RefObject<HTMLDivElement | null>,
  position: { x: number; y: number } | null,
  anchorKey: string | undefined,
) {
  const [rect, setRect] = useState<WalkthroughViewportRect | null>(null);
  useEffect(() => {
    const card = cardRef.current;
    if (!card) return;
    const update = () => setRect(viewportRectFromDomRect(card.getBoundingClientRect()));
    update();
    const observer = new ResizeObserver(update);
    observer.observe(card);
    window.addEventListener("resize", update);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", update);
    };
  }, [cardRef, position, anchorKey]);
  return rect;
}

function useVisibleWalkthroughAnchor() {
  const anchor = useWalkthroughEditorAnchor();
  useEffect(() => {
    if (!anchor?.container) return;
    const check = () => {
      if (!isWalkthroughAnchorTargetVisible(anchor.container ?? null, anchor.rect)) {
        clearWalkthroughEditorAnchor(anchor.key);
      }
    };
    check();
    const interval = window.setInterval(check, 150);
    window.addEventListener("resize", check);
    window.addEventListener("scroll", check, true);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("resize", check);
      window.removeEventListener("scroll", check, true);
    };
  }, [anchor]);
  return anchor;
}

function WalkthroughConnector({ cardRect }: { cardRect: WalkthroughViewportRect | null }) {
  const anchor = useVisibleWalkthroughAnchor();
  if (!anchor || !cardRect) return null;
  if (anchor.container && !isWalkthroughAnchorTargetVisible(anchor.container, anchor.rect)) {
    return null;
  }
  const path = computeWalkthroughConnectorPath(cardRect, anchor.rect);
  if (!path) return null;
  return (
    <svg
      aria-hidden="true"
      data-testid="walkthrough-connector"
      className="pointer-events-none fixed inset-0 z-[42] hidden h-screen w-screen sm:block"
    >
      <path
        d={path}
        fill="none"
        stroke="var(--primary)"
        strokeLinecap="round"
        strokeWidth="2"
        opacity="0.72"
      />
      <circle
        cx={anchor.rect.right}
        cy={anchor.rect.top + anchor.rect.height / 2}
        r="4"
        fill="var(--primary)"
      />
    </svg>
  );
}

/**
 * The primary walkthrough surface: a floating card that, for each step, opens the
 * step's file in its *current state* and reveals/centers the anchored line in the
 * editor. Works uniformly for changed and unchanged files (onboarding / whole-repo
 * tours), so it does not depend on the file being part of a diff.
 */
export function WalkthroughFloatingWindow({
  onClose,
  onSelectFile,
}: {
  onClose: () => void;
  onSelectFile?: OpenFileFn;
}) {
  const hasStep = useHasActiveWalkthroughStep();
  const { openFile: defaultOpenFile } = useFileEditors();
  const openFile = onSelectFile ?? defaultOpenFile;
  const cardRef = useRef<HTMLDivElement>(null);
  const isDesktop = useIsDesktop();
  const { position, onPointerDown } = useDraggableWindow(cardRef, isDesktop);
  const anchor = useWalkthroughEditorAnchor();
  const cardRect = useCardRect(cardRef, position, anchor?.key);
  // Select primitives (not a fresh object) so the selector stays referentially
  // stable — returning a new object here would loop the store subscription.
  const stepFile = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return null;
    const wt = s.walkthroughs.byTaskId[taskId];
    const idx = s.walkthroughs.activeStepByTaskId[taskId] ?? 0;
    return wt?.steps[idx]?.file ?? null;
  });
  const stepLine = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return 0;
    const wt = s.walkthroughs.byTaskId[taskId];
    const idx = s.walkthroughs.activeStepByTaskId[taskId] ?? 0;
    return wt?.steps[idx]?.line ?? 0;
  });
  const stepRepo = useAppStore((s) => {
    const taskId = s.tasks.activeTaskId;
    if (!taskId) return undefined;
    const wt = s.walkthroughs.byTaskId[taskId];
    const idx = s.walkthroughs.activeStepByTaskId[taskId] ?? 0;
    return wt?.steps[idx]?.repo;
  });

  // Open the step's file (current state) and reveal its line whenever the step changes.
  useEffect(() => {
    if (stepFile) revealFileAtLine(openFile, stepFile, stepLine, stepRepo);
  }, [stepFile, stepLine, stepRepo, openFile]);

  if (!hasStep) return null;

  return (
    <>
      {isDesktop ? <WalkthroughConnector cardRect={cardRect} /> : null}
      <div
        ref={cardRef}
        data-testid="walkthrough-floating"
        data-mobile-variant="bottom-sheet"
        className={cn(
          "fixed inset-x-2 bottom-2 z-[43] max-h-[calc(100dvh-1rem)] overflow-y-auto rounded-xl",
          "sm:inset-x-auto sm:bottom-auto sm:w-[400px] sm:max-w-[calc(100vw-2rem)]",
        )}
        style={isDesktop && position ? { left: position.x, top: position.y } : undefined}
        onPointerDown={onPointerDown}
      >
        <WalkthroughStepInner onClose={onClose} onSelectFile={openFile} />
      </div>
    </>
  );
}
