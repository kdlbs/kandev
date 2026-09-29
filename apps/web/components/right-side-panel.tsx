"use client";

import { useCallback, useRef, type ReactNode } from "react";
import { useKanbanLayout } from "@/hooks/use-kanban-layout";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { PREVIEW_PANEL } from "@/lib/settings/constants";
import { getRenderedPreviewPanelWidth } from "@/lib/settings/preview-panel-width";

export type RightSidePanelProps = {
  open: boolean;
  onClose: () => void;
  /** Chosen width in px; the rendered width is floored to the pointer-aware minimum. */
  widthPx: number;
  onWidthChange: (widthPx: number) => void;
  backdropLabel: string;
  /** Content beside which the panel sits. */
  main: ReactNode;
  children: ReactNode;
  /**
   * "measured" sizes the main content to the layout hook's width and swaps between
   * an inline and a floating tree. "fluid" never sizes it and keeps one tree, so the
   * main content is not remounted when the panel flips between inline and floating.
   */
  mainSizing?: "measured" | "fluid";
  /** Below the mobile breakpoint the panel covers the viewport above the status bar. */
  mobileFullScreen?: boolean;
  /** Close on Escape while focus is inside the panel and no inner layer consumed it. */
  closeOnEscape?: boolean;
  panelTestId?: string;
};

type PanelFrameProps = Pick<
  RightSidePanelProps,
  "onClose" | "backdropLabel" | "closeOnEscape" | "panelTestId" | "children"
> & {
  widthPx: number;
  onResizeMouseDown: (e: React.MouseEvent) => void;
};

function suffixed(id: string | undefined, suffix: string) {
  return id ? `${id}-${suffix}` : undefined;
}

function ResizeHandle({
  onMouseDown,
  testId,
}: {
  onMouseDown: (e: React.MouseEvent) => void;
  testId?: string;
}) {
  return (
    <div
      data-testid={testId}
      className="w-1 bg-border hover:bg-primary cursor-col-resize flex-shrink-0 relative group"
      onMouseDown={onMouseDown}
    >
      <div className="absolute inset-y-0 -left-2 -right-2" />
      <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-1 h-8 bg-border group-hover:bg-primary rounded-full transition-colors" />
    </div>
  );
}

function usePanelWidth(widthPx: number, isFinePointer: boolean) {
  const minWidthPx = isFinePointer ? PREVIEW_PANEL.MIN_WIDTH_PX : PREVIEW_PANEL.COARSE_MIN_WIDTH_PX;
  const renderedWidthPx = getRenderedPreviewPanelWidth(widthPx, isFinePointer);
  const minWidthPxRef = useRef(minWidthPx);
  minWidthPxRef.current = minWidthPx;
  return { renderedWidthPx, minWidthPxRef };
}

function useResizeHandler(
  widthPx: number,
  onWidthChange: (widthPx: number) => void,
  minWidthPxRef: React.RefObject<number>,
) {
  return useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      const startX = e.clientX;
      const handleMouseMove = (moveEvent: MouseEvent) => {
        const deltaX = startX - moveEvent.clientX;
        onWidthChange(Math.max(widthPx + deltaX, minWidthPxRef.current));
      };
      const handleMouseUp = () => {
        window.removeEventListener("mousemove", handleMouseMove);
        window.removeEventListener("mouseup", handleMouseUp);
      };
      window.addEventListener("mousemove", handleMouseMove);
      window.addEventListener("mouseup", handleMouseUp);
    },
    [widthPx, onWidthChange, minWidthPxRef],
  );
}

function escapeHandler(closeOnEscape: boolean | undefined, onClose: () => void) {
  if (!closeOnEscape) return undefined;
  return (e: React.KeyboardEvent) => {
    if (e.key === "Escape" && !e.defaultPrevented) onClose();
  };
}

function PanelBody({
  onResizeMouseDown,
  panelTestId,
  children,
}: Pick<PanelFrameProps, "onResizeMouseDown" | "panelTestId" | "children">) {
  return (
    <>
      <ResizeHandle
        onMouseDown={onResizeMouseDown}
        testId={suffixed(panelTestId, "resize-handle")}
      />
      <div className="flex-1 min-w-0 overflow-hidden">{children}</div>
    </>
  );
}

