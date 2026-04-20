"use client";

import { useCallback } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import { cn } from "@/lib/utils";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { PreviewType } from "@/lib/state/dockview-panel-actions";

/**
 * Middle-click to close any tab (preview or pinned).
 * Call `event.preventDefault()` to suppress the browser autoscroll gesture.
 */
export function useMiddleClickClose(
  api: IDockviewPanelHeaderProps["api"],
  containerApi: IDockviewPanelHeaderProps["containerApi"],
) {
  return useCallback(
    (event: React.MouseEvent<HTMLDivElement>) => {
      if (event.button !== 1) return;
      event.preventDefault();
      event.stopPropagation();
      const panel = containerApi.getPanel(api.id);
      if (panel) containerApi.removePanel(panel);
    },
    [api, containerApi],
  );
}

function useTabContextActions(
  api: IDockviewPanelHeaderProps["api"],
  containerApi: IDockviewPanelHeaderProps["containerApi"],
) {
  const handleClose = useCallback(() => {
    const panel = containerApi.getPanel(api.id);
    if (panel) containerApi.removePanel(panel);
  }, [api, containerApi]);

  const handleCloseOthers = useCallback(() => {
    const toClose = api.group.panels.filter(
      (p) => p.id !== api.id && p.id !== "chat" && !p.id.startsWith("session:"),
    );
    for (const panel of toClose) containerApi.removePanel(panel);
  }, [api, containerApi]);

  return { handleClose, handleCloseOthers };
}

/**
 * Preview tab: italic title + double-click to pin + middle-click to close.
 * One per preview type (file-editor / file-diff / commit-detail).
 */
function PreviewTab(props: IDockviewPanelHeaderProps & { type: PreviewType }) {
  const { api, containerApi, type } = props;
  const promote = useDockviewStore((s) => s.promotePreviewToPinned);
  const onMouseDown = useMiddleClickClose(api, containerApi);
  const { handleClose, handleCloseOthers } = useTabContextActions(api, containerApi);
  const isPromoted = (props.params as Record<string, unknown> | undefined)?.promoted === true;

  const onDoubleClick = useCallback(() => {
    if (!isPromoted) promote(type);
  }, [promote, type, isPromoted]);

  const handleKeepOpen = useCallback(() => {
    promote(type);
  }, [promote, type]);

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div
          className={cn("flex h-full items-center", !isPromoted && "italic")}
          onMouseDown={onMouseDown}
          onDoubleClick={onDoubleClick}
          title={isPromoted ? undefined : "Double-click to keep this tab open"}
          data-testid={`preview-tab-${type}`}
        >
          <DockviewDefaultTab {...props} />
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem onSelect={handleClose}>Close</ContextMenuItem>
        <ContextMenuItem onSelect={handleCloseOthers}>Close Others</ContextMenuItem>
        {!isPromoted && (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem onSelect={handleKeepOpen}>Keep Open</ContextMenuItem>
          </>
        )}
      </ContextMenuContent>
    </ContextMenu>
  );
}

export function PreviewFileTab(props: IDockviewPanelHeaderProps) {
  return <PreviewTab {...props} type="file-editor" />;
}
export function PreviewDiffTab(props: IDockviewPanelHeaderProps) {
  return <PreviewTab {...props} type="file-diff" />;
}
export function PreviewCommitTab(props: IDockviewPanelHeaderProps) {
  return <PreviewTab {...props} type="commit-detail" />;
}

/**
 * Default (non-preview) tab for pinned file/diff/commit panels.
 * Adds middle-click-to-close and a right-click context menu.
 */
export function PinnedDefaultTab(props: IDockviewPanelHeaderProps) {
  const { api, containerApi } = props;
  const onMouseDown = useMiddleClickClose(api, containerApi);
  const { handleClose, handleCloseOthers } = useTabContextActions(api, containerApi);

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div className="flex h-full items-center" onMouseDown={onMouseDown}>
          <DockviewDefaultTab {...props} />
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem onSelect={handleClose}>Close</ContextMenuItem>
        <ContextMenuItem onSelect={handleCloseOthers}>Close Others</ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}
