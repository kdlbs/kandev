"use client";

import { useCallback } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
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

/**
 * Preview tab: italic title + double-click to pin + middle-click to close.
 * One per preview type (file-editor / file-diff / commit-detail).
 */
function PreviewTab(props: IDockviewPanelHeaderProps & { type: PreviewType }) {
  const { api, containerApi, type } = props;
  const promote = useDockviewStore((s) => s.promotePreviewToPinned);
  const onMouseDown = useMiddleClickClose(api, containerApi);

  const onDoubleClick = useCallback(() => {
    const newId = promote(type);
    if (newId) {
      // Focus the newly-created pinned tab so the user sees the promotion.
      const pinned = containerApi.getPanel(newId);
      if (pinned) pinned.api.setActive();
    }
  }, [promote, type, containerApi]);

  return (
    <div
      className="flex h-full items-center italic"
      onMouseDown={onMouseDown}
      onDoubleClick={onDoubleClick}
      title="Double-click to keep this tab open"
      data-testid={`preview-tab-${type}`}
    >
      <DockviewDefaultTab {...props} />
    </div>
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
 * Just adds middle-click-to-close on top of the dockview default.
 */
export function PinnedDefaultTab(props: IDockviewPanelHeaderProps) {
  const onMouseDown = useMiddleClickClose(props.api, props.containerApi);
  return (
    <div className="flex h-full items-center" onMouseDown={onMouseDown}>
      <DockviewDefaultTab {...props} />
    </div>
  );
}
