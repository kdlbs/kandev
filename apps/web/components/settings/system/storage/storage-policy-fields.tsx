import type { ReactNode } from "react";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import { settingsControlClassName } from "@/components/settings/settings-control";
import { StorageSettingHelp } from "./storage-setting-help";
import { storageSettingsTarget } from "@/lib/settings-discovery/catalog/system";

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
      discoveryTargetId={storageSettingsTarget(sectionId)}
      data-testid={`storage-policy-section-${sectionId}`}
    >
      <SettingsCardHeader title={title} description={description} />
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
          <SettingsFieldLabel className="text-sm">{title}</SettingsFieldLabel>
          <StorageSettingHelp label={title}>{help}</StorageSettingHelp>
        </div>
        <SettingsFieldDescription>{description}</SettingsFieldDescription>
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
        <SettingsFieldLabel htmlFor={testId}>{label}</SettingsFieldLabel>
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
        className={settingsControlClassName("h-11")}
        data-testid={testId}
        data-settings-dirty={isDirty}
      />
    </div>
  );
}
