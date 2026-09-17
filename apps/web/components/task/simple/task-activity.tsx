"use client";

import { useContext } from "react";
import { PersonaIdentityContext } from "./persona-identity-context";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import { formatRelativeTime } from "@/lib/utils";
import { AgentAvatar } from "@/components/shared/agent-avatar";
import type { TaskActivityEntry } from "@/components/task/simple/types";
import { useTranslation } from "react-i18next";

type TaskActivityProps = {
  taskId: string;
  entries: TaskActivityEntry[];
};

function ActivityRow({ entry }: { entry: TaskActivityEntry }) {
  const { t } = useTranslation();
  const agentName = useAppStore((s) =>
    entry.actorType === "agent"
      ? (selectOfficeAgentProfiles(s).find((a) => a.id === entry.actorId)?.name ?? t("task:agent"))
      : "",
  );
  const persona = useContext(PersonaIdentityContext);
  const identity = persona?.id === entry.actorId ? persona : null;
  let actorName = t("task:system");
  if (entry.actorType === "user") actorName = t("task:you");
  else if (entry.actorType === "agent") actorName = identity?.name ?? agentName;

  return (
    <div className="flex items-start gap-3 px-0 py-2 text-sm">
      {entry.actorType === "agent" ? (
        <AgentAvatar name={actorName} icon={identity?.icon} size="sm" className="mt-0.5" />
      ) : (
        <div className="h-6 w-6 rounded-md bg-muted flex items-center justify-center shrink-0 mt-0.5">
          <span className="text-[10px] font-medium text-muted-foreground">
            {actorName.charAt(0).toUpperCase()}
          </span>
        </div>
      )}
      <div className="flex-1 min-w-0">
        <span className="font-medium">{actorName}</span>
        <span className="text-muted-foreground"> {entry.actionVerb} </span>
        {entry.targetName && <span className="font-medium">{entry.targetName}</span>}
      </div>
      <span className="text-xs text-muted-foreground shrink-0">
        {formatRelativeTime(entry.createdAt)}
      </span>
    </div>
  );
}

export function TaskActivity({ taskId, entries }: TaskActivityProps) {
  const { t } = useTranslation();
  void taskId;

  if (entries.length === 0) {
    return <p className="text-sm text-muted-foreground py-4">{t("task:noActivityYet")}</p>;
  }

  return (
    <div className="divide-y divide-border/50">
      {entries.map((entry) => (
        <ActivityRow key={entry.id} entry={entry} />
      ))}
    </div>
  );
}
