"use client";

import dynamic from "@/lib/routing/client-dynamic";
import Link from "@/components/routing/app-link";
import { IconInfoCircle } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { StatusIcon } from "./status-icon";
import { useAdvancedSession } from "./use-advanced-session";
import { ExecutionIndicator } from "../../components/execution-indicator";
import { useOfficeTopbar } from "../../components/office-topbar-context";
import type { Task } from "./types";
import { useTranslation } from "react-i18next";

const OfficeDockviewLayout = dynamic(
  () => import("./office-dockview-layout").then((m) => ({ default: m.OfficeDockviewLayout })),
  { ssr: false },
);

type TaskAdvancedModeProps = {
  task: Task;
  onToggleSimple: () => void;
};

export function TaskAdvancedMode({ task, onToggleSimple }: TaskAdvancedModeProps) {
  const { t } = useTranslation();
  const { sessionId, isSessionEnded } = useAdvancedSession(task.id);

  useOfficeTopbar({
    title: task.title,
    icon: <StatusIcon status={task.status} className="h-4 w-4" />,
    titleSlot: (
      <>
        <span className="text-xs font-mono text-muted-foreground">{task.identifier}</span>
        <span className="truncate text-sm font-medium">{task.title}</span>
      </>
    ),
    parents: [{ label: t("office:tasks"), href: "/office/tasks" }],
    actions: (
      <>
        <ExecutionIndicator status={task.rawStatus ?? task.status} />
        <div className="flex items-center gap-2">
          <Label htmlFor="advanced-toggle" className="text-xs text-muted-foreground cursor-pointer">
            {t("office:advanced")}
          </Label>
          <Switch id="advanced-toggle" checked onCheckedChange={() => onToggleSimple()} />
        </div>
        <Link
          href={`/t/${task.id}`}
          className="text-xs text-muted-foreground underline-offset-2 hover:underline cursor-pointer whitespace-nowrap"
          data-testid="task-cross-link"
        >
          {t("office:openInAdvancedView")}
        </Link>
      </>
    ),
  });

  return (
    <div className="flex flex-col h-full">
      {isSessionEnded && (
        <div className="flex items-center gap-2 px-4 py-2 bg-muted border-b border-border shrink-0">
          <IconInfoCircle className="h-4 w-4 text-muted-foreground" />
          <span className="text-sm text-muted-foreground">{t("office:agentSessionEnded")}</span>
        </div>
      )}
      <OfficeDockviewLayout taskId={task.id} sessionId={sessionId} task={task} />
    </div>
  );
}
