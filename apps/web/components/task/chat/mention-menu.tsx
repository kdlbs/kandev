"use client";

import {
  IconAt,
  IconFile,
  IconFolder,
  IconListCheck,
  IconClipboardList,
} from "@tabler/icons-react";
import type { MentionItem } from "@/hooks/use-inline-mention";
import { getFileName, isDirectory } from "@/lib/utils/file-path";
import { PopupMenu, PopupMenuItem, useMenuItemRefs } from "./popup-menu";
import { useTranslation } from "react-i18next";

type MentionMenuProps = {
  isOpen: boolean;
  isLoading: boolean;
  position?: { x: number; y: number } | null;
  clientRect?: (() => DOMRect | null) | null;
  items: MentionItem[];
  query: string;
  selectedIndex: number;
  onSelect: (item: MentionItem) => void;
  onClose: () => void;
  setSelectedIndex: (index: number) => void;
};

// Get the appropriate icon for an item
function getItemIcon(item: MentionItem) {
  if (item.kind === "prompt") {
    return <IconAt className="h-4 w-4" />;
  }
  if (item.kind === "plan") {
    return <IconListCheck className="h-4 w-4" />;
  }
  if (item.kind === "task") {
    return <IconClipboardList className="h-4 w-4" />;
  }
  const isDir = isDirectory(item.label);
  return isDir ? <IconFolder className="h-4 w-4" /> : <IconFile className="h-4 w-4" />;
}

// Get the label and description for an item
function getItemDisplay(item: MentionItem): { label: string; description?: string } {
  if (item.kind === "prompt" || item.kind === "plan" || item.kind === "task") {
    return { label: item.label, description: item.description };
  }
  const name = getFileName(item.label);
  const parent = item.label.slice(0, item.label.length - name.length);
  return { label: name, description: parent || undefined };
}

export function MentionMenu({
  isOpen,
  isLoading,
  position,
  clientRect,
  items,
  query,
  selectedIndex,
  onSelect,
  onClose,
  setSelectedIndex,
}: MentionMenuProps) {
  const { t } = useTranslation();
  const { setItemRef } = useMenuItemRefs(selectedIndex);

  const emptyState = (
    <div className="px-3 py-1 text-center text-xs text-muted-foreground">
      {(() => {
        if (isLoading) return t("task:loading");
        if (query) return t("task:noResultsFound");
        return t("task:typeToSearch");
      })()}
    </div>
  );

  return (
    <PopupMenu
      isOpen={isOpen}
      position={position ?? null}
      clientRect={clientRect}
      title={t("task:mentionTasksFilesPrompts")}
      selectedIndex={selectedIndex}
      onClose={onClose}
      hasItems={items.length > 0}
      emptyState={emptyState}
    >
      {items.map((item, index) => {
        const { label, description } = getItemDisplay(item);
        return (
          <PopupMenuItem
            key={item.id}
            icon={getItemIcon(item)}
            label={label}
            description={description}
            isSelected={selectedIndex === index}
            onClick={() => onSelect(item)}
            onMouseEnter={() => setSelectedIndex(index)}
            itemRef={setItemRef(index)}
          />
        );
      })}
    </PopupMenu>
  );
}
