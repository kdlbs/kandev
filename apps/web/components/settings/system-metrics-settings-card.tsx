"use client";

import { useEffect, useState } from "react";
import { Checkbox } from "@kandev/ui/checkbox";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { fetchSystemMetricsSettings, updateSystemMetricsSettings } from "@/lib/api";
import type { SystemMetricId, SystemMetricsGlobalSettings } from "@/lib/types/system";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { SettingsCard } from "./settings-card";
import { useTranslation } from "react-i18next";

const METRIC_OPTIONS: Array<{ id: SystemMetricId; labelKey: string }> = [
  { id: "cpu_percent", labelKey: "settings:cpu" },
  { id: "memory_percent", labelKey: "settings:memory" },
  { id: "disk_percent", labelKey: "settings:disk" },
  { id: "cpu_temp", labelKey: "settings:cpuTemp" },
  { id: "io_load", labelKey: "settings:systemLoad1Min" },
];

const DEFAULT_METRICS_SETTINGS: SystemMetricsGlobalSettings = {
  metrics: ["cpu_percent", "memory_percent", "disk_percent"],
  interval_seconds: 5,
  backend_disk_path: "/",
  collect_execution: false,
};

export function SystemMetricsSettingsCard({
  showInTopbar,
  isShowInTopbarDirty,
  onShowInTopbarChange,
  simplified,
  isSimplifiedDirty,
  onSimplifiedChange,
}: {
  showInTopbar: boolean;
  isShowInTopbarDirty?: boolean;
  onShowInTopbarChange: (checked: boolean) => void;
  simplified: boolean;
  isSimplifiedDirty?: boolean;
  onSimplifiedChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
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
        if (!cancelled) setLoaded(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const revision = JSON.stringify(settings);
  const isDirty = loaded && revision !== JSON.stringify(savedSettings);
  useSettingsSaveContributor({
    id: "general-appearance-metrics",
    order: 20,
    revision,
    isDirty,
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
    <SettingsCard isDirty={isDirty || Boolean(isShowInTopbarDirty) || Boolean(isSimplifiedDirty)}>
      <CardHeader>
        <CardTitle className="text-base">{t("settings:resourceMetrics")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-5">
        <p className="max-w-3xl text-sm text-muted-foreground">
          {t("settings:usefulWhenKandevIsSelfHosted")}
        </p>
        <MetricsDisplayToggle
          checked={showInTopbar}
          isDirty={Boolean(isShowInTopbarDirty)}
          onCheckedChange={onShowInTopbarChange}
        />
        <SimplifiedMetricsToggle
          checked={simplified}
          isDirty={Boolean(isSimplifiedDirty)}
          onCheckedChange={onSimplifiedChange}
        />
        <MetricsSamplerControls
          settings={settings}
          savedSettings={savedSettings}
          isSaving={!loaded}
          onToggleMetric={toggleMetric}
          onChangeSettings={setSettings}
          onDraftSettings={setSettings}
        />
        <ExecutionMetricsToggle
          checked={settings.collect_execution}
          isDirty={settings.collect_execution !== savedSettings.collect_execution}
          disabled={!loaded}
          onCheckedChange={(checked) => setSettings({ ...settings, collect_execution: checked })}
        />
      </CardContent>
    </SettingsCard>
  );
}

function SimplifiedMetricsToggle({
  checked,
  isDirty,
  onCheckedChange,
}: {
  checked: boolean;
  isDirty: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="space-y-1">
        <Label htmlFor="simplified-system-metrics">{t("settings:simplifiedMetrics")}</Label>
        <p className="text-xs text-muted-foreground">
          {t("settings:removesTheHostMarkerAndProgress")}
        </p>
      </div>
      <Switch
        id="simplified-system-metrics"
        checked={checked}
        data-settings-dirty={isDirty}
        onCheckedChange={onCheckedChange}
      />
    </div>
  );
}

function MetricsDisplayToggle({
  checked,
  isDirty,
  onCheckedChange,
}: {
  checked: boolean;
  isDirty: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="space-y-1">
        <Label htmlFor="show-system-metrics">{t("settings:showHostMetricsInStatusBar")}</Label>
        <p className="text-xs text-muted-foreground">
          {t("settings:showsKandevHostValuesOnlyCollection")}
        </p>
      </div>
      <Switch
        id="show-system-metrics"
        checked={checked}
        data-settings-dirty={isDirty}
        onCheckedChange={onCheckedChange}
      />
    </div>
  );
}

function MetricsSamplerControls({
  settings,
  savedSettings,
  isSaving,
  onToggleMetric,
  onChangeSettings,
  onDraftSettings,
}: {
  settings: SystemMetricsGlobalSettings;
  savedSettings: SystemMetricsGlobalSettings;
  isSaving: boolean;
  onToggleMetric: (metric: SystemMetricId, checked: boolean) => void;
  onChangeSettings: (settings: SystemMetricsGlobalSettings) => void;
  onDraftSettings: (settings: SystemMetricsGlobalSettings) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-4 md:grid-cols-[1fr_180px_180px]">
      <MetricCheckboxes
        settings={settings}
        savedSettings={savedSettings}
        isSaving={isSaving}
        onToggleMetric={onToggleMetric}
      />
      <div className="space-y-2">
        <Label htmlFor="metrics-interval">{t("settings:frequencySeconds")}</Label>
        <Input
          id="metrics-interval"
          type="number"
          min={1}
          max={300}
          value={settings.interval_seconds}
          data-settings-dirty={settings.interval_seconds !== savedSettings.interval_seconds}
          disabled={isSaving}
          onChange={(event) =>
            onChangeSettings({ ...settings, interval_seconds: clampInterval(event.target.value) })
          }
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="metrics-disk-path">{t("settings:diskPath")}</Label>
        <Input
          id="metrics-disk-path"
          value={settings.backend_disk_path}
          data-settings-dirty={settings.backend_disk_path !== savedSettings.backend_disk_path}
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
  savedSettings,
  isSaving,
  onToggleMetric,
}: {
  settings: SystemMetricsGlobalSettings;
  savedSettings: SystemMetricsGlobalSettings;
  isSaving: boolean;
  onToggleMetric: (metric: SystemMetricId, checked: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <Label>{t("settings:metrics")}</Label>
      <div className="grid gap-2 sm:grid-cols-2">
        {METRIC_OPTIONS.map((metric) => (
          <label key={metric.id} className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={settings.metrics.includes(metric.id)}
              data-settings-dirty={
                settings.metrics.includes(metric.id) !== savedSettings.metrics.includes(metric.id)
              }
              disabled={isSaving}
              onCheckedChange={(checked) => onToggleMetric(metric.id, checked === true)}
            />
            <span>{t(metric.labelKey)}</span>
          </label>
        ))}
      </div>
    </div>
  );
}

function ExecutionMetricsToggle({
  checked,
  isDirty,
  disabled,
  onCheckedChange,
}: {
  checked: boolean;
  isDirty: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="space-y-1">
        <Label htmlFor="collect-execution-metrics">
          {t("settings:collectExecutionEnvironmentMetrics")}
        </Label>
        <p className="text-xs text-muted-foreground">
          {t("settings:makesAgentctlValuesAvailableToPlugins")}
        </p>
      </div>
      <Switch
        id="collect-execution-metrics"
        checked={checked}
        data-settings-dirty={isDirty}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
      />
    </div>
  );
}

function clampInterval(raw: string) {
  return Math.min(300, Math.max(1, Number(raw) || 5));
}
