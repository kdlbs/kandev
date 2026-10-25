"use client";

import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import { useTabMaximizeOnDoubleClick } from "./use-tab-maximize";
import { useTabContextActions } from "./use-tab-context-actions";
import { useTranslation } from "react-i18next";

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
 *  Always provides "Close" and "Close Others". Panels may inject additional
 *  items via props.params.contextMenuItems. */
export function ContextMenuTab(props: IDockviewPanelHeaderProps) {
  const { t } = useTranslation();
  const { api, containerApi } = props;
  const onDoubleClick = useTabMaximizeOnDoubleClick(api);
  const { handleClose, handleCloseOthers } = useTabContextActions(api, containerApi);

  const extraItems: TabContextMenuItem[] =
    (props.params as TabContextMenuParams | undefined)?.contextMenuItems ?? [];

  return (
    <ContextMenu>
      <ContextMenuTrigger
        className="flex h-full items-center cursor-pointer select-none"
        onDoubleClick={onDoubleClick}
      >
        <DockviewDefaultTab {...props} />
      </ContextMenuTrigger>
      <ContextMenuContent>
        {extraItems.map((item) => (
          <ContextMenuItem
            key={item.label}
            className="cursor-pointer"
            onSelect={item.onSelect}
            disabled={item.disabled}
          >
            {item.label}
          </ContextMenuItem>
        ))}
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
