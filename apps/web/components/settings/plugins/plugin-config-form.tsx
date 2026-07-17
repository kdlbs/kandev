"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import type { PluginConfigField } from "@/lib/plugins/config-schema";

type PluginConfigFormProps = {
  fields: PluginConfigField[];
  values: Record<string, string | boolean>;
  disabled: boolean;
  onChange: (name: string, value: string | boolean) => void;
};

/**
 * Schema-driven settings form: one control per config_schema property.
 * Secret fields render as password inputs pre-filled with the backend's
 * mask; editing replaces the value, leaving it untouched keeps the stored
 * secret. Purely controlled — load/save/dirty state lives in PluginDetail.
 */
export function PluginConfigForm({ fields, values, disabled, onChange }: PluginConfigFormProps) {
  return (
    <div className="space-y-5">
      {fields.map((field) => (
        <ConfigFieldRow
          key={field.name}
          field={field}
          value={values[field.name] ?? ""}
          disabled={disabled}
          onChange={onChange}
        />
      ))}
    </div>
  );
}

type ConfigFieldRowProps = {
  field: PluginConfigField;
  value: string | boolean;
  disabled: boolean;
  onChange: (name: string, value: string | boolean) => void;
};

function ConfigFieldRow({ field, value, disabled, onChange }: ConfigFieldRowProps) {
  const inputId = `plugin-config-${field.name}`;
  return (
    <div className="space-y-1.5" data-testid={`plugin-config-field-${field.name}`}>
      <Label htmlFor={inputId} className="text-sm">
        {field.label}
        {field.required && <span className="text-destructive"> *</span>}
      </Label>
      <ConfigFieldControl
        field={field}
        inputId={inputId}
        value={value}
        disabled={disabled}
        onChange={onChange}
      />
      {field.description && <p className="text-xs text-muted-foreground">{field.description}</p>}
    </div>
  );
}

type ConfigFieldControlProps = ConfigFieldRowProps & { inputId: string };

function ConfigFieldControl({
  field,
  inputId,
  value,
  disabled,
  onChange,
}: ConfigFieldControlProps) {
  if (field.type === "boolean") {
    return (
      <div>
        <Switch
          id={inputId}
          checked={value === true}
          disabled={disabled}
          onCheckedChange={(checked) => onChange(field.name, checked)}
        />
      </div>
    );
  }

  if (field.type === "enum") {
    return (
      <Select
        value={typeof value === "string" ? value : ""}
        disabled={disabled}
        onValueChange={(next) => onChange(field.name, next)}
      >
        <SelectTrigger id={inputId} className="max-w-md cursor-pointer">
          <SelectValue placeholder="Select..." />
        </SelectTrigger>
        <SelectContent>
          {(field.enumValues ?? []).map((option) => (
            <SelectItem key={option} value={option} className="cursor-pointer">
              {option}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  return (
    <Input
      id={inputId}
      type={inputType(field)}
      value={typeof value === "string" ? value : ""}
      disabled={disabled}
      autoComplete={field.secret ? "off" : undefined}
      className="max-w-md"
      onChange={(event) => onChange(field.name, event.target.value)}
    />
  );
}

function inputType(field: PluginConfigField): string {
  if (field.secret) return "password";
  if (field.type === "number" || field.type === "integer") return "number";
  return "text";
}
