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

function actorInitial(actorType: string, actorId: string, actorName?: string): string {
  if (actorType === "system") return "SY";
  if (actorType === "agent") {
    const trimmed = (actorName || actorId).trim();
    return trimmed.slice(0, 2).toUpperCase() || "AG";
  }
  return "U";
}

function actorLabel(entry: ActivityEntry): string {
  if (entry.actorType === "system") return t("office:system");
  // Historical rows may have no resolved label, so retain the identifier.
  return entry.actorName || entry.actorId || entry.actorType;
}

function taskIdentifier(details: Record<string, unknown> | undefined): string | null {
  const id = details?.task_identifier;
  if (typeof id === "string" && id) return id;
  return null;
}

function taskRefNode(
  entry: ActivityEntry,
  details: Record<string, unknown> | undefined,
): React.ReactNode {
  const id = details?.task_id ?? entry.targetId;
  const identifier = entry.targetIdentifier ?? taskIdentifier(details);
  if (!id && !identifier) return null;
  const fallback = identifier || (typeof id === "string" ? id : "");
  let label = fallback;
  if (entry.targetName) {
    label = identifier ? `${identifier} ${entry.targetName}` : entry.targetName;
  }
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

function renderStaleCancellation(entry: ActivityEntry): React.ReactNode {
  const d = entry.details;
  const label = cancelReasonLabel(typeof d?.reason === "string" ? d.reason : "");
  return (
    <>
      <span className="text-muted-foreground"> {t("office:staleRunCancelled")}</span>
      {taskRefNode(entry, d)}
      {label && <span className="text-muted-foreground"> - {truncate(label)}</span>}
    </>
  );
}

function renderReassignmentCancellation(entry: ActivityEntry): React.ReactNode {
  return (
    <>
      <span className="text-muted-foreground"> {t("office:retryCancelledReassigned")}</span>
      {taskRefNode(entry, entry.details)}
    </>
  );
}

function renderRecoveryDispatch(entry: ActivityEntry): React.ReactNode {
  return (
    <>
      <span className="text-muted-foreground"> {t("office:unstartedTaskRecovered")}</span>
      {taskRefNode(entry, entry.details)}
    </>
  );
}

function renderTaskStatusChange(entry: ActivityEntry): React.ReactNode {
  const d = entry.details;
  // One key for the whole clause. The raw status resolves through the shared
  // label map without freezing the English word order.
  const status = taskStatusLabel(typeof d?.new_status === "string" ? d.new_status : "");
  return (
    <>
      <span className="text-muted-foreground">
        {" "}
        {status ? t("office:activityStatusChangedTo", { status }) : t("office:statusChanged")}
      </span>
      {taskRefNode(entry, d)}
    </>
  );
}

const SPECIAL_ACTION_RENDERERS: Record<string, (entry: ActivityEntry) => React.ReactNode> = {
  run_stale_cancelled: renderStaleCancellation,
  run_retry_cancelled: renderReassignmentCancellation,
  recovery_dispatch: renderRecoveryDispatch,
  task_status_changed: renderTaskStatusChange,
};

function renderFallbackAction(entry: ActivityEntry): React.ReactNode {
  // NOT localized deliberately: these are open-ended backend identifiers.
  const formatted = truncate(entry.action.replace(/[._]/g, " "));
  return (
    <>
      <span className="text-muted-foreground"> {formatted} </span>
      {entry.targetType && (
        <span className="font-medium">
          {entry.targetName || entry.targetType}
          {entry.targetIdentifier || entry.targetId
            ? ` ${entry.targetIdentifier || entry.targetId}`
            : ""}
        </span>
      )}
    </>
  );
}

function renderAction(entry: ActivityEntry): React.ReactNode {
  const renderer = SPECIAL_ACTION_RENDERERS[entry.action];
  if (renderer) return renderer(entry);

  return renderFallbackAction(entry);
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
        {actorInitial(entry.actorType, entry.actorId, entry.actorName)}
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
