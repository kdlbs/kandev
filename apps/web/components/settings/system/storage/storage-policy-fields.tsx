import type { ReactNode } from "react";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { SettingsCard } from "@/components/settings/settings-card";
import { StorageSettingHelp } from "./storage-setting-help";

export function PolicySection({
  sectionId,
  title,
  description,
  children,
  isDirty = false,
}: {
  sectionId: string;
  title: string;
  description: string;
  children: ReactNode;
  isDirty?: boolean;
}) {
  return (
    <SettingsCard
      className="min-w-0"
      isDirty={isDirty}
      data-testid={`storage-policy-section-${sectionId}`}
    >
      <CardHeader>
        <CardTitle className="text-sm">{title}</CardTitle>
        <CardDescription className="text-xs">{description}</CardDescription>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </SettingsCard>
  );
}

export function SettingRow({
  title,
  description,
  help,
  control,
}: {
  title: string;
  description: string;
  help: string;
  control: ReactNode;
}) {
  return (
    <div className="flex min-h-11 items-center justify-between gap-4 border-b py-3 last:border-b-0">
      <div className="min-w-0">
        <div className="flex items-center gap-1">
          <Label className="text-sm">{title}</Label>
          <StorageSettingHelp label={title}>{help}</StorageSettingHelp>
        </div>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <div className="shrink-0">{control}</div>
    </div>
  );
}

export function NumberField({
  label,
  help,
  value,
  min,
  max,
  disabled,
  onChange,
  testId,
  isDirty = false,
}: {
  label: string;
  help: string;
  value: number;
  min: number;
  max?: number;
  disabled?: boolean;
  onChange: (value: number) => void;
  testId: string;
  isDirty?: boolean;
}) {
  return (
    <div className="min-w-0 space-y-1">
      <div className="flex items-center gap-1">
        <Label htmlFor={testId} className="text-xs text-muted-foreground">
          {label}
        </Label>
        <StorageSettingHelp label={label}>{help}</StorageSettingHelp>
      </div>
      <Input
        id={testId}
        type="number"
        min={min}
        max={max}
        disabled={disabled}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
        className="h-11"
        data-testid={testId}
        data-settings-dirty={isDirty}
      />
    </div>
  );
}
