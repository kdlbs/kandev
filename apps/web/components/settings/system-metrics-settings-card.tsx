"use client";

import { useEffect, useState } from "react";
import { Checkbox } from "@kandev/ui/checkbox";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { fetchSystemMetricsSettings, updateSystemMetricsSettings } from "@/lib/api";
import type { SystemMetricId, SystemMetricsGlobalSettings } from "@/lib/types/system";
import { useSettingsSaveContributor } from "./settings-save-provider";

const METRIC_OPTIONS: Array<{ id: SystemMetricId; label: string }> = [
  { id: "cpu_percent", label: "CPU %" },
  { id: "memory_percent", label: "Memory %" },
  { id: "disk_percent", label: "Disk %" },
  { id: "cpu_temp", label: "CPU temp" },
  { id: "io_load", label: "Load avg" },
];

const DEFAULT_METRICS_SETTINGS: SystemMetricsGlobalSettings = {
  metrics: ["cpu_percent", "memory_percent", "disk_percent"],
  interval_seconds: 5,
  backend_disk_path: "/",
  collect_execution: false,
};

export function SystemMetricsSettingsCard({
  showInTopbar,
  onShowInTopbarChange,
}: {
  showInTopbar: boolean;
  onShowInTopbarChange: (checked: boolean) => void;
}) {
  const [settings, setSettings] = useState<SystemMetricsGlobalSettings>(DEFAULT_METRICS_SETTINGS);
  const [savedSettings, setSavedSettings] =
    useState<SystemMetricsGlobalSettings>(DEFAULT_METRICS_SETTINGS);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let cancelled = false;
    fetchSystemMetricsSettings()
      .then((res) => {
        if (!cancelled) {
          setSettings(res.settings);
          setSavedSettings(res.settings);
          setLoaded(true);
        }
      })
      .catch(() => {
        if (!cancelled) setLoaded(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const revision = JSON.stringify(settings);
  useSettingsSaveContributor({
    id: "general-appearance-metrics",
    order: 20,
    revision,
    isDirty: loaded && revision !== JSON.stringify(savedSettings),
    save: async () => {
      const submitted = settings;
      const response = await updateSystemMetricsSettings(submitted);
      setSavedSettings(response.settings);
      setSettings((current) => (current === submitted ? response.settings : current));
    },
    discard: () => setSettings(savedSettings),
  });

  const toggleMetric = (metric: SystemMetricId, checked: boolean) => {
    const nextMetrics = checked
      ? Array.from(new Set([...settings.metrics, metric]))
      : settings.metrics.filter((id) => id !== metric);
    if (nextMetrics.length > 0) setSettings({ ...settings, metrics: nextMetrics });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Resource Metrics</CardTitle>
      </CardHeader>
      <CardContent className="space-y-5">
        <p className="max-w-3xl text-sm text-muted-foreground">
          Useful when Kandev is self-hosted on a remote server and you want a lightweight view of
          the machine resources from the kanban or task topbar.
        </p>
        <MetricsDisplayToggle checked={showInTopbar} onCheckedChange={onShowInTopbarChange} />
        <MetricsSamplerControls
          settings={settings}
          isSaving={!loaded}
          onToggleMetric={toggleMetric}
          onChangeSettings={setSettings}
          onDraftSettings={setSettings}
        />
        <ExecutionMetricsToggle
          checked={settings.collect_execution}
          disabled={!loaded}
          onCheckedChange={(checked) => setSettings({ ...settings, collect_execution: checked })}
        />
      </CardContent>
    </Card>
  );
}

function MetricsDisplayToggle({
  checked,
  onCheckedChange,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="space-y-1">
        <Label htmlFor="show-system-metrics">Show in topbars</Label>
        <p className="text-xs text-muted-foreground">
          Collection starts only while at least one client displays metrics.
        </p>
      </div>
      <Switch id="show-system-metrics" checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  );
}

function MetricsSamplerControls({
  settings,
  isSaving,
  onToggleMetric,
  onChangeSettings,
  onDraftSettings,
}: {
  settings: SystemMetricsGlobalSettings;
  isSaving: boolean;
  onToggleMetric: (metric: SystemMetricId, checked: boolean) => void;
  onChangeSettings: (settings: SystemMetricsGlobalSettings) => void;
  onDraftSettings: (settings: SystemMetricsGlobalSettings) => void;
}) {
  return (
    <div className="grid gap-4 md:grid-cols-[1fr_180px_180px]">
      <MetricCheckboxes settings={settings} isSaving={isSaving} onToggleMetric={onToggleMetric} />
      <div className="space-y-2">
        <Label htmlFor="metrics-interval">Frequency (seconds)</Label>
        <Input
          id="metrics-interval"
          type="number"
          min={1}
          max={300}
          value={settings.interval_seconds}
          disabled={isSaving}
          onChange={(event) =>
            onChangeSettings({ ...settings, interval_seconds: clampInterval(event.target.value) })
          }
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="metrics-disk-path">Disk path</Label>
        <Input
          id="metrics-disk-path"
          value={settings.backend_disk_path}
          disabled={isSaving}
          onChange={(event) =>
            onDraftSettings({ ...settings, backend_disk_path: event.target.value })
          }
          onBlur={() => onChangeSettings(settings)}
        />
      </div>
    </div>
  );
}

function MetricCheckboxes({
  settings,
  isSaving,
  onToggleMetric,
}: {
  settings: SystemMetricsGlobalSettings;
  isSaving: boolean;
  onToggleMetric: (metric: SystemMetricId, checked: boolean) => void;
}) {
  return (
    <div className="space-y-2">
      <Label>Metrics</Label>
      <div className="grid gap-2 sm:grid-cols-2">
        {METRIC_OPTIONS.map((metric) => (
          <label key={metric.id} className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={settings.metrics.includes(metric.id)}
              disabled={isSaving}
              onCheckedChange={(checked) => onToggleMetric(metric.id, checked === true)}
            />
            <span>{metric.label}</span>
          </label>
        ))}
      </div>
    </div>
  );
}

function ExecutionMetricsToggle({
  checked,
  disabled,
  onCheckedChange,
}: {
  checked: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="space-y-1">
        <Label htmlFor="collect-execution-metrics">Collect execution environment metrics</Label>
        <p className="text-xs text-muted-foreground">
          Adds agentctl values for Docker, SSH, Sprites, and remote executors.
        </p>
      </div>
      <Switch
        id="collect-execution-metrics"
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
      />
    </div>
  );
}

function clampInterval(raw: string) {
  return Math.min(300, Math.max(1, Number(raw) || 5));
}
