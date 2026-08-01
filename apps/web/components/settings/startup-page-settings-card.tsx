"use client";

import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { StartupPage } from "@/lib/types/http";
import { SettingsCard } from "./settings-card";

const OPTIONS: Array<{ value: StartupPage; label: string; description: string }> = [
  {
    value: "task_overview",
    label: "Task overview",
    description: "Start on your saved Kanban, Pipeline, or List view.",
  },
  {
    value: "last_task",
    label: "Last visited task",
    description: "Resume the most recently opened task in this workspace on this device.",
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
  return (
    <SettingsCard isDirty={isDirty} data-testid="startup-page-settings-card">
      <CardHeader>
        <CardTitle className="text-base">Open Kandev to</CardTitle>
        <CardDescription>
          This applies when Kandev starts or you open its bare home page. Home, Back, and workflow
          navigation always open the task overview.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <RadioGroup
          aria-label="Startup page"
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
                    {option.label}
                  </span>
                  <span
                    id={descriptionId}
                    className="block whitespace-normal break-words text-xs text-muted-foreground"
                  >
                    {option.description}
                  </span>
                </span>
              </Label>
            );
          })}
        </RadioGroup>
      </CardContent>
    </SettingsCard>
  );
}
