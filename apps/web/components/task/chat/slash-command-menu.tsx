"use client";

import { IconRobot } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import type { SlashCommand, SlashCommandModeState } from "./slash-command-types";
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
        <SlashCommandOption
          key={command.id}
          command={command}
          index={index}
          selectedIndex={selectedIndex}
          onSelect={onSelect}
          setSelectedIndex={setSelectedIndex}
          setItemRef={setItemRef}
        />
      ))}
    </PopupMenu>
  );
}

type SlashCommandOptionProps = {
  command: SlashCommand;
  index: number;
  selectedIndex: number;
  onSelect: (command: SlashCommand) => void;
  setSelectedIndex: (index: number) => void;
  setItemRef: (index: number) => (element: HTMLButtonElement | null) => void;
};

function planModeDescriptionKey(modeState: SlashCommandModeState | undefined): string {
  if (modeState === "active") return "task:slashCommandPlanModeOff";
  if (modeState === "default") return "task:slashCommandPlanModeOn";
  return "task:slashCommandPlanModeToggle";
}

function SlashCommandOption({
  command,
  index,
  selectedIndex,
  onSelect,
  setSelectedIndex,
  setItemRef,
}: SlashCommandOptionProps) {
  const { t } = useTranslation();
  const category = command.kind === "skill" ? t("task:slashCommandSkill") : undefined;
  const mode = command.modeAction ? t("task:slashCommandMode") : undefined;
  const active = command.modeState === "active" ? t("task:slashCommandActive") : undefined;
  const description = command.modeAction
    ? t(planModeDescriptionKey(command.modeState))
    : command.description;
  const hint = command.inputHint
    ? `${t("task:slashCommandArguments")}: ${command.inputHint}`
    : undefined;
  const accessibleLabel = [command.label, category, mode, active, description, hint]
    .filter(Boolean)
    .join(", ");

  return (
    <PopupMenuItem
      icon={<IconRobot className="h-4 w-4" />}
      label={command.label}
      description={description}
      badges={
        category || mode || active ? (
          <span className="flex shrink-0 items-center gap-1">
            {category && (
              <Badge variant="outline" className="h-4 rounded px-1 py-0 text-[10px]">
                {category}
              </Badge>
            )}
            {mode && (
              <Badge variant="outline" className="h-4 rounded px-1 py-0 text-[10px]">
                {mode}
              </Badge>
            )}
            {active && (
              <Badge variant="secondary" className="h-4 rounded px-1 py-0 text-[10px]">
                {active}
              </Badge>
            )}
          </span>
        ) : undefined
      }
      hint={hint}
      accessibleLabel={accessibleLabel}
      isSelected={selectedIndex === index}
      onClick={() => onSelect(command)}
      onMouseEnter={() => setSelectedIndex(index)}
      itemRef={setItemRef(index)}
    />
  );
}
