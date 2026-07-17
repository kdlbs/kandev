"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import type { PluginConfigField } from "@/lib/plugins/config-schema";

/** Sentinel for the "Not set" select item — an actual enum option can never
 * collide with it, and Radix rejects value="" items. */
const ENUM_UNSET_SENTINEL = "__kandev_enum_unset__";

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
    // Radix Select forbids an item with value="", so an explicit "Not set"
    // sentinel lets optional enums be cleared back to unset (serialization
    // omits empty strings).
    return (
      <Select
        value={typeof value === "string" ? value : ""}
        disabled={disabled}
        onValueChange={(next) => onChange(field.name, next === ENUM_UNSET_SENTINEL ? "" : next)}
      >
        <SelectTrigger id={inputId} className="max-w-md cursor-pointer">
          <SelectValue placeholder="Select..." />
        </SelectTrigger>
        <SelectContent>
          {!field.required && (
            <SelectItem
              value={ENUM_UNSET_SENTINEL}
              className="cursor-pointer text-muted-foreground"
            >
              Not set
            </SelectItem>
          )}
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
      // step="1" on integer fields nudges the browser to flag non-integral
      // input in-place; serializeConfigValues still rejects it as a backstop.
      step={!field.secret && field.type === "integer" ? "1" : undefined}
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
