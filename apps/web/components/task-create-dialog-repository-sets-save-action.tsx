"use client";

import { IconDeviceFloppy } from "@tabler/icons-react";
import { DropdownMenuItem } from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";

/**
 * The "Save as set" entry in the Sets menu. Separate from the dialog so the menu
 * body stays a list of menu items and the dialog is mounted by the row rather
 * than inside a menu that closes on select.
 */
export function SaveRepositorySetMenuAction({ onSelect }: { onSelect: () => void }) {
  const { t } = useTranslation();
  return (
    <DropdownMenuItem
      className="cursor-pointer gap-2"
      data-testid="repository-set-save-action"
      onSelect={onSelect}
    >
      <IconDeviceFloppy className="h-3.5 w-3.5" />
      <span>{t("task:repositorySetsSaveAction")}</span>
    </DropdownMenuItem>
  );
}
