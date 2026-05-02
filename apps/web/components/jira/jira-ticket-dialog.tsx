"use client";

import { IconExternalLink, IconRefresh, IconPlus } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import type { JiraTicket, JiraTransition } from "@/lib/types/jira";
import {
  IconLabel,
  JiraErrorMessage,
  PersonCell,
  formatRelative,
  statusBadgeClass,
  useTicketState,
  type TicketState,
} from "./jira-ticket-common";
import type { JiraTaskPreset } from "./my-jira/presets";

type JiraTicketDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null | undefined;
  ticketKey: string | null | undefined;
  /** Shown while the full ticket is loading so the dialog doesn't flash empty. */
  initialTicket?: JiraTicket | null;
  /** Presets shown in the footer "Start task" menu. Required when onStartTask is set. */
  presets?: JiraTaskPreset[];
  /** If provided, shows a "Start task" menu in the dialog footer. */
  onStartTask?: (ticket: JiraTicket, preset: JiraTaskPreset) => void;
};

function dialogTitle(ticket: JiraTicket | null, ticketKey: string | null | undefined): string {
  return ticket?.summary ?? ticketKey ?? "Jira ticket";
}

function effectiveTicketKey(
  ticket: JiraTicket | null,
  ticketKey: string | null | undefined,
): string {
  return ticketKey ?? ticket?.key ?? "";
}