function FloatingPanel(props: PanelFrameProps) {
  const { onClose, backdropLabel, closeOnEscape, panelTestId, widthPx } = props;
  return (
    <>
      <div
        data-testid={suffixed(panelTestId, "backdrop")}
        className="fixed inset-0 bg-black/30 z-30"
        onClick={onClose}
        aria-label={backdropLabel}
      />
      <div
        data-testid={panelTestId}
        className="fixed top-0 right-0 bottom-[var(--app-status-bar-height)] z-40 flex bg-background shadow-2xl"
        style={{ width: `${widthPx}px`, maxWidth: `${PREVIEW_PANEL.MAX_WIDTH_VW}vw` }}
        onKeyDown={escapeHandler(closeOnEscape, onClose)}
      >
        <PanelBody {...props} />
      </div>
    </>
  );
}

function InlinePanel(props: PanelFrameProps) {
  const { onClose, closeOnEscape, panelTestId, widthPx } = props;
  return (
    <div
      data-testid={panelTestId}
      className="flex-shrink-0 border-l bg-background flex"
      style={{ width: `${widthPx}px` }}
      onKeyDown={escapeHandler(closeOnEscape, onClose)}
    >
      <PanelBody {...props} />
    </div>
  );
}

function FullScreenPanel({
  onClose,
  closeOnEscape,
  panelTestId,
  children,
}: Pick<PanelFrameProps, "onClose" | "closeOnEscape" | "panelTestId" | "children">) {
  return (
    <div
      data-testid={panelTestId}
      className="fixed inset-x-0 top-0 bottom-[var(--app-status-bar-height)] z-40 flex min-w-0 bg-background"
      onKeyDown={escapeHandler(closeOnEscape, onClose)}
    >
      <div className="flex-1 min-w-0 overflow-hidden">{children}</div>
    </div>
  );
}

function renderPanel(frame: PanelFrameProps, fullScreen: boolean, shouldFloat: boolean) {
  if (fullScreen) return <FullScreenPanel {...frame} />;
  return shouldFloat ? <FloatingPanel {...frame} /> : <InlinePanel {...frame} />;
}

export function RightSidePanel({
  open,
  onClose,
  widthPx,
  onWidthChange,
  backdropLabel,
  main,
  children,
  mainSizing = "measured",
  mobileFullScreen = false,
  closeOnEscape,
  panelTestId,
}: RightSidePanelProps) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const { renderedWidthPx, minWidthPxRef } = usePanelWidth(widthPx, isFinePointer);
  const { containerRef, shouldFloat, kanbanWidth } = useKanbanLayout(open, renderedWidthPx);
  const onResizeMouseDown = useResizeHandler(renderedWidthPx, onWidthChange, minWidthPxRef);
  const fullScreen = mobileFullScreen && isMobile;
  const frame: PanelFrameProps = {
    onClose,
    backdropLabel,
    closeOnEscape,
    panelTestId,
    widthPx: renderedWidthPx,
    onResizeMouseDown,
    children,
  };
  const panel = open ? renderPanel(frame, fullScreen, shouldFloat) : null;
  const containerClass = "relative flex h-full min-h-0 w-full flex-col bg-background";

  if (mainSizing === "fluid") {
    return (
      <div ref={containerRef} className={containerClass}>
        <div className="flex-1 flex min-h-0 overflow-hidden">
          <div className="min-w-0 flex-1 overflow-hidden">{main}</div>
          {panel}
        </div>
      </div>
    );
  }

  const mainStyle = { width: `${kanbanWidth}px` };
  if (open && shouldFloat && !fullScreen) {
    return (
      <div ref={containerRef} className={containerClass}>
        <div className="flex-1 overflow-hidden" style={mainStyle}>
          {main}
        </div>
        {panel}
      </div>
    );
  }
  return (
    <div ref={containerRef} className={containerClass}>
      <div className="flex-1 flex overflow-hidden">
        <div className="overflow-hidden" style={mainStyle}>
          {main}
        </div>
        {panel}
      </div>
    </div>
  );
}
