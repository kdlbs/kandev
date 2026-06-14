"use client";

import { useEffect, useRef, useState } from "react";
import { Checkbox } from "@kandev/ui/checkbox";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  fetchSystemMetricsSettings,
  updateSystemMetricsSettings,
  updateUserSettings,
} from "@/lib/api";
import type { SystemMetricId, SystemMetricsGlobalSettings } from "@/lib/types/system";

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

export function SystemMetricsSettingsCard() {
  const storeApi = useAppStoreApi();
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const [settings, setSettings] = useState<SystemMetricsGlobalSettings>(DEFAULT_METRICS_SETTINGS);
  const [isSaving, setIsSaving] = useState(false);
  const saveSeqRef = useRef(0);

  useEffect(() => {
    let cancelled = false;
    fetchSystemMetricsSettings()
      .then((res) => {
        if (!cancelled) setSettings(res.settings);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  const saveGlobal = async (next: SystemMetricsGlobalSettings) => {
    const previous = settings;
    const seq = saveSeqRef.current + 1;
    saveSeqRef.current = seq;
    setSettings(next);
    setIsSaving(true);
    try {
      const res = await updateSystemMetricsSettings(next);
      if (seq === saveSeqRef.current) setSettings(res.settings);
    } catch {
      if (seq === saveSeqRef.current) setSettings(previous);
    } finally {
      if (seq === saveSeqRef.current) setIsSaving(false);
    }
  };

  const toggleMetric = (metric: SystemMetricId, checked: boolean) => {
    const nextMetrics = checked
      ? Array.from(new Set([...settings.metrics, metric]))
      : settings.metrics.filter((id) => id !== metric);
    if (nextMetrics.length > 0) void saveGlobal({ ...settings, metrics: nextMetrics });
  };

  const toggleDisplay = async (checked: boolean) => {
    const current = storeApi.getState().userSettings;
    const previous = current.systemMetricsDisplay;
    try {
      setUserSettings({ ...current, systemMetricsDisplay: { showInTopbar: checked } });
      await updateUserSettings({
        workspace_id: current.workspaceId || "",
        repository_ids: current.repositoryIds || [],
        system_metrics_display: { show_in_topbar: checked },
      });
    } catch {
      setUserSettings({ ...storeApi.getState().userSettings, systemMetricsDisplay: previous });
    }
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
        <MetricsDisplayToggle
          checked={userSettings.systemMetricsDisplay.showInTopbar}
          onCheckedChange={toggleDisplay}
        />
        <MetricsSamplerControls
          settings={settings}
          isSaving={isSaving}
          onToggleMetric={toggleMetric}
          onChangeSettings={(next) => void saveGlobal(next)}
          onDraftSettings={setSettings}
        />
        <ExecutionMetricsToggle
          checked={settings.collect_execution}
          disabled={isSaving}
          onCheckedChange={(checked) =>
            void saveGlobal({ ...settings, collect_execution: checked })
          }
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
        <Label htmlFor="show-system-metrics">Show in desktop topbars</Label>
        <p className="text-xs text-muted-foreground">
          Collection starts only while at least one desktop or tablet client displays metrics.
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
