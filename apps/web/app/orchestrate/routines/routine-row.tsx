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
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { Routine, AgentInstance } from "@/lib/state/slices/orchestrate/types";
import { timeAgo } from "../components/shared/time-ago";

type RoutineRowProps = {
  routine: Routine;
  agents: AgentInstance[];
  expanded: boolean;
  onToggle: (id: string, active: boolean) => void;
  onRunNow: (id: string) => void;
  onDelete: (id: string) => void;
  onClick: (id: string) => void;
};

export function RoutineRow({
  routine,
  agents,
  expanded,
  onToggle,
  onRunNow,
  onDelete,
  onClick,
}: RoutineRowProps) {
  const assignee = agents.find((a) => a.id === routine.assigneeAgentInstanceId);
  const isActive = routine.status === "active";
  const template = routine.taskTemplate as { title?: string; description?: string } | undefined;

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
          <p className="text-sm font-medium truncate">{routine.name}</p>
          <div className="flex items-center gap-2 mt-0.5 text-xs text-muted-foreground">
            {assignee && <span>{assignee.name}</span>}
            <span>{routine.lastRunAt ? timeAgo(routine.lastRunAt) : "Never run"}</span>
            <span className="capitalize">{routine.concurrencyPolicy.replace(/_/g, " ")}</span>
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
        <DropdownMenuItem className="cursor-pointer" onClick={(e) => e.stopPropagation()}>
          <IconPencil className="h-4 w-4 mr-2" /> Edit
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
  assignee: AgentInstance | undefined;
  template: { title?: string; description?: string } | undefined;
}) {
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
      <DetailField label="Concurrency" value={routine.concurrencyPolicy.replace(/_/g, " ")} />
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
