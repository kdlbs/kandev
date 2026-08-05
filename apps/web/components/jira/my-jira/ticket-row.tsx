"use client";

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
import type { JiraTicket } from "@/lib/types/jira";
import { formatRelative, statusBadgeClass } from "@/components/jira/jira-ticket-common";
import type { JiraTaskPreset } from "./presets";
import { useTranslation } from "react-i18next";

type TicketRowProps = {
  ticket: JiraTicket;
  presets: JiraTaskPreset[];
  onStartTask: (ticket: JiraTicket, preset: JiraTaskPreset) => void;
  onOpen?: (ticket: JiraTicket) => void;
};

function AssigneeCell({ ticket }: { ticket: JiraTicket }) {
  const { t } = useTranslation();
  if (!ticket.assigneeName) {
    return <span className="text-xs text-muted-foreground">{t("jira:unassigned")}</span>;
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
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="outline" className="cursor-pointer h-7 px-2 gap-1 text-xs">
          <IconPlus className="h-3.5 w-3.5" />
          {t("jira:startTask")}
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
  const { t } = useTranslation();
  const relative = formatRelative(ticket.updated);
  return (
    <div className="flex items-start gap-3 py-3 border-b last:border-b-0">
      <button
        type="button"
        onClick={() => onOpen?.(ticket)}
        className="flex-1 min-w-0 space-y-1 text-left cursor-pointer rounded -mx-2 px-2 py-1 hover:bg-muted/50 transition-colors"
        title={t("jira:openTicketDetails")}
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
          {relative && (
            <span className="text-xs text-muted-foreground">
              {t("jira:updatedAgo", { time: relative })}
            </span>
          )}
        </div>
      </button>
      <div className="flex items-center gap-1 shrink-0">
        <Button asChild variant="ghost" size="icon-sm" className="cursor-pointer">
          <a href={ticket.url} target="_blank" rel="noreferrer" title={t("jira:openInAtlassian")}>
            <IconExternalLink className="h-3.5 w-3.5" />
          </a>
        </Button>
        <StartTaskMenu ticket={ticket} presets={presets} onStartTask={onStartTask} />
      </div>
    </div>
  );
}