export function JiraTicketDialog({
  open,
  onOpenChange,
  workspaceId,
  ticketKey,
  initialTicket,
  presets,
  onStartTask,
}: JiraTicketDialogProps) {
  const enabled = open && !!workspaceId && !!ticketKey;
  const state = useTicketState(workspaceId ?? "", ticketKey ?? "", enabled);
  const ticket = state.ticket ?? initialTicket ?? null;
  const showFooter = ticket !== null && onStartTask !== undefined && presets !== undefined;

  const errorOnly = state.error !== null && ticket === null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="!max-w-[min(1280px,95vw)] w-[95vw] max-h-[90vh] overflow-hidden flex flex-col gap-0 p-0 sm:rounded-lg">
        <DialogTitle className="sr-only">{dialogTitle(ticket, ticketKey)}</DialogTitle>
        {errorOnly ? (
          <div className="flex-1 flex items-center justify-center px-8 py-16">
            <JiraErrorMessage error={state.error ?? ""} />
          </div>
        ) : (
          <>
            <TicketTopBar
              ticket={ticket}
              loading={state.loading}
              onRefresh={() => void state.load()}
            />
            <DialogBody
              ticket={ticket}
              state={state}
              ticketKey={effectiveTicketKey(ticket, ticketKey)}
            />
            {showFooter && (
              <TicketFooter ticket={ticket} presets={presets} onStartTask={onStartTask} />
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

function DialogBody({
  ticket,
  state,
  ticketKey,
}: {
  ticket: JiraTicket | null;
  state: TicketState;
  ticketKey: string;
}) {
  return (
    <div className="flex-1 overflow-y-auto">
      {state.error && ticket && (
        <div className="px-8 pt-4">
          <JiraErrorMessage error={state.error} compact />
        </div>
      )}
      {!ticket && state.loading && (
        <div className="text-sm text-muted-foreground py-16 text-center">Loading ticket…</div>
      )}
      {ticket && <TicketBody ticket={ticket} state={state} ticketKey={ticketKey} />}
    </div>
  );
}

function TicketFooter({
  ticket,
  presets,
  onStartTask,
}: {
  ticket: JiraTicket;
  presets: JiraTaskPreset[];
  onStartTask: (ticket: JiraTicket, preset: JiraTaskPreset) => void;
}) {
  return (
    <div className="flex items-center justify-end gap-2 px-6 py-3 border-t bg-muted/20 shrink-0">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" variant="default" className="cursor-pointer gap-1.5">
            <IconPlus className="h-4 w-4" />
            Start task
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-60">
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
    </div>
  );
}

type TopBarProps = {
  ticket: JiraTicket | null;
  loading: boolean;
  onRefresh: () => void;
};

function TicketTopBar({ ticket, loading, onRefresh }: TopBarProps) {
  return (
    <div className="flex items-center justify-end gap-1 pl-4 pr-12 py-2 border-b shrink-0">
      {ticket?.url && (
        <Button
          asChild
          variant="ghost"
          size="icon-sm"
          className="cursor-pointer"
          title="Open in Atlassian"
        >
          <a href={ticket.url} target="_blank" rel="noreferrer">
            <IconExternalLink className="h-4 w-4" />
          </a>
        </Button>
      )}
      <Button
        variant="ghost"
        size="icon-sm"
        className="cursor-pointer"
        onClick={onRefresh}
        disabled={loading}
        title="Refresh"
      >
        <IconRefresh className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
      </Button>
    </div>
  );
}

function TicketBody({
  ticket,
  state,
  ticketKey,
}: {
  ticket: JiraTicket;
  state: TicketState;
  ticketKey: string;
}) {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_340px] gap-8 px-8 py-6">
      <div className="min-w-0 space-y-6">
        <TicketHeading ticket={ticket} ticketKey={ticketKey} />
        <DescriptionSection description={ticket.description} />
      </div>
      <aside className="space-y-4">
        <StatusBlock
          ticket={ticket}
          transitions={ticket.transitions}
          pending={state.pendingTransition}
          onTransition={(id) => void state.handleTransition(id)}
        />
        <DetailsCard ticket={ticket} />
        <MetaFooter ticket={ticket} />
      </aside>
    </div>
  );
}

function TicketHeading({ ticket, ticketKey }: { ticket: JiraTicket; ticketKey: string }) {
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground font-mono">
        <span>{ticketKey}</span>
        {ticket.issueType && (
          <>
            <span>·</span>
            <IconLabel icon={ticket.issueTypeIcon} label={ticket.issueType} />
          </>
        )}
      </div>
      <h2 className="text-2xl font-semibold leading-tight">{ticket.summary}</h2>
    </div>
  );
}

function SectionHeading({ children }: { children: React.ReactNode }) {
  return <div className="text-sm font-semibold">{children}</div>;
}

function DescriptionSection({ description }: { description: string }) {
  return (
    <section className="space-y-2">
      <SectionHeading>Description</SectionHeading>
      <div className="rounded-md border bg-muted/30 px-4 py-3">
        {description ? (
          <div className="text-sm whitespace-pre-wrap leading-relaxed">{description}</div>
        ) : (
          <div className="text-sm text-muted-foreground italic">No description.</div>
        )}
      </div>
    </section>
  );
}

type StatusBlockProps = {
  ticket: JiraTicket;
  transitions: JiraTransition[] | null | undefined;
  pending: string | null;
  onTransition: (id: string) => void;
};

function StatusBlock({ ticket, transitions, pending, onTransition }: StatusBlockProps) {
  const list = transitions ?? [];
  const hasTransitions = list.length > 0;
  return (
    <section className="space-y-2">
      {ticket.statusName && (
        <div>
          <Badge
            variant="outline"
            className={`text-sm px-3 py-1 ${statusBadgeClass(ticket.statusCategory)}`}
          >
            {ticket.statusName}
          </Badge>
        </div>
      )}
      {hasTransitions && (
        <div className="flex flex-wrap gap-1.5 pt-1">
          {list.map((t) => (
            <Button
              key={t.id}
              variant="outline"
              size="sm"
              className="cursor-pointer h-7 text-xs"
              disabled={pending !== null}
              onClick={() => onTransition(t.id)}
            >
              {pending === t.id ? "…" : `→ ${t.name}`}
            </Button>
          ))}
        </div>
      )}
    </section>
  );
}

function DetailsCard({ ticket }: { ticket: JiraTicket }) {
  return (
    <section className="rounded-md border bg-background overflow-hidden">
      <div className="px-4 py-3 border-b bg-muted/30">
        <SectionHeading>Details</SectionHeading>
      </div>
      <div className="px-4 py-3 space-y-3 text-sm">
        <DetailRow label="Assignee">
          <PersonCell name={ticket.assigneeName} avatar={ticket.assigneeAvatar} />
        </DetailRow>
        <DetailRow label="Reporter">
          <PersonCell name={ticket.reporterName} avatar={ticket.reporterAvatar} />
        </DetailRow>
        <DetailRow label="Priority">
          <IconLabel icon={ticket.priorityIcon} label={ticket.priority} />
        </DetailRow>
        <DetailRow label="Project">
          <span className="font-mono text-xs">{ticket.projectKey}</span>
        </DetailRow>
      </div>
    </section>
  );
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3">
      <span className="text-muted-foreground text-xs w-20 shrink-0 pt-0.5">{label}</span>
      <div className="min-w-0 flex items-center gap-1.5 flex-1">{children}</div>
    </div>
  );
}

function MetaFooter({ ticket }: { ticket: JiraTicket }) {
  const updated = formatRelative(ticket.updated);
  if (!updated) return null;
  return (
    <div className="text-xs text-muted-foreground px-1" title={ticket.updated}>
      Updated {updated}
    </div>
  );
}
