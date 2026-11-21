import type { TFunction } from "i18next";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import Link from "@/components/routing/app-link";
import { linkToTask } from "@/lib/links";
import type { OverviewEvent } from "@/lib/types/control-overview";
import { circuitTitle, modelBlockSubject, occurredTime, relativeTime } from "./overview-format";
import { OverviewEventExtras } from "./overview-event-extras";
import { eventToneName, statusBadgeClass } from "./overview-status-colors";
import { ControlStatusPill } from "./control-status-pill";
import type { HistoryItem } from "./control-step-moves";

const EVENT_LABEL_KEYS: Record<OverviewEvent["kind"], string> = {
  task_created: "office:overviewEventTaskCreated",
  server_started: "office:overviewServerStarted",
  task_completed: "common:taskStateCompleted",
  session_failed: "common:sessionStateFailed",
  automation_run: "common:automation",
  model_blocked: "office:overviewEventModelBlocked",
  model_unblocked: "office:overviewEventModelUnblocked",
  pr_merged: "office:overviewEventPRMerged",
  automation_failed: "office:overviewEventAutomationFailed",
  owner_decision: "office:overviewEventOwnerDecision",
  step_move: "office:overviewEventStepMove",
  session_started: "sidebar:controlNative.sessionStarted",
  session_finished: "sidebar:controlNative.sessionFinished",
  run_idle_skipped: "sidebar:controlSummary.history.runIdleSkipped",
  run_cancelled_stale: "sidebar:controlSummary.history.runCancelledStale",
};

export function eventLabel(t: TFunction, kind: string) {
  const key = kind === "all" ? "office:overviewEventAllKinds" : EVENT_LABEL_KEYS[kind];
  return key ? t(key) : kind;
}

function eventHref(event: OverviewEvent): string | undefined {
  if (event.kind === "automation_run" && event.workspace_id && event.automation_id) {
    return `/settings/workspaces/${encodeURIComponent(event.workspace_id)}/automations/${encodeURIComponent(event.automation_id)}`;
  }
  if (event.task_id) {
    return linkToTask(
      event.task_id,
      event.session_id ? { sessionId: event.session_id } : undefined,
    );
  }
  return undefined;
}

/**
 * What a row is about when the record carries no title: an office run reads as
 * an automatic office run, anything else by its kind. A raw record id is never
 * the subject a reader is shown.
 */
function eventSubject(t: TFunction, event: OverviewEvent): string {
  const circuit = modelBlockSubject(event);
  if (circuit) return circuitTitle(t, circuit);
  if (event.title) return event.title;
  if (event.kind.startsWith("run_")) return t("sidebar:controlSummary.history.officeRun");
  return eventLabel(t, event.kind);
}

const ROW_CLASS =
  "flex flex-wrap items-center gap-3 px-4 py-2 text-sm [@media(pointer:coarse)]:min-h-11";

function EventTime({ at }: { at: string }) {
  return (
    <span
      className="shrink-0 whitespace-nowrap text-xs tabular-nums text-muted-foreground"
      title={relativeTime(at)}
      data-testid="overview-event-time"
    >
      {occurredTime(at)}
    </span>
  );
}

function WorkspaceName({ id, names }: { id?: string; names: Record<string, string> }) {
  if (!id) return null;
  return <span className="mr-1.5 text-xs text-muted-foreground">{names[id]}</span>;
}

export function OverviewEventRow({
  event,
  workspaceNames,
}: {
  event: OverviewEvent;
  workspaceNames: Record<string, string>;
}) {
  const { t } = useTranslation();
  const href = eventHref(event);
  const content = (
    <>
      <EventTime at={event.at} />
      <Badge
        variant="outline"
        className={`shrink-0 ${statusBadgeClass(eventToneName(event.kind))}`}
      >
        {eventLabel(t, event.kind)}
      </Badge>
      <span className="min-w-0 flex-1">
        <span className="block truncate">
          <WorkspaceName id={event.workspace_id} names={workspaceNames} />
          {eventSubject(t, event)}
          {event.detail && (
            <span
              className="ml-1.5 max-w-[40%] truncate font-mono text-[11px] text-muted-foreground"
              title={event.detail}
            >
              {event.detail}
            </span>
          )}
        </span>
        <OverviewEventExtras event={event} />
        {event.recorded_state && (
          <span className="block text-xs text-muted-foreground">
            {t("sidebar:controlNative.currentRecordedState")}: {event.recorded_state}
          </span>
        )}
      </span>
    </>
  );
  return href ? (
    <Link
      href={href}
      className={`${ROW_CLASS} cursor-pointer hover:bg-muted/50`}
      data-testid="overview-event"
    >
      {content}
    </Link>
  ) : (
    <div className={ROW_CLASS} data-testid="overview-event">
      {content}
    </div>
  );
}

/** A task's consecutive step moves as one line, expandable to each move. */
export function StepMoveRunRow({
  item,
  workspaceNames,
  repeating,
}: {
  item: Extract<HistoryItem, { type: "moves" }>;
  workspaceNames: Record<string, string>;
  /** The task crossed the repeat rule somewhere in the window. */
  repeating: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const newest = item.events[0];
  return (
    <div data-testid="control-history-move-run">
      <div className={ROW_CLASS}>
        <EventTime at={item.end} />
        {repeating ? (
          <ControlStatusPill state="repeat" />
        ) : (
          <Badge
            variant="outline"
            className={`shrink-0 ${statusBadgeClass(eventToneName("step_move"))}`}
          >
            {eventLabel(t, "step_move")}
          </Badge>
        )}
        <span className="min-w-0 flex-1 truncate">
          <WorkspaceName id={newest.workspace_id} names={workspaceNames} />
          <Link className="underline-offset-4 hover:underline" href={linkToTask(item.taskId)}>
            {newest.title || t("common:unknown")}
          </Link>
          <span className="ml-1.5 text-muted-foreground">
            {t("sidebar:controlSummary.history.moveRun", {
              moves: item.events.length,
              start: occurredTime(item.start),
              end: occurredTime(item.end),
            })}
          </span>
        </span>
        <button
          type="button"
          className="min-h-7 cursor-pointer text-xs text-primary [@media(pointer:coarse)]:min-h-11"
          aria-expanded={open}
          onClick={() => setOpen((value) => !value)}
        >
          {t(open ? "office:overviewCollapseDetails" : "office:overviewExpandDetails")}
        </button>
      </div>
      {open && (
        <ul className="flex flex-wrap gap-x-3 gap-y-1 px-4 pb-2 pl-16 font-mono text-xs text-muted-foreground">
          {item.events.map((event) => (
            <li key={event.id ?? event.at}>{occurredTime(event.at)}</li>
          ))}
        </ul>
      )}
    </div>
  );
}
