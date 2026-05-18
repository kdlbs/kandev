"use client";

import Link from "next/link";
import type { ActivityEntry } from "@/lib/state/slices/office/types";
import { timeAgo } from "../../components/shared/time-ago";

const CANCEL_REASON_LABEL: Record<string, string> = {
  assignee_changed: "assignee changed",
  task_terminal: "task completed",
  task_not_found: "task not found",
  review_participant_changed: "reviewer changed",
};

const MAX_DESCRIPTION_LENGTH = 80;

function actorInitial(actorType: string, actorId: string): string {
  if (actorType === "system") return "SY";
  if (actorType === "agent") {
    const trimmed = actorId.trim();
    return trimmed.slice(0, 2).toUpperCase() || "AG";
  }
  return "U";
}

function actorLabel(entry: ActivityEntry): string {
  if (entry.actorType === "system") return "System";
  return entry.actorId || entry.actorType;
}

function taskIdentifier(details: Record<string, unknown> | undefined): string | null {
  const id = details?.task_identifier;
  if (typeof id === "string" && id) return id;
  return null;
}

function taskRefNode(details: Record<string, unknown> | undefined): React.ReactNode {
  const id = details?.task_id;
  const identifier = taskIdentifier(details);
  if (!id && !identifier) return null;
  const label = identifier ?? (typeof id === "string" ? id : "");
  return <span className="font-bold"> {label}</span>;
}

function truncate(text: string): string {
  if (text.length <= MAX_DESCRIPTION_LENGTH) return text;
  return `${text.slice(0, MAX_DESCRIPTION_LENGTH)}…`;
}

function renderAction(entry: ActivityEntry): React.ReactNode {
  const d = entry.details;

  if (entry.action === "run_stale_cancelled") {
    const reason = typeof d?.reason === "string" ? d.reason : "";
    const label = CANCEL_REASON_LABEL[reason] ?? reason.replace(/_/g, " ");
    return (
      <>
        <span className="text-muted-foreground"> stale run cancelled</span>
        {taskRefNode(d)}
        {label && <span className="text-muted-foreground"> — {truncate(label)}</span>}
      </>
    );
  }

  if (entry.action === "run_retry_cancelled") {
    return (
      <>
        <span className="text-muted-foreground"> retry cancelled — reassigned</span>
        {taskRefNode(d)}
      </>
    );
  }

  if (entry.action === "recovery_dispatch") {
    return (
      <>
        <span className="text-muted-foreground"> unstarted task recovered</span>
        {taskRefNode(d)}
      </>
    );
  }

  if (entry.action === "task_status_changed") {
    const newStatus = typeof d?.new_status === "string" ? d.new_status.replace(/_/g, " ") : "";
    return (
      <>
        <span className="text-muted-foreground"> status changed</span>
        {newStatus && <span className="text-muted-foreground"> to {newStatus}</span>}
        {taskRefNode(d)}
      </>
    );
  }

  const formatted = truncate(entry.action.replace(/[._]/g, " "));
  return (
    <>
      <span className="text-muted-foreground"> {formatted} </span>
      {entry.targetType && (
        <span className="font-medium">
          {entry.targetType}
          {entry.targetId ? ` ${entry.targetId}` : ""}
        </span>
      )}
    </>
  );
}

function runHref(entry: ActivityEntry): string | null {
  if (!entry.runId) return null;
  const agentID = resolveAgentId(entry);
  if (!agentID) return null;
  return `/office/agents/${encodeURIComponent(agentID)}/runs/${encodeURIComponent(entry.runId)}`;
}

function resolveAgentId(entry: ActivityEntry): string | null {
  if (entry.actorType === "agent" && entry.actorId) return entry.actorId;
  const fallback = entry.details?.agent_id;
  return typeof fallback === "string" && fallback ? fallback : null;
}

type Props = {
  entry: ActivityEntry;
};

export function ActivityRow({ entry }: Props) {
  const href = runHref(entry);
  return (
    <div className="flex items-start gap-3 px-4 py-2.5 text-sm hover:bg-accent/50 transition-colors">
      <div className="h-6 w-6 rounded-full bg-muted flex items-center justify-center shrink-0 text-[10px] font-medium uppercase text-muted-foreground">
        {actorInitial(entry.actorType, entry.actorId)}
      </div>
      <div className="flex-1 min-w-0 truncate">
        <span className="font-medium">{actorLabel(entry)}</span>
        {renderAction(entry)}
      </div>
      {href && (
        <Link
          href={href}
          className="text-xs text-muted-foreground hover:text-foreground shrink-0 cursor-pointer"
        >
          Run
        </Link>
      )}
      <span className="text-xs text-muted-foreground shrink-0">{timeAgo(entry.createdAt)}</span>
    </div>
  );
}
