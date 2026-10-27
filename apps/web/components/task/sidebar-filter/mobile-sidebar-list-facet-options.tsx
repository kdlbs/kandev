"use client";

import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";
import { SidebarSettingsDisclosure } from "./sidebar-settings-disclosure";
import type { MobileListFacetSettings } from "@/lib/tasks/mobile-list-query";

export function MobileSidebarListFacetOptions({
  settings,
}: {
  settings?: MobileListFacetSettings;
}) {
  const { t } = useTranslation();
  if (!settings || (!settings.options.length && !settings.sort && !settings.group)) return null;
  const summary = [
    settings.sort
      ? `${t("tasks:resultSort")}: ${optionLabel(settings.sort, settings, t("tasks:facetUnavailable"))}`
      : null,
    settings.group
      ? `${t("tasks:resultGroup")}: ${optionLabel(settings.group, settings, t("tasks:facetUnavailable"))}`
      : null,
  ]
    .filter(Boolean)
    .join("; ");
  const options = [
    ...settings.options,
    ...[settings.sort, settings.group]
      .filter((key): key is string => Boolean(key))
      .filter((key) => !settings.options.some((option) => option.value === key))
      .map((key) => ({ value: key, label: t("tasks:facetUnavailable") })),
  ];

  return (
    <SidebarSettingsDisclosure
      title={t("tasks:additionalListOptions")}
      summary={summary || undefined}
      testId="mobile-list-additional-options"
      className="border-b"
      contentClassName="space-y-3 px-2 pb-3 pt-1"
    >
      <FacetSelect
        label={t("tasks:resultSort")}
        testId="mobile-list-facet-sort"
        value={settings.sort ?? "none"}
        options={options}
        onChange={settings.onSortChange}
      />
      <FacetSelect
        label={t("tasks:resultGroup")}
        testId="mobile-list-facet-group"
        value={settings.group ?? "none"}
        options={options}
        onChange={settings.onGroupChange}
      />
      {(settings.sort || settings.group) && (
        <Button
          type="button"
          variant="ghost"
          className="h-10 cursor-pointer px-2"
          onClick={settings.onReset}
          data-testid="mobile-list-reset-facet-overrides"
        >
          {t("tasks:resetResultOverrides")}
        </Button>
      )}
    </SidebarSettingsDisclosure>
  );
}

function FacetSelect({
  label,
  testId,
  value,
  options,
  onChange,
}: {
  label: string;
  testId: string;
  value: string;
  options: ReadonlyArray<{ value: string; label: string }>;
  onChange: (facetKey: string | null) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <label className="text-xs font-medium text-muted-foreground" id={`${testId}-label`}>
        {label}
      </label>
      <Select value={value} onValueChange={(next) => onChange(next === "none" ? null : next)}>
        <SelectTrigger
          className="h-11 w-full cursor-pointer"
          aria-labelledby={`${testId}-label`}
          data-testid={testId}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="none">{t("task:none")}</SelectItem>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function optionLabel(key: string, settings: MobileListFacetSettings, unavailableLabel: string) {
  return settings.options.find((option) => option.value === key)?.label ?? unavailableLabel;
}
