"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import {
  hasPluginTaskUsage,
  usePluginTaskStatus,
  usePluginTaskUsage,
} from "@/lib/plugins/host-queries";
import type {
  PluginWorkspaceTaskSurfaceProps,
  PluginWorkspaceTaskUsageProps,
} from "@kandev/plugin-sdk";

function TaskStatusContent({ query }: { query: ReturnType<typeof usePluginTaskStatus> }) {
  const { t } = useTranslation("plugins");
  const summary = query.data?.statusSummary;
  if (query.loading && !query.data) {
    return <p className="text-muted-foreground">{t("managedChatLoading")}</p>;
  }
  if (query.error && !query.data) {
    return (
      <p role="status" className="text-muted-foreground">
        {t("managedChatQueryFailed")}
      </p>
    );
  }
  if (!query.data) return <p className="text-muted-foreground">{t("managedChatNoTask")}</p>;
  return (
    <>
      <div className="flex min-w-0 items-center justify-between gap-2">
        <span className="truncate font-medium">{query.data.title}</span>
        <Badge variant="secondary">{t("managedChatTaskState", { state: query.data.state })}</Badge>
      </div>
      {summary?.foregroundActivity ? (
        <p className="text-muted-foreground">
          {t("managedChatActivity", { activity: summary.foregroundActivity })}
        </p>
      ) : null}
      {summary?.completionGate ? (
        <p className="text-muted-foreground">
          {t("managedChatCompletionProgress", {
            verified: summary.completionGate.verifiedCount,
            total: summary.completionGate.criteriaCount,
          })}
          {summary.completionGate.blocked ? ` · ${t("managedChatBlocked")}` : ""}
        </p>
      ) : null}
      {summary?.activeError ? (
        <p role="alert" className="break-words text-destructive">
          {summary.activeError.preview}
        </p>
      ) : null}
    </>
  );
}

// eslint-disable-next-line complexity -- The query's loading, error, empty, and partial-usage states stay in one projection.
function TaskUsageContent({ query }: { query: ReturnType<typeof usePluginTaskUsage> }) {
  const { t, i18n } = useTranslation("plugins");
  const usage = query.data;
  const hasUsage = usage ? hasPluginTaskUsage(usage) : false;
  const currency = new Intl.NumberFormat(i18n.resolvedLanguage ?? i18n.language, {
    style: "currency",
    currency: "USD",
  }).format((usage?.costSubcents ?? 0) / 10_000);
  const tokens = new Intl.NumberFormat(i18n.resolvedLanguage ?? i18n.language).format(
    usage?.tokensTotal ?? 0,
  );
  if (query.loading && !usage) {
    return <p className="text-muted-foreground">{t("managedChatLoading")}</p>;
  }
  if (query.error && !usage) {
    return (
      <p role="status" className="text-muted-foreground">
        {t("managedChatQueryFailed")}
      </p>
    );
  }
  if (!usage || !hasUsage) {
    return <p className="text-muted-foreground">{t("managedChatNoUsage")}</p>;
  }
  return (
    <>
      <p>{t("managedChatTokenTotal", { count: usage.tokensTotal, formattedCount: tokens })}</p>
      <p className="text-muted-foreground">{currency}</p>
      {usage.estimatedEventCount > 0 || usage.unpricedEventCount > 0 ? (
        <p className="text-muted-foreground">{t("managedChatCostIsPartial")}</p>
      ) : null}
      {!usage.outputTokensComplete ? (
        <p className="text-muted-foreground">{t("managedChatOutputTokensIncomplete")}</p>
      ) : null}
    </>
  );
}

export function WorkspaceTaskStatus({ taskId }: PluginWorkspaceTaskSurfaceProps) {
  const { t } = useTranslation("plugins");
  const query = usePluginTaskStatus(taskId);

  return (
    <Card data-testid="workspace-task-status" className="min-w-0">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">{t("managedChatTaskStatus")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        <TaskStatusContent query={query} />
      </CardContent>
    </Card>
  );
}

export function WorkspaceTaskUsage({ taskId, sessionId }: PluginWorkspaceTaskUsageProps) {
  const { t } = useTranslation("plugins");
  const query = usePluginTaskUsage(taskId, sessionId);

  return (
    <Card data-testid="workspace-task-usage" className="min-w-0">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">{t("managedChatUsage")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-1 text-sm">
        <TaskUsageContent query={query} />
      </CardContent>
    </Card>
  );
}
