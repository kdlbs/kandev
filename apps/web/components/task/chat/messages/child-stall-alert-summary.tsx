"use client";

import { IconAlertTriangle, IconSubtask } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { linkToTask } from "@/lib/links";

/** One child-turn stalled alert carried in a parent message's metadata. */
export type ChildStallAlertDescriptor = {
  child_task_id: string;
  child_task_title?: string;
  turn_id?: string;
  step_name?: string;
  cause?: string;
};

const CAUSE_KEYS: Record<string, string> = {
  input_required: "chat:childStallCauseInputRequired",
  quota: "chat:childStallCauseQuota",
  execution_error: "chat:childStallCauseExecutionError",
  missing_completion_signal: "chat:childStallCauseMissingCompletionSignal",
};

const TURN_REFERENCE_LENGTH = 8;

function isDescriptor(value: unknown): value is ChildStallAlertDescriptor {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { child_task_id?: unknown }).child_task_id === "string"
  );
}

/** Returns the alerts carried by a child stall alert message, or an empty list. */
export function childStallAlertsFromMetadata(
  metadata: Record<string, unknown> | null | undefined,
): ChildStallAlertDescriptor[] {
  if (!metadata || metadata.child_stall_alert !== true) return [];
  const raw = metadata.child_stall_alerts;
  return Array.isArray(raw) ? raw.filter(isDescriptor) : [];
}

function ChildStallAlertRow({ alert }: { alert: ChildStallAlertDescriptor }) {
  const { t } = useTranslation();
  const causeKey = alert.cause ? CAUSE_KEYS[alert.cause] : undefined;
  const cause = causeKey ? t(causeKey) : (alert.cause ?? "");
  const title = alert.child_task_title || alert.child_task_id;
  return (
    <li className="flex min-w-0 flex-col gap-1" data-testid="child-stall-alert">
      <Link
        href={linkToTask(alert.child_task_id)}
        className="inline-flex min-w-0 max-w-full items-center gap-1.5 self-start rounded-full bg-purple-500/20 px-2.5 py-1 font-medium text-purple-300 hover:bg-purple-500/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring [@media(pointer:coarse)]:min-h-11"
        data-testid="child-stall-alert-link"
      >
        <IconSubtask className="shrink-0" size={14} aria-hidden="true" />
        <span className="min-w-0 break-words">{title}</span>
      </Link>
      <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-muted-foreground">
        {alert.step_name && <span>{t("chat:childStallAlertStep", { step: alert.step_name })}</span>}
        {cause && <span>{t("chat:childStallAlertCause", { cause })}</span>}
        {alert.turn_id && (
          <span>
            {t("chat:childStallAlertTurn", {
              turn: alert.turn_id.slice(0, TURN_REFERENCE_LENGTH),
            })}
          </span>
        )}
      </div>
    </li>
  );
}

/** Attributed summary rendered above a parent message that carries child stall alerts. */
export function ChildStallAlertSummary({ alerts }: { alerts: ChildStallAlertDescriptor[] }) {
  const { t } = useTranslation();
  if (alerts.length === 0) return null;
  return (
    <section
      aria-label={t("chat:childStallAlertsLabel")}
      data-testid="child-stall-alert-summary"
      className="mb-1 flex flex-col gap-2 rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs"
    >
      <div className="flex items-center gap-1.5 font-medium text-amber-400">
        <IconAlertTriangle size={14} aria-hidden="true" />
        <span>{t("chat:childStallAlertTitle")}</span>
      </div>
      <ul className="flex flex-col gap-2">
        {alerts.map((alert, index) => (
          <ChildStallAlertRow
            key={alert.turn_id ?? `${alert.child_task_id}-${index}`}
            alert={alert}
          />
        ))}
      </ul>
    </section>
  );
}
