"use client";

import { formatDistanceToNow } from "date-fns";
import { IconExternalLink, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Avatar, AvatarFallback, AvatarImage } from "@kandev/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import type { JiraTicket, JiraStatusCategory } from "@/lib/types/jira";
import type { JiraTaskPreset } from "./presets";

function statusBadgeClass(category: JiraStatusCategory | undefined): string {
  switch (category) {
    case "done":
      return "bg-green-500/15 text-green-700 dark:text-green-400 border-green-500/30";
    case "indeterminate":
      return "bg-amber-500/15 text-amber-700 dark:text-amber-400 border-amber-500/30";
    case "new":
      return "bg-blue-500/15 text-blue-700 dark:text-blue-400 border-blue-500/30";
    default:
      return "";
  }
}

function formatRelative(iso: string | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return formatDistanceToNow(d, { addSuffix: true });
}

type TicketRowProps = {
  ticket: JiraTicket;
  presets: JiraTaskPreset[];
  onStartTask: (ticket: JiraTicket, preset: JiraTaskPreset) => void;
  onOpen?: (ticket: JiraTicket) => void;
};

function AssigneeCell({ ticket }: { ticket: JiraTicket }) {
  if (!ticket.assigneeName) {
    return <span className="text-xs text-muted-foreground">Unassigned</span>;
  }
  return (
    <div className="flex items-center gap-1.5 min-w-0">
      <Avatar size="sm" className="size-5">
        {ticket.assigneeAvatar && (
          <AvatarImage src={ticket.assigneeAvatar} alt={ticket.assigneeName} />
        )}
        <AvatarFallback className="text-[10px]">{ticket.assigneeName.charAt(0)}</AvatarFallback>
      </Avatar>
      <span className="text-xs text-muted-foreground truncate">{ticket.assigneeName}</span>
    </div>
  );
}

function StartTaskMenu({
  ticket,
  presets,
  onStartTask,
}: {
  ticket: JiraTicket;
  presets: JiraTaskPreset[];
  onStartTask: TicketRowProps["onStartTask"];
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="outline" className="cursor-pointer h-7 px-2 gap-1 text-xs">
          <IconPlus className="h-3.5 w-3.5" />
          Start task
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        {presets.map((p) => {
          const Icon = p.icon;
          return (
            <DropdownMenuItem
              key={p.id}
              onClick={() => onStartTask(ticket, p)}
              className="cursor-pointer"
            >
              <Icon className="h-4 w-4 mr-2" />
              <div className="flex flex-col">
                <span>{p.label}</span>
                <span className="text-[11px] text-muted-foreground">{p.hint}</span>
              </div>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function TicketRow({ ticket, presets, onStartTask, onOpen }: TicketRowProps) {
  const relative = formatRelative(ticket.updated);
  return (
    <div className="flex items-start gap-3 py-3 border-b last:border-b-0">
      <button
        type="button"
        onClick={() => onOpen?.(ticket)}
        className="flex-1 min-w-0 space-y-1 text-left cursor-pointer rounded -mx-2 px-2 py-1 hover:bg-muted/50 transition-colors"
        title="Open ticket details"
      >
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-mono">{ticket.key}</span>
          {ticket.issueType && <span>· {ticket.issueType}</span>}
          {ticket.priority && <span>· {ticket.priority}</span>}
        </div>
        <div className="text-sm font-medium truncate" title={ticket.summary}>
          {ticket.summary}
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          {ticket.statusName && (
            <Badge variant="outline" className={statusBadgeClass(ticket.statusCategory)}>
              {ticket.statusName}
            </Badge>
          )}
          <AssigneeCell ticket={ticket} />
          {relative && <span className="text-xs text-muted-foreground">· updated {relative}</span>}
        </div>
      </button>
      <div className="flex items-center gap-1 shrink-0">
        <Button asChild variant="ghost" size="icon-sm" className="cursor-pointer">
          <a href={ticket.url} target="_blank" rel="noreferrer" title="Open in Atlassian">
            <IconExternalLink className="h-3.5 w-3.5" />
          </a>
        </Button>
        <StartTaskMenu ticket={ticket} presets={presets} onStartTask={onStartTask} />
      </div>
    </div>
  );
}
