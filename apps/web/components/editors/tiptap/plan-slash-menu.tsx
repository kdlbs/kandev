"use client";

import { createElement } from "react";
import { PopupMenu, PopupMenuItem, useMenuItemRefs } from "@/components/task/chat/popup-menu";
import type { MenuState } from "@/components/task/chat/tiptap-suggestion";
import type { PlanSlashCategory, PlanSlashCommand } from "./plan-slash-commands";
import { useTranslation } from "react-i18next";

/**
 * Category ids are stable grouping keys, so the heading copy is resolved here at
 * render rather than stored on the command. Holds keys, never `t()` results.
 */
const CATEGORY_LABEL_KEYS: Record<PlanSlashCategory, string> = {
  text: "editors:slashCategoryText",
  lists: "editors:slashCategoryLists",
  blocks: "editors:slashCategoryBlocks",
};

type PlanSlashMenuProps = {
  menuState: MenuState<PlanSlashCommand>;
  selectedIndex: number;
  setSelectedIndex: (index: number) => void;
};

export function PlanSlashMenu({ menuState, selectedIndex, setSelectedIndex }: PlanSlashMenuProps) {
  const { t } = useTranslation();
  const { setItemRef } = useMenuItemRefs(selectedIndex);

  if (!menuState.isOpen || menuState.items.length === 0) return null;

  // Group items by category
  const grouped = new Map<PlanSlashCategory, { items: PlanSlashCommand[]; startIndex: number }>();
  let idx = 0;
  for (const item of menuState.items) {
    const existing = grouped.get(item.category);
    if (existing) {
      existing.items.push(item);
    } else {
      grouped.set(item.category, { items: [item], startIndex: idx });
    }
    idx++;
  }

  return (
    <PopupMenu
      isOpen={menuState.isOpen}
      position={null}
      clientRect={menuState.clientRect}
      title={t("editors:insertBlock")}
      selectedIndex={selectedIndex}
      onClose={() => menuState.command?.(menuState.items[0]!)}
      hasItems={menuState.items.length > 0}
      placement="below"
    >
      {Array.from(grouped.entries()).map(([category, group]) => (
        <div key={category}>
          <div className="px-3 pt-2 pb-1">
            <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground/70">
              {t(CATEGORY_LABEL_KEYS[category])}
            </span>
          </div>
          {group.items.map((item) => {
            const itemIndex = menuState.items.indexOf(item);
            return (
              <PopupMenuItem
                key={item.id}
                icon={createElement(item.icon, { className: "h-4 w-4" })}
                label={item.label}
                description={item.description}
                isSelected={itemIndex === selectedIndex}
                onClick={() => menuState.command?.(item)}
                onMouseEnter={() => setSelectedIndex(itemIndex)}
                itemRef={setItemRef(itemIndex)}
              />
            );
          })}
        </div>
      ))}
    </PopupMenu>
  );
}
