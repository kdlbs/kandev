"use client";

import { memo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import type { Message } from "@/lib/types/http";
import type { TurnGroup } from "@/hooks/use-processed-messages";
import {
  TurnGroupContent,
  type TurnGroupContentProps,
} from "@/components/task/chat/messages/turn-group-message";
import { formatActivityDuration, type ActivityChipInfo } from "./activity-display";

type ActivityChipProps = Omit<TurnGroupContentProps, "group"> & {
  group: TurnGroup;
  chip: ActivityChipInfo;
  permissionsByToolCallId: Map<string, Message>;
};

/** The chip's label: count, then duration and failed calls when known. */
export function useActivityChipLabel(chip: ActivityChipInfo): string {
  const { t } = useTranslation("coordinator");
  const parts = [t("activityChipSources", { count: chip.count })];
  if (chip.durationSeconds !== null) parts.push(formatActivityDuration(chip.durationSeconds));
  if (chip.failed > 0) parts.push(t("activityChipFailed", { count: chip.failed }));
  return parts.join(" · "); // i18n-exempt: separator
}

/** One collapsed chip holding a finished turn's tool calls. The expanded state
 *  is component state, keyed by the group id (turn id plus segment ordinal). */
export const ActivityChip = memo(function ActivityChip({
  chip,
  ...contentProps
}: ActivityChipProps) {
  const [expanded, setExpanded] = useState(false);
  const label = useActivityChipLabel(chip);
  return (
    <div className="w-full" data-testid="activity-chip">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((value) => !value)}
        className="flex items-center gap-2 w-full text-left px-2 py-1.5 -mx-2 rounded hover:bg-muted/30 transition-colors cursor-pointer"
      >
        {expanded ? (
          <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0" />
        ) : (
          <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0" />
        )}
        <span className="text-xs text-muted-foreground">{label}</span>
      </button>
      {expanded && <TurnGroupContent {...contentProps} />}
    </div>
  );
});
