"use client";

import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IconAlertTriangle,
  IconCalendarOff,
  IconCircleCheck,
  IconHelpCircle,
  IconWebhook,
} from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import type { Routine } from "@/lib/state/slices/office/types";
import {
  hasSchedulableUnarmedEntry,
  readScheduleState,
  readUnarmedCronTriggers,
  scheduleStateGroup,
  type ScheduleStateGroup,
} from "./schedule-state";

const GROUP_LABEL_KEYS: Record<ScheduleStateGroup, string> = {
  armed: "office:scheduleStateArmed",
  broken: "office:scheduleStateBroken",
  event_only: "office:scheduleStateEventOnly",
  no_schedule: "office:scheduleStateNoSchedule",
  unknown: "office:scheduleStateUnknown",
};

const GROUP_DESCRIPTION_KEYS: Record<ScheduleStateGroup, string> = {
  armed: "office:scheduleStateArmedDescription",
  broken: "office:scheduleStateBrokenDescription",
  event_only: "office:scheduleStateEventOnlyDescription",
  no_schedule: "office:scheduleStateNoScheduleDescription",
  unknown: "office:scheduleStateUnknownDescription",
};

const GROUP_ICONS: Record<ScheduleStateGroup, Icon> = {
  armed: IconCircleCheck,
  broken: IconAlertTriangle,
  event_only: IconWebhook,
  no_schedule: IconCalendarOff,
  unknown: IconHelpCircle,
};

const GROUP_VARIANTS: Record<
  ScheduleStateGroup,
  "default" | "secondary" | "destructive" | "outline"
> = {
  armed: "default",
  broken: "destructive",
  event_only: "outline",
  no_schedule: "secondary",
  unknown: "outline",
};

/**
 * ScheduleStateBadge renders the second, independent switch
 * (REQ-OFFICE-ROUTINE-ARMING-001's schedule state) alongside a routine's
 * existing intent badge. Each of the five label groups carries its own text
 * and icon (AC-002.9, AC-002.10) so it never collapses to color alone.
 */
export function ScheduleStateBadge({
  routine,
  className,
}: {
  routine: Routine;
  className?: string;
}) {
  const { t } = useTranslation();
  const group = scheduleStateGroup(readScheduleState(routine));
  const GroupIcon = GROUP_ICONS[group];
  const label = t(GROUP_LABEL_KEYS[group]);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant={GROUP_VARIANTS[group]} className={className} aria-label={label}>
          <GroupIcon aria-hidden="true" />
          <span>{label}</span>
        </Badge>
      </TooltipTrigger>
      <TooltipContent>{t(GROUP_DESCRIPTION_KEYS[group])}</TooltipContent>
    </Tooltip>
  );
}

// unarmedHintLabel resolves AC-OFFICE-ROUTINE-ARMING-002.4's schedulable vs.
// non-schedulable distinction to copy. Only called once the unarmed list is
// confirmed non-empty, so "schedulable" and "not schedulable" are always a
// real two-way split, never a vacuous one.
function unarmedHintLabel(t: TFunction, schedulable: boolean): string {
  return schedulable ? t("office:unarmedNeedsRearm") : t("office:unarmedNeedsEdit");
}

/**
 * UnarmedScheduleHint renders nothing when a routine's unarmed cron trigger
 * list is empty (AC-002.4's suppression rule), and otherwise distinguishes a
 * routine that only needs its existing triggers re-armed from one whose cron
 * expression or timezone needs editing first.
 */
export function UnarmedScheduleHint({
  routine,
  className,
}: {
  routine: Routine;
  className?: string;
}) {
  const { t } = useTranslation();
  const unarmed = readUnarmedCronTriggers(routine);
  if (unarmed.length === 0) return null;

  const schedulable = hasSchedulableUnarmedEntry(unarmed);
  const label = unarmedHintLabel(t, schedulable);

  return (
    <Badge variant="outline" className={className} aria-label={label}>
      <span>{label}</span>
    </Badge>
  );
}
