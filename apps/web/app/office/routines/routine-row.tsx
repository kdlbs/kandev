"use client";

import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import {
  IconDots,
  IconPlayerPlay,
  IconTrash,
  IconPencil,
  IconChevronDown,
} from "@tabler/icons-react";
import Link from "next/link";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { Routine, AgentProfile, RoutineTrigger } from "@/lib/state/slices/office/types";
import { timeAgo } from "../components/shared/time-ago";

type RoutineRowProps = {
  routine: Routine;
  agents: AgentProfile[];
  triggers: RoutineTrigger[];
  expanded: boolean;
  onToggle: (id: string, active: boolean) => void;
  onRunNow: (id: string) => void;
  onDelete: (id: string) => void;
  onClick: (id: string) => void;
};

// nextFireText returns a human "next fire in <relative>" string for the
// closest cron trigger, or "" when no cron trigger has a known next
// fire (manual routines, disabled triggers, fresh-create with no
// next_run_at yet). Empty string lets the caller skip rendering.
function nextFireText(triggers: RoutineTrigger[]): string {
  const cron = triggers
    .filter((t) => t.kind === "cron" && t.enabled && t.nextRunAt)
    .map((t) => new Date(t.nextRunAt as string).getTime())
    .filter((ms) => !Number.isNaN(ms))
    .sort((a, b) => a - b);
  if (cron.length === 0) return "";
  const ms = cron[0] - Date.now();
  if (ms <= 0) return "fires now";
  if (ms < 60_000) return "<1m";
  if (ms < 3_600_000) return `${Math.round(ms / 60_000)}m`;
  if (ms < 86_400_000) return `${Math.round(ms / 3_600_000)}h`;
  return `${Math.round(ms / 86_400_000)}d`;
}

export function RoutineRow({
  routine,
  agents,
  triggers,
  expanded,
  onToggle,
  onRunNow,
  onDelete,
  onClick,
}: RoutineRowProps) {
  // The API may return snake_case fields (assignee_agent_profile_id, concurrency_policy)
  // before any mapping layer converts them. Use both camelCase and snake_case lookups.
  const routineRaw = routine as unknown as Record<string, unknown>;
  const assigneeId =
    routine.assigneeAgentProfileId ?? (routineRaw.assignee_agent_profile_id as string | undefined);
  const concurrencyPolicy =
    routine.concurrencyPolicy ?? (routineRaw.concurrency_policy as string | undefined) ?? "";
  const assignee = agents.find((a) => a.id === assigneeId);
  const isActive = routine.status === "active";
  const template = routine.taskTemplate as { title?: string; description?: string } | undefined;
  const cronTrigger = triggers.find((t) => t.kind === "cron");
  const nextFire = nextFireText(triggers);

  return (
    <div>
      <div
        className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent/50 transition-colors cursor-pointer"
        onClick={() => onClick(routine.id)}
      >
        <IconChevronDown
          className={`h-4 w-4 text-muted-foreground shrink-0 transition-transform ${expanded ? "" : "-rotate-90"}`}
        />
        <div className="flex-1 min-w-0">
          <Link
            href={`/office/routines/${routine.id}`}
            className="text-sm font-medium truncate cursor-pointer hover:underline"
            onClick={(e) => e.stopPropagation()}
          >
            {routine.name}
          </Link>
          <div className="flex items-center gap-2 mt-0.5 text-xs text-muted-foreground">
            {assignee && <span>{assignee.name}</span>}
            {cronTrigger?.cronExpression && (
              <span className="font-mono">{cronTrigger.cronExpression}</span>
            )}
            {nextFire && <span>next in {nextFire}</span>}
            <span>{routine.lastRunAt ? timeAgo(routine.lastRunAt) : "Never run"}</span>
            <span className="capitalize">{concurrencyPolicy.replace(/_/g, " ")}</span>
          </div>
        </div>
        <Badge variant={isActive ? "default" : "secondary"}>{isActive ? "On" : "Off"}</Badge>
        <Switch
          checked={isActive}
          onCheckedChange={(checked) => {
            onToggle(routine.id, checked);
          }}
          onClick={(e) => e.stopPropagation()}
          className="cursor-pointer"
        />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              asChild
              variant="ghost"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              onClick={(e) => e.stopPropagation()}
            >
              <Link href={`/office/routines/${routine.id}`}>
                <IconPencil className="h-4 w-4" />
              </Link>
            </Button>
          </TooltipTrigger>
          <TooltipContent>Edit routine</TooltipContent>
        </Tooltip>
        <RoutineActions
          onRunNow={() => onRunNow(routine.id)}
          onDelete={() => onDelete(routine.id)}
        />
      </div>
      {expanded && (
        <RoutineExpandedDetail routine={routine} assignee={assignee} template={template} />
      )}
    </div>
  );
}

function RoutineActions({ onRunNow, onDelete }: { onRunNow: () => void; onDelete: () => void }) {
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              onClick={(e) => e.stopPropagation()}
            >
              <IconDots className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>Actions</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          className="cursor-pointer"
          onClick={(e) => {
            e.stopPropagation();
            onRunNow();
          }}
        >
          <IconPlayerPlay className="h-4 w-4 mr-2" /> Run Now
        </DropdownMenuItem>
        <DropdownMenuItem
          className="text-red-600 cursor-pointer"
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
        >
          <IconTrash className="h-4 w-4 mr-2" /> Delete
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function RoutineExpandedDetail({
  routine,
  assignee,
  template,
}: {
  routine: Routine;
  assignee: AgentProfile | undefined;
  template: { title?: string; description?: string } | undefined;
}) {
  const routineRaw = routine as unknown as Record<string, unknown>;
  const concurrencyPolicy =
    routine.concurrencyPolicy ?? (routineRaw.concurrency_policy as string | undefined) ?? "";
  return (
    <div className="px-4 pb-3 pt-1 ml-7 border-t border-border/50 space-y-2 text-sm">
      {routine.description && <DetailField label="Description" value={routine.description} />}
      {template?.title && <DetailField label="Task title" value={template.title} />}
      {template?.description && (
        <DetailField label="Task description" value={template.description} />
      )}
      <DetailField label="Assignee" value={assignee?.name ?? "Unassigned"} />
      <DetailField
        label="Last run"
        value={routine.lastRunAt ? timeAgo(routine.lastRunAt) : "Never"}
      />
      <DetailField label="Concurrency" value={concurrencyPolicy.replace(/_/g, " ")} />
      {routine.variables && Object.keys(routine.variables).length > 0 && (
        <div>
          <span className="text-xs font-medium text-muted-foreground">Variables</span>
          <div className="mt-1 space-y-0.5">
            {Object.entries(routine.variables).map(([key, val]) => (
              <div key={key} className="text-xs font-mono text-muted-foreground">
                {key}: {String(val)}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function DetailField({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-2">
      <span className="text-xs font-medium text-muted-foreground w-28 shrink-0">{label}</span>
      <span className="text-xs">{value}</span>
    </div>
  );
}
