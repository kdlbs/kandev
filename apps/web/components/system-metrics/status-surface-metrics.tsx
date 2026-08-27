"use client";

import {
  IconActivity,
  IconCpu,
  IconDatabase,
  IconDisc,
  IconFlame,
  IconGauge,
  IconServer,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { Locale } from "date-fns";
import type { TFunction } from "i18next";
import { formatTimeDistance, useDateLocale } from "@/lib/i18n/date-locale";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useSystemMetricsSubscription } from "@/hooks/use-system-metrics-subscription";
import type { SystemMetricSample, SystemMetricsSource } from "@/lib/types/system";

type StatusSurfaceMetricsProps = {
  presentation: "bar" | "mobile-drawer";
  density: "full" | "compact";
  drawerOpen: boolean;
  iconSize?: "size-3.5" | "size-4";
};

export function StatusSurfaceMetrics({
  presentation,
  density,
  drawerOpen,
  iconSize = "size-3.5",
}: StatusSurfaceMetricsProps) {
  const { t } = useTranslation();
  // Wire/storage name stays stable for existing user settings and API payloads.
  const enabled = useAppStore((state) => state.userSettings.systemMetricsDisplay.showInTopbar);
  const simplified = useAppStore((state) => state.userSettings.systemMetricsDisplay.simplified);
  const snapshot = useAppStore((state) => state.system.metrics);
  const { isMobile } = useResponsiveBreakpoint();
  const shouldSubscribe = enabled && (!isMobile || drawerOpen);
  useSystemMetricsSubscription(shouldSubscribe);

  if (!enabled || (isMobile && !drawerOpen)) return null;
  if (!snapshot) return null;

  const host = snapshot.sources.find((source) => source.kind === "backend");
  if (presentation === "mobile-drawer") {
    return (
      <section
        data-testid="app-status-metrics"
        className="w-full min-w-0 space-y-2 py-0.5"
        aria-label={t("system:systemMetrics")}
      >
        <h3 className="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
          {t("system:systemMetrics")}
        </h3>
        {!host ? (
          <EmptyMetrics drawer iconSize={iconSize} />
        ) : (
          <DrawerSourceMetrics
            source={host}
            updatedAt={snapshot?.timestamp}
            simplified={simplified}
            iconSize={iconSize}
          />
        )}
      </section>
    );
  }

  return (
    <div
      data-testid="app-status-metrics"
      className="flex h-full max-w-[52vw] items-center overflow-hidden leading-none text-current"
      aria-label={t("system:systemMetrics")}
    >
      {!host ? (
        <EmptyMetrics iconSize={iconSize} />
      ) : (
        <BarSourceMetrics
          source={host}
          updatedAt={snapshot?.timestamp}
          showSourceLabel={density === "full"}
          metricLimit={density === "compact" ? 2 : 4}
          simplified={simplified}
          iconSize={iconSize}
        />
      )}
    </div>
  );
}

function EmptyMetrics({ drawer = false, iconSize }: { drawer?: boolean; iconSize: string }) {
  const { t } = useTranslation();
  return (
    <div
      className={
        drawer
          ? "flex min-h-11 items-center gap-2 rounded-md px-3 text-sm text-muted-foreground"
          : "flex h-full items-center gap-2 text-[11px] text-current opacity-70"
      }
    >
      <IconActivity className={iconSize} />
      <span>{t("system:metricsUnavailable")}</span>
    </div>
  );
}

function BarSourceMetrics({
  source,
  updatedAt,
  showSourceLabel,
  metricLimit,
  simplified,
  iconSize,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  showSourceLabel: boolean;
  metricLimit: number;
  simplified: boolean;
  iconSize: string;
}) {
  return (
    <div className="flex h-full max-w-[360px] items-center gap-3 overflow-hidden text-[11px]">
      {!simplified ? (
        <SourceBadge
          source={source}
          updatedAt={updatedAt}
          showLabel={showSourceLabel}
          iconSize={iconSize}
        />
      ) : null}
      <MetricValues
        source={source}
        updatedAt={updatedAt}
        limit={metricLimit}
        layout="inline"
        simplified={simplified}
        iconSize={iconSize}
      />
    </div>
  );
}

function DrawerSourceMetrics({
  source,
  updatedAt,
  simplified,
  iconSize,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  simplified: boolean;
  iconSize: string;
}) {
  return (
    <div className="flex min-h-11 w-full min-w-0 flex-col items-stretch gap-1 px-0 text-sm">
      {!simplified ? (
        <div className="flex min-h-11 w-full min-w-0 items-center">
          <SourceBadge source={source} updatedAt={updatedAt} showLabel iconSize={iconSize} />
        </div>
      ) : null}
      <MetricValues
        source={source}
        updatedAt={updatedAt}
        layout="grid"
        simplified={simplified}
        iconSize={iconSize}
      />
    </div>
  );
}

