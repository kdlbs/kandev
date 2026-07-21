"use client";

import {
  IconActivity,
  IconCpu,
  IconDatabase,
  IconDeviceDesktopAnalytics,
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
  activeSessionId: string | null;
  presentation: "bar" | "mobile-drawer";
  density: "full" | "compact";
  drawerOpen: boolean;
};

export function StatusSurfaceMetrics({
  activeSessionId,
  presentation,
  density,
  drawerOpen,
}: StatusSurfaceMetricsProps) {
  // Wire/storage name stays stable for existing user settings and API payloads.
  const enabled = useAppStore((state) => state.userSettings.systemMetricsDisplay.showInTopbar);
  const snapshot = useAppStore((state) => state.system.metrics);
  const { isMobile } = useResponsiveBreakpoint();
  const shouldSubscribe = enabled && (!isMobile || drawerOpen);
  useSystemMetricsSubscription(shouldSubscribe);

  if (!enabled || (isMobile && !drawerOpen)) return null;

  const sources = selectSources(snapshot?.sources ?? [], activeSessionId, density);
  if (presentation === "mobile-drawer") {
    return (
      <section data-testid="app-status-metrics" className="space-y-1" aria-label="System metrics">
        <h3 className="px-1 text-sm font-medium">System metrics</h3>
        {sources.length === 0 ? (
          <EmptyMetrics drawer />
        ) : (
          sources.map((source) => (
            <DrawerSourceMetrics key={source.id} source={source} updatedAt={snapshot?.timestamp} />
          ))
        )}
      </section>
    );
  }

  return (
    <div
      data-testid="app-status-metrics"
      className="flex h-6 max-w-[42vw] items-center gap-1 overflow-hidden"
      aria-label="System metrics"
    >
      {sources.length === 0 ? (
        <EmptyMetrics />
      ) : (
        sources.map((source) => (
          <BarSourceMetrics
            key={source.id}
            source={source}
            updatedAt={snapshot?.timestamp}
            showSource={density === "compact" || sources.length > 1}
            metricLimit={density === "compact" ? 2 : 4}
          />
        ))
      )}
    </div>
  );
}

function selectSources(
  sources: SystemMetricsSource[],
  activeSessionId: string | null,
  density: "full" | "compact",
) {
  if (density === "compact") {
    return [sources.find((source) => source.session_id === activeSessionId) ?? sources[0]].filter(
      Boolean,
    ) as SystemMetricsSource[];
  }
  if (!activeSessionId) return sources.slice(0, 2);
  return [
    sources.find((source) => source.kind === "backend"),
    sources.find((source) => source.session_id === activeSessionId),
  ].filter(Boolean) as SystemMetricsSource[];
}

function EmptyMetrics({ drawer = false }: { drawer?: boolean }) {
  return (
    <div
      className={
        drawer
          ? "flex min-h-11 items-center gap-2 rounded-md px-3 text-sm text-muted-foreground"
          : "flex h-6 items-center gap-1 px-1 text-xs text-muted-foreground"
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
  showSource,
  metricLimit,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  showSource: boolean;
  metricLimit: number;
}) {
  return (
    <div className="flex h-6 max-w-[220px] items-center gap-1 overflow-hidden border-l border-border px-1.5 text-xs first:border-l-0">
      {showSource ? <SourceBadge source={source} updatedAt={updatedAt} /> : null}
      <MetricValues source={source} updatedAt={updatedAt} limit={metricLimit} />
    </div>
  );
}

function DrawerSourceMetrics({
  source,
  updatedAt,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
}) {
  return (
    <div className="flex min-h-11 items-center gap-2 rounded-md px-3 text-sm hover:bg-muted/60">
      <SourceBadge source={source} updatedAt={updatedAt} />
      <div className="flex min-w-0 flex-1 items-center justify-end gap-3 overflow-hidden">
        <MetricValues source={source} updatedAt={updatedAt} limit={4} />
      </div>
    </div>
  );
}

function SourceBadge({ source, updatedAt }: { source: SystemMetricsSource; updatedAt?: string }) {
  const isHost = source.kind === "backend";
  const label = isHost ? "Host" : "Executor";
  const Icon = isHost ? IconServer : IconDeviceDesktopAnalytics;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="flex shrink-0 items-center gap-1 text-muted-foreground"
          aria-label={`${label} metrics`}
        >
          <Icon className="h-3.5 w-3.5" />
          <span className="max-w-20 truncate text-xs">{label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <div className="space-y-1">
          <div className="font-medium">{label}</div>
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
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  limit: number;
}) {
  const metrics = source.metrics.slice(0, limit);
  if (metrics.length === 0) return <span className="text-muted-foreground">-</span>;
  return metrics.map((metric) => (
    <MetricValue key={metric.id} metric={metric} source={source} updatedAt={updatedAt} />
  ));
}

function MetricValue({
  metric,
  source,
  updatedAt,
}: {
  metric: SystemMetricSample;
  source: SystemMetricsSource;
  updatedAt?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={`inline-flex shrink-0 items-center gap-0.5 tabular-nums ${metricColor(metric)}`}
          aria-label={`${metricLabel(metric.id)} ${formatMetric(metric)}`}
        >
          {metricIcon(metric.id)}
          <span>{formatMetric(metric)}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <div className="space-y-1">
          <div className="font-medium">{metricLabel(metric.id)}</div>
          <div className="text-xs text-muted-foreground">
            {source.kind === "backend" ? "Host" : "Executor"}: {source.label}
          </div>
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
      io_load: "Load avg",
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
  return <Icon className="h-3.5 w-3.5" />;
}

function formatMetric(metric: SystemMetricSample) {
  if (typeof metric.value !== "number") return "-";
  const value = metric.unit === "%" ? Math.round(metric.value) : Math.round(metric.value * 10) / 10;
  return `${value}${metric.unit ?? ""}`;
}

function metricColor(metric: SystemMetricSample) {
  if (!metric.available) return "text-muted-foreground";
  const thresholdValue = metric.unit === "%" || metric.id === "cpu_temp" ? metric.value : null;
  if (typeof thresholdValue !== "number") return "text-muted-foreground";
  if (thresholdValue > 95) return "text-destructive";
  if (thresholdValue >= 80) return "text-yellow-500 dark:text-yellow-400";
  return "text-muted-foreground";
}

function lastUpdatedText(updatedAt?: string) {
  if (!updatedAt) return "Last update unknown";
  const date = new Date(updatedAt);
  if (Number.isNaN(date.getTime())) return "Last update unknown";
  return `Updated ${formatDistanceToNow(date, { addSuffix: true })}`;
}
