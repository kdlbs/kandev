"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { formatRelativeTime } from "@/lib/i18n/formats";
import { getTaskStateIcon } from "@/lib/ui/state-icons";
import { FAILED_TASK_STATUS } from "@/lib/threads/thread-session-status";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import {
  failedInboxOriginMarkerKey,
  hasResolvableFailedInboxReason,
} from "@/lib/failed-inbox/row-presentation";
import type { FailedInboxRow as FailedInboxRowData } from "@/lib/types/failed-inbox";

// The row addresses a task, never a session (design-01#Session-resolution):
// the open-task control needs only the task id.
function taskHrefForRow(row: FailedInboxRowData): string {
  return `/t/${row.task_id}`;
}

export function FailedInboxRow({ row }: { row: FailedInboxRowData }) {
  const { t } = useTranslation();
  const originMarkerKey = failedInboxOriginMarkerKey(row.origin);
  const reasonText = hasResolvableFailedInboxReason(row.reason)
    ? row.reason
    : t("failedInbox:reasonUnknown");
  // Date.parse (which formatRelativeTime's `new Date(...)` uses internally)
  // normalizes malformed or non-RFC3339 wire values -- e.g. "0" becomes 1970
  // and "2026-02-30" becomes March 2 -- into a plausible but wrong instant
  // instead of failing, so shape validation must happen first or the unknown-
  // time fallback below is silently bypassed.
  const relativeTime =
    row.failure_instant && parseStrictRfc3339Timestamp(row.failure_instant) !== null
      ? formatRelativeTime(row.failure_instant)
      : null;

  return (
    <div className="flex items-center gap-3 px-4 py-2.5" data-testid="failed-inbox-row">
      <span
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted"
        data-testid="failed-inbox-status-icon"
        role="img"
        aria-label={t(FAILED_TASK_STATUS.labelKey)}
      >
        {getTaskStateIcon("FAILED")}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{row.title}</span>
        <span className="block truncate text-xs text-muted-foreground">
          {!hasResolvableFailedInboxReason(row.reason) ? (
            <span data-testid="failed-inbox-reason-fallback">{reasonText}</span>
          ) : (
            reasonText
          )}
        </span>
        {originMarkerKey && (
          <span
            className="block truncate text-xs text-muted-foreground"
            data-testid="failed-inbox-origin-marker"
          >
            {t(originMarkerKey)}
          </span>
        )}
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">
        {relativeTime !== null && relativeTime !== "" ? (
          relativeTime
        ) : (
          <span data-testid="failed-inbox-unknown-time">{t("failedInbox:unknownFailureTime")}</span>
        )}
      </span>
      {/* Always visible at every viewport width -- AC-UI-INBOX-FAILED-001.18
          requires the open-task control be reachable where a fixed-width
          button would otherwise be withheld, and this row has no actions
          menu to fall back to like the Needs-you row does. */}
      <Button
        asChild
        variant="outline"
        size="sm"
        className="shrink-0 cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      >
        <Link href={taskHrefForRow(row)} data-testid="failed-inbox-open-task">
          {t("failedInbox:openTask")}
        </Link>
      </Button>
    </div>
  );
}
