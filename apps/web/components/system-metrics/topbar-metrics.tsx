"use client";

import { IconActivity } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useSystemMetricsSubscription } from "@/hooks/use-system-metrics-subscription";
import type { SystemMetricSample, SystemMetricsSource } from "@/lib/types/system";

type TopbarMetricsProps = {
  activeSessionId?: string | null;
};

export function TopbarMetrics({ activeSessionId }: TopbarMetricsProps) {
  const enabled = useAppStore((s) => s.userSettings.systemMetricsDisplay.showInTopbar);
  const snapshot = useAppStore((s) => s.system.metrics);
  useSystemMetricsSubscription(enabled);

  if (!enabled) return null;
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
        <div
          key={source.id}
          className="flex h-7 max-w-[360px] items-center gap-1 overflow-hidden rounded border border-border px-2 text-xs"
          title={source.label}
        >
          <span className="shrink-0 text-muted-foreground">
            {source.kind === "backend" ? "Host" : "Exec"}
          </span>
          {source.metrics
            .filter((metric) => metric.available)
            .slice(0, 4)
            .map((metric) => (
              <MetricChip key={metric.id} metric={metric} />
            ))}
        </div>
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

function MetricChip({ metric }: { metric: SystemMetricSample }) {
  return (
    <span className={`shrink-0 tabular-nums ${metricColor(metric)}`}>
      {shortLabel(metric.id)} {formatMetric(metric)}
    </span>
  );
}

function shortLabel(id: string) {
  switch (id) {
    case "cpu_percent":
      return "CPU";
    case "memory_percent":
      return "Mem";
    case "disk_percent":
      return "Disk";
    case "cpu_temp":
      return "Temp";
    case "io_load":
      return "Load";
    default:
      return id;
  }
}

function formatMetric(metric: SystemMetricSample) {
  if (typeof metric.value !== "number") return "-";
  const value = metric.unit === "%" ? Math.round(metric.value) : Math.round(metric.value * 10) / 10;
  return `${value}${metric.unit ?? ""}`;
}

function metricColor(metric: SystemMetricSample) {
  if (metric.unit !== "%" || typeof metric.value !== "number") return "text-muted-foreground";
  if (metric.value > 95) return "text-destructive";
  if (metric.value >= 80) return "text-yellow-500 dark:text-yellow-400";
  return "text-muted-foreground";
}
