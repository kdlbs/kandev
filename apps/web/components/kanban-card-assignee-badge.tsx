"use client";

import { useTranslation } from "react-i18next";
import { Avatar, AvatarFallback } from "@kandev/ui/avatar";
import { useDirectoryNames } from "@/hooks/domains/users/use-assignable-people";
import { initialsFor } from "@/lib/user-initials";

/**
 * Who owns this task, so a board answers "who is on what" at a glance.
 *
 * Read-only on the card: assigning happens in the task's top bar, where there
 * is room for a picker and where taking a task over is a deliberate act rather
 * than something a stray click on a dense board can do.
 */
export function AssigneeBadge({ userId }: { userId: string }) {
  const { t } = useTranslation();
  const { nameFor } = useDirectoryNames();
  const name = nameFor(userId);
  return (
    <span
      className="flex items-center gap-1 text-muted-foreground"
      title={t("task:assignedToName", { name })}
      data-testid="kanban-card-assignee"
    >
      <Avatar className="h-4 w-4 shrink-0">
        <AvatarFallback className="text-[9px]">{initialsFor(name)}</AvatarFallback>
      </Avatar>
      <span className="max-w-24 truncate text-[10px]">{name}</span>
    </span>
  );
}
