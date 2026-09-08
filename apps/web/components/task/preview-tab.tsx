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
import { useTabMaximizeOnDoubleClick } from "./use-tab-maximize";
import { useTabContextActions } from "./use-tab-context-actions";
import { useTranslation } from "react-i18next";

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

/**
 * Preview tab: italic title + double-click to pin + middle-click to close.
 * One per preview type (file-editor / file-diff / commit-detail).
 */
function PreviewTab(props: IDockviewPanelHeaderProps & { type: PreviewType }) {
  const { t } = useTranslation();
  const { api, containerApi, type } = props;
  const promote = useDockviewStore((s) => s.promotePreviewToPinned);
  const onMouseDown = useMiddleClickClose(api, containerApi);
  const { handleClose, handleCloseOthers } = useTabContextActions(api, containerApi);
  const handleMaximizeDblClick = useTabMaximizeOnDoubleClick(api);
  const isPromoted = (props.params as Record<string, unknown> | undefined)?.promoted === true;

  const onDoubleClick = useCallback(
    (event: React.MouseEvent) => {
      if (!isPromoted) {
        promote(type);
        return;
      }
      handleMaximizeDblClick(event);
    },
    [promote, type, isPromoted, handleMaximizeDblClick],
  );

  const handleKeepOpen = useCallback(() => {
    promote(type);
  }, [promote, type]);

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div
          className={cn(
            "flex h-full items-center cursor-pointer select-none",
            !isPromoted && "italic",
          )}
          onMouseDown={onMouseDown}
          onDoubleClick={onDoubleClick}
          title={isPromoted ? undefined : t("task:doubleClickToKeepThisTab")}
          data-testid={`preview-tab-${type}`}
        >
          <DockviewDefaultTab {...props} />
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem className="cursor-pointer" onSelect={handleClose}>
          {t("task:close")}
        </ContextMenuItem>
        <ContextMenuItem className="cursor-pointer" onSelect={handleCloseOthers}>
          {t("task:closeOthers")}
        </ContextMenuItem>
        {!isPromoted && (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem className="cursor-pointer" onSelect={handleKeepOpen}>
              {t("task:keepOpen")}
            </ContextMenuItem>
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
  const { t } = useTranslation();
  const { api, containerApi } = props;
  const onMouseDown = useMiddleClickClose(api, containerApi);
  const { handleClose, handleCloseOthers } = useTabContextActions(api, containerApi);
  const onDoubleClick = useTabMaximizeOnDoubleClick(api);

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div
          className="flex h-full items-center cursor-pointer select-none"
          onMouseDown={onMouseDown}
          onDoubleClick={onDoubleClick}
        >
          <DockviewDefaultTab {...props} />
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem className="cursor-pointer" onSelect={handleClose}>
          {t("task:close")}
        </ContextMenuItem>
        <ContextMenuItem className="cursor-pointer" onSelect={handleCloseOthers}>
          {t("task:closeOthers")}
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}
