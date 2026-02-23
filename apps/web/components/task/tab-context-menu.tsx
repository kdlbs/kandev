"use client";

import { useCallback } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";

/** An item in the tab right-click context menu.
 *  Items are ephemeral — not serialized to the saved layout. */
export type TabContextMenuItem = {
  label: string;
  onSelect: () => void;
  disabled?: boolean;
};

/** Params shape that panels can use to inject context menu items. */
export type TabContextMenuParams = {
  contextMenuItems?: TabContextMenuItem[];
};

/** Default tab component — wraps DockviewDefaultTab with a right-click menu.
 *  Always provides "Close Others". Panels may inject additional items via
 *  props.params.contextMenuItems. */
export function ContextMenuTab(props: IDockviewPanelHeaderProps) {
  const { api, containerApi } = props;

  const handleCloseOthers = useCallback(() => {
    const toClose = api.group.panels.filter((p) => p.id !== api.id && p.id !== "chat");
    for (const panel of toClose) containerApi.removePanel(panel);
  }, [api, containerApi]);

  const extraItems: TabContextMenuItem[] =
    (props.params as TabContextMenuParams | undefined)?.contextMenuItems ?? [];

  return (
    <ContextMenu>
      <ContextMenuTrigger className="flex h-full items-center">
        <DockviewDefaultTab {...props} />
      </ContextMenuTrigger>
      <ContextMenuContent>
        {extraItems.map((item) => (
          <ContextMenuItem key={item.label} onSelect={item.onSelect} disabled={item.disabled}>
            {item.label}
          </ContextMenuItem>
        ))}
        <ContextMenuItem onSelect={handleCloseOthers}>Close Others</ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}
