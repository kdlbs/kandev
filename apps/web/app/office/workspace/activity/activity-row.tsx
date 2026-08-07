"use client";

import Link from "@/components/routing/app-link";
import type { ActivityEntry } from "@/lib/state/slices/office/types";
import { timeAgo } from "@/lib/utils/time";
import type { OfficeTaskStatus } from "@/lib/state/slices/office/types";
import { STATUS_LABEL_KEYS } from "../../lib/label-keys";
import { useTranslation } from "react-i18next";
// Module-level `t`, resolved at call time: `renderAction` and `actorLabel` are
// plain helpers called during the row's render, not components, so there is no
// hook to bind. The row re-renders on `languageChanged` through the
// `useTranslation()` below.
import { t } from "@/lib/i18n";

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale. The
// record keys are the wire cancellation reasons.
const CANCEL_REASON_LABEL_KEYS: Record<string, string> = {
  assignee_changed: "office:cancelReasonAssigneeChanged",
  task_terminal: "office:cancelReasonTaskTerminal",
  task_not_found: "office:cancelReasonTaskNotFound",
  review_participant_changed: "office:cancelReasonReviewParticipantChanged",
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
  if (entry.actorType === "system") return t("office:system");
  // An agent id or the raw actor type — identifiers, never copy.
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

/** `.replace(/_/g, " ")` keeps an unmapped wire reason visible rather than blank. */
function cancelReasonLabel(reason: string): string {
  if (!reason) return "";
  const key = CANCEL_REASON_LABEL_KEYS[reason];
  return key ? t(key) : reason.replace(/_/g, " ");
}

/** Same fallback rule for a task status that is not in the shared label map. */
function taskStatusLabel(raw: string): string {
  if (!raw) return "";
  const key = STATUS_LABEL_KEYS[raw as OfficeTaskStatus];
  return key ? t(key) : raw.replace(/_/g, " ");
}

function renderAction(entry: ActivityEntry): React.ReactNode {
  const d = entry.details;

  if (entry.action === "run_stale_cancelled") {
    const label = cancelReasonLabel(typeof d?.reason === "string" ? d.reason : "");
    return (
      <>
        <span className="text-muted-foreground"> {t("office:staleRunCancelled")}</span>
        {taskRefNode(d)}
        {label && <span className="text-muted-foreground"> — {truncate(label)}</span>}
      </>
    );
  }

  if (entry.action === "run_retry_cancelled") {
    return (
      <>
        <span className="text-muted-foreground"> {t("office:retryCancelledReassigned")}</span>
        {taskRefNode(d)}
      </>
    );
  }

  if (entry.action === "recovery_dispatch") {
    return (
      <>
        <span className="text-muted-foreground"> {t("office:unstartedTaskRecovered")}</span>
        {taskRefNode(d)}
      </>
    );
  }

  if (entry.action === "task_status_changed") {
    // One key for the whole clause. "status changed" + " to {status}" as two
    // fragments freezes the English order, and the status arrived as the raw
    // wire value; it now resolves through the shared status label map.
    const status = taskStatusLabel(typeof d?.new_status === "string" ? d.new_status : "");
    return (
      <>
        <span className="text-muted-foreground">
          {" "}
          {status ? t("office:activityStatusChangedTo", { status }) : t("office:statusChanged")}
        </span>
        {taskRefNode(d)}
      </>
    );
  }

  // NOT localized, deliberately: `entry.action` is an open-ended backend
  // activity identifier with no closed union on the wire, so a key map would
  // silently fall through for any action the backend adds. `targetType` below is
  // a wire value for the same reason.
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
  const { t } = useTranslation();
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
          {t("office:run")}
        </Link>
      )}
      <span className="text-xs text-muted-foreground shrink-0">{timeAgo(entry.createdAt)}</span>
    </div>
  );
}
