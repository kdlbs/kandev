"use client";

import {
  IconActivity,
  IconCpu,
  IconDatabase,
  IconDeviceDesktopAnalytics,
  IconFlame,
  IconGauge,
  IconDisc,
  IconServer,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { formatDistanceToNow } from "date-fns";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useSystemMetricsSubscription } from "@/hooks/use-system-metrics-subscription";
import type { SystemMetricSample, SystemMetricsSource } from "@/lib/types/system";

type TopbarMetricsProps = {
  activeSessionId?: string | null;
};

export function TopbarMetrics({ activeSessionId }: TopbarMetricsProps) {
  const enabled = useAppStore((s) => s.userSettings.systemMetricsDisplay.showInTopbar);
  const snapshot = useAppStore((s) => s.system.metrics);
  const { isMobile } = useResponsiveBreakpoint();
  const shouldRender = enabled && !isMobile;
  useSystemMetricsSubscription(shouldRender);

  if (!shouldRender) return null;
  const sources = selectSources(snapshot?.sources ?? [], activeSessionId);
  if (sources.length === 0) {
    return (
      <div className="hidden md:flex h-7 items-center gap-1 rounded border border-border px-2 text-xs text-muted-foreground">
        <IconActivity className="h-3.5 w-3.5" />
        <span>Metrics</span>
      </div>
    );
  }
  return (
    <div className="hidden md:flex max-w-[42vw] items-center gap-1 overflow-hidden">
      {sources.map((source) => (
        <SourceMetrics
          key={source.id}
          source={source}
          updatedAt={snapshot?.timestamp}
          showSource={sources.length > 1}
        />
      ))}
    </div>
  );
}

function selectSources(sources: SystemMetricsSource[], activeSessionId?: string | null) {
  if (!activeSessionId) return sources.slice(0, 2);
  const backend = sources.find((source) => source.kind === "backend");
  const execution = sources.find((source) => source.session_id === activeSessionId);
  return [backend, execution].filter(Boolean) as SystemMetricsSource[];
}

function SourceMetrics({
  source,
  updatedAt,
  showSource,
}: {
  source: SystemMetricsSource;
  updatedAt?: string;
  showSource: boolean;
}) {
  const metrics = source.metrics.slice(0, 4);

  return (
    <div className="flex h-7 max-w-[220px] items-center gap-1 overflow-hidden rounded border border-border px-1.5 text-xs">
      {showSource ? <SourceBadge source={source} updatedAt={updatedAt} /> : null}
      {metrics.length > 0 ? (
        metrics.map((metric) => (
          <MetricChip key={metric.id} metric={metric} source={source} updatedAt={updatedAt} />
        ))
      ) : (
        <span className="px-1 text-muted-foreground">-</span>
      )}
    </div>
  );
}

function SourceBadge({ source, updatedAt }: { source: SystemMetricsSource; updatedAt?: string }) {
  const isHost = source.kind === "backend";
  const label = isHost ? "Host" : "Executor";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="flex h-5 w-5 shrink-0 items-center justify-center rounded text-muted-foreground"
          aria-label={`${label} metrics`}
        >
          {isHost ? (
            <IconServer className="h-3.5 w-3.5" />
          ) : (
            <IconDeviceDesktopAnalytics className="h-3.5 w-3.5" />
          )}
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

function MetricChip({
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
          className={`flex h-5 shrink-0 items-center gap-0.5 rounded px-1 tabular-nums ${metricColor(metric)}`}
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
  switch (id) {
    case "cpu_percent":
      return "CPU";
    case "memory_percent":
      return "Memory";
    case "disk_percent":
      return "Disk";
    case "cpu_temp":
      return "CPU temperature";
    case "io_load":
      return "Load avg";
    default:
      return id;
  }
}

function metricIcon(id: string) {
  switch (id) {
    case "cpu_percent":
      return <IconCpu className="h-3.5 w-3.5" />;
    case "memory_percent":
      return <IconDatabase className="h-3.5 w-3.5" />;
    case "disk_percent":
      return <IconDisc className="h-3.5 w-3.5" />;
    case "cpu_temp":
      return <IconFlame className="h-3.5 w-3.5" />;
    case "io_load":
      return <IconGauge className="h-3.5 w-3.5" />;
    default:
      return <IconActivity className="h-3.5 w-3.5" />;
  }
}

function formatMetric(metric: SystemMetricSample) {
  if (typeof metric.value !== "number") return "-";
  const value = metric.unit === "%" ? Math.round(metric.value) : Math.round(metric.value * 10) / 10;
  return `${value}${metric.unit ?? ""}`;
}

function metricColor(metric: SystemMetricSample) {
  if (!metric.available) return "text-muted-foreground";
  const thresholdValue = metricThresholdValue(metric);
  if (thresholdValue === null) return "text-muted-foreground";
  if (thresholdValue > 95) return "text-destructive";
  if (thresholdValue >= 80) return "text-yellow-500 dark:text-yellow-400";
  return "text-muted-foreground";
}

function metricThresholdValue(metric: SystemMetricSample) {
  if (typeof metric.value !== "number") return null;
  if (metric.unit === "%" || metric.id === "cpu_temp") return metric.value;
  return null;
}

function lastUpdatedText(updatedAt?: string) {
  if (!updatedAt) return "Last update unknown";
  const date = new Date(updatedAt);
  if (Number.isNaN(date.getTime())) return "Last update unknown";
  return `Updated ${formatDistanceToNow(date, { addSuffix: true })}`;
}