function SourceBadge({
  source,
  updatedAt,
  showLabel,
  iconSize,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  showLabel: boolean;
  iconSize: string;
}) {
  const { t } = useTranslation();
  const locale = useDateLocale();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="flex shrink-0 items-center gap-1.5 text-current"
          aria-label={t("system:hostMetrics")}
        >
          <IconServer className={iconSize} stroke={1.6} />
          {showLabel ? <span className="max-w-20 truncate">{t("system:host")}</span> : null}
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <div className="space-y-1">
          <div className="font-medium">{t("system:host")}</div>
          <div className="text-xs text-muted-foreground">{source.label}</div>
          <div className="text-xs text-muted-foreground">
            {lastUpdatedText(t, locale, updatedAt)}
          </div>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

function MetricValues({
  source,
  updatedAt,
  limit,
  layout,
  simplified,
  iconSize,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  limit?: number;
  layout: "inline" | "grid";
  simplified: boolean;
  iconSize: string;
}) {
  const metrics = limit === undefined ? source.metrics : source.metrics.slice(0, limit);
  if (metrics.length === 0) return <span className="text-muted-foreground">-</span>;
  const values = metrics.map((metric) => (
    <MetricValue
      key={metric.id}
      metric={metric}
      source={source}
      updatedAt={updatedAt}
      layout={layout}
      simplified={simplified}
      iconSize={iconSize}
    />
  ));
  if (layout === "grid") {
    return (
      // Keep detailed cells compact enough for two columns in the Pixel 5 drawer.
      <div className="grid w-full min-w-0 grid-cols-[repeat(auto-fit,minmax(8rem,1fr))] gap-x-3 gap-y-1">
        {values}
      </div>
    );
  }
  return <span className="flex min-w-0 items-center gap-3 overflow-hidden">{values}</span>;
}

function MetricValue({
  metric,
  source,
  updatedAt,
  layout,
  simplified,
  iconSize,
}: {
  metric: SystemMetricSample;
  source: SystemMetricsSource;
  updatedAt?: string;
  layout: "inline" | "grid";
  simplified: boolean;
  iconSize: string;
}) {
  const { t } = useTranslation();
  const locale = useDateLocale();
  const help = metric.id === "io_load" ? t("system:ioLoadHelp") : null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={`${layout === "grid" ? "flex min-h-11 min-w-0 w-full" : "inline-flex shrink-0"} items-center gap-1.5 tabular-nums ${metricColor(metric)}`}
          aria-label={`${metricLabel(t, metric.id)} ${formatMetric(metric)}`}
        >
          {metricIcon(metric.id, iconSize)}
          {!simplified ? <MetricMeter metric={metric} /> : null}
          <span className="font-medium tracking-[-0.015em] [font-family:var(--font-geist-mono)]">
            {formatMetric(metric)}
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <div className="space-y-1">
          <div className="font-medium">{metricLabel(t, metric.id)}</div>
          {help ? <div className="max-w-72 text-xs text-muted-foreground">{help}</div> : null}
          <div className="text-xs text-muted-foreground">
            {t("system:hostWithLabel", { label: source.label })}
          </div>
          <div className="text-xs tabular-nums">{formatMetric(metric)}</div>
          {metric.error ? (
            <div className="text-xs text-muted-foreground">{metric.error}</div>
          ) : null}
          <div className="text-xs text-muted-foreground">
            {lastUpdatedText(t, locale, updatedAt)}
          </div>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

// The map keys are wire metric ids and the `?? id` fallback returns an
// untranslated id on purpose — both are protocol. Only the values are copy.
function metricLabel(t: TFunction, id: string) {
  return (
    {
      cpu_percent: t("system:metricCpuPercent"),
      memory_percent: t("system:metricMemoryPercent"),
      disk_percent: t("system:metricDiskPercent"),
      cpu_temp: t("system:metricCpuTemp"),
      io_load: t("system:metricIoLoad"),
    }[id] ?? id
  );
}

function metricIcon(id: string, iconSize: string) {
  const Icon =
    {
      cpu_percent: IconCpu,
      memory_percent: IconDatabase,
      disk_percent: IconDisc,
      cpu_temp: IconFlame,
      io_load: IconGauge,
    }[id] ?? IconActivity;
  return <Icon className={`${iconSize} opacity-80`} stroke={1.6} />;
}

function MetricMeter({ metric }: { metric: SystemMetricSample }) {
  if (metric.unit !== "%" || typeof metric.value !== "number") return null;
  const width = `${Math.max(0, Math.min(100, metric.value))}%`;
  return (
    <span
      data-testid="system-metric-meter"
      className="h-1 w-7 overflow-hidden rounded-full bg-muted-foreground/20"
      aria-hidden="true"
    >
      <span className="block h-full rounded-full bg-current opacity-65" style={{ width }} />
    </span>
  );
}

function formatMetric(metric: SystemMetricSample) {
  if (typeof metric.value !== "number") return "-";
  const value = metric.unit === "%" ? Math.round(metric.value) : Math.round(metric.value * 10) / 10;
  return `${value}${metric.unit ?? ""}`;
}

function metricColor(metric: SystemMetricSample) {
  if (!metric.available) return "text-current opacity-50";
  const thresholdValue = metric.unit === "%" || metric.id === "cpu_temp" ? metric.value : null;
  if (typeof thresholdValue !== "number") return "text-current";
  if (thresholdValue > 95) return "text-destructive";
  if (thresholdValue >= 80) return "text-yellow-500 dark:text-yellow-400";
  return "text-current";
}

// `relative` is interpolated into a translated sentence, so an unlocalized
// date-fns distance would leave "about 1 hour ago" sitting inside Portuguese
// prose. Passing the value through `values` is already the right shape; it just
// needs the active locale to reach the formatter.
function lastUpdatedText(t: TFunction, locale: Locale, updatedAt?: string) {
  const relative = formatTimeDistance(updatedAt, locale);
  if (!relative) return t("system:lastUpdateUnknown");
  return t("system:updatedRelative", { relative });
}
