"use client";

import { IconRobot } from "@tabler/icons-react";
import type { SlashCommand } from "./slash-command-types";
import { PopupMenu, PopupMenuItem, useMenuItemRefs } from "./popup-menu";
import { useTranslation } from "react-i18next";

type SlashCommandMenuProps = {
  isOpen: boolean;
  position?: { x: number; y: number } | null;
  clientRect?: (() => DOMRect | null) | null;
  commands: SlashCommand[];
  selectedIndex: number;
  onSelect: (command: SlashCommand) => void;
  onClose: () => void;
  setSelectedIndex: (index: number) => void;
};

export function SlashCommandMenu({
  isOpen,
  position,
  clientRect,
  commands,
  selectedIndex,
  onSelect,
  onClose,
  setSelectedIndex,
}: SlashCommandMenuProps) {
  const { t } = useTranslation();
  const { setItemRef } = useMenuItemRefs(selectedIndex);

  if (commands.length === 0) {
    return null;
  }

  return (
    <PopupMenu
      isOpen={isOpen}
      position={position ?? null}
      clientRect={clientRect}
      title={t("common:commandGroupCommands")}
      selectedIndex={selectedIndex}
      onClose={onClose}
    >
      {commands.map((command, index) => (
        <PopupMenuItem
          key={command.id}
          icon={<IconRobot className="h-4 w-4" />}
          label={command.label}
          description={command.description}
          isSelected={selectedIndex === index}
          onClick={() => onSelect(command)}
          onMouseEnter={() => setSelectedIndex(index)}
          itemRef={setItemRef(index)}
        />
      ))}
    </PopupMenu>
  );
}
