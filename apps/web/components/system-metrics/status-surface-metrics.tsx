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
import { formatDistanceToNow } from "date-fns";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useSystemMetricsSubscription } from "@/hooks/use-system-metrics-subscription";
import type { SystemMetricSample, SystemMetricsSource } from "@/lib/types/system";

type StatusSurfaceMetricsProps = {
  presentation: "bar" | "mobile-drawer";
  density: "full" | "compact";
  drawerOpen: boolean;
};

export function StatusSurfaceMetrics({
  presentation,
  density,
  drawerOpen,
}: StatusSurfaceMetricsProps) {
  // Wire/storage name stays stable for existing user settings and API payloads.
  const enabled = useAppStore((state) => state.userSettings.systemMetricsDisplay.showInTopbar);
  const simplified = useAppStore((state) => state.userSettings.systemMetricsDisplay.simplified);
  const snapshot = useAppStore((state) => state.system.metrics);
  const { isMobile } = useResponsiveBreakpoint();
  const shouldSubscribe = enabled && (!isMobile || drawerOpen);
  useSystemMetricsSubscription(shouldSubscribe);

  if (!enabled || (isMobile && !drawerOpen)) return null;

  const host = snapshot?.sources.find((source) => source.kind === "backend");
  if (presentation === "mobile-drawer") {
    return (
      <section data-testid="app-status-metrics" className="space-y-1" aria-label="System metrics">
        <h3 className="px-1 text-sm font-medium">System metrics</h3>
        {!host ? (
          <EmptyMetrics drawer />
        ) : (
          <DrawerSourceMetrics
            source={host}
            updatedAt={snapshot?.timestamp}
            simplified={simplified}
          />
        )}
      </section>
    );
  }

  return (
    <div
      data-testid="app-status-metrics"
      className="flex h-full max-w-[52vw] items-center overflow-hidden leading-none text-current"
      aria-label="System metrics"
    >
      {!host ? (
        <EmptyMetrics />
      ) : (
        <BarSourceMetrics
          source={host}
          updatedAt={snapshot?.timestamp}
          showSourceLabel={density === "full"}
          metricLimit={density === "compact" ? 2 : 4}
          simplified={simplified}
        />
      )}
    </div>
  );
}

function EmptyMetrics({ drawer = false }: { drawer?: boolean }) {
  return (
    <div
      className={
        drawer
          ? "flex min-h-11 items-center gap-2 rounded-md px-3 text-sm text-muted-foreground"
          : "flex h-full items-center gap-2 text-[11px] text-current opacity-70"
      }
    >
      <IconActivity className="h-3.5 w-3.5" />
      <span>Metrics unavailable</span>
    </div>
  );
}

function BarSourceMetrics({
  source,
  updatedAt,
  showSourceLabel,
  metricLimit,
  simplified,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  showSourceLabel: boolean;
  metricLimit: number;
  simplified: boolean;
}) {
  return (
    <div className="flex h-full max-w-[360px] items-center gap-3 overflow-hidden text-[11px]">
      {!simplified ? (
        <SourceBadge source={source} updatedAt={updatedAt} showLabel={showSourceLabel} />
      ) : null}
      <MetricValues
        source={source}
        updatedAt={updatedAt}
        limit={metricLimit}
        simplified={simplified}
      />
    </div>
  );
}

function DrawerSourceMetrics({
  source,
  updatedAt,
  simplified,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  simplified: boolean;
}) {
  return (
    <div className="flex min-h-11 items-center gap-2 rounded-md px-3 text-sm hover:bg-muted/60">
      {!simplified ? <SourceBadge source={source} updatedAt={updatedAt} showLabel /> : null}
      <div className="flex min-w-0 flex-1 items-center justify-end gap-3 overflow-hidden">
        <MetricValues source={source} updatedAt={updatedAt} limit={4} simplified={simplified} />
      </div>
    </div>
  );
}

function SourceBadge({
  source,
  updatedAt,
  showLabel,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  showLabel: boolean;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="flex shrink-0 items-center gap-1.5 text-current" aria-label="Host metrics">
          <IconServer className="size-3.5" stroke={1.6} />
          {showLabel ? <span className="max-w-20 truncate">Host</span> : null}
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <div className="space-y-1">
          <div className="font-medium">Host</div>
          <div className="text-xs text-muted-foreground">{source.label}</div>
          <div className="text-xs text-muted-foreground">{lastUpdatedText(updatedAt)}</div>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

function MetricValues({
  source,
  updatedAt,
  limit,
  simplified,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  limit: number;
  simplified: boolean;
}) {
  const metrics = source.metrics.slice(0, limit);
  if (metrics.length === 0) return <span className="text-muted-foreground">-</span>;
  return (
    <span className="flex min-w-0 items-center gap-3 overflow-hidden">
      {metrics.map((metric) => (
        <MetricValue
          key={metric.id}
          metric={metric}
          source={source}
          updatedAt={updatedAt}
          simplified={simplified}
        />
      ))}
    </span>
  );
}

function MetricValue({
  metric,
  source,
  updatedAt,
  simplified,
}: {
  metric: SystemMetricSample;
  source: SystemMetricsSource;
  updatedAt?: string;
  simplified: boolean;
}) {
  const help =
    metric.id === "io_load"
      ? "Average number of tasks running or waiting for CPU during the last minute. Compare this value with the host's CPU core count."
      : null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={`inline-flex shrink-0 items-center gap-1.5 tabular-nums ${metricColor(metric)}`}
          aria-label={`${metricLabel(metric.id)} ${formatMetric(metric)}`}
        >
          {metricIcon(metric.id)}
          {!simplified ? <MetricMeter metric={metric} /> : null}
          <span className="font-medium tracking-[-0.015em] [font-family:var(--font-geist-mono)]">
            {formatMetric(metric)}
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <div className="space-y-1">
          <div className="font-medium">{metricLabel(metric.id)}</div>
          {help ? <div className="max-w-72 text-xs text-muted-foreground">{help}</div> : null}
          <div className="text-xs text-muted-foreground">Host: {source.label}</div>
          <div className="text-xs tabular-nums">{formatMetric(metric)}</div>
          {metric.error ? (
            <div className="text-xs text-muted-foreground">{metric.error}</div>
          ) : null}
          <div className="text-xs text-muted-foreground">{lastUpdatedText(updatedAt)}</div>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

function metricLabel(id: string) {
  return (
    {
      cpu_percent: "CPU",
      memory_percent: "Memory",
      disk_percent: "Disk",
      cpu_temp: "CPU temperature",
      io_load: "System load (1 min)",
    }[id] ?? id
  );
}

function metricIcon(id: string) {
  const Icon =
    {
      cpu_percent: IconCpu,
      memory_percent: IconDatabase,
      disk_percent: IconDisc,
      cpu_temp: IconFlame,
      io_load: IconGauge,
    }[id] ?? IconActivity;
  return <Icon className="size-3.5 opacity-80" stroke={1.6} />;
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

function lastUpdatedText(updatedAt?: string) {
  if (!updatedAt) return "Last update unknown";
  const date = new Date(updatedAt);
  if (Number.isNaN(date.getTime())) return "Last update unknown";
  return `Updated ${formatDistanceToNow(date, { addSuffix: true })}`;
}
