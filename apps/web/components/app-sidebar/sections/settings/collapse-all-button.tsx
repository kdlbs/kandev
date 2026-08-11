"use client";

import { IconChevronsUp } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

import { useAppStore } from "@/components/state-provider";

/**
 * Shuts every open branch of the settings menu.
 *
 * Only in `persistent` mode, and only there because only there is it possible
 * to be stuck: accordion holds one path open and derives it from the route, so
 * moving anywhere already closes what you left, and `flat` has no branches at
 * all. Persistent is the mode that accumulates — that is its whole point — so
 * it is the one that needs a way back to a short menu.
 *
 * Disabled rather than hidden when nothing is open: a control that comes and
 * goes as you expand things is harder to find than one that greys out.
 */
export function CollapseAllButton() {
  const { t } = useTranslation();
  const mode = useAppStore((s) => s.settingsMenu.mode);
  const expandedKeys = useAppStore((s) => s.settingsMenu.expandedKeys);
  const setExpandedKeys = useAppStore((s) => s.setSettingsMenuExpandedKeys);

  if (mode !== "persistent") return null;

  const label = t("sidebar:collapseAllBranches");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="ml-auto h-6 w-6 shrink-0 cursor-pointer"
          onClick={() => setExpandedKeys([])}
          disabled={expandedKeys.length === 0}
          aria-label={label}
          data-testid="settings-collapse-all"
        >
          <IconChevronsUp className="h-3.5 w-3.5" />
        </Button>
      </TooltipTrigger>
      <TooltipContent side="bottom">{label}</TooltipContent>
    </Tooltip>
  );
}
