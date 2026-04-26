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
import { IconDots, IconPlayerPlay, IconTrash, IconPencil } from "@tabler/icons-react";
import type { Routine, AgentInstance } from "@/lib/state/slices/orchestrate/types";
import { timeAgo } from "../components/shared/time-ago";

type RoutineRowProps = {
  routine: Routine;
  agents: AgentInstance[];
  onToggle: (id: string, active: boolean) => void;
  onRunNow: (id: string) => void;
  onDelete: (id: string) => void;
  onClick: (id: string) => void;
};

export function RoutineRow({
  routine,
  agents,
  onToggle,
  onRunNow,
  onDelete,
  onClick,
}: RoutineRowProps) {
  const assignee = agents.find((a) => a.id === routine.assigneeAgentInstanceId);
  const isActive = routine.status === "active";

  return (
    <div
      className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent/50 transition-colors cursor-pointer"
      onClick={() => onClick(routine.id)}
    >
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
      <DropdownMenu>
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
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            className="cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onRunNow(routine.id);
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
              onDelete(routine.id);
            }}
          >
            <IconTrash className="h-4 w-4 mr-2" /> Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
