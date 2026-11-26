"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import TaskLink from "@/components/routing/task-link";
import { usageAmount } from "@/lib/api/domains/token-usage-amounts";
import {
  actorAttributionLabel,
  usageCalendarLabel,
  usageCurrencyLabel,
  usageProviderLabel,
  usageRowLabel,
  usageTaskDestination,
} from "./token-usage-metadata";
import {
  costBasisLabel,
  coverageLabelKey,
  formatCost,
  formatRate,
  formatTokens,
  type TokenUsageAggregate,
  type TokenUsageView,
} from "./token-usage-utils";

type RowProps = { row: TokenUsageAggregate; view: TokenUsageView };

export function UsageRowIdentity({ row, view, link = true }: RowProps & { link?: boolean }) {
  const { t } = useTranslation();
  const label = usageRowLabel(row, view, t);
  const destination = link ? usageTaskDestination(row, view) : null;
  return (
    <span className="flex min-w-0 items-center gap-2">
      {destination ? (
        <TaskLink
          {...destination}
          className="cursor-pointer truncate underline-offset-2 hover:underline"
        >
          {label}
        </TaskLink>
      ) : (
        <span className="truncate">{label}</span>
      )}
      {row.task_archived && row.task_available && (
        <Badge variant="outline">{t("stats:archivedTask")}</Badge>
      )}
    </span>
  );
}

export function UsageDetails({ row, view, showLink = false }: RowProps & { showLink?: boolean }) {
  const { t } = useTranslation();
  const unavailable = t("common:unavailable");
  const unknown = t("common:unknown");
  const fields = [
    [t("stats:models"), row.model || unknown],
    [t("stats:provider"), usageProviderLabel(row.provider, t)],
    [t("stats:agentClient"), row.agent_client || unknown],
    [t("stats:tokenUserAttribution"), actorAttributionLabel(row.token_source, t)],
    [t("stats:costUserAttribution"), actorAttributionLabel(row.cost_source, t)],
    [t("stats:tokenCollector"), row.token_source?.source || unknown],
    [t("stats:costCollector"), row.cost_source?.source || unknown],
    [t("stats:inputTokens"), formatTokens(usageAmount(row, "input_tokens"), unavailable)],
    [t("stats:outputTokens"), formatTokens(usageAmount(row, "output_tokens"), unavailable)],
    [t("stats:cacheReadTokens"), formatTokens(usageAmount(row, "cache_read_tokens"), unavailable)],
    [
      t("stats:cacheWriteTokens"),
      formatTokens(usageAmount(row, "cache_write_tokens"), unavailable),
    ],
    [t("stats:reasoningTokens"), formatTokens(usageAmount(row, "reasoning_tokens"), unavailable)],
    [t("stats:totalTokens"), formatTokens(usageAmount(row, "total_tokens"), unavailable)],
    [
      t("stats:sourceTotalTokens"),
      formatTokens(usageAmount(row, "source_total_tokens"), unavailable),
    ],
    [t("stats:totalTurns"), formatTokens(usageAmount(row, "turns"), unavailable)],
    [t("stats:cost"), formatCost(usageAmount(row, "cost_subcents"), row.currency, unavailable)],
    [t("stats:costBasis"), costBasisLabel(row.cost_basis, t)],
    [t("stats:currency"), usageCurrencyLabel(row.currency, t)],
    [t("stats:costCoverage"), t(coverageLabelKey(row.cost_coverage))],
    [
      t("stats:effectiveRate"),
      formatRate(row.effective_cost_per_million, row.currency, unavailable),
    ],
    [t("stats:calendar"), usageCalendarLabel(row, t)],
    [t("stats:coverage"), t(coverageLabelKey(row.stale ? "stale" : row.coverage))],
  ];
  const destination = showLink ? usageTaskDestination(row, view) : null;
  return (
    <div className="min-w-0 space-y-3 whitespace-normal" data-testid="token-usage-details">
      <dl className="grid min-w-0 grid-cols-2 gap-3 text-xs">
        {fields.map(([label, value]) => (
          <div key={label} className="min-w-0 [overflow-wrap:anywhere]">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
      {row.calendar_basis === "source" && (
        <p className="text-xs text-muted-foreground">{t("stats:calendarSourceNotice")}</p>
      )}
      {destination && (
        <TaskLink
          {...destination}
          className="inline-flex min-h-11 cursor-pointer items-center text-sm underline"
        >
          {usageRowLabel(row, view, t)}
        </TaskLink>
      )}
    </div>
  );
}

export function UsageDetailsDisclosure({ row, view }: RowProps) {
  const { t } = useTranslation();
  return (
    <details className="mt-1 max-w-xs">
      <summary
        className="flex min-h-7 cursor-pointer items-center text-xs text-muted-foreground [@media(pointer:coarse)]:min-h-11"
        aria-label={t("stats:usageDetailsFor", { label: usageRowLabel(row, view, t) })}
      >
        {t("stats:usageDetails")}
      </summary>
      <div className="min-w-64 py-2">
        <UsageDetails row={row} view={view} />
      </div>
    </details>
  );
}
