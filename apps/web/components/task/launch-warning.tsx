"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { formatRelative } from "@/lib/i18n/formats";

type LaunchWarningProps = {
  sessionId: string;
};

function reasonKey(reason: string): string {
  switch (reason) {
    case "config":
      return "task:launchWarningReasonConfig";
    case "timeout":
      return "task:launchWarningReasonTimeout";
    case "host_key":
      return "task:launchWarningReasonHostKey";
    case "auth":
      return "task:launchWarningReasonAuth";
    case "network":
      return "task:launchWarningReasonNetwork";
    default:
      return "task:launchWarningReasonUnknown";
  }
}

/**
 * Renders the session.launch.warning event verbatim at the point the user
 * initiated the launch — never re-derives reachability from the settings
 * store slice, and never blocks or confirms the launch it describes.
 */
export function LaunchWarning({ sessionId }: LaunchWarningProps) {
  const { t } = useTranslation();
  const entry = useAppStore((state) => state.launchWarning.bySessionId[sessionId]);

  if (!entry) return null;

  return (
    <div
      className="flex min-w-0 gap-3 border-b border-border/50 py-3"
      data-testid="launch-warning"
      role="status"
    >
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-amber-500/10 text-amber-600 dark:text-amber-400">
        <IconAlertTriangle className="h-4 w-4" aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1 space-y-1 text-sm">
        <p className="font-medium">{t("task:launchWarningTitle")}</p>
        {entry.host ? (
          <p className="break-words text-muted-foreground">
            {t("task:launchWarningHost", { host: entry.host })}
          </p>
        ) : null}
        <p data-testid="launch-warning-last-success" className="text-muted-foreground">
          {entry.lastSuccessAt
            ? t("task:launchWarningLastSuccess", { age: formatRelative(entry.lastSuccessAt) })
            : t("task:launchWarningNeverSucceeded")}
        </p>
        <p data-testid="launch-warning-reason" className="text-muted-foreground">
          {t(reasonKey(entry.reason))}
        </p>
      </div>
    </div>
  );
}
