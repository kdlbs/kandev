"use client";

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useCoordinatorPhase3Effective } from "@/hooks/domains/settings/use-coordinator-phase3-effective";
import { useWakeRun } from "@/hooks/domains/coordinator/use-wake-run";
import { Skeleton } from "@kandev/ui/skeleton";
import type { RunRead } from "@/lib/api/domains/coordinator-autonomy-api";
import type { Message } from "@/lib/types/http";

/** The unattended-turn id a message carries, or null for an ordinary message. */
export function wakeTurnIdOf(metadata: Message["metadata"]): string | null {
  const value = (metadata as Record<string, unknown> | undefined)?.coordinator_wake_turn_id;
  return typeof value === "string" && value !== "" ? value : null;
}

function WakeRows({ run }: { run: RunRead }) {
  return (
    <ul className="mt-1 space-y-0.5 text-xs" data-testid="woken-by-list">
      {run.wakes.map((wake) => (
        <li key={wake.id} className="flex min-w-0 gap-1.5" data-testid="woken-by-row">
          <span className="shrink-0 text-muted-foreground">{wake.kind}</span>
          <span className="shrink-0 font-medium">{wake.task_identifier ?? wake.task_id}</span>
          <span className="min-w-0 truncate">{wake.task_title ?? ""}</span>
        </li>
      ))}
    </ul>
  );
}

type ShellProps = {
  header: string;
  denied?: string;
  detail: ReactNode;
  state: string;
};

function Shell({ header, denied, detail, state }: ShellProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <div
      className="w-full min-w-0 max-w-full overflow-hidden rounded-lg border bg-muted/30 px-3 py-2 text-sm"
      data-testid="woken-by-entry"
      data-state={state}
    >
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <span className="font-medium" data-testid="woken-by-header">
          {header}
        </span>
        {denied && (
          <span className="text-xs text-muted-foreground" data-testid="woken-by-denied">
            {denied}
          </span>
        )}
        <button
          type="button"
          className="min-h-11 min-w-11 cursor-pointer text-xs underline sm:min-h-0 sm:min-w-0"
          aria-expanded={open}
          onClick={() => setOpen((value) => !value)}
          data-testid="woken-by-toggle"
        >
          {open ? t("coordinator:wokenByHide") : t("coordinator:wokenByShow")}
        </button>
      </div>
      {open && detail}
    </div>
  );
}

function Entry({ taskId, turnId, body }: { taskId: string; turnId: string; body: string }) {
  const { t } = useTranslation();
  const entry = useWakeRun(taskId, turnId);
  if (entry.status === "loading") {
    return (
      <Skeleton
        className="h-9 w-full"
        data-testid="woken-by-entry"
        data-state="loading"
        aria-label={t("coordinator:wokenByLoading")}
      />
    );
  }
  if (entry.status === "unavailable") {
    return (
      <Shell
        state="unavailable"
        header={t("coordinator:wokenByUnknown")}
        detail={
          <p className="mt-1 whitespace-pre-wrap break-words text-xs" data-testid="woken-by-body">
            {body}
          </p>
        }
      />
    );
  }
  const run = entry.run;
  return (
    <Shell
      state="loaded"
      header={t("coordinator:wokenByEvents", { count: run.wake_count })}
      denied={
        run.denied_permissions > 0
          ? t("coordinator:wokenByDenied", { count: run.denied_permissions })
          : undefined
      }
      detail={<WakeRows run={run} />}
    />
  );
}

type Props = { comment: Message; turnId: string; fallback: ReactNode };

/** The "Woken by N events" entry, inert (the ordinary message) while phase 3 is not effective. */
export function WokenByEntry({ comment, turnId, fallback }: Props) {
  const phase3 = useCoordinatorPhase3Effective();
  if (!phase3) return <>{fallback}</>;
  return <Entry taskId={comment.task_id} turnId={turnId} body={comment.content} />;
}
