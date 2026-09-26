"use client";

import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { StartupPage } from "@/lib/types/http";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { useSettingsTargetRegistration } from "./settings-target-provider";

/**
 * Holds catalog KEYS, not copy. A module-scope table is evaluated once at import,
 * so a `t()` call here would freeze at the boot locale and never update on a
 * switch — and the lint guard cannot see literals in a SCREAMING_CASE constant,
 * so nothing would flag it. Resolved at render in the component below.
 */
const OPTIONS: Array<{ value: StartupPage; labelKey: string; descriptionKey: string }> = [
  {
    value: "task_overview",
    labelKey: "settings:taskOverview",
    descriptionKey: "settings:startOnYourSavedKanbanPipelineOrList",
  },
  {
    value: "last_task",
    labelKey: "settings:lastVisitedTask",
    descriptionKey: "settings:resumeTheMostRecentlyOpenedTask",
  },
  {
    value: "threads",
    labelKey: "kanban:threads",
    descriptionKey: "settings:openThreadsOnStartupAndHome",
  },
];

export function StartupPageSettingsCard({
  value,
  isDirty,
  onChange,
}: {
  value: StartupPage;
  isDirty: boolean;
  onChange: (value: StartupPage) => void;
}) {
  const { t } = useTranslation();
  const registerTarget = useSettingsTargetRegistration(GENERAL_SETTINGS_TARGETS.startupPage);
  return (
    <div
      ref={registerTarget}
      className="space-y-4 py-3"
      data-testid="startup-page-settings-card"
      data-settings-dirty={isDirty}
    >
      <div>
        <h4 className="text-sm font-semibold">{t("settings:openKandevTo")}</h4>
        <p className="text-xs text-muted-foreground">{t("settings:thisAppliesWhenKandevStarts")}</p>
      </div>
      <RadioGroup
        aria-label={t("settings:startupPage")}
        value={value}
        onValueChange={(next) => onChange(next as StartupPage)}
        data-settings-dirty={isDirty}
        className="gap-3"
      >
        {OPTIONS.map((option) => {
          const labelId = `startup-page-${option.value}-label`;
          const descriptionId = `startup-page-${option.value}-description`;
          const selected = value === option.value;
          return (
            <Label
              key={option.value}
              htmlFor={`startup-page-${option.value}`}
              className={`flex min-h-11 w-full min-w-0 cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors ${
                selected ? "border-primary bg-primary/5" : "border-border hover:bg-muted/30"
              }`}
            >
              <RadioGroupItem
                id={`startup-page-${option.value}`}
                value={option.value}
                aria-labelledby={labelId}
                aria-describedby={descriptionId}
                className="mt-0.5 border border-muted-foreground/80 data-[state=checked]:border-primary"
              />
              <span className="min-w-0 space-y-1">
                <span id={labelId} className="block text-sm font-medium">
                  {t(option.labelKey)}
                </span>
                <span
                  id={descriptionId}
                  className="block whitespace-normal break-words text-xs text-muted-foreground"
                >
                  {t(option.descriptionKey)}
                </span>
              </span>
            </Label>
          );
        })}
      </RadioGroup>
    </div>
  );
}
