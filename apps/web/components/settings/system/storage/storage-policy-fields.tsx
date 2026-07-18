import type { ReactNode } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { StorageSettingHelp } from "./storage-setting-help";

export function PolicySection({
  sectionId,
  title,
  description,
  children,
}: {
  sectionId: string;
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <Card className="min-w-0" data-testid={`storage-policy-section-${sectionId}`}>
      <CardHeader>
        <CardTitle className="text-sm">{title}</CardTitle>
        <CardDescription className="text-xs">{description}</CardDescription>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
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
}: {
  label: string;
  help: string;
  value: number;
  min: number;
  max?: number;
  disabled?: boolean;
  onChange: (value: number) => void;
  testId: string;
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
      />
    </div>
  );
}
