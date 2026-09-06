"use client";

import { useMemo, useState } from "react";
import { IconUserPlus } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { useTranslation } from "react-i18next";
import { Avatar, AvatarFallback } from "@kandev/ui/avatar";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { useAppStore } from "@/components/state-provider";
import { useAssignablePeople } from "@/hooks/domains/users/use-assignable-people";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { initialsFor } from "@/lib/user-initials";

// i18n-exempt: sentinels compared with ===, never displayed.
const UNASSIGNED = "__unassigned__";
// i18n-exempt: sentinel compared with ===, never displayed.
const ASSIGN_TO_ME = "__assign_to_me__";

type Props = {
  taskId?: string | null;
  workspaceId?: string | null;
  isArchived?: boolean;
};

/**
 * The human assignee for a kanban task, in the task top bar.
 *
 * The office properties panel has a roomier row for the same field; this is
 * the same capability shaped for a crowded bar, so "Assign to me" is the first
 * dropdown entry rather than a separate always-visible button.
 *
 * No optimistic patch: the backend publishes `task.updated` with the new
 * `assignee_user_id` and the single kanban mapper puts it back into the store,
 * which is the same path every other top-bar mutation relies on.
 */
export function TaskAssigneeControl({ taskId, workspaceId, isArchived }: Props) {
  const { t } = useTranslation();
  const currentUser = useAppStore((s) => s.auth.user);
  // Read from the store rather than through props: `task.updated` lands the
  // new assignee there, so another person taking the task over shows up here
  // without a refetch, and there is no prop chain to drop a hop in.
  const assigneeUserId = useAppStore(
    (s) => s.kanban.tasks.find((entry: { id: string }) => entry.id === taskId)?.assigneeUserId,
  );
  const enabled = Boolean(currentUser) && Boolean(taskId) && !isArchived;
  const { people, nameFor } = useAssignablePeople(workspaceId, { enabled });
  const [pending, setPending] = useState(false);

  const assignee = assigneeUserId ?? "";
  const isMine = Boolean(currentUser) && assignee === currentUser?.id;

  const options = useMemo<ComboboxOption[]>(() => {
    const entries: ComboboxOption[] = [
      {
        value: UNASSIGNED,
        label: t("task:unassigned"),
        keywords: ["none", "unassigned"],
        renderLabel: () => (
          <span className="flex items-center gap-1.5 text-muted-foreground">
            <IconUserPlus className="h-4 w-4 shrink-0 opacity-70" />
            <span className="truncate">{t("task:unassigned")}</span>
          </span>
        ),
      },
    ];
    if (currentUser && !isMine) {
      entries.unshift({
        value: ASSIGN_TO_ME,
        label: t("task:assignToMe"),
        keywords: ["me", "self"],
        renderLabel: () => <span className="font-medium">{t("task:assignToMe")}</span>,
      });
    }
    const known = new Map(people.map((p) => [p.id, p.name]));
    if (assignee && !known.has(assignee)) known.set(assignee, nameFor(assignee));
    for (const [id, name] of known) {
      entries.push({
        value: id,
        label: name,
        keywords: [name, id],
        // The trigger renders the selected option's label, so the avatar comes
        // along for free rather than needing a trigger-prefix escape hatch.
        renderLabel: () => (
          <span className="flex min-w-0 items-center gap-1.5">
            <Avatar className="h-4 w-4 shrink-0">
              <AvatarFallback className="text-[9px]">{initialsFor(name)}</AvatarFallback>
            </Avatar>
            <span className="truncate">{name}</span>
          </span>
        ),
      });
    }
    return entries;
  }, [people, assignee, currentUser, isMine, nameFor, t]);

  // With authentication disabled every visitor is the same anonymous user, so
  // there is nobody to assign to and the control would be a choice that cannot
  // mean anything.
  if (!enabled || !currentUser || !taskId) return null;

  const resolveChoice = (choice: string): string => {
    if (choice === ASSIGN_TO_ME) return currentUser.id ?? "";
    if (choice === UNASSIGNED) return "";
    return choice;
  };

  const apply = async (choice: string) => {
    const next = resolveChoice(choice);
    if (next === assignee) return;
    setPending(true);
    try {
      await updateTask(taskId, { assignee_user_id: next });
    } catch (err) {
      // The server refuses an assignee who cannot reach the workspace, with a
      // message written to be shown as-is.
      toast.error(err instanceof Error ? err.message : t("task:updateFailed"));
    } finally {
      setPending(false);
    }
  };

  return (
    <Combobox
      options={options}
      value={assignee || UNASSIGNED}
      onValueChange={(next) => void apply(next)}
      loading={pending}
      ariaLabel={t("task:assignedTo")}
      placeholder={t("task:unassigned")}
      searchPlaceholder={t("task:searchPeople")}
      emptyMessage={t("task:noPeopleFound")}
      triggerClassName="h-7 w-auto gap-1 px-2 text-xs"
      popoverAlign="end"
      testId="task-assignee-control"
    />
  );
}
