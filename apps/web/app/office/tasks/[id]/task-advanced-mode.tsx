"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { IconInfoCircle } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { StatusIcon } from "./status-icon";
import { useAdvancedSession } from "./use-advanced-session";
import { ExecutionIndicator } from "../../components/execution-indicator";
import { OfficeTopbarPortal } from "../../components/office-topbar-portal";
import type { Task } from "./types";

const OfficeDockviewLayout = dynamic(
  () => import("./office-dockview-layout").then((m) => ({ default: m.OfficeDockviewLayout })),
  { ssr: false },
);

type TaskAdvancedModeProps = {
  task: Task;
  onToggleSimple: () => void;
};

export function TaskAdvancedMode({ task, onToggleSimple }: TaskAdvancedModeProps) {
  const { sessionId, isSessionEnded } = useAdvancedSession(task.id);

  return (
    <div className="flex flex-col h-full">
      <OfficeTopbarPortal>
        <StatusIcon status={task.status} className="h-4 w-4" />
        <span className="text-xs font-mono text-muted-foreground">{task.identifier}</span>
        <span className="text-sm font-medium truncate">{task.title}</span>
        <span className="flex-1" />
        <ExecutionIndicator status={task.status} />
        <div className="flex items-center gap-2">
          <Label htmlFor="advanced-toggle" className="text-xs text-muted-foreground cursor-pointer">
            Advanced
          </Label>
          <Switch id="advanced-toggle" checked onCheckedChange={() => onToggleSimple()} />
        </div>
        <Link
          href={`/t/${task.id}`}
          className="text-xs text-muted-foreground underline-offset-2 hover:underline cursor-pointer whitespace-nowrap"
          data-testid="task-cross-link"
        >
          Open in advanced view
        </Link>
      </OfficeTopbarPortal>
      {isSessionEnded && (
        <div className="flex items-center gap-2 px-4 py-2 bg-muted border-b border-border shrink-0">
          <IconInfoCircle className="h-4 w-4 text-muted-foreground" />
          <span className="text-sm text-muted-foreground">Agent session ended</span>
        </div>
      )}
      <OfficeDockviewLayout taskId={task.id} sessionId={sessionId} />
    </div>
  );
}
