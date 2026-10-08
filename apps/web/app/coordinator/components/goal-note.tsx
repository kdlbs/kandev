"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useGoal } from "@/hooks/domains/coordinator/use-goal";
import type { Goal } from "@/lib/api/domains/coordinator-api";
import {
  formatCalendarDate,
  formatTimestampDate,
  isOverdue,
  metCriteriaCount,
} from "@/lib/coordinators/goal-form";

type GoalNoteProps = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
};

function goalSectionHref(workspaceId: string, coordinatorId: string): string {
  return `/settings/workspaces/${workspaceId}/coordinators/${coordinatorId}?section=goal`;
}

function ActiveGoalLine({ goal }: { goal: Goal }) {
  const { t, i18n } = useTranslation();
  const due = goal.due_on
    ? t(
        isOverdue(goal.due_on, new Date())
          ? "coordinator:goalNoteOverdue"
          : "coordinator:goalNoteDue",
        {
          date: formatCalendarDate(goal.due_on, i18n.language),
        },
      )
    : null;
  const criteria = t("coordinator:goalNoteCriteria", {
    met: metCriteriaCount(goal.criteria),
    count: goal.criteria.length,
  });
  return (
    <p className="min-w-0 break-words text-sm">
      <span className="font-medium">{goal.name}</span>
      {due && <span className="text-muted-foreground">{` · ${due}`}</span>}
      <span className="text-muted-foreground">{` · ${criteria}`}</span>
    </p>
  );
}

/** Goal state above the Needs you list; hidden while loading and when the goal read fails. */
export function GoalNote({ workspaceId, coordinatorId, canManage }: GoalNoteProps) {
  const { t, i18n } = useTranslation();
  const { data, status } = useGoal(workspaceId, coordinatorId);
  if (status !== "ready" || !data) return null;

  const active = data.active;
  const lastMet = data.last_met;
  let body: React.ReactNode;
  let actionKey: string | null = null;
  if (active) {
    body = <ActiveGoalLine goal={active} />;
  } else if (lastMet) {
    body = (
      <p className="text-sm">
        {t("coordinator:goalNoteMet", {
          name: lastMet.name,
          date: lastMet.met_at ? formatTimestampDate(lastMet.met_at, i18n.language) : "",
        })}
      </p>
    );
    actionKey = "coordinator:goalNoteSetNext";
  } else {
    body = <p className="text-sm">{t("coordinator:goalNoteNone")}</p>;
    actionKey = "coordinator:goalNoteSet";
  }

  return (
    <section
      aria-label={t("coordinator:goalNoteLabel")}
      data-testid="goal-note"
      className="mb-4 flex flex-col gap-2 rounded-md border p-3 sm:flex-row sm:items-center sm:justify-between"
    >
      {body}
      {canManage && actionKey && (
        <Button
          asChild
          variant="outline"
          size="sm"
          className="min-h-12 w-full cursor-pointer sm:min-h-9 sm:w-auto"
        >
          <Link href={goalSectionHref(workspaceId, coordinatorId)} data-testid="goal-note-action">
            {t(actionKey)}
          </Link>
        </Button>
      )}
    </section>
  );
}
