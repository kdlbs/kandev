"use client";

import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import { useTabMaximizeOnDoubleClick } from "./use-tab-maximize";

export function NoteTab(props: IDockviewPanelHeaderProps) {
  const onDoubleClick = useTabMaximizeOnDoubleClick(props.api);
  return (
    <div
      data-testid="notes-tab"
      className="cursor-pointer select-none"
      onDoubleClick={onDoubleClick}
    >
      <DockviewDefaultTab {...props} />
    </div>
  );
}
